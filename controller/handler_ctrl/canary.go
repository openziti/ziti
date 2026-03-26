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

package handler_ctrl

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// canaryHandler receives canary sequence numbers and epochs from routers and
// stores them in the gossip store for distribution to peer controllers.
type canaryHandler struct {
	r       *model.Router
	network *network.Network
}

func newCanaryHandler(r *model.Router, network *network.Network) *canaryHandler {
	return &canaryHandler{r: r, network: network}
}

func (h *canaryHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CanaryType)
}

func (h *canaryHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	seq, ok := msg.GetUint64Header(int32(ctrl_pb.ControlHeaders_CanarySeqHeader))
	if !ok {
		return
	}

	epoch, _ := msg.Headers[int32(ctrl_pb.ControlHeaders_EpochHeader)]

	value := &network.CanaryValue{
		Seq:   seq,
		Epoch: epoch,
	}

	// Parse per-store max sent versions and entry hashes from the body.
	if len(msg.Body) > 0 {
		payload := &gossip_pb.CanaryPayload{}
		if err := proto.Unmarshal(msg.Body, payload); err == nil {
			value.MaxSentVersions = payload.MaxSentVersions
			value.EntryHashes = payload.EntryHashes
			value.EntryCounts = payload.EntryCounts
		}
	}

	if err := h.network.GetRouterEventsPool().QueueOrError(func() {
		if err := h.network.CanaryGossipType.Set(h.r.Id, h.network.GetAppId(), value); err != nil {
			pfxlog.Logger().WithError(err).WithField("routerId", h.r.Id).
				Error("failed to set canary in gossip store")
		}
	}); err != nil {
		pfxlog.Logger().WithField("routerId", h.r.Id).Info("router events pool full, dropping canary")
	}
}

const (
	// gossipStaleBehindTicks is how many consecutive staleness check ticks
	// (10s each) must pass before triggering a digest exchange.
	gossipStaleBehindTicks = 3
)

// startCanaryStatusSender launches a goroutine that periodically sends this
// controller's view of the router's canary sequence back to the router. It also
// checks for gossip staleness: if the router reports a higher max sent version
// than this controller has, it triggers a digest exchange after a brief delay.
func startCanaryStatusSender(ch channel.Channel, router *model.Router, n *network.Network, closeNotify <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		log := pfxlog.Logger().WithField("routerId", router.Id)

		behindTicks := map[string]int{}
		lastSyncedMaxSent := map[string]uint64{}
		hashState := map[string]*hashMismatchState{}

		for {
			select {
			case <-closeNotify:
				return
			case <-ticker.C:
				if ch.IsClosed() {
					return
				}

				seq, ok := n.GetCanaryForRouter(router.Id)
				if !ok {
					continue
				}

				msg := channel.NewMessage(int32(ctrl_pb.ContentType_CanaryStatusType), nil)
				msg.PutUint64Header(int32(ctrl_pb.ControlHeaders_CanarySeqHeader), seq)

				if err := msg.WithTimeout(5 * time.Second).Send(ch); err != nil {
					if !ch.IsClosed() {
						log.WithError(err).Debug("failed to send canary status to router")
					}
					return
				}

				// Check for gossip staleness per store type.
				checkGossipStaleness(ch, router, n, log, behindTicks, lastSyncedMaxSent, hashState)
			}
		}
	}()
}

// hashMismatchState tracks previous hash values for a store type so we can
// detect when both sides have stabilized but still disagree.
type hashMismatchState struct {
	ticks          int
	prevRouterHash uint64
	prevCtrlHash   uint64
}

// checkGossipStaleness compares the router's reported max sent versions and
// entry hashes against this controller's gossip store. If the controller is
// behind on versions or has a stable hash mismatch for a sustained period, it
// triggers a digest exchange.
func checkGossipStaleness(ch channel.Channel, router *model.Router, n *network.Network,
	log *logrus.Entry, behindTicks map[string]int, lastSyncedMaxSent map[string]uint64,
	hashState map[string]*hashMismatchState) {

	canary, ok := n.GetCanaryValueForRouter(router.Id)
	if !ok {
		return
	}

	for storeType, routerMaxSent := range canary.MaxSentVersions {
		if routerMaxSent == 0 {
			continue
		}

		// Already synced for this version; skip.
		if routerMaxSent <= lastSyncedMaxSent[storeType] {
			delete(behindTicks, storeType)
			continue
		}

		// Look up the gossip type to get the controller's max version.
		gossipType := n.GetGossipType(storeType)
		if gossipType == nil {
			continue
		}

		ctrlMax := gossipType.MaxVersionForOwner(router.Id)

		if routerMaxSent <= ctrlMax {
			// In sync.
			delete(behindTicks, storeType)
			lastSyncedMaxSent[storeType] = routerMaxSent
			continue
		}

		// Controller is behind.
		behindTicks[storeType]++
		if behindTicks[storeType] >= gossipStaleBehindTicks {
			log.WithField("storeType", storeType).
				WithField("routerMaxSent", routerMaxSent).
				WithField("ctrlMax", ctrlMax).
				WithField("behindTicks", behindTicks[storeType]).
				Info("gossip staleness detected, triggering digest exchange")

			sendRouterGossipDigest(ch, router, n, storeType)
			lastSyncedMaxSent[storeType] = routerMaxSent
			delete(behindTicks, storeType)
		}
	}

	// Check entry hash mismatches. First compare entry counts as a cheap
	// fast-path: if counts differ, we know the state is stale without
	// computing the expensive hash. Only fall through to hash comparison
	// when counts match.
	for storeType, routerHash := range canary.EntryHashes {
		gossipType := n.GetGossipType(storeType)
		if gossipType == nil {
			continue
		}

		// Fast-path: if the router sent an entry count and it doesn't match,
		// skip the expensive hash computation.
		if routerCount, ok := canary.EntryCounts[storeType]; ok {
			ctrlCount := gossipType.NonTombstoneCount(router.Id)
			if routerCount != ctrlCount {
				st := hashState[storeType]
				if st == nil {
					st = &hashMismatchState{}
					hashState[storeType] = st
				}
				st.ticks++
				if st.ticks >= gossipStaleBehindTicks {
					log.WithField("storeType", storeType).
						WithField("routerCount", routerCount).
						WithField("ctrlCount", ctrlCount).
						Info("gossip entry count mismatch detected, triggering digest exchange")
					sendRouterGossipDigest(ch, router, n, storeType)
					delete(hashState, storeType)
				}
				continue
			}
		}

		ctrlHash := gossipType.HashForOwner(router.Id)
		if routerHash == ctrlHash {
			delete(hashState, storeType)
			continue
		}

		st := hashState[storeType]
		if st == nil {
			st = &hashMismatchState{}
			hashState[storeType] = st
		}

		if routerHash != st.prevRouterHash || ctrlHash != st.prevCtrlHash {
			// One or both sides changed since last check — still in flux.
			st.ticks = 0
		}
		st.prevRouterHash = routerHash
		st.prevCtrlHash = ctrlHash

		st.ticks++
		if st.ticks >= gossipStaleBehindTicks {
			log.WithField("storeType", storeType).
				WithField("routerHash", routerHash).
				WithField("ctrlHash", ctrlHash).
				WithField("stableMismatchTicks", st.ticks).
				Info("gossip entry hash mismatch detected, triggering digest exchange")

			sendRouterGossipDigest(ch, router, n, storeType)
			delete(hashState, storeType)
		}
	}
}
