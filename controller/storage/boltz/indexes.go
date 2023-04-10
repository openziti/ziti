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
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Constrained interface {
	AddConstraint(constraint Constraint)
}

type Indexer struct {
	constraints []Constraint
	basePath    []string
}

type FieldTypeAndValue struct {
	FieldType
	Value []byte
}

type IndexingContext struct {
	Parent *IndexingContext
	*Indexer
	IsCreate   bool
	Ctx        MutateContext
	RowId      []byte
	ErrHolder  errorz.ErrorHolder
	AtomStates map[Constraint][]byte
	SetStates  map[Constraint][]FieldTypeAndValue
}

func (ctx *IndexingContext) Tx() *bbolt.Tx {
	return ctx.Ctx.Tx()
}

func (ctx *IndexingContext) PushState(constraint Constraint, fieldType FieldType, fieldValue []byte) {
	states := ctx.SetStates[constraint]
	ctx.SetStates[constraint] = append(states, FieldTypeAndValue{
		FieldType: fieldType,
		Value:     fieldValue,
	})
}

func (ctx *IndexingContext) PopState(constraint Constraint) *FieldTypeAndValue {
	states := ctx.SetStates[constraint]
	if len(states) == 0 {
		return nil
	}
	result := states[0]
	ctx.SetStates[constraint] = states[1:]
	return &result
}

func (ctx *IndexingContext) PopStateString(constraint Constraint) string {
	fieldAndValue := ctx.PopState(constraint)
	if fieldAndValue == nil {
		return ""
	}
	result := FieldToString(fieldAndValue.FieldType, fieldAndValue.Value)
	return stringz.OrEmpty(result)
}

func (ctx *IndexingContext) PopStateBool(constraint Constraint) bool {
	fieldAndValue := ctx.PopState(constraint)
	if fieldAndValue == nil {
		return false
	}
	result := FieldToBool(fieldAndValue.FieldType, fieldAndValue.Value)
	return result != nil && *result
}

func NewIndexer(basePath ...string) *Indexer {
	return &Indexer{basePath: basePath}
}

func (indexer *Indexer) AddUniqueIndex(symbol EntitySymbol) ReadIndex {
	return indexer.addUniqueIndex(symbol, false)
}

func (indexer *Indexer) AddNullableUniqueIndex(symbol EntitySymbol) ReadIndex {
	return indexer.addUniqueIndex(symbol, true)
}

func (indexer *Indexer) addUniqueIndex(symbol EntitySymbol, nullable bool) ReadIndex {
	index := &uniqueIndex{
		symbol:    symbol,
		nullable:  nullable,
		indexPath: indexer.getIndexPath(symbol),
	}

	indexer.constraints = append(indexer.constraints, index)
	return index
}

func (indexer *Indexer) AddSetIndex(symbol EntitySetSymbol) SetReadIndex {
	index := &setIndex{
		symbol:    symbol,
		indexPath: indexer.getIndexPath(symbol),
	}
	indexer.constraints = append(indexer.constraints, index)
	return index
}

func (indexer *Indexer) AddFkIndex(symbol EntitySymbol, fkSymbol EntitySetSymbol) {
	indexer.addFkIndex(symbol, fkSymbol, false)
}

func (indexer *Indexer) AddNullableFkIndex(symbol EntitySymbol, fkSymbol EntitySetSymbol) {
	indexer.addFkIndex(symbol, fkSymbol, true)
}

func (indexer *Indexer) addFkIndex(symbol EntitySymbol, fkSymbol EntitySetSymbol, nullable bool) {
	index := &fkIndex{
		symbol:   symbol,
		nullable: nullable,
		fkSymbol: fkSymbol,
	}

	indexer.AddConstraint(index)
	fkStore := fkSymbol.GetStore()
	if baseStore, ok := fkStore.(Constrained); ok {
		baseStore.AddConstraint(&fkDeleteConstraint{
			symbol:   fkSymbol,
			fkSymbol: symbol,
		})
	} else {
		panic(errors.Errorf("linked store %v is not constrained, can't enforce validity of constraint on delete",
			fkSymbol.GetStore().GetEntityType()))
	}
}

