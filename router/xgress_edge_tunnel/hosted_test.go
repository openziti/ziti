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

package xgress_edge_tunnel

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/v2/ziti"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/openziti/ziti/v2/tunnel"
	"github.com/stretchr/testify/require"
)

// countingCtrlChannel is a channel.Channel that records how many create requests were sent. Only
// the methods establishTerminator touches are implemented; the rest panic via the nil embedded
// interface if the test strays onto an unexpected path.
type countingCtrlChannel struct {
	channel.Channel
	sends atomic.Int32
}

func (self *countingCtrlChannel) Id() string { return "test-ctrl" }

func (self *countingCtrlChannel) TrySend(channel.Sendable) (bool, error) {
	self.sends.Add(1)
	return true, nil
}

// settleTestEnv supplies only the env surface establishTerminator uses: the router id, the model
// update channel, and a rate limiter that never limits.
type settleTestEnv struct {
	routerEnv.RouterEnv
	ctrlCh *countingCtrlChannel
}

func (self *settleTestEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{Token: "test-router"}
}

func (self *settleTestEnv) GetNetworkControllers() routerEnv.NetworkControllers {
	return &routerEnv.MockNetworkControllers{Channel: self.ctrlCh}
}

func (self *settleTestEnv) GetCtrlRateLimiter() rate.AdaptiveRateLimitTracker {
	return rate.NoOpAdaptiveRateLimitTracker{}
}

// settleTestHostingContext supplies the three accessors establishTerminator reads to build the
// create request.
type settleTestHostingContext struct {
	tunnel.HostingContext
}

func (settleTestHostingContext) ServiceName() string                { return "test-service" }
func (settleTestHostingContext) ServiceId() string                  { return "test-service-id" }
func (settleTestHostingContext) ListenOptions() *ziti.ListenOptions { return &ziti.ListenOptions{} }

func newSettleTestRegistry() (*HostedServiceRegistry, *countingCtrlChannel) {
	ctrlCh := &countingCtrlChannel{}
	return &HostedServiceRegistry{
		establishSet: map[string]*tunnelTerminator{},
		env:          &settleTestEnv{ctrlCh: ctrlCh},
	}, ctrlCh
}

func newSettleTestTerminator(id string, createTime time.Time) *tunnelTerminator {
	terminator := &tunnelTerminator{
		id:         id,
		context:    settleTestHostingContext{},
		createTime: createTime,
	}
	terminator.state.Store(xgress_common.TerminatorStateEstablishing)
	return terminator
}

// TestSettleGateHoldsNewTerminator verifies that a terminator still inside its settle window is
// not sent to the controller, stays queued, and arms the re-check timer.
func TestSettleGateHoldsNewTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now())
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.Contains(registry.establishSet, terminator.id, "terminator inside its settle window must stay queued")
	req.Equal(int32(0), ctrlCh.sends.Load(), "no create should be sent inside the settle window")
	req.NotNil(registry.establishNotify, "a re-check must be scheduled so the terminator is retried after the window")
}

// TestSettleGateReleasesSettledTerminator verifies that once the settle window has passed the
// terminator is sent and removed from the queue.
func TestSettleGateReleasesSettledTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.NotContains(registry.establishSet, terminator.id, "a settled terminator must be dequeued once attempted")
	req.Equal(int32(1), ctrlCh.sends.Load(), "a settled terminator must be sent to the controller")
}

// TestSettleGateDoesNotDelayRequeuedTerminator verifies that the delay applies only to a
// terminator's first attempt. A retry reuses the terminator with its original createTime, so it
// is already past the window and must be sent again without waiting.
func TestSettleGateDoesNotDelayRequeuedTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()
	req.Equal(int32(1), ctrlCh.sends.Load())

	// model a controller response that left the terminator establishing: it is requeued with the
	// same createTime and its in-flight flag cleared.
	terminator.operationActive.Store(false)
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.Equal(int32(2), ctrlCh.sends.Load(), "a requeued terminator must be retried without a second settle delay")
	req.NotContains(registry.establishSet, terminator.id)
}

// TestSettleGateIsPerTerminator verifies the gate is evaluated independently per terminator: a
// settled terminator is sent even while a newer one is still holding, and neither starves the
// other.
func TestSettleGateIsPerTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	settled := newSettleTestTerminator("settled", time.Now().Add(-establishSettleTime-time.Second))
	holding := newSettleTestTerminator("holding", time.Now())
	registry.establishSet[settled.id] = settled
	registry.establishSet[holding.id] = holding

	registry.evaluateEstablishQueue()

	req.Equal(int32(1), ctrlCh.sends.Load(), "only the settled terminator should be sent")
	req.NotContains(registry.establishSet, settled.id, "the settled terminator must be dequeued")
	req.Contains(registry.establishSet, holding.id, "the holding terminator must remain queued")
}
