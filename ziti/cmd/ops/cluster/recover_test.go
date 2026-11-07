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
	"time"

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
	r.NotEmpty(snaps, "expected recoverDataDir to produce at least one snapshot")

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

func TestRecoverDataDir_RecoveryIndex(t *testing.T) {
	const survivorID = raft.ServerID("survivor")
	const survivorAddr = raft.ServerAddress("127.0.0.1:6262")

	bootstrapServers := []raft.Server{
		{ID: survivorID, Address: survivorAddr},
		{ID: "dead1", Address: "127.0.0.1:6263"},
	}
	// Quorum is lost right after a membership change, so the log ends with it.
	entries := []*raft.Log{
		{Index: 2, Term: 1, Type: raft.LogNoop},
		{Index: 3, Term: 1, Type: raft.LogConfiguration, Data: raft.EncodeConfiguration(raft.Configuration{
			Servers: append(bootstrapServers, raft.Server{ID: "dead2", Address: "127.0.0.1:6264"}),
		})},
	}

	t.Run("the recovered configuration is logged above the last configuration entry", func(t *testing.T) {
		req := require.New(t)

		// arrange
		dataDir := t.TempDir()
		req.NoError(writeRaftState(dataDir, survivorID, survivorAddr, bootstrapServers, entries))

		// act
		err := recoverDataDir(dataDir, survivorID, survivorAddr)

		// assert
		req.NoError(err)
		entry, err := lastLogEntry(dataDir)
		req.NoError(err)
		req.Equal(uint64(4), entry.Index)
		req.Equal(uint64(1), entry.Term)
		req.Equal(raft.LogConfiguration, entry.Type)
		req.Equal([]raft.Server{{ID: survivorID, Address: survivorAddr, Suffrage: raft.Voter}},
			raft.DecodeConfiguration(entry.Data).Servers)
	})

	t.Run("the controller's member list takes the recovered configuration's index once raft commits it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		dataDir := t.TempDir()
		req.NoError(writeRaftState(dataDir, survivorID, survivorAddr, bootstrapServers, entries))
		req.NoError(recoverDataDir(dataDir, survivorID, survivorAddr))

		node, fsm, err := startRaft(t, dataDir, survivorID, survivorAddr)
		req.NoError(err)
		select {
		case isLeader := <-node.LeaderCh():
			req.True(isLeader)
		case <-time.After(5 * time.Second):
			req.Fail("the recovered node did not become leader")
		}

		// act: wait until the fsm has applied everything raft has committed
		err = node.Barrier(5 * time.Second).Error()

		// assert
		req.NoError(err)
		state := fsm.GetCachedServers()
		req.Equal(uint64(4), state.Index)
		req.Equal([]raft.Server{{ID: survivorID, Address: survivorAddr, Suffrage: raft.Voter}}, state.Servers)
	})

	t.Run("raft accepts a membership change conditional on the recovered configuration's index", func(t *testing.T) {
		req := require.New(t)

		// arrange
		dataDir := t.TempDir()
		req.NoError(writeRaftState(dataDir, survivorID, survivorAddr, bootstrapServers, entries))
		req.NoError(recoverDataDir(dataDir, survivorID, survivorAddr))
		recovered, err := lastLogEntry(dataDir)
		req.NoError(err)

		node, _, err := startRaft(t, dataDir, survivorID, survivorAddr)
		req.NoError(err)
		select {
		case isLeader := <-node.LeaderCh():
			req.True(isLeader)
		case <-time.After(5 * time.Second):
			req.Fail("the recovered node did not become leader")
		}

		// act
		err = node.AddNonvoter("new", "127.0.0.1:6265", recovered.Index, 5*time.Second).Error()

		// assert
		req.NoError(err)
		cfgFuture := node.GetConfiguration()
		req.NoError(cfgFuture.Error())
		req.Len(cfgFuture.Configuration().Servers, 2)
		req.Greater(node.LastIndex(), recovered.Index)
	})
}

// writeRaftState bootstraps a cluster of servers in dataDir, as localID at localAddr, and appends
// entries to its log.
func writeRaftState(dataDir string, localID raft.ServerID, localAddr raft.ServerAddress, servers []raft.Server, entries []*raft.Log) error {
	boltStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
	if err != nil {
		return err
	}
	defer func() { _ = boltStore.Close() }()

	snapStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, raft.DefaultConfig().Logger)
	if err != nil {
		return err
	}

	conf := raft.DefaultConfig()
	conf.LocalID = localID
	_, transport := raft.NewInmemTransport(localAddr)
	if err = raft.BootstrapCluster(conf, boltStore, boltStore, snapStore, transport, raft.Configuration{Servers: servers}); err != nil {
		return err
	}
	return boltStore.StoreLogs(entries)
}

// lastLogEntry returns the last entry in the raft log in dataDir.
func lastLogEntry(dataDir string) (*raft.Log, error) {
	boltStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = boltStore.Close() }()

	lastIndex, err := boltStore.LastIndex()
	if err != nil {
		return nil, err
	}
	entry := &raft.Log{}
	if err = boltStore.GetLog(lastIndex, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// startRaft starts a raft node as localID on the stores in dataDir, with short timeouts, and returns it
// with its fsm. Both are closed at test cleanup.
func startRaft(t *testing.T, dataDir string, localID raft.ServerID, localAddr raft.ServerAddress) (*raft.Raft, *zitiraft.BoltDbFsm, error) {
	boltStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { _ = boltStore.Close() })

	snapStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, raft.DefaultConfig().Logger)
	if err != nil {
		return nil, nil, err
	}

	fsm := zitiraft.NewFsm(dataDir, false, command.GetDefaultDecoders(), zitiraft.NewIndexTracker(), event.DispatcherMock{})
	if err = fsm.Init(); err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { _ = fsm.Close() })

	conf := raft.DefaultConfig()
	conf.LocalID = localID
	conf.NoSnapshotRestoreOnStart = true
	conf.HeartbeatTimeout = 50 * time.Millisecond
	conf.ElectionTimeout = 50 * time.Millisecond
	conf.LeaderLeaseTimeout = 50 * time.Millisecond
	conf.CommitTimeout = 5 * time.Millisecond
	_, transport := raft.NewInmemTransport(localAddr)

	node, err := raft.NewRaft(conf, fsm, boltStore, boltStore, snapStore, transport)
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { _ = node.Shutdown().Error() })
	return node, fsm, nil
}