func (indexer *Indexer) AddFkConstraint(symbol EntitySymbol, nullable bool, cascade CascadeType) {
	if symbol.GetLinkedType() == nil {
		panic(errors.Errorf("invalid symbol for foreign key: %v.%v. It is not linked to another store",
			symbol.GetStore().GetEntityType(), symbol.GetName()))
	}

	index := &fkConstraint{
		symbol:   symbol,
		nullable: nullable,
	}

	indexer.AddConstraint(index)

	if cascade != CascadeCreateUpdate {
		if baseStore, ok := symbol.GetLinkedType().(Constrained); ok {
			baseStore.AddConstraint(&fkDeleteCascadeConstraint{
				symbol:      symbol,
				cascadeType: cascade,
			})
		} else {
			panic(errors.Errorf("linked store %v is not constrained, can't enforce validity of constraint on delete",
				symbol.GetLinkedType().GetEntityType()))
		}
	}
}

func (indexer *Indexer) AddConstraint(constraint Constraint) {
	indexer.constraints = append(indexer.constraints, constraint)
}

func (ctx *IndexingContext) ProcessBeforeUpdate() {
	if ctx.Parent != nil {
		ctx.Parent.ProcessBeforeUpdate()
	}

	if !ctx.ErrHolder.HasError() {
		for _, index := range ctx.constraints {
			index.ProcessBeforeUpdate(ctx)
		}
	}
}

func (ctx *IndexingContext) ProcessAfterUpdate() {
	if ctx.Parent != nil {
		ctx.Parent.ProcessAfterUpdate()
	}

	if !ctx.ErrHolder.HasError() {
		for _, index := range ctx.constraints {
			index.ProcessAfterUpdate(ctx)
		}
	}
}

func (ctx *IndexingContext) ProcessBeforeDelete() {
	if ctx.Parent != nil {
		ctx.Parent.ProcessBeforeDelete()
	}

	if !ctx.ErrHolder.HasError() {
		for _, index := range ctx.constraints {
			index.ProcessBeforeDelete(ctx)
		}
	}
}

func (indexer *Indexer) InitializeIndexes(tx *bbolt.Tx, errorHolder errorz.ErrorHolder) {
	if !errorHolder.HasError() {
		for _, index := range indexer.constraints {
			index.Initialize(tx, errorHolder)
		}
	}
}

func (indexer *Indexer) getIndexPath(symbol EntitySymbol) []string {
	var result []string
	result = append(result, indexer.basePath...)
	result = append(result, symbol.GetStore().GetEntityType(), symbol.GetName())
	return result
}

type ReadIndex interface {
	Read(tx *bbolt.Tx, val []byte) []byte
}

type SetChangeListener func(ctx MutateContext, rowId []byte, old []FieldTypeAndValue, new []FieldTypeAndValue, holder errorz.ErrorHolder)

type SetReadIndex interface {
	GetSymbol() EntitySetSymbol
	Read(tx *bbolt.Tx, key []byte, f func(val []byte))
	ReadKeys(tx *bbolt.Tx, f func(val []byte))
	OpenValueCursor(tx *bbolt.Tx, key []byte, forward bool) ast.SetCursor
	OpenKeyCursor(tx *bbolt.Tx, forward bool) ast.SetCursor
	AddListener(listener SetChangeListener)
}

type Constraint interface {
	Checkable
	ProcessBeforeUpdate(ctx *IndexingContext)
	ProcessAfterUpdate(ctx *IndexingContext)
	ProcessBeforeDelete(ctx *IndexingContext)
	Initialize(tx *bbolt.Tx, errorHolder errorz.ErrorHolder)
}

type uniqueIndex struct {
	symbol    EntitySymbol
	nullable  bool
	indexPath []string
}

func (index *uniqueIndex) Read(tx *bbolt.Tx, val []byte) []byte {
	indexBucket := index.getIndexBucket(tx)
	if indexBucket.Err != nil {
		return nil
	}
	return indexBucket.Get(val)
}

func (index *uniqueIndex) getIndexBucket(tx *bbolt.Tx) *TypedBucket {
	indexBucket := Path(tx, index.indexPath...)
	if indexBucket != nil {
		return indexBucket
	}
	return GetOrCreatePath(tx, index.indexPath...)
}

func (index *uniqueIndex) Initialize(tx *bbolt.Tx, errorHolder errorz.ErrorHolder) {
	if !errorHolder.HasError() {
		bucket := GetOrCreatePath(tx, index.indexPath...)
		errorHolder.SetError(bucket.Err)
	}
}

