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
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/pb/cmd_pb"
	"time"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Member struct {
	Id        string
	Addr      string
	Voter     bool
	Leader    bool
	Version   string
	Connected bool
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
		result = append(result, &Member{
			Id:     string(srv.ID),
			Addr:   string(srv.Address),
			Voter:  srv.Suffrage == raft.Voter,
			Leader: srv.Address == leaderAddr,
			Version: func() string {
				if srv.Address == leaderAddr {
					return self.env.GetVersionProvider().Version()
				}
				if peer, exists := peers[string(srv.Address)]; exists {
					return peer.Version.Version
				}
				return "N/A"
			}(),
			Connected: true,
		})
	}

	for addr, peer := range peers {
		if _, exists := memberSet[addr]; exists {
			continue
		}
		result = append(result, &Member{
			Id:        string(peer.Id),
			Addr:      peer.Address,
			Voter:     false,
			Leader:    peer.Address == string(leaderAddr),
			Version:   peer.Version.Version,
			Connected: true,
		})
	}

	return result, nil
}

func (self *Controller) HandleAddPeerAsLeader(req *cmd_pb.AddPeerRequest) error {
	r := self.GetRaft()

	configFuture := r.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return errors.Wrap(err, "failed to get raft configuration")
	}

	id := raft.ServerID(req.Id)
	addr := raft.ServerAddress(req.Addr)

	for _, srv := range configFuture.Configuration().Servers {
		// If a node already exists with either the joining node's ID or address,
		// that node may need to be removed from the config first.
		if srv.ID == id || srv.Address == addr {
			// However, if *both* the ID and the address are the same, then nothing -- not even
			// a join operation -- is needed.
			if srv.ID == id && srv.Address == addr {
				logrus.Infof("node %s at %s already member of cluster, ignoring join request", id, addr)
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
		return errors.Wrapf(err, "error transfering leadership")
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
	peer, err := self.GetMesh().GetOrConnectPeer(self.GetLeaderAddr(), 5*time.Second)
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
