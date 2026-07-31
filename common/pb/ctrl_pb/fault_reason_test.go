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

package ctrl_pb

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// TestFaultReasonZeroValueIsUnspecified guards the load-bearing numbering: a Fault emitted without
// a Reason (an old or unstamped router) must decode to ReasonUnspecified, which the controller
// treats as teardown. Only ChannelClosed is Limbo-eligible, so an absent reason can never
// resurrect a circuit.
func TestFaultReasonZeroValueIsUnspecified(t *testing.T) {
	if FaultReason_ReasonUnspecified != 0 {
		t.Fatalf("ReasonUnspecified must be the proto3 zero value, got %d", FaultReason_ReasonUnspecified)
	}
	if FaultReason_ChannelClosed == 0 {
		t.Fatal("ChannelClosed must not be the zero value, or absent faults would become Limbo-eligible")
	}

	// A Fault with no Reason set, round-tripped through the wire, must decode to ReasonUnspecified.
	encoded, err := proto.Marshal(&Fault{Subject: FaultSubject_IngressFault, Id: "circuit-1"})
	if err != nil {
		t.Fatalf("failed to marshal fault: %v", err)
	}
	decoded := &Fault{}
	if err := proto.Unmarshal(encoded, decoded); err != nil {
		t.Fatalf("failed to unmarshal fault: %v", err)
	}
	if decoded.Reason != FaultReason_ReasonUnspecified {
		t.Fatalf("a fault with no reason must decode to ReasonUnspecified, got %v", decoded.Reason)
	}
}
