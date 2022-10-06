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
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type RefCountedLinkCollection interface {
	IncrementLinkCount(tx *bbolt.Tx, id []byte, key []byte) (int, error)
	DecrementLinkCount(tx *bbolt.Tx, id []byte, key []byte) (int, error)
	GetLinkCount(tx *bbolt.Tx, id []byte, relatedId []byte) *int32
	GetLinkCounts(tx *bbolt.Tx, id []byte, relatedId []byte) (*int32, *int32)
	SetLinkCount(tx *bbolt.Tx, id []byte, key []byte, count int) (*int32, *int32, error)
	EntityDeleted(tx *bbolt.Tx, id string) error
	IterateLinks(tx *bbolt.Tx, id []byte) ast.SeekableSetCursor
	GetFieldSymbol() EntitySymbol
	GetLinkedSymbol() EntitySymbol
}

type rcLinkCollectionImpl struct {
	field      EntitySymbol
	otherField *RefCountedLinkedSetSymbol
}

func (collection *rcLinkCollectionImpl) GetFieldSymbol() EntitySymbol {
	return collection.field
}

func (collection *rcLinkCollectionImpl) GetLinkedSymbol() EntitySymbol {
	return collection.otherField
}

func (collection *rcLinkCollectionImpl) getFieldBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
	entityBucket := collection.field.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		return ErrBucket(errors.Errorf("%v not found with id %v", collection.field.GetStore().GetEntityType(), string(id)))
	}
	return entityBucket.GetOrCreatePath(collection.field.GetPath()...)
}

func (collection *rcLinkCollectionImpl) SetLinkCount(tx *bbolt.Tx, id []byte, key []byte, count int) (*int32, *int32, error) {
	fieldBucket := collection.getFieldBucket(tx, id)
	if fieldBucket.HasError() {
		return nil, nil, fieldBucket.GetError()
	}
	return collection.setLinkCount(tx, fieldBucket, id, key, count)
}

func (collection *rcLinkCollectionImpl) GetLinkCounts(tx *bbolt.Tx, id []byte, key []byte) (*int32, *int32) {
	var sourceVal, targetVal *int32
	if entityBucket := collection.field.GetStore().GetEntityBucket(tx, id); entityBucket != nil {
		if fieldBucket := entityBucket.GetPath(collection.field.GetPath()...); fieldBucket != nil {
			sourceVal = fieldBucket.GetLinkCount(TypeString, key)
		}
	}

	if entityBucket := collection.otherField.GetStore().GetEntityBucket(tx, key); entityBucket != nil {
		if fieldBucket := entityBucket.GetPath(collection.otherField.GetPath()...); fieldBucket != nil {
			targetVal = fieldBucket.GetLinkCount(TypeString, id)
		}
	}

	return sourceVal, targetVal
}

func (collection *rcLinkCollectionImpl) IncrementLinkCount(tx *bbolt.Tx, id []byte, key []byte) (int, error) {
	fieldBucket := collection.getFieldBucket(tx, id)
	if fieldBucket.HasError() {
		return 0, fieldBucket.GetError()
	}
	return collection.incrementLinkCount(tx, fieldBucket, id, key)
}

func (collection *rcLinkCollectionImpl) DecrementLinkCount(tx *bbolt.Tx, id []byte, key []byte) (int, error) {
	fieldBucket := collection.getFieldBucket(tx, id)
	if fieldBucket.HasError() {
		return 0, fieldBucket.GetError()
	}
	return collection.decrementLinkCount(tx, fieldBucket, id, key)
}

func (collection *rcLinkCollectionImpl) GetLinkCount(tx *bbolt.Tx, id []byte, relatedId []byte) *int32 {
	fieldBucket := collection.getFieldBucket(tx, id)
	if fieldBucket.HasError() {
		return nil
	}
	return fieldBucket.GetLinkCount(TypeString, relatedId)
}

func (collection *rcLinkCollectionImpl) EntityDeleted(tx *bbolt.Tx, id string) error {
	bId := []byte(id)
	fieldBucket := collection.getFieldBucket(tx, bId)

	if !fieldBucket.HasError() {
		cursor := fieldBucket.Cursor()
		for val, _ := cursor.First(); val != nil; val, _ = cursor.Next() {
			_, key := GetTypeAndValue(val)
			// We don't need to remove the local entry because the parent bucket is getting deleted
			if err := collection.otherField.unlink(tx, key, bId); err != nil {
				return err
			}
		}
	}

	return fieldBucket.Err
}

