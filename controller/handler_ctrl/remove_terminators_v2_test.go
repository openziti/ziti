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

package handler_ctrl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_filterConfirmedAbsentTerminators(t *testing.T) {
	present := map[string]bool{"exists": true, "gone": false}
	isPresent := func(id string) (bool, error) {
		if id == "err" {
			return false, errors.New("boom")
		}
		return present[id], nil
	}

	t.Run("off the leader keeps every id", func(t *testing.T) {
		req := require.New(t)
		kept, skipped := filterConfirmedAbsentTerminators([]string{"gone", "exists"}, []bool{true, true}, false, isPresent)
		req.Equal([]string{"gone", "exists"}, kept)
		req.Zero(skipped)
	})

	t.Run("leader skips confirmed and absent", func(t *testing.T) {
		req := require.New(t)
		kept, skipped := filterConfirmedAbsentTerminators([]string{"gone"}, []bool{true}, true, isPresent)
		req.Empty(kept, "a confirmed, already-removed terminator must be skipped")
		req.Equal(1, skipped)
	})

	t.Run("leader keeps confirmed but still present", func(t *testing.T) {
		req := require.New(t)
		kept, skipped := filterConfirmedAbsentTerminators([]string{"exists"}, []bool{true}, true, isPresent)
		req.Equal([]string{"exists"}, kept, "a terminator that still exists must go through the ordered delete")
		req.Zero(skipped)
	})

	t.Run("leader keeps unconfirmed even when absent", func(t *testing.T) {
		req := require.New(t)
		kept, skipped := filterConfirmedAbsentTerminators([]string{"gone"}, []bool{false}, true, isPresent)
		req.Equal([]string{"gone"}, kept, "an unconfirmed create may be committed but not yet applied, so it must not be skipped")
		req.Zero(skipped)
	})

	t.Run("leader treats a missing createConfirmed entry as false", func(t *testing.T) {
		req := require.New(t)
		// createConfirmed shorter than ids (e.g. an older router) -> the uncovered id is unconfirmed
		kept, skipped := filterConfirmedAbsentTerminators([]string{"gone", "gone"}, []bool{true}, true, isPresent)
		req.Equal([]string{"gone"}, kept, "the id without a createConfirmed entry must be kept")
		req.Equal(1, skipped)
	})

	t.Run("leader keeps a confirmed id when the presence check errors", func(t *testing.T) {
		req := require.New(t)
		kept, skipped := filterConfirmedAbsentTerminators([]string{"err"}, []bool{true}, true, isPresent)
		req.Equal([]string{"err"}, kept, "a presence-check error must fall back to keeping the id")
		req.Zero(skipped)
	})

	t.Run("leader mixes kept and skipped correctly", func(t *testing.T) {
		req := require.New(t)
		kept, skipped := filterConfirmedAbsentTerminators(
			[]string{"gone", "exists", "gone", "err"},
			[]bool{true, true, false, true},
			true, isPresent)
		req.Equal([]string{"exists", "gone", "err"}, kept)
		req.Equal(1, skipped)
	})
}
