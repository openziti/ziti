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

package raft

import (
	"math/rand/v2"
	"time"

	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/sirupsen/logrus"
)

const memberAddressProbeTimeout = 2 * time.Second

// advertiseAddressBackoff paces advertise address reconcile attempts. See runAdvertiseAddressReconcile.
type advertiseAddressBackoff struct {
	minDelay   time.Duration
	maxDelay   time.Duration
	multiplier int
}

var defaultAdvertiseAddressBackoff = advertiseAddressBackoff{
	minDelay:   time.Second,
	maxDelay:   time.Minute,
	multiplier: 2,
}

// advertiseAddressMismatch describes a raft configuration whose entry for this node carries an
// address other than the configured advertise address.
type advertiseAddressMismatch struct {
	stored     raft.ServerAddress
	configured raft.ServerAddress
}

func (self *advertiseAddressMismatch) logger() *logrus.Entry {
	return pfxlog.Logger().
		WithField("storedAddr", self.stored).
		WithField("configuredAddr", self.configured)
}

// getAdvertiseAddressMismatch returns the mismatch between this node's configured advertise address
// and its address in the latest raft configuration, or nil if they match or this node is not a
// member.
func (self *Controller) getAdvertiseAddressMismatch() (*advertiseAddressMismatch, error) {
	cfgFuture := self.Raft.GetConfiguration()
	if err := cfgFuture.Error(); err != nil {
		return nil, err
	}

	localId := raft.ServerID(self.env.GetId().Token)
	configured := self.Mesh.GetAdvertiseAddr()

	for _, srv := range cfgFuture.Configuration().Servers {
		if srv.ID == localId {
			if srv.Address == configured {
				return nil, nil
			}
			return &advertiseAddressMismatch{
				stored:     srv.Address,
				configured: configured,
			}, nil
		}
	}
	return nil, nil
}

// setupAdvertiseAddressReconcile checks this node's configured advertise address against the raft
// configuration and, while they differ, asks the leader to update the stored address each time a
// leader becomes known. Does nothing on a node that hasn't been bootstrapped or joined, since
// bootstrap and join record the configured address.
func (self *Controller) setupAdvertiseAddressReconcile() {
	if self.Raft.LastIndex() == 0 {
		return
	}

	mismatch, err := self.getAdvertiseAddressMismatch()
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to read raft configuration to check advertise address")
	} else if mismatch != nil {
		mismatch.logger().Warn("configured advertise address differs from the address stored in the cluster " +
			"configuration, the stored address will be updated once the cluster has a leader")
	}

	self.RegisterClusterEventHandler(func(evt ClusterEvent, state ClusterState, leaderId string) {
		if evt == ClusterEventLeadershipGained || evt == ClusterEventHasLeader {
			self.triggerAdvertiseAddressReconcile()
		}
	})

	// The event loop may have reported the current leader before the handler was registered.
	if _, leaderId := self.Raft.LeaderWithID(); leaderId != "" {
		self.triggerAdvertiseAddressReconcile()
	}
}

// triggerAdvertiseAddressReconcile starts a loop that updates this node's stored raft address to
// its configured advertise address, or wakes the loop if one is already running. It does not block.
func (self *Controller) triggerAdvertiseAddressReconcile() {
	select {
	case self.advertiseReconcileWake <- struct{}{}:
	default:
	}

	if !self.advertiseReconcileRunning.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer self.advertiseReconcileRunning.Store(false)
		loggedTimeout := false
		runAdvertiseAddressReconcile(func() bool {
			return self.tryReconcileAdvertiseAddress(&loggedTimeout)
		}, defaultAdvertiseAddressBackoff, self.advertiseReconcileWake, self.env.GetCloseNotify())
	}()
}

// runAdvertiseAddressReconcile calls attempt until it returns true or closeNotify is closed. The
// first attempt, and the first after a signal on wake, waits a random interval of up to
// backoff.minDelay. After a failed attempt it waits a random interval between half and all of the
// current delay, which starts at backoff.minDelay and is multiplied after each failure up to
// backoff.maxDelay. A signal on wake cuts that wait short and resets the delay.
func runAdvertiseAddressReconcile(attempt func() bool, backoff advertiseAddressBackoff, wake <-chan struct{}, closeNotify <-chan struct{}) {
	delay := backoff.minDelay
	wait := randomDelay(0, backoff.minDelay)
	for {
		select {
		case <-closeNotify:
			return
		case <-time.After(wait):
		}

		if attempt() {
			return
		}

		select {
		case <-closeNotify:
			return
		case <-wake:
			delay = backoff.minDelay
			wait = randomDelay(0, backoff.minDelay)
		case <-time.After(randomDelay(delay/2, delay)):
			delay = min(delay*time.Duration(backoff.multiplier), backoff.maxDelay)
			wait = 0
		}
	}
}

// randomDelay returns a uniformly random duration in [low, high], or low if high is not above low.
func randomDelay(low, high time.Duration) time.Duration {
	if high <= low {
		return low
	}
	return low + rand.N(high-low+1)
}

// tryReconcileAdvertiseAddress makes one attempt to update this node's stored raft address. It
// returns true once the stored address matches the configured one. The first request timeout is
// logged at Warn and later ones at Debug, since a leader without the update handler never replies.
func (self *Controller) tryReconcileAdvertiseAddress(loggedTimeout *bool) bool {
	mismatch, err := self.getAdvertiseAddressMismatch()
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to read raft configuration to reconcile advertise address")
		return false
	}

	if mismatch == nil {
		return true
	}

	log := mismatch.logger()

	// The request names the address this node believes is stored. Its copy of the configuration may
	// be stale, and the leader refuses a request that its own configuration has overtaken.
	localId := raft.ServerID(self.env.GetId().Token)
	if self.IsLeader() {
		err = self.updateMemberAddressAsLeader(localId, mismatch.stored, mismatch.configured)
	} else {
		err = self.forwardToLeader(&cmd_pb.UpdatePeerAddressRequest{
			Id:       string(localId),
			FromAddr: string(mismatch.stored),
			Addr:     string(mismatch.configured),
		})
	}

	if channel.IsTimeout(err) {
		if !*loggedTimeout {
			log.WithError(err).Warn("request to update advertise address in cluster configuration timed out, " +
				"will retry. The leader may be running a version that does not support address updates")
			*loggedTimeout = true
		} else {
			log.WithError(err).Debug("request to update advertise address in cluster configuration timed out, will retry")
		}
		return false
	}

	if err != nil {
		log.WithError(err).Error("failed to update advertise address in cluster configuration, will retry")
		return false
	}

	// Keep going until the change shows up in this node's copy of the configuration.
	log.Info("requested update of advertise address in cluster configuration")
	return false
}