func (index *uniqueIndex) ProcessBeforeUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		_, fieldValue := index.symbol.Eval(ctx.Tx(), ctx.RowId)
		ctx.AtomStates[index] = fieldValue
	}
}

func (index *uniqueIndex) ProcessAfterUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		_, newValue := index.symbol.Eval(ctx.Tx(), ctx.RowId)
		oldValue := ctx.AtomStates[index]

		if !ctx.IsCreate && bytes.Equal(oldValue, newValue) {
			return
		}

		indexBucket := index.getIndexBucket(ctx.Tx())

		if len(oldValue) > 0 {
			ctx.ErrHolder.SetError(indexBucket.DeleteValue(oldValue).Err)
		}

		if len(newValue) > 0 {
			if indexBucket.Get(newValue) != nil {
				ctx.ErrHolder.SetError(&UniqueIndexDuplicateError{
					Field:      index.symbol.GetName(),
					Value:      string(newValue),
					EntityType: index.symbol.GetStore().GetEntityType(),
				})
			} else {
				ctx.ErrHolder.SetError(indexBucket.PutValue(newValue, ctx.RowId).Err)
			}
		} else if !index.nullable {
			ctx.ErrHolder.SetError(errors.Errorf("index on %v.%v does not allow null or empty values",
				index.symbol.GetStore().GetEntityType(), index.symbol.GetName()))
		}
	}
}

func (index *uniqueIndex) processIntegrityFix(ctx *IndexingContext) error {
	ctx.ErrHolder = &errorz.ErrorHolderImpl{}
	index.ProcessAfterUpdate(ctx)
	return ctx.ErrHolder.GetError()
}

func (index *uniqueIndex) ProcessBeforeDelete(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		if _, value := index.symbol.Eval(ctx.Tx(), ctx.RowId); len(value) > 0 {
			indexBucket := index.getIndexBucket(ctx.Tx())
			ctx.ErrHolder.SetError(indexBucket.DeleteValue(value).Err)
		}
	}
}

func (index *uniqueIndex) CheckIntegrity(ctx MutateContext, fix bool, errorSink func(error, bool)) error {
	tx := ctx.Tx()
	indexBucket := index.getIndexBucket(tx)
	cursor := indexBucket.Cursor()
	store := index.symbol.GetStore()
	for key, val := cursor.First(); key != nil; key, val = cursor.Next() {
		if !store.IsEntityPresent(tx, string(val)) {
			if fix {
				if err := cursor.Delete(); err != nil {
					return err
				}
			}
			errorSink(errors.Errorf("unique index %v.%v references %v for value %v, which doesn't exist",
				store.GetEntityType(), index.symbol.GetName(), string(val), string(key)), fix)
		} else {
			_, fieldVal := index.symbol.Eval(tx, val)
			if !bytes.Equal(key, fieldVal) {
				if fix {
					// just delete it here. It may be a duplicate. If it's not a duplicate, the correct value
					// will be created when we scan the other side
					if err := cursor.Delete(); err != nil {
						return err
					}
				}

				errorSink(errors.Errorf("unique index %v.%v references %v for value %v which should be %v",
					store.GetEntityType(), index.symbol.GetName(), string(val), string(key), string(fieldVal)), fix)
			}
		}
	}

	for entityCursor := index.symbol.GetStore().IterateValidIds(tx, ast.BoolNodeTrue); entityCursor.IsValid(); entityCursor.Next() {
		id := entityCursor.Current()
		_, fieldVal := index.symbol.Eval(tx, id)
		idxId := index.Read(tx, fieldVal)

		if idxId == nil {
			if fix {
				ctx := store.NewIndexingContext(false, ctx, string(id), nil)
				if err := index.processIntegrityFix(ctx); err != nil {
					return err
				}
			}

			errorSink(errors.Errorf("unique index %v.%v missing value %v for id %v",
				store.GetEntityType(), index.symbol.GetName(), string(fieldVal), string(id)), fix)

		} else if !bytes.Equal(idxId, id) {
			// We've already verify above that all index values are pointing to entities with the correct field value
			// so this means we've got a uniqueness contraint violation, which we can't fix
			errorSink(errors.Errorf("unique index %v.%v has constraint violation as both %v and %v have value %v. Unable to fix automatically",
				store.GetEntityType(), index.symbol.GetName(), string(idxId), string(id), string(fieldVal)), false)
		}
	}
	return nil
}

type setIndex struct {
	symbol    EntitySetSymbol
	indexPath []string
	listeners []SetChangeListener
}

