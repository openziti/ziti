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

package xlink_transport

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xlink"

	"github.com/openziti/foundation/v2/goroutines"
	"github.com/stretchr/testify/require"
)

// minimalLinkEnv implements LinkEnv with just the methods cleanupExpiredPartialLinks
// touches. Anything else panics; the close tests don't exercise those paths.
type minimalLinkEnv struct {
	closeNotify chan struct{}
}

func newMinimalLinkEnv() *minimalLinkEnv {
	return &minimalLinkEnv{closeNotify: make(chan struct{})}
}

func (e *minimalLinkEnv) GetCloseNotify() <-chan struct{} { return e.closeNotify }
func (e *minimalLinkEnv) GetMetricsRegistry() metrics.UsageRegistry {
	panic("not used in close tests")
}
func (e *minimalLinkEnv) GetXLinkRegistry() xlink.Registry      { panic("not used in close tests") }
func (e *minimalLinkEnv) GetNetworkControllers() env.NetworkControllers { panic("not used in close tests") }
func (e *minimalLinkEnv) GetRateLimiterPool() goroutines.Pool   { panic("not used in close tests") }
func (e *minimalLinkEnv) GetRouterId() *identity.TokenId        { panic("not used in close tests") }
func (e *minimalLinkEnv) GetLinkPayloadSenderQueueSize() int    { return 1 }
func (e *minimalLinkEnv) GetLinkAckSenderQueueSize() int        { return 1 }

// newCloseTestListener builds a listener bare enough that we can drive its
// Close lifecycle without setting up a real transport. The cleanup goroutine
// is started manually (Listen() would also start it, but Listen needs a
// real socket which we don't want in unit tests).
func newCloseTestListener(env *minimalLinkEnv) *listener {
	return &listener{
		env:          env,
		pendingLinks: map[string]*pendingLink{},
		stopC:        make(chan struct{}),
		lock:         sync.Mutex{},
	}
}

func Test_Listener_Close_StopsCleanupGoroutine(t *testing.T) {
	req := require.New(t)
	l := newCloseTestListener(newMinimalLinkEnv())

	done := make(chan struct{})
	go func() {
		l.cleanupExpiredPartialLinks()
		close(done)
	}()

	req.NoError(l.Close())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanupExpiredPartialLinks did not exit within 2s of Close()")
	}
}

func Test_Listener_Close_RouterShutdownAlsoStopsCleanupGoroutine(t *testing.T) {
	// Sanity-check the existing router-wide close path still works after the
	// per-listener stopC addition.
	req := require.New(t)
	env := newMinimalLinkEnv()
	l := newCloseTestListener(env)

	done := make(chan struct{})
	go func() {
		l.cleanupExpiredPartialLinks()
		close(done)
	}()

	close(env.closeNotify)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanupExpiredPartialLinks did not exit on env close-notify")
	}
	req.NoError(l.Close())
}

func Test_Listener_Close_Idempotent(t *testing.T) {
	req := require.New(t)
	l := newCloseTestListener(newMinimalLinkEnv())

	go l.cleanupExpiredPartialLinks()

	req.NoError(l.Close())
	// Second close must not panic on the already-closed stopC channel.
	req.NoError(l.Close())
	req.NoError(l.Close())
}

func Test_Listener_OpenClose_NoCleanupGoroutineLeak(t *testing.T) {
	req := require.New(t)

	const cycles = 25
	for range cycles {
		l := newCloseTestListener(newMinimalLinkEnv())
		go l.cleanupExpiredPartialLinks()
		req.NoError(l.Close())
	}

	// Give the goroutines a beat to exit after their Close() returned.
	req.Eventually(func() bool {
		return countGoroutinesIn("cleanupExpiredPartialLinks") == 0
	}, time.Second, 10*time.Millisecond, "cleanupExpiredPartialLinks goroutines leaked after open/close cycles")
}

// countGoroutinesIn returns the number of currently-running goroutines whose
// stack contains the given function name. Used to detect leaks of specific
// goroutines without worrying about runtime-managed background goroutines.
func countGoroutinesIn(funcName string) int {
	buf := make([]byte, 64*1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	stacks := strings.Split(string(buf), "\n\n")
	count := 0
	for _, s := range stacks {
		if strings.Contains(s, funcName) {
			count++
		}
	}
	return count
}
