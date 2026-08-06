package mesh

import (
	"runtime"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/openziti/channel/v5"
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
		env:             &clusterIdEnv{},
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
		env:             &clusterIdEnv{},
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

// Test_GetPeerInfo_ReusesExistingConnectionWithoutDialing verifies that when the
// leader adds a member that is behind a firewall (no inbound ports open), the
// already-established inbound connection is reused. The joining node has opened an
// outbound connection to the leader, and GetPeerInfo is the first place the
// add-member flow could dial out, so it must return the already-connected peer's
// id/address directly instead of dialing the unreachable advertise address (which
// would time out and fail the join).
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

// Test_WaitForPeer_ReturnsExistingPeerWithoutDialing verifies that WaitForPeer
// returns an already-connected inbound peer immediately. The raft StreamLayer Dial
// path (used when the leader replicates to a new voter) goes through WaitForPeer, so
// an existing peer must be returned right away rather than blocking while a dial out
// to the (possibly unreachable) node is attempted.
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

// Test_GetOrConnectPeer_ReusesExistingConnection verifies that command forwarding to
// the leader reuses an established peer connection and does not dial out when one
// already exists.
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

func Test_peersWithMismatchedClusterId(t *testing.T) {
	t.Run("returns nothing when local cluster id is empty", func(t *testing.T) {
		peers := map[string]*Peer{
			"a": {Id: "a", ClusterId: "cluster-1"},
		}
		assert.Empty(t, peersWithMismatchedClusterId("", peers))
	})

	t.Run("returns nothing when all peers match", func(t *testing.T) {
		peers := map[string]*Peer{
			"a": {Id: "a", ClusterId: "cluster-1"},
			"b": {Id: "b", ClusterId: "cluster-1"},
		}
		assert.Empty(t, peersWithMismatchedClusterId("cluster-1", peers))
	})

	t.Run("ignores a peer with an empty cluster id", func(t *testing.T) {
		// A blank peer is a legitimate joiner that has not yet adopted a cluster id.
		peers := map[string]*Peer{
			"a": {Id: "a", ClusterId: ""},
		}
		assert.Empty(t, peersWithMismatchedClusterId("cluster-1", peers))
	})

	t.Run("returns only the peers whose cluster id differs", func(t *testing.T) {
		mismatch := &Peer{Id: "mismatch", ClusterId: "cluster-2"}
		peers := map[string]*Peer{
			"match":    {Id: "match", ClusterId: "cluster-1"},
			"blank":    {Id: "blank", ClusterId: ""},
			"mismatch": mismatch,
		}
		assert.Equal(t, []*Peer{mismatch}, peersWithMismatchedClusterId("cluster-1", peers))
	})
}

// closeRecordingChannel is a channel.Channel that records whether Close was called. Only Close is
// exercised by RevalidatePeerClusterIds; the embedded nil interface satisfies the rest.
type closeRecordingChannel struct {
	channel.Channel
	closed bool
}

func (self *closeRecordingChannel) Close() error {
	self.closed = true
	return nil
}

// clusterIdEnv is a mesh Env that reports a fixed cluster id. Only GetClusterId is exercised by
// RevalidatePeerClusterIds; the embedded nil interface satisfies the rest.
type clusterIdEnv struct {
	Env
	clusterId string
}

func (self *clusterIdEnv) GetClusterId() string {
	return self.clusterId
}

func Test_RevalidatePeerClusterIds_ClosesOnlyMismatchedPeers(t *testing.T) {
	matchCh := &closeRecordingChannel{}
	blankCh := &closeRecordingChannel{}
	mismatchCh := &closeRecordingChannel{}

	m := &impl{
		env: &clusterIdEnv{clusterId: "cluster-1"},
		Peers: map[string]*Peer{
			"match":    {Id: "match", ClusterId: "cluster-1", Channel: matchCh},
			"blank":    {Id: "blank", ClusterId: "", Channel: blankCh},
			"mismatch": {Id: "mismatch", ClusterId: "cluster-2", Channel: mismatchCh},
		},
	}

	m.RevalidatePeerClusterIds()

	assert.False(t, matchCh.closed, "peer with matching cluster id should not be closed")
	assert.False(t, blankCh.closed, "peer with an empty cluster id should not be closed")
	assert.True(t, mismatchCh.closed, "peer with a mismatched cluster id should be closed")
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
