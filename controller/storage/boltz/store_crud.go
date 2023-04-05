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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/genext"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

func (store *BaseStore[E]) GetParentStore() CrudBaseStore {
	return store.parent
}

func (store *BaseStore[E]) NewStoreEntity() E {
	return store.entityStrategy.New()
}

func (store *BaseStore[E]) GetEntityStrategy() EntityStrategy[E] {
	return store.entityStrategy
}

func (store *BaseStore[E]) AddLinkCollection(local EntitySymbol, remote EntitySymbol) LinkCollection {
	result := &linkCollectionImpl{
		field:      local,
		otherField: &LinkedSetSymbol{EntitySymbol: remote},
	}
	store.links[local.GetName()] = result
	return result
}

func (store *BaseStore[E]) AddRefCountedLinkCollection(local EntitySymbol, remote EntitySymbol) RefCountedLinkCollection {
	result := &rcLinkCollectionImpl{
		field:      local,
		otherField: &RefCountedLinkedSetSymbol{EntitySymbol: remote},
	}
	store.refCountedLinks[local.GetName()] = result
	return result
}

func (store *BaseStore[E]) GetLinkCollection(name string) LinkCollection {
	return store.links[name]
}

func (store *BaseStore[E]) getLinks() map[string]LinkCollection {
	return store.links
}

func (store *BaseStore[E]) GetRefCountedLinkCollection(name string) RefCountedLinkCollection {
	return store.refCountedLinks[name]
}

func (store *BaseStore[E]) defaultEntityValue() E {
	var result E
	return result
}

func (store *BaseStore[E]) LoadEntity(tx *bbolt.Tx, id string, entity E) (bool, error) {
	bucket := store.getEntityBucketForLoad(tx, id)
	if bucket == nil {
		return false, nil
	}

	entity.SetId(id)
	store.entityStrategy.LoadEntity(entity, bucket)
	if bucket.HasError() {
		return false, bucket.GetError()
	}
	return true, nil
}

func (store *BaseStore[E]) FindById(tx *bbolt.Tx, id string) (E, bool, error) {
	bucket := store.getEntityBucketForLoad(tx, id)
	if bucket == nil {
		return store.defaultEntityValue(), false, nil
	}

	entity := store.entityStrategy.New()
	entity.SetId(id)
	store.entityStrategy.LoadEntity(entity, bucket)
	if bucket.HasError() {
		return store.defaultEntityValue(), false, bucket.GetError()
	}
	return entity, true, nil
}

func (store *BaseStore[E]) getEntityBucketForLoad(tx *bbolt.Tx, id string) *TypedBucket {
	bucket := store.GetEntityBucket(tx, []byte(id))
	if bucket == nil {
		if store.IsExtended() {
			bucket = store.parent.GetEntityBucket(tx, []byte(id))
			if bucket == nil {
				return nil
			}
			return NewTypedBucket(bucket, nil)
		}
	}
	return bucket
}

func (store *BaseStore[E]) FindOneByQuery(tx *bbolt.Tx, query string) (E, bool, error) {
	ids, _, err := store.QueryIds(tx, query)
	if err != nil {
		return store.defaultEntityValue(), false, err
	}
	if len(ids) == 0 {
		return store.defaultEntityValue(), false, nil
	}
	entity, found, err := store.FindById(tx, ids[0])
	if !found || err != nil {
		return store.defaultEntityValue(), found, err
	}
	return entity, found, err
}

