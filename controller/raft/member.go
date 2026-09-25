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
	"fmt"
	"time"

	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/controller/apierror"

	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Member describes a single member of the raft cluster as seen from the local controller.
type Member struct {
	Id              string `json:"id"`
	Addr            string `json:"addr"`
	Voter           bool   `json:"isVoter"`
	Leader          bool   `json:"isLeader"`
	Version         string `json:"version"`
	Connected       bool   `json:"isConnected"`
	PreferredLeader bool   `json:"isPreferredLeader"`
	// IsSelf is true when this Member entry describes the local controller.
	// RaftConnCount is not meaningful for self (there is no local raft channel
	// to ourselves) and will be zero in that case — consumers should consult
	// IsSelf to distinguish "self" from "peer with zero raft conns".
	IsSelf        bool `json:"isSelf"`
	RaftConnCount int  `json:"raftConnCount"`
}

func (self *Controller) ListMembers() ([]*Member, error) {
	configFuture := self.GetRaft().GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return nil, errors.Wrap(err, "failed to get raft configuration")
	}

	var result []*Member

	leaderAddr, _ := self.GetRaft().LeaderWithID()

	peers := self.GetMesh().GetPeers()

	memberSet := make(map[string]bool)

	for _, srv := range configFuture.Configuration().Servers {
		memberSet[string(srv.Address)] = true

		version := "<not connected>"
		connected := false
		preferredLeader := false
		raftConnCount := 0
		isSelf := false
		if string(srv.ID) == self.env.GetId().Token {
			version = self.env.GetVersionProvider().Version()
			connected = true
			preferredLeader = self.Config.PreferredLeader
			isSelf = true
		} else if peer, exists := peers[string(srv.Address)]; exists {
			version = peer.Version.Version
			connected = true
			preferredLeader = peer.PreferredLeader
			raftConnCount = len(peer.RaftConns.AsMap())
		}

		result = append(result, &Member{
			Id:              string(srv.ID),
			Addr:            string(srv.Address),
			Voter:           srv.Suffrage == raft.Voter,
			Leader:          srv.Address == leaderAddr,
			Version:         version,
			Connected:       connected,
			PreferredLeader: preferredLeader,
			IsSelf:          isSelf,
			RaftConnCount:   raftConnCount,
		})
	}

	for addr, peer := range peers {
		if _, exists := memberSet[addr]; exists {
			continue
		}
		result = append(result, &Member{
			Id:              string(peer.Id),
			Addr:            peer.Address,
			Voter:           false,
			Leader:          peer.Address == string(leaderAddr),
			Version:         peer.Version.Version,
			Connected:       true,
			PreferredLeader: peer.PreferredLeader,
			RaftConnCount:   len(peer.RaftConns.AsMap()),
		})
	}

	return result, nil
}

func (self *Controller) HandleAddPeerAsLeader(req *cmd_pb.AddPeerRequest) error {
	if _, err := transport.ParseAddress(req.Addr); err != nil {
		return fmt.Errorf("unsupported peer address format '%s'", req.Addr)
	}

	r := self.GetRaft()

	configFuture := r.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return errors.Wrap(err, "failed to get raft configuration")
	}

	// A request naming an existing member at a new address, with unchanged suffrage, moves it.
	for _, srv := range configFuture.Configuration().Servers {
		if string(srv.ID) == req.Id && string(srv.Address) != req.Addr && (srv.Suffrage == raft.Voter) == req.IsVoter {
			return self.UpdateMemberAddressAsLeader(srv.ID, "", raft.ServerAddress(req.Addr))
		}
	}

	peerId, peerAddr, err := self.Mesh.GetPeerInfo(req.Addr, 15*time.Second)
	if err != nil {
		// GetPeerInfo reuses an already-established connection if one exists, so reaching
		// this point means the joining node is neither connected nor dialable from here.
		// A node with no inbound ports open (e.g. behind a firewall) cannot be dialed by
		// the leader, and must instead initiate the join itself so the leader can reuse the
		// inbound connection. Surface that guidance rather than an opaque dial timeout.
		return errors.Wrapf(err, "unable to reach peer at '%s' to add it to the cluster; "+
			"if this node has no inbound ports open (e.g. behind a firewall), run the join from the "+
			"joining node instead ('ziti agent cluster add -i <joining-node> <joining-node-advertise-addr>')", req.Addr)
	}

	id := peerId
	addr := peerAddr

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == id && srv.Address != addr && (srv.Suffrage == raft.Voter) == req.IsVoter {
			return self.UpdateMemberAddressAsLeader(id, "", addr)
		}
	}

	for _, srv := range configFuture.Configuration().Servers {
		// If a node already exists with either the joining node's ID or address,
		// that node may need to be removed from the config first.
		if srv.ID == id || srv.Address == addr {
			// However, if *both* the ID and the address are the same, then nothing -- not even
			// a join operation -- is needed.
			if srv.ID == id && srv.Address == addr && ((srv.Suffrage == raft.Voter) == req.IsVoter) {
				logrus.Infof("node %s at %s already member of cluster matching request, ignoring join request", id, addr)
				return nil
			}

			future := r.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return errors.Wrapf(err, "error removing existing node %s at %s", id, addr)
			}
		}
	}

	var f raft.IndexFuture
	if req.IsVoter {
		f = r.AddVoter(id, addr, 0, 0)
	} else {
		f = r.AddNonvoter(id, addr, 0, 0)
	}

	if err := f.Error(); err != nil {
		return errors.Wrap(err, "add peer failed")
	}

	return nil
}

