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
	"time"

	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/sirupsen/logrus"
)

const (
	memberAddressProbeTimeout      = 3 * time.Second
	advertiseAddressRetryMinDelay  = time.Second
	advertiseAddressRetryMaxDelay  = time.Minute
	advertiseAddressRetryDelayMult = 2
)

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
			go self.reconcileAdvertiseAddress()
		}
	})

	// The event loop may have reported the current leader before the handler was registered.
	if _, leaderId := self.Raft.LeaderWithID(); leaderId != "" {
		go self.reconcileAdvertiseAddress()
	}
}

// reconcileAdvertiseAddress updates this node's stored raft address to its configured advertise
// address, retrying with backoff until the addresses match or the controller shuts down. Only one
// reconcile runs at a time.
func (self *Controller) reconcileAdvertiseAddress() {
	if !self.advertiseReconcileRunning.CompareAndSwap(false, true) {
		return
	}
	defer self.advertiseReconcileRunning.Store(false)

	delay := advertiseAddressRetryMinDelay
	loggedTimeout := false
	for !self.tryReconcileAdvertiseAddress(&loggedTimeout) {
		select {
		case <-self.env.GetCloseNotify():
			return
		case <-time.After(delay):
		}

		delay = min(delay*advertiseAddressRetryDelayMult, advertiseAddressRetryMaxDelay)
	}
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
		err = self.UpdateMemberAddressAsLeader(localId, mismatch.stored, mismatch.configured)
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
