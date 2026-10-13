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

package xgress_edge

import (
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/identity"
	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// reconcileTestChannel answers only Label and IsClosed. Every other method dispatches through the nil
// embedded interface and panics.
type reconcileTestChannel struct {
	channel.Channel
	closed bool
}

func (self *reconcileTestChannel) Label() string  { return "reconcile-test" }
func (self *reconcileTestChannel) IsClosed() bool { return self.closed }

type reconcileTestSdkChannel struct {
	sdkedge.SdkChannel
	ch *reconcileTestChannel
}

func (self *reconcileTestSdkChannel) GetChannel() channel.Channel { return self.ch }

func newReconcileTestRegistry() *hostedServiceRegistry {
	return &hostedServiceRegistry{
		env:                  &createResponseTestEnv{},
		terminators:          cmap.New[*edgeTerminator](),
		establishSet:         map[string]*edgeTerminator{},
		postCreateInspectSet: map[string]*pendingPostCreateInspect{},
		events:               make(chan terminatorEvent, 4),
		triggerEvalC:         make(chan struct{}, 1),
	}
}

// newBoundTerminator builds an establishing terminator for a bind of listener id l1 on its own sdk
// connection, as processBindV2 does before checking for an existing terminator.
func newBoundTerminator(registry *hostedServiceRegistry, connId uint32, cost uint16, publicKey string) *edgeTerminator {
	sdkCh := &reconcileTestSdkChannel{ch: &reconcileTestChannel{}}
	conn := &edgeClientConn{
		ch:              sdkCh,
		apiSessionToken: &state.ApiSessionToken{ApiSession: &edge_ctrl_pb.ApiSession{IdentityId: "i1"}},
		listener: &listener{
			id:      &identity.TokenId{Token: "r1"},
			factory: &Factory{hostedServices: registry},
		},
	}
	term := &edgeTerminator{
		MsgChannel:          *sdkedge.NewEdgeMsgChannel(sdkCh, connId),
		edgeClientConn:      conn,
		terminatorId:        "t1",
		listenerId:          "l1",
		serviceSessionToken: &state.ServiceSessionToken{Claims: &common.ServiceAccessClaims{}},
		cost:                cost,
		precedence:          edge_ctrl_pb.TerminatorPrecedence_Default,
		hostData:            map[uint32][]byte{uint32(sdkedge.PublicKeyHeader): []byte(publicKey)},
		createTime:          time.Now(),
	}
	term.state.Store(xgress_common.TerminatorStateEstablishing)
	term.establishStart.Store(time.Now())
	return term
}

// takeOver runs the listener id check for replacement against a registry holding existing, whose sdk
// connection is closed so the takeover does not try to notify it.
func takeOver(t *testing.T, registry *hostedServiceRegistry, existing, replacement *edgeTerminator) {
	existing.MsgChannel.SdkChannel.(*reconcileTestSdkChannel).ch.closed = true
	registry.Put(existing)

	event := &findMatchingEvent{terminator: replacement, resultC: make(chan listenerIdCheckResult, 1)}
	event.handle(registry)
	result := <-event.resultC

	require.True(t, result.replaceExisting)
	require.Same(t, replacement, result.terminator)
	require.Same(t, existing, result.previous)
	current, _ := registry.Get("t1")
	require.Same(t, replacement, current)
}

func requireCreateValues(t *testing.T, term *edgeTerminator, cost uint16, publicKey string) {
	t.Helper()
	actualCost, _, hostData := term.takeCreateValues()
	require.Equal(t, cost, actualCost)
	require.Equal(t, publicKey, string(hostData[uint32(sdkedge.PublicKeyHeader)]))
}

// Test_findMatchingEvent_takeoverReconciles covers a bind that takes over a terminator by listener id.
// The terminator id carries over, so whatever the controller holds for it stays unless the takeover
// sends a create with the new bind's values.
func Test_findMatchingEvent_takeoverReconciles(t *testing.T) {
	t.Run("taking over an established terminator with a new key re-creates it", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		existing := newBoundTerminator(registry, 1, 10, "key1")
		existing.state.Store(xgress_common.TerminatorStateEstablished)
		replacement := newBoundTerminator(registry, 2, 10, "key2")

		takeOver(t, registry, existing, replacement)

		req.Equal(xgress_common.TerminatorStateEstablishing, replacement.state.Load())
		req.Same(replacement, registry.establishSet["t1"], "the controller still has the old key, so a create must be queued")
		requireCreateValues(t, replacement, 10, "key2")
	})

	t.Run("taking over an established terminator with a new cost re-creates it", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		existing := newBoundTerminator(registry, 1, 10, "key1")
		existing.state.Store(xgress_common.TerminatorStateEstablished)
		replacement := newBoundTerminator(registry, 2, 20, "key1")

		takeOver(t, registry, existing, replacement)

		req.Equal(xgress_common.TerminatorStateEstablishing, replacement.state.Load())
		req.Same(replacement, registry.establishSet["t1"])
		requireCreateValues(t, replacement, 20, "key1")
	})

	t.Run("taking over an established terminator with the same values sends nothing", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		existing := newBoundTerminator(registry, 1, 10, "key1")
		existing.state.Store(xgress_common.TerminatorStateEstablished)
		replacement := newBoundTerminator(registry, 2, 10, "key1")

		takeOver(t, registry, existing, replacement)

		req.Equal(xgress_common.TerminatorStateEstablished, replacement.state.Load())
		req.Empty(registry.establishSet)
		req.False(replacement.needsRecreate())
	})

	t.Run("taking over a terminator whose create has not been sent queues the replacement", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		existing := newBoundTerminator(registry, 1, 10, "key1")
		registry.establishSet["t1"] = existing
		replacement := newBoundTerminator(registry, 2, 10, "key1")

		takeOver(t, registry, existing, replacement)

		req.Equal(xgress_common.TerminatorStateEstablishing, replacement.state.Load())
		req.Same(replacement, registry.establishSet["t1"], "the queued create was for the terminator taken over")
	})

	// The create in flight was built from the adopted terminator's values. Its success confirms the
	// terminator id, not the new bind's values, so another create has to follow it.
	t.Run("taking over during a create sends another once it succeeds", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		existing := newBoundTerminator(registry, 1, 10, "key1")
		existing.operationActive.Store(true)
		existing.lastAttempt = time.Now()
		existing.noteCreateRequestSent("ctrl1", 7)
		replacement := newBoundTerminator(registry, 2, 10, "key2")

		takeOver(t, registry, existing, replacement)

		req.Empty(registry.establishSet, "a create is in flight, so nothing more is sent until it is answered")
		req.True(replacement.hasOutstandingCreate())

		registry.HandleCreateTerminatorResponse(createTerminatorResponse("t1", edge_ctrl_pb.CreateTerminatorResult_Success, 7), &stubCtrlCh{id: "ctrl1"})
		(<-registry.events).handle(registry)

		req.Equal(xgress_common.TerminatorStateEstablishing, replacement.state.Load())
		req.Same(replacement, registry.establishSet["t1"])
		req.False(replacement.operationActive.Load())
		requireCreateValues(t, replacement, 10, "key2")

		replacement.noteCreateRequestSent("ctrl1", 8)
		registry.HandleCreateTerminatorResponse(createTerminatorResponse("t1", edge_ctrl_pb.CreateTerminatorResult_Success, 8), &stubCtrlCh{id: "ctrl1"})
		(<-registry.events).handle(registry)

		req.Equal(xgress_common.TerminatorStateEstablished, replacement.state.Load())
	})
}