// UpdateMemberAddressAsLeader changes the raft address of an existing member in place, keeping its
// suffrage. It succeeds without change if the member is already at addr. It fails if id is not a
// member, if another member is stored at addr, or if fromAddr is non-empty and the member is stored
// at an address other than fromAddr. Unless id is this node, addr is dialed first and must present
// id. The change is conditioned on the configuration index it was checked against, so a concurrent
// membership change makes it fail rather than apply. Must be called on the leader.
func (self *Controller) UpdateMemberAddressAsLeader(id raft.ServerID, fromAddr, addr raft.ServerAddress) error {
	r := self.GetRaft()

	configuration, configIndex, err := self.getLatestConfiguration()
	if err != nil {
		return errors.Wrap(err, "failed to get raft configuration")
	}

	var member *raft.Server
	for _, srv := range configuration.Servers {
		if srv.ID == id {
			member = &srv
		} else if srv.Address == addr {
			return errors.Errorf("unable to move member %s to address %s, address belongs to member %s", id, addr, srv.ID)
		}
	}

	if member == nil {
		return errors.Errorf("unable to update address of %s, not a cluster member", id)
	}

	if member.Address == addr {
		return nil
	}

	if fromAddr != "" && member.Address != fromAddr {
		return errors.Errorf("unable to move member %s from address %s, it is stored at %s", id, fromAddr, member.Address)
	}

	if string(id) != self.env.GetId().Token {
		probedId, _, err := self.Mesh.ProbePeer(string(addr), memberAddressProbeTimeout)
		if err != nil {
			return errors.Wrapf(err, "unable to reach member %s at new address %s", id, addr)
		}
		if probedId != id {
			return errors.Errorf("unable to move member %s to address %s, address is served by %s", id, addr, probedId)
		}
	}

	var f raft.IndexFuture
	if member.Suffrage == raft.Voter {
		f = r.AddVoter(id, addr, configIndex, 0)
	} else {
		f = r.AddNonvoter(id, addr, configIndex, 0)
	}

	if err := f.Error(); err != nil {
		return errors.Wrapf(err, "failed to update address of member %s", id)
	}

	logrus.WithField("memberId", id).
		WithField("oldAddr", member.Address).
		WithField("newAddr", addr).
		Info("updated cluster member address")

	return nil
}

// getLatestConfiguration returns the latest raft configuration, committed or not, with the index of
// the log entry that set it, read from the raft log and snapshot stores. Unlike
// raft.Raft.GetConfiguration, whose Index is always zero, the index can be passed as prevIndex to
// make a membership change conditional. An entry appended after the read makes the pair stale, which
// a conditional change then rejects.
func (self *Controller) getLatestConfiguration() (raft.Configuration, uint64, error) {
	var configuration raft.Configuration
	var configIndex, snapshotIndex uint64

	snapshots, err := self.snapshotStore.List()
	if err != nil {
		return configuration, 0, errors.Wrap(err, "unable to list raft snapshots")
	}
	if len(snapshots) > 0 {
		configuration, configIndex = snapshots[0].Configuration, snapshots[0].ConfigurationIndex
		snapshotIndex = snapshots[0].Index
	}

	firstIndex, err := self.logStore.FirstIndex()
	if err != nil {
		return configuration, 0, errors.Wrap(err, "unable to read first raft log index")
	}
	lastIndex, err := self.logStore.LastIndex()
	if err != nil {
		return configuration, 0, errors.Wrap(err, "unable to read last raft log index")
	}

	// Only entries above the snapshot can supersede its configuration. Retained entries below it may
	// be separated from newer ones by a gap the snapshot covers, e.g. after a snapshot install.
	for idx := lastIndex; idx > snapshotIndex && idx >= firstIndex && idx > 0; idx-- {
		entry := &raft.Log{}
		if err = self.logStore.GetLog(idx, entry); err != nil {
			return configuration, 0, errors.Wrapf(err, "unable to read raft log entry %d", idx)
		}
		if entry.Type == raft.LogConfiguration {
			return raft.DecodeConfiguration(entry.Data), idx, nil
		}
	}

	if configIndex == 0 {
		return configuration, 0, errors.New("no raft configuration found")
	}
	return configuration, configIndex, nil
}

