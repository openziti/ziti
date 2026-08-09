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

package router

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// capturingPool records what was queued without running it, so a test can tell whether work was handed off
// or run on the calling goroutine.
type capturingPool struct {
	queued atomic.Int32
	reject bool
}

func (p *capturingPool) QueueOrError(f func()) error {
	if p.reject {
		return fmt.Errorf("pool full")
	}
	p.queued.Add(1)
	return nil
}

func (p *capturingPool) Queue(f func()) error                             { return p.QueueOrError(f) }
func (p *capturingPool) QueueWithTimeout(f func(), _ time.Duration) error { return p.QueueOrError(f) }
func (p *capturingPool) GetWorkerCount() uint32                           { return 0 }
func (p *capturingPool) GetQueueSize() uint32                             { return 0 }
func (p *capturingPool) GetBusyWorkers() uint32                           { return 0 }
func (p *capturingPool) GetOutstanding() uint32                           { return 0 }
func (p *capturingPool) Shutdown()                                        {}
func (p *capturingPool) ShutdownAndWait(time.Duration) error              { return nil }
func (p *capturingPool) AwaitIdle(time.Duration) error                    { return nil }

func digestMessage(t *testing.T, storeType string) *channel.Message {
	t.Helper()
	body, err := proto.Marshal(&gossip_pb.GossipDigest{StoreType: storeType})
	require.NoError(t, err)
	return channel.NewMessage(gossip_pb.GossipDigestType, body)
}

// TestHandleDigest_OffloadsOffTheReceiveGoroutine is the guard on the antipattern this fixed. Comparing a
// digest walks every entry the controller reported and every entry this router holds, then sends a response,
// and doing that on a channel's receive goroutine stops the router reading anything else from that control
// channel until it finishes. Measured, that stalled control channels for a half second at the median and up
// to four seconds, which is what the controller sees as send back-pressure.
func TestHandleDigest_OffloadsOffTheReceiveGoroutine(t *testing.T) {
	g := &gossipClient{routerId: "r1", stores: map[string]*gossipStore{}}
	pool := &capturingPool{}
	g.setPool(pool)

	g.HandleDigest(digestMessage(t, "links"), nil)

	require.Equal(t, int32(1), pool.queued.Load(),
		"digest comparison must be handed to the pool, not run on the receive goroutine")
}

// TestHandleDigest_DropsWhenPoolFull: a dropped digest exchange is safe, since this is anti-entropy and the
// next round repairs whatever this one would have. What must not happen is falling back to the receive
// goroutine, which is the stall being avoided.
func TestHandleDigest_DropsWhenPoolFull(t *testing.T) {
	g := &gossipClient{routerId: "r1", stores: map[string]*gossipStore{}}
	pool := &capturingPool{reject: true}
	g.setPool(pool)

	require.NotPanics(t, func() { g.HandleDigest(digestMessage(t, "links"), nil) })
	require.Equal(t, int32(0), pool.queued.Load())
}

// TestHandleDigest_HandlesInlineWithoutAPool covers startup, before the pool exists. Handling inline is
// correct then, since there is nowhere else to run and no traffic to stall yet.
func TestHandleDigest_HandlesInlineWithoutAPool(t *testing.T) {
	g := &gossipClient{routerId: "r1", stores: map[string]*gossipStore{}}
	require.Nil(t, g.getPool())

	// An unknown store type returns before touching the channel, so this exercises the inline path safely.
	require.NotPanics(t, func() { g.HandleDigest(digestMessage(t, "unknown"), nil) })
}

// TestHandleDigest_MalformedBodyIsIgnored: the unmarshal is the only part still on the receive goroutine, so
// a bad body must not reach the pool or panic.
func TestHandleDigest_MalformedBodyIsIgnored(t *testing.T) {
	g := &gossipClient{routerId: "r1", stores: map[string]*gossipStore{}}
	pool := &capturingPool{}
	g.setPool(pool)

	require.NotPanics(t, func() {
		g.HandleDigest(channel.NewMessage(gossip_pb.GossipDigestType, []byte{0xff, 0xfe, 0xfd}), nil)
	})
	require.Equal(t, int32(0), pool.queued.Load())
}

// closedSendChannel is a control channel whose send fails and which reports itself closed. That combination
// is the state sendDelta must not report as a successful publish.
type closedSendChannel struct {
	channel.Channel
	sends atomic.Int32
}

