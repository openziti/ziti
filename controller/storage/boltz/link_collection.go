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
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"sort"
)

type LinkCollection interface {
	Checkable
	AddLinks(tx *bbolt.Tx, id string, keys ...string) error
	AddLink(tx *bbolt.Tx, id []byte, key []byte) (bool, error)
	RemoveLinks(tx *bbolt.Tx, id string, keys ...string) error
	RemoveLink(tx *bbolt.Tx, id []byte, keys []byte) (bool, error)
	SetLinks(tx *bbolt.Tx, id string, keys []string) error
	GetLinks(tx *bbolt.Tx, id string) []string
	IterateLinks(tx *bbolt.Tx, id []byte) ast.SeekableSetCursor
	IsLinked(tx *bbolt.Tx, id, relatedId []byte) bool
	EntityDeleted(tx *bbolt.Tx, id string) error
	GetFieldSymbol() EntitySymbol
	GetLinkedSymbol() EntitySymbol
}

type linkCollectionImpl struct {
	field      EntitySymbol
	otherField *LinkedSetSymbol
}

func (collection *linkCollectionImpl) GetFieldSymbol() EntitySymbol {
	return collection.field
}

func (collection *linkCollectionImpl) GetLinkedSymbol() EntitySymbol {
	return collection.otherField
}

func (collection *linkCollectionImpl) getFieldBucketForStringId(tx *bbolt.Tx, id string) *TypedBucket {
	return collection.getFieldBucket(tx, []byte(id))
}

func (collection *linkCollectionImpl) getFieldBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
	entityBucket := collection.field.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		return ErrBucket(errors.Errorf("%v not found with id %v", collection.field.GetStore().GetEntityType(), id))
	}
	return entityBucket.GetOrCreatePath(collection.field.GetPath()...)
}

func (collection *linkCollectionImpl) AddLinks(tx *bbolt.Tx, id string, keys ...string) error {
	fieldBucket := collection.getFieldBucketForStringId(tx, id)
	if !fieldBucket.HasError() {
		byteId := []byte(id)
		for _, key := range keys {
			if err := collection.link(tx, fieldBucket, byteId, []byte(key)); err != nil {
				return err
			}
		}
	}
	return fieldBucket.Err
}

func (collection *linkCollectionImpl) AddLink(tx *bbolt.Tx, id []byte, key []byte) (bool, error) {
	fieldBucket := collection.getFieldBucket(tx, id)
	if !fieldBucket.HasError() {
		return collection.checkAndLink(tx, fieldBucket, id, key)
	}
	return false, fieldBucket.Err
}

func (collection *linkCollectionImpl) RemoveLinks(tx *bbolt.Tx, id string, keys ...string) error {
	fieldBucket := collection.getFieldBucketForStringId(tx, id)
	if !fieldBucket.HasError() {
		byteId := []byte(id)
		for _, key := range keys {
			if err := collection.unlink(tx, fieldBucket, byteId, []byte(key)); err != nil {
				return err
			}
		}
	}
	return fieldBucket.Err
}

func (collection *linkCollectionImpl) RemoveLink(tx *bbolt.Tx, id []byte, key []byte) (bool, error) {
	fieldBucket := collection.getFieldBucket(tx, id)
	if !fieldBucket.HasError() {
		return collection.checkAndUnlink(tx, fieldBucket, id, key)
	}
	return false, fieldBucket.Err
}

