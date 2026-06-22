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
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

type recordedIdentityEvent struct {
	state     *IdentityState
	eventType IdentityEventType
}

// recordingIdentitySubscriber records identity event notifications for assertions.
type recordingIdentitySubscriber struct {
	events chan recordedIdentityEvent
}

var _ IdentityEventSubscriber = (*recordingIdentitySubscriber)(nil)

func (self *recordingIdentitySubscriber) NotifyIdentityEvent(state *IdentityState, eventType IdentityEventType) {
	self.events <- recordedIdentityEvent{state: state, eventType: eventType}
}

func (self *recordingIdentitySubscriber) NotifyServiceChange(*IdentityState, *IdentityService, *IdentityService, ServiceEventType) {
}

func (self *recordingIdentitySubscriber) NotifyBatchComplete(*RouterDataModel, uint64) {
}

func identityCreateEvent(identityId string) (*edge_ctrl_pb.DataState_Event, *edge_ctrl_pb.DataState_Event_Identity) {
	model := &edge_ctrl_pb.DataState_Event_Identity{
		Identity: &edge_ctrl_pb.DataState_Identity{
			Id:   identityId,
			Name: identityId,
		},
	}
	return &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create, Model: model}, model
}

// Test_SubscribeToIdentityChanges_PendingIdentity covers subscribing before the identity has
// synced to the router's data model (e.g. an SDK connects to a router whose RDM lags the
// controller): the subscription must register as pending and deliver IdentityFullState once the
// identity arrives, instead of being rejected and leaving the subscriber permanently unnotified.
func Test_SubscribeToIdentityChanges_PendingIdentity(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	rdm := NewReceiverRouterDataModel("r1", closeNotify)
	sub := &recordingIdentitySubscriber{events: make(chan recordedIdentityEvent, 16)}

	rdm.SubscribeToIdentityChanges("identity1", sub, false)
	req.True(rdm.subscriptions.Has("identity1"), "unknown identity must register a pending subscription")

	select {
	case evt := <-sub.events:
		t.Fatalf("unexpected notification before the identity arrived: %v", evt.eventType)
	case <-time.After(100 * time.Millisecond):
	}

	event, model := identityCreateEvent("identity1")
	rdm.HandleIdentityEvent(1, event, model)

	select {
	case evt := <-sub.events:
		req.Equal(IdentityFullState, evt.eventType)
		req.NotNil(evt.state)
		req.NotNil(evt.state.Identity)
		req.Equal("identity1", evt.state.Identity.Id)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for IdentityFullState after the identity arrived")
	}
}

// Test_SubscribeToIdentityChanges_KnownIdentity pins the pre-existing behavior: subscribing to an
// identity already in the data model delivers IdentityFullState synchronously.
func Test_SubscribeToIdentityChanges_KnownIdentity(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	rdm := NewReceiverRouterDataModel("r1", closeNotify)

	event, model := identityCreateEvent("identity1")
	rdm.HandleIdentityEvent(1, event, model)

	sub := &recordingIdentitySubscriber{events: make(chan recordedIdentityEvent, 16)}
	rdm.SubscribeToIdentityChanges("identity1", sub, false)

	select {
	case evt := <-sub.events:
		req.Equal(IdentityFullState, evt.eventType)
		req.NotNil(evt.state.Identity)
		req.Equal("identity1", evt.state.Identity.Id)
	default:
		t.Fatal("expected a synchronous IdentityFullState for a known identity")
	}
}