func (c *closedSendChannel) Send(channel.Sendable) error {
	c.sends.Add(1)
	return fmt.Errorf("channel is closed")
}

func (c *closedSendChannel) IsClosed() bool { return true }

// Test_sendDelta_ReportsAFailedSendOnAClosedChannel covers the answer every caller's bookkeeping hangs off.
// Reporting success for a closed channel left the router recording entries as published, links as notified,
// faults as no longer pending and a higher version as sent, for state the controller never received, with
// nothing local left to retry it.
func Test_sendDelta_ReportsAFailedSendOnAClosedChannel(t *testing.T) {
	g := newGossipClient("router1", nil, nil)
	ch := &closedSendChannel{}

	entries := []*gossip_pb.GossipEntry{{
		Key:     "link1:1",
		Version: 1,
		Owner:   "router1",
	}}

	err := g.sendDelta(ch, linkGossipStoreType, entries)
	require.Error(t, err, "a send that did not happen must not be reported as a publish")
	require.Equal(t, int32(1), ch.sends.Load(), "the send should have been attempted")

	// The watermark the canary reports is only advanced by callers that saw a successful send, so a failed
	// one must leave it alone.
	require.Zero(t, g.GetMaxSentVersions()[linkGossipStoreType],
		"a failed send must not advance the version the router claims to have sent")
}

// Test_gossipStore_hashConvergesAfterConcurrentMutation covers the property the whole hash gate rests on: once
// mutation stops, what the router reports describes what it holds.
//
// A pass over the entries is not atomic with publishing its result, so one that begins before a mutation can
// finish after it. Clearing the cache on mutation let such a pass overwrite the clear, leaving a wrong hash and
// count published with nothing left to invalidate them until the next mutation. For a router whose links have
// settled that is indefinitely, and a wrong hash is exactly what makes the controller's comparison either skip
// a reconcile it needs or run ones it does not.
func Test_gossipStore_hashConvergesAfterConcurrentMutation(t *testing.T) {
	for round := 0; round < 50; round++ {
		s := newGossipStore(&linkGossipSource{})

		const entryCount = 64
		var wg sync.WaitGroup

		// Readers race the writers, so a read's pass straddles mutations.
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < entryCount; j++ {
					s.hash()
					s.count()
				}
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < entryCount; j++ {
				s.currentEntries.Set(fmt.Sprintf("link%03d:1", j), &gossip_pb.GossipEntry{
					Key:     fmt.Sprintf("link%03d:1", j),
					Version: uint64(j + 1),
				})
				s.invalidateHash()
			}
		}()

		wg.Wait()

		// Quiesced, so the published count must be the truth rather than whatever a straddling pass left.
		require.Equal(t, int64(entryCount), s.count(), "round %d published a stale count", round)
	}
}

// Test_gossipStore_staleGenerationIsNotTrusted covers the state that race leaves behind directly: a cached
// value stamped with a generation the store has moved past must be recomputed, not returned.
func Test_gossipStore_staleGenerationIsNotTrusted(t *testing.T) {
	s := newGossipStore(&linkGossipSource{})
	s.currentEntries.Set("link1:1", &gossip_pb.GossipEntry{Key: "link1:1", Version: 1})
	s.invalidateHash()

	real := s.hash()
	require.Equal(t, int64(1), s.count())

	// What a pass that began a generation ago would publish.
	s.cached.Store(&hashCount{hash: real + 1, count: 99, generation: s.generation.Load() - 1})

	require.Equal(t, real, s.hash(), "a hash from a superseded generation must not be published")
	require.Equal(t, int64(1), s.count())
}

// epochAt builds an epoch carrying the given millisecond timestamp, so a test can state the wall-clock
// relationship between two router lifetimes rather than depend on when it happened to run.
func epochAt(millis uint64) []byte {
	epoch := make([]byte, 16)
	for i := 5; i >= 0; i-- {
		epoch[i] = byte(millis)
		millis >>= 8
	}
	return epoch
}

