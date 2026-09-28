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

package handler_link

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"math"
	"testing"
	"time"

	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
)

// Test_leafFingerprint covers the fingerprint used to verify an incoming link's router: it must come
// only from the leaf certificate the handshake proved (certs[0]). The central case is that a victim
// router's certificate presented as filler elsewhere in the chain must not contribute a fingerprint,
// so it cannot be matched against the victim's enrolled fingerprint to impersonate the victim.
func Test_leafFingerprint(t *testing.T) {
	cert := func(cn string) *x509.Certificate {
		return &x509.Certificate{Subject: pkix.Name{CommonName: cn}, Raw: []byte("der-" + cn)}
	}

	t.Run("returns the leaf fingerprint", func(t *testing.T) {
		req := require.New(t)
		leaf := cert("router-A")

		fingerprint, err := leafFingerprint([]*x509.Certificate{leaf})
		req.NoError(err)
		req.Equal(nfpem.FingerprintFromCertificate(leaf), fingerprint)
	})

	t.Run("ignores filler certs in the rest of the chain", func(t *testing.T) {
		req := require.New(t)
		leaf := cert("router-A")
		filler := cert("router-D")

		fingerprint, err := leafFingerprint([]*x509.Certificate{leaf, filler})
		req.NoError(err)
		req.Equal(nfpem.FingerprintFromCertificate(leaf), fingerprint)
		req.NotEqual(nfpem.FingerprintFromCertificate(filler), fingerprint,
			"a victim cert presented as filler must not contribute a fingerprint")
	})

	t.Run("rejects when no certificates are presented", func(t *testing.T) {
		req := require.New(t)
		_, err := leafFingerprint(nil)
		req.Error(err)
	})
}

func Test_heartbeatCallback_GenerationChangeRearmsDeadline(t *testing.T) {
	hb := func(gen uint64, send, check, closeTimeout time.Duration) xlink.HeartbeatSettings {
		return xlink.HeartbeatSettings{
			Generation:               gen,
			SendInterval:             send,
			CheckInterval:            check,
			CloseUnresponsiveTimeout: closeTimeout,
		}
	}

	// Under send 30s / check 20s a healthy link's last response is legitimately
	// up to ~50s old, and the heartbeat sent on this very pulse cannot have been
	// answered yet.
	const healthyAgeMs = 50_000
	now := time.Now().UnixMilli()

	t.Run("tightening does not close a healthy link", func(t *testing.T) {
		req := require.New(t)
		cb := &heartbeatCallback{generation: 1, lastResponse: now - healthyAgeMs}

		_, shouldClose := cb.evaluate(now, hb(2, time.Second, 500*time.Millisecond, 10*time.Second))
		req.False(shouldClose, "accumulated age under the old intervals must not condemn the link")
		req.Equal(now, cb.lastResponse, "the deadline is re-armed to this check")
		req.Equal(uint64(2), cb.generation)
	})

	t.Run("the grace is one window, not immunity", func(t *testing.T) {
		req := require.New(t)
		cb := &heartbeatCallback{generation: 1, lastResponse: now - healthyAgeMs}
		// Above the unhealthy threshold on purpose: a timeout below it is inert
		// today, which is a separate defect this test should not depend on.
		settings := hb(2, 30*time.Second, 10*time.Second, 90*time.Second)

		cb.evaluate(now, settings)

		// Still silent a full new timeout later: detection must resume.
		later := now + settings.CloseUnresponsiveTimeout.Milliseconds() + 1
		unhealthy, shouldClose := cb.evaluate(later, settings)
		req.True(unhealthy)
		req.True(shouldClose, "a genuinely dead link still closes once the new timeout elapses")
	})

	t.Run("no generation change, no re-arm", func(t *testing.T) {
		req := require.New(t)
		cb := &heartbeatCallback{generation: 2, lastResponse: now - 90_000}

		unhealthy, shouldClose := cb.evaluate(now, hb(2, 30*time.Second, 20*time.Second, time.Minute))
		req.True(unhealthy)
		req.True(shouldClose, "an unresponsive link on a steady generation is still closed")
	})
}

