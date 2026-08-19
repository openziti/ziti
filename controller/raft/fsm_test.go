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
	"testing"

	hraft "github.com/hashicorp/raft"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

// TestBoltDbFsm_GetStartIndex_AfterRestart guards against a regression where
// BoltDbFsm.Init() loaded the persisted raft index into self.index but failed
// to also populate self.startIndex. That left GetStartIndex() returning 0 on
// every restart, which in turn caused the RDM to seed its RaftIndexProvider
// at 0 and report a stale index until the next command flowed through.
func TestBoltDbFsm_GetStartIndex_AfterRestart(t *testing.T) {
	req := require.New(t)

	dataDir := t.TempDir()
	fsm := NewFsm(dataDir, false, command.GetDefaultDecoders(), NewIndexTracker(), event.DispatcherMock{})
	req.NoError(fsm.Init())
	req.Equal(uint64(0), fsm.GetStartIndex(), "fresh fsm should start at index 0")

	const persistedIndex uint64 = 42
	req.NoError(fsm.db.Update(nil, func(ctx boltz.MutateContext) error {
		return fsm.updateIndexInTx(ctx.Tx(), persistedIndex)
	}))
	req.NoError(fsm.Close())

	fsm2 := NewFsm(dataDir, false, command.GetDefaultDecoders(), NewIndexTracker(), event.DispatcherMock{})
	req.NoError(fsm2.Init())
	req.Equal(persistedIndex, fsm2.GetStartIndex(), "GetStartIndex should reflect the persisted raft index after restart")
	req.NoError(fsm2.Close())
}

// TestBoltDbFsm_AppliedIndex_AfterConfigurationAndRestart verifies that live and reloaded progress
// use the same persisted index.
func TestBoltDbFsm_AppliedIndex_AfterConfigurationAndRestart(t *testing.T) {
	req := require.New(t)
	dataDir := t.TempDir()

	tracker := NewIndexTracker()
	fsm := NewFsm(dataDir, false, command.GetDefaultDecoders(), tracker, event.DispatcherMock{})
	req.NoError(fsm.Init())

	const configurationIndex uint64 = 42
	fsm.StoreConfiguration(configurationIndex, hraft.Configuration{})
	req.Equal(configurationIndex, tracker.Index())
	req.NoError(fsm.Close())

	reloadedTracker := NewIndexTracker()
	reloadedFsm := NewFsm(dataDir, false, command.GetDefaultDecoders(), reloadedTracker, event.DispatcherMock{})
	req.NoError(reloadedFsm.Init())
	req.Equal(configurationIndex, reloadedTracker.Index())
	req.NoError(reloadedFsm.Close())
}

// TestBoltDbFsm_AppliedIndex_DoesNotAdvanceForRejectedCommand verifies that unpersisted progress is
// not exposed as applied.
func TestBoltDbFsm_AppliedIndex_DoesNotAdvanceForRejectedCommand(t *testing.T) {
	req := require.New(t)
	dataDir := t.TempDir()
	tracker := NewIndexTracker()
	fsm := NewFsm(dataDir, false, command.GetDefaultDecoders(), tracker, event.DispatcherMock{})
	req.NoError(fsm.Init())

	result := fsm.Apply(&hraft.Log{
		Index: 42,
		Type:  hraft.LogCommand,
		Data:  []byte{1},
	})
	req.Error(result.(error))
	req.Zero(tracker.Index())
	req.NoError(fsm.Close())

	reloadedTracker := NewIndexTracker()
	reloadedFsm := NewFsm(dataDir, false, command.GetDefaultDecoders(), reloadedTracker, event.DispatcherMock{})
	req.NoError(reloadedFsm.Init())
	req.Zero(reloadedTracker.Index())
	req.NoError(reloadedFsm.Close())
}