// Test_canaryEmitter_sequenceOrdersAcrossLifetimes covers the guarantee a restart depends on. A controller
// stores a canary under its sequence as the version and refuses anything not newer, so the sequence a restarted
// router starts from has to exceed whatever its predecessor reached.
//
// Seeding from the epoch gives that with no margin to choose, which is the point: the epoch advances by the
// whole elapsed wall time, the previous lifetime's uptime included, while the sequence advances once per tick.
// A lifetime can only tick its way to uptime/interval, so however long it ran and however briefly it was down,
// the next epoch is already past it.
func Test_canaryEmitter_sequenceOrdersAcrossLifetimes(t *testing.T) {
	const tickMillis = 5_000 // the emitter's interval, which is what bounds how far a lifetime can tick

	tests := []struct {
		name     string
		uptime   time.Duration
		downtime time.Duration
	}{
		{name: "a brief lifetime, restarted at once", uptime: time.Second, downtime: time.Millisecond},
		{name: "a day of uptime, restarted at once", uptime: 24 * time.Hour, downtime: time.Millisecond},
		{name: "a year of uptime, restarted at once", uptime: 365 * 24 * time.Hour, downtime: time.Millisecond},
		{name: "a year of uptime, down for a week", uptime: 365 * 24 * time.Hour, downtime: 7 * 24 * time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			startMillis := uint64(1_700_000_000_000)

			previous := newCanaryEmitter(nil, epochAt(startMillis), nil, nil, nil, nil)
			require.Equal(t, startMillis, previous.seq.Load(),
				"the sequence starts at the epoch's millisecond timestamp, not at zero")

			// As far as the lifetime could have ticked in its uptime.
			ticks := uint64(test.uptime.Milliseconds()) / tickMillis
			previous.seq.Add(ticks)
			previousLast := previous.seq.Load()

			// The restart takes its epoch at startup, so the whole uptime and the downtime have elapsed.
			restartMillis := startMillis + uint64(test.uptime.Milliseconds()) + uint64(test.downtime.Milliseconds())
			current := newCanaryEmitter(nil, epochAt(restartMillis), nil, nil, nil, nil)

			require.Greater(t, current.seq.Add(1), previousLast,
				"a restarted router's first sequence must exceed its predecessor's last, or its canaries are refused")
		})
	}
}

func Test_EpochMillis(t *testing.T) {
	t.Run("reads the leading 48 bits big-endian", func(t *testing.T) {
		epoch := make([]byte, 16)
		epoch[4] = 0x01
		epoch[5] = 0x02
		require.Equal(t, uint64(0x0102), idgen.EpochMillis(epoch))

		epoch[0] = 0xFF
		require.Equal(t, uint64(0xFF0000000102), idgen.EpochMillis(epoch))
	})

	t.Run("a real epoch reads as a plausible wall clock", func(t *testing.T) {
		// Sanity that the field really is a unix millisecond timestamp: somewhere after 2020 and before 2100.
		millis := idgen.EpochMillis(idgen.NewEpochBytes())
		require.Greater(t, millis, uint64(1_577_836_800_000))
		require.Less(t, millis, uint64(4_102_444_800_000))
	})

	t.Run("anything that is not an epoch reads as zero", func(t *testing.T) {
		require.Zero(t, idgen.EpochMillis(nil))
		require.Zero(t, idgen.EpochMillis([]byte{1, 2, 3}))
	})
}

// stubGossipSource advertises a fixed set of keys, standing in for a router's source of truth.
type stubGossipSource struct {
	name      string
	advertise map[string][]byte
}

func (s *stubGossipSource) storeType() string { return s.name }

func (s *stubGossipSource) iterateAdvertised(fn func(key string, value []byte)) {
	for key, value := range s.advertise {
		fn(key, value)
	}
}

// capturingChannel records the digest responses sent on it.
type capturingChannel struct {
	channel.Channel
	sent []*gossip_pb.GossipDigestResponse
}

func (c *capturingChannel) Send(s channel.Sendable) error {
	resp := &gossip_pb.GossipDigestResponse{}
	if err := proto.Unmarshal(s.Msg().Body, resp); err != nil {
		return err
	}
	c.sent = append(c.sent, resp)
	return nil
}

func (c *capturingChannel) IsClosed() bool { return false }

func (c *capturingChannel) entries() []*gossip_pb.GossipEntry {
	var all []*gossip_pb.GossipEntry
	for _, resp := range c.sent {
		all = append(all, resp.Entries...)
	}
	return all
}

