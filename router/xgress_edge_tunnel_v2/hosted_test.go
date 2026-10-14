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

package xgress_edge_tunnel_v2

import (
	"testing"

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/tunnel"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// createTestCtrlChannel is a channel.Channel that keeps the last message sent. Only the methods
// establishTerminator touches are implemented; the rest panic via the nil embedded interface if the
// test strays onto an unexpected path.
type createTestCtrlChannel struct {
	channel.Channel
	lastSent *channel.Message
}

func (self *createTestCtrlChannel) Id() string { return "test-ctrl" }

func (self *createTestCtrlChannel) TrySend(s channel.Sendable) (bool, error) {
	self.lastSent = s.Msg()
	return true, nil
}

// testCtrls offers ctrlCh, which may be nil, as the model update channel.
type testCtrls struct {
	routerEnv.NetworkControllers
	ctrlCh channel.Channel
}

func (self *testCtrls) GetModelUpdateCtrlChannel() channel.Channel { return self.ctrlCh }

// createTestEnv supplies the env surface establishTerminator uses: the router id, the model update
// channel, and a rate limiter that never limits.
type createTestEnv struct {
	routerEnv.RouterEnv
	ctrlCh *createTestCtrlChannel
}

func (self *createTestEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{Token: "test-router"}
}

func (self *createTestEnv) GetNetworkControllers() routerEnv.NetworkControllers {
	return &testCtrls{ctrlCh: self.ctrlCh}
}

func (self *createTestEnv) GetCtrlRateLimiter() rate.AdaptiveRateLimitTracker {
	return rate.NoOpAdaptiveRateLimitTracker{}
}

// noCtrlTestEnv reaches no controller, so a terminator update fails before anything is sent.
type noCtrlTestEnv struct {
	routerEnv.RouterEnv
}

func (self *noCtrlTestEnv) GetNetworkControllers() routerEnv.NetworkControllers {
	return &testCtrls{}
}

// configuredHostingContext supplies the accessors establishTerminator and inspect read, with a
// configured cost and precedence.
type configuredHostingContext struct {
	tunnel.HostingContext
	options *ziti.ListenOptions
}

func (self *configuredHostingContext) ServiceName() string                { return "test-service" }
func (self *configuredHostingContext) ServiceId() string                  { return "test-service-id" }
func (self *configuredHostingContext) ListenOptions() *ziti.ListenOptions { return self.options }

// Test_tunnelTerminator_createCarriesHealthUpdates verifies a create carries the configured cost and
// precedence until a health check update replaces them, and the updated values from then on, even when
// the update itself fails. Inspect reports the same values.
func Test_tunnelTerminator_createCarriesHealthUpdates(t *testing.T) {
	req := require.New(t)

	ctrlCh := &createTestCtrlChannel{}
	registry := &HostedServiceRegistry{
		establishSet: map[string]*tunnelTerminator{},
		terminators:  cmap.New[*tunnelTerminator](),
		env:          &createTestEnv{ctrlCh: ctrlCh},
	}

	hostCtx := &configuredHostingContext{options: &ziti.ListenOptions{Cost: 7, Precedence: ziti.PrecedenceRequired}}
	terminator := newTunnelTerminator("t1", &fabricProvider{env: &noCtrlTestEnv{}}, hostCtx)
	registry.terminators.Set(terminator.id, terminator)

	sendCreate := func() *edge_ctrl_pb.CreateTunnelTerminatorRequestV2 {
		terminator.operationActive.Store(false)
		registry.establishSet[terminator.id] = terminator
		registry.evaluateEstablishQueue()
		request := &edge_ctrl_pb.CreateTunnelTerminatorRequestV2{}
		req.NoError(proto.Unmarshal(ctrlCh.lastSent.Body, request))
		return request
	}

	request := sendCreate()
	req.Equal(uint32(7), request.Cost)
	req.Equal(edge_ctrl_pb.TerminatorPrecedence_Required, request.Precedence)

	req.Error(terminator.UpdateCostAndPrecedence(20, edge.PrecedenceFailed))
	request = sendCreate()
	req.Equal(uint32(20), request.Cost)
	req.Equal(edge_ctrl_pb.TerminatorPrecedence_Failed, request.Precedence)

	req.Error(terminator.UpdateCost(30))
	request = sendCreate()
	req.Equal(uint32(30), request.Cost)
	req.Equal(edge_ctrl_pb.TerminatorPrecedence_Failed, request.Precedence, "a cost update leaves precedence as it is")

	evt := &inspectTerminatorsEvent{done: make(chan struct{})}
	evt.handle(registry)
	entries := *evt.result.Load()
	req.Len(entries, 1)
	req.Equal(uint16(30), entries[0].Cost)
	req.Equal(ziti.PrecedenceFailed.String(), entries[0].Precedence)
}
