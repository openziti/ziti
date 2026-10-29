package state

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/posture"
	"github.com/stretchr/testify/require"
)

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

// stubConnectionTracker records whether the posture evaluation reached the point of looking for
// the identity's connections, which is the first thing it does once it has posture data.
type stubConnectionTracker struct {
	lookups atomic.Int32
}

func (self *stubConnectionTracker) GetChannels() map[string][]channel.Channel {
	return nil
}

func (self *stubConnectionTracker) GetChannelsByIdentityId(string) []channel.Channel {
	self.lookups.Add(1)
	return nil
}

func newPostureEvalTestManager(t *testing.T, pool goroutines.Pool) (*ManagerImpl, *stubConnectionTracker) {
	tracker := &stubConnectionTracker{}
	mgr := &ManagerImpl{
		postureCache:      posture.NewCache(nil),
		postureEvalPool:   pool,
		connectionTracker: tracker,
	}
	mgr.routerDataModel.Store(common.NewBareRouterDataModel())
	t.Cleanup(func() {
		if pool != nil {
			pool.Shutdown()
		}
	})
	return mgr, tracker
}

func newTestPool(t *testing.T, queueSize uint32) goroutines.Pool {
	pool, err := goroutines.NewPool(goroutines.PoolConfig{
		QueueSize:    queueSize,
		MinWorkers:   0,
		MaxWorkers:   1,
		IdleTime:     time.Second,
		CloseNotify:  make(chan struct{}),
		PanicHandler: func(interface{}) {},
	})
	require.NoError(t, err)
	return pool
}

func seedPosture(mgr *ManagerImpl, apiSessionId string) {
	mgr.postureCache.AddResponses("identity-1", apiSessionId, &edge_client_pb.PostureResponses{
		Responses: []*edge_client_pb.PostureResponse{
			{
				Type: &edge_client_pb.PostureResponse_Domain_{
					Domain: &edge_client_pb.PostureResponse_Domain{Name: "example.com"},
				},
			},
		},
	})
}

func Test_EvaluatePostureChange_NoPostureDataIsANoop(t *testing.T) {
	req := require.New(t)
	mgr, tracker := newPostureEvalTestManager(t, newTestPool(t, 16))

	// the api session's posture data has been evicted; there is nothing to re-evaluate and no
	// connections to look for
	mgr.evaluatePostureChange("identity-1", "evicted-session")

	req.Equal(int32(0), tracker.lookups.Load())
}

func Test_EvaluatePostureChange_ReadsCurrentPostureData(t *testing.T) {
	req := require.New(t)
	mgr, tracker := newPostureEvalTestManager(t, newTestPool(t, 16))

	seedPosture(mgr, "session-1")
	mgr.evaluatePostureChange("identity-1", "session-1")

	req.Equal(int32(1), tracker.lookups.Load(), "posture data present, so the evaluation proceeds")
}

// Test_OnPostureDataUpdate_RunsInlineWhenPoolRejects covers the fail-safe: an evaluation that
// cannot be queued must still run, since dropping it would leave access in place that current
// posture no longer permits.
func Test_OnPostureDataUpdate_RunsInlineWhenPoolRejects(t *testing.T) {
	req := require.New(t)

	mgr, tracker := newPostureEvalTestManager(t, rejectingPool{})
	seedPosture(mgr, "session-1")

	mgr.onPostureDataUpdate(&posture.InstanceData{IdentityId: "identity-1", ApiSessionId: "session-1"})

	req.Equal(int32(1), tracker.lookups.Load(), "a rejected evaluation must still run inline")
}

func Test_OnPostureDataUpdate_SchedulesOntoThePool(t *testing.T) {
	req := require.New(t)
	mgr, tracker := newPostureEvalTestManager(t, newTestPool(t, 16))
	seedPosture(mgr, "session-1")

	mgr.onPostureDataUpdate(&posture.InstanceData{IdentityId: "identity-1", ApiSessionId: "session-1"})

	req.Eventually(func() bool {
		return tracker.lookups.Load() == 1
	}, 5*time.Second, time.Millisecond, "the scheduled evaluation must run")
}

