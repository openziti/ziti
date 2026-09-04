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

package state

import (
	"errors"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/posture"
	"github.com/stretchr/testify/require"
)

const (
	passingMac = "001122334455"
	failingMac = "66778899aabb"
)

func Test_EvaluatePostureChange(t *testing.T) {
	t.Run("an evicted api session is not evaluated", func(t *testing.T) {
		req := require.New(t)

		// arrange: the api session has no posture data
		mgr, tracker := newPostureEvalTestManager(nil)

		// act
		mgr.evaluatePostureChange("identity-1", "evicted-session")

		// assert
		req.Empty(tracker.lookups, "an evicted session has nothing to re-evaluate and no connections to look for")
	})

	t.Run("an api session with posture data has its connections evaluated", func(t *testing.T) {
		req := require.New(t)

		// arrange
		mgr, tracker := newPostureEvalTestManager(nil)
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(passingMac))

		// act
		mgr.evaluatePostureChange("identity-1", "session-1")

		// assert
		req.Len(tracker.lookups, 1, "the evaluation must look up the identity's connections")
	})
}

// Test_EvaluatePostureChange_JudgesAgainstPostureReadAfterCollecting covers an evaluation that is
// overtaken while it runs: a newer, passing posture report lands, and a dial or bind is admitted
// under it, before the evaluation reaches that circuit or terminator. Each "admitted under newer
// passing posture" case lands the newer report while the connection is being iterated, so an
// evaluation judging against posture read before iterating closes what current posture allows.
func Test_EvaluatePostureChange_JudgesAgainstPostureReadAfterCollecting(t *testing.T) {
	t.Run("a dial circuit is closed when current posture fails", func(t *testing.T) {
		req := require.New(t)

		// arrange
		mgr, tracker := newPostureEvalTestManager(nil)
		mgr.routerDataModel.Store(newPostureEvalTestRdm())
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(failingMac))
		circuit := &postureEvalTestCircuit{serviceId: "svc-1"}
		conn := newPostureEvalTestConn(mgr.postureCache, "identity-1", "session-1")
		conn.circuits = []*postureEvalTestCircuit{circuit}
		tracker.channels = []channel.Channel{&postureEvalTestChannel{userData: conn}}

		// act
		mgr.evaluatePostureChange("identity-1", "session-1")

		// assert
		req.True(circuit.closed, "the MAC check fails, so the dial circuit loses access")
	})

	t.Run("a dial circuit admitted under newer passing posture is kept", func(t *testing.T) {
		req := require.New(t)

		// arrange
		mgr, tracker := newPostureEvalTestManager(nil)
		mgr.routerDataModel.Store(newPostureEvalTestRdm())
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(failingMac))
		circuit := &postureEvalTestCircuit{serviceId: "svc-1"}
		conn := newPostureEvalTestConn(mgr.postureCache, "identity-1", "session-1")
		conn.circuits = []*postureEvalTestCircuit{circuit}
		conn.newerPosture = macPostureResponses(passingMac)
		tracker.channels = []channel.Channel{&postureEvalTestChannel{userData: conn}}

		// act
		mgr.evaluatePostureChange("identity-1", "session-1")

		// assert
		req.False(circuit.closed, "the circuit was admitted under passing posture, which is still current")
	})

	t.Run("a terminator is closed when current posture fails", func(t *testing.T) {
		req := require.New(t)

		// arrange
		mgr, tracker := newPostureEvalTestManager(nil)
		mgr.routerDataModel.Store(newPostureEvalTestRdm())
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(failingMac))
		terminator := &postureEvalTestTerminator{serviceId: "svc-1"}
		conn := newPostureEvalTestConn(mgr.postureCache, "identity-1", "session-1")
		conn.terminators = []*postureEvalTestTerminator{terminator}
		tracker.channels = []channel.Channel{&postureEvalTestChannel{userData: conn}}

		// act
		mgr.evaluatePostureChange("identity-1", "session-1")

		// assert
		req.True(terminator.closed, "the MAC check fails, so the terminator loses bind access")
	})

	t.Run("a terminator admitted under newer passing posture is kept", func(t *testing.T) {
		req := require.New(t)

		// arrange
		mgr, tracker := newPostureEvalTestManager(nil)
		mgr.routerDataModel.Store(newPostureEvalTestRdm())
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(failingMac))
		terminator := &postureEvalTestTerminator{serviceId: "svc-1"}
		conn := newPostureEvalTestConn(mgr.postureCache, "identity-1", "session-1")
		conn.terminators = []*postureEvalTestTerminator{terminator}
		conn.newerPosture = macPostureResponses(passingMac)
		tracker.channels = []channel.Channel{&postureEvalTestChannel{userData: conn}}

		// act
		mgr.evaluatePostureChange("identity-1", "session-1")

		// assert
		req.False(terminator.closed, "the terminator was admitted under passing posture, which is still current")
	})
}