func Test_edgeTerminator_replaceRecreateNeeded(t *testing.T) {
	newPair := func() (*edgeTerminator, *edgeTerminator) {
		other := &edgeTerminator{terminatorId: "t1", cost: 10, hostData: map[uint32][]byte{1: []byte("key1")}}
		replacement := &edgeTerminator{cost: 10, hostData: map[uint32][]byte{1: []byte("key1")}}
		return other, replacement
	}

	t.Run("same values", func(t *testing.T) {
		other, replacement := newPair()
		replacement.replace(other)
		require.False(t, replacement.needsRecreate())
	})

	t.Run("no peer data on either side", func(t *testing.T) {
		other, replacement := newPair()
		other.hostData = nil
		replacement.hostData = map[uint32][]byte{}
		replacement.replace(other)
		require.False(t, replacement.needsRecreate())
	})

	t.Run("different cost", func(t *testing.T) {
		other, replacement := newPair()
		replacement.cost = 20
		replacement.replace(other)
		require.True(t, replacement.needsRecreate())
	})

	t.Run("different precedence", func(t *testing.T) {
		other, replacement := newPair()
		replacement.precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
		replacement.replace(other)
		require.True(t, replacement.needsRecreate())
	})

	t.Run("different peer data", func(t *testing.T) {
		other, replacement := newPair()
		replacement.hostData = map[uint32][]byte{1: []byte("key2")}
		replacement.replace(other)
		require.True(t, replacement.needsRecreate())
	})

	t.Run("adopted terminator already out of date", func(t *testing.T) {
		other, replacement := newPair()
		other.recreateNeeded = true
		replacement.replace(other)
		require.True(t, replacement.needsRecreate())
	})
}

