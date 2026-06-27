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

	"github.com/openziti/channel/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
)

// stubXlink is a minimal xlink.Xlink for the staleness-logic tests.
// Only the accessors CheckDialerSide / CheckListenerSide consult are
// real; the rest panic to surface accidental use.
type stubXlink struct {
	id           string
	dialed       bool
	dialAddress  string
	linkProtocol string
	linkKey      xlink.LinkKey
	closed       bool
}

func (*stubXlink) GetDestinationType() string                            { return "link" }
func (*stubXlink) SetHeartbeatControl(channel.HeartbeatControl)          {}
func (*stubXlink) UpdateHeartbeatIntervals(time.Duration, time.Duration) {}
func (s *stubXlink) Id() string                                          { return s.id }
func (s *stubXlink) Key() string                                         { return s.linkKey.String() }
func (s *stubXlink) DialAddress() string                                 { return s.dialAddress }
func (s *stubXlink) LinkKey() xlink.LinkKey                              { return s.linkKey }
func (s *stubXlink) LinkProtocol() string                                { return s.linkProtocol }
func (s *stubXlink) IsDialed() bool                                      { return s.dialed }
func (s *stubXlink) IsClosed() bool                                      { return s.closed }

// Required by the interface but not exercised by these tests.
func (*stubXlink) Iteration() uint32           { return 0 }
func (*stubXlink) DestinationId() string       { return "" }
func (*stubXlink) DestVersion() string         { return "" }
func (*stubXlink) CloseOnce(func())            {}
func (s *stubXlink) Close() error              { s.closed = true; return nil }
func (*stubXlink) CloseNotified() error        { return nil }
func (*stubXlink) AreFaultsSent() bool         { return false }
func (*stubXlink) DuplicatesRejected() uint32  { return 0 }
func (*stubXlink) Init(metrics.Registry) error { panic("not used") }
func (*stubXlink) SendPayload(*xgress.Payload, time.Duration, xgress.PayloadType) error {
	panic("not used")
}
func (*stubXlink) SendAcknowledgement(*xgress.Acknowledgement) error { panic("not used") }
func (*stubXlink) SendControl(*xgress.Control) error                 { panic("not used") }
func (*stubXlink) InspectCircuit(*xgress.CircuitInspectDetail)       { panic("not used") }
func (*stubXlink) InspectLink() *inspect.LinkInspectDetail           { panic("not used") }
func (*stubXlink) GetLinkConnState() *ctrl_pb.LinkConnState          { panic("not used") }

type stubXlinkListener struct {
	advertise    string
	protocol     string
	localBinding string
	groups       []string
}

func (s *stubXlinkListener) Listen() error             { panic("not used") }
func (s *stubXlinkListener) GetAdvertisement() string  { return s.advertise }
func (s *stubXlinkListener) GetLinkProtocol() string   { return s.protocol }
func (s *stubXlinkListener) GetLinkCostTags() []string { return nil }
func (s *stubXlinkListener) GetGroups() []string       { return s.groups }
func (s *stubXlinkListener) GetLocalBinding() string   { return s.localBinding }
func (s *stubXlinkListener) Close() error              { return nil }

type stubXlinkDialer struct {
	binding string
	groups  []string
}

func (s *stubXlinkDialer) Dial(xlink.Dial) (xlink.Xlink, error)         { panic("not used") }
func (s *stubXlinkDialer) GetGroups() []string                          { return s.groups }
func (s *stubXlinkDialer) GetBinding() string                           { return s.binding }
func (s *stubXlinkDialer) GetHealthyBackoffConfig() xlink.BackoffConfig { return nil }
func (s *stubXlinkDialer) GetUnhealthyBackoffConfig() xlink.BackoffConfig {
	return nil
}
func (s *stubXlinkDialer) AdoptBinding(xlink.Listener) {}

func destSnapshot(destId string, listeners ...*ctrl_pb.Listener) map[string][]*ctrl_pb.Listener {
	return map[string][]*ctrl_pb.Listener{destId: listeners}
}

func dialedXlink(dialerBinding, protocol, destId, listenerBinding string) *stubXlink {
	return &stubXlink{
		dialed:       true,
		linkProtocol: protocol,
		linkKey: xlink.LinkKey{
			DialerBinding:   dialerBinding,
			Protocol:        protocol,
			DestId:          destId,
			ListenerBinding: listenerBinding,
		},
	}
}

// --- dialer-side: orphaned mode (binding + protocol + group overlap) ---

func compatibleDest() map[string][]*ctrl_pb.Listener {
	return destSnapshot("destA", &ctrl_pb.Listener{
		Protocol:     "tls",
		LocalBinding: "lb1",
		Groups:       []string{"g1"},
	})
}

func Test_CheckDialerSide_Orphaned_FullMatch(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}}
	stale, reason := CheckDialerSide(link, dialers, compatibleDest(), StalenessModeOrphaned)
	require.False(t, stale)
	require.Empty(t, reason)
}

func Test_CheckDialerSide_Orphaned_BindingRemoved(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "ws", groups: []string{"g1"}}}
	stale, reason := CheckDialerSide(link, dialers, compatibleDest(), StalenessModeOrphaned)
	require.True(t, stale)
	require.Contains(t, reason, "transport")
}

