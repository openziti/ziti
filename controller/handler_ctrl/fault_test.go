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
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/version"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

// faultTestConfig is the minimal network.Config needed to stand up a network for the fault handler tests.
type faultTestConfig struct {
	ctx             *model.TestContext
	options         *config.NetworkConfig
	metricsRegistry metrics.Registry
	closeNotify     chan struct{}
}

func (self *faultTestConfig) GetId() *identity.TokenId             { return &identity.TokenId{Token: "test"} }
func (self *faultTestConfig) GetMetricsRegistry() metrics.Registry { return self.metricsRegistry }
func (self *faultTestConfig) GetOptions() *config.NetworkConfig    { return self.options }
func (self *faultTestConfig) GetDb() boltz.Db                      { return self.ctx.GetDb() }
func (self *faultTestConfig) GetEventDispatcher() event.Dispatcher { return event.DispatcherMock{} }
func (self *faultTestConfig) GetCloseNotify() <-chan struct{}      { return self.closeNotify }
func (self *faultTestConfig) IsRaftEnabled() bool                  { return false }
func (self *faultTestConfig) IsRaftLeader() bool                   { return false }

func (self *faultTestConfig) GetCommandDispatcher() command.Dispatcher {
	return &command.LocalDispatcher{Limiter: command.NoOpRateLimiter{}}
}

func (self *faultTestConfig) GetVersionProvider() versions.VersionProvider {
	return version.GetCmdBuildInfo()
}

// faultTestSender discards everything sent to it.
type faultTestSender struct {
	channel.Sender
}

func (*faultTestSender) Send(channel.Sendable) error { return nil }

// faultTestCtrlChannel is a router control channel which accepts and discards the unroutes and forwarded
// faults the controller sends while handling a fault.
type faultTestCtrlChannel struct {
	ctrlchan.CtrlChannel
}

func (*faultTestCtrlChannel) GetDefaultSender() channel.Sender      { return &faultTestSender{} }
func (*faultTestCtrlChannel) GetHighPrioritySender() channel.Sender { return &faultTestSender{} }

// faultTestChannel is the channel a fault arrives on. The handler only uses it to label its log output.
type faultTestChannel struct {
	channel.Channel
}

func (*faultTestChannel) Label() string { return "test" }

func newFaultTestNetwork(t *testing.T) *network.Network {
	ctx := model.NewTestContext(t)
	t.Cleanup(ctx.Cleanup)

	cfg := &faultTestConfig{
		ctx:             ctx,
		options:         config.DefaultNetworkConfig(),
		metricsRegistry: metrics.NewRegistry("test", nil),
		closeNotify:     make(chan struct{}),
	}
	t.Cleanup(func() { close(cfg.closeNotify) })

	n, err := network.NewNetwork(cfg, ctx)
	require.NoError(t, err)
	return n
}

func newFaultTestRouter(id string) *model.Router {
	return model.NewRouterForTest(id, "", nil, &faultTestCtrlChannel{}, 0, false)
}

func reportFault(n *network.Network, reporter *model.Router, subject ctrl_pb.FaultSubject, id string) {
	newFaultHandler(reporter, n).handleFault(nil, &faultTestChannel{}, &ctrl_pb.Fault{Subject: subject, Id: id})
}

