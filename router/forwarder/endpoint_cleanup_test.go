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

package forwarder

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/xgress"
)

// fakeXgressDestination is a minimal XgressDestination that records whether Unrouted was called.
type fakeXgressDestination struct {
	unrouted atomic.Bool
}

func (f *fakeXgressDestination) SendPayload(*xgress.Payload, time.Duration, xgress.PayloadType) error {
	return nil
}
func (f *fakeXgressDestination) SendAcknowledgement(*xgress.Acknowledgement) error { return nil }
func (f *fakeXgressDestination) SendControl(*xgress.Control) error                 { return nil }
func (f *fakeXgressDestination) InspectCircuit(*xgress.CircuitInspectDetail)       {}
func (f *fakeXgressDestination) GetDestinationType() string                        { return "test" }
func (f *fakeXgressDestination) Unrouted()                                         { f.unrouted.Store(true) }
func (f *fakeXgressDestination) GetTimeOfLastRxFromLink() int64                    { return 0 }

func waitUnrouted(t *testing.T, d *fakeXgressDestination) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if d.unrouted.Load() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("expected Unrouted to have been called")
}

func TestUnlinkDestinationFromCircuit(t *testing.T) {
	dt := newDestinationTable()
	dt.linkDestinationToCircuit("c1", "a")
	dt.linkDestinationToCircuit("c1", "b")

	if remaining := dt.unlinkDestinationFromCircuit("c1", "a"); !remaining {
		t.Fatal("expected addresses to remain after removing one of two")
	}
	addrs, found := dt.getAddressesForCircuit("c1")
	if !found || len(addrs) != 1 || addrs[0] != "b" {
		t.Fatalf("expected only [b] to remain, got %v (found=%v)", addrs, found)
	}

	if remaining := dt.unlinkDestinationFromCircuit("c1", "b"); remaining {
		t.Fatal("expected no addresses to remain after removing the last")
	}
	if _, found := dt.getAddressesForCircuit("c1"); found {
		t.Fatal("expected circuit entry to be removed once its last address was unlinked")
	}

	// unlinking from an unknown circuit is a no-op that reports no remaining addresses
	if remaining := dt.unlinkDestinationFromCircuit("missing", "x"); remaining {
		t.Fatal("expected false when unlinking from an unknown circuit")
	}
}

func TestUnregisterDestinationIsEndpointScoped(t *testing.T) {
	f := &Forwarder{destinations: newDestinationTable()}

	dialer := &fakeXgressDestination{}
	terminator := &fakeXgressDestination{}
	f.RegisterDestination("c1", "dialer", dialer)
	f.RegisterDestination("c1", "terminator", terminator)

	// closing the dialer endpoint must retire only its destination, leaving the co-located
	// terminator (the single-router-circuit case) intact.
	f.UnregisterDestination("c1", "dialer")

	if f.HasDestination("dialer") {
		t.Fatal("expected dialer destination to be removed")
	}
	if !f.HasDestination("terminator") {
		t.Fatal("expected terminator destination to be preserved")
	}
	waitUnrouted(t, dialer)
	if terminator.unrouted.Load() {
		t.Fatal("terminator must not be unrouted when only the dialer endpoint closed")
	}
	if addrs, found := f.destinations.getAddressesForCircuit("c1"); !found || len(addrs) != 1 || addrs[0] != "terminator" {
		t.Fatalf("expected circuit to still reference [terminator], got %v (found=%v)", addrs, found)
	}

	// closing the remaining terminator endpoint removes the circuit entry entirely.
	f.UnregisterDestination("c1", "terminator")
	if f.HasDestination("terminator") {
		t.Fatal("expected terminator destination to be removed")
	}
	waitUnrouted(t, terminator)
	if _, found := f.destinations.getAddressesForCircuit("c1"); found {
		t.Fatal("expected circuit entry to be gone after its last endpoint was removed")
	}
}
