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

	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

func Test_filterRemovableTerminators(t *testing.T) {
	const me = "router-me"

	// mine: present, owned by the requesting router; other: present, owned by a different router;
	// gone: not present; err: lookup fails.
	lookup := func(id string) (string, bool, error) {
		switch id {
		case "mine":
			return me, true, nil
		case "other":
			return "router-other", true, nil
		case "err":
			return "", false, errors.New("boom")
		default: // "gone" and anything else
			return "", false, nil
		}
	}

	t.Run("off the leader keeps owned and absent ids", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"gone", "mine"}, []bool{true, true}, me, false, lookup)
		req.Equal([]string{"gone", "mine"}, kept)
		req.Zero(rejected)
		req.Zero(skipped, "off the leader nothing is skipped as a no-op")
	})

	t.Run("rejects ids owned by another router", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"other", "mine"}, nil, me, true, lookup)
		req.Equal([]string{"mine"}, kept, "a router may only remove terminators it owns")
		req.Equal(1, rejected)
		req.Zero(skipped)
	})

	t.Run("rejects every id when none are owned", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"other", "other"}, nil, me, true, lookup)
		req.Empty(kept, "an all-foreign batch must delete nothing")
		req.Equal(2, rejected)
		req.Zero(skipped)
	})

	t.Run("rejects another router's id even when confirmed on the leader", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"other"}, []bool{true}, me, true, lookup)
		req.Empty(kept)
		req.Equal(1, rejected)
		req.Zero(skipped)
	})

	t.Run("leader skips confirmed and absent", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"gone"}, []bool{true}, me, true, lookup)
		req.Empty(kept, "a confirmed, already-removed terminator must be skipped")
		req.Zero(rejected)
		req.Equal(1, skipped)
	})

	t.Run("leader keeps confirmed but still present", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"mine"}, []bool{true}, me, true, lookup)
		req.Equal([]string{"mine"}, kept, "a terminator that still exists must go through the ordered delete")
		req.Zero(rejected)
		req.Zero(skipped)
	})

	t.Run("leader keeps unconfirmed even when absent", func(t *testing.T) {
		req := require.New(t)
		kept, _, skipped := filterRemovableTerminators([]string{"gone"}, []bool{false}, me, true, lookup)
		req.Equal([]string{"gone"}, kept, "an unconfirmed create may be committed but not yet applied, so it must not be skipped")
		req.Zero(skipped)
	})

	t.Run("nil createConfirmed treats every id as unconfirmed", func(t *testing.T) {
		req := require.New(t)
		// The v1 request carries no confirmation, so absent ids are kept (ownership still applies).
		kept, rejected, skipped := filterRemovableTerminators([]string{"gone", "other", "mine"}, nil, me, true, lookup)
		req.Equal([]string{"gone", "mine"}, kept)
		req.Equal(1, rejected)
		req.Zero(skipped)
	})

	t.Run("leader treats a missing createConfirmed entry as false", func(t *testing.T) {
		req := require.New(t)
		// createConfirmed shorter than ids (e.g. an older router) -> the uncovered id is unconfirmed
		kept, _, skipped := filterRemovableTerminators([]string{"gone", "gone"}, []bool{true}, me, true, lookup)
		req.Equal([]string{"gone"}, kept, "the id without a createConfirmed entry must be kept")
		req.Equal(1, skipped)
	})

	t.Run("a lookup error keeps the id", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators([]string{"err"}, []bool{true}, me, true, lookup)
		req.Equal([]string{"err"}, kept, "a lookup error must fall back to keeping the id")
		req.Zero(rejected, "an unresolved owner must not be treated as an ownership violation")
		req.Zero(skipped)
	})

	t.Run("leader mixes kept, rejected and skipped correctly", func(t *testing.T) {
		req := require.New(t)
		kept, rejected, skipped := filterRemovableTerminators(
			[]string{"gone", "mine", "other", "gone", "err"},
			[]bool{true, true, true, false, true},
			me, true, lookup)
		req.Equal([]string{"mine", "gone", "err"}, kept)
		req.Equal(1, rejected)
		req.Equal(1, skipped)
	})
}

// Test_ownsTerminator covers the single-terminator check used by the remove and update handlers,
// which reject outright rather than filtering.
func Test_ownsTerminator(t *testing.T) {
	handler := &baseHandler{router: &model.Router{BaseEntity: models.BaseEntity{Id: "router-me"}}}

	t.Run("owned by the requesting router", func(t *testing.T) {
		require.True(t, handler.ownsTerminator(&model.Terminator{Router: "router-me"}))
	})

	t.Run("owned by a different router", func(t *testing.T) {
		require.False(t, handler.ownsTerminator(&model.Terminator{Router: "router-other"}))
	})

	t.Run("unset owner", func(t *testing.T) {
		require.False(t, handler.ownsTerminator(&model.Terminator{}),
			"a terminator with no owner must not be treated as owned by the requester")
	})
}
