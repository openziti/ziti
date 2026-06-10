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
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

const (
	entityTypeTaggedThing = "taggedThings"
	fieldTags             = "tags"
)

type taggedThing struct {
	Id   string
	Tags []string
}

func (e *taggedThing) GetId() string              { return e.Id }
func (e *taggedThing) SetId(id string)            { e.Id = id }
func (e *taggedThing) GetEntityType() string      { return entityTypeTaggedThing }

type taggedThingEntityStrategy struct{}

func (taggedThingEntityStrategy) NewEntity() *taggedThing {
	return new(taggedThing)
}

func (taggedThingEntityStrategy) FillEntity(entity *taggedThing, bucket *TypedBucket) {
	entity.Tags = bucket.GetStringList(fieldTags)
}

func (taggedThingEntityStrategy) PersistEntity(entity *taggedThing, ctx *PersistContext) {
	ctx.SetStringList(fieldTags, entity.Tags)
}

type taggedThingStore struct {
	*BaseStore[*taggedThing]

	symbolTags EntitySetSymbol
	// indexHashTags indexes only entries that start with '#', stripping the prefix.
	indexHashTags SetReadIndex
}

// keepHashPrefixOnly is the transform used by indexHashTags. It accepts only
// string values that start with '#' and have a non-empty suffix, yielding the
// value with the '#' stripped. The empty-suffix guard matches the constraint
// that a SetIndex cannot create a sub-bucket with an empty key.
var keepHashPrefixOnly SetIndexValueTransform = func(ft FieldType, v []byte) (bool, FieldType, []byte) {
	if ft != TypeString || len(v) < 2 || v[0] != '#' {
		return false, ft, v
	}
	return true, ft, v[1:]
}

func newTaggedThingStore() *taggedThingStore {
	storeDef := StoreDefinition[*taggedThing]{
		EntityType:     entityTypeTaggedThing,
		EntityStrategy: taggedThingEntityStrategy{},
		EntityNotFoundF: func(id string) error {
			return NewNotFoundError(entityTypeTaggedThing, "id", id)
		},
		BasePath: []string{"stores"},
	}
	store := &taggedThingStore{
		BaseStore: NewBaseStore(storeDef),
	}
	store.InitImpl(store)

	store.AddIdSymbol("id", ast.NodeTypeString)
	store.symbolTags = store.AddSetSymbol(fieldTags, ast.NodeTypeString)
	store.indexHashTags = store.AddSetIndexWithTransform(store.symbolTags, keepHashPrefixOnly)
	return store
}

type derivedSetIndexTest struct {
	dbTest
	store *taggedThingStore
}

func (t *derivedSetIndexTest) init() {
	t.dbTest.init()
	t.store = newTaggedThingStore()
	err := t.db.Update(func(tx *bbolt.Tx) error {
		t.store.InitializeIndexes(tx, t)
		return nil
	})
	t.NoError(err)
}

func (t *derivedSetIndexTest) sortedIds(tx *bbolt.Tx, key string) []string {
	var out []string
	t.store.indexHashTags.Read(tx, []byte(key), func(val []byte) {
		out = append(out, string(val))
	})
	sort.Strings(out)
	return out
}

func (t *derivedSetIndexTest) keys() []string {
	var out []string
	err := t.db.View(func(tx *bbolt.Tx) error {
		t.store.indexHashTags.ReadKeys(tx, func(val []byte) {
			out = append(out, string(val))
		})
		return nil
	})
	t.NoError(err)
	sort.Strings(out)
	return out
}

func (t *derivedSetIndexTest) mutate(fn func(ctx MutateContext) error) {
	err := t.db.Update(func(tx *bbolt.Tx) error {
		return fn(NewTxMutateContext(context.Background(), tx))
	})
	t.NoError(err)
}

