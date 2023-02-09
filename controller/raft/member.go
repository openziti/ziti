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

// MemberModel presents information about and operations on RAFT membership
type MemberModel interface {
	// ListMembers returns the current set of raft members
	ListMembers() ([]*Member, error)
	// HandleJoin adds a node to the raft cluster
	HandleJoin(req *JoinRequest) error
	// HandleRemove removes a node from the raft cluster
	HandleRemove(req *RemoveRequest) error
}

func (self *Controller) ListMembers() ([]*Member, error) {
	configFuture := self.GetRaft().GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return nil, errors.Wrap(err, "failed to get raft configuration")
	}

	var result []*Member

	leaderAddr := self.GetRaft().Leader()

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
					return self.version.Version()
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

func (self *Controller) HandleJoinAsLeader(req *JoinRequest) error {
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
		return errors.Wrap(err, "join failed")
	}

	return nil
}

func (self *Controller) HandleRemoveAsLeader(req *RemoveRequest) error {
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

func (self *Controller) HandleJoin(req *JoinRequest) error {
	if self.IsLeader() {
		return self.HandleJoinAsLeader(req)
	}

	peer, err := self.GetMesh().GetOrConnectPeer(self.GetLeaderAddr(), 5*time.Second)
	if err != nil {
		return err
	}

	msg, err := req.Encode()
	if err != nil {
		return err
	}

	result, err := msg.WithTimeout(5 * time.Second).SendForReply(peer.Channel)
	if err != nil {
		return err
	}

	if result.ContentType == SuccessResponseType {
		return nil
	}

	if result.ContentType == ErrorResponseType {
		return errors.New(string(result.Body))
	}

	return errors.Errorf("unexpected response type %v", result.ContentType)
}

func (self *Controller) HandleRemove(req *RemoveRequest) error {
	if self.IsLeader() {
		return self.HandleRemoveAsLeader(req)
	}

	peer, err := self.GetMesh().GetOrConnectPeer(self.GetLeaderAddr(), 5*time.Second)
	if err != nil {
		return err
	}

	msg, err := req.Encode()
	if err != nil {
		return err
	}

	result, err := msg.WithTimeout(5 * time.Second).SendForReply(peer.Channel)
	if err != nil {
		return err
	}

	if result.ContentType == SuccessResponseType {
		return nil
	}

	if result.ContentType == ErrorResponseType {
		return errors.New(string(result.Body))
	}

	return errors.Errorf("unexpected response type %v", result.ContentType)
}
