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
	"github.com/stretchr/testify/require"
)

// concurrentRounds is how many times the concurrency tests repeat an interleaving, so that a lost
// update shows up in a single run.
const concurrentRounds = 2000

func Test_UnregisterDestination(t *testing.T) {
	t.Run("a closing endpoint leaves the co-located endpoint of its circuit routed", func(t *testing.T) {
		req := require.New(t)

		// arrange: both endpoints of a single-router circuit
		f := &Forwarder{destinations: newDestinationTable()}
		dialer := &countingDestination{unrouted: &unrouteCounter{want: 1}}
		terminator := &countingDestination{unrouted: &unrouteCounter{want: 1}}
		f.RegisterDestination("c1", "dialer", dialer)
		f.RegisterDestination("c1", "terminator", terminator)

		// act
		f.UnregisterDestination("c1", "dialer", dialer)

		// assert
		req.False(f.HasDestination("dialer"), "the closing endpoint must be removed")
		req.True(f.HasDestination("terminator"), "the co-located endpoint must stay registered")
		addresses, found := f.destinations.getAddressesForCircuit("c1")
		req.True(found, "the circuit must still list its remaining endpoint")
		req.Equal([]xgress.Address{"terminator"}, addresses)
		req.Eventually(dialer.unrouted.reachedWant, time.Second, time.Millisecond, "the closing endpoint must be unrouted")
		req.Zero(terminator.unrouted.count.Load(), "the co-located endpoint must not be unrouted")
	})

	t.Run("the last endpoint of a circuit takes the circuit's entry with it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f := &Forwarder{destinations: newDestinationTable()}
		dialer := &countingDestination{unrouted: &unrouteCounter{want: 1}}
		terminator := &countingDestination{unrouted: &unrouteCounter{want: 1}}
		f.RegisterDestination("c1", "dialer", dialer)
		f.RegisterDestination("c1", "terminator", terminator)
		f.UnregisterDestination("c1", "dialer", dialer)

		// act
		f.UnregisterDestination("c1", "terminator", terminator)

		// assert
		req.False(f.HasDestination("terminator"), "the last endpoint must be removed")
		_, found := f.destinations.getAddressesForCircuit("c1")
		req.False(found, "the circuit's entry must go with its last endpoint")
		req.Eventually(terminator.unrouted.reachedWant, time.Second, time.Millisecond, "the last endpoint must be unrouted")
	})

	t.Run("a destination no longer registered at its address leaves the address's destination in place", func(t *testing.T) {
		req := require.New(t)

		// arrange: the address now holds a different destination than the one being retired
		f := &Forwarder{destinations: newDestinationTable()}
		retired := &countingDestination{unrouted: &unrouteCounter{}}
		current := &countingDestination{unrouted: &unrouteCounter{}}
		f.RegisterDestination("c1", "ingress", retired)
		f.destinations.addDestination("ingress", current)

		// act
		f.UnregisterDestination("c1", "ingress", retired)

		// assert
		destination, found := f.destinations.getDestination("ingress")
		req.True(found, "the address must stay registered")
		req.Same(current, destination, "the address must keep its current destination")
		addresses, found := f.destinations.getAddressesForCircuit("c1")
		req.True(found, "the circuit must still list the address")
		req.Equal([]xgress.Address{"ingress"}, addresses)
		req.Never(current.unrouted.exceededWant, 50*time.Millisecond, 10*time.Millisecond, "the current destination must not be unrouted")
		req.Zero(retired.unrouted.count.Load(), "a destination that was not removed must not be unrouted")
	})

	t.Run("an endpoint removed while its circuit is torn down is unrouted once", func(t *testing.T) {
		req := require.New(t)

		// arrange and act: race the endpoint's removal against the circuit's teardown, many times
		unrouted := &unrouteCounter{want: concurrentRounds}
		for i := range concurrentRounds {
			f := &Forwarder{destinations: newDestinationTable()}
			circuitId := fmt.Sprintf("c-%d", i)
			address := xgress.Address(fmt.Sprintf("endpoint-%d", i))
			destination := &countingDestination{unrouted: unrouted}
			f.RegisterDestination(circuitId, address, destination)

			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go unregisterDestinationOnStart(f, circuitId, address, destination, start, &wg)
			go unregisterDestinationsOnStart(f, circuitId, start, &wg)
			close(start)
			wg.Wait()
		}

		// assert
		req.Eventually(unrouted.reachedWant, 5*time.Second, time.Millisecond, "every endpoint must be unrouted")
		req.Never(unrouted.exceededWant, 100*time.Millisecond, 10*time.Millisecond,
			"an endpoint must be unrouted once, by whichever removal takes it")
	})
}

