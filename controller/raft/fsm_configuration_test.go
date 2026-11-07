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
	"sync"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/stretchr/testify/require"
)

func Test_BoltDbFsm_adoptConfiguration(t *testing.T) {
	t.Run("an adopted configuration saves its servers but not the raft index", func(t *testing.T) {
		req := require.New(t)

		// arrange: the db was restored from a snapshot taken while ctrl1 was the only member
		dataDir := t.TempDir()
		fsm, dispatcher := newConfigurationTestFsmIn(t, dataDir, 1796, "ctrl1")

		// act
		fsm.dropConfigurationIndex()
		fsm.adoptConfiguration(2414, configurationOf("ctrl1", "ctrl2", "ctrl3"), false)

		// assert
		state := fsm.GetCachedServers()
		req.Equal(uint64(2414), state.Index, "the list must carry the index of the configuration entry")
		req.ElementsMatch(members("ctrl1", "ctrl2", "ctrl3"), state.Servers)

		dbIndex, err := fsm.loadCurrentIndex()
		req.NoError(err)
		req.Equal(uint64(1796), dbIndex, "the db's raft index must not move, or replay skips the commands below the configuration entry")
		req.Empty(dispatcher.events(), "an adoption without announce must not dispatch an event")

		req.NoError(fsm.Close())
		reopened, _ := newConfigurationTestFsmIn(t, dataDir, 1796)
		req.ElementsMatch(members("ctrl1", "ctrl2", "ctrl3"), reopened.GetCachedServers().Servers,
			"the adopted configuration's servers must be saved")
	})

	t.Run("replaying configuration entries at or below its index changes nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		fsm, dispatcher := newConfigurationTestFsmIn(t, t.TempDir(), 1796, "ctrl1")
		fsm.dropConfigurationIndex()
		fsm.adoptConfiguration(2414, configurationOf("ctrl1", "ctrl2", "ctrl3"), true)

		// act: raft replays the entries that added ctrl3, then ctrl2
		fsm.StoreConfiguration(2412, configurationOf("ctrl1", "ctrl3"))
		fsm.StoreConfiguration(2414, configurationOf("ctrl1", "ctrl2", "ctrl3"))

		// assert
		state := fsm.GetCachedServers()
		req.Equal(uint64(2414), state.Index)
		req.ElementsMatch(members("ctrl1", "ctrl2", "ctrl3"), state.Servers, "an older configuration entry must not replace the list")
		req.Len(dispatcher.events(), 1, "only the adoption may be announced")
	})

	t.Run("a newer configuration entry replaces it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		fsm, dispatcher := newConfigurationTestFsmIn(t, t.TempDir(), 1796, "ctrl1")
		fsm.dropConfigurationIndex()
		fsm.adoptConfiguration(2414, configurationOf("ctrl1", "ctrl2", "ctrl3"), false)

		// act
		fsm.StoreConfiguration(2500, configurationOf("ctrl1", "ctrl2"))

		// assert
		state := fsm.GetCachedServers()
		req.Equal(uint64(2500), state.Index)
		req.ElementsMatch(members("ctrl1", "ctrl2"), state.Servers)
		events := dispatcher.events()
		req.Len(events, 1)
		req.Equal(uint64(2500), events[0].Index)
	})

	t.Run("an adoption at or below the cached index changes nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange: replay has already applied a newer entry
		fsm, dispatcher := newConfigurationTestFsmIn(t, t.TempDir(), 1796, "ctrl1")
		fsm.dropConfigurationIndex()
		fsm.StoreConfiguration(2500, configurationOf("ctrl1", "ctrl2"))

		// act
		fsm.adoptConfiguration(2414, configurationOf("ctrl1", "ctrl2", "ctrl3"), true)

		// assert
		state := fsm.GetCachedServers()
		req.Equal(uint64(2500), state.Index)
		req.ElementsMatch(members("ctrl1", "ctrl2"), state.Servers)
		req.Len(dispatcher.events(), 1, "an adoption that changes nothing must not be announced")
	})
}