// Test_edgeTerminator_updateCostAndPrecedence covers recording an sdk's update bind so that every later
// create for the terminator carries it, and deciding whether the update itself can go to the controller.
func Test_edgeTerminator_updateCostAndPrecedence(t *testing.T) {
	cost := func(v uint16) *uint16 { return &v }
	precedence := func(v edge_ctrl_pb.TerminatorPrecedence) *edge_ctrl_pb.TerminatorPrecedence { return &v }

	t.Run("an established terminator takes the update and records it for re-creates", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{cost: 10}
		term.state.Store(xgress_common.TerminatorStateEstablished)

		req.True(term.updateCostAndPrecedence(cost(20), precedence(edge_ctrl_pb.TerminatorPrecedence_Failed)))
		req.False(term.needsRecreate())

		actualCost, actualPrecedence, _ := term.takeCreateValues()
		req.Equal(uint16(20), actualCost)
		req.Equal(edge_ctrl_pb.TerminatorPrecedence_Failed, actualPrecedence)
	})

	t.Run("an absent value is left as it is", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{cost: 10, precedence: edge_ctrl_pb.TerminatorPrecedence_Required}
		term.state.Store(xgress_common.TerminatorStateEstablished)

		req.True(term.updateCostAndPrecedence(cost(20), nil))
		actualCost, actualPrecedence := term.getCostAndPrecedence()
		req.Equal(uint16(20), actualCost)
		req.Equal(edge_ctrl_pb.TerminatorPrecedence_Required, actualPrecedence)
	})

	t.Run("an establishing terminator gets the change from its create", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{cost: 10}
		term.state.Store(xgress_common.TerminatorStateEstablishing)

		req.False(term.updateCostAndPrecedence(cost(20), nil), "the controller may not have the terminator yet")
		req.True(term.needsRecreate())
	})

	t.Run("an established terminator with a create outstanding gets the change from a later create", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{cost: 10}
		term.state.Store(xgress_common.TerminatorStateEstablished)
		term.noteCreateRequestSent("ctrl1", 7)

		req.False(term.updateCostAndPrecedence(cost(20), nil), "the outstanding create would revert the update")
		req.True(term.needsRecreate())
	})

	// An sdk re-sends its values on every bind confirmation, which a re-create produces, so treating
	// an unchanged value as a change would re-create without end.
	t.Run("an unchanged value does not call for a create", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{cost: 10}
		term.state.Store(xgress_common.TerminatorStateEstablishing)

		req.False(term.updateCostAndPrecedence(cost(10), precedence(edge_ctrl_pb.TerminatorPrecedence_Default)))
		req.False(term.needsRecreate())
	})

	t.Run("an update during a create is carried by the create that follows it", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		term := newBoundTerminator(registry, 1, 10, "key1")
		term.operationActive.Store(true)
		term.lastAttempt = time.Now()
		term.noteCreateRequestSent("ctrl1", 7)
		registry.Put(term)

		req.False(term.updateCostAndPrecedence(cost(20), nil))

		registry.HandleCreateTerminatorResponse(createTerminatorResponse("t1", edge_ctrl_pb.CreateTerminatorResult_Success, 7), &stubCtrlCh{id: "ctrl1"})
		(<-registry.events).handle(registry)

		req.Equal(xgress_common.TerminatorStateEstablishing, term.state.Load())
		req.Same(term, registry.establishSet["t1"])
		requireCreateValues(t, term, 20, "key1")
	})
}

func Test_recreateTerminatorEvent_handle(t *testing.T) {
	t.Run("an established terminator is re-created", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		term := newBoundTerminator(registry, 1, 10, "key1")
		term.state.Store(xgress_common.TerminatorStateEstablished)
		registry.Put(term)

		term.markRecreateNeeded()
		(&recreateTerminatorEvent{terminator: term}).handle(registry)

		req.Equal(xgress_common.TerminatorStateEstablishing, term.state.Load())
		req.Same(term, registry.establishSet["t1"])
	})

	t.Run("a terminator that has been replaced is left alone", func(t *testing.T) {
		req := require.New(t)

		registry := newReconcileTestRegistry()
		term := newBoundTerminator(registry, 1, 10, "key1")
		term.state.Store(xgress_common.TerminatorStateEstablished)
		registry.Put(newBoundTerminator(registry, 2, 10, "key1"))

		term.markRecreateNeeded()
		(&recreateTerminatorEvent{terminator: term}).handle(registry)

		req.Equal(xgress_common.TerminatorStateEstablished, term.state.Load())
		req.Empty(registry.establishSet)
	})
}