func Test_UnregisterDestinations(t *testing.T) {
	t.Run("an endpoint registering during teardown is retired or stays reachable from its circuit", func(t *testing.T) {
		req := require.New(t)

		for i := range concurrentRounds {
			// arrange
			f := &Forwarder{destinations: newDestinationTable()}
			circuitId := fmt.Sprintf("c-%d", i)
			established := xgress.Address(fmt.Sprintf("established-%d", i))
			arriving := xgress.Address(fmt.Sprintf("arriving-%d", i))
			f.RegisterDestination(circuitId, established, &countingDestination{unrouted: &unrouteCounter{}})

			// act
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go unregisterDestinationsOnStart(f, circuitId, start, &wg)
			go registerOnStart(f, circuitId, arriving, &countingDestination{unrouted: &unrouteCounter{}}, start, &wg)
			close(start)
			wg.Wait()

			// assert: registered but unreachable from its circuit is the stranded state, which nothing
			// would ever unroute
			if !f.HasDestination(arriving) {
				continue
			}
			addresses, found := f.destinations.getAddressesForCircuit(circuitId)
			req.True(found, "round %d: the circuit must list the arriving endpoint", i)
			req.Equal([]xgress.Address{arriving}, addresses, "round %d: the arriving endpoint must be reachable from its circuit", i)
		}
	})
}

func Test_LinkDestinationToCircuit(t *testing.T) {
	t.Run("endpoints linking concurrently are both kept", func(t *testing.T) {
		req := require.New(t)

		for i := range concurrentRounds {
			// arrange
			dt := newDestinationTable()
			circuitId := fmt.Sprintf("c-%d", i)
			dialer := xgress.Address(fmt.Sprintf("dialer-%d", i))
			terminator := xgress.Address(fmt.Sprintf("terminator-%d", i))

			// act
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go linkOnStart(dt, circuitId, dialer, start, &wg)
			go linkOnStart(dt, circuitId, terminator, start, &wg)
			close(start)
			wg.Wait()

			// assert
			addresses, found := dt.getAddressesForCircuit(circuitId)
			req.True(found, "round %d: the circuit must be listed", i)
			req.ElementsMatch([]xgress.Address{dialer, terminator}, addresses, "round %d: both endpoints must be linked", i)
		}
	})
}

