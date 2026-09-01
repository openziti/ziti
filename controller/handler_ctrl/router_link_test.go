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
	"testing"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// Test_linksToPrune covers the selection a full refresh drives. Its input is now the reporting router's own
// links rather than the whole link table, so the filter has to be the thing that keeps a report from pruning
// links the router did not dial.
func Test_linksToPrune(t *testing.T) {
	const me = "router-me"

	self := model.NewRouter(me, "", "", 0, true)
	other := model.NewRouter("router-other", "", "", 0, true)

	dialed := func(id string) *model.Link {
		return &model.Link{Id: id, Src: self, DstId: other.Id}
	}
	accepted := func(id string) *model.Link {
		return &model.Link{Id: id, Src: other, DstId: me}
	}
	reported := func(ids ...string) []*ctrl_pb.RouterLinks_RouterLink {
		var result []*ctrl_pb.RouterLinks_RouterLink
		for _, id := range ids {
			result = append(result, &ctrl_pb.RouterLinks_RouterLink{Id: id})
		}
		return result
	}

	ids := func(links []*model.Link) []string {
		var result []string
		for _, link := range links {
			result = append(result, link.Id)
		}
		return result
	}

	t.Run("prunes dialed links the report omits", func(t *testing.T) {
		current := []*model.Link{dialed("kept"), dialed("gone")}
		require.Equal(t, []string{"gone"}, ids(linksToPrune(me, current, reported("kept"))))
	})

	t.Run("keeps every dialed link a full report lists", func(t *testing.T) {
		current := []*model.Link{dialed("a"), dialed("b")}
		require.Empty(t, linksToPrune(me, current, reported("a", "b")))
	})

	t.Run("an empty report prunes every dialed link", func(t *testing.T) {
		current := []*model.Link{dialed("a"), dialed("b")}
		require.Equal(t, []string{"a", "b"}, ids(linksToPrune(me, current, nil)))
	})

	t.Run("never prunes an accepted link", func(t *testing.T) {
		// forRouter answers on either endpoint, so the router's accepted links arrive here too. They are
		// the other router's to report, and an omission says nothing about them.
		current := []*model.Link{accepted("inbound"), dialed("gone")}
		require.Equal(t, []string{"gone"}, ids(linksToPrune(me, current, nil)),
			"a report from one end must not remove links the other end dialed")
	})

	t.Run("a report naming links the router has no record of prunes nothing", func(t *testing.T) {
		require.Empty(t, linksToPrune(me, nil, reported("unknown")))
	})
}
