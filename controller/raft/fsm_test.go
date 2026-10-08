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
	"encoding/binary"
	"errors"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/ziti/v2/controller/change"
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

const fsmTestCmdType int32 = 9999

// fsmTestCmd is a decodable command whose Apply the test supplies.
type fsmTestCmd struct {
	apply func(ctx boltz.MutateContext) error
}

func (c *fsmTestCmd) Apply(ctx boltz.MutateContext) error { return c.apply(ctx) }
func (c *fsmTestCmd) GetChangeContext() *change.Context   { return nil }
func (c *fsmTestCmd) Encode() ([]byte, error)             { return encodeFsmTestCmd(), nil }

type fsmTestCriticalCmd struct{ fsmTestCmd }

func (c *fsmTestCriticalCmd) IsCriticalCommand() {}

func encodeFsmTestCmd() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(fsmTestCmdType))
	return b
}

func newTestFsm(t *testing.T, decoders command.Decoders) (*BoltDbFsm, *[]error) {
	fsm := NewFsm(t.TempDir(), false, decoders, NewIndexTracker(), event.DispatcherMock{})
	require.NoError(t, fsm.Init())
	t.Cleanup(func() { _ = fsm.Close() })
	halts := &[]error{}
	fsm.halt = func(err error) { *halts = append(*halts, err) }
	return fsm, halts
}

func persistedIndex(t *testing.T, fsm *BoltDbFsm) uint64 {
	idx, err := fsm.loadCurrentIndex()
	require.NoError(t, err)
	return idx
}

func TestBoltDbFsm_Apply_UndecodableEntryHalts(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	fsm, halts := newTestFsm(t, decoders)

	entry := &raft.Log{Index: 7, Type: raft.LogCommand, Data: encodeFsmTestCmd()}
	result := fsm.Apply(entry)
	_, isErr := result.(error)
	req.True(isErr, "an undecodable entry returns an error")
	req.Len(*halts, 1, "an undecodable committed entry halts the member")
	req.Equal(uint64(0), fsm.index, "in-memory index must not advance past an entry that was not applied")
	req.Equal(uint64(0), persistedIndex(t, fsm), "persisted index must not advance either")
	req.Equal(uint64(0), fsm.indexTracker.Index(), "tracker must not report an entry that was not applied")

	decoders.RegisterF(fsmTestCmdType, func(int32, []byte) (command.Command, error) {
		return &fsmTestCmd{apply: func(ctx boltz.MutateContext) error {
			return fsm.GetDb().Update(ctx, func(boltz.MutateContext) error { return nil })
		}}, nil
	})
	*halts = nil
	req.Nil(fsm.Apply(entry), "after the type is registered the same entry applies")
	req.Empty(*halts)
	req.Equal(uint64(7), fsm.index)
	req.Equal(uint64(7), persistedIndex(t, fsm))
	req.Equal(uint64(7), fsm.indexTracker.Index())
}

func TestBoltDbFsm_Apply_CriticalCommandFailureHalts(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	fsm, halts := newTestFsm(t, decoders)
	decoders.RegisterF(fsmTestCmdType, func(int32, []byte) (command.Command, error) {
		return &fsmTestCriticalCmd{fsmTestCmd{apply: func(ctx boltz.MutateContext) error {
			return fsm.GetDb().Update(ctx, func(boltz.MutateContext) error { return errors.New("base state failed") })
		}}}, nil
	})

	entry := &raft.Log{Index: 3, Type: raft.LogCommand, Data: encodeFsmTestCmd()}
	_, isErr := fsm.Apply(entry).(error)
	req.True(isErr)
	req.Len(*halts, 1, "a failed critical command halts the member")
	req.Equal(uint64(0), persistedIndex(t, fsm), "the in-tx index update rolled back with the apply")
}

func TestBoltDbFsm_Apply_OrdinaryErrorAdvancesIndex(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	fsm, halts := newTestFsm(t, decoders)
	decoders.RegisterF(fsmTestCmdType, func(int32, []byte) (command.Command, error) {
		return &fsmTestCmd{apply: func(ctx boltz.MutateContext) error {
			return fsm.GetDb().Update(ctx, func(boltz.MutateContext) error { return errors.New("ordinary failure") })
		}}, nil
	})

	entry := &raft.Log{Index: 5, Type: raft.LogCommand, Data: encodeFsmTestCmd()}
	_, isErr := fsm.Apply(entry).(error)
	req.True(isErr)
	req.Empty(*halts, "an ordinary apply error is logged and the member advances")
	req.Equal(uint64(5), fsm.index)
	req.Equal(uint64(5), persistedIndex(t, fsm))
	req.Equal(uint64(5), fsm.indexTracker.Index())
}

func TestController_ApplyEncodedCommand_RefusesUndecodable(t *testing.T) {
	// Raft and the rate limiter are nil: reaching either means the entry was submitted.
	ctrl := &Controller{decoders: command.NewDecoders()}
	_, err := ctrl.ApplyEncodedCommand(encodeFsmTestCmd())
	require.Error(t, err, "the leader must not submit an entry its own registry cannot decode")
}

// validatingFsmCmd fails Validate when its payload byte is non-zero.
type validatingFsmCmd struct {
	fsmTestCmd
	invalid bool
}

func (c *validatingFsmCmd) Validate() error {
	if c.invalid {
		return errors.New("invalid command")
	}
	return nil
}

func encodeValidatingFsmCmd(invalid bool) []byte {
	b := append(encodeFsmTestCmd(), 0)
	if invalid {
		b[4] = 1
	}
	return b
}

// stopAtLimiter records that the submission reached the rate limiter and refuses it, so the
// test never needs a raft instance.
type stopAtLimiter struct{ calls int }

func (s *stopAtLimiter) RunRateLimited(string) (rate.RateLimitControl, error) {
	s.calls++
	return nil, errors.New("stop")
}
func (s *stopAtLimiter) RunRateLimitedF(string, func(rate.RateLimitControl) error) error {
	s.calls++
	return errors.New("stop")
}
func (s *stopAtLimiter) IsRateLimited() bool { return false }

func TestController_ApplyTwoPhase_Preflight(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	decoders.RegisterF(fsmTestCmdType, func(_ int32, data []byte) (command.Command, error) {
		return &validatingFsmCmd{invalid: data[0] == 1}, nil
	})
	limiter := &stopAtLimiter{}
	ctrl := &Controller{decoders: decoders, raftRateLimiter: limiter}

	_, err := ctrl.ApplyTwoPhase(encodeValidatingFsmCmd(true))
	req.Error(err)
	req.Contains(err.Error(), "invalid command")
	req.Equal(0, limiter.calls, "a command whose Validate fails never takes a rate limiter slot")

	unknown := make([]byte, 4)
	binary.BigEndian.PutUint32(unknown, 4242)
	_, err = ctrl.ApplyTwoPhase(unknown)
	req.Error(err, "an encoding this controller cannot decode is refused")
	req.Equal(0, limiter.calls)

	_, err = ctrl.ApplyTwoPhase(encodeValidatingFsmCmd(false))
	req.Error(err, "the stub limiter refuses, which proves a valid command reached it")
	req.Equal(1, limiter.calls)
}