// Test_faultHandler_xgressFaultOwnership checks that an ingress or egress fault only removes the circuit when
// it is reported by the router hosting that end of the circuit.
func Test_faultHandler_xgressFaultOwnership(t *testing.T) {
	n := newFaultTestNetwork(t)

	ingress := newFaultTestRouter("ingress")
	transit := newFaultTestRouter("transit")
	egress := newFaultTestRouter("egress")
	outsider := newFaultTestRouter("outsider")

	const circuitId = "circuit"
	addCircuit := func(nodes ...*model.Router) {
		n.Circuit.Add(&model.Circuit{
			Id:         circuitId,
			ServiceId:  "svc",
			Terminator: &model.RoutingTerminator{Terminator: &model.Terminator{}},
			Path:       &model.Path{Nodes: nodes},
		})
	}

	tests := []struct {
		name     string
		reporter *model.Router
		subject  ctrl_pb.FaultSubject
		removed  bool
	}{
		{"ingress fault from the ingress router", ingress, ctrl_pb.FaultSubject_IngressFault, true},
		{"egress fault from the egress router", egress, ctrl_pb.FaultSubject_EgressFault, true},
		{"ingress fault from a router not on the path", outsider, ctrl_pb.FaultSubject_IngressFault, false},
		{"egress fault from a router not on the path", outsider, ctrl_pb.FaultSubject_EgressFault, false},
		{"ingress fault from a transit router", transit, ctrl_pb.FaultSubject_IngressFault, false},
		{"egress fault from a transit router", transit, ctrl_pb.FaultSubject_EgressFault, false},
		{"ingress fault from the egress router", egress, ctrl_pb.FaultSubject_IngressFault, false},
		{"egress fault from the ingress router", ingress, ctrl_pb.FaultSubject_EgressFault, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addCircuit(ingress, transit, egress)
			t.Cleanup(func() {
				if circuit, found := n.GetCircuit(circuitId); found {
					n.Circuit.Remove(circuit)
				}
			})

			reportFault(n, test.reporter, test.subject, circuitId)

			_, found := n.GetCircuit(circuitId)
			require.Equal(t, test.removed, !found)
		})
	}

	t.Run("a single router circuit accepts both faults from its router", func(t *testing.T) {
		for _, subject := range []ctrl_pb.FaultSubject{ctrl_pb.FaultSubject_IngressFault, ctrl_pb.FaultSubject_EgressFault} {
			addCircuit(ingress)
			reportFault(n, ingress, subject, circuitId)
			_, found := n.GetCircuit(circuitId)
			require.False(t, found, "%s from the only router on the path must remove the circuit", subject)
		}
	})

	t.Run("a circuit which is still being built is left alone", func(t *testing.T) {
		addCircuit()
		t.Cleanup(func() {
			if circuit, found := n.GetCircuit(circuitId); found {
				n.Circuit.Remove(circuit)
			}
		})

		reportFault(n, outsider, ctrl_pb.FaultSubject_IngressFault, circuitId)

		_, found := n.GetCircuit(circuitId)
		require.True(t, found, "a reserved circuit has no ingress router yet")
	})

	t.Run("a fault for an unknown circuit is tolerated", func(t *testing.T) {
		reportFault(n, outsider, ctrl_pb.FaultSubject_IngressFault, "no-such-circuit")
	})
}

// Test_faultHandler_linkFaultOwnership checks that a link fault only removes the link when it is reported by
// one of the two routers the link connects.
func Test_faultHandler_linkFaultOwnership(t *testing.T) {
	n := newFaultTestNetwork(t)

	src := newFaultTestRouter("src")
	dst := newFaultTestRouter("dst")
	outsider := newFaultTestRouter("outsider")

	const linkId = "link"

	tests := []struct {
		name     string
		reporter *model.Router
		subject  ctrl_pb.FaultSubject
		removed  bool
	}{
		{"link fault from the dialing router", src, ctrl_pb.FaultSubject_LinkFault, true},
		{"link fault from the listening router", dst, ctrl_pb.FaultSubject_LinkFault, true},
		{"duplicate link from the dialing router", src, ctrl_pb.FaultSubject_LinkDuplicate, true},
		{"link fault from a router which is not an endpoint", outsider, ctrl_pb.FaultSubject_LinkFault, false},
		{"duplicate link from a router which is not an endpoint", outsider, ctrl_pb.FaultSubject_LinkDuplicate, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			link := model.NewTestLink(linkId, src, dst)
			n.Link.Add(link)
			t.Cleanup(func() { n.Link.Remove(link) })

			reportFault(n, test.reporter, test.subject, linkId)

			_, found := n.GetLink(linkId)
			require.Equal(t, test.removed, !found)
			if !test.removed {
				require.NotEqual(t, model.Failed, link.CurrentState().Mode, "a rejected fault must not fail the link")
			}
		})
	}

	t.Run("a fault for an unknown link is tolerated", func(t *testing.T) {
		reportFault(n, outsider, ctrl_pb.FaultSubject_LinkFault, "no-such-link")
	})
}
