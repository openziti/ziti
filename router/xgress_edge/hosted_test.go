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

package xgress_edge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubRateLimitControl is a rate.RateLimitControl that records how each outcome was signaled, so
// tests can assert the router classified an establishment as a success or a backoff.
type stubRateLimitControl struct {
	success int
	backoff int
	failed  int
}

func (self *stubRateLimitControl) Success() { self.success++ }
func (self *stubRateLimitControl) Backoff() { self.backoff++ }
func (self *stubRateLimitControl) Failed()  { self.failed++ }

func Test_edgeTerminator_rateLimitSignaling(t *testing.T) {
	t.Run("establishment under threshold reports success", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(ctrl)
		req.Equal(0, ctrl.backoff, "storing a control with no prior attempt must not signal backoff")

		term.resolveRateLimitCallback(establishmentTimeout - time.Second)

		req.Equal(1, ctrl.success)
		req.Equal(0, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		req.Nil(term.GetAndClearRateLimitCallback(), "control must be cleared once resolved")
	})

	t.Run("establishment at or over threshold reports backoff", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(ctrl)

		term.resolveRateLimitCallback(establishmentTimeout)

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		req.Nil(term.GetAndClearRateLimitCallback(), "control must be cleared once resolved")
	})

	t.Run("well over threshold reports backoff", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(ctrl)

		term.resolveRateLimitCallback(2 * establishmentTimeout)

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff)
	})

	t.Run("resolving with no outstanding control is a no-op", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{}
		req.NotPanics(func() {
			term.resolveRateLimitCallback(time.Hour)
		})
	})

	t.Run("re-send resolves the prior control with backoff instead of orphaning it", func(t *testing.T) {
		req := require.New(t)

		prior := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(prior)
		req.Equal(0, prior.backoff)

		// A 30s re-send acquires a fresh control and supersedes the prior attempt's control.
		next := &stubRateLimitControl{}
		term.replaceRateLimitCallback(next)

		req.Equal(1, prior.backoff, "the superseded control must be resolved with backoff, not orphaned")
		req.Equal(0, prior.success)
		req.Equal(0, next.backoff, "the new control is still outstanding and must not be resolved yet")

		got := term.GetAndClearRateLimitCallback()
		req.NotNil(got)
		req.Same(next, got.(*stubRateLimitControl), "the new control must be the one now stored")
	})
}
