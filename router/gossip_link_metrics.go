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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/router/xlink"
	"google.golang.org/protobuf/proto"
)

// linkMetricsGossipStoreType is the gossip store type name for per-link latency.
// Must match the controller's LinkMetricsGossipStoreType.
const linkMetricsGossipStoreType = "link-metrics"

// Link-metrics publish thresholds. A link's latency is republished when it has
// changed by more than an absolute or a relative amount since the last publish,
// so a steady link rarely publishes while a meaningfully-changed one does. These
// are starting points; tune from a soak (see the gossip-metrics design doc).
const (
	linkMetricsMinChangeNanos    = int64(2 * 1000 * 1000) // 2ms
	linkMetricsMinChangeFraction = 0.15                   // 15%
)

// publishedMetric records the last latency a router published for a link, so the
// periodic publisher can apply the change-threshold and force-on-iteration rules.
type publishedMetric struct {
	iteration    uint32
	latencyNanos int64
}

// linkMetricsGossipSource adapts a router's per-link latency measurements as a
// gossip source for the link-metrics store. Unlike the link-state source, it
// advertises an entry for every link the router is an endpoint of (dialed or
// not), since both ends of a link publish their own latency measurement. The
// entry is keyed by link id alone (no iteration) so a re-dial updates the entry
// in place rather than churning a tombstone per iteration.
type linkMetricsGossipSource struct {
	linkIterator    func() <-chan xlink.Xlink
	metricsRegistry metrics.Registry
	lastPublished   map[string]publishedMetric // link id -> last published; publisher-goroutine only
}

func (s *linkMetricsGossipSource) storeType() string { return linkMetricsGossipStoreType }

// latencyNanosFor returns the router's current latency cost for a link: the
// latency mean plus the queue_time mean, in nanoseconds, matching what
// AcceptMetricsMsg computes. Returns 0 when no latency has been measured yet
// (no heartbeat sample), so callers can skip publishing a meaningless zero.
func (s *linkMetricsGossipSource) latencyNanosFor(linkId string) int64 {
	if s.metricsRegistry == nil {
		return 0
	}
	latency := s.metricsRegistry.Histogram("link." + linkId + ".latency").Mean()
	queueTime := s.metricsRegistry.Histogram("link." + linkId + ".queue_time").Mean()
	return int64(latency + queueTime)
}

func (s *linkMetricsGossipSource) iterateAdvertised(fn func(key string, value []byte)) {
	if s.linkIterator == nil {
		return
	}
	for l := range s.linkIterator() {
		latencyNanos := s.latencyNanosFor(l.Id())
		if latencyNanos <= 0 {
			continue
		}
		value, err := marshalLinkMetrics(l.Id(), l.Iteration(), latencyNanos)
		if err != nil {
			continue
		}
		fn(l.Id(), value)
	}
}

// marshalLinkMetrics encodes a link-metrics gossip value.
func marshalLinkMetrics(linkId string, iteration uint32, latencyNanos int64) ([]byte, error) {
	return proto.Marshal(&ctrl_pb.LinkMetrics{
		LinkId:       linkId,
		Iteration:    iteration,
		LatencyNanos: latencyNanos,
	})
}

// linkMetricsStore returns the link-metrics gossip store.
func (g *gossipClient) linkMetricsStore() *gossipStore {
	return g.stores[linkMetricsGossipStoreType]
}

// linkMetricsChanged reports whether a new latency differs meaningfully from the
// last published one, per the absolute and relative thresholds.
func linkMetricsChanged(oldNanos, newNanos int64) bool {
	diff := newNanos - oldNanos
	if diff < 0 {
		diff = -diff
	}
	if diff >= linkMetricsMinChangeNanos {
		return true
	}
	return oldNanos > 0 && float64(diff)/float64(oldNanos) >= linkMetricsMinChangeFraction
}

// publishLinkMetrics publishes per-link latency to the link-metrics gossip store
// for links whose latency changed meaningfully since the last publish, and force-
// publishes any link whose dial iteration advanced (so a re-dialed link gets fresh
// latency immediately rather than waiting on the change threshold). Driven by the
// gossipRefresher tick, so "at most once per tick per link" falls out of the
// cadence. A no-op until every controller is gossip-capable.
func (g *gossipClient) publishLinkMetrics() {
	if !g.ctrls.AllControllersHaveCapability(capabilities.ControllerLinkGossip) {
		return
	}

	s := g.linkMetricsStore()
	src, ok := s.source.(*linkMetricsGossipSource)
	if !ok || src.linkIterator == nil {
		return
	}

	sub := g.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return
	}

	var entries []*gossip_pb.GossipEntry
	pending := map[string]publishedMetric{}
	for l := range src.linkIterator() {
		linkId := l.Id()
		iteration := l.Iteration()
		latencyNanos := src.latencyNanosFor(linkId)
		if latencyNanos <= 0 {
			continue
		}

		last, seen := src.lastPublished[linkId]
		if seen && last.iteration == iteration && !linkMetricsChanged(last.latencyNanos, latencyNanos) {
			continue
		}

		value, err := marshalLinkMetrics(linkId, iteration, latencyNanos)
		if err != nil {
			continue
		}
		entries = append(entries, &gossip_pb.GossipEntry{
			Key:     linkId,
			Value:   value,
			Version: g.nextVersion(),
			Owner:   g.routerId,
			Epoch:   g.epoch,
		})
		pending[linkId] = publishedMetric{iteration: iteration, latencyNanos: latencyNanos}
	}

	if len(entries) == 0 {
		return
	}

	if err := g.sendDelta(sub.Channel(), linkMetricsGossipStoreType, entries); err != nil {
		pfxlog.Logger().WithError(err).Warn("failed to publish link metrics via gossip, will retry")
		return
	}

	for _, entry := range entries {
		s.currentEntries.Set(entry.Key, entry)
	}
	s.invalidateHash()
	for linkId, pm := range pending {
		src.lastPublished[linkId] = pm
	}
	g.updateMaxSentFromEntries(linkMetricsGossipStoreType, entries)
	g.sendToStaleControllers(sub.Channel().Id(), linkMetricsGossipStoreType, entries)
}
