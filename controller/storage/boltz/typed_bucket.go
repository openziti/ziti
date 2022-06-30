/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package boltz

import (
	"bytes"
	"encoding/binary"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/ast"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"math"
	"reflect"
	"strconv"
	"time"
)

type FieldType byte

const (
	TypeBool    FieldType = 1
	TypeInt32   FieldType = 2
	TypeInt64   FieldType = 3
	TypeFloat64 FieldType = 4
	TypeString  FieldType = 5
	TypeTime    FieldType = 6
	TypeNil     FieldType = 7
)

const (
	ListSizeKeyName = "__list__size__36484231-110c-4767-afe2-01b6e3db107a"
)

type FieldChecker interface {
	IsUpdated(string) bool
}

type UpdatedFields interface {
	FieldChecker
	ToSlice() []string
}

func UpdatedFieldsToSlice(fields UpdatedFields) ([]string, error) {
	if fields == nil {
		return nil, nil
	}
	result := fields.ToSlice()
	if len(result) == 0 {
		return nil, errors.New("no fields updated, nothing to do")
	}
	return result, nil
}

func SliceToUpdatedFields(val []string) UpdatedFields {
	if len(val) == 0 {
		return nil
	}
	result := MapFieldChecker{}
	for _, k := range val {
		result[k] = struct{}{}
	}
	return result
}

type MapFieldChecker map[string]struct{}

func (m MapFieldChecker) IsUpdated(name string) bool {
	_, found := m[name]
	return found
}

func (m MapFieldChecker) ToSlice() []string {
	var result []string
	for k := range m {
		result = append(result, k)
	}
	return result
}

func NewMappedFieldChecker(checker FieldChecker, mappings map[string]string) FieldChecker {
	return &MappedFieldChecker{
		checker:  checker,
		mappings: mappings,
	}
}

type MappedFieldChecker struct {
	checker  FieldChecker
	mappings map[string]string
}

func (f *MappedFieldChecker) IsUpdated(field string) bool {
	if override, ok := f.mappings[field]; ok {
		return f.checker.IsUpdated(override)
	}
	return f.checker.IsUpdated(field)
}

func ErrBucket(err error) *TypedBucket {
	return &TypedBucket{ErrorHolderImpl: &errorz.ErrorHolderImpl{Err: err}}
}

func newRootTypedBucket(bucket *bbolt.Bucket) *TypedBucket {
	return newTypedBucket(nil, bucket)
}

func newTypedBucket(parent *TypedBucket, bucket *bbolt.Bucket) *TypedBucket {
	return &TypedBucket{
		Bucket:          bucket,
		parent:          parent,
		ErrorHolderImpl: &errorz.ErrorHolderImpl{},
	}
}

type TypedBucket struct {
	*bbolt.Bucket
	parent *TypedBucket
	*errorz.ErrorHolderImpl
}

func (bucket *TypedBucket) Tx() *bbolt.Tx {
	if bucket.Bucket == nil && bucket.parent != nil {
		return bucket.parent.Tx()
	}
	return bucket.Bucket.Tx()
}

func (bucket *TypedBucket) GetParent() *TypedBucket {
	return bucket.parent
}

func (bucket *TypedBucket) GetOrCreateBucket(name string) *TypedBucket {
	if bucket.Err != nil {
		return bucket
	}
	key := []byte(name)
	child := bucket.Bucket.Bucket(key)
	if child != nil {
		return newTypedBucket(bucket, child)
	}
	child, err := bucket.CreateBucketIfNotExists(key)
	if err != nil {
		return ErrBucket(err)
	}
	return newTypedBucket(bucket, child)
}

func (bucket *TypedBucket) GetBucket(name string) *TypedBucket {
	key := []byte(name)
	return bucket.GetBucketByKey(key)
}