// HandleUpdatePeerAddress moves an existing member to a new raft address, keeping its suffrage. See
// UpdateMemberAddressAsLeader. Forwarded to the leader when this node isn't the leader.
func (self *Controller) HandleUpdatePeerAddress(req *cmd_pb.UpdatePeerAddressRequest) error {
	if self.IsLeader() {
		if _, err := transport.ParseAddress(req.Addr); err != nil {
			return fmt.Errorf("unsupported peer address format '%s'", req.Addr)
		}
		return self.UpdateMemberAddressAsLeader(raft.ServerID(req.Id), raft.ServerAddress(req.FromAddr), raft.ServerAddress(req.Addr))
	}
	return self.forwardToLeader(req)
}

func (self *Controller) HandleRemovePeerAsLeader(req *cmd_pb.RemovePeerRequest) error {
	r := self.GetRaft()

	configFuture := r.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return errors.Wrap(err, "failed to get raft configuration")
	}

	id := raft.ServerID(req.Id)

	future := r.RemoveServer(id, 0, 0)
	if err := future.Error(); err != nil {
		return errors.Wrapf(err, "error removing existing node %s", id)
	}
	return nil
}

func (self *Controller) HandleTransferLeadershipAsLeader(req *cmd_pb.TransferLeadershipRequest) error {
	r := self.GetRaft()

	var future raft.Future
	if req.Id == "" {
		future = r.LeadershipTransfer()
	} else {
		configFuture := r.GetConfiguration()
		if err := configFuture.Error(); err != nil {
			return errors.Wrap(err, "failed to get raft configuration")
		}

		var targetServer *raft.Server
		for _, v := range configFuture.Configuration().Servers {
			if v.ID == raft.ServerID(req.Id) {
				targetServer = &v
				break
			}
		}
		if targetServer == nil {
			return errors.Errorf("no cluster node found with id %v", req.Id)
		}

		if targetServer.Suffrage != raft.Voter {
			return errors.Errorf("cluster node %v is not a voting member", req.Id)
		}

		future = r.LeadershipTransferToServer(targetServer.ID, targetServer.Address)
	}

	if err := future.Error(); err != nil {
		return errors.Wrapf(err, "error transferring leadership")
	}
	return nil
}

func (self *Controller) HandleAddPeer(req *cmd_pb.AddPeerRequest) error {
	if self.IsLeader() {
		return self.HandleAddPeerAsLeader(req)
	}
	return self.forwardToLeader(req)
}

func (self *Controller) HandleRemovePeer(req *cmd_pb.RemovePeerRequest) error {
	if self.IsLeader() {
		return self.HandleRemovePeerAsLeader(req)
	}
	return self.forwardToLeader(req)
}

func (self *Controller) HandleTransferLeadership(req *cmd_pb.TransferLeadershipRequest) error {
	if self.IsLeader() {
		return self.HandleTransferLeadershipAsLeader(req)
	}
	return self.forwardToLeader(req)
}

func (self *Controller) forwardToLeader(req protobufs.TypedMessage) error {
	leader := self.GetLeaderAddr()
	if leader == "" {
		return apierror.NewClusterHasNoLeaderError()
	}

	return self.ForwardToAddr(leader, req)
}

func (self *Controller) ForwardToAddr(addr string, req protobufs.TypedMessage) error {
	peer, err := self.GetMesh().GetOrConnectPeer(addr, 5*time.Second)
	if err != nil {
		return err
	}

	result, err := protobufs.MarshalTyped(req).WithTimeout(5 * time.Second).SendForReply(peer.Channel)
	if err != nil {
		return err
	}

	if result.ContentType == int32(cmd_pb.ContentType_SuccessResponseType) {
		return nil
	}

	if result.ContentType == int32(cmd_pb.ContentType_ErrorResponseType) {
		return errors.New(string(result.Body))
	}

	return errors.Errorf("unexpected response type %v", result.ContentType)
}
