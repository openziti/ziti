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
	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
)

var _ EntitySymbol = (*entitySymbol)(nil)
var _ EntitySymbol = (*entityIdSymbol)(nil)
var _ EntitySymbol = (*entitySetSymbolImpl)(nil)

type entityMapSymbol struct {
	store      ListStore
	key        string
	symbolType ast.NodeType
	prefix     []string
}

func (self *entityMapSymbol) createElementSymbol(name string, parts []string) EntitySymbol {
	var prefix []string
	prefix = append(prefix, self.prefix...)
	prefix = append(prefix, self.key)
	if len(parts) > 2 {
		middle := parts[1 : len(parts)-1]
		prefix = append(prefix, middle...)
	}
	key := parts[len(parts)-1]
	return self.store.newEntitySymbol(name, self.symbolType, key, nil, prefix...)
}

type entitySymbol struct {
	store      ListStore
	name       string
	getBucketF func(entityBucket *TypedBucket) *TypedBucket
	symbolType ast.NodeType
	prefix     []string
	key        string
	path       []string
	linkedType ListStore // only set if this is an id field or set
}

func (symbol *entitySymbol) GetStore() ListStore {
	return symbol.store
}

func (symbol *entitySymbol) GetLinkedType() ListStore {
	return symbol.linkedType
}

func (symbol *entitySymbol) GetPath() []string {
	return symbol.path
}

func (symbol *entitySymbol) GetName() string {
	return symbol.name
}

func (symbol *entitySymbol) IsSet() bool {
	return false
}

func (symbol *entitySymbol) GetType() ast.NodeType {
	return symbol.symbolType
}

func (symbol *entitySymbol) Eval(tx *bbolt.Tx, rowId []byte) (FieldType, []byte) {
	entityBucket := symbol.getBucketF(symbol.store.GetEntityBucket(tx, rowId))
	if entityBucket == nil {
		return TypeNil, nil
	}
	return entityBucket.getTyped(symbol.key)
}

func (symbol *entitySymbol) getLinkedType() ListStore {
	return symbol.linkedType
}

func (symbol *entitySymbol) newQueryPath(index int, cursor *stackedCursor, key []byte) queryPathElem {
	val := PrependFieldType(symbol.Eval(cursor.tx, key))
	return &fkQueryPath{index: index, value: val}
}

type fkQueryPath struct {
	index int
	value []byte
}

func (elem *fkQueryPath) Next() []byte {
	result := elem.value
	elem.value = nil
	return result
}

func (elem *fkQueryPath) Index() int {
	return elem.index
}

type entityIdSymbol struct {
	store      ListStore
	symbolType ast.NodeType
	path       []string
}

func (symbol *entityIdSymbol) GetStore() ListStore {
	return symbol.store
}

func (symbol *entityIdSymbol) GetLinkedType() ListStore {
	return nil
}

func (symbol *entityIdSymbol) GetPath() []string {
	return symbol.path
}

func (symbol *entityIdSymbol) GetName() string {
	return "id"
}

func (symbol *entityIdSymbol) IsSet() bool {
	return false
}

func (symbol *entityIdSymbol) GetType() ast.NodeType {
	return symbol.symbolType
}

func (symbol *entityIdSymbol) Eval(_ *bbolt.Tx, rowId []byte) (FieldType, []byte) {
	return TypeString, rowId
}

type entitySetSymbolImpl struct {
	*entitySymbol
}

func (symbol *entitySetSymbolImpl) GetRuntimeSymbol() RuntimeEntitySetSymbol {
	return &entitySetSymbolRuntime{
		entitySetSymbolImpl: symbol,
	}
}

func (symbol *entitySetSymbolImpl) openBoltCursor(tx *bbolt.Tx, key []byte) *bbolt.Cursor {
	bucket := symbol.getBucketF(symbol.store.GetEntityBucket(tx, key))
	if bucket != nil {
		bucket = bucket.GetBucket(symbol.key)
	}
	if bucket != nil {
		return bucket.Cursor()
	}
	return nil
}

func (symbol *entitySetSymbolImpl) EvalStringList(tx *bbolt.Tx, key []byte) []string {
	bucket := symbol.getBucketF(symbol.store.GetEntityBucket(tx, key))
	if bucket == nil {
		return nil
	}
	return bucket.GetStringList(symbol.key)
}