func (index *setIndex) AddListener(listener SetChangeListener) {
	index.listeners = append(index.listeners, listener)
}

func (index *setIndex) GetSymbol() EntitySetSymbol {
	return index.symbol
}

func (index *setIndex) OpenValueCursor(tx *bbolt.Tx, key []byte, forward bool) ast.SetCursor {
	indexBaseBucket := Path(tx, index.indexPath...)
	if indexBaseBucket == nil {
		return ast.OpenEmptyCursor(tx, forward)
	}
	indexBucket := indexBaseBucket.Bucket.Bucket(key)
	if indexBucket == nil {
		return ast.OpenEmptyCursor(tx, forward)
	}
	cursor := indexBucket.Cursor()
	if forward {
		return NewTypedForwardBoltCursor(cursor, TypeString)
	}
	return NewTypedReverseBoltCursor(cursor, TypeString)
}

func (index *setIndex) Read(tx *bbolt.Tx, key []byte, f func(val []byte)) {
	indexBaseBucket := Path(tx, index.indexPath...)
	if indexBaseBucket == nil {
		return
	}
	indexBucket := indexBaseBucket.Bucket.Bucket(key)
	if indexBucket == nil {
		return
	}
	cursor := indexBucket.Cursor()
	for val, _ := cursor.First(); val != nil; val, _ = cursor.Next() {
		_, value := GetTypeAndValue(val)
		f(value)
	}
}

func (index *setIndex) OpenKeyCursor(tx *bbolt.Tx, forward bool) ast.SetCursor {
	indexBucket := Path(tx, index.indexPath...)
	if indexBucket == nil {
		return ast.OpenEmptyCursor(tx, forward)
	}
	cursor := indexBucket.Cursor()
	return NewBoltCursor(cursor, forward)
}

func (index *setIndex) ReadKeys(tx *bbolt.Tx, f func(val []byte)) {
	indexBucket := Path(tx, index.indexPath...)
	if indexBucket == nil {
		return
	}
	cursor := indexBucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		f(key)
	}
}

func (index *setIndex) visitCurrent(ctx *IndexingContext, f func(fieldType FieldType, value []byte)) {
	rtSymbol := index.symbol.GetRuntimeSymbol()
	cursor := rtSymbol.OpenCursor(ctx.Tx(), ctx.RowId)
	for cursor.IsValid() {
		fieldType, value := rtSymbol.Eval(ctx.Tx(), ctx.RowId)
		f(fieldType, value)
		cursor.Next()
	}
}

func (index *setIndex) getCurrentValues(ctx *IndexingContext) []FieldTypeAndValue {
	var result []FieldTypeAndValue
	index.visitCurrent(ctx, func(fieldType FieldType, value []byte) {
		result = append(result, FieldTypeAndValue{
			FieldType: fieldType,
			Value:     value,
		})
	})
	return result
}

func (index *setIndex) ProcessBeforeUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		ctx.SetStates[index] = index.getCurrentValues(ctx)
	}
}

func (index *setIndex) ProcessAfterUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		oldValues := ctx.SetStates[index]
		newValues := index.getCurrentValues(ctx)

		changed := false
		if len(oldValues) != len(newValues) {
			changed = true
		} else {
			for idx, oldVal := range oldValues {
				newVal := newValues[idx]
				if oldVal.FieldType != newVal.FieldType || !bytes.Equal(oldVal.Value, newVal.Value) {
					changed = true
					break
				}
			}
		}

		if !changed {
			return
		}

		for _, oldVal := range oldValues {
			indexBucket := index.getIndexBucket(ctx.Tx(), oldVal.Value)
			ctx.ErrHolder.SetError(indexBucket.DeleteListEntry(TypeString, ctx.RowId).Err)
			if k, _ := indexBucket.Cursor().First(); k == nil {
				ctx.ErrHolder.SetError(index.deleteIndexKey(ctx.Tx(), oldVal.Value))
			}
		}
		for _, newVal := range newValues {
			indexBucket := index.getIndexBucket(ctx.Tx(), newVal.Value)
			ctx.ErrHolder.SetError(indexBucket.SetListEntry(TypeString, ctx.RowId).Err)
		}
		for _, listener := range index.listeners {
			listener(ctx.Ctx, ctx.RowId, oldValues, newValues, ctx.ErrHolder)
		}
	}
}