func (collection *rcLinkCollectionImpl) IterateLinks(tx *bbolt.Tx, id []byte) ast.SeekableSetCursor {
	if fieldBucket := collection.getFieldBucket(tx, id); !fieldBucket.HasError() {
		return fieldBucket.IterateStringList()
	}
	return ast.EmptyCursor
}

func (collection *rcLinkCollectionImpl) setLinkCount(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte, value int) (*int32, *int32, error) {
	oldVal, err := fieldBucket.SetLinkCount(TypeString, associatedId, value)
	if err != nil {
		return nil, nil, err
	}
	otherOldVal, err := collection.otherField.setLinkCount(tx, associatedId, id, value)
	if err != nil {
		return nil, nil, err
	}
	return oldVal, otherOldVal, nil
}

func (collection *rcLinkCollectionImpl) incrementLinkCount(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte) (int, error) {
	newVal, err := fieldBucket.IncrementLinkCount(TypeString, associatedId)
	if err != nil {
		return 0, err
	}
	otherNewVal, err := collection.otherField.incrementLinkCount(tx, associatedId, id)
	if err != nil {
		return 0, err
	}
	if newVal != otherNewVal {
		return 0, errors.Errorf("unexpected mismatch when incrementing reference counts from %v %v (%v) <-> %v %v (%v)",
			collection.field.GetStore().GetSingularEntityType(), string(id), newVal,
			collection.otherField.GetStore().GetSingularEntityType(), string(associatedId), otherNewVal)
	}

	return newVal, nil
}

func (collection *rcLinkCollectionImpl) decrementLinkCount(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte) (int, error) {
	newVal, err := fieldBucket.DecrementLinkCount(TypeString, associatedId)
	if err != nil {
		return 0, err
	}
	otherNewVal, err := collection.otherField.decrementLinkCount(tx, associatedId, id)
	if err != nil {
		return 0, err
	}
	if newVal != otherNewVal {
		return 0, errors.Errorf("unexpected mismatch when decrementing reference counts from %v %v (%v) <-> %v %v (%v)",
			collection.field.GetStore().GetSingularEntityType(), string(id), newVal,
			collection.otherField.GetStore().GetSingularEntityType(), string(associatedId), otherNewVal)
	}

	return newVal, nil
}

type RefCountedLinkedSetSymbol struct {
	EntitySymbol
}

func (symbol *RefCountedLinkedSetSymbol) setLinkCount(tx *bbolt.Tx, id []byte, link []byte, count int) (*int32, error) {
	entityBucket := symbol.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		return nil, NewNotFoundError(symbol.GetStore().GetSingularEntityType(), "id", string(id))
	}
	fieldBucket := entityBucket.GetOrCreatePath(symbol.GetPath()...)
	return fieldBucket.SetLinkCount(TypeString, link, count)
}

func (symbol *RefCountedLinkedSetSymbol) incrementLinkCount(tx *bbolt.Tx, id []byte, link []byte) (int, error) {
	entityBucket := symbol.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		return 0, NewNotFoundError(symbol.GetStore().GetSingularEntityType(), "id", string(id))
	}
	fieldBucket := entityBucket.GetOrCreatePath(symbol.GetPath()...)
	return fieldBucket.IncrementLinkCount(TypeString, link)
}

func (symbol *RefCountedLinkedSetSymbol) decrementLinkCount(tx *bbolt.Tx, id []byte, link []byte) (int, error) {
	entityBucket := symbol.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		// attempt to unlink something that doesn't exist. nothing to do on fk side
		return -1, nil
	}
	fieldBucket := entityBucket.GetPath(symbol.GetPath()...)
	if fieldBucket == nil {
		// attempt to unlink something that's not linked. nothing to do on fk side
		return -1, nil
	}
	return fieldBucket.DecrementLinkCount(TypeString, link)
}

func (symbol *RefCountedLinkedSetSymbol) unlink(tx *bbolt.Tx, id []byte, link []byte) error {
	entityBucket := symbol.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		// attempt to unlink something that doesn't exist. nothing to do on fk side
		return nil
	}
	fieldBucket := entityBucket.GetPath(symbol.GetPath()...)
	if fieldBucket == nil {
		// attempt to unlink something that's not linked. nothing to do on fk side
		return nil
	}
	return fieldBucket.DeleteListEntry(TypeString, link).GetError()
}