func (store *BaseStore[E]) NewIndexingContext(isCreate bool, ctx MutateContext, id string, holder errorz.ErrorHolder) *IndexingContext {
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

func (store *BaseStore[E]) newEntityChangeFlow() EntityChangeFlow {
	return &EntityChangeState[E]{
		store: store,
	}
}

func (store *BaseStore[E]) fireParentEvent(changeFlow EntityChangeFlow) error {
	if store.parent != nil {
		parentEntityChangeFlow := store.parent.newEntityChangeFlow()
		parentEntityChangeFlow.InitFromChild(changeFlow)
		return parentEntityChangeFlow.FireEvents()
	}
	return nil
}

// Create stores a new entity in the datastore
//
// Creates must be called on the top level, so we don't need to worry about created
// being called on a parent store.
func (store *BaseStore[E]) Create(ctx MutateContext, entity E) error {
	var tmp Entity = entity
	if tmp == nil || reflect.ValueOf(tmp).IsNil() {
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
	store.entityStrategy.PersistEntity(entity, persistCtx)
	indexingContext := store.NewIndexingContext(true, ctx, entity.GetId(), bucket)
	indexingContext.ProcessAfterUpdate()

	changeFlow := &EntityChangeState[E]{
		ChangeType: EntityCreated,
		Id:         entity.GetId(),
		Ctx:        ctx,
		FinalState: entity,
		store:      store,
	}

	if err := changeFlow.LoadFinalState(); err != nil {
		return err
	}

	if err := store.fireParentEvent(changeFlow); err != nil {
		return err
	}

	if err := changeFlow.FireEvents(); err != nil {
		return err
	}

	return bucket.Err
}

func (store *BaseStore[E]) Update(ctx MutateContext, entity E, checker FieldChecker) error {
	var tmp Entity = entity
	if tmp == nil || reflect.ValueOf(tmp).IsNil() {
		return errors.Errorf("cannot update %v from nil value", store.GetSingularEntityType())
	}

	for _, childStoreStrategy := range store.childStoreStragies {
		if handled, err := childStoreStrategy.HandleUpdate(ctx, entity, checker); handled {
			return err
		}
	}

	if entity.GetEntityType() != store.GetEntityType() {
		return errors.Errorf("wrong type in update. expected %v, got instance of %v",
			store.GetEntityType(), entity.GetEntityType())
	}

	baseEntity, found, err := store.FindById(ctx.Tx(), entity.GetId())
	if err != nil {
		return err
	}

	if !found {
		return store.entityNotFoundF(entity.GetId())
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
	store.entityStrategy.PersistEntity(entity, persistCtx)
	indexingContext.ProcessAfterUpdate() // add new values, using updated values in store

	changeFlow := &EntityChangeState[E]{
		ChangeType:   EntityUpdated,
		Id:           entity.GetId(),
		Ctx:          ctx,
		InitialState: baseEntity,
		store:        store,
	}

	if err = changeFlow.LoadFinalState(); err != nil {
		return err
	}

	if err = store.fireParentEvent(changeFlow); err != nil {
		return err
	}

	if err = changeFlow.FireEvents(); err != nil {
		return err
	}

	return bucket.Err
}

func (store *BaseStore[E]) GetRelatedEntitiesIdList(tx *bbolt.Tx, id string, field string) []string {
	bucket := store.GetEntityBucket(tx, []byte(id))
	if bucket == nil {
		return nil
	}
	return bucket.GetStringList(field)
}

func (store *BaseStore[E]) GetRelatedEntitiesCursor(tx *bbolt.Tx, id string, field string, forward bool) ast.SetCursor {
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

func (store *BaseStore[E]) IsEntityRelated(tx *bbolt.Tx, id string, field string, relatedEntityId string) bool {
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

func (store *BaseStore[E]) IsChildStore() bool {
	return store.parent != nil
}

func (store *BaseStore[E]) IsEntityPresent(tx *bbolt.Tx, id string) bool {
	return nil != store.GetEntityBucket(tx, []byte(id))
}

func (store *BaseStore[E]) cleanupLinks(tx *bbolt.Tx, id string, holder errorz.ErrorHolder) {
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

func (store *BaseStore[E]) processDeleteConstraints(ctx MutateContext, id string) (EntityChangeFlow, error) {
	changeFlow := &EntityChangeState[E]{
		ChangeType: EntityDeleted,
		Id:         id,
		store:      store,
	}

	found, err := changeFlow.Init(ctx)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil
	}

	errHolder := &errorz.ErrorHolderImpl{}

	indexingContext := store.NewIndexingContext(false, ctx, id, errHolder)
	indexingContext.ProcessBeforeDelete()
	store.cleanupLinks(ctx.Tx(), id, errHolder)

	return changeFlow, errHolder.Err
}

func (store *BaseStore[E]) DeleteById(ctx MutateContext, id string) error {
	if store.parent != nil {
		// this will trigger call to CleanupExternal here based on delete handlers
		return store.parent.DeleteById(ctx, id)
	}

	entity, found, err := store.FindById(ctx.Tx(), id)
	if err != nil {
		return err
	}
	if !found {
		return store.entityNotFoundF(id)
	}

	var changeFlows = []EntityChangeFlow{nil}
	for _, handler := range store.childStoreStragies {
		if changeFlow, err := handler.GetStore().processDeleteConstraints(ctx, id); err != nil {
			return err
		} else if changeFlow != nil {
			changeFlows = append(changeFlows, changeFlow)
		}
		if err = handler.HandleDelete(ctx, entity); err != nil {
			return err
		}
	}

	if changeFlows[0], err = store.impl.processDeleteConstraints(ctx, id); err != nil {
		return err
	}

	// delete entity
	bucket := store.GetEntitiesBucket(ctx.Tx())
	if bucket == nil {
		return nil
	}
	bucket.DeleteEntity(id)

	if bucket.Err != nil {
		return bucket.Err
	}

	for _, changeFlow := range changeFlows {
		if err = changeFlow.FireEvents(); err != nil {
			return nil
		}
	}

	return nil
}

func (store *BaseStore[E]) DeleteWhere(ctx MutateContext, query string) error {
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

func (*BaseStore[E]) FindMatching(tx *bbolt.Tx, readIndex SetReadIndex, values []string) []string {
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

func (*BaseStore[E]) FindMatchingAnyOf(tx *bbolt.Tx, readIndex SetReadIndex, values []string) []string {
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

func (*BaseStore[E]) IteratorMatchingAllOf(readIndex SetReadIndex, values []string) ast.SetCursorProvider {
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

func (*BaseStore[E]) IteratorMatchingAnyOf(readIndex SetReadIndex, values []string) ast.SetCursorProvider {
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

func (store *BaseStore[E]) CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(err error, fixed bool)) error {
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

func (store *BaseStore[E]) AddEntityConstraint(constraint EntityConstraint[E]) {
	store.entityConstraints.Append(constraint)
}

func (store *BaseStore[E]) AddEntityEventListener(listener EntityEventListener[E], changeType EntityEventType, changeTypes ...EntityEventType) {
	store.entityConstraints.Append(&EntityConstraintWrapper[E]{
		eventListener: listener,
		changeTypes:   append([]EntityEventType{changeType}, changeTypes...),
	})
}

func (store *BaseStore[E]) AddListener(listener func(Entity), changeType EntityEventType, changeTypes ...EntityEventType) {
	store.entityConstraints.Append(&untypedEntityConstraintWrapper[E]{
		eventListener: listener,
		changeTypes:   append([]EntityEventType{changeType}, changeTypes...),
	})
}

type EntityConstraintWrapper[E Entity] struct {
	eventListener EntityEventListener[E]
	changeTypes   []EntityEventType
}

func (self *EntityConstraintWrapper[E]) ProcessPreCommit(*EntityChangeState[E]) error {
	return nil
}

func (self *EntityConstraintWrapper[E]) ProcessPostCommit(state *EntityChangeState[E]) {
	if state.ChangeType == EntityCreated && genext.Contains(self.changeTypes, EntityCreated) {
		self.eventListener.HandleEntityEvent(state.FinalState)
	} else if state.ChangeType == EntityUpdated && genext.Contains(self.changeTypes, EntityUpdated) {
		self.eventListener.HandleEntityEvent(state.FinalState)
	} else if state.ChangeType == EntityDeleted && genext.Contains(self.changeTypes, EntityDeleted) {
		self.eventListener.HandleEntityEvent(state.InitialState)
	}
}

type untypedEntityConstraintWrapper[E Entity] struct {
	eventListener func(Entity)
	changeTypes   []EntityEventType
}

func (self *untypedEntityConstraintWrapper[E]) ProcessPreCommit(*EntityChangeState[E]) error {
	return nil
}

func (self *untypedEntityConstraintWrapper[E]) ProcessPostCommit(state *EntityChangeState[E]) {
	if state.ChangeType == EntityCreated && genext.Contains(self.changeTypes, EntityCreated) {
		self.eventListener(state.FinalState)
	} else if state.ChangeType == EntityUpdated && genext.Contains(self.changeTypes, EntityUpdated) {
		self.eventListener(state.FinalState)
	} else if state.ChangeType == EntityDeleted && genext.Contains(self.changeTypes, EntityDeleted) {
		self.eventListener(state.InitialState)
	}
}

type ChildStoreUpdateHandler[E Entity, C Entity] struct {
	Store  CrudStore[C]
	Mapper func(ctx MutateContext, parent E) (C, bool)
}

func (self *ChildStoreUpdateHandler[E, C]) HandleUpdate(ctx MutateContext, entity E, checker FieldChecker) (bool, error) {
	if child, handle := self.Mapper(ctx, entity); handle {
		return true, self.Store.Update(ctx, child, checker)
	}
	return false, nil
}

func (self *ChildStoreUpdateHandler[E, C]) HandleDelete(ctx MutateContext, entity E) error {
	return nil
}

func (self *ChildStoreUpdateHandler[E, C]) GetStore() CrudBaseStore {
	return self.Store
}
