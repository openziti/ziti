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

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/controller/raft/mesh"
	"github.com/stretchr/testify/require"
)

// idEnv is an Env whose only implemented method is GetId.
type idEnv struct {
	Env
	id string
}

func (self *idEnv) GetId() *identity.TokenId {
	return &identity.TokenId{Token: self.id}
}

// newControllerWithServers returns a Controller for node ctrl1 whose FSM holds servers, with ctrl1
// stored at an address other than any it would advertise.
func newControllerWithServers() *Controller {
	fsm := &BoltDbFsm{}
	fsm.currentState.Store(&ServersWithIndex{
		Servers: []raft.Server{
			{ID: "ctrl1", Address: "tls:ctrl1-old.example:6262", Suffrage: raft.Voter},
			{ID: "ctrl2", Address: "tls:ctrl2.example:6262", Suffrage: raft.Voter},
			{ID: "ctrl3", Address: "tls:ctrl3.example:6262", Suffrage: raft.Nonvoter},
		},
		Index: 5,
	})
	fsm.stateInitialized.Store(true)

	return &Controller{
		env: &idEnv{id: "ctrl1"},
		Fsm: fsm,
	}
}

func Test_GetPeerAddresses_ExcludesSelfById(t *testing.T) {
	ctrl := newControllerWithServers()
	require.ElementsMatch(t, []string{"tls:ctrl2.example:6262", "tls:ctrl3.example:6262"}, ctrl.GetPeerAddresses())
}

func Test_GetMemberId(t *testing.T) {
	ctrl := newControllerWithServers()

	id, found := ctrl.GetMemberId("tls:ctrl3.example:6262")
	require.True(t, found)
	require.Equal(t, raft.ServerID("ctrl3"), id)

	id, found = ctrl.GetMemberId("tls:ctrl1-old.example:6262")
	require.True(t, found)
	require.Equal(t, raft.ServerID("ctrl1"), id)

	_, found = ctrl.GetMemberId("tls:unknown.example:6262")
	require.False(t, found)
}

func Test_GetMemberId_BeforeFsmInit(t *testing.T) {
	ctrl := &Controller{Fsm: &BoltDbFsm{}}
	_, found := ctrl.GetMemberId("tls:ctrl1.example:6262")
	require.False(t, found)
}

// probeMesh is a Mesh whose only implemented method is ProbePeer.
type probeMesh struct {
	mesh.Mesh
	probe func(address string) (raft.ServerID, raft.ServerAddress, error)
}

func (self *probeMesh) ProbePeer(address string, _ time.Duration) (raft.ServerID, raft.ServerAddress, error) {
	return self.probe(address)
}

type nopFsm struct{}

func (nopFsm) Apply(*raft.Log) interface{}         { return nil }
func (nopFsm) Snapshot() (raft.FSMSnapshot, error) { return nopSnapshot{}, nil }
func (nopFsm) Restore(rc io.ReadCloser) error      { return rc.Close() }

type nopSnapshot struct{}

func (nopSnapshot) Persist(sink raft.SnapshotSink) error { return sink.Close() }
func (nopSnapshot) Release()                             {}

// newTestLeader starts an in-memory raft whose leader is ctrl1, with ctrl2 and ctrl3 as non-voters
// so that configuration changes commit on ctrl1's vote alone. It returns a Controller for ctrl1
// whose mesh probes the given address by calling probe.
func newTestLeader(t *testing.T, probe func(address string) (raft.ServerID, raft.ServerAddress, error)) *Controller {
	conf := raft.DefaultConfig()
	conf.LocalID = "ctrl1"
	conf.HeartbeatTimeout = 50 * time.Millisecond
	conf.ElectionTimeout = 50 * time.Millisecond
	conf.LeaderLeaseTimeout = 50 * time.Millisecond
	conf.CommitTimeout = 5 * time.Millisecond
	conf.TrailingLogs = 0
	conf.Logger = hclog.NewNullLogger()

	logs := raft.NewInmemStore()
	snapshots := raft.NewInmemSnapshotStore()
	_, transport := raft.NewInmemTransport("tls:ctrl1.example:6262")

	require.NoError(t, raft.BootstrapCluster(conf, logs, logs, snapshots, transport, raft.Configuration{
		Servers: []raft.Server{
			{ID: "ctrl1", Address: "tls:ctrl1.example:6262", Suffrage: raft.Voter},
			{ID: "ctrl2", Address: "tls:ctrl2.example:6262", Suffrage: raft.Nonvoter},
			{ID: "ctrl3", Address: "tls:ctrl3.example:6262", Suffrage: raft.Nonvoter},
		},
	}))

	r, err := raft.NewRaft(conf, nopFsm{}, logs, logs, snapshots, transport)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = r.Shutdown().Error()
	})

	require.Eventually(t, func() bool {
		return r.State() == raft.Leader
	}, 5*time.Second, 10*time.Millisecond)

	return &Controller{
		env:           &idEnv{id: "ctrl1"},
		Raft:          r,
		Mesh:          &probeMesh{probe: probe},
		logStore:      logs,
		snapshotStore: snapshots,
	}
}

