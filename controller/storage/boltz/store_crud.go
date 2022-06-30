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
	"github.com/google/uuid"
	"github.com/kataras/go-events"
	"github.com/openziti/storage/ast"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

func (store *BaseStore) GetParentStore() CrudStore {
	return store.parent
}

func (store *BaseStore) AddLinkCollection(local EntitySymbol, remote EntitySymbol) LinkCollection {
	result := &linkCollectionImpl{
		field:      local,
		otherField: &LinkedSetSymbol{EntitySymbol: remote},
	}
	store.links[local.GetName()] = result
	return result
}

func (store *BaseStore) AddRefCountedLinkCollection(local EntitySymbol, remote EntitySymbol) RefCountedLinkCollection {
	result := &rcLinkCollectionImpl{
		field:      local,
		otherField: &RefCountedLinkedSetSymbol{EntitySymbol: remote},
	}
	store.refCountedLinks[local.GetName()] = result
	return result
}

func (store *BaseStore) GetLinkCollection(name string) LinkCollection {
	return store.links[name]
}

func (store *BaseStore) GetRefCountedLinkCollection(name string) RefCountedLinkCollection {
	return store.refCountedLinks[name]
}

func (store *BaseStore) BaseLoadOneById(tx *bbolt.Tx, id string, entity Entity) (bool, error) {
	if entity == nil {
		return false, errors.Errorf("cannot load into nil %v", store.GetEntityType())
	}

	bucket := store.GetEntityBucket(tx, []byte(id))
	if bucket == nil {
		if store.IsExtended() {
			bucket = store.parent.GetEntityBucket(tx, []byte(id))
			if bucket == nil {
				return false, nil
			}
			bucket = newTypedBucket(bucket, nil)
		} else {
			return false, nil
		}
	}

	entity.SetId(id)
	entity.LoadValues(store.impl, bucket)
	if bucket.HasError() {
		return false, bucket.GetError()
	}
	return true, nil
}

func (store *BaseStore) BaseLoadOneChildById(tx *bbolt.Tx, id string, childId string, entity Entity) (bool, error) {
	if entity == nil {
		return false, errors.Errorf("cannot load child into nil %v", store.GetEntityType())
	}

	parentBucket := store.GetEntityBucket(tx, []byte(id))
	if parentBucket == nil {
		return false, nil
	}
	bucket := parentBucket.GetPath(entity.GetEntityType(), childId)
	if bucket == nil {
		return false, nil
	}

	entity.SetId(childId)

	entity.LoadValues(store.impl, bucket)
	if bucket.HasError() {
		return false, bucket.GetError()
	}
	return true, nil
}

func (store *BaseStore) BaseLoadOneByQuery(tx *bbolt.Tx, query string, entity Entity) (bool, error) {
	ids, _, err := store.QueryIds(tx, query)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	return store.BaseLoadOneById(tx, ids[0], entity)
}

func (store *BaseStore) NewIndexingContext(isCreate bool, ctx MutateContext, id string, holder errorz.ErrorHolder) *IndexingContext {
	var parentContext *IndexingContext
	if store.parent != nil {
		parentContext = store.parent.NewIndexingContext(isCreate, ctx, id, holder)
	}
	return &IndexingContext{
		Parent:     parentContext,
		Indexer:    &store.Indexer,
		IsCreate:   isCreate,
		Ctx:        ctx,
		RowId:      []byte(id),
		ErrHolder:  holder,
		AtomStates: map[Constraint][]byte{},
		SetStates:  map[Constraint][]FieldTypeAndValue{},
	}
}

func (store *BaseStore) AddEvent(ctx MutateContext, entity Entity, name events.EventName) {
	if store.parent != nil {
		store.parent.AddEvent(ctx, store.parentMapper(entity), name)
	}
	ctx.AddEvent(store, name, entity)
}