func (collection *linkCollectionImpl) SetLinks(tx *bbolt.Tx, id string, keys []string) error {
	sort.Strings(keys)
	fieldBucket := collection.getFieldBucketForStringId(tx, id)

	var toAdd []string
	var toRemove []string

	if !fieldBucket.HasError() {
		cursor := fieldBucket.Cursor()
		for row, _ := cursor.First(); row != nil; row, _ = cursor.Next() {
			_, val := GetTypeAndValue(row)
			rowHandled := false
			for len(keys) > 0 {
				cursorCurrent := string(val)
				compare := keys[0]

				if compare < cursorCurrent {
					toAdd = append(toAdd, compare)
					keys = keys[1:]
					for len(keys) > 0 && keys[0] == compare { // skip over duplicate entries
						keys = keys[1:]
					}
				} else if compare > cursorCurrent {
					toRemove = append(toRemove, string(val))
					rowHandled = true
					break
				} else {
					keys = keys[1:]
					rowHandled = true
					break
				}
			}

			if !rowHandled {
				toRemove = append(toRemove, string(val))
				continue
			}
		}
	}

	if fieldBucket.HasError() {
		return fieldBucket.Err
	}

	if err := collection.RemoveLinks(tx, id, toRemove...); err != nil {
		return err
	}

	toAdd = append(toAdd, keys...)
	return collection.AddLinks(tx, id, toAdd...)
}

func (collection *linkCollectionImpl) EntityDeleted(tx *bbolt.Tx, id string) error {
	bId := []byte(id)
	fieldBucket := collection.getFieldBucketForStringId(tx, id)

	if !fieldBucket.HasError() {
		cursor := fieldBucket.Cursor()
		for val, _ := cursor.First(); val != nil; val, _ = cursor.Next() {
			_, key := GetTypeAndValue(val)
			// We don't need to delete the local entry b/c the parent bucket is getting deleted
			if err := collection.otherField.RemoveLink(tx, key, bId); err != nil {
				return err
			}
		}
	}

	return fieldBucket.Err
}

func (collection *linkCollectionImpl) IsLinked(tx *bbolt.Tx, id, relatedId []byte) bool {
	cursor := collection.IterateLinks(tx, id)
	cursor.Seek(relatedId)
	return cursor.IsValid() && bytes.Equal(cursor.Current(), relatedId)
}

func (collection *linkCollectionImpl) GetLinks(tx *bbolt.Tx, id string) []string {
	fieldBucket := collection.getFieldBucketForStringId(tx, id)
	if !fieldBucket.HasError() {
		return fieldBucket.ReadStringList()
	}
	return nil
}

func (collection *linkCollectionImpl) IterateLinks(tx *bbolt.Tx, id []byte) ast.SeekableSetCursor {
	fieldBucket := collection.getFieldBucket(tx, id)
	if !fieldBucket.HasError() {
		return fieldBucket.IterateStringList()
	}
	return ast.EmptyCursor
}

func (collection *linkCollectionImpl) CheckIntegrity(ctx MutateContext, fix bool, errorSink func(err error, fixed bool)) error {
	tx := ctx.Tx()
	foundInverse := false
	for _, lc := range collection.otherField.GetStore().getLinks() {
		if collection.GetFieldSymbol().GetName() == lc.GetLinkedSymbol().GetName() &&
			collection.GetFieldSymbol().GetStore().GetEntityType() == lc.GetLinkedSymbol().GetStore().GetEntityType() &&
			collection.GetLinkedSymbol().GetName() == lc.GetFieldSymbol().GetName() &&
			collection.GetLinkedSymbol().GetStore().GetEntityType() == lc.GetFieldSymbol().GetStore().GetEntityType() {
			foundInverse = true
			break
		}
	}
	if !foundInverse {
		errorSink(errors.Errorf("%v store has link collection from %v -> %v.%v but no inverse collection found in %v",
			collection.field.GetStore().GetEntityType(), collection.field.GetName(),
			collection.otherField.GetStore().GetEntityType(), collection.otherField.GetName(),
			collection.otherField.GetStore().GetEntityType()), false)
	}

	for idCursor := collection.field.GetStore().IterateValidIds(tx, ast.BoolNodeTrue); idCursor.IsValid(); idCursor.Next() {
		id := idCursor.Current()
		for linkCursor := collection.IterateLinks(tx, id); linkCursor.IsValid(); linkCursor.Next() {
			linkId := linkCursor.Current()
			linkValid := collection.otherField.GetStore().IsEntityPresent(tx, string(linkId))
			if !linkValid {
				if fix {
					if _, err := collection.RemoveLink(tx, id, linkId); err != nil {
						return err
					}
				}
				err := errors.Errorf("%v %v references %v %v, which doesn't exist",
					collection.field.GetStore().GetSingularEntityType(), string(id),
					collection.otherField.GetStore().GetSingularEntityType(), string(linkId))
				errorSink(err, fix)
			} else if !collection.otherField.IsLinked(tx, linkId, id) {
				if fix {
					if err := collection.otherField.AddLink(tx, linkId, id); err != nil {
						return err
					}
				}
				err := errors.Errorf("%v %v references %v %v, but reverse link is missing",
					collection.field.GetStore().GetSingularEntityType(), string(id),
					collection.otherField.GetStore().GetSingularEntityType(), string(linkId))
				errorSink(err, fix)
			}
		}
	}
	return nil
}