func (bucket *TypedBucket) GetBucketByKey(key []byte) *TypedBucket {
	if bucket == nil {
		return nil
	}
	if bucket.HasError() {
		return bucket
	}
	child := bucket.Bucket.Bucket(key)
	if child == nil {
		return nil
	}
	return newTypedBucket(bucket, child)
}

func (bucket *TypedBucket) DeleteEntity(id string) {
	if bucket.Err == nil {
		bucket.Err = bucket.DeleteBucket([]byte(id))
	}
}

func (bucket *TypedBucket) DeleteValue(key []byte) *TypedBucket {
	if bucket.Err == nil {
		bucket.Err = bucket.Delete(key)
	}
	return bucket
}

func (bucket *TypedBucket) PutValue(key []byte, value []byte) *TypedBucket {
	if bucket.Err == nil {
		bucket.Err = bucket.Put(key, value)
	}
	return bucket
}

func (bucket *TypedBucket) GetPath(path ...string) *TypedBucket {
	if bucket == nil {
		return nil
	}
	if bucket.HasError() {
		return bucket
	}
	if len(path) == 0 {
		return bucket
	}
	next := bucket
	for _, pathElem := range path {
		next = next.GetBucket(pathElem)
		if next == nil {
			return nil
		}
	}
	return next
}

func (bucket *TypedBucket) GetOrCreatePath(path ...string) *TypedBucket {
	if bucket.HasError() {
		return bucket
	}
	if len(path) == 0 {
		return bucket
	}
	next := bucket
	for _, pathElem := range path {
		next = next.GetOrCreateBucket(pathElem)
		if next.Err != nil {
			return next
		}
	}
	return next
}

func (bucket *TypedBucket) EmptyBucket(name string) (*TypedBucket, error) {
	key := []byte(name)
	child := bucket.Bucket.Bucket(key)
	if child != nil {
		err := bucket.DeleteBucket(key)
		if err != nil {
			return nil, err
		}
	}
	child, err := bucket.CreateBucketIfNotExists(key)
	if err != nil {
		return nil, err
	}
	return newTypedBucket(bucket, child), nil
}

func GetTypeAndValue(bytes []byte) (FieldType, []byte) {
	if len(bytes) == 0 {
		return TypeNil, nil
	}

	fieldType := FieldType(bytes[0])
	if len(bytes) > 1 {
		return fieldType, bytes[1:]
	}
	return fieldType, nil
}

func (bucket *TypedBucket) getTyped(name string) (FieldType, []byte) {
	var fieldBytes []byte
	if bucket.Bucket != nil {
		fieldBytes = bucket.Get([]byte(name))
	}
	return GetTypeAndValue(fieldBytes)
}

func PrependFieldType(fieldType FieldType, value []byte) []byte {
	destBuf := make([]byte, len(value)+1)
	destBuf[0] = byte(fieldType)
	copy(destBuf[1:], value)
	return destBuf
}

func (bucket *TypedBucket) setTyped(fieldType FieldType, name string, value []byte) {
	if fieldType == TypeNil || value == nil {
		bucket.Err = bucket.Put([]byte(name), []byte{byte(TypeNil)})
	} else {
		bucket.Err = bucket.Put([]byte(name), PrependFieldType(fieldType, value))
	}
}

func BytesToString(buf []byte) *string {
	result := string(clone(buf))
	return &result
}

func FieldToString(fieldType FieldType, value []byte) *string {
	switch fieldType {
	case TypeString:
		return BytesToString(value)
	case TypeBool:
		boolVal := FieldToBool(fieldType, value)
		result := strconv.FormatBool(*boolVal)
		return &result
	case TypeInt32, TypeInt64:
		intVal := FieldToInt64(fieldType, value)
		result := strconv.Itoa(int(*intVal))
		return &result
	case TypeFloat64:
		floatVal := FieldToFloat64(fieldType, value)
		result := strconv.FormatFloat(*floatVal, 'f', -1, 64)
		return &result
	case TypeTime:
		if timeVal := FieldToDatetime(fieldType, value, "string"); timeVal != nil {
			result, err := timeVal.MarshalText()
			if err == nil {
				strResult := string(result)
				return &strResult
			}
		}
	case TypeNil:
		return nil
	}
	return nil
}