func Test_heartbeatCallback_HonorsConfiguredTimeout(t *testing.T) {
	hb := func(closeTimeout time.Duration) xlink.HeartbeatSettings {
		return xlink.HeartbeatSettings{
			Generation:               1,
			SendInterval:             5 * time.Second,
			CheckInterval:            time.Second,
			CloseUnresponsiveTimeout: closeTimeout,
		}
	}
	now := time.Now().UnixMilli()
	silentFor := func(d time.Duration) *heartbeatCallback {
		return &heartbeatCallback{generation: 1, lastResponse: now - d.Milliseconds()}
	}

	t.Run("a timeout under the unhealthy threshold still closes", func(t *testing.T) {
		req := require.New(t)
		// The close timeout is evaluated on its own, not behind the 30s unhealthy
		// threshold.
		_, shouldClose := silentFor(11*time.Second).evaluate(now, hb(10*time.Second))
		req.True(shouldClose, "a 10s timeout must close at 11s, not wait for 30s")
	})

	t.Run("closing always reports unhealthy", func(t *testing.T) {
		req := require.New(t)
		// Otherwise a short timeout closes the link with no preceding warning
		// and without poisoning the latency metric watchers key off.
		unhealthy, shouldClose := silentFor(11*time.Second).evaluate(now, hb(10*time.Second))
		req.True(shouldClose)
		req.True(unhealthy)
	})

	t.Run("still healthy before the timeout", func(t *testing.T) {
		req := require.New(t)
		unhealthy, shouldClose := silentFor(9*time.Second).evaluate(now, hb(10*time.Second))
		req.False(shouldClose)
		req.False(unhealthy)
	})

	t.Run("a zero timeout closes nothing", func(t *testing.T) {
		req := require.New(t)
		// Zero means no settings have been published yet, not that the link
		// should go immediately. Without the guard, decoupling the thresholds
		// turns "unset" into "close on the first check".
		_, shouldClose := silentFor(5*time.Minute).evaluate(now, hb(0))
		req.False(shouldClose, "an unset timeout must never close a link")
	})

	t.Run("unhealthy is still reported on a long timeout", func(t *testing.T) {
		req := require.New(t)
		unhealthy, shouldClose := silentFor(31*time.Second).evaluate(now, hb(60*time.Second))
		req.True(unhealthy, "the 30s unhealthy signal survives the decoupling")
		req.False(shouldClose)
	})
}

func Test_heartbeatCallback_UnhealthyThresholdScalesWithIntervals(t *testing.T) {
	hb := func(send, check, closeTimeout time.Duration) xlink.HeartbeatSettings {
		return xlink.HeartbeatSettings{Generation: 1, SendInterval: send, CheckInterval: check, CloseUnresponsiveTimeout: closeTimeout}
	}
	now := time.Now().UnixMilli()
	silentFor := func(d time.Duration) *heartbeatCallback {
		return &heartbeatCallback{generation: 1, lastResponse: now - d.Milliseconds()}
	}

	t.Run("a long send interval is not unhealthy between heartbeats", func(t *testing.T) {
		req := require.New(t)
		// Reporting unhealthy poisons the latency metric the controller routes on, so
		// a healthy link must not trip it while waiting for a heartbeat that isn't due.
		settings := hb(45*time.Second, time.Second, 2*time.Minute)
		unhealthy, _ := silentFor(44*time.Second).evaluate(now, settings)
		req.False(unhealthy, "the next heartbeat is not even due yet")

		unhealthy, shouldClose := silentFor(93*time.Second).evaluate(now, settings)
		req.True(unhealthy, "a whole cycle with no response is a missed response")
		req.False(shouldClose)
	})

	t.Run("the defaults keep the 30s floor", func(t *testing.T) {
		req := require.New(t)
		settings := hb(10*time.Second, time.Second, time.Minute)
		unhealthy, _ := silentFor(29*time.Second).evaluate(now, settings)
		req.False(unhealthy)
		unhealthy, _ = silentFor(31*time.Second).evaluate(now, settings)
		req.True(unhealthy)
	})

	t.Run("a thin margin still reports unhealthy on close", func(t *testing.T) {
		req := require.New(t)
		// The threshold (92s) lands past the timeout (90s), leaving no early
		// warning, but a close is always reported unhealthy.
		unhealthy, shouldClose := silentFor(91*time.Second).evaluate(now, hb(45*time.Second, time.Second, 90*time.Second))
		req.True(shouldClose)
		req.True(unhealthy)
	})

	t.Run("doubling a huge gap does not wrap", func(t *testing.T) {
		req := require.New(t)
		huge := time.Duration(math.MaxInt64 / 2)
		req.Equal(time.Duration(math.MaxInt64).Milliseconds(), unhealthyThresholdMs(hb(huge, time.Second, time.Duration(math.MaxInt64))))
	})
}