func Test_OnPostureDataUpdate(t *testing.T) {
	// Dropping an evaluation that can't be queued would leave access in place that current posture
	// no longer permits.
	t.Run("an evaluation the pool rejects runs inline", func(t *testing.T) {
		req := require.New(t)

		// arrange
		mgr, tracker := newPostureEvalTestManager(rejectingPool{})
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(passingMac))

		// act
		mgr.onPostureDataUpdate(&posture.InstanceData{IdentityId: "identity-1", ApiSessionId: "session-1"})

		// assert
		req.Len(tracker.lookups, 1, "a rejected evaluation must still run, before the update returns")
	})

	t.Run("an evaluation is scheduled onto the pool", func(t *testing.T) {
		req := require.New(t)

		// arrange
		pool, err := goroutines.NewPool(goroutines.PoolConfig{
			QueueSize:   16,
			MaxWorkers:  1,
			IdleTime:    time.Second,
			CloseNotify: make(chan struct{}),
		})
		req.NoError(err)
		t.Cleanup(pool.Shutdown)
		mgr, tracker := newPostureEvalTestManager(pool)
		mgr.postureCache.AddResponses("identity-1", "session-1", macPostureResponses(passingMac))

		// act
		mgr.onPostureDataUpdate(&posture.InstanceData{IdentityId: "identity-1", ApiSessionId: "session-1"})

		// assert
		select {
		case identityId := <-tracker.lookups:
			req.Equal("identity-1", identityId)
		case <-time.After(5 * time.Second):
			req.Fail("the scheduled evaluation never ran")
		}
	})
}

// rejectingPool is a goroutines.Pool that accepts no work, standing in for a pool whose workers
// and queue are saturated. Shutting a real pool down is not usable here: QueueOrError selects over
// the queue and the close signal together, so a shut-down pool still accepts work about half the
// time.
type rejectingPool struct{}

func (rejectingPool) Queue(func()) error                           { return errors.New("rejected") }
func (rejectingPool) QueueWithTimeout(func(), time.Duration) error { return errors.New("rejected") }
func (rejectingPool) QueueOrError(func()) error                    { return errors.New("rejected") }
func (rejectingPool) GetWorkerCount() uint32                       { return 0 }
func (rejectingPool) GetQueueSize() uint32                         { return 0 }
func (rejectingPool) GetBusyWorkers() uint32                       { return 0 }
func (rejectingPool) GetOutstanding() uint32                       { return 0 }
func (rejectingPool) Shutdown()                                    {}
func (rejectingPool) ShutdownAndWait(time.Duration) error          { return nil }
func (rejectingPool) AwaitIdle(time.Duration) error                { return nil }

// stubConnectionTracker returns channels for any identity, and sends the identity id on lookups
// each time it is asked for them. Posture evaluation asks once it has found the session's posture
// data.
type stubConnectionTracker struct {
	lookups  chan string
	channels []channel.Channel
}

func (self *stubConnectionTracker) GetChannels() map[string][]channel.Channel {
	return nil
}

func (self *stubConnectionTracker) GetChannelsByIdentityId(identityId string) []channel.Channel {
	self.lookups <- identityId
	return self.channels
}

// newPostureEvalTestManager returns a manager with an empty posture cache and router data model,
// which evaluates posture changes on pool. pool may be nil when the test calls
// evaluatePostureChange directly.
func newPostureEvalTestManager(pool goroutines.Pool) (*ManagerImpl, *stubConnectionTracker) {
	tracker := &stubConnectionTracker{lookups: make(chan string, 16)}
	mgr := &ManagerImpl{
		postureCache:      posture.NewCache(nil),
		postureEvalPool:   pool,
		connectionTracker: tracker,
	}
	mgr.routerDataModel.Store(common.NewBareRouterDataModel(""))
	return mgr, tracker
}

// postureEvalTestChannel is a channel.Channel that carries only user data, which is all posture
// evaluation reads from a channel. Any other method panics.
type postureEvalTestChannel struct {
	channel.Channel
	userData interface{}
}

func (self *postureEvalTestChannel) GetUserData() interface{} {
	return self.userData
}

// postureEvalTestConn is a ConnProvider for one api session. When newerPosture is set, iterating
// its circuits or terminators first applies newerPosture to the posture cache, standing in for a
// newer report, and the dial or bind admitted under it, landing while an evaluation is in flight.
type postureEvalTestConn struct {
	cache        *posture.Cache
	token        *ApiSessionToken
	circuits     []*postureEvalTestCircuit
	terminators  []*postureEvalTestTerminator
	newerPosture *edge_client_pb.PostureResponses
}