func (bucket *TypedBucket) GetString(name string) *string {
	fieldType, value := bucket.getTyped(name)
	return FieldToString(fieldType, value)
}

func (bucket *TypedBucket) GetStringWithDefault(name string, defaultValue string) string {
	value := bucket.GetString(name)
	if value == nil {
		return defaultValue
	}
	return *value
}

func (bucket *TypedBucket) GetStringOrError(name string) string {
	value := bucket.GetString(name)
	if value == nil {
		bucket.SetError(errors.Errorf("non-nullable field %v is null", name))
		return ""
	}
	return *value
}

func (bucket *TypedBucket) GetAndSetString(name string, value string, fieldChecker FieldChecker) (*string, bool) {
	if bucket.ProceedWithSet(name, fieldChecker) {
		oldValue := bucket.GetString(name)
		bucket.setTyped(TypeString, name, []byte(value))
		if oldValue == nil {
			return nil, true
		}
		return oldValue, *oldValue != value
	}
	return nil, false
}

func (bucket *TypedBucket) SetString(name string, value string, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		bucket.setTyped(TypeString, name, []byte(value))
	}
	return bucket
}

func (bucket *TypedBucket) SetStringP(name string, value *string, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		if value == nil {
			bucket.SetNil(name)
		} else {
			bucket.setTyped(TypeString, name, []byte(*value))
		}
	}
	return bucket
}

func BytesToBool(value []byte) *bool {
	if len(value) == 0 {
		return nil
	}
	result := value[0] == 1
	return &result
}

func FieldToBool(fieldType FieldType, value []byte) *bool {
	if fieldType == TypeBool {
		return BytesToBool(value)
	}
	return nil
}

func (bucket *TypedBucket) GetBoolWithDefault(name string, defaultValue bool) bool {
	val := bucket.GetBool(name)
	if val != nil {
		return *val
	}
	return defaultValue
}

func (bucket *TypedBucket) GetBool(name string) *bool {
	fieldType, value := bucket.getTyped(name)
	return FieldToBool(fieldType, value)
}

func (bucket *TypedBucket) SetBool(name string, value bool, checker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, checker) {
		buf := make([]byte, 2) // 1 byte for type + 1 byte for value
		buf[0] = byte(TypeBool)
		if value {
			buf[1] = 1
		}
		bucket.Err = bucket.Put([]byte(name), buf)
	}
	return bucket
}

func BytesToInt64(buf []byte) *int64 {
	if len(buf) != 8 {
		return nil
	}
	result := int64(binary.LittleEndian.Uint64(buf))
	return &result
}

func FieldToInt64(fieldType FieldType, value []byte) *int64 {
	switch fieldType {
	case TypeInt32:
		int32val := BytesToInt32(value)
		if int32val == nil {
			return nil
		}
		result := int64(*int32val)
		return &result
	case TypeInt64:
		return BytesToInt64(value)
	}
	return nil
}

func (bucket *TypedBucket) GetInt64(name string) *int64 {
	fieldType, value := bucket.getTyped(name)
	return FieldToInt64(fieldType, value)
}

func (bucket *TypedBucket) GetInt64WithDefault(name string, defaultValue int64) int64 {
	result := bucket.GetInt64(name)
	if result == nil {
		return defaultValue
	}
	return *result
}

func BytesToFloat64(buf []byte) *float64 {
	if len(buf) != 8 {
		return nil
	}
	result := math.Float64frombits(binary.LittleEndian.Uint64(buf))
	return &result
}

