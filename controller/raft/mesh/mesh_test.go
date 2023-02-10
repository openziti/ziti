package mesh

import (
	"github.com/openziti/foundation/v2/versions"
	"runtime"
	"testing"
	"time"

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
		Peers:   map[string]*Peer{},
		version: NewVersionProviderTest(),
	}

	p := &Peer{Version: testVersion("1")}

	m.AddPeer(p)
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_AddPeer_TurnsReadonlyWhenVersionsDoNotMatch(t *testing.T) {
	m := &impl{
		Peers:   map[string]*Peer{},
		version: NewVersionProviderTest(),
	}

	p := &Peer{Version: testVersion("dne")}

	m.AddPeer(p)
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_RemovePeer_StaysReadonlyWhenDeletingPeerAndStillHasMismatchedVersions(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
			"2": {Version: testVersion("dne"), Address: "2"},
		},
		version: NewVersionProviderTest(),
	}
	m.readonly.Store(true)

	m.RemovePeer(m.Peers["1"])
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_RemovePeer_RemovesReadonlyWhenDeletingPeerWithNoOtherMismatches(t *testing.T) {
	m := &impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
			"2": {Version: testVersion("1"), Address: "2"},
		},
		version: NewVersionProviderTest(),
	}
	m.readonly.Store(true)

	m.RemovePeer(m.Peers["1"])
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_RemovePeer_RemovesReadonlyWhenDeletingLastPeer(t *testing.T) {
	m := impl{
		Peers: map[string]*Peer{
			"1": {Version: testVersion("dne"), Address: "1"},
		},
		version: NewVersionProviderTest(),
	}
	m.readonly.Store(true)

	m.RemovePeer(m.Peers["1"])
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
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