func (index *setIndex) ProcessBeforeDelete(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		values := index.getCurrentValues(ctx)
		for _, val := range values {
			indexBucket := index.getIndexBucket(ctx.Tx(), val.Value)
			ctx.ErrHolder.SetError(indexBucket.DeleteListEntry(TypeString, ctx.RowId).Err)
		}
	}
}

func (index *setIndex) Initialize(tx *bbolt.Tx, errorHolder errorz.ErrorHolder) {
	if !errorHolder.HasError() {
		bucket := GetOrCreatePath(tx, index.indexPath...)
		errorHolder.SetError(bucket.Err)
	}
}

func (index *setIndex) getIndexBucket(tx *bbolt.Tx, key []byte) *TypedBucket {
	indexBucket := Path(tx, index.indexPath...)
	if indexBucket == nil {
		return ErrBucket(errors.Errorf("bucket at %+v for index not created", index.indexPath))
	}
	return indexBucket.GetOrCreateBucket(string(key))
}

func (index *setIndex) deleteIndexKey(tx *bbolt.Tx, key []byte) error {
	indexBucket := Path(tx, index.indexPath...)
	if indexBucket == nil {
		return errors.Errorf("bucket at %+v for index not created", index.indexPath)
	}
	return indexBucket.DeleteBucket(key)
}

func (index *setIndex) CheckIntegrity(ctx MutateContext, fix bool, errorSink func(error, bool)) error {
	tx := ctx.Tx()
	if indexBaseBucket := Path(tx, index.indexPath...); indexBaseBucket != nil {
		cursor := indexBaseBucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			if indexBucket := indexBaseBucket.Bucket.Bucket(key); indexBucket != nil {
				idsCursor := indexBucket.Cursor()
				for val, _ := idsCursor.First(); val != nil; val, _ = idsCursor.Next() {
					_, id := GetTypeAndValue(val)
					if !index.symbol.GetStore().IsEntityPresent(tx, string(id)) {
						// entry has been deleted, remove
						if fix {
							if err := idsCursor.Delete(); err != nil {
								return err
							}
						}
						errorSink(errors.Errorf("for index on %v.%v, val %v references id %v, which doesn't exist",
							index.symbol.GetStore().GetEntityType(), index.GetSymbol().GetName(),
							string(key), string(id)), fix)
					} else {
						rtSymbol := index.symbol.GetRuntimeSymbol()
						found := false
						for setCursor := rtSymbol.OpenCursor(tx, id); setCursor.IsValid(); setCursor.Next() {
							_, value := rtSymbol.Eval(tx, id)
							if bytes.Equal(value, key) {
								found = true
								break
							}
						}
						if !found {
							if fix {
								if err := idsCursor.Delete(); err != nil {
									return err
								}
							}
							errorSink(errors.Errorf("for index on %v.%v, val %v references id %v, which doesn't contain the value",
								index.symbol.GetStore().GetEntityType(), index.GetSymbol().GetName(),
								string(key), string(id)), fix)
						}
					}
				}
			} else {
				// If key has no values, delete the key
				if err := cursor.Delete(); err != nil {
					return err
				}
			}
		}
	}

	for idsCursor := index.symbol.GetStore().IterateValidIds(tx, ast.BoolNodeTrue); idsCursor.IsValid(); idsCursor.Next() {
		id := idsCursor.Current()
		entityBucket := index.symbol.GetStore().GetEntityBucket(tx, id)
		setBucket := entityBucket.GetPath(index.symbol.GetPath()...)
		if setBucket == nil {
			continue
		}
		valuesCursor := setBucket.Cursor()
		for val, _ := valuesCursor.First(); val != nil; val, _ = valuesCursor.Next() {
			_, value := GetTypeAndValue(val)
			idxBucket := index.getIndexBucket(tx, value)
			key := PrependFieldType(TypeString, id)
			if !idxBucket.IsKeyPresent(key) {
				if fix {
					if err := idxBucket.Put(key, nil); err != nil {
						return err
					}
				}
				errorSink(errors.Errorf("for index on %v.%v, id %v has val %v, but is not in the index",
					index.symbol.GetStore().GetEntityType(), index.GetSymbol().GetName(), string(id), string(value)), fix)
			}
		}
	}

	return nil
}

type fkIndex struct {
	symbol   EntitySymbol
	fkSymbol EntitySymbol
	nullable bool
}

