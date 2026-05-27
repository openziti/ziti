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

package cluster

import (
	"path/filepath"
	"testing"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/event"
	zitiraft "github.com/openziti/ziti/v2/controller/raft"
	"github.com/stretchr/testify/require"
)

func TestRecoverDataDir_ReducesMultiNodeConfigToSingleNode(t *testing.T) {
	r := require.New(t)
	dataDir := t.TempDir()

	const survivorID = raft.ServerID("survivor")
	const survivorAddr = raft.ServerAddress("127.0.0.1:6262")
	const deadID = raft.ServerID("dead")
	const deadAddr = raft.ServerAddress("127.0.0.1:6263")

	bootstrap := func() {
		boltStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
		r.NoError(err)
		defer func() { _ = boltStore.Close() }()

		snapStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, raft.DefaultConfig().Logger)
		r.NoError(err)

		conf := raft.DefaultConfig()
		conf.LocalID = survivorID
		_, transport := raft.NewInmemTransport(survivorAddr)

		err = raft.BootstrapCluster(conf, boltStore, boltStore, snapStore, transport, raft.Configuration{
			Servers: []raft.Server{
				{ID: survivorID, Address: survivorAddr},
				{ID: deadID, Address: deadAddr},
			},
		})
		r.NoError(err)
	}
	bootstrap()

	r.NoError(recoverDataDir(dataDir, survivorID, survivorAddr))

	snapStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, raft.DefaultConfig().Logger)
	r.NoError(err)

	snaps, err := snapStore.List()
	r.NoError(err)
	r.NotEmpty(snaps, "expected RecoverCluster to produce at least one snapshot")

	latest := snaps[0]
	r.Equal(1, len(latest.Configuration.Servers), "post-recovery configuration should have a single server")
	r.Equal(survivorID, latest.Configuration.Servers[0].ID)
	r.Equal(survivorAddr, latest.Configuration.Servers[0].Address)

	// The FSM-tracked servers list in ctrl-ha.db must also reflect the recovered
	// configuration so the controller does not keep accepting reconnects from
	// the removed peer via IsPeerMember on next startup.
	fsm := zitiraft.NewFsm(dataDir, false, command.GetDefaultDecoders(), zitiraft.NewIndexTracker(), event.DispatcherMock{})
	r.NoError(fsm.Init())
	defer func() { _ = fsm.Close() }()

	state := fsm.GetCachedServers()
	r.NotNil(state, "FSM should have loaded the servers list from ctrl-ha.db")
	r.Equal(1, len(state.Servers), "FSM-tracked servers should reflect the recovered single-node config")
	r.Equal(survivorID, state.Servers[0].ID)
	r.Equal(survivorAddr, state.Servers[0].Address)
}

// TestRecoverDataDir_GetCurrentStateAfterRaftStart simulates the production
// startup path: post-recovery, the controller boots a real raft instance
// against the recovered stores, then 'getClusterPeersForEvent' calls
// Fsm.GetCurrentState(raft) to build clusterEvent.Peers. That peers list
// drives DeleteRemovedPeers in broker.AcceptClusterEvent. This test ensures
// the chain returns exactly the recovered single-server configuration so
// DeleteRemovedPeers cannot accidentally prune (or fail to prune) the wrong
// rows.
func TestRecoverDataDir_GetCurrentStateAfterRaftStart(t *testing.T) {
	r := require.New(t)
	dataDir := t.TempDir()

	const survivorID = raft.ServerID("survivor")
	const survivorAddr = raft.ServerAddress("127.0.0.1:7262")
	const deadID = raft.ServerID("dead")
	const deadAddr = raft.ServerAddress("127.0.0.1:7263")

	bootstrap := func() {
		boltStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
		r.NoError(err)
		defer func() { _ = boltStore.Close() }()

		snapStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, raft.DefaultConfig().Logger)
		r.NoError(err)

		conf := raft.DefaultConfig()
		conf.LocalID = survivorID
		_, transport := raft.NewInmemTransport(survivorAddr)

		err = raft.BootstrapCluster(conf, boltStore, boltStore, snapStore, transport, raft.Configuration{
			Servers: []raft.Server{
				{ID: survivorID, Address: survivorAddr},
				{ID: deadID, Address: deadAddr},
			},
		})
		r.NoError(err)
	}
	bootstrap()

	r.NoError(recoverDataDir(dataDir, survivorID, survivorAddr))

	// Simulate the controller restarting against the recovered stores.
	boltStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
	r.NoError(err)
	defer func() { _ = boltStore.Close() }()

	snapStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, raft.DefaultConfig().Logger)
	r.NoError(err)

	fsm := zitiraft.NewFsm(dataDir, false, command.GetDefaultDecoders(), zitiraft.NewIndexTracker(), event.DispatcherMock{})
	r.NoError(fsm.Init())
	defer func() { _ = fsm.Close() }()

	conf := raft.DefaultConfig()
	conf.LocalID = survivorID
	conf.NoSnapshotRestoreOnStart = true
	_, transport := raft.NewInmemTransport(survivorAddr)

	rNode, err := raft.NewRaft(conf, fsm, boltStore, boltStore, snapStore, transport)
	r.NoError(err)
	defer func() { _ = rNode.Shutdown().Error() }()

	// raft.GetConfiguration() should match the recovered single-server config
	// regardless of leadership state — it reads from the snapshot meta.
	cfgFuture := rNode.GetConfiguration()
	r.NoError(cfgFuture.Error())
	cfg := cfgFuture.Configuration()
	r.Equal(1, len(cfg.Servers), "raft.GetConfiguration() should report the recovered single-server config")
	r.Equal(survivorID, cfg.Servers[0].ID)
	r.Equal(survivorAddr, cfg.Servers[0].Address)

	// This is the exact call getClusterPeersForEvent makes when building
	// clusterEvent.Peers for ClusterLeadershipGained. It must return the
	// survivor only — no dead-peer leakage from any cached state.
	current := fsm.GetCurrentState(rNode)
	r.NotNil(current)
	r.Equal(1, len(current.Servers), "Fsm.GetCurrentState should report the recovered single-server config")
	r.Equal(survivorID, current.Servers[0].ID)
	r.Equal(survivorAddr, current.Servers[0].Address)
}
