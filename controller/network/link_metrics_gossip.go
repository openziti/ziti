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

package network

import (
	"time"

	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/gossip"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/controller/model"
	"google.golang.org/protobuf/proto"
)

// linkMetricsGossipLog is the logger for controller-side link-metrics gossip. Its
// channel name is "controller.link.metrics.gossip".
var linkMetricsGossipLog = logging.For("controller.link.metrics.gossip")

// LinkMetricsGossipStoreType is the gossip store type name for link metrics
// (per-link latency). It is keyed by link id and owned by the reporting router.
const LinkMetricsGossipStoreType = "link-metrics"

// InitLinkMetricsGossip registers the link-metrics gossip state type. Must be
// called after InitGossipStore so that GossipStore is non-nil.
func (network *Network) InitLinkMetricsGossip() *gossip.StateType[*ctrl_pb.LinkMetrics] {
	listener := &linkMetricsGossipListener{network: network}

	metricsType := gossip.Register[*ctrl_pb.LinkMetrics](network.GossipStore, gossip.StateTypeConfig[*ctrl_pb.LinkMetrics]{
		Name: LinkMetricsGossipStoreType,
		Encode: func(v *ctrl_pb.LinkMetrics) ([]byte, error) {
			return proto.Marshal(v)
		},
		Decode: func(b []byte) (*ctrl_pb.LinkMetrics, error) {
			v := &ctrl_pb.LinkMetrics{}
			return v, proto.Unmarshal(b, v)
		},
		Tombstones:          true,
		TombstoneTTL:        5 * time.Minute,
		AntiEntropy:         true,
		AntiEntropyInterval: 30 * time.Second,
		Listener:            listener,
	})

	network.LinkMetricsType = metricsType
	network.RegisterGossipType(LinkMetricsGossipStoreType, metricsType)
	return metricsType
}

// applyLinkMetric sets the source or destination latency of the given link from a
// link-metrics gossip entry, but only when the entry's iteration matches the
// link's current iteration. An older iteration is ignored; a newer iteration is
// left unapplied (link creation reconciles it from the store once the link-state
// entry for that iteration arrives). The owner of the entry is the reporting
// router, so owner == link.Src.Id sets source latency and owner == link.DstId
// sets destination latency, the same directionality AcceptMetricsMsg uses.
func (network *Network) applyLinkMetric(value *ctrl_pb.LinkMetrics, owner string) {
	link, found := network.Link.Get(value.LinkId)
	if !found {
		return
	}

	if value.Iteration != link.Iteration {
		return
	}

	if owner == link.Src.Id {
		link.SetSrcLatency(value.LatencyNanos)
	} else if owner == link.DstId {
		link.SetDstLatency(value.LatencyNanos)
	} else {
		linkMetricsGossipLog.Debug("link-metrics entry owner is neither endpoint of the link",
			"linkId", value.LinkId,
			"owner", owner,
			"srcRouterId", link.Src.Id,
			"destRouterId", link.DstId)
	}
}

// reconcileLinkMetricsForLink applies the current link-metrics entries for the
// given link from both its source and destination owners. Called when the
// link-state listener creates or replaces a link, so a metrics entry that arrived
// before the link existed (the two stores replicate independently) lands on the
// new link as soon as it is created. Each side applies only when its entry's
// iteration matches the link's current iteration.
func (network *Network) reconcileLinkMetricsForLink(link *model.Link) {
	if network.LinkMetricsType == nil {
		return
	}
	owners := []string{link.Src.Id}
	if link.DstId != "" && link.DstId != link.Src.Id {
		owners = append(owners, link.DstId)
	}
	for _, owner := range owners {
		if value, _, ok := network.LinkMetricsType.GetForOwner(owner, link.Id); ok {
			network.applyLinkMetric(value, owner)
		}
	}
}

// linkMetricsGossipListener implements gossip.StateListener for link metrics.
type linkMetricsGossipListener struct {
	network *Network
}

func (l *linkMetricsGossipListener) EntryChanged(_ string, value *ctrl_pb.LinkMetrics, _ uint64, owner string, _ bool, _ gossip.ChangeOrigin) {
	l.network.applyLinkMetric(value, owner)
}

func (l *linkMetricsGossipListener) EntryRemoved(key string, owner string, _ uint64, _ gossip.ChangeOrigin) {
	// A link-metrics tombstone means the link was removed. The link-state store's
	// own tombstone removes the link from the link manager and reroutes, so there
	// is nothing to undo here; the latency dies with the link.
	linkMetricsGossipLog.Debug("link-metrics entry removed", "linkId", key, "owner", owner)
}

// LinkMetricsInspectResult is the JSON structure returned by the
// "gossip-link-metrics" inspect key.
type LinkMetricsInspectResult struct {
	Entries []LinkMetricsEntry `json:"entries"`
	Count   int                `json:"count"`
}

// LinkMetricsEntry describes a single link-metrics entry in the gossip store.
type LinkMetricsEntry struct {
	LinkId       string `json:"linkId"`
	Iteration    uint32 `json:"iteration"`
	Owner        string `json:"owner"`
	Version      uint64 `json:"version"`
	LatencyNanos int64  `json:"latencyNanos"`
	Epoch        string `json:"epoch,omitempty"`
}

func (network *Network) inspectGossipLinkMetrics() *LinkMetricsInspectResult {
	result := &LinkMetricsInspectResult{}
	network.LinkMetricsType.IterFull(func(key string, value *ctrl_pb.LinkMetrics, owner string, version uint64, epoch []byte) {
		result.Entries = append(result.Entries, LinkMetricsEntry{
			LinkId:       key,
			Iteration:    value.Iteration,
			Owner:        owner,
			Version:      version,
			LatencyNanos: value.LatencyNanos,
			Epoch:        idgen.FormatEpoch(epoch),
		})
	})
	result.Count = len(result.Entries)
	return result
}
