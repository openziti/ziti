package posture

import (
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/stretchr/testify/require"
)

func seedInstance(cache *Cache, apiSessionId string) {
	cache.AddResponses("identity-1", apiSessionId, &edge_client_pb.PostureResponses{
		Responses: []*edge_client_pb.PostureResponse{
			{
				Type: &edge_client_pb.PostureResponse_Domain_{
					Domain: &edge_client_pb.PostureResponse_Domain{Name: "example.com"},
				},
			},
		},
	})
}

func connectedSet(ids ...string) func(string) bool {
	connected := map[string]struct{}{}
	for _, id := range ids {
		connected[id] = struct{}{}
	}
	return func(apiSessionId string) bool {
		_, ok := connected[apiSessionId]
		return ok
	}
}

func Test_EvictInactive_KeepsConnectedSessions(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "connected")

	// no disconnect recorded, so there is no retention window to expire
	req.Equal(0, cache.EvictInactive(0))
	req.NotNil(cache.GetInstance("connected"))
}

func Test_EvictInactive_KeepsSessionWithinRetention(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "gone")

	cache.MarkDisconnected("gone")

	req.Equal(0, cache.EvictInactive(time.Minute))
	req.NotNil(cache.GetInstance("gone"), "posture data must survive a brief disconnect")
}

func Test_EvictInactive_EvictsAfterRetention(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "gone")

	cache.MarkDisconnected("gone")

	req.Equal(1, cache.EvictInactive(0))
	req.Nil(cache.GetInstance("gone"))
}

func Test_EvictInactive_EvictsOnlyDisconnectedSessions(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "keep")
	seedInstance(cache, "drop-1")
	seedInstance(cache, "drop-2")

	cache.MarkDisconnected("drop-1")
	cache.MarkDisconnected("drop-2")

	req.Equal(2, cache.EvictInactive(0))
	req.NotNil(cache.GetInstance("keep"))
	req.Nil(cache.GetInstance("drop-1"))
	req.Nil(cache.GetInstance("drop-2"))
}

func Test_MarkConnected_CancelsPendingEviction(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "reconnected")

	cache.MarkDisconnected("reconnected")
	cache.MarkConnected("reconnected")

	req.Equal(0, cache.EvictInactive(0))
	req.NotNil(cache.GetInstance("reconnected"), "a reconnect must cancel a pending eviction")
}

func Test_MarkDisconnected_KeepsEarliestDisconnect(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "gone")

	first := time.Now().Add(-time.Hour)
	cache.markDisconnectedAt("gone", first)
	cache.markDisconnectedAt("gone", time.Now())

	recorded, disconnected := cache.GetInstance("gone").getDisconnected()
	req.True(disconnected)
	req.True(recorded.Equal(first), "a later disconnect must not extend the retention window")
}

func Test_MarkDisconnected_UnknownSessionIsNoop(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)

	cache.MarkDisconnected("never-seen")
	cache.MarkConnected("never-seen")

	req.Nil(cache.GetInstance("never-seen"), "notifications must not create posture instances")
}

func Test_PostureActivityCancelsPendingEviction(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "busy")

	cache.MarkDisconnected("busy")

	// posture data arriving means the session is in use, whatever the last notification said
	seedInstance(cache, "busy")

	req.Equal(0, cache.EvictInactive(0))
	req.NotNil(cache.GetInstance("busy"))
}

func Test_ReconcileDisconnected_RecordsMissedDisconnect(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "drifted")

	// the disconnect notification never arrived, so the session looks connected
	req.Equal(0, cache.EvictInactive(0))

	// reconciling against the connection tracker records the disconnect, making it evictable
	cache.ReconcileDisconnected(connectedSet())
	req.Equal(1, cache.EvictInactive(0))
	req.Nil(cache.GetInstance("drifted"))
}

func Test_ReconcileDisconnected_ClearsMissedReconnect(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "drifted")

	// the connect notification never arrived, so the session still looks disconnected
	cache.MarkDisconnected("drifted")

	cache.ReconcileDisconnected(connectedSet("drifted"))
	req.Equal(0, cache.EvictInactive(0))
	req.NotNil(cache.GetInstance("drifted"), "reconciling must not evict a connected session")
}

func Test_ReconcileDisconnected_NeverEvicts(t *testing.T) {
	req := require.New(t)
	cache := NewCache(nil)
	seedInstance(cache, "gone")

	cache.markDisconnectedAt("gone", time.Now().Add(-time.Hour))

	cache.ReconcileDisconnected(connectedSet())
	req.NotNil(cache.GetInstance("gone"), "reconciling corrects state; only EvictInactive removes")
}

func Test_DisconnectedAtIs_IdentifiesTheJudgedIncarnation(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	judged := instance.setDisconnected(time.Now())
	req.True(instance.disconnectedAtIs(judged), "the time just recorded is the judged incarnation")

	// a reconnect clears the recorded disconnect, so the verdict no longer applies
	instance.clearDisconnected()
	req.False(instance.disconnectedAtIs(judged))

	// a later disconnect is a different incarnation than the one judged
	relapsed := instance.setDisconnected(judged.Add(time.Minute))
	req.False(instance.disconnectedAtIs(judged))
	req.True(instance.disconnectedAtIs(relapsed))
}

// Test_MarkConnected_IsAtomicWithRemoval races a reconnect against the removal decision of a
// sweep that has already judged the same session evictable, and asserts the outcome is always one
// of the two consistent ones: either the reconnect won and the session is still cached, or the
// eviction won and the instance it removed still carries the disconnect time it judged. The
// inconsistent outcome, an entry removed while the reconnect cleared the disconnect on the
// detached instance, is what a lookup-then-update MarkConnected produces.
//
// Note this is a probabilistic guard, not proof: the window it targets is a few instructions
// wide. Atomicity rests on MarkConnected and the removal predicate sharing one critical section.
func Test_MarkConnected_IsAtomicWithRemoval(t *testing.T) {
	req := require.New(t)

	for range 2000 {
		cache := NewCache(nil)
		seedInstance(cache, "racing")
		instance := cache.GetInstance("racing")
		req.NotNil(instance)

		cache.markDisconnectedAt("racing", time.Now().Add(-time.Hour))

		var wg sync.WaitGroup
		start := make(chan struct{})
		evicted := 0

		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			evicted = cache.EvictInactive(0)
		}()
		go func() {
			defer wg.Done()
			<-start
			cache.MarkConnected("racing")
		}()
		close(start)
		wg.Wait()

		if evicted > 0 {
			req.Nil(cache.GetInstance("racing"))
			_, stillDisconnected := instance.getDisconnected()
			req.True(stillDisconnected,
				"eviction won, so the reconnect must not have cleared the disconnect of the instance it removed")
		} else {
			req.NotNil(cache.GetInstance("racing"), "the reconnect won, so the session must still be cached")
		}
	}
}