func requireServer(t *testing.T, ctrl *Controller, id raft.ServerID) raft.Server {
	cfgFuture := ctrl.Raft.GetConfiguration()
	require.NoError(t, cfgFuture.Error())
	for _, srv := range cfgFuture.Configuration().Servers {
		if srv.ID == id {
			return srv
		}
	}
	require.Failf(t, "member not found", "%s", id)
	return raft.Server{}
}

func Test_getLatestConfiguration(t *testing.T) {
	ctrl := newTestLeader(t, nil)

	requireLatest := func(expectedIndex uint64) {
		configuration, index, err := ctrl.getLatestConfiguration()
		require.NoError(t, err)
		require.Equal(t, expectedIndex, index)
		require.Equal(t, ctrl.Raft.GetConfiguration().Configuration(), configuration)
	}

	requireLatest(1)

	f := ctrl.Raft.AddNonvoter("ctrl3", "tls:ctrl3-new.example:6262", 0, 0)
	require.NoError(t, f.Error())
	requireLatest(f.Index())

	// With no trailing logs, the snapshot compacts the log and the configuration comes from the
	// snapshot's metadata.
	require.NoError(t, ctrl.Raft.Barrier(0).Error())
	require.NoError(t, ctrl.Raft.Snapshot().Error())
	requireLatest(f.Index())

	f = ctrl.Raft.AddNonvoter("ctrl2", "tls:ctrl2-new.example:6262", 0, 0)
	require.NoError(t, f.Error())
	requireLatest(f.Index())
}

