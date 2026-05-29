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

package agent

import (
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/agentid"
	"github.com/stretchr/testify/require"
)

// recordingCallbacks records the arguments passed to each log-level callback so
// the end-to-end test can assert that commands reached the handlers correctly.
type recordingCallbacks struct {
	mu              sync.Mutex
	globalLevel     LogLevel
	globalSet       bool
	channelName     string
	channelLevel    LogLevel
	channelSet      bool
	clearedChannel  string
	clearedChannelN int
}

func (r *recordingCallbacks) asCallbacks() LogLevelCallbacks {
	return LogLevelCallbacks{
		SetLogLevel: func(l LogLevel) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.globalLevel = l
			r.globalSet = true
		},
		SetChannelLogLevel: func(c string, l LogLevel) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.channelName = c
			r.channelLevel = l
			r.channelSet = true
		},
		ClearChannelLogLevel: func(c string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.clearedChannel = c
			r.clearedChannelN++
		},
	}
}

func TestLogLevelCommandsEndToEnd(t *testing.T) {
	resetCaps()

	rec := &recordingCallbacks{}
	require.NoError(t, RegisterLogLevelHandlers(rec.asCallbacks()))

	sock := filepath.Join(t.TempDir(), "agent.sock")
	cleanup := false
	require.NoError(t, Listen(Options{
		Addr:            "unix:" + sock,
		ShutdownCleanup: &cleanup,
		AppType:         "test-app",
		AppId:           "test-1",
		CustomOps: map[byte]func(conn net.Conn) error{
			CustomOpAsync: func(conn net.Conn) error {
				return HandleChannelConnection(conn, &identity.TokenId{Token: "test"}, 99, nil)
			},
		},
	}))
	t.Cleanup(func() {
		Close()
		mu.Lock()
		listener = nil
		tmpfile = ""
		mu.Unlock()
	})

	dial := func() net.Conn {
		conn, err := net.Dial("unix", sock)
		require.NoError(t, err)
		return conn
	}

	// AppInfoV2 advertises the capability now that handlers are registered.
	require.NoError(t, MakeRequestToConn(dial(), AppInfoV2, nil, func(conn net.Conn) error {
		resp, ok, err := ReadAppInfoV2Response(conn)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "test-app", resp.Type)
		require.Contains(t, resp.AgentCapabilities, "logging.slog-levels")
		return nil
	}))

	// v2 set-log-level over the channel reaches the global callback.
	require.NoError(t, MakeRequestToConn(dial(), CustomOpAsync, []byte{agentid.AppIdAny}, ConnToChannel(func(ch channel.Channel) error {
		msg, err := SendSetLogLevelV2(ch, DebugLevel, time.Second)
		require.NoError(t, err)
		require.Contains(t, msg, "debug")
		return nil
	})))
	rec.mu.Lock()
	require.True(t, rec.globalSet)
	require.Equal(t, DebugLevel, rec.globalLevel)
	rec.mu.Unlock()

	// v2 set-channel-log-level reaches the per-channel callback.
	require.NoError(t, MakeRequestToConn(dial(), CustomOpAsync, []byte{agentid.AppIdAny}, ConnToChannel(func(ch channel.Channel) error {
		_, err := SendSetChannelLogLevelV2(ch, "network.gossip", TraceLevel, time.Second)
		return err
	})))
	rec.mu.Lock()
	require.True(t, rec.channelSet)
	require.Equal(t, "network.gossip", rec.channelName)
	require.Equal(t, TraceLevel, rec.channelLevel)
	rec.mu.Unlock()

	// v2 clear-channel-log-level reaches the clear callback.
	require.NoError(t, MakeRequestToConn(dial(), CustomOpAsync, []byte{agentid.AppIdAny}, ConnToChannel(func(ch channel.Channel) error {
		_, err := SendClearChannelLogLevelV2(ch, "network.gossip", time.Second)
		return err
	})))
	rec.mu.Lock()
	require.Equal(t, "network.gossip", rec.clearedChannel)
	require.Equal(t, 1, rec.clearedChannelN)
	rec.mu.Unlock()

	// The framed set-log-level command routes through the same callback.
	require.NoError(t, MakeRequestToConn(dial(), SetLogLevel, []byte{byte(InfoLevel)}, func(conn net.Conn) error {
		_, _ = conn.Read(make([]byte, 256))
		return nil
	}))
	rec.mu.Lock()
	require.Equal(t, InfoLevel, rec.globalLevel)
	rec.mu.Unlock()
}
