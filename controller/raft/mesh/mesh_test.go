package mesh

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkState_ReadonlyFalseWhenAllVersionsMatch(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(false)
	m := impl{
		readonly: ro,
		Peers: map[string]*Peer{
			"1": {Version: "1", Address: "1"},
			"2": {Version: "1", Address: "2"},
		},
		version: "1",
	}

	m.checkState()
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_checkState_ReadonlyTrueWhenAllVersionsDoNotMatch(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(false)
	m := impl{
		readonly: ro,
		Peers: map[string]*Peer{
			"1": {Version: "dne", Address: "1"},
			"2": {Version: "dne", Address: "2"},
		},
		version: "1",
	}

	m.checkState()
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_checkState_ReadonlySetToFalseWhenPreviouslyTrueAndAllVersionsNowMatch(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(true)
	m := impl{
		readonly: ro,
		Peers: map[string]*Peer{
			"1": {Version: "1", Address: "1"},
			"2": {Version: "1", Address: "2"},
		},
		version: "1",
	}

	m.checkState()
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_AddPeer_PassesReadonlyWhenVersionsMatch(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(false)
	m := impl{
		readonly: ro,
		Peers:    map[string]*Peer{},
		version:  "1",
	}

	p := &Peer{Version: "1"}

	m.AddPeer(p)
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_AddPeer_TurnsReadonlyWhenVersionsDoNotMatch(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(false)
	m := impl{
		readonly: ro,
		Peers:    map[string]*Peer{},
		version:  "1",
	}

	p := &Peer{Version: "dne"}

	m.AddPeer(p)
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_RemovePeer_StaysReadonlyWhenDeletingPeerAndStillHasMismatchedVersions(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(true)
	m := impl{
		readonly: ro,
		Peers: map[string]*Peer{
			"1": {Version: "dne", Address: "1"},
			"2": {Version: "dne", Address: "2"},
		},
		version: "1",
	}

	m.RemovePeer(m.Peers["1"])
	assert.Equal(t, true, m.readonly.Load(), "Expected readonly to be true, got ", m.readonly.Load())
}

func Test_RemovePeer_RemovesReadonlyWhenDeletingPeerWithNoOtherMismatches(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(true)
	m := impl{
		readonly: ro,
		Peers: map[string]*Peer{
			"1": {Version: "dne", Address: "1"},
			"2": {Version: "1", Address: "2"},
		},
		version: "1",
	}

	m.RemovePeer(m.Peers["1"])
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}

func Test_RemovePeer_RemovesReadonlyWhenDeletingLastPeer(t *testing.T) {
	ro := atomic.Bool{}
	ro.Store(true)
	m := impl{
		readonly: ro,
		Peers: map[string]*Peer{
			"1": {Version: "dne", Address: "1"},
		},
		version: "1",
	}

	m.RemovePeer(m.Peers["1"])
	assert.Equal(t, false, m.readonly.Load(), "Expected readonly to be false, got ", m.readonly.Load())
}
