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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_RequireLoopback(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		allowed bool
	}{
		{name: "ipv4 loopback", addr: "127.0.0.1:10001", allowed: true},
		{name: "ipv4 loopback range", addr: "127.0.0.2:10001", allowed: true},
		{name: "ipv6 loopback", addr: "[::1]:10001", allowed: true},
		{name: "localhost", addr: "localhost:10001", allowed: true},
		{name: "all interfaces", addr: ":10001", allowed: false},
		{name: "all interfaces explicit", addr: "0.0.0.0:10001", allowed: false},
		{name: "all interfaces ipv6", addr: "[::]:10001", allowed: false},
		{name: "routable address", addr: "10.1.2.3:10001", allowed: false},
		{name: "no port", addr: "127.0.0.1", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := requireLoopback(test.addr)

			if test.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_Listen_RefusesNonLoopbackTcp(t *testing.T) {
	req := require.New(t)

	err := Listen(Options{Addr: "tcp:0.0.0.0:0", ShutdownCleanup: boolPtr(false)})

	req.Error(err)
	req.Nil(listener, "no socket may be opened when the address is refused")
}

func Test_Listen_AcceptsLoopbackTcp(t *testing.T) {
	req := require.New(t)

	defer resetAgent()

	req.NoError(Listen(Options{Addr: "tcp:127.0.0.1:0", ShutdownCleanup: boolPtr(false)}))
	req.NotNil(listener)
}

func Test_HeapDumpTargetIsNotOverwritten(t *testing.T) {
	req := require.New(t)

	existing := filepath.Join(t.TempDir(), "already-here")
	req.NoError(os.WriteFile(existing, []byte("do not clobber me"), 0600))

	// Same flags the heap dump handler uses.
	_, err := os.OpenFile(existing, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	req.Error(err)

	content, err := os.ReadFile(existing)
	req.NoError(err)
	req.Equal("do not clobber me", string(content))
}

func boolPtr(b bool) *bool {
	return &b
}

// resetAgent closes the package listener so a later test can open its own.
func resetAgent() {
	mu.Lock()
	defer mu.Unlock()

	if listener != nil {
		_ = listener.Close()
		listener = nil
	}

	if tmpfile != "" {
		_ = os.Remove(tmpfile)
		tmpfile = ""
	}
}
