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

package sync_strats

import (
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// stubCtrlChannel is a no-op ctrlchan.CtrlChannel. It carries an id field so it isn't
// zero-sized: distinct instances must have distinct addresses (Go may co-locate zero-size
// allocations), since GetOrCreate compares control channels by identity.
type stubCtrlChannel struct{ id string }

func (*stubCtrlChannel) InitChannel(channel.Channel)           {}
func (s *stubCtrlChannel) PeerId() string                      { return s.id }
func (*stubCtrlChannel) GetChannel() channel.Channel           { return nil }
func (*stubCtrlChannel) GetDefaultSender() channel.Sender      { return nil }
func (*stubCtrlChannel) GetHighPrioritySender() channel.Sender { return nil }
func (*stubCtrlChannel) GetLowPrioritySender() channel.Sender  { return nil }
func (*stubCtrlChannel) IsConnected() bool                     { return true }
func (*stubCtrlChannel) Close() error                          { return nil }
func (*stubCtrlChannel) IsClosed() bool                        { return false }

func newRouterTxMap() *routerTxMap {
	return &routerTxMap{internalMap: cmap.New[*RouterSender]()}
}

// newStubSender builds a RouterSender without starting its run loop, wired up enough for
// GetOrCreate/RangeEdge and Stop() (running + closeNotify).
func newStubSender(id string, ctrl ctrlchan.CtrlChannel, isEdge bool) *RouterSender {
	rtx := &RouterSender{
		Router:      &model.Router{BaseEntity: models.BaseEntity{Id: id}, Control: ctrl},
		closeNotify: make(chan struct{}),
		isEdge:      isEdge,
	}
	rtx.running.Store(true)
	return rtx
}

func isStopped(rtx *RouterSender) bool {
	select {
	case <-rtx.closeNotify:
		return true
	default:
		return false
	}
}

func TestRouterTxMap_GetOrCreate(t *testing.T) {
	t.Run("creates when absent", func(t *testing.T) {
		req := require.New(t)
		m := newRouterTxMap()
		ch := &stubCtrlChannel{}

		var factoryCalls int
		want := newStubSender("r1", ch, false)
		got, created := m.GetOrCreate("r1", ch, func() *RouterSender {
			factoryCalls++
			return want
		})

		req.True(created)
		req.Equal(1, factoryCalls)
		req.Same(want, got)
		req.Same(want, m.Get("r1"))
	})

	t.Run("reuses existing sender when the control channel matches", func(t *testing.T) {
		req := require.New(t)
		m := newRouterTxMap()
		ch := &stubCtrlChannel{}
		existing := newStubSender("r1", ch, true)
		m.Add("r1", existing)

		got, created := m.GetOrCreate("r1", ch, func() *RouterSender {
			t.Fatal("factory must not be called when a matching sender exists")
			return nil
		})

		req.False(created)
		req.Same(existing, got)
		req.False(isStopped(existing))
	})

	t.Run("replaces and stops a sender bound to a stale channel", func(t *testing.T) {
		req := require.New(t)
		m := newRouterTxMap()
		oldCh := &stubCtrlChannel{id: "old"}
		newCh := &stubCtrlChannel{id: "new"}
		stale := newStubSender("r1", oldCh, true)
		m.Add("r1", stale)

		replacement := newStubSender("r1", newCh, false)
		got, created := m.GetOrCreate("r1", newCh, func() *RouterSender {
			return replacement
		})

		req.True(created)
		req.Same(replacement, got)
		req.Same(replacement, m.Get("r1"))
		req.True(isStopped(stale), "stale sender should have been stopped")
	})

	t.Run("concurrent calls create exactly one sender", func(t *testing.T) {
		req := require.New(t)
		m := newRouterTxMap()
		ch := &stubCtrlChannel{}

		const goroutines = 32
		var factoryCalls atomic.Int32
		results := make([]*RouterSender, goroutines)
		start := make(chan struct{})
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				rtx, _ := m.GetOrCreate("r1", ch, func() *RouterSender {
					factoryCalls.Add(1)
					return newStubSender("r1", ch, false)
				})
				results[idx] = rtx
			}(i)
		}

		close(start)
		wg.Wait()

		req.Equal(int32(1), factoryCalls.Load(), "factory must run exactly once")
		for i := 1; i < goroutines; i++ {
			req.Same(results[0], results[i], "all callers must get the same sender")
		}
	})
}

func TestRouterTxMap_RangeEdge(t *testing.T) {
	req := require.New(t)
	m := newRouterTxMap()
	m.Add("edge-1", newStubSender("edge-1", &stubCtrlChannel{}, true))
	m.Add("transit-1", newStubSender("transit-1", &stubCtrlChannel{}, false))
	m.Add("edge-2", newStubSender("edge-2", &stubCtrlChannel{}, true))

	collect := func(r func(func(*RouterSender))) []string {
		var ids []string
		r(func(rtx *RouterSender) { ids = append(ids, rtx.Router.Id) })
		sort.Strings(ids)
		return ids
	}

	req.Equal([]string{"edge-1", "edge-2"}, collect(m.RangeEdge), "RangeEdge must skip non-edge senders")
	req.Equal([]string{"edge-1", "edge-2", "transit-1"}, collect(m.Range), "Range must visit all senders")
}
