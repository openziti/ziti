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

package handler_ctrl

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/servermetrics"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// faultTestEnv is an env.RouterEnv exposing only what the fault handler reads.
type faultTestEnv struct {
	env.RouterEnv
	pool     goroutines.Pool
	registry xlink.Registry
	metrics  servermetrics.UsageRegistry
}

func (self *faultTestEnv) GetRxPool() goroutines.Pool                      { return self.pool }
func (self *faultTestEnv) GetXlinkRegistry() xlink.Registry                { return self.registry }
func (self *faultTestEnv) GetMetricsRegistry() servermetrics.UsageRegistry { return self.metrics }

// faultTestPool runs work inline and counts it, or refuses everything when full.
type faultTestPool struct {
	goroutines.Pool
	full   bool
	queued atomic.Int32
}

func (self *faultTestPool) QueueOrError(f func()) error {
	if self.full {
		return errors.New("pool full")
	}
	self.queued.Add(1)
	f()
	return nil
}

type faultTestRegistry struct {
	xlink.Registry
	link xlink.Xlink
}

func (self *faultTestRegistry) GetLinkById(string) (xlink.Xlink, bool) { return self.link, true }

type faultTestLink struct {
	xlink.Xlink
	closed atomic.Bool
}

func (self *faultTestLink) Iteration() uint32 { return 0 }
func (self *faultTestLink) CloseNotified() error {
	self.closed.Store(true)
	return nil
}

type faultTestChannel struct {
	channel.Channel
}

func (faultTestChannel) Label() string { return "test" }

func newFaultTest(t *testing.T, pool *faultTestPool) (*faultHandler, *faultTestLink, servermetrics.UsageRegistry) {
	t.Helper()
	closeNotify := make(chan struct{})
	t.Cleanup(func() { close(closeNotify) })

	link := &faultTestLink{}
	registry := servermetrics.NewUsageRegistry(servermetrics.DefaultUsageRegistryConfig("test", closeNotify))
	handler := newFaultHandler(&faultTestEnv{
		pool:     pool,
		registry: &faultTestRegistry{link: link},
		metrics:  registry,
	})
	return handler, link, registry
}

func faultMessage(t *testing.T) *channel.Message {
	t.Helper()
	body, err := proto.Marshal(&ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: "l1"})
	require.NoError(t, err)
	return channel.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
}

// Test_faultHandler_HandsFaultsToTheReceivePool: a fault is handled on the receive pool rather than on a
// goroutine of its own, so a burst of faults is bounded.
func Test_faultHandler_HandsFaultsToTheReceivePool(t *testing.T) {
	pool := &faultTestPool{}
	handler, link, registry := newFaultTest(t, pool)

	handler.HandleReceive(faultMessage(t), faultTestChannel{})

	require.Equal(t, int32(1), pool.queued.Load(), "the fault must be handled on the receive pool")
	require.True(t, link.closed.Load())
	require.Equal(t, int64(0), registry.Meter("link.fault.rx_pool_fallback").Count())
}

// Test_faultHandler_SpawnsWhenThePoolIsFull: a dropped fault leaves a dead link marked up, so a full pool
// falls back to spawning, and counts doing so.
func Test_faultHandler_SpawnsWhenThePoolIsFull(t *testing.T) {
	handler, link, registry := newFaultTest(t, &faultTestPool{full: true})

	handler.HandleReceive(faultMessage(t), faultTestChannel{})

	require.Eventually(t, link.closed.Load, time.Second, 10*time.Millisecond, "the fault must still be handled")
	require.Equal(t, int64(1), registry.Meter("link.fault.rx_pool_fallback").Count())
}