func FieldToFloat64(fieldType FieldType, value []byte) *float64 {
	switch fieldType {
	case TypeInt32, TypeInt64:
		int64Result := FieldToInt64(fieldType, value)
		if int64Result == nil {
			return nil
		}
		result := float64(*int64Result)
		return &result
	case TypeFloat64:
		return BytesToFloat64(value)
	}
	return nil
}

func (bucket *TypedBucket) GetFloat64(name string) *float64 {
	fieldType, value := bucket.getTyped(name)
	return FieldToFloat64(fieldType, value)
}

func (bucket *TypedBucket) SetFloat64(name string, value float64, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		buf := make([]byte, 9) // 1 byte for type + 8 bytes for float64
		buf[0] = byte(TypeFloat64)
		binary.LittleEndian.PutUint64(buf[1:], math.Float64bits(value))
		bucket.Err = bucket.Put([]byte(name), buf)
	}
	return bucket
}

func (bucket *TypedBucket) SetInt64(name string, value int64, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		buf := make([]byte, 9) // 1 byte for type + 8 bytes for int64
		buf[0] = byte(TypeInt64)
		binary.LittleEndian.PutUint64(buf[1:], uint64(value))
		bucket.Err = bucket.Put([]byte(name), buf)
	}
	return bucket
}

func BytesToInt32(buf []byte) *int32 {
	if len(buf) != 4 {
		return nil
	}
	result := int32(binary.LittleEndian.Uint32(buf))
	return &result
}

func FieldToInt32(fieldType FieldType, value []byte) *int32 {
	switch fieldType {
	case TypeInt32:
		return BytesToInt32(value)
	}
	return nil
}

func (bucket *TypedBucket) GetInt32(name string) *int32 {
	fieldType, value := bucket.getTyped(name)
	switch fieldType {
	case TypeInt32:
		return BytesToInt32(value)
	}
	return nil
}

func (bucket *TypedBucket) GetInt32WithDefault(name string, defaultValue int32) int32 {
	result := bucket.GetInt32(name)
	if result == nil {
		return defaultValue
	}
	return *result
}

func Int32ToBytes(value int32) []byte {
	buf := make([]byte, 5) // 1 byte for type + 4 bytes for int32
	buf[0] = byte(TypeInt32)
	binary.LittleEndian.PutUint32(buf[1:], uint32(value))
	return buf
}

func (bucket *TypedBucket) SetInt32(name string, value int32, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		buf := Int32ToBytes(value)
		bucket.Err = bucket.Put([]byte(name), buf)
	}
	return bucket
}

func BytesToDatetime(buf []byte, name string) *time.Time {
	result := &time.Time{}
	if buf == nil {
		return nil
	}
	if err := result.UnmarshalBinary(buf); err != nil {
		pfxlog.Logger().Errorf("failed to convert time for %v to time.Time", name)
		return nil
	}
	return result
}

func FieldToDatetime(fieldType FieldType, value []byte, name string) *time.Time {
	if fieldType == TypeTime {
		return BytesToDatetime(value, name)
	}
	return nil
}

func (bucket *TypedBucket) GetTime(name string) *time.Time {
	fieldType, value := bucket.getTyped(name)
	return FieldToDatetime(fieldType, value, name)
}

func (bucket *TypedBucket) GetTimeOrError(name string) time.Time {
	fieldType, value := bucket.getTyped(name)
	result := FieldToDatetime(fieldType, value, name)
	if result == nil {
		bucket.SetError(errors.Errorf("non-nullable field %v is null", name))
		return time.Time{}
	}
	return *result
}

func (bucket *TypedBucket) SetTime(name string, value time.Time, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		if fieldBytes, err := value.UTC().MarshalBinary(); err == nil {
			bucket.setTyped(TypeTime, name, fieldBytes)
		} else {
			bucket.Err = err
		}
	}
	return bucket
}

