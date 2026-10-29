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

// fsmTestCmd applies as a db Update that returns applyErr, so the FSM's in-tx index update
// commits or rolls back with it.
type fsmTestCmd struct {
	db       boltz.Db
	applyErr error
}

func (c *fsmTestCmd) Apply(ctx boltz.MutateContext) error {
	return c.db.Update(ctx, func(boltz.MutateContext) error { return c.applyErr })
}
func (c *fsmTestCmd) GetChangeContext() *change.Context { return nil }
func (c *fsmTestCmd) Encode() ([]byte, error)           { return encodeFsmTestCmd(), nil }

type fsmTestCriticalCmd struct{ fsmTestCmd }

func (c *fsmTestCriticalCmd) IsCriticalCommand() {}

// fsmTestDecoder decodes every entry to an fsmTestCmd against db, or an fsmTestCriticalCmd
// when critical is set.
type fsmTestDecoder struct {
	db       boltz.Db
	applyErr error
	critical bool
}

func (d *fsmTestDecoder) Decode(int32, []byte) (command.Command, error) {
	cmd := fsmTestCmd{db: d.db, applyErr: d.applyErr}
	if d.critical {
		return &fsmTestCriticalCmd{cmd}, nil
	}
	return &cmd, nil
}

// encodeFsmTestCmd returns an entry carrying only the fsmTestCmdType header.
func encodeFsmTestCmd() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(fsmTestCmdType))
	return b
}

// newTestFsm returns an initialized FSM in a temp dir, closed at test cleanup, along with the
// errors it halts on; halting records the error instead of ending the process.
func newTestFsm(t *testing.T, decoders command.Decoders) (*BoltDbFsm, *[]error, error) {
	t.Helper()
	fsm := NewFsm(t.TempDir(), false, decoders, NewIndexTracker(), event.DispatcherMock{})
	if err := fsm.Init(); err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { _ = fsm.Close() })
	halts := &[]error{}
	fsm.halt = func(err error) { *halts = append(*halts, err) }
	return fsm, halts, nil
}

func TestBoltDbFsm_Apply_UndecodableEntryHalts(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	fsm, halts, err := newTestFsm(t, decoders)
	req.NoError(err)
	entry := &raft.Log{Index: 7, Type: raft.LogCommand, Data: encodeFsmTestCmd()}

	t.Run("undecodable entry halts without advancing the index", func(t *testing.T) {
		req := require.New(t)

		result := fsm.Apply(entry)

		_, isErr := result.(error)
		req.True(isErr, "an undecodable entry returns an error")
		req.Len(*halts, 1, "an undecodable committed entry halts the member")
		req.Equal(uint64(0), fsm.index, "in-memory index must not advance past an entry that was not applied")
		persisted, err := fsm.loadCurrentIndex()
		req.NoError(err)
		req.Equal(uint64(0), persisted, "persisted index must not advance either")
		req.Equal(uint64(0), fsm.indexTracker.Index(), "tracker must not report an entry that was not applied")

		t.Run("same entry applies once its type is registered", func(t *testing.T) {
			req := require.New(t)
			decoders.Register(fsmTestCmdType, &fsmTestDecoder{db: fsm.GetDb()})
			*halts = nil

			result := fsm.Apply(entry)

			req.Nil(result, "the failed apply did not consume the entry")
			req.Empty(*halts)
			req.Equal(uint64(7), fsm.index)
			persisted, err := fsm.loadCurrentIndex()
			req.NoError(err)
			req.Equal(uint64(7), persisted)
			req.Equal(uint64(7), fsm.indexTracker.Index())
		})
	})
}

func TestBoltDbFsm_Apply_CriticalCommandFailureHalts(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	fsm, halts, err := newTestFsm(t, decoders)
	req.NoError(err)
	decoders.Register(fsmTestCmdType, &fsmTestDecoder{db: fsm.GetDb(), applyErr: errors.New("base state failed"), critical: true})
	entry := &raft.Log{Index: 3, Type: raft.LogCommand, Data: encodeFsmTestCmd()}

	result := fsm.Apply(entry)

	_, isErr := result.(error)
	req.True(isErr)
	req.Len(*halts, 1, "a failed critical command halts the member")
	persisted, err := fsm.loadCurrentIndex()
	req.NoError(err)
	req.Equal(uint64(0), persisted, "the in-tx index update rolled back with the apply")
}