func Test_UpdateMemberAddressAsLeader(t *testing.T) {
	const newAddr = raft.ServerAddress("tls:ctrl3-new.example:6262")
	probeCtrl3 := func(string) (raft.ServerID, raft.ServerAddress, error) {
		return "ctrl3", newAddr, nil
	}

	t.Run("moves the member and keeps its suffrage", func(t *testing.T) {
		ctrl := newTestLeader(t, probeCtrl3)
		require.NoError(t, ctrl.UpdateMemberAddressAsLeader("ctrl3", "tls:ctrl3.example:6262", newAddr))
		srv := requireServer(t, ctrl, "ctrl3")
		require.Equal(t, newAddr, srv.Address)
		require.Equal(t, raft.Nonvoter, srv.Suffrage)
	})

	t.Run("refuses a member that is not in the cluster", func(t *testing.T) {
		ctrl := newTestLeader(t, probeCtrl3)
		require.ErrorContains(t, ctrl.UpdateMemberAddressAsLeader("ctrl4", "", newAddr), "not a cluster member")
	})

	t.Run("refuses an address stored for another member", func(t *testing.T) {
		ctrl := newTestLeader(t, probeCtrl3)
		err := ctrl.UpdateMemberAddressAsLeader("ctrl3", "", "tls:ctrl2.example:6262")
		require.ErrorContains(t, err, "address belongs to member ctrl2")
	})

	t.Run("refuses a request whose from address is not the stored address", func(t *testing.T) {
		ctrl := newTestLeader(t, probeCtrl3)
		err := ctrl.UpdateMemberAddressAsLeader("ctrl3", "tls:ctrl3-other.example:6262", newAddr)
		require.ErrorContains(t, err, "it is stored at tls:ctrl3.example:6262")
		require.Equal(t, raft.ServerAddress("tls:ctrl3.example:6262"), requireServer(t, ctrl, "ctrl3").Address)
	})

	t.Run("refuses when a different member answers at the new address", func(t *testing.T) {
		ctrl := newTestLeader(t, func(string) (raft.ServerID, raft.ServerAddress, error) {
			return "ctrl9", newAddr, nil
		})
		require.ErrorContains(t, ctrl.UpdateMemberAddressAsLeader("ctrl3", "", newAddr), "address is served by ctrl9")
	})

	t.Run("does not undo a removal made during the probe", func(t *testing.T) {
		var ctrl *Controller
		ctrl = newTestLeader(t, func(string) (raft.ServerID, raft.ServerAddress, error) {
			require.NoError(t, ctrl.Raft.RemoveServer("ctrl3", 0, 0).Error())
			return "ctrl3", newAddr, nil
		})
		require.ErrorContains(t, ctrl.UpdateMemberAddressAsLeader("ctrl3", "", newAddr), "configuration changed since")
		for _, srv := range ctrl.Raft.GetConfiguration().Configuration().Servers {
			require.NotEqual(t, raft.ServerID("ctrl3"), srv.ID, "removed member was re-added")
		}
	})

	t.Run("does not apply over another membership change made during the probe", func(t *testing.T) {
		var ctrl *Controller
		ctrl = newTestLeader(t, func(string) (raft.ServerID, raft.ServerAddress, error) {
			require.NoError(t, ctrl.Raft.AddNonvoter("ctrl2", "tls:ctrl2-new.example:6262", 0, 0).Error())
			return "ctrl3", newAddr, nil
		})
		require.ErrorContains(t, ctrl.UpdateMemberAddressAsLeader("ctrl3", "", newAddr), "configuration changed since")
		require.Equal(t, raft.ServerAddress("tls:ctrl3.example:6262"), requireServer(t, ctrl, "ctrl3").Address)
		require.Equal(t, raft.ServerAddress("tls:ctrl2-new.example:6262"), requireServer(t, ctrl, "ctrl2").Address)
	})
}

// Test_getLatestConfiguration_LogGapBelowSnapshot covers a log store holding old entries, a gap
// covered by the snapshot, and newer entries, as left by installing a snapshot from the leader.
func Test_getLatestConfiguration_LogGapBelowSnapshot(t *testing.T) {
	oldConfig := raft.Configuration{Servers: []raft.Server{
		{ID: "ctrl1", Address: "tls:ctrl1.example:6262", Suffrage: raft.Voter},
	}}
	snapshotConfig := raft.Configuration{Servers: []raft.Server{
		{ID: "ctrl1", Address: "tls:ctrl1.example:6262", Suffrage: raft.Voter},
		{ID: "ctrl2", Address: "tls:ctrl2.example:6262", Suffrage: raft.Voter},
	}}

	logs := raft.NewInmemStore()
	require.NoError(t, logs.StoreLogs([]*raft.Log{
		{Index: 1, Term: 1, Type: raft.LogConfiguration, Data: raft.EncodeConfiguration(oldConfig)},
		{Index: 2, Term: 1, Type: raft.LogCommand},
		{Index: 3, Term: 1, Type: raft.LogCommand},
		{Index: 11, Term: 2, Type: raft.LogCommand},
		{Index: 12, Term: 2, Type: raft.LogCommand},
	}))

	snapshots := raft.NewInmemSnapshotStore()
	_, transport := raft.NewInmemTransport("tls:ctrl1.example:6262")
	sink, err := snapshots.Create(raft.SnapshotVersionMax, 10, 2, snapshotConfig, 8, transport)
	require.NoError(t, err)
	require.NoError(t, sink.Close())

	ctrl := &Controller{logStore: logs, snapshotStore: snapshots}
	configuration, index, err := ctrl.getLatestConfiguration()
	require.NoError(t, err)
	require.Equal(t, uint64(8), index)
	require.Equal(t, snapshotConfig, configuration)
}