// newPostureEvalTestConn returns a connection for the given api session with no circuits or
// terminators, reading and writing posture through cache.
func newPostureEvalTestConn(cache *posture.Cache, identityId, apiSessionId string) *postureEvalTestConn {
	return &postureEvalTestConn{
		cache: cache,
		token: &ApiSessionToken{ApiSession: &edge_ctrl_pb.ApiSession{Id: apiSessionId, IdentityId: identityId}},
	}
}

func (self *postureEvalTestConn) GetApiSessionToken() *ApiSessionToken {
	return self.token
}

func (self *postureEvalTestConn) GetConnIdToSinks() map[uint32]edge.MsgSink[*ConnState] {
	return nil
}

func (self *postureEvalTestConn) CloseConn(uint32, string) error {
	return nil
}

func (self *postureEvalTestConn) IterateEdgeCircuits(f func(EdgeCircuit)) {
	self.landNewerPosture()
	for _, c := range self.circuits {
		f(c)
	}
}

func (self *postureEvalTestConn) IterateBindTerminators(f func(BindTerminator)) {
	self.landNewerPosture()
	for _, t := range self.terminators {
		f(t)
	}
}

// landNewerPosture applies newerPosture to the posture cache, if it is set.
func (self *postureEvalTestConn) landNewerPosture() {
	if self.newerPosture != nil {
		self.cache.AddResponses(self.token.IdentityId, self.token.Id, self.newerPosture)
	}
}

// postureEvalTestCircuit is a dial-side EdgeCircuit that records whether it was closed.
type postureEvalTestCircuit struct {
	serviceId string
	closed    bool
}

func (self *postureEvalTestCircuit) GetCircuitId() string      { return "circuit-1" }
func (self *postureEvalTestCircuit) GetServiceId() string      { return self.serviceId }
func (self *postureEvalTestCircuit) IsHostSide() bool          { return false }
func (self *postureEvalTestCircuit) CloseForAccessLoss(string) { self.closed = true }

// postureEvalTestTerminator is a BindTerminator that records whether it was closed.
type postureEvalTestTerminator struct {
	serviceId string
	closed    bool
}

func (self *postureEvalTestTerminator) GetTerminatorId() string { return "terminator-1" }
func (self *postureEvalTestTerminator) GetServiceId() string    { return self.serviceId }
func (self *postureEvalTestTerminator) CloseForBindAccessLoss() { self.closed = true }

// macPostureResponses returns a posture report of a single MAC address.
func macPostureResponses(mac string) *edge_client_pb.PostureResponses {
	return &edge_client_pb.PostureResponses{
		Responses: []*edge_client_pb.PostureResponse{
			{
				Type: &edge_client_pb.PostureResponse_Macs_{
					Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{mac}},
				},
			},
		},
	}
}

// newPostureEvalTestRdm returns a router data model in which identity-1 may dial and bind svc-1,
// both through policies requiring the MAC check chk-1, which passes only for passingMac.
func newPostureEvalTestRdm() *common.RouterDataModel {
	rdm := common.NewBareRouterDataModel("r1")
	rdm.HandleIdentityEvent(1,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Identity{Identity: &edge_ctrl_pb.DataState_Identity{Id: "identity-1", Name: "identity-1"}})
	rdm.HandleServiceEvent(2,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Service{Service: &edge_ctrl_pb.DataState_Service{Id: "svc-1", Name: "svc-1"}})
	rdm.HandlePostureCheckEvent(3,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_PostureCheck{PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:     "chk-1",
			Name:   "mac-check",
			TypeId: "MAC",
			Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
				Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{MacAddresses: []string{passingMac}},
			},
		}})

	index := uint64(3)
	for _, policyType := range []edge_ctrl_pb.PolicyType{edge_ctrl_pb.PolicyType_DialPolicy, edge_ctrl_pb.PolicyType_BindPolicy} {
		policyId := "sp-" + policyType.String()
		index++
		rdm.HandleServicePolicyEvent(index,
			&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
			&edge_ctrl_pb.DataState_Event_ServicePolicy{ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
				Id:         policyId,
				Name:       policyId,
				PolicyType: policyType,
			}})
		related := map[edge_ctrl_pb.ServicePolicyRelatedEntityType]string{
			edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity:     "identity-1",
			edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService:      "svc-1",
			edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck: "chk-1",
		}
		for entityType, entityId := range related {
			index++
			rdm.HandleServicePolicyChange(index, &edge_ctrl_pb.DataState_ServicePolicyChange{
				PolicyId:          policyId,
				RelatedEntityIds:  []string{entityId},
				RelatedEntityType: entityType,
				Add:               true,
			})
		}
	}
	rdm.SetCurrentIndex(index)
	return rdm
}