func (bucket *TypedBucket) SetTimeP(name string, value *time.Time, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		if value == nil {
			bucket.SetNil(name)
		} else if fieldBytes, err := value.UTC().MarshalBinary(); err == nil {
			bucket.setTyped(TypeTime, name, fieldBytes)
		} else {
			bucket.Err = err
		}
	}
	return bucket
}

func (bucket *TypedBucket) SetNil(name string) {
	if bucket.Err == nil {
		bucket.setTyped(TypeNil, name, nil)
	}
}

func (bucket *TypedBucket) GetStringList(name string) []string {
	listBucket := bucket.GetBucket(name)
	if listBucket == nil {
		return nil
	}
	return listBucket.ReadStringList()
}

func (bucket *TypedBucket) ReadStringList() []string {
	var result []string
	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		_, val := GetTypeAndValue(key)
		result = append(result, string(val))
	}
	return result
}

func (bucket *TypedBucket) IterateStringList() ast.SeekableSetCursor {
	return NewTypedForwardBoltCursor(bucket.Cursor(), TypeString)
}

func (bucket *TypedBucket) IsStringListEmpty(name string) bool {
	listBucket := bucket.GetBucket(name)
	if listBucket == nil {
		return true
	}
	cursor := listBucket.Cursor()
	if key, _ := cursor.First(); key == nil {
		return true
	}
	return false
}

func (bucket *TypedBucket) GetAndSetStringList(name string, value []string, fieldChecker FieldChecker) ([]string, bool) {
	result := bucket.GetStringList(name)
	if bucket.ProceedWithSet(name, fieldChecker) {
		listBucket, err := bucket.EmptyBucket(name)
		if err != nil {
			bucket.Err = err
			return result, true
		}

		for _, key := range value {
			if listBucket.SetListEntry(TypeString, []byte(key)).Err != nil {
				bucket.Err = listBucket.Err
				return result, true
			}
		}
		return result, true
	}
	return result, false
}

func (bucket *TypedBucket) SetStringList(name string, value []string, fieldChecker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, fieldChecker) {
		listBucket, err := bucket.EmptyBucket(name)
		if err != nil {
			bucket.Err = err
			return bucket
		}

		for _, key := range value {
			if listBucket.SetListEntry(TypeString, []byte(key)).Err != nil {
				bucket.Err = listBucket.Err
				return bucket
			}
		}
	}
	return bucket
}

func (bucket *TypedBucket) CheckAndSetListEntry(fieldType FieldType, value []byte) (bool, error) {
	if !bucket.HasError() {
		key := PrependFieldType(fieldType, value)
		if !bucket.IsKeyPresent(key) {
			if bucket.SetError(bucket.Put(key, nil)) {
				return false, bucket.Err
			}
			return true, nil
		}
	}
	return false, bucket.GetError()
}

func (bucket *TypedBucket) SetLinkCount(fieldType FieldType, value []byte, count int) (*int32, error) {
	if !bucket.HasError() {
		key := string(PrependFieldType(fieldType, value))
		current := bucket.GetInt32(key)
		if current == nil && count == 0 {
			return nil, nil
		}
		if current != nil && count == 0 {
			err := bucket.Delete([]byte(key))
			return current, err
		}
		bucket.SetInt32(key, int32(count), nil)
		return current, nil
	}
	return nil, bucket.GetError()
}

func (bucket *TypedBucket) IncrementLinkCount(fieldType FieldType, value []byte) (int, error) {
	result := 0
	if !bucket.HasError() {
		key := string(PrependFieldType(fieldType, value))
		next := int32(1)
		if current := bucket.GetInt32(key); current != nil {
			next = *current + 1
		}
		bucket.SetInt32(key, next, nil)
		result = int(next)
	}
	return result, bucket.GetError()
}

func (bucket *TypedBucket) DecrementLinkCount(fieldType FieldType, value []byte) (int, error) {
	result := -1
	if !bucket.HasError() {
		key := string(PrependFieldType(fieldType, value))
		if current := bucket.GetInt32(key); current != nil {
			next := *current - 1
			if next > 0 {
				bucket.SetInt32(key, next, nil)
			} else {
				bucket.SetError(bucket.Delete([]byte(key)))
			}
			result = int(next)
		}
	}
	return result, bucket.GetError()
}