func TestBoltDbFsm_Apply_OrdinaryErrorAdvancesIndex(t *testing.T) {
	req := require.New(t)
	decoders := command.NewDecoders()
	fsm, halts, err := newTestFsm(t, decoders)
	req.NoError(err)
	decoders.Register(fsmTestCmdType, &fsmTestDecoder{db: fsm.GetDb(), applyErr: errors.New("ordinary failure")})
	entry := &raft.Log{Index: 5, Type: raft.LogCommand, Data: encodeFsmTestCmd()}

	result := fsm.Apply(entry)

	_, isErr := result.(error)
	req.True(isErr)
	req.Empty(*halts, "an ordinary apply error is logged and the member advances")
	req.Equal(uint64(5), fsm.index)
	persisted, err := fsm.loadCurrentIndex()
	req.NoError(err)
	req.Equal(uint64(5), persisted)
	req.Equal(uint64(5), fsm.indexTracker.Index())
}

// validatingFsmCmd fails Validate when invalid is set.
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

// validatingFsmDecoder decodes every entry to a validatingFsmCmd, invalid when the payload
// byte is 1.
type validatingFsmDecoder struct{}

func (validatingFsmDecoder) Decode(_ int32, data []byte) (command.Command, error) {
	return &validatingFsmCmd{invalid: data[0] == 1}, nil
}

// encodeValidatingFsmCmd returns an entry that validatingFsmDecoder decodes to a command whose
// Validate fails when invalid is true.
func encodeValidatingFsmCmd(invalid bool) []byte {
	b := append(encodeFsmTestCmd(), 0)
	if invalid {
		b[4] = 1
	}
	return b
}

var errStopAtLimiter = errors.New("stopped at rate limiter")

// stopAtLimiter records that the submission reached the rate limiter and refuses it with
// errStopAtLimiter, so the test never needs a raft instance.
type stopAtLimiter struct{ calls int }

func (s *stopAtLimiter) RunRateLimited(string) (rate.RateLimitControl, error) {
	s.calls++
	return nil, errStopAtLimiter
}
func (s *stopAtLimiter) RunRateLimitedF(string, func(rate.RateLimitControl) error) error {
	s.calls++
	return errStopAtLimiter
}
func (s *stopAtLimiter) IsRateLimited() bool { return false }

func TestController_ApplyTwoPhase_Preflight(t *testing.T) {
	decoders := command.NewDecoders()
	decoders.Register(fsmTestCmdType, validatingFsmDecoder{})

	t.Run("command failing Validate is refused before the rate limiter", func(t *testing.T) {
		req := require.New(t)
		limiter := &stopAtLimiter{}
		ctrl := &Controller{decoders: decoders, raftRateLimiter: limiter}

		_, err := ctrl.ApplyTwoPhase(encodeValidatingFsmCmd(true))

		req.Error(err)
		req.Contains(err.Error(), "invalid command")
		req.Equal(0, limiter.calls, "a command whose Validate fails never takes a rate limiter slot")
	})

	t.Run("undecodable encoding is refused before the rate limiter", func(t *testing.T) {
		req := require.New(t)
		limiter := &stopAtLimiter{}
		ctrl := &Controller{decoders: decoders, raftRateLimiter: limiter}
		unknown := make([]byte, 4)
		binary.BigEndian.PutUint32(unknown, 4242)

		_, err := ctrl.ApplyTwoPhase(unknown)

		req.Error(err, "an encoding this controller cannot decode is refused")
		req.Equal(0, limiter.calls)
	})

	t.Run("valid command reaches the rate limiter", func(t *testing.T) {
		req := require.New(t)
		limiter := &stopAtLimiter{}
		ctrl := &Controller{decoders: decoders, raftRateLimiter: limiter}

		_, err := ctrl.ApplyTwoPhase(encodeValidatingFsmCmd(false))

		req.ErrorIs(err, errStopAtLimiter)
		req.Equal(1, limiter.calls)
	})
}

func TestController_ApplyEncodedCommand_RefusesUndecodable(t *testing.T) {
	req := require.New(t)
	limiter := &stopAtLimiter{}
	ctrl := &Controller{decoders: command.NewDecoders(), raftRateLimiter: limiter}

	_, err := ctrl.ApplyEncodedCommand(encodeFsmTestCmd())

	req.Error(err)
	req.Equal(0, limiter.calls, "the leader must not submit an entry its own registry cannot decode")
}
