package mesh

import (
	"runtime"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/controller/event"

	"github.com/stretchr/testify/assert"
)

func Test_checkState_ReadonlyFalseWhenAllVersionsMatch(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("1"), Address: "1"},
			"2": {Version: testVersion("1"), Address: "2"},
		},
		version: NewVersionProviderTest(),
	}

	m.updateClusterState()
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_checkState_ReadonlyTrueWhenAllVersionsDoNotMatch(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
			"2": {Version: testVersion("dne"), Address: "2"},
		},
		version: NewVersionProviderTest(),
	}

	m.updateClusterState()
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_checkState_ReadonlySetToFalseWhenPreviouslyTrueAndAllVersionsNowMatch(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("1"), Address: "1"},
			"2": {Version: testVersion("1"), Address: "2"},
		},
		version: NewVersionProviderTest(),
	}
	m.readonly.Store(true)

	m.updateClusterState()
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_AddPeer_PassesReadonlyWhenVersionsMatch(t *testing.T) {
	m := &impl{
		Peers:           map[string]*Peer{},
		version:         NewVersionProviderTest(),
		eventDispatcher: event.DispatcherMock{},
	}

	p := &Peer{Version: testVersion("1")}

	assert.NoError(t, m.PeerConnected(p, true))
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_AddPeer_TurnsReadonlyWhenVersionsDoNotMatch(t *testing.T) {
	m := &impl{
		Peers:           map[string]*Peer{},
		version:         NewVersionProviderTest(),
		eventDispatcher: event.DispatcherMock{},
	}

	p := &Peer{Version: testVersion("dne")}

	assert.NoError(t, m.PeerConnected(p, true))
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_RemovePeer_StaysReadonlyWhenDeletingPeerAndStillHasMismatchedVersions(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
			"2": {Version: testVersion("dne"), Address: "2"},
		},
		version:         NewVersionProviderTest(),
		eventDispatcher: event.DispatcherMock{},
	}
	m.readonly.Store(true)

	m.PeerDisconnected(m.Peers["1"])
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_RemovePeer_RemovesReadonlyWhenDeletingPeerWithNoOtherMismatches(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
			"2": {Version: testVersion("1"), Address: "2"},
		},
		version:         NewVersionProviderTest(),
		eventDispatcher: event.DispatcherMock{},
	}
	m.readonly.Store(true)

	m.PeerDisconnected(m.Peers["1"])
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_RemovePeer_RemovesReadonlyWhenDeletingLastPeer(t *testing.T) {
	m := impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
		},
		version:         NewVersionProviderTest(),
		eventDispatcher: event.DispatcherMock{},
	}
	m.readonly.Store(true)

	m.PeerDisconnected(m.Peers["1"])
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

// firewalledPeerAddr is a well-formed transport address that is not reachable.
// The reuse tests below register a peer at this address, then assert the mesh
// returns the existing connection without attempting to dial it.
const firewalledPeerAddr = "tls:firewalled-node.example:6262"

// Test_GetPeerInfo_ReusesExistingConnectionWithoutDialing is a regression test
// for #3841. When the leader adds a member that is behind a firewall (no inbound
// ports open), the joining node has already opened an outbound connection to the
// leader. GetPeerInfo is the first place the add-member flow could dial out, so it
// must return the already-connected peer's id/address directly instead of dialing
// the unreachable advertise address (which previously timed out and failed the join).
func Test_GetPeerInfo_ReusesExistingConnectionWithoutDialing(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			firewalledPeerAddr: {Id: "joining-node-id", Address: firewalledPeerAddr},
		},
	}

	start := time.Now()
	id, addr, err := m.GetPeerInfo(firewalledPeerAddr, time.Second)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, raft.ServerID("joining-node-id"), id)
	assert.Equal(t, raft.ServerAddress(firewalledPeerAddr), addr)
	assert.Less(t, elapsed, 200*time.Millisecond, "should reuse existing connection, not dial out")
}

// Test_WaitForPeer_ReturnsExistingPeerWithoutDialing is a regression test for
// #3841. The raft StreamLayer Dial path (used when the leader replicates to a new
// voter) goes through WaitForPeer. An already-connected inbound peer must be
// returned immediately rather than blocking while a dial out to the (possibly
// unreachable) node is attempted.
func Test_WaitForPeer_ReturnsExistingPeerWithoutDialing(t *testing.T) {
	peer := &Peer{Id: "joining-node-id", Address: firewalledPeerAddr}
	m := &impl{
		Peers:       map[string]*Peer{firewalledPeerAddr: peer},
		peerWaiters: map[string][]chan *Peer{},
		closeNotify: make(chan struct{}),
	}

	start := time.Now()
	got, err := m.WaitForPeer(firewalledPeerAddr, time.Second)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Same(t, peer, got)
	assert.Less(t, elapsed, 200*time.Millisecond, "should return immediately, not wait/dial")
}

// Test_GetOrConnectPeer_ReusesExistingConnection is a regression test for #3841.
// Command forwarding to the leader reuses an established peer connection; it must
// not dial out when one already exists.
func Test_GetOrConnectPeer_ReusesExistingConnection(t *testing.T) {
	peer := &Peer{Id: "joining-node-id", Address: firewalledPeerAddr}
	m := &impl{
		Peers:       map[string]*Peer{firewalledPeerAddr: peer},
		peerWaiters: map[string][]chan *Peer{},
		closeNotify: make(chan struct{}),
	}

	got, err := m.GetOrConnectPeer(firewalledPeerAddr, time.Second)
	assert.NoError(t, err)
	assert.Same(t, peer, got)
}

func testVersion(v string) *versions.VersionInfo {
	return &versions.VersionInfo{Version: v}
}

type VersionProviderTest struct {
}

func (v VersionProviderTest) Branch() string {
	return "local"
}

func (v VersionProviderTest) EncoderDecoder() versions.VersionEncDec {
	return &versions.StdVersionEncDec
}

func (v VersionProviderTest) Version() string {
	return "1"
}

func (v VersionProviderTest) BuildDate() string {
	return time.Now().String()
}

func (v VersionProviderTest) Revision() string {
	return ""
}

func (v VersionProviderTest) AsVersionInfo() *versions.VersionInfo {
	return &versions.VersionInfo{
		Version:   v.Version(),
		Revision:  v.Revision(),
		BuildDate: v.BuildDate(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func NewVersionProviderTest() versions.VersionProvider {
	return &VersionProviderTest{}
}
