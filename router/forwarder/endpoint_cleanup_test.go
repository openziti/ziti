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
	"fmt"
	"sync"
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

// hasAddresses reports whether addresses holds exactly want, in any order.
func hasAddresses(addresses []xgress.Address, want ...xgress.Address) bool {
	if len(addresses) != len(want) {
		return false
	}
	for _, w := range want {
		found := false
		for _, a := range addresses {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TestLinkDestinationToCircuitConcurrent covers both endpoints of a single-router circuit
// registering at once, which is what happens when a circuit's dialer and terminator are hosted by
// the same router. Losing an address strands its destination: it is never unrouted, and once the
// circuit's forward table is removed nothing can reach it again.
func TestLinkDestinationToCircuitConcurrent(t *testing.T) {
	const rounds = 2000

	for i := 0; i < rounds; i++ {
		dt := newDestinationTable()
		circuitId := fmt.Sprintf("c-%d", i)
		dialer := xgress.Address(fmt.Sprintf("dialer-%d", i))
		terminator := xgress.Address(fmt.Sprintf("terminator-%d", i))

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		for _, addr := range []xgress.Address{dialer, terminator} {
			go func(addr xgress.Address) {
				defer wg.Done()
				<-start
				dt.linkDestinationToCircuit(circuitId, addr)
			}(addr)
		}
		close(start)
		wg.Wait()

		addresses, found := dt.getAddressesForCircuit(circuitId)
		if !found || !hasAddresses(addresses, dialer, terminator) {
			t.Fatalf("round %d: expected both endpoints linked, got %v (found=%v)", i, addresses, found)
		}
	}
}

// TestUnregisterDestinationsConcurrentRegistration covers an endpoint registering while the whole
// circuit is being torn down, which the ordinary xgress close handler reaches via EndCircuit.
//
// Whichever way the two interleave, the arriving endpoint must end up either retired along with
// the rest of the circuit, or still reachable through it. Being left in the destination table
// while unreachable from any circuit is the stranded state: nothing will ever unroute it, and no
// later teardown can find it.
func TestUnregisterDestinationsConcurrentRegistration(t *testing.T) {
	const rounds = 2000

	for i := 0; i < rounds; i++ {
		f := &Forwarder{destinations: newDestinationTable()}
		circuitId := fmt.Sprintf("c-%d", i)
		established := xgress.Address(fmt.Sprintf("established-%d", i))
		arriving := xgress.Address(fmt.Sprintf("arriving-%d", i))

		f.RegisterDestination(circuitId, established, &fakeXgressDestination{})

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			f.UnregisterDestinations(circuitId)
		}()
		go func() {
			defer wg.Done()
			<-start
			f.RegisterDestination(circuitId, arriving, &fakeXgressDestination{})
		}()
		close(start)
		wg.Wait()

		if !f.HasDestination(arriving) {
			continue // swept along with the circuit
		}

		addresses, found := f.destinations.getAddressesForCircuit(circuitId)
		if !found || !hasAddresses(addresses, arriving) {
			t.Fatalf("round %d: arriving endpoint is registered but unreachable from its circuit, got %v (found=%v)",
				i, addresses, found)
		}
	}
}

// TestLinkAndUnlinkDestinationConcurrent covers one endpoint retiring while another registers
// against the same circuit, which is what a reroute looks like: a disconnected side is cleaned up
// while the side that reconnected links itself in. Either order leaves only the new endpoint, so
// seeing the retired address survive, or the new one vanish, means an update was lost.
func TestLinkAndUnlinkDestinationConcurrent(t *testing.T) {
	const rounds = 2000

	for i := 0; i < rounds; i++ {
		dt := newDestinationTable()
		circuitId := fmt.Sprintf("c-%d", i)
		retiring := xgress.Address(fmt.Sprintf("retiring-%d", i))
		arriving := xgress.Address(fmt.Sprintf("arriving-%d", i))

		dt.linkDestinationToCircuit(circuitId, retiring)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			dt.unlinkDestinationFromCircuit(circuitId, retiring)
		}()
		go func() {
			defer wg.Done()
			<-start
			dt.linkDestinationToCircuit(circuitId, arriving)
		}()
		close(start)
		wg.Wait()

		addresses, found := dt.getAddressesForCircuit(circuitId)
		if !found || !hasAddresses(addresses, arriving) {
			t.Fatalf("round %d: expected only the arriving endpoint linked, got %v (found=%v)", i, addresses, found)
		}
	}
}