func (index *fkIndex) ProcessBeforeUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		_, fieldValue := index.symbol.Eval(ctx.Tx(), ctx.RowId)
		ctx.AtomStates[index] = fieldValue
	}
}

func (index *fkIndex) ProcessAfterUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		_, newValue := index.symbol.Eval(ctx.Tx(), ctx.RowId)
		oldValue := ctx.AtomStates[index]

		if !ctx.IsCreate && bytes.Equal(oldValue, newValue) {
			return
		}

		if len(oldValue) > 0 {
			indexBucket := index.getIndexBucket(ctx.Tx(), oldValue)
			ctx.ErrHolder.SetError(indexBucket.DeleteListEntry(TypeString, ctx.RowId).Err)
		}

		if len(newValue) > 0 {
			indexBucket := index.getIndexBucket(ctx.Tx(), newValue)
			ctx.ErrHolder.SetError(indexBucket.SetListEntry(TypeString, ctx.RowId).Err)
		} else if !index.nullable {
			ctx.ErrHolder.SetError(errors.Errorf("index on %v.%v does not allow null or empty values",
				index.symbol.GetStore().GetEntityType(), index.symbol.GetName()))
		}
	}
}

func (index *fkIndex) ProcessBeforeDelete(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		if _, value := index.symbol.Eval(ctx.Tx(), ctx.RowId); len(value) > 0 {
			indexBucket := index.getIndexBucket(ctx.Tx(), value)
			ctx.ErrHolder.SetError(indexBucket.DeleteListEntry(TypeString, ctx.RowId).Err)
		}
	}
}

func (index *fkIndex) getIndexBucket(tx *bbolt.Tx, fkId []byte) *TypedBucket {
	fkStore := index.fkSymbol.GetStore()
	entityBucket := fkStore.GetEntityBucket(tx, fkId)
	if entityBucket == nil {
		return ErrBucket(NewNotFoundError(fkStore.GetSingularEntityType(), "id", string(fkId)))
	}
	return entityBucket.GetOrCreatePath(index.fkSymbol.GetPath()...)
}

func (index *fkIndex) getIndexBucketReadOnly(tx *bbolt.Tx, fkId []byte) *TypedBucket {
	fkStore := index.fkSymbol.GetStore()
	entityBucket := fkStore.GetEntityBucket(tx, fkId)
	if entityBucket == nil {
		return nil
	}
	return entityBucket.GetPath(index.fkSymbol.GetPath()...)
}

func (index *fkIndex) Initialize(_ *bbolt.Tx, _ errorz.ErrorHolder) {
	// nothing to do, as this index has no static location
}