func (bucket *TypedBucket) GetLinkCount(fieldType FieldType, value []byte) *int32 {
	if bucket.HasError() {
		return nil
	}
	return bucket.GetInt32(string(PrependFieldType(fieldType, value)))
}

func (bucket *TypedBucket) SetListEntry(fieldType FieldType, value []byte) *TypedBucket {
	if !bucket.HasError() {
		bucket.SetError(bucket.Put(PrependFieldType(fieldType, value), nil))
	}
	return bucket
}

func (bucket *TypedBucket) CheckAndDeleteListEntry(fieldType FieldType, value []byte) (bool, error) {
	if !bucket.HasError() {
		key := PrependFieldType(fieldType, value)
		if bucket.IsKeyPresent(key) {
			if bucket.SetError(bucket.Delete(key)) {
				return false, bucket.Err
			}
			return true, nil
		}
	}
	return false, bucket.GetError()
}

func (bucket *TypedBucket) DeleteListEntry(fieldType FieldType, value []byte) *TypedBucket {
	if !bucket.HasError() {
		bucket.SetError(bucket.Delete(PrependFieldType(fieldType, value)))
	}
	return bucket
}

func (bucket *TypedBucket) getMarshaled(name string) interface{} {
	// If there's a sub bucket, then this is either a nested map or list
	if childBucket := bucket.GetBucket(name); childBucket != nil {
		if listSize := childBucket.GetInt32(ListSizeKeyName); listSize != nil {
			return bucket.GetList(name)
		}
		return bucket.GetMap(name)
	}
	fieldType, value := bucket.getTyped(name)
	switch fieldType {
	case TypeString:
		result := BytesToString(value)
		if result == nil {
			return nil
		}
		return *result
	case TypeInt32:
		result := BytesToInt32(value)
		if result == nil {
			return nil
		}
		return *result
	case TypeInt64:
		result := BytesToInt64(value)
		if result == nil {
			return nil
		}
		return *result
	case TypeFloat64:
		result := BytesToFloat64(value)
		if result == nil {
			return nil
		}
		return *result
	case TypeTime:
		result := BytesToDatetime(value, name)
		if result == nil {
			return nil
		}
		return *result
	case TypeBool:
		result := BytesToBool(value)
		if result == nil {
			return nil
		}
		return *result
	}
	return nil
}

func (bucket *TypedBucket) setMarshaled(name string, value interface{}, allowNested bool) *TypedBucket {
	if bucket.Err != nil {
		return bucket
	}
	if value == nil {
		bucket.SetNil(name)
		return bucket
	}

	switch val := value.(type) {
	case string:
		bucket.SetString(name, val, nil)
	case int32:
		bucket.SetInt32(name, val, nil)
	case int64:
		bucket.SetInt64(name, val, nil)
	case int:
		bucket.SetInt64(name, int64(val), nil)
	case float32:
		bucket.SetFloat64(name, float64(val), nil)
	case float64:
		bucket.SetFloat64(name, val, nil)
	case time.Time:
		bucket.SetTime(name, val, nil)
	case bool:
		bucket.SetBool(name, val, nil)
	case map[string]interface{}:
		if allowNested {
			bucket.PutMap(name, val, nil, true)
		} else {
			bucket.SetError(errors.New("nested maps not supported"))
		}
	case []interface{}:
		if allowNested {
			bucket.PutList(name, val, nil)
		} else {
			bucket.SetError(errors.New("nested lists not supported"))
		}
	default:
		bucket.SetError(errors.Errorf("unsupported type %v in map", reflect.TypeOf(val)))
	}

	return bucket
}

