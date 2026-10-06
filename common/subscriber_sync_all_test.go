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

package common

import (
	"fmt"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// unsubscribingSubscriber unsubscribes on the notifying goroutine when told its identity is gone, as an
// edge client connection does by closing its channel, whose close handler runs synchronously.
type unsubscribingSubscriber struct {
	rdm     *RouterDataModel
	deleted chan struct{}
}

var _ IdentityEventSubscriber = (*unsubscribingSubscriber)(nil)

func (self *unsubscribingSubscriber) NotifyIdentityEvent(state *IdentityState, eventType IdentityEventType) {
	if eventType == IdentityDeletedEvent {
		self.rdm.UnsubscribeFromIdentityChanges(state.Identity.Id, self)
		close(self.deleted)
	}
}

func (self *unsubscribingSubscriber) NotifyServiceChange(*IdentityState, *IdentityService, *IdentityService, ServiceEventType) {
}

func (self *unsubscribingSubscriber) NotifyBatchComplete(*RouterDataModel, uint64) {
}

// Test_SyncAllSubscribers_UnsubscribeOnDeletedIdentity: syncing subscribers into a replacement model that
// no longer has a subscribed identity notifies the subscriber, which can unsubscribe from inside the
// notification and the sync still completes.
func Test_SyncAllSubscribers_UnsubscribeOnDeletedIdentity(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	old := NewReceiverRouterDataModel("r1", closeNotify)
	event, model := identityCreateEvent("identity1")
	old.HandleIdentityEvent(1, event, model)

	sub := &unsubscribingSubscriber{rdm: old, deleted: make(chan struct{})}
	old.SubscribeToIdentityChanges("identity1", sub, false)

	replacement := NewReceiverRouterDataModel("r1", closeNotify)
	replacement.InheritLocalData(old)
	sub.rdm = replacement

	done := make(chan struct{})
	go func() {
		replacement.SyncAllSubscribers()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		req.FailNow("syncing subscribers did not complete")
	}

	select {
	case <-sub.deleted:
	default:
		req.FailNow("subscriber was not told its identity was deleted")
	}
	req.False(replacement.subscriptions.Has("identity1"), "the subscriber's unsubscribe must remove its subscription")
}

// swappingSubscriber, when told its identity is gone, replaces the last listener on a target identity
// with a new one and then updates the target, all while the sync is still running.
type swappingSubscriber struct {
	rdm         *RouterDataModel
	targetId    string
	oldListener IdentityEventSubscriber
	newListener IdentityEventSubscriber
}

var _ IdentityEventSubscriber = (*swappingSubscriber)(nil)

func (self *swappingSubscriber) NotifyIdentityEvent(_ *IdentityState, eventType IdentityEventType) {
	if eventType == IdentityDeletedEvent {
		self.rdm.UnsubscribeFromIdentityChanges(self.targetId, self.oldListener)
		self.rdm.SubscribeToIdentityChanges(self.targetId, self.newListener, false)
		model := &edge_ctrl_pb.DataState_Event_Identity{
			Identity: &edge_ctrl_pb.DataState_Identity{Id: self.targetId, Name: "renamed"},
		}
		self.rdm.HandleIdentityEvent(3, &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update, Model: model}, model)
	}
}

func (self *swappingSubscriber) NotifyServiceChange(*IdentityState, *IdentityService, *IdentityService, ServiceEventType) {
}

func (self *swappingSubscriber) NotifyBatchComplete(*RouterDataModel, uint64) {
}

// barrierEvent closes done when the subscriber event loop reaches it, after every event queued before it.
type barrierEvent struct {
	done chan struct{}
}

func (self barrierEvent) process(*RouterDataModel) {
	close(self.done)
}

// idIteratedAfter returns an id that IterCb always visits after first, by placing it in a later shard.
func idIteratedAfter(first string) string {
	m := cmap.New[struct{}]()
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("target%d", i)
		if m.GetShard(candidate) == m.GetShard(first) {
			continue
		}
		m.Set(first, struct{}{})
		m.Set(candidate, struct{}{})
		var order []string
		m.IterCb(func(key string, _ struct{}) {
			order = append(order, key)
		})
		if order[0] == first {
			return candidate
		}
		m.Remove(candidate)
	}
}

// Test_SyncAllSubscribers_ReplacedSubscriptionGetsUpdate: when a subscription is replaced during the sync
// and its identity then updated, the sync checks the replacement rather than the subscription it
// collected, so the new listener sees the update.
func Test_SyncAllSubscribers_ReplacedSubscriptionGetsUpdate(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	triggerId := "trigger"
	targetId := idIteratedAfter(triggerId)

	old := NewReceiverRouterDataModel("r1", closeNotify)
	event, model := identityCreateEvent(triggerId)
	old.HandleIdentityEvent(1, event, model)
	event, model = identityCreateEvent(targetId)
	old.HandleIdentityEvent(2, event, model)

	oldListener := &recordingIdentitySubscriber{events: make(chan recordedIdentityEvent, 16)}
	newListener := &recordingIdentitySubscriber{events: make(chan recordedIdentityEvent, 16)}
	trigger := &swappingSubscriber{rdm: old, targetId: targetId, oldListener: oldListener, newListener: newListener}
	old.SubscribeToIdentityChanges(triggerId, trigger, false)
	old.SubscribeToIdentityChanges(targetId, oldListener, false)

	// The replacement drops the trigger identity, so the sync tells the trigger it was deleted.
	replacement := NewReceiverRouterDataModel("r1", closeNotify)
	event, model = identityCreateEvent(targetId)
	replacement.HandleIdentityEvent(2, event, model)
	replacement.InheritLocalData(old)
	trigger.rdm = replacement

	done := make(chan struct{})
	go func() {
		replacement.SyncAllSubscribers()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		req.FailNow("syncing subscribers did not complete")
	}

	barrier := barrierEvent{done: make(chan struct{})}
	replacement.queueEvent(barrier)
	select {
	case <-barrier.done:
	case <-time.After(5 * time.Second):
		req.FailNow("subscriber event loop did not drain")
	}

	for {
		select {
		case evt := <-newListener.events:
			if evt.eventType == IdentityUpdatedEvent {
				req.Equal("renamed", evt.state.Identity.Name)
				return
			}
		default:
			req.FailNow("the replacement subscription's listener was not told of the update")
		}
	}
}
