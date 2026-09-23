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

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
)

func listenerSet(addr string) []*ctrl_pb.Listener {
	return []*ctrl_pb.Listener{{Address: addr, Protocol: "tls"}}
}

func TestRouter_SetLinkListeners_DropsSupersededGeneration(t *testing.T) {
	req := require.New(t)
	r := &Router{}

	req.True(r.SetLinkListeners(listenerSet("a"), 1))
	req.Equal("a", r.GetLinkListeners()[0].Address)

	req.True(r.SetLinkListeners(listenerSet("b"), 2))
	req.Equal("b", r.GetLinkListeners()[0].Address)

	// An update that lost the race must not push the router back onto listeners
	// it has already replaced, since the set is redistributed to every peer.
	req.False(r.SetLinkListeners(listenerSet("a"), 1))
	req.Equal("b", r.GetLinkListeners()[0].Address, "the newer set must survive")

	// The same generation is also refused: nothing new to record.
	req.False(r.SetLinkListeners(listenerSet("c"), 2))
	req.Equal("b", r.GetLinkListeners()[0].Address)

	req.True(r.SetLinkListeners(listenerSet("d"), 3))
	req.Equal("d", r.GetLinkListeners()[0].Address)
}

func TestRouter_ResetLinkListeners_AllowsRestartedRouter(t *testing.T) {
	// A router mints generations per process, so a restart begins at 1 again. If
	// the controller kept the generation from the previous session, every update
	// from the restarted router would be refused as superseded.
	req := require.New(t)
	r := &Router{}

	req.True(r.SetLinkListeners(listenerSet("old"), 47))
	req.False(r.SetLinkListeners(listenerSet("new"), 1), "without a reset, a restart looks superseded")

	r.ResetLinkListeners()
	req.Empty(r.GetLinkListeners(), "reset clears the recorded set")
	req.True(r.SetLinkListeners(listenerSet("new"), 1), "after a reset the new session starts fresh")
	req.Equal("new", r.GetLinkListeners()[0].Address)
}

func TestRouter_SetLinkListeners_FirstUpdateAlwaysRecorded(t *testing.T) {
	// Generation zero from a router that predates the field must still land,
	// rather than being read as "already superseded".
	req := require.New(t)
	r := &Router{}
	req.True(r.SetLinkListeners(listenerSet("a"), 0))
	req.Equal("a", r.GetLinkListeners()[0].Address)
}
