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
	"io"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/openziti/identity"
	"github.com/stretchr/testify/require"
)

func Test_Controller_CtrlAddresses(t *testing.T) {
	leader := newSingleNodeRaft(t, "ctrl2", true)
	require.Eventually(t, func() bool { return leader.State() == raft.Leader }, 5*time.Second, 10*time.Millisecond)

	t.Run("a leader's list naming the leader is the leader's list", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrl := newCachedListController(leader, "ctrl2", 2414, "ctrl1", "ctrl2", "ctrl3")

		// act
		index, addresses, isLeaderList := ctrl.CtrlAddresses()

		// assert
		req.Equal(uint64(2414), index)
		req.Equal([]string{"tls:ctrl1.example:6262", "tls:ctrl2.example:6262", "tls:ctrl3.example:6262"}, addresses)
		req.True(isLeaderList)
	})

	t.Run("a leader's list omitting the leader is not the leader's list", func(t *testing.T) {
		req := require.New(t)

		// arrange: ctrl2 leads with the member list restored from a snapshot taken before it joined
		ctrl := newCachedListController(leader, "ctrl2", 1796, "ctrl1")

		// act
		index, addresses, isLeaderList := ctrl.CtrlAddresses()

		// assert
		req.Equal(uint64(1796), index)
		req.Equal([]string{"tls:ctrl1.example:6262"}, addresses)
		req.False(isLeaderList, "a list omitting the leader must not go to routers as the leader's list")
	})

	t.Run("a follower's list is not the leader's list", func(t *testing.T) {
		req := require.New(t)

		// arrange
		follower := newSingleNodeRaft(t, "ctrl1", false)
		ctrl := newCachedListController(follower, "ctrl1", 2414, "ctrl1", "ctrl2", "ctrl3")

		// act
		_, _, isLeaderList := ctrl.CtrlAddresses()

		// assert
		req.False(isLeaderList)
	})
}

// newSingleNodeRaft returns an in-memory raft node with id id, shut down at test cleanup. If bootstrap is
// set, it is the only member of its cluster and elects itself; otherwise it has no configuration and stays
// a follower.
func newSingleNodeRaft(t *testing.T, id string, bootstrap bool) *raft.Raft {
	t.Helper()
	conf := raft.DefaultConfig()
	conf.LocalID = raft.ServerID(id)
	conf.HeartbeatTimeout = 50 * time.Millisecond
	conf.ElectionTimeout = 50 * time.Millisecond
	conf.LeaderLeaseTimeout = 50 * time.Millisecond
	conf.CommitTimeout = 5 * time.Millisecond
	conf.LogOutput = io.Discard

	store := raft.NewInmemStore()
	snapshots := raft.NewInmemSnapshotStore()
	addr, transport := raft.NewInmemTransport(raft.ServerAddress(id))
	if bootstrap {
		require.NoError(t, raft.BootstrapCluster(conf, store, store, snapshots, transport, raft.Configuration{
			Servers: []raft.Server{{ID: conf.LocalID, Address: addr}},
		}))
	}

	r, err := raft.NewRaft(conf, &raft.MockFSM{}, store, store, snapshots, transport)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Shutdown().Error() })
	return r
}

// newCachedListController returns a controller with node id id, running r, whose FSM has cached ids at
// index as the cluster member list.
func newCachedListController(r *raft.Raft, id string, index uint64, ids ...string) *Controller {
	fsm := &BoltDbFsm{}
	var servers []raft.Server
	for _, memberId := range ids {
		servers = append(servers, raft.Server{
			ID:       raft.ServerID(memberId),
			Address:  raft.ServerAddress("tls:" + memberId + ".example:6262"),
			Suffrage: raft.Voter,
		})
	}
	fsm.currentState.Store(&ServersWithIndex{Servers: servers, Index: index})
	return &Controller{env: idEnv{id: id}, Raft: r, Fsm: fsm}
}

// idEnv is an Env that knows only its node id.
type idEnv struct {
	Env
	id string
}

func (e idEnv) GetId() *identity.TokenId {
	return &identity.TokenId{Token: e.id}
}