func (store *BaseStore) Create(ctx MutateContext, entity Entity) error {
	if entity == nil {
		return errors.Errorf("cannot create %v from nil value", store.GetSingularEntityType())
	}

	if entity.GetEntityType() != store.GetEntityType() {
		return errors.Errorf("wrong type in create. expected %v, got instance of %v",
			store.GetEntityType(), entity.GetEntityType())
	}

	if entity.GetId() == "" {
		return errors.Errorf("cannot create %v with blank id", store.GetSingularEntityType())
	}

	if store.IsEntityPresent(ctx.Tx(), entity.GetId()) {
		return errors.Errorf("an entity of type %v already exists with id %v", store.GetSingularEntityType(), entity.GetId())
	}

	bucket := store.GetOrCreateEntityBucket(ctx.Tx(), []byte(entity.GetId()))
	persistCtx := &PersistContext{
		MutateContext: ctx,
		Id:            entity.GetId(),
		Store:         store.impl,
		Bucket:        bucket,
		IsCreate:      true,
	}
	entity.SetValues(persistCtx)
	indexingContext := store.NewIndexingContext(true, ctx, entity.GetId(), bucket)
	indexingContext.ProcessAfterUpdate()

	store.AddEvent(ctx, entity, EventCreate)
	return bucket.Err
}

func (store *BaseStore) Update(ctx MutateContext, entity Entity, checker FieldChecker) error {
	if entity == nil {
		return errors.Errorf("cannot update %v from nil value", store.GetSingularEntityType())
	}

	if entity.GetEntityType() != store.GetEntityType() {
		return errors.Errorf("wrong type in update. expected %v, got instance of %v",
			store.GetEntityType(), entity.GetEntityType())
	}

	if entity.GetId() == "" {
		return errors.Errorf("cannot update %v with blank id", store.GetSingularEntityType())
	}

	bucket := store.GetEntityBucket(ctx.Tx(), []byte(entity.GetId()))
	if bucket == nil {
		return store.entityNotFoundF(entity.GetId())
	}

	indexingContext := store.NewIndexingContext(false, ctx, entity.GetId(), bucket)
	indexingContext.ProcessBeforeUpdate() // remove old values, using existing values in store
	persistCtx := &PersistContext{
		MutateContext: ctx,
		Id:            entity.GetId(),
		Store:         store.impl,
		Bucket:        bucket,
		FieldChecker:  checker,
		IsCreate:      false,
	}
	entity.SetValues(persistCtx)
	indexingContext.ProcessAfterUpdate() // add new values, using updated values in store

	// for consistent ordering we do this before update handlers,
	// since child events will be generated there. It's ok to do
	// this early as if there is an error, the events won't be
	// propagated
	store.AddEvent(ctx, entity, EventUpdate)

	if err := store.updateHandlers.Handle(ctx, entity.GetId()); err != nil {
		return err
	}

	return bucket.Err
}

func (store *BaseStore) CreateChild(ctx MutateContext, id string, entity Entity) error {
	if entity == nil {
		return errors.Errorf("cannot create child of %v from nil value", store.GetEntityType())
	}

	if entity.GetId() == "" {
		entity.SetId(uuid.New().String())
	}

	parentBucket := store.GetEntityBucket(ctx.Tx(), []byte(id))
	if parentBucket == nil {
		return store.entityNotFoundF(id)
	}
	bucket := parentBucket.GetOrCreatePath(entity.GetEntityType(), entity.GetId())
	persistCtx := &PersistContext{
		MutateContext: ctx,
		Id:            entity.GetId(),
		Store:         store.impl,
		Bucket:        bucket,
		IsCreate:      true,
	}
	entity.SetValues(persistCtx)

	// TODO: Figure out how to handle child entities with emitter
	//if !bucket.HasError() {
	//	go store.Emit(EventCreate, entity)
	//}
	return bucket.Err
}

func (store *BaseStore) UpdateChild(ctx MutateContext, id string, entity Entity, checker FieldChecker) error {
	if entity == nil {
		return errors.Errorf("cannot update child of %v from nil value", store.GetEntityType())
	}

	if entity.GetId() == "" {
		return errors.Errorf("cannot update %v with blank id", entity.GetEntityType())
	}

	parentBucket := store.GetEntityBucket(ctx.Tx(), []byte(id))
	if parentBucket == nil {
		return store.entityNotFoundF(id)
	}
	bucket := parentBucket.GetPath(entity.GetEntityType(), entity.GetId())
	if bucket == nil {
		return store.entityNotFoundF(entity.GetId())
	}
	persistCtx := &PersistContext{
		MutateContext: ctx,
		Id:            entity.GetId(),
		Store:         store.impl,
		Bucket:        bucket,
		FieldChecker:  checker,
		IsCreate:      false,
	}
	entity.SetValues(persistCtx)

	// TODO: Figure out how to handle child entities with emitter
	//if !bucket.HasError() {
	//	go store.Emit(EventUpdate, entity)
	//}
	return bucket.Err
}