func Test_Controller_syncFsmConfiguration(t *testing.T) {
	t.Run("a configuration at or below the applied index is adopted at once", func(t *testing.T) {
		req := require.New(t)

		// arrange: a restart of a member whose db is ahead of the last membership change
		logs := raft.NewInmemStore()
		req.NoError(logs.StoreLogs([]*raft.Log{
			{Index: 2414, Term: 3, Type: raft.LogConfiguration, Data: raft.EncodeConfiguration(configurationOf("ctrl1", "ctrl2", "ctrl3"))},
			{Index: 2415, Term: 3, Type: raft.LogCommand},
			{Index: 2416, Term: 3, Type: raft.LogCommand},
		}))
		fsm, dispatcher := newConfigurationTestFsmIn(t, t.TempDir(), 2416, "ctrl1", "ctrl2", "ctrl3")
		ctrl := &Controller{logStore: logs, snapshotStore: raft.NewInmemSnapshotStore(), Fsm: fsm}

		// act
		waiting := ctrl.syncFsmConfiguration()

		// assert
		req.False(waiting)
		state := fsm.GetCachedServers()
		req.Equal(uint64(2414), state.Index, "the list must carry the configuration's index, not the db's raft index")
		req.ElementsMatch(members("ctrl1", "ctrl2", "ctrl3"), state.Servers)
		req.Empty(dispatcher.events())
	})

	t.Run("a configuration above the applied index waits to be committed", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrl, fsm, _ := newRestoredSnapshotController(t)

		// act
		waiting := ctrl.syncFsmConfiguration()

		// assert
		req.True(waiting)
		state := fsm.GetCachedServers()
		req.Zero(state.Index, "the loaded list must not keep the db's raft index")
		req.ElementsMatch(members("ctrl1"), state.Servers, "a configuration not known to be committed must not be adopted")
	})

	t.Run("with no raft configuration the fsm keeps the list it loaded", func(t *testing.T) {
		req := require.New(t)

		// arrange
		logs := raft.NewInmemStore()
		req.NoError(logs.StoreLogs([]*raft.Log{{Index: 1, Term: 1, Type: raft.LogCommand}}))
		fsm, _ := newConfigurationTestFsmIn(t, t.TempDir(), 1, "ctrl1")
		before := fsm.GetCachedServers()
		ctrl := &Controller{logStore: logs, snapshotStore: raft.NewInmemSnapshotStore(), Fsm: fsm}

		// act
		waiting := ctrl.syncFsmConfiguration()

		// assert
		req.False(waiting)
		req.Same(before, fsm.GetCachedServers())
	})
}

func Test_Controller_tryAdoptCommittedConfiguration(t *testing.T) {
	t.Run("the configuration is adopted once the commit index reaches it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrl, fsm, dispatcher := newRestoredSnapshotController(t)
		req.True(ctrl.syncFsmConfiguration())

		// act and assert
		req.False(ctrl.tryAdoptCommittedConfiguration(2413), "a configuration above the commit index must not be adopted")
		req.ElementsMatch(members("ctrl1"), fsm.GetCachedServers().Servers)

		req.True(ctrl.tryAdoptCommittedConfiguration(2414))
		state := fsm.GetCachedServers()
		req.Equal(uint64(2414), state.Index)
		req.ElementsMatch(members("ctrl1", "ctrl2", "ctrl3"), state.Servers)
		events := dispatcher.events()
		req.Len(events, 1, "the adoption must be announced")
		req.Equal(uint64(2414), events[0].Index)
	})

	t.Run("a configuration the leader replaced is never adopted", func(t *testing.T) {
		req := require.New(t)

		// arrange: the log ends with a membership change this member appended but never committed
		ctrl, fsm, _ := newRestoredSnapshotController(t)
		logs := ctrl.logStore.(*raft.InmemStore)
		req.NoError(logs.StoreLogs([]*raft.Log{
			{Index: 2415, Term: 3, Type: raft.LogConfiguration, Data: raft.EncodeConfiguration(configurationOf("ctrl1", "ctrl2", "ctrl3", "ctrl4"))},
		}))
		req.True(ctrl.syncFsmConfiguration())
		req.False(ctrl.tryAdoptCommittedConfiguration(2414))

		// act: the new leader overwrites the entry with commands and commits past it
		req.NoError(logs.DeleteRange(2415, 2415))
		req.NoError(logs.StoreLogs([]*raft.Log{
			{Index: 2415, Term: 4, Type: raft.LogCommand},
			{Index: 2416, Term: 4, Type: raft.LogCommand},
		}))
		adopted := ctrl.tryAdoptCommittedConfiguration(2416)

		// assert
		req.True(adopted)
		state := fsm.GetCachedServers()
		req.Equal(uint64(2414), state.Index)
		req.ElementsMatch(members("ctrl1", "ctrl2", "ctrl3"), state.Servers, "the committed configuration must be adopted, not the replaced one")
	})

	t.Run("adopting after replay caught up announces nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrl, fsm, dispatcher := newRestoredSnapshotController(t)
		req.True(ctrl.syncFsmConfiguration())
		fsm.StoreConfiguration(2414, configurationOf("ctrl1", "ctrl2", "ctrl3"))

		// act
		adopted := ctrl.tryAdoptCommittedConfiguration(2414)

		// assert
		req.True(adopted)
		req.Len(dispatcher.events(), 1, "only the replayed entry may be announced")
	})
}

