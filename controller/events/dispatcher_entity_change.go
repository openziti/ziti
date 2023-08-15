/*
	Copyright NetFoundry Inc.

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

package events

import (
	"context"
	"encoding/binary"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/v2/genext"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
	"time"
)

const (
	entityChangeEventsBucket = "entityChangeEvents"
)

func (self *Dispatcher) AddEntityChangeEventHandler(handler event.EntityChangeEventHandler) {
	self.entityChangeEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveEntityChangeEventHandler(handler event.EntityChangeEventHandler) {
	self.entityChangeEventHandlers.DeleteIf(func(val event.EntityChangeEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.EntityChangeEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptEntityChangeEvent(event *event.EntityChangeEvent) {
	// don't do these in a separate goroutine to minimize the chance of losing events
	// If we need to, the handler can spin up a separate goroutine
	for _, handler := range self.entityChangeEventHandlers.Value() {
		handler.AcceptEntityChangeEvent(event)
	}
}

func (self *Dispatcher) registerEntityChangeEventHandler(val interface{}, options map[string]interface{}) error {
	handler, ok := val.(event.EntityChangeEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/EntityChangeEventHandler interface.", reflect.TypeOf(val))
	}

	propagateAlways := false
	if val, found := options["propagateAlways"]; found {
		if b, ok := val.(bool); ok {
			propagateAlways = b
		} else if s, ok := val.(string); ok {
			propagateAlways = strings.EqualFold(s, "true")
		} else {
			return errors.New("invalid value for entityChange.propagateAlways, must be boolean or string")
		}
	}

	includeParentEvents := false
	if val, found := options["includeParentEvents"]; found {
		if b, ok := val.(bool); ok {
			includeParentEvents = b
		} else if s, ok := val.(string); ok {
			includeParentEvents = strings.EqualFold(s, "true")
		} else {
			return errors.New("invalid value for entityChange.includeParentEvents, must be boolean or string")
		}
	}

	filter := &entityChangeEventFilter{
		EntityChangeEventHandler: handler,
		propagateAlways:          propagateAlways,
		includeParentEvents:      includeParentEvents,
	}

	if val, found := options["include"]; found {
		includes := map[string]struct{}{}
		if list, ok := val.([]interface{}); ok {
			for _, val := range list {
				if entityType, ok := val.(string); ok {
					includes[entityType] = struct{}{}
				} else {
					return errors.Errorf("invalid value type [%T] for entityChange include list, must be string list", val)
				}
			}
		} else {
			return errors.Errorf("invalid value type [%T] for entityChange include list, must be string list", val)
		}

		if len(includes) == 0 {
			return errors.Errorf("no values provided in include list for entityChange events, either drop includes stanza or provide at least one entity type to include")
		}

		for entityType := range includes {
			if !genext.Contains(self.entityTypes, entityType) {
				return errors.Errorf("invalid entity type [%v] in entityChange events include list, valid values include: %v", entityType, self.entityTypes)
			}
		}

		filter.entityTypes = includes
	}

	self.AddEntityChangeEventHandler(filter)

	return nil
}

func (self *Dispatcher) unregisterEntityChangeEventHandler(val interface{}) {
	if handler, ok := val.(event.EntityChangeEventHandler); ok {
		self.RemoveEntityChangeEventHandler(handler)
	}
}

func (self *Dispatcher) initEntityChangeEvents(n *network.Network) {
	self.entityChangeEventsDispatcher.network = n
	for _, store := range n.GetStores().GetStoreList() {
		self.AddEntityChangeSource(store)
	}
	self.AddGlobalEntityChangeMetadata("version", n.VersionProvider.Version())
	go self.entityChangeEventsDispatcher.flushLoop()
}

func (self *Dispatcher) AddEntityChangeSource(store boltz.Store) {
	store.AddUntypedEntityConstraint(&self.entityChangeEventsDispatcher)
	self.entityTypes = append(self.entityTypes, store.GetEntityType())
}

func (self *Dispatcher) AddGlobalEntityChangeMetadata(k string, v any) {
	self.entityChangeEventsDispatcher.globalMetadata[k] = v
}

func txIdToBytes(txId uint64) []byte {
	return binary.LittleEndian.AppendUint64(nil, txId)
}

func bytesToTxId(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

type entityChangeEventDispatcher struct {
	network        *network.Network
	dispatcher     *Dispatcher
	notifyCh       chan struct{}
	globalMetadata map[string]any
}

func (self *entityChangeEventDispatcher) logTxEvent(state boltz.UntypedEntityChangeState) error {
	tx := state.GetCtx().Tx()
	rowId := txIdToBytes(uint64(tx.ID()))
	eventsBucket := boltz.GetOrCreatePath(tx, db.RootBucket, db.MetadataBucket, entityChangeEventsBucket)
	txBucket, err := eventsBucket.CreateBucketIfNotExists(rowId)
	if err != nil {
		return err
	}
	return txBucket.Put([]byte(state.GetEventId()), []byte(state.GetStore().GetEntityType()))
}

func (self *entityChangeEventDispatcher) processPreviousTxEvents(tx *bbolt.Tx, emit bool) {
	currentTxId := uint64(tx.ID())

	eventsBucket := boltz.GetOrCreatePath(tx, db.RootBucket, db.MetadataBucket, entityChangeEventsBucket)
	txCursor := eventsBucket.OpenCursor(tx, true)

	var toDelete []uint64
	for txCursor.IsValid() {
		rowId := txCursor.Current()
		txId := bytesToTxId(rowId)
		log := pfxlog.Logger().WithField("txId", txId)
		if txId != currentTxId {
			log.Debug("cleaning up entity change events for tx")

			if emit {
				txBucket := eventsBucket.GetBucketByKey(rowId)
				eventIdCursor := txBucket.Cursor()
				for k, v := eventIdCursor.First(); k != nil; k, v = eventIdCursor.Next() {
					eventId := string(k)
					entityType := string(v)
					log.WithField("eventId", eventId).WithField("entityType", entityType).Debug("emitting event for tx")
					self.emitRecoveryEvent(eventId, entityType)
					eventIdCursor.Next()
				}
			}
			toDelete = append(toDelete, txId)
		}
		txCursor.Next()
	}

	if len(toDelete) > 0 {
		for _, txId := range toDelete {
			if err := eventsBucket.DeleteBucket(txIdToBytes(txId)); err != nil {
				pfxlog.Logger().WithError(err).WithField("txId", txId).Error("unable to delete event bucket for tx")
			}
		}
	}
}

func (self *entityChangeEventDispatcher) ProcessPreCommit(state boltz.UntypedEntityChangeState) error {
	self.processPreviousTxEvents(state.GetCtx().Tx(), false)

	var changeType event.EntityChangeEventType
	if state.GetChangeType() == boltz.EntityCreated {
		changeType = event.EntityChangeTypeEntityCreated
	} else if state.GetChangeType() == boltz.EntityUpdated {
		changeType = event.EntityChangeTypeEntityUpdated
	} else if state.GetChangeType() == boltz.EntityDeleted {
		changeType = event.EntityChangeTypeEntityDeleted
	}

	isParentEvent := state.IsParentEvent()
	evt := &event.EntityChangeEvent{
		Namespace:          event.EntityChangeEventsNs,
		EventId:            state.GetEventId(),
		EventType:          changeType,
		EntityType:         state.GetStore().GetEntityType(),
		IsParentEvent:      &isParentEvent,
		Timestamp:          time.Now(),
		Metadata:           map[string]any{},
		InitialState:       state.GetInitialState(),
		FinalState:         state.GetFinalState(),
		PropagateIndicator: self.network.Dispatcher.IsLeaderOrLeaderless(),
	}

	changeCtx := change.FromContext(state.GetCtx().Context())
	if changeCtx == nil {
		changeCtx = change.New()
		state.GetCtx().UpdateContext(func(ctx context.Context) context.Context {
			return changeCtx.AddToContext(ctx)
		})
	}

	changeCtx.PopulateMetadata(evt.Metadata)

	for k, v := range self.globalMetadata {
		evt.Metadata[k] = v
	}

	self.dispatcher.AcceptEntityChangeEvent(evt)
	return self.logTxEvent(state)
}

func (self *entityChangeEventDispatcher) emitRecoveryEvent(eventId string, entityType string) {
	evt := &event.EntityChangeEvent{
		Namespace:       event.EntityChangeEventsNs,
		EventId:         eventId,
		EntityType:      entityType,
		EventType:       event.EntityChangeTypeCommitted,
		Timestamp:       time.Now(),
		IsRecoveryEvent: true,
	}
	self.dispatcher.AcceptEntityChangeEvent(evt)
}

func (self *entityChangeEventDispatcher) ProcessPostCommit(state boltz.UntypedEntityChangeState) {
	isParentEvent := state.IsParentEvent()
	evt := &event.EntityChangeEvent{
		Namespace:          event.EntityChangeEventsNs,
		EventId:            state.GetEventId(),
		EventType:          event.EntityChangeTypeCommitted,
		EntityType:         state.GetStore().GetEntityType(),
		Timestamp:          time.Now(),
		IsParentEvent:      &isParentEvent,
		PropagateIndicator: self.network.Dispatcher.IsLeaderOrLeaderless(),
	}
	self.dispatcher.AcceptEntityChangeEvent(evt)
	self.notifyFlush()
}

func (self *entityChangeEventDispatcher) notifyFlush() {
	select {
	case self.notifyCh <- struct{}{}:
	default:
	}
}

func (self *entityChangeEventDispatcher) flushLoop() {
	for {
		// wait to be notified of an event
		<-self.notifyCh

		// wait until we've not gotten an event for 5 seconds before cleaning up
		flushed := false
		for !flushed {
			select {
			case <-self.notifyCh:
			case <-time.After(5 * time.Second):
				pfxlog.Logger().Debug("cleaning up entity change events")
				self.flushCommittedTxEvents(false)
				flushed = true
			}
		}
	}
}

func (self *entityChangeEventDispatcher) flushCommittedTxEvents(emit bool) {
	err := self.network.GetDb().Update(nil, func(ctx boltz.MutateContext) error {
		self.processPreviousTxEvents(ctx.Tx(), emit)
		return nil
	})
	if err != nil {
		pfxlog.Logger().WithError(err).Error("error while flushing committed tx entity change events")
	}
}

type entityChangeEventFilter struct {
	event.EntityChangeEventHandler
	propagateAlways     bool
	includeParentEvents bool
	entityTypes         map[string]struct{}
}

func (self *entityChangeEventFilter) IsWrapping(value event.EntityChangeEventHandler) bool {
	if self.EntityChangeEventHandler == value {
		return true
	}
	if w, ok := self.EntityChangeEventHandler.(event.EntityChangeEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

func (self *entityChangeEventFilter) AcceptEntityChangeEvent(evt *event.EntityChangeEvent) {
	if !evt.IsRecoveryEvent {
		if !self.propagateAlways && !evt.PropagateIndicator {
			return
		}

		if *evt.IsParentEvent && !self.includeParentEvents {
			return
		}
	}

	if self.entityTypes != nil {
		if _, found := self.entityTypes[evt.EntityType]; !found {
			return
		}
	}

	if evt.EventType == event.EntityChangeTypeCommitted {
		evt.IsParentEvent = nil
		evt.EntityType = ""
	}

	self.EntityChangeEventHandler.AcceptEntityChangeEvent(evt)
}
