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

func Test_filterOwnedTerminators(t *testing.T) {
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

	t.Run("keeps owned and absent ids", func(t *testing.T) {
		req := require.New(t)
		kept, rejected := filterOwnedTerminators([]string{"mine", "gone"}, me, lookup)
		req.Equal([]string{"mine", "gone"}, kept)
		req.Zero(rejected)
	})

	t.Run("rejects ids owned by another router", func(t *testing.T) {
		req := require.New(t)
		kept, rejected := filterOwnedTerminators([]string{"other", "mine"}, me, lookup)
		req.Equal([]string{"mine"}, kept, "a router may only remove terminators it owns")
		req.Equal(1, rejected)
	})

	t.Run("a lookup error keeps the id", func(t *testing.T) {
		req := require.New(t)
		kept, rejected := filterOwnedTerminators([]string{"err"}, me, lookup)
		req.Equal([]string{"err"}, kept, "an unresolved owner must not be treated as an ownership violation")
		req.Zero(rejected)
	})

	t.Run("mixes kept and rejected", func(t *testing.T) {
		req := require.New(t)
		kept, rejected := filterOwnedTerminators([]string{"mine", "other", "gone", "err"}, me, lookup)
		req.Equal([]string{"mine", "gone", "err"}, kept)
		req.Equal(1, rejected)
	})
}
