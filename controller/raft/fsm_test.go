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

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/event"
	"github.com/stretchr/testify/require"
)

// TestBoltDbFsm_GetStartIndex checks that Init seeds the start index from the index persisted in the
// database. The router data model is built from that database and gates changesets on this value, so
// a start index that doesn't match what the database holds makes the model reject what follows.
func TestBoltDbFsm_GetStartIndex(t *testing.T) {
	req := require.New(t)

	dataDir := t.TempDir()
	fsm := NewFsm(dataDir, command.GetDefaultDecoders(), NewIndexTracker(), event.DispatcherMock{})
	req.NoError(fsm.Init())
	req.Equal(uint64(0), fsm.GetStartIndex(), "a fresh fsm starts at index 0")

	const persistedIndex uint64 = 42
	req.NoError(fsm.GetDb().Update(nil, func(ctx boltz.MutateContext) error {
		return fsm.updateIndexInTx(ctx.Tx(), persistedIndex)
	}))
	req.NoError(fsm.Close())

	restarted := NewFsm(dataDir, command.GetDefaultDecoders(), NewIndexTracker(), event.DispatcherMock{})
	req.NoError(restarted.Init())
	req.Equal(persistedIndex, restarted.GetStartIndex(), "a restarted fsm starts at the persisted index")
	req.NoError(restarted.Close())
}
