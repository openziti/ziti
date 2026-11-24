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

	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

func Test_CleanupXgressCircuit(t *testing.T) {
	t.Run("retires only the closing endpoint's destination", func(t *testing.T) {
		req := require.New(t)

		// arrange
		fwd := &recordingForwarder{}
		conn := &edgeClientConn{forwarder: fwd, xgCircuits: cmap.New[*xgEdgeForwarder]()}
		edgeForwarder := &xgEdgeForwarder{
			edgeClientConn: conn,
			circuitId:      "circuit-1",
			address:        "dialer-address",
			originator:     xgress.Initiator,
			ctrlId:         "ctrl-1",
		}
		conn.xgCircuits.Set("circuit-1", edgeForwarder)

		// act
		conn.cleanupXgressCircuit(edgeForwarder)

		// assert: ending the circuit would also remove the other endpoint of a single-router circuit
		req.Equal([]unregisteredDestination{{circuitId: "circuit-1", address: "dialer-address", destination: edgeForwarder}}, fwd.unregistered)
		req.Empty(fwd.endedCircuits, "cleaning up one endpoint must not end the whole circuit")
		req.False(conn.xgCircuits.Has("circuit-1"), "the closing endpoint must leave the connection's circuits")
	})
}

// unregisteredDestination is one UnregisterDestination call seen by recordingForwarder.
type unregisteredDestination struct {
	circuitId   string
	address     xgress.Address
	destination env.Destination
}

// recordingForwarder records destination unregistrations and circuit ends. Its other forwarder
// methods are unimplemented and panic if called.
type recordingForwarder struct {
	env.Forwarder
	unregistered  []unregisteredDestination
	endedCircuits []string
}

func (self *recordingForwarder) UnregisterDestination(circuitId string, address xgress.Address, destination env.Destination) {
	self.unregistered = append(self.unregistered, unregisteredDestination{circuitId: circuitId, address: address, destination: destination})
}

func (self *recordingForwarder) EndCircuit(circuitId string) {
	self.endedCircuits = append(self.endedCircuits, circuitId)
}

func (self *recordingForwarder) ReportCircuitEndpointFault(string, string, ctrl_pb.FaultSubject) {}