// TestSetIndexWithTransform_CreateUpdateDelete exercises the transform on the
// three mutating lifecycle paths and verifies only transformed values land in
// the index while raw values with the wrong prefix are dropped.
func TestSetIndexWithTransform_CreateUpdateDelete(t *testing.T) {
	test := &derivedSetIndexTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	e1 := &taggedThing{Id: uuid.New().String(), Tags: []string{"#sales", "@id-123", "#marketing", "#"}}
	e2 := &taggedThing{Id: uuid.New().String(), Tags: []string{"@id-999", "#sales"}}
	e3 := &taggedThing{Id: uuid.New().String(), Tags: []string{"@id-777"}}

	test.mutate(func(ctx MutateContext) error {
		test.NoError(test.store.Create(ctx, e1))
		test.NoError(test.store.Create(ctx, e2))
		test.NoError(test.store.Create(ctx, e3))
		return nil
	})

	// Only '#'-prefixed entries are indexed, with the '#' stripped. '#' by
	// itself (empty suffix) and '@'-prefixed entries are dropped.
	test.Equal([]string{"marketing", "sales"}, test.keys())

	err := test.db.View(func(tx *bbolt.Tx) error {
		test.Equal(sortStrings([]string{e1.Id, e2.Id}), test.sortedIds(tx, "sales"))
		test.Equal([]string{e1.Id}, test.sortedIds(tx, "marketing"))

		// transformed index has no "" key since a bare "#" is filtered out
		test.Nil(test.sortedIds(tx, ""))

		// raw string with the prefix intact isn't in the index
		test.Nil(test.sortedIds(tx, "#sales"))
		return nil
	})
	test.NoError(err)

	// Update: e1 drops marketing, adds finance; e2 drops sales, adds #devops
	e1.Tags = []string{"#sales", "@id-123", "#finance"}
	e2.Tags = []string{"@id-999", "#devops"}
	test.mutate(func(ctx MutateContext) error {
		test.NoError(test.store.Update(ctx, e1, nil))
		test.NoError(test.store.Update(ctx, e2, nil))
		return nil
	})

	test.Equal([]string{"devops", "finance", "sales"}, test.keys())

	err = test.db.View(func(tx *bbolt.Tx) error {
		test.Equal([]string{e1.Id}, test.sortedIds(tx, "sales"))
		test.Equal([]string{e1.Id}, test.sortedIds(tx, "finance"))
		test.Equal([]string{e2.Id}, test.sortedIds(tx, "devops"))
		test.Nil(test.sortedIds(tx, "marketing"))
		return nil
	})
	test.NoError(err)

	// Delete e1; finance and sales should drop out since only e1 referenced them.
	test.mutate(func(ctx MutateContext) error {
		test.NoError(test.store.DeleteById(ctx, e1.Id))
		return nil
	})

	test.Equal([]string{"devops"}, test.keys())
}

// TestSetIndexWithTransform_CheckIntegrity_Rebuild verifies CheckIntegrity(fix=true)
// can populate an empty derived index from scratch, using the transform to compute
// the correct keys from the entity's raw field values.
func TestSetIndexWithTransform_CheckIntegrity_Rebuild(t *testing.T) {
	test := &derivedSetIndexTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	e1 := &taggedThing{Id: uuid.New().String(), Tags: []string{"#sales", "@id-1"}}
	e2 := &taggedThing{Id: uuid.New().String(), Tags: []string{"#sales", "#marketing"}}

	test.mutate(func(ctx MutateContext) error {
		test.NoError(test.store.Create(ctx, e1))
		test.NoError(test.store.Create(ctx, e2))
		return nil
	})

	// Sanity: healthy index, no integrity errors.
	test.mutate(func(ctx MutateContext) error {
		return test.store.CheckIntegrity(ctx, false, func(err error, fixed bool) {
			t.Fatalf("unexpected integrity error before wipe: %v", err)
		})
	})

	// Nuke the index bucket to simulate an upgrade from a DB where the derived
	// index didn't exist yet.
	index := test.store.indexHashTags.(*setIndex)
	test.mutate(func(ctx MutateContext) error {
		indexBase := Path(ctx.Tx(), index.indexPath...)
		test.NotNil(indexBase)
		// Delete every sub-bucket (one per key)
		cursor := indexBase.Cursor()
		var keys [][]byte
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			keys = append(keys, append([]byte{}, k...))
		}
		for _, k := range keys {
			test.NoError(indexBase.DeleteBucket(k))
		}
		return nil
	})

	// No keys present after wipe.
	test.Empty(test.keys())

	// CheckIntegrity(fix=true) should rebuild the index to match live data.
	var sawErrs int
	test.mutate(func(ctx MutateContext) error {
		return test.store.CheckIntegrity(ctx, true, func(err error, fixed bool) {
			sawErrs++
			test.True(fixed)
		})
	})
	test.Greater(sawErrs, 0)

	// Re-verify index contents via transform.
	test.Equal([]string{"marketing", "sales"}, test.keys())
	err := test.db.View(func(tx *bbolt.Tx) error {
		test.Equal(sortStrings([]string{e1.Id, e2.Id}), test.sortedIds(tx, "sales"))
		test.Equal([]string{e2.Id}, test.sortedIds(tx, "marketing"))
		return nil
	})
	test.NoError(err)

	// A clean integrity run should now be silent.
	test.mutate(func(ctx MutateContext) error {
		return test.store.CheckIntegrity(ctx, false, func(err error, fixed bool) {
			t.Fatalf("unexpected integrity error after rebuild: %v", err)
		})
	})
}

func sortStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