func (symbol *entitySetSymbolImpl) Map(tx *bbolt.Tx, key []byte, f func(ctx *MapContext)) error {
	bucket := symbol.getBucketF(symbol.store.GetEntityBucket(tx, key))
	if bucket == nil {
		return nil
	}
	listBucket := bucket.GetBucket(symbol.key)
	if listBucket == nil {
		return nil
	}

	var newValues []*FieldTypeAndValue
	cursor := listBucket.Cursor()
	ctx := &MapContext{}

	var toDelete [][]byte
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		ctx.next(GetTypeAndValue(key))
		f(ctx)
		if ctx.HasError() {
			return ctx.GetError()
		}
		if ctx.replace {
			toDelete = append(toDelete, key)
			if ctx.newVal != nil {
				newValues = append(newValues, &FieldTypeAndValue{FieldType: ctx.newType, Value: ctx.newVal})
			}
		}
		if ctx.stop {
			break
		}
	}

	for _, key := range toDelete {
		if err := listBucket.Delete(key); err != nil {
			return err
		}
	}

	for _, val := range newValues {
		listBucket.SetListEntry(val.FieldType, val.Value)
	}
	return listBucket.Err
}

func (symbol *entitySetSymbolImpl) newQueryPath(index int, cursor *stackedCursor, key []byte) queryPathElem {
	boltCursor := symbol.openBoltCursor(cursor.tx, key)
	var next []byte
	if boltCursor != nil {
		next, _ = boltCursor.First()
	}
	result := &fkSetQueryPath{
		index:  index,
		cursor: boltCursor,
		next:   next,
	}
	return result
}

func (symbol *entitySetSymbolImpl) IsSet() bool {
	return true
}

func (symbol *entitySetSymbolImpl) Eval(_ *bbolt.Tx, _ []byte) (FieldType, []byte) {
	return 0, nil
}

type fkSetQueryPath struct {
	index  int
	cursor *bbolt.Cursor
	next   []byte
}

func (elem *fkSetQueryPath) Next() []byte {
	result := elem.next
	if elem.cursor != nil {
		elem.next, _ = elem.cursor.Next()
	}
	return result
}

func (elem *fkSetQueryPath) Index() int {
	return elem.index
}

type entitySetSymbolRuntime struct {
	*entitySetSymbolImpl
	cursor *bbolt.Cursor
	value  []byte
}

func (symbol *entitySetSymbolRuntime) Current() []byte {
	_, value := GetTypeAndValue(symbol.value)
	return value
}

func (symbol *entitySetSymbolRuntime) Next() {
	if symbol.cursor != nil {
		symbol.value, _ = symbol.cursor.Next()
	}
}

func (symbol *entitySetSymbolRuntime) Seek(val []byte) {
	if symbol.cursor != nil {
		symbol.value, _ = symbol.cursor.Seek(val)
	}
}

func (symbol *entitySetSymbolRuntime) SeekToString(val string) {
	if symbol.cursor != nil {
		seekVal := PrependFieldType(TypeString, []byte(val))
		symbol.value, _ = symbol.cursor.Seek(seekVal)
	}
}

func (symbol *entitySetSymbolRuntime) IsValid() bool {
	return symbol.value != nil
}

func (symbol *entitySetSymbolRuntime) OpenCursor(tx *bbolt.Tx, rowId []byte) ast.SetCursor {
	symbol.cursor = symbol.openBoltCursor(tx, rowId)
	if symbol.cursor != nil {
		symbol.value, _ = symbol.cursor.First()
	} else {
		symbol.value = nil
	}
	return symbol
}

func (symbol *entitySetSymbolRuntime) Eval(_ *bbolt.Tx, _ []byte) (FieldType, []byte) {
	if symbol.value == nil {
		return TypeNil, nil
	}
	return GetTypeAndValue(symbol.value)
}

type iterableEntitySymbol interface {
	EntitySymbol
	newQueryPath(index int, cursor *stackedCursor, key []byte) queryPathElem
}

type linkedEntitySymbol interface {
	iterableEntitySymbol
	getLinkedType() ListStore
}

type compositeEntitySymbol interface {
	EntitySymbol
	getChain() []EntitySymbol
}

type nonSetCompositeEntitySymbol struct {
	name       string
	symbolType ast.NodeType
	chain      []EntitySymbol
}

func (symbol *nonSetCompositeEntitySymbol) GetChain() []EntitySymbol {
	return symbol.chain
}

func (symbol *nonSetCompositeEntitySymbol) GetStore() ListStore {
	return symbol.chain[0].GetStore()
}

func (symbol *nonSetCompositeEntitySymbol) GetLinkedType() ListStore {
	return symbol.chain[len(symbol.chain)-1].GetLinkedType()
}