// postureCheckedRdm builds the smallest data model in which an access check actually evaluates
// posture: an identity, a service, and a dial policy joining them that carries a MAC check.
// Without the policy the check short-circuits before it reads any posture data.
func postureCheckedRdm(req *require.Assertions) *common.RouterDataModel {
	rdm := common.NewBareRouterDataModel()

	rdm.HandleIdentityEvent(1, &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Identity{Identity: &edge_ctrl_pb.DataState_Identity{Id: "identity-1", Name: "identity-1"}})

	rdm.HandleServiceEvent(2, &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Service{Service: &edge_ctrl_pb.DataState_Service{Id: "service-1", Name: "service-1"}})

	// A domain check, so evaluation reads the same field the posture responses below write. A check
	// on some other field would never touch the memory being mutated.
	rdm.HandlePostureCheckEvent(3, &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_PostureCheck{PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id: "check-1", Name: "domain-check", TypeId: "DOMAIN",
			Subtype: &edge_ctrl_pb.DataState_PostureCheck_Domains_{
				Domains: &edge_ctrl_pb.DataState_PostureCheck_Domains{Domains: []string{"example.com"}},
			},
		}})

	rdm.HandleServicePolicyEvent(4, &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_ServicePolicy{ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
			Id: "policy-1", Name: "policy-1", PolicyType: edge_ctrl_pb.PolicyType_DialPolicy,
		}})

	link := func(index uint64, entityType edge_ctrl_pb.ServicePolicyRelatedEntityType, id string) {
		rdm.Handle(index, &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          "policy-1",
					RelatedEntityIds:  []string{id},
					RelatedEntityType: entityType,
					Add:               true,
				},
			},
		})
	}

	link(5, edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity, "identity-1")
	link(6, edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService, "service-1")
	link(7, edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck, "check-1")

	rdm.SetCurrentIndex(8)

	policies, err := rdm.GetServiceAccessPolicies("identity-1", "service-1", edge_ctrl_pb.PolicyType_DialPolicy)
	req.NoError(err, "the model must actually resolve a policy, or the check never reads posture data")
	req.NotEmpty(policies.PostureChecks, "the policy must carry a posture check")

	return rdm
}

// Test_HasAccess_DoesNotRacePostureResponses covers the access-check path reading posture data
// while responses are applied to it. Handing the evaluator a pointer into the live instance is an
// unsynchronized read of fields the response path writes under the instance lock. Run under -race.
func Test_HasAccess_DoesNotRacePostureResponses(t *testing.T) {
	req := require.New(t)
	mgr, _ := newPostureEvalTestManager(t, newTestPool(t, 16))
	mgr.routerDataModel.Store(postureCheckedRdm(req))

	const apiSessionId = "session-1"

	// Alternating values, because Apply skips a response that reports what it already holds: a
	// writer that never writes cannot race a reader.
	reportDomain := func(domain string) {
		mgr.postureCache.AddResponses("identity-1", apiSessionId, &edge_client_pb.PostureResponses{
			Responses: []*edge_client_pb.PostureResponse{
				{
					Type: &edge_client_pb.PostureResponse_Domain_{
						Domain: &edge_client_pb.PostureResponse_Domain{Name: domain},
					},
				},
			},
		})
	}
	reportDomain("example.com")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range 500 {
			if i%2 == 0 {
				reportDomain("example.com")
			} else {
				reportDomain("other.example.com")
			}
		}
	}()

	go func() {
		defer wg.Done()
		for range 500 {
			_, _ = mgr.HasAccess("identity-1", apiSessionId, "service-1", edge_ctrl_pb.PolicyType_DialPolicy)
		}
	}()

	wg.Wait()
	req.NotNil(mgr.postureCache.GetInstance(apiSessionId))
}
