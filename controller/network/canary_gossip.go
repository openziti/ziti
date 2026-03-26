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
	"bytes"
	"sync"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/controller/gossip"
	"github.com/openziti/ziti/v2/controller/idgen"
	"google.golang.org/protobuf/proto"
)

const CanaryGossipStoreType = "canaries"

// CanaryValue is the gossip store value for canary entries. It is serialized as
// protobuf for forward-compatible extensibility.
type CanaryValue = gossip_pb.CanaryGossipValue

// InitCanaryGossip registers the canary gossip state type. Canaries use no
// tombstones and no anti-entropy — they are purely fire-and-forget probes.
// Must be called after InitGossipStore.
func (network *Network) InitCanaryGossip() *gossip.StateType[*CanaryValue] {
	listener := &canaryGossipListener{network: network}

	canaryType := gossip.Register[*CanaryValue](network.GossipStore, gossip.StateTypeConfig[*CanaryValue]{
		Name: CanaryGossipStoreType,
		Encode: encodeCanaryValue,
		Decode: decodeCanaryValue,
		Tombstones:  false,
		AntiEntropy: false,
		Listener:    listener,
	})

	network.CanaryGossipType = canaryType
	network.canaryListener = listener
	return canaryType
}

// GetCanaryForRouter returns the latest canary sequence number this controller
// knows about for the given router, as received directly or via gossip.
func (network *Network) GetCanaryForRouter(routerId string) (uint64, bool) {
	return network.canaryListener.getSeq(routerId)
}

// GetCanaryValueForRouter returns the full canary value for a router, including
// epoch, per-store max sent versions, and entry hashes.
func (network *Network) GetCanaryValueForRouter(routerId string) (*CanaryValue, bool) {
	return network.canaryListener.getValue(routerId)
}

// canaryGossipListener tracks the latest canary state per router and detects
// epoch changes to trigger cleanup of old link gossip entries.
type canaryGossipListener struct {
	network  *Network
	canaries sync.Map // routerId (string) -> CanaryValue
}

func (l *canaryGossipListener) EntryChanged(key string, value *CanaryValue, _ uint64, _ string, _ bool, _ gossip.ChangeOrigin) {
	prev, loaded := l.canaries.Swap(key, value)

	if loaded && len(value.Epoch) > 0 {
		prevVal := prev.(*CanaryValue)
		if len(prevVal.Epoch) > 0 && bytes.Compare(value.Epoch, prevVal.Epoch) > 0 {
			// New epoch is newer — the router restarted. Clean up old-epoch
			// link entries for this router.
			log := pfxlog.Logger().
				WithField("routerId", key).
				WithField("oldEpoch", idgen.FormatEpoch(prevVal.Epoch)).
				WithField("newEpoch", idgen.FormatEpoch(value.Epoch))
			log.Info("router epoch changed, cleaning up old link gossip entries")

			if l.network.LinkGossipType != nil {
				l.network.LinkGossipType.DeleteByOwnerBefore(key, value.Epoch)
			}
		}
	}
}

func (l *canaryGossipListener) EntryRemoved(key string, _ string, _ gossip.ChangeOrigin) {
	l.canaries.Delete(key)
}

// checkEpoch compares the given epoch against the stored epoch for a router.
// If the new epoch is newer, old-epoch link entries are cleaned up. Also
// handles the case where no previous epoch is stored (controller restart):
// checks link gossip entries directly for stale epochs.
func (l *canaryGossipListener) checkEpoch(routerId string, epoch []byte) {
	if len(epoch) == 0 {
		return
	}

	prev, loaded := l.canaries.Load(routerId)
	if loaded {
		prevVal := prev.(*CanaryValue)
		if len(prevVal.Epoch) > 0 && bytes.Compare(epoch, prevVal.Epoch) > 0 {
			pfxlog.Logger().
				WithField("routerId", routerId).
				WithField("oldEpoch", idgen.FormatEpoch(prevVal.Epoch)).
				WithField("newEpoch", idgen.FormatEpoch(epoch)).
				Info("router epoch changed, cleaning up old link gossip entries")

			if l.network.LinkGossipType != nil {
				l.network.LinkGossipType.DeleteByOwnerBefore(routerId, epoch)
			}
		}
	} else if l.network.LinkGossipType != nil {
		// No stored epoch (controller may have restarted). Check link gossip
		// entries directly for any from an older epoch. This catches the case
		// where old entries arrived via a peer snapshot but the canary store
		// doesn't have the previous epoch to compare against.
		l.network.LinkGossipType.DeleteByOwnerBefore(routerId, epoch)
	}
}

func (l *canaryGossipListener) getSeq(routerId string) (uint64, bool) {
	if v, ok := l.canaries.Load(routerId); ok {
		return v.(*CanaryValue).Seq, true
	}
	return 0, false
}

func (l *canaryGossipListener) getValue(routerId string) (*CanaryValue, bool) {
	if v, ok := l.canaries.Load(routerId); ok {
		return v.(*CanaryValue), true
	}
	return nil, false
}

func encodeCanaryValue(v *CanaryValue) ([]byte, error) {
	return proto.Marshal(v)
}

func decodeCanaryValue(b []byte) (*CanaryValue, error) {
	v := &CanaryValue{}
	if err := proto.Unmarshal(b, v); err != nil {
		return v, err
	}
	return v, nil
}