func Test_UnlinkDestinationFromCircuit(t *testing.T) {
	t.Run("the circuit's other addresses stay linked", func(t *testing.T) {
		req := require.New(t)

		// arrange
		dt := newDestinationTable()
		dt.linkDestinationToCircuit("c1", "a")
		dt.linkDestinationToCircuit("c1", "b")

		// act
		dt.unlinkDestinationFromCircuit("c1", "a")

		// assert
		addresses, found := dt.getAddressesForCircuit("c1")
		req.True(found, "the circuit must still be listed")
		req.Equal([]xgress.Address{"b"}, addresses)
	})

	t.Run("unlinking the last address removes the circuit's entry", func(t *testing.T) {
		req := require.New(t)

		// arrange
		dt := newDestinationTable()
		dt.linkDestinationToCircuit("c1", "a")

		// act
		dt.unlinkDestinationFromCircuit("c1", "a")

		// assert
		_, found := dt.getAddressesForCircuit("c1")
		req.False(found, "an empty circuit entry must be removed")
	})

	t.Run("unlinking from an unknown circuit creates no entry", func(t *testing.T) {
		req := require.New(t)

		// arrange
		dt := newDestinationTable()

		// act
		dt.unlinkDestinationFromCircuit("missing", "a")

		// assert
		_, found := dt.getAddressesForCircuit("missing")
		req.False(found)
	})

	t.Run("an address linking while another unlinks is kept", func(t *testing.T) {
		req := require.New(t)

		for i := range concurrentRounds {
			// arrange
			dt := newDestinationTable()
			circuitId := fmt.Sprintf("c-%d", i)
			retiring := xgress.Address(fmt.Sprintf("retiring-%d", i))
			arriving := xgress.Address(fmt.Sprintf("arriving-%d", i))
			dt.linkDestinationToCircuit(circuitId, retiring)

			// act
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go unlinkOnStart(dt, circuitId, retiring, start, &wg)
			go linkOnStart(dt, circuitId, arriving, start, &wg)
			close(start)
			wg.Wait()

			// assert: either order leaves only the arriving address
			addresses, found := dt.getAddressesForCircuit(circuitId)
			req.True(found, "round %d: the circuit must be listed", i)
			req.Equal([]xgress.Address{arriving}, addresses, "round %d: only the arriving address must be linked", i)
		}
	})
}

// unrouteCounter counts Unrouted calls, possibly from several destinations, against the number expected.
type unrouteCounter struct {
	count atomic.Int32
	want  int32
}

// reachedWant reports whether at least the expected number of Unrouted calls have been made.
func (self *unrouteCounter) reachedWant() bool {
	return self.count.Load() >= self.want
}

// exceededWant reports whether more than the expected number of Unrouted calls have been made.
func (self *unrouteCounter) exceededWant() bool {
	return self.count.Load() > self.want
}

// countingDestination is an XgressDestination that counts its Unrouted calls in unrouted and ignores
// everything else.
type countingDestination struct {
	unrouted *unrouteCounter
}

func (self *countingDestination) SendPayload(*xgress.Payload, time.Duration, xgress.PayloadType) error {
	return nil
}

func (self *countingDestination) SendAcknowledgement(*xgress.Acknowledgement) error {
	return nil
}

func (self *countingDestination) SendControl(*xgress.Control) error {
	return nil
}

func (self *countingDestination) InspectCircuit(*xgress.CircuitInspectDetail) {}

func (self *countingDestination) GetDestinationType() string {
	return "test"
}

func (self *countingDestination) Unrouted() {
	self.unrouted.count.Add(1)
}

func (self *countingDestination) GetTimeOfLastRxFromLink() int64 {
	return 0
}

// linkOnStart links address to circuitId once start closes, then marks wg done.
func linkOnStart(dt *destinationTable, circuitId string, address xgress.Address, start <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	<-start
	dt.linkDestinationToCircuit(circuitId, address)
}

// unlinkOnStart unlinks address from circuitId once start closes, then marks wg done.
func unlinkOnStart(dt *destinationTable, circuitId string, address xgress.Address, start <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	<-start
	dt.unlinkDestinationFromCircuit(circuitId, address)
}

// registerOnStart registers destination at address on circuitId once start closes, then marks wg done.
func registerOnStart(f *Forwarder, circuitId string, address xgress.Address, destination XgressDestination, start <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	<-start
	f.RegisterDestination(circuitId, address, destination)
}

// unregisterDestinationOnStart unregisters destination from address once start closes, then marks wg
// done.
func unregisterDestinationOnStart(f *Forwarder, circuitId string, address xgress.Address, destination XgressDestination, start <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	<-start
	f.UnregisterDestination(circuitId, address, destination)
}

// unregisterDestinationsOnStart unregisters every destination of circuitId once start closes, then
// marks wg done.
func unregisterDestinationsOnStart(f *Forwarder, circuitId string, start <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	<-start
	f.UnregisterDestinations(circuitId)
}
