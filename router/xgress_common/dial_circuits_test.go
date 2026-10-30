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

package xgress_common

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/stretchr/testify/require"
)

// blockingDataPlane holds every ForwardPayload until release is closed, standing in for a forwarder
// that keeps timing out on a congested circuit. Each ForwardPayload signals forwarding on entry.
type blockingDataPlane struct {
	xgress.DataPlaneAdapter
	forwarding  chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func (self *blockingDataPlane) ForwardPayload(payload *xgress.Payload, _ *xgress.Xgress, _ context.Context) {
	self.forwarding <- payload.CircuitId
	<-self.release
}

func (self *blockingDataPlane) unblock() {
	self.releaseOnce.Do(func() { close(self.release) })
}

type nopPeer struct {
	xgress.Connection
}

func (self *nopPeer) Close() error {
	return nil
}

func newTrackedXgress(registry *DialCircuitRegistry, circuitId string, dataPlane xgress.DataPlaneAdapter) *xgress.Xgress {
	x := xgress.NewXgress(circuitId, "ctrl", xgress.Address(circuitId), &nopPeer{}, xgress.Initiator, xgress.DefaultOptions(), nil)
	x.SetDataPlaneAdapter(dataPlane)
	registry.PrepareTrack(x)
	registry.Publish("service", x)
	return x
}

// TestCloseAllDoesNotWaitOnBlockedClose verifies that closing tracked circuits returns while their
// end-of-circuit forwards are stalled, and that one stalled close does not hold up the others.
func TestCloseAllDoesNotWaitOnBlockedClose(t *testing.T) {
	req := require.New(t)

	dataPlane := &blockingDataPlane{
		forwarding: make(chan string, 2),
		release:    make(chan struct{}),
	}
	t.Cleanup(dataPlane.unblock)

	registry := NewDialCircuitRegistry()
	newTrackedXgress(registry, "circuit-1", dataPlane)
	newTrackedXgress(registry, "circuit-2", dataPlane)

	done := make(chan struct{})
	go func() {
		registry.CloseAll("test")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		req.Fail("CloseAll waited on an xgress whose end-of-circuit could not be forwarded")
	}

	forwarding := map[string]bool{}
	for len(forwarding) < 2 {
		select {
		case circuitId := <-dataPlane.forwarding:
			forwarding[circuitId] = true
		case <-time.After(time.Second):
			req.Failf("not every close reached the data plane", "reached: %v", forwarding)
		}
	}

	req.Equal(2, registry.circuits.Count(), "circuits stay tracked until their close completes")

	dataPlane.unblock()
	req.Eventually(registry.circuits.IsEmpty, time.Second, 10*time.Millisecond)
}

// TestRevalidateIfRequestedRunsOncePerRequest verifies that RevalidateIfRequested checks nothing
// until revalidation is requested, and that repeated requests before a run collapse into one run.
func TestRevalidateIfRequestedRunsOncePerRequest(t *testing.T) {
	req := require.New(t)

	dataPlane := &blockingDataPlane{
		forwarding: make(chan string, 1),
		release:    make(chan struct{}),
	}
	dataPlane.unblock()

	registry := NewDialCircuitRegistry()
	newTrackedXgress(registry, "circuit-1", dataPlane)

	checks := 0
	check := func(string) error {
		checks++
		return nil
	}

	registry.RevalidateIfRequested(check)
	req.Equal(0, checks)

	registry.RequestRevalidation()
	registry.RequestRevalidation()
	registry.RevalidateIfRequested(check)
	req.Equal(1, checks)

	registry.RevalidateIfRequested(check)
	req.Equal(1, checks)
}