func Test_CheckDialerSide_Orphaned_NoDialersAtAll(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	stale, _ := CheckDialerSide(link, nil, compatibleDest(), StalenessModeOrphaned)
	require.True(t, stale)
}

func Test_CheckDialerSide_Orphaned_DestinationGone(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}}
	stale, reason := CheckDialerSide(link, dialers, nil, StalenessModeOrphaned)
	require.True(t, stale)
	require.Contains(t, reason, "destA")
}

func Test_CheckDialerSide_Orphaned_GroupsNoLongerOverlap(t *testing.T) {
	// Functional staleness: dialer binding still exists but groups no
	// longer overlap with any remote listener -> orphaned-stale.
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}}
	dest := destSnapshot("destA", &ctrl_pb.Listener{
		Protocol:     "tls",
		LocalBinding: "lb1",
		Groups:       []string{"g2"},
	})
	stale, reason := CheckDialerSide(link, dialers, dest, StalenessModeOrphaned)
	require.True(t, stale)
	require.Contains(t, reason, "group-overlapping")
}

func Test_CheckDialerSide_Orphaned_ProtocolMismatch(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}}
	dest := destSnapshot("destA", &ctrl_pb.Listener{
		Protocol:     "ws",
		LocalBinding: "lb1",
		Groups:       []string{"g1"},
	})
	stale, _ := CheckDialerSide(link, dialers, dest, StalenessModeOrphaned)
	require.True(t, stale)
}

// --- dialer-side: changed mode (orphaned's check + listenerBinding identity) ---

func Test_CheckDialerSide_Changed_FullMatch(t *testing.T) {
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}}
	stale, reason := CheckDialerSide(link, dialers, compatibleDest(), StalenessModeChanged)
	require.False(t, stale)
	require.Empty(t, reason)
}

func Test_CheckDialerSide_Changed_ListenerBindingGone(t *testing.T) {
	// Compatible listener exists, but with a different localBinding. The
	// link key would drift -> changed-stale; not orphaned-stale because
	// re-dial via the renamed listener is still possible.
	link := dialedXlink("transport", "tls", "destA", "lb1")
	dialers := []xlink.Dialer{&stubXlinkDialer{binding: "transport", groups: []string{"g1"}}}
	dest := destSnapshot("destA", &ctrl_pb.Listener{
		Protocol:     "tls",
		LocalBinding: "lb2",
		Groups:       []string{"g1"},
	})
	stale, reason := CheckDialerSide(link, dialers, dest, StalenessModeChanged)
	require.True(t, stale)
	require.Contains(t, reason, "lb1")

	staleOrphaned, _ := CheckDialerSide(link, dialers, dest, StalenessModeOrphaned)
	require.False(t, staleOrphaned, "renamed listener with compatible config shouldn't trigger orphaned-stale")
}

// --- listener-side orphaned ---

func Test_CheckListenerSide_Orphaned_ProtocolMatch(t *testing.T) {
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	listeners := []xlink.Listener{&stubXlinkListener{advertise: "tls:1.2.3.4:7000", protocol: "tls"}}
	stale, _ := CheckListenerSide(link, listeners, StalenessModeOrphaned)
	require.False(t, stale)
}

func Test_CheckListenerSide_Orphaned_ProtocolGone(t *testing.T) {
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	listeners := []xlink.Listener{&stubXlinkListener{advertise: "ws:1.2.3.4:7000", protocol: "ws"}}
	stale, reason := CheckListenerSide(link, listeners, StalenessModeOrphaned)
	require.True(t, stale)
	require.Contains(t, reason, "tls")
}

// --- listener-side changed ---

func Test_CheckListenerSide_Changed_FullMatch(t *testing.T) {
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	listeners := []xlink.Listener{&stubXlinkListener{
		advertise:    "tls:1.2.3.4:6000",
		protocol:     "tls",
		localBinding: "lb1",
	}}
	stale, _ := CheckListenerSide(link, listeners, StalenessModeChanged)
	require.False(t, stale)
}

func Test_CheckListenerSide_Changed_AdvertiseChanged(t *testing.T) {
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	listeners := []xlink.Listener{&stubXlinkListener{
		advertise:    "tls:1.2.3.4:7000",
		protocol:     "tls",
		localBinding: "lb1",
	}}
	stale, reason := CheckListenerSide(link, listeners, StalenessModeChanged)
	require.True(t, stale)
	require.Contains(t, reason, "tls:1.2.3.4:6000")
}

func Test_CheckListenerSide_Changed_LocalBindingChanged(t *testing.T) {
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	listeners := []xlink.Listener{&stubXlinkListener{
		advertise:    "tls:1.2.3.4:6000",
		protocol:     "tls",
		localBinding: "lb2",
	}}
	stale, reason := CheckListenerSide(link, listeners, StalenessModeChanged)
	require.True(t, stale)
	require.Contains(t, reason, "lb1")
}

func Test_CheckListenerSide_Changed_NoListeners(t *testing.T) {
	link := &stubXlink{
		dialAddress:  "tls:1.2.3.4:6000",
		linkProtocol: "tls",
		linkKey:      xlink.LinkKey{Protocol: "tls", ListenerBinding: "lb1"},
	}
	stale, _ := CheckListenerSide(link, nil, StalenessModeChanged)
	require.True(t, stale)
}