func (store *BaseStore) DeleteChild(ctx MutateContext, id string, entity Entity) error {
	if entity == nil {
		return errors.Errorf("cannot update child of %v from nil value", store.GetEntityType())
	}

	if entity.GetId() == "" {
		return errors.Errorf("cannot update %v with blank id", entity.GetEntityType())
	}

	parentBucket := store.GetEntityBucket(ctx.Tx(), []byte(id))
	if parentBucket == nil {
		return store.entityNotFoundF(id)
	}
	childrenBucket := parentBucket.GetPath(entity.GetEntityType())
	if childrenBucket == nil {
		return store.entityNotFoundF(entity.GetId())
	}
	bucket := childrenBucket.GetBucket(entity.GetId())
	if bucket == nil {
		return store.entityNotFoundF(entity.GetId())
	}
	if err := childrenBucket.DeleteBucket([]byte(entity.GetId())); err != nil {
		return err
	}

	// TODO: Figure out how to handle child entities with emitter
	//if !bucket.HasError() {
	//	go store.Emit(EventDelete, entity)
	//}
	return bucket.Err
}

func (store *BaseStore) ListChildIds(tx *bbolt.Tx, id string, childType string) []string {
	parentBucket := store.GetEntityBucket(tx, []byte(id))
	if parentBucket == nil {
		return nil
	}
	childrenBucket := parentBucket.GetPath(childType)
	if childrenBucket == nil {
		return nil
	}
	var result []string
	cursor := childrenBucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		result = append(result, string(key))
	}
	return result
}

func (store *BaseStore) GetRelatedEntitiesIdList(tx *bbolt.Tx, id string, field string) []string {
	bucket := store.GetEntityBucket(tx, []byte(id))
	if bucket == nil {
		return nil
	}
	return bucket.GetStringList(field)
}

func (store *BaseStore) GetRelatedEntitiesCursor(tx *bbolt.Tx, id string, field string, forward bool) ast.SetCursor {
	bucket := store.GetEntityBucket(tx, []byte(id))
	if bucket == nil {
		return ast.NewEmptyCursor()
	}
	listBucket := bucket.GetBucket(field)
	if listBucket == nil {
		return ast.NewEmptyCursor()
	}
	return listBucket.OpenTypedCursor(tx, forward)
}

func (store *BaseStore) IsEntityRelated(tx *bbolt.Tx, id string, field string, relatedEntityId string) bool {
	bucket := store.GetEntityBucket(tx, []byte(id))
	if bucket == nil {
		return false
	}
	listBucket := bucket.GetBucket(field)
	if listBucket == nil {
		return false
	}
	key := PrependFieldType(TypeString, []byte(relatedEntityId))
	return listBucket.IsKeyPresent(key)
}

func (store *BaseStore) IsChildStore() bool {
	return store.parent != nil
}

func (store *BaseStore) IsEntityPresent(tx *bbolt.Tx, id string) bool {
	return nil != store.GetEntityBucket(tx, []byte(id))
}

func (store *BaseStore) cleanupLinks(tx *bbolt.Tx, id string, holder errorz.ErrorHolder) {
	// cascade delete n-n links
	for _, val := range store.links {
		if !holder.HasError() {
			holder.SetError(val.EntityDeleted(tx, id))
		}
	}

	for _, val := range store.refCountedLinks {
		if !holder.HasError() {
			holder.SetError(val.EntityDeleted(tx, id))
		}
	}
}

func (store *BaseStore) cleanupExternal(ctx MutateContext, id string) error {
	errHolder := &errorz.ErrorHolderImpl{}
	indexingContext := store.NewIndexingContext(false, ctx, id, errHolder)
	indexingContext.ProcessBeforeDelete()
	store.cleanupLinks(ctx.Tx(), id, errHolder)
	return errHolder.Err
}