func (bucket *TypedBucket) GetMap(name string) map[string]interface{} {
	result := make(map[string]interface{})
	mapBucket := bucket.GetBucket(name)
	if mapBucket != nil {
		cursor := mapBucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			tagKey := string(key)
			result[tagKey] = mapBucket.getMarshaled(tagKey)
		}
	}
	return result
}

func (bucket *TypedBucket) PutMap(name string, value map[string]interface{}, checker FieldChecker, allowNested bool) *TypedBucket {
	if bucket.ProceedWithSet(name, checker) {
		tagsBucket, err := bucket.EmptyBucket(name)
		if err != nil {
			bucket.Err = err
			return bucket
		}
		for key, val := range value {
			tagsBucket.setMarshaled(key, val, allowNested)
		}
		bucket.Err = tagsBucket.Err
	}
	return bucket
}

func (bucket *TypedBucket) PutList(name string, value []interface{}, checker FieldChecker) *TypedBucket {
	if bucket.ProceedWithSet(name, checker) {
		listBucket, err := bucket.EmptyBucket(name)
		if err != nil {
			bucket.Err = err
			return bucket
		}
		for idx, val := range value {
			key := Int32ToBytes(int32(idx))
			listBucket.setMarshaled(string(key), val, true)
		}
		listBucket.SetInt32(ListSizeKeyName, int32(len(value)), nil)
		bucket.Err = listBucket.Err
	}
	return bucket
}

func (bucket *TypedBucket) GetList(name string) []interface{} {
	listBucket := bucket.GetBucket(name)
	size := listBucket.GetInt32(ListSizeKeyName)
	if size == nil {
		return nil
	}

	result := make([]interface{}, *size)
	for idx := int32(0); idx < (*size); idx++ {
		key := Int32ToBytes(idx)
		value := listBucket.getMarshaled(string(key))
		result[idx] = value
	}

	return result
}

func (bucket *TypedBucket) ProceedWithSet(name string, checker FieldChecker) bool {
	return bucket.Err == nil && (checker == nil || checker.IsUpdated(name))
}

func (bucket *TypedBucket) OpenSeekableCursor() ast.SeekableSetCursor {
	return NewForwardBoltCursor(bucket.Cursor())
}

func (bucket *TypedBucket) OpenCursor(_ *bbolt.Tx, forward bool) ast.SetCursor {
	return NewBoltCursor(bucket.Cursor(), forward)
}

func (bucket *TypedBucket) OpenTypedCursor(_ *bbolt.Tx, forward bool) ast.SetCursor {
	if forward {
		return NewTypedForwardBoltCursor(bucket.Cursor(), TypeString)
	}
	return NewTypedReverseBoltCursor(bucket.Cursor(), TypeString)
}

func (bucket *TypedBucket) IsKeyPresent(key []byte) bool {
	result, _ := bucket.Cursor().Seek(key)
	return result != nil && bytes.Equal(key, result)
}

func clone(val []byte) []byte {
	if val == nil {
		return nil
	}
	result := make([]byte, len(val))
	copy(result, val)
	return result
}

func (bucket *TypedBucket) Copy(other *TypedBucket, filterF func(path []string) bool) error {
	return bucket.copyImpl(other, filterF, nil)
}

func (bucket *TypedBucket) copyImpl(other *TypedBucket, filterF func(path []string) bool, path []string) error {
	cursor := other.Cursor()
	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		keyName := string(key)
		if !filterF(append(path, keyName)) {
			continue
		}
		childBucket := other.Bucket.Bucket(key)
		if childBucket == nil {
			if err := bucket.Bucket.Put(key, value); err != nil {
				return err
			}
		} else {
			otherChild := other.GetBucket(keyName)
			localChild := bucket.GetOrCreateBucket(keyName)
			if err := localChild.copyImpl(otherChild, filterF, append(path, keyName)); err != nil {
				return err
			}
		}
	}
	return nil
}
