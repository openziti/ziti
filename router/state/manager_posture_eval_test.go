package state

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common"
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
	mgr.routerDataModel.Store(common.NewBareRouterDataModel(""))
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
