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

package model

import (
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

func newMarkDisconnectedTestManager() *RouterManager {
	return &RouterManager{connected: cmap.New[*Router]()}
}

// TestMarkDisconnected_LeavesStateOfNonHolderAlone: a connection owns only the state on its own instance, so
// one that has already been replaced must not clear it. The connected flag decides whether the controller
// accepts that router's link reports, so clearing it for the wrong connection silences a router that is up
// and reporting with nothing to correct it.
func TestMarkDisconnected_LeavesStateOfNonHolderAlone(t *testing.T) {
	mgr := newMarkDisconnectedTestManager()

	superseded := NewRouter("r1", "r1", "", 0, false)
	current := NewRouter("r1", "r1", "", 0, false)

	superseded.Connected.Store(true)
	superseded.routerLinks.Add(&Link{Id: "l1", DstId: "dst"}, "dst")

	mgr.MarkConnected(current)

	mgr.MarkDisconnected(superseded)

	require.Same(t, current, mgr.GetConnected("r1"), "the registration holder must be left in place")
	require.True(t, current.Connected.Load(), "the holder's connected flag must not be cleared")
	require.True(t, superseded.Connected.Load(),
		"a non-holder's state is not this call's to clear")
	require.Len(t, superseded.GetLinks(), 1, "a non-holder's links are not this call's to clear")
}

// TestMarkDisconnected_ClearsStateOfHolder is the ordinary case: the registration holder disconnecting gives
// up the registration and its per-connection state together.
func TestMarkDisconnected_ClearsStateOfHolder(t *testing.T) {
	mgr := newMarkDisconnectedTestManager()

	current := NewRouter("r1", "r1", "", 0, false)
	current.routerLinks.Add(&Link{Id: "l1", DstId: "dst"}, "dst")
	mgr.MarkConnected(current)
	require.True(t, current.Connected.Load())

	mgr.MarkDisconnected(current)

	require.Nil(t, mgr.GetConnected("r1"), "the registration must be given up")
	require.False(t, current.Connected.Load(), "the holder's connected flag must be cleared")
	require.Empty(t, current.GetLinks(), "the holder's links must be cleared")
}

// TestMarkDisconnected_UnregisteredIsANoop: a connection that never registered, or whose registration is
// already gone, has nothing to give up.
func TestMarkDisconnected_UnregisteredIsANoop(t *testing.T) {
	mgr := newMarkDisconnectedTestManager()

	r := NewRouter("r1", "r1", "", 0, false)
	r.Connected.Store(true)

	mgr.MarkDisconnected(r)

	require.Nil(t, mgr.GetConnected("r1"))
	require.True(t, r.Connected.Load(), "with no registration to give up there is nothing to clear")
}