func (symbol *nonSetCompositeEntitySymbol) GetPath() []string {
	return symbol.chain[0].GetPath()
}

func (symbol *nonSetCompositeEntitySymbol) GetName() string {
	return symbol.name
}

func (symbol *nonSetCompositeEntitySymbol) IsSet() bool {
	return false
}

func (symbol *nonSetCompositeEntitySymbol) GetType() ast.NodeType {
	return symbol.symbolType
}

func (symbol *nonSetCompositeEntitySymbol) Eval(tx *bbolt.Tx, rowId []byte) (FieldType, []byte) {
	currentValue := rowId
	var fieldType FieldType
	for _, current := range symbol.chain {
		fieldType, currentValue = current.Eval(tx, currentValue)
	}
	return fieldType, currentValue
}

type compositeEntitySetSymbol struct {
	name        string
	symbolType  ast.NodeType
	chain       []iterableEntitySymbol
	cursor      *stackedCursor
	cursorLastF func(tx *bbolt.Tx, key []byte) (FieldType, []byte)
}

func (symbol *compositeEntitySetSymbol) getChain() []EntitySymbol {
	var result []EntitySymbol
	for _, chainSymbol := range symbol.chain {
		result = append(result, chainSymbol)
	}
	return result
}

func (symbol *compositeEntitySetSymbol) GetStore() ListStore {
	return symbol.chain[0].GetStore()
}

func (symbol *compositeEntitySetSymbol) GetLinkedType() ListStore {
	return symbol.chain[len(symbol.chain)-1].GetLinkedType()
}

func (symbol *compositeEntitySetSymbol) GetPath() []string {
	return symbol.chain[0].GetPath()
}

func (symbol *compositeEntitySetSymbol) GetName() string {
	return symbol.name
}

func (symbol *compositeEntitySetSymbol) OpenCursor(tx *bbolt.Tx, rowId []byte) ast.SetCursor {
	stackCursor := &stackedCursor{
		symbol: symbol,
		tx:     tx,
		stack:  make([]queryPathElem, len(symbol.chain)),
	}
	nextPathElem := symbol.chain[0].newQueryPath(0, stackCursor, rowId)
	stackCursor.stack[0] = nextPathElem
	calculateNextCursorPosition(stackCursor, nextPathElem, nextPathElem.Next())
	symbol.cursor = stackCursor
	return symbol.cursor
}

func (symbol *compositeEntitySetSymbol) IsSet() bool {
	return true
}

func (symbol *compositeEntitySetSymbol) GetType() ast.NodeType {
	return symbol.symbolType
}

func (symbol *compositeEntitySetSymbol) Eval(tx *bbolt.Tx, _ []byte) (FieldType, []byte) {
	return symbol.cursorLastF(tx, symbol.cursor.key)
}

func calculateNextCursorPosition(stackCursor *stackedCursor, stackElem queryPathElem, key []byte) {
	for {
		if key == nil { // end of this level of cursor
			if stackElem.Index() == 0 { // end of total cursor
				stackCursor.key = nil
				return
			}

			// back up the stack, advance that cursor and start the loop over
			stackElem = stackCursor.stack[stackElem.Index()-1]
			key = stackElem.Next()
			continue
		}

		if stackElem.Index()+1 == len(stackCursor.stack) { // we're at the top of the stack and have a valid next value
			stackCursor.key = key
			return
		}

		nextKey := key

		// Hop up the stack
		index := stackElem.Index() + 1
		nextLink := stackCursor.symbol.chain[index]
		_, rowKey := GetTypeAndValue(nextKey)
		stackElem = nextLink.newQueryPath(index, stackCursor, rowKey)
		stackCursor.stack[index] = stackElem
		key = stackElem.Next()
	}
}

type queryPathElem interface {
	Next() []byte
	Index() int
}

type stackedCursor struct {
	symbol *compositeEntitySetSymbol
	tx     *bbolt.Tx
	stack  []queryPathElem
	key    []byte
}

func (cursor *stackedCursor) Current() []byte {
	_, value := GetTypeAndValue(cursor.key)
	return value
}

func (cursor *stackedCursor) Next() {
	if cursor.IsValid() {
		topStackElem := cursor.stack[len(cursor.stack)-1]
		key := topStackElem.Next()
		calculateNextCursorPosition(cursor, topStackElem, key)
	}
}

func (cursor *stackedCursor) IsValid() bool {
	return cursor.key != nil
}