func (index *fkIndex) CheckIntegrity(ctx MutateContext, fix bool, errorSink func(error, bool)) error {
	tx := ctx.Tx()
	for idsCursor := index.fkSymbol.GetStore().IterateValidIds(tx, ast.BoolNodeTrue); idsCursor.IsValid(); idsCursor.Next() {
		id := idsCursor.Current()
		entityBucket := index.fkSymbol.GetStore().GetEntityBucket(tx, id)
		setBucket := entityBucket.GetPath(index.fkSymbol.GetPath()...)
		if setBucket == nil {
			continue
		}
		fkCursor := setBucket.Cursor()
		for val, _ := fkCursor.First(); val != nil; val, _ = fkCursor.Next() {
			_, fkId := GetTypeAndValue(val)
			if !index.symbol.GetStore().IsEntityPresent(tx, string(fkId)) {
				if fix {
					if err := fkCursor.Delete(); err != nil {
						return err
					}
				}
				errorSink(errors.Errorf("for fk %v.%v, %v %v references %v %v, which doesn't exist",
					index.symbol.GetStore().GetEntityType(), index.symbol.GetName(),
					index.fkSymbol.GetStore().GetSingularEntityType(), string(id),
					index.symbol.GetStore().GetSingularEntityType(), string(fkId)), fix)
			} else {
				_, key := index.symbol.Eval(tx, fkId)
				if key == nil || !bytes.Equal(key, id) {
					if fix {
						if err := fkCursor.Delete(); err != nil {
							return err
						}
					}

					logVal := string(key)
					if key == nil {
						logVal = "(nil)"
					}

					errorSink(errors.Errorf("for fk %v.%v, %v %v references %v %v, which has non-matching value %v",
						index.symbol.GetStore().GetEntityType(), index.symbol.GetName(),
						index.fkSymbol.GetStore().GetSingularEntityType(), string(id),
						index.symbol.GetStore().GetSingularEntityType(), string(fkId), logVal), fix)
				}
			}
		}
	}

	for idsCursor := index.symbol.GetStore().IterateValidIds(tx, ast.BoolNodeTrue); idsCursor.IsValid(); idsCursor.Next() {
		id := idsCursor.Current()
		_, key := index.symbol.Eval(tx, id)
		if key == nil {
			if !index.nullable {
				errorSink(errors.Errorf("%v.%v is non-nillable, but %v with id %v has nil value",
					index.symbol.GetStore().GetEntityType(), index.symbol.GetName(),
					index.symbol.GetStore().GetSingularEntityType(), string(id)), false)
			}
		} else {
			if !index.fkSymbol.GetStore().IsEntityPresent(tx, string(key)) {
				tryFix := index.nullable && fix && len(index.symbol.GetPath()) == 1
				if tryFix {
					entityBucket := index.symbol.GetStore().GetEntityBucket(tx, id)
					if entityBucket.HasError() {
						return entityBucket.GetError()
					}
					if err := entityBucket.Put([]byte(index.symbol.GetPath()[0]), nil); err != nil {
						return err
					}
				}
				errorSink(errors.Errorf("%v.%v has invalid value for %v %v, which references invalid %v %v",
					index.symbol.GetStore().GetEntityType(), index.symbol.GetName(),
					index.symbol.GetStore().GetSingularEntityType(), string(id),
					index.fkSymbol.GetStore().GetSingularEntityType(), string(key)), tryFix)
			} else {
				indexBucket := index.getIndexBucketReadOnly(tx, key)
				typedKey := PrependFieldType(TypeString, id)
				if indexBucket == nil || !indexBucket.IsKeyPresent(typedKey) {
					if fix {
						indexBucket := index.getIndexBucket(tx, key)
						indexBucket.SetListEntry(TypeString, id)
						if indexBucket.HasError() {
							return indexBucket.GetError()
						}
					}
					errorSink(errors.Errorf("for %v %v field %v references %v %v, but no back-reference exists",
						index.symbol.GetStore().GetSingularEntityType(), string(id), index.symbol.GetName(),
						index.fkSymbol.GetStore().GetSingularEntityType(), string(key)), fix)
				}
			}
		}
	}
	return nil
}

type fkDeleteConstraint struct {
	symbol   EntitySetSymbol
	fkSymbol EntitySymbol
}

func (index *fkDeleteConstraint) ProcessBeforeUpdate(_ *IndexingContext) {
}

func (index *fkDeleteConstraint) ProcessAfterUpdate(_ *IndexingContext) {
}

func (index *fkDeleteConstraint) ProcessBeforeDelete(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		rtSymbol := index.symbol.GetRuntimeSymbol()
		if rtSymbol.OpenCursor(ctx.Tx(), ctx.RowId).IsValid() {
			_, firstId := rtSymbol.Eval(ctx.Tx(), ctx.RowId)
			ctx.ErrHolder.SetError(NewReferenceByIdError(
				index.symbol.GetStore().GetEntityType(),
				string(ctx.RowId),
				index.fkSymbol.GetStore().GetEntityType(),
				string(firstId),
				index.fkSymbol.GetName()))
		}
	}
}

func (index *fkDeleteConstraint) Initialize(*bbolt.Tx, errorz.ErrorHolder) {
	// nothing to do, as this index has no static location
}

func (index *fkDeleteConstraint) CheckIntegrity(MutateContext, bool, func(error, bool)) error {
	return nil
}

type CascadeType int

const (
	CascadeNone         = 1
	CascadeDelete       = 2
	CascadeCreateUpdate = 3
)

type fkConstraint struct {
	symbol   EntitySymbol
	nullable bool
}

func (index *fkConstraint) ProcessBeforeUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		_, fieldValue := index.symbol.Eval(ctx.Tx(), ctx.RowId)
		ctx.AtomStates[index] = fieldValue
	}
}