func (collection *linkCollectionImpl) checkAndLink(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte) (bool, error) {
	changed, err := fieldBucket.CheckAndSetListEntry(TypeString, associatedId)
	if err != nil {
		return false, err
	}
	return changed, collection.otherField.AddLink(tx, associatedId, id)
}

func (collection *linkCollectionImpl) link(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte) error {
	if fieldBucket.SetListEntry(TypeString, associatedId).Err != nil {
		return fieldBucket.Err
	}
	return collection.otherField.AddLink(tx, associatedId, id)
}

func (collection *linkCollectionImpl) checkAndUnlink(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte) (bool, error) {
	changed, err := fieldBucket.CheckAndDeleteListEntry(TypeString, associatedId)
	if err != nil {
		return false, err
	}
	return changed, collection.otherField.RemoveLink(tx, associatedId, id)
}

func (collection *linkCollectionImpl) unlink(tx *bbolt.Tx, fieldBucket *TypedBucket, id []byte, associatedId []byte) error {
	if fieldBucket.DeleteListEntry(TypeString, associatedId).Err != nil {
		return fieldBucket.Err
	}
	return collection.otherField.RemoveLink(tx, associatedId, id)
}

const MaxLinkedSetKeySize = 4096

type LinkedSetSymbol struct {
	EntitySymbol
}

func (symbol *LinkedSetSymbol) AddCompoundLink(tx *bbolt.Tx, id string, linkIds []string) error {
	key, err := EncodeStringSlice(linkIds)
	if err != nil {
		return err
	}
	return symbol.AddLink(tx, []byte(id), key)
}

func (symbol *LinkedSetSymbol) RemoveCompoundLink(tx *bbolt.Tx, id string, linkIds []string) error {
	key, err := EncodeStringSlice(linkIds)
	if err != nil {
		return err
	}
	return symbol.RemoveLink(tx, []byte(id), key)
}

func (symbol *LinkedSetSymbol) AddLinkS(tx *bbolt.Tx, id string, link string) error {
	return symbol.AddLink(tx, []byte(id), []byte(link))
}

func (symbol *LinkedSetSymbol) AddLink(tx *bbolt.Tx, id []byte, link []byte) error {
	entityBucket := symbol.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		return NewNotFoundError(symbol.GetStore().GetSingularEntityType(), "id", string(id))
	}
	fieldBucket := entityBucket.GetOrCreatePath(symbol.GetPath()...)
	return fieldBucket.SetListEntry(TypeString, link).Err
}

func (symbol *LinkedSetSymbol) RemoveLink(tx *bbolt.Tx, id []byte, link []byte) error {
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
	return fieldBucket.DeleteListEntry(TypeString, link).Err
}

func (symbol *LinkedSetSymbol) IsLinked(tx *bbolt.Tx, id []byte, link []byte) bool {
	entityBucket := symbol.GetStore().GetEntityBucket(tx, id)
	if entityBucket == nil {
		return false
	}
	fieldBucket := entityBucket.GetPath(symbol.GetPath()...)
	if fieldBucket == nil {
		return false
	}
	key := PrependFieldType(TypeString, link)
	return fieldBucket.IsKeyPresent(key)
}