func (store *BaseStore) DeleteById(ctx MutateContext, id string) error {
	if store.parent != nil {
		// this will trigger call to CleanupExternal here based on delete handlers
		return store.parent.DeleteById(ctx, id)
	}

	entity := store.impl.NewStoreEntity()
	found, err := store.BaseLoadOneById(ctx.Tx(), id, entity)
	if err != nil {
		return err
	}
	if !found {
		return store.entityNotFoundF(id)
	}

	// for consistent ordering we do this before delete handlers,
	// since child events will be generated there. It's ok to do
	// this early as if there is an error, the events won't be
	// propagated
	store.AddEvent(ctx, entity, EventDelete)

	if err := store.deleteHandlers.Handle(ctx, id); err != nil {
		return err
	}

	if err := store.impl.cleanupExternal(ctx, id); err != nil {
		return err
	}

	// delete entity
	bucket := store.GetEntitiesBucket(ctx.Tx())
	if bucket == nil {
		return nil
	}
	bucket.DeleteEntity(id)

	return bucket.Err
}

func (store *BaseStore) DeleteWhere(ctx MutateContext, query string) error {
	ids, _, err := store.QueryIds(ctx.Tx(), query)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := store.impl.DeleteById(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (*BaseStore) FindMatching(tx *bbolt.Tx, readIndex SetReadIndex, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	if len(values) == 1 {
		readIndex.Read(tx, []byte(values[0]), func(val []byte) {
			result = append(result, string(val))
		})
	} else {
		rest := values[1:]
		readIndex.Read(tx, []byte(values[0]), func(val []byte) {
			currentRowValues := readIndex.GetSymbol().EvalStringList(tx, val)
			for _, required := range rest {
				if !stringz.Contains(currentRowValues, required) {
					return
				}
			}
			result = append(result, string(val))
		})
	}
	return result
}

func (*BaseStore) FindMatchingAnyOf(tx *bbolt.Tx, readIndex SetReadIndex, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	if len(values) == 1 {
		readIndex.Read(tx, []byte(values[0]), func(val []byte) {
			result = append(result, string(val))
		})
		return result
	}

	// If there are multiple roles, we want to avoid duplicates
	set := map[string]struct{}{}
	for _, role := range values {
		readIndex.Read(tx, []byte(role), func(val []byte) {
			set[string(val)] = struct{}{}
		})
	}

	for key := range set {
		result = append(result, key)
	}

	return result
}

func (*BaseStore) IteratorMatchingAllOf(readIndex SetReadIndex, values []string) ast.SetCursorProvider {
	if len(values) == 0 {
		return ast.OpenEmptyCursor
	}

	if len(values) == 1 {
		return func(tx *bbolt.Tx, forward bool) ast.SetCursor {
			return readIndex.OpenValueCursor(tx, []byte(values[0]), forward)
		}
	}

	return func(tx *bbolt.Tx, forward bool) ast.SetCursor {
		cursor := readIndex.OpenValueCursor(tx, []byte(values[0]), forward)
		return ast.NewFilteredCursor(cursor, func(val []byte) bool {
			currentRowValues := readIndex.GetSymbol().EvalStringList(tx, val)
			return stringz.ContainsAll(currentRowValues, values[1:]...)
		})
	}
}

func (*BaseStore) IteratorMatchingAnyOf(readIndex SetReadIndex, values []string) ast.SetCursorProvider {
	if len(values) == 0 {
		return ast.OpenEmptyCursor
	}

	if len(values) == 1 {
		return func(tx *bbolt.Tx, forward bool) ast.SetCursor {
			return readIndex.OpenValueCursor(tx, []byte(values[0]), forward)
		}
	}

	return func(tx *bbolt.Tx, forward bool) ast.SetCursor {
		set := ast.NewTreeSet(forward)
		for _, role := range values {
			readIndex.Read(tx, []byte(role), func(val []byte) {
				set.Add(val)
			})
		}
		return set.ToCursor()
	}
}

func (store *BaseStore) CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(err error, fixed bool)) error {
	for _, linkCollection := range store.links {
		if err := linkCollection.CheckIntegrity(tx, fix, errorSink); err != nil {
			return err
		}
	}
	for _, constraint := range store.Indexer.constraints {
		if err := constraint.CheckIntegrity(tx, fix, errorSink); err != nil {
			return err
		}
	}
	return nil
}