// newDigestTestClient returns a client whose only store advertises the given keys.
func newDigestTestClient(t *testing.T, storeType string, advertise map[string][]byte) (*gossipClient, *gossipStore) {
	t.Helper()
	source := &stubGossipSource{name: storeType, advertise: advertise}
	store := newGossipStore(source)
	g := &gossipClient{
		routerId: "r1",
		epoch:    idgen.NewEpochBytes(),
		stores:   map[string]*gossipStore{storeType: store},
	}
	return g, store
}

// Test_handleDigest_RestampsWhenTheControllerIsAhead covers the state a restart leaves behind. The Lamport
// clock is in memory, so it returns to zero, while the controller still holds versions from the previous
// incarnation under the same key. Link metrics are keyed by link id alone, and that id belongs to the
// dialer, so an acceptor's restart does not change it.
//
// Keeping the stored version would send nothing, and every later digest would compare the same two numbers
// and reach the same answer, so the exchange whose job is to repair divergence would be the thing
// perpetuating it.
func Test_handleDigest_RestampsWhenTheControllerIsAhead(t *testing.T) {
	req := require.New(t)

	g, store := newDigestTestClient(t, "test-store", map[string][]byte{"link1": []byte("live")})
	// What the router published before it restarted, re-established with a clock back at zero.
	store.currentEntries.Set("link1", &gossip_pb.GossipEntry{Key: "link1", Version: 1, Owner: "r1"})

	ch := &capturingChannel{}
	g.handleDigest(&gossip_pb.GossipDigest{
		StoreType: "test-store",
		Entries:   []*gossip_pb.DigestEntry{{Key: "link1", Version: 500}},
	}, ch)

	sent := ch.entries()
	req.Len(sent, 1, "the live value must be sent, not skipped for being lower versioned")
	req.Equal("link1", sent[0].Key)
	req.Equal([]byte("live"), sent[0].Value, "the value sent must be the source of truth, not the stored copy")
	req.Greater(sent[0].Version, uint64(500), "the entry must outrank what the controller already holds")

	stored, ok := store.currentEntries.Get("link1")
	req.True(ok)
	req.Equal(sent[0].Version, stored.Version, "the new version must be recorded, or the next digest restamps again")
}

// Test_handleDigest_DoesNotRestampWhenInAgreement guards the other side. Restamping whenever the controller
// reports a key would resend the whole advertised set on every exchange and climb the clock with each one,
// so the version would count digests rather than changes.
func Test_handleDigest_DoesNotRestampWhenInAgreement(t *testing.T) {
	req := require.New(t)

	g, store := newDigestTestClient(t, "test-store", map[string][]byte{"link1": []byte("live")})
	store.currentEntries.Set("link1", &gossip_pb.GossipEntry{Key: "link1", Version: 7, Owner: "r1"})

	ch := &capturingChannel{}
	digest := &gossip_pb.GossipDigest{
		StoreType: "test-store",
		Entries:   []*gossip_pb.DigestEntry{{Key: "link1", Version: 7}},
	}
	g.handleDigest(digest, ch)
	g.handleDigest(digest, ch)

	req.Empty(ch.entries(), "a controller already in agreement must not be sent anything")

	stored, _ := store.currentEntries.Get("link1")
	req.Equal(uint64(7), stored.Version, "an agreed entry must keep its version")
}

// Test_handleDigest_RestampIsIdempotent: once the controller has caught up, the next exchange must be
// quiet. Without recording the new version this would restamp forever, trading a silent divergence for a
// loop that republishes on every digest.
func Test_handleDigest_RestampIsIdempotent(t *testing.T) {
	req := require.New(t)

	g, store := newDigestTestClient(t, "test-store", map[string][]byte{"link1": []byte("live")})
	store.currentEntries.Set("link1", &gossip_pb.GossipEntry{Key: "link1", Version: 1, Owner: "r1"})

	ch := &capturingChannel{}
	g.handleDigest(&gossip_pb.GossipDigest{
		StoreType: "test-store",
		Entries:   []*gossip_pb.DigestEntry{{Key: "link1", Version: 500}},
	}, ch)
	req.Len(ch.entries(), 1)
	restamped := ch.entries()[0].Version

	// The controller now reports what it was just sent.
	ch.sent = nil
	g.handleDigest(&gossip_pb.GossipDigest{
		StoreType: "test-store",
		Entries:   []*gossip_pb.DigestEntry{{Key: "link1", Version: restamped}},
	}, ch)

	req.Empty(ch.entries(), "a caught-up controller must end the exchange, not start another restamp")
}