// newRestoredSnapshotController returns a Controller over the stores and db of a member that installed
// a snapshot taken while ctrl1 was the only member (index 1796, configuration index 1), with the entries
// adding ctrl3 (2412) and ctrl2 (2414) in its log but not yet applied.
func newRestoredSnapshotController(t *testing.T) (*Controller, *BoltDbFsm, *recordingDispatcher) {
	t.Helper()
	req := require.New(t)

	logs := raft.NewInmemStore()
	req.NoError(logs.StoreLogs([]*raft.Log{
		{Index: 2412, Term: 3, Type: raft.LogConfiguration, Data: raft.EncodeConfiguration(configurationOf("ctrl1", "ctrl3"))},
		{Index: 2413, Term: 3, Type: raft.LogCommand},
		{Index: 2414, Term: 3, Type: raft.LogConfiguration, Data: raft.EncodeConfiguration(configurationOf("ctrl1", "ctrl2", "ctrl3"))},
	}))
	snapshots := raft.NewInmemSnapshotStore()
	_, transport := raft.NewInmemTransport("tls:ctrl2.example:6262")
	sink, err := snapshots.Create(raft.SnapshotVersionMax, 1796, 2, configurationOf("ctrl1"), 1, transport)
	req.NoError(err)
	req.NoError(sink.Close())

	fsm, dispatcher := newConfigurationTestFsmIn(t, t.TempDir(), 1796, "ctrl1")
	return &Controller{logStore: logs, snapshotStore: snapshots, Fsm: fsm}, fsm, dispatcher
}

// newConfigurationTestFsmIn returns an FSM opened on dataDir, closed at test cleanup, with the dispatcher
// recording its cluster events. If ids are given, it first saves them as the cluster members and dbIndex
// as the db's raft index, then reopens the db, as a restarting controller loads them.
func newConfigurationTestFsmIn(t *testing.T, dataDir string, dbIndex uint64, ids ...string) (*BoltDbFsm, *recordingDispatcher) {
	t.Helper()
	if len(ids) > 0 {
		seed := NewFsm(dataDir, false, command.GetDefaultDecoders(), NewIndexTracker(), event.DispatcherMock{})
		require.NoError(t, seed.Init())
		require.NoError(t, seed.OverwriteServers(members(ids...)))
		seed.updateIndex(dbIndex)
		require.NoError(t, seed.Close())
	}

	dispatcher := &recordingDispatcher{}
	fsm := NewFsm(dataDir, false, command.GetDefaultDecoders(), NewIndexTracker(), dispatcher)
	require.NoError(t, fsm.Init())
	t.Cleanup(func() { _ = fsm.Close() })
	return fsm, dispatcher
}

// members returns a voting member for each of ids.
func members(ids ...string) []raft.Server {
	var result []raft.Server
	for _, id := range ids {
		result = append(result, raft.Server{
			ID:       raft.ServerID(id),
			Address:  raft.ServerAddress("tls:" + id + ".example:6262"),
			Suffrage: raft.Voter,
		})
	}
	return result
}

func configurationOf(ids ...string) raft.Configuration {
	return raft.Configuration{Servers: members(ids...)}
}

// recordingDispatcher records the cluster events dispatched to it.
type recordingDispatcher struct {
	event.DispatcherMock
	lock          sync.Mutex
	clusterEvents []*event.ClusterEvent
}

func (d *recordingDispatcher) AcceptClusterEvent(evt *event.ClusterEvent) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.clusterEvents = append(d.clusterEvents, evt)
}

func (d *recordingDispatcher) events() []*event.ClusterEvent {
	d.lock.Lock()
	defer d.lock.Unlock()
	return append([]*event.ClusterEvent(nil), d.clusterEvents...)
}
