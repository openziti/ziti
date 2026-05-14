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

package link

import (
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
)

// gcStubRegistry is a minimal xlink.Registry for the auto-GC tests.
// Iter returns the prepared link set; GetDestinationListeners returns
// the prepared snapshot.
type gcStubRegistry struct {
	links         []xlink.Xlink
	destListeners map[string][]*ctrl_pb.Listener
}

func (r *gcStubRegistry) Iter() <-chan xlink.Xlink {
	out := make(chan xlink.Xlink, len(r.links))
	for _, l := range r.links {
		out <- l
	}
	close(out)
	return out
}

func (r *gcStubRegistry) GetDestinationListeners() map[string][]*ctrl_pb.Listener {
	return r.destListeners
}

// The rest of xlink.Registry isn't exercised by RunStaleLinkGc.
func (*gcStubRegistry) UpdateLinkDest(string, string, bool, []*ctrl_pb.Listener) {}
func (*gcStubRegistry) RemoveLinkDest(string)                                    {}
func (*gcStubRegistry) GetLink(string) (xlink.Xlink, bool)                       { return nil, false }
func (*gcStubRegistry) GetLinkById(string) (xlink.Xlink, bool)                   { return nil, false }
func (*gcStubRegistry) DialSucceeded(xlink.Xlink) (xlink.Xlink, bool)            { return nil, false }
func (*gcStubRegistry) LinkAccepted(xlink.Xlink) (xlink.Xlink, bool)             { return nil, false }
func (*gcStubRegistry) LinkClosed(xlink.Xlink)                                   {}
func (*gcStubRegistry) Shutdown()                                                {}
func (*gcStubRegistry) SendRouterLinkMessage(xlink.Xlink, ...channel.Channel)    {}
func (*gcStubRegistry) Inspect(time.Duration) *inspect.LinksInspectResult        { return nil }
func (*gcStubRegistry) DebugForgetLink(string) bool                              { return false }
func (*gcStubRegistry) GetLinkKey(string, string, string, string) string         { return "" }
func (*gcStubRegistry) RescanForDialOpportunities()                              {}

// gcStubEnv satisfies link.XlinkEnv.
type gcStubEnv struct {
	registry  xlink.Registry
	listeners []xlink.Listener
	dialers   []xlink.Dialer
}

func (e *gcStubEnv) GetXlinkRegistry() xlink.Registry    { return e.registry }
func (e *gcStubEnv) GetXlinkListeners() []xlink.Listener { return e.listeners }
func (e *gcStubEnv) GetXlinkDialers() []xlink.Dialer     { return e.dialers }

func Test_RunStaleLinkGc_Preserve_DoesNothing(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	env := &gcStubEnv{
		registry: &gcStubRegistry{links: []xlink.Xlink{link}},
		dialers:  []xlink.Dialer{&stubXlinkDialer{binding: "ws"}}, // would be stale
	}
	closed := RunStaleLinkGc(env, GcModePreserve)
	require.Empty(t, closed)
	require.False(t, link.IsClosed())
}

func Test_RunStaleLinkGc_Orphaned_ClosesStaleDialerSide(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	env := &gcStubEnv{
		registry: &gcStubRegistry{links: []xlink.Xlink{link}},
		dialers:  []xlink.Dialer{&stubXlinkDialer{binding: "ws"}}, // wrong binding -> stale
	}
	closed := RunStaleLinkGc(env, GcModeOrphaned)
	require.Len(t, closed, 1)
	require.True(t, link.IsClosed())
}

func Test_RunStaleLinkGc_Orphaned_KeepsHealthyLinks(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	env := &gcStubEnv{
		registry: &gcStubRegistry{
			links: []xlink.Xlink{link},
			destListeners: map[string][]*ctrl_pb.Listener{
				"destA": {{Protocol: "tls", LocalBinding: "lb1", Groups: []string{"g1"}}},
			},
		},
		dialers: []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}},
	}
	closed := RunStaleLinkGc(env, GcModeOrphaned)
	require.Empty(t, closed)
	require.False(t, link.IsClosed())
}

func Test_RunStaleLinkGc_Changed_ClosesListenerBindingDrift(t *testing.T) {
	// Dialer still exists, but the remote listener's localBinding moved.
	link := dialedXlink("transport", "tls", "destA", "lb1")
	env := &gcStubEnv{
		registry: &gcStubRegistry{
			links: []xlink.Xlink{link},
			destListeners: map[string][]*ctrl_pb.Listener{
				"destA": {{Protocol: "tls", LocalBinding: "lb2", Groups: []string{"g1"}}},
			},
		},
		dialers: []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}},
	}
	closed := RunStaleLinkGc(env, GcModeChanged)
	require.Len(t, closed, 1)
	require.True(t, link.IsClosed())
}

func Test_RunStaleLinkGc_Changed_TolerantUnderOrphaned(t *testing.T) {
	// Same listener-binding drift as the previous test: orphaned mode
	// tolerates the rename because a compatible (protocol + groups)
	// listener still exists — only the link key would drift.
	link := dialedXlink("transport", "tls", "destA", "lb1")
	env := &gcStubEnv{
		registry: &gcStubRegistry{
			links: []xlink.Xlink{link},
			destListeners: map[string][]*ctrl_pb.Listener{
				"destA": {{Protocol: "tls", LocalBinding: "lb2", Groups: []string{"g1"}}},
			},
		},
		dialers: []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}},
	}
	closed := RunStaleLinkGc(env, GcModeOrphaned)
	require.Empty(t, closed)
	require.False(t, link.IsClosed())
}

func Test_RunStaleLinkGc_SkipsClosedLinks(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	link.closed = true // mark as already closed
	env := &gcStubEnv{
		registry: &gcStubRegistry{links: []xlink.Xlink{link}},
		dialers:  []xlink.Dialer{&stubXlinkDialer{binding: "ws"}}, // would be stale
	}
	closed := RunStaleLinkGc(env, GcModeOrphaned)
	require.Empty(t, closed)
}

func Test_RunStaleLinkGc_ListenerSide(t *testing.T) {
	// Accepted link (IsDialed=false). Listener side: orphaned mode flags
	// it because no listener with this protocol remains.
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	env := &gcStubEnv{
		registry:  &gcStubRegistry{links: []xlink.Xlink{link}},
		listeners: []xlink.Listener{&stubXlinkListener{protocol: "ws", localBinding: "lb1"}},
	}
	closed := RunStaleLinkGc(env, GcModeOrphaned)
	require.Len(t, closed, 1)
	require.True(t, link.IsClosed())
}