func (index *fkConstraint) ProcessAfterUpdate(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		_, newValue := index.symbol.Eval(ctx.Tx(), ctx.RowId)
		oldValue := ctx.AtomStates[index]

		if !ctx.IsCreate && bytes.Equal(oldValue, newValue) {
			return
		}

		if len(newValue) > 0 {
			fkId := string(newValue)
			if !index.symbol.GetLinkedType().IsEntityPresent(ctx.Tx(), fkId) {
				err := NewNotFoundError(index.symbol.GetLinkedType().GetSingularEntityType(), "id", fkId)
				ctx.ErrHolder.SetError(err)
			}
		} else if !index.nullable {
			ctx.ErrHolder.SetError(errors.Errorf("fk constraint on %v.%v does not allow null or empty values",
				index.symbol.GetStore().GetEntityType(), index.symbol.GetName()))
		}
	}

}

func (index *fkConstraint) ProcessBeforeDelete(*IndexingContext) {}

func (index *fkConstraint) Initialize(*bbolt.Tx, errorz.ErrorHolder) {
	// nothing to do, as this index has no static location
}

func (index *fkConstraint) CheckIntegrity(ctx MutateContext, fix bool, errorSink func(error, bool)) error {
	tx := ctx.Tx()
	for idsCursor := index.symbol.GetStore().IterateValidIds(tx, ast.BoolNodeTrue); idsCursor.IsValid(); idsCursor.Next() {
		id := idsCursor.Current()
		_, key := index.symbol.Eval(tx, id)
		if key == nil {
			if !index.nullable {
				errorSink(errors.Errorf("%v.%v is non-nillable, but %v with id %v has nil value",
					index.symbol.GetStore().GetEntityType(), index.symbol.GetName(),
					index.symbol.GetStore().GetSingularEntityType(), string(id)), false)
			}
		} else {
			if !index.symbol.GetLinkedType().IsEntityPresent(tx, string(key)) {
				tryFix := index.nullable && fix && len(index.symbol.GetPath()) == 1
				if tryFix {
					entityBucket := index.symbol.GetStore().GetEntityBucket(tx, id)
					if entityBucket.HasError() {
						return entityBucket.GetError()
					}
					if err := entityBucket.Put([]byte(index.symbol.GetPath()[0]), nil); err != nil {
						return err
					}
				}
				errorSink(errors.Errorf("%v.%v has invalid value for %v %v, which references invalid %v %v",
					index.symbol.GetStore().GetEntityType(), index.symbol.GetName(),
					index.symbol.GetStore().GetSingularEntityType(), string(id),
					index.symbol.GetLinkedType().GetSingularEntityType(), string(key)), tryFix)
			}
		}
	}
	return nil
}

type fkDeleteCascadeConstraint struct {
	symbol      EntitySymbol
	cascadeType CascadeType
}

func (index *fkDeleteCascadeConstraint) ProcessBeforeUpdate(*IndexingContext) {
}

func (index *fkDeleteCascadeConstraint) ProcessAfterUpdate(*IndexingContext) {
}

func (index *fkDeleteCascadeConstraint) ProcessBeforeDelete(ctx *IndexingContext) {
	if !ctx.ErrHolder.HasError() {
		filter, err := ast.Parse(index.symbol.GetStore(), fmt.Sprintf(`%v = "%v"`, index.symbol.GetName(), string(ctx.RowId)))
		if ctx.ErrHolder.SetError(err) {
			return
		}

		targetStore := index.symbol.GetStore()

		if index.cascadeType == CascadeNone {
			cursor := targetStore.IterateValidIds(ctx.Tx(), filter)
			if cursor.IsValid() {
				ctx.ErrHolder.SetError(NewReferenceByIdError(
					index.symbol.GetLinkedType().GetSingularEntityType(),
					string(ctx.RowId),
					index.symbol.GetStore().GetSingularEntityType(),
					string(cursor.Current()),
					index.symbol.GetName()))
				return
			}
		}

		if index.cascadeType == CascadeDelete {
			cursor := targetStore.IterateValidIds(ctx.Tx(), filter)
			for cursor.IsValid() {
				if ctx.ErrHolder.SetError(targetStore.DeleteById(ctx.Ctx, string(cursor.Current()))) {
					return
				}

				// There is a bug in bolt where cursor next will sometimes skip the next row if you delete the
				// current row, either via cursor delete or just bucket delete. Using seek works around this
				cursor.Seek(cursor.Current())
			}
		}
	}
}

func (index *fkDeleteCascadeConstraint) Initialize(*bbolt.Tx, errorz.ErrorHolder) {
	// nothing to do, as this index has no static location
}

func (index *fkDeleteCascadeConstraint) CheckIntegrity(MutateContext, bool, func(error, bool)) error {
	return nil
}
