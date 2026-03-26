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
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/router/env"
	"google.golang.org/protobuf/proto"
)

// canaryEmitter periodically sends a monotonically increasing canary sequence
// number and the router's epoch to the subscription controller. The controller
// gossips the canary to peers, allowing them to detect router restarts (epoch
// change) and gossip health (sequence lag).
type canaryEmitter struct {
	ctrls              env.NetworkControllers
	epoch              []byte
	seq                atomic.Uint64
	getMaxSentVersions func() map[string]uint64
	getEntryHashes     func() map[string]uint64
	getEntryCounts     func() map[string]int64
	closeC             <-chan struct{}
}

func newCanaryEmitter(ctrls env.NetworkControllers, epoch []byte,
	getMaxSentVersions func() map[string]uint64,
	getEntryHashes func() map[string]uint64,
	getEntryCounts func() map[string]int64,
	closeC <-chan struct{},
) *canaryEmitter {
	return &canaryEmitter{
		ctrls:              ctrls,
		epoch:              epoch,
		getMaxSentVersions: getMaxSentVersions,
		getEntryHashes:     getEntryHashes,
		getEntryCounts:     getEntryCounts,
		closeC:             closeC,
	}
}

func (e *canaryEmitter) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.closeC:
			return
		case <-ticker.C:
			e.send()
		}
	}
}

func (e *canaryEmitter) send() {
	sub := e.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return
	}

	seq := e.seq.Add(1)

	msg := channel.NewMessage(int32(ctrl_pb.ContentType_CanaryType), nil)
	msg.PutUint64Header(int32(ctrl_pb.ControlHeaders_CanarySeqHeader), seq)
	msg.Headers[int32(ctrl_pb.ControlHeaders_EpochHeader)] = e.epoch

	var payload *gossip_pb.CanaryPayload
	if e.getMaxSentVersions != nil {
		if versions := e.getMaxSentVersions(); len(versions) > 0 {
			payload = &gossip_pb.CanaryPayload{MaxSentVersions: versions}
		}
	}
	if e.getEntryHashes != nil {
		if hashes := e.getEntryHashes(); len(hashes) > 0 {
			if payload == nil {
				payload = &gossip_pb.CanaryPayload{}
			}
			payload.EntryHashes = hashes
		}
	}
	if e.getEntryCounts != nil {
		if counts := e.getEntryCounts(); len(counts) > 0 {
			if payload == nil {
				payload = &gossip_pb.CanaryPayload{}
			}
			payload.EntryCounts = counts
		}
	}
	if payload != nil {
		if body, err := proto.Marshal(payload); err == nil {
			msg.Body = body
		}
	}

	if err := msg.WithTimeout(5 * time.Second).Send(sub.Channel()); err != nil {
		if !sub.Channel().IsClosed() {
			pfxlog.Logger().WithError(err).Debug("failed to send canary to subscription controller")
		}
	}
}
