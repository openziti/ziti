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

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestReporter builds a reporter with the given queue depth and dial function,
// bypassing newReporter so no Ziti context is needed. Callers use stop, not close.
func newTestReporter(t *testing.T, depth int, dial func() (net.Conn, error)) *reporter {
	t.Helper()
	r := &reporter{
		clientId:            "driver1",
		events:              make(chan trafficEvent, depth),
		closeStartNotify:    make(chan struct{}),
		closeCompleteNotify: make(chan struct{}),
		dialResults:         dial,
		writeTimeout:        100 * time.Millisecond,
	}
	go r.run()
	return r
}

// collectEvents reads newline-delimited trafficEvents off conn until it closes.
func collectEvents(conn net.Conn, out *[]trafficEvent, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var evt trafficEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}
		mu.Lock()
		*out = append(*out, evt)
		mu.Unlock()
	}
}

// TestReporterDeliversEvents checks the happy path: queued events reach the wire.
func TestReporterDeliversEvents(t *testing.T) {
	client, server := net.Pipe()

	var mu sync.Mutex
	var got []trafficEvent
	var wg sync.WaitGroup
	wg.Add(1)
	go collectEvents(server, &got, &mu, &wg)

	r := newTestReporter(t, 16, func() (net.Conn, error) { return client, nil })

	r.send(trafficEvent{Type: "dial", Status: "ok", ClientId: "target1"})
	r.send(trafficEvent{Type: "dial", Status: "error", ClientId: "target2"})
	r.stop()

	client.Close()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(got), got)
	}
	if got[0].ClientId != "target1" || got[1].ClientId != "target2" {
		t.Fatalf("events out of order or wrong: %+v", got)
	}
}

// TestReporterSendNeverBlocks is the core of the fix: a results connection that
// never accepts a write must not stall the caller of send. Before the queue was
// introduced, send did the write inline with no deadline, so the traffic loops
// blocked behind a wedged results circuit.
func TestReporterSendNeverBlocks(t *testing.T) {
	// net.Pipe is unbuffered and nothing reads the far side, so the sender's
	// write parks until its deadline expires.
	client, server := net.Pipe()
	defer server.Close()

	r := newTestReporter(t, 4, func() (net.Conn, error) { return client, nil })
	defer r.stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// More events than the queue holds; the excess must be dropped, not block.
		for i := 0; i < 100; i++ {
			r.send(trafficEvent{Type: "dial", Status: "ok", ClientId: "target1"})
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("send blocked; the traffic loop would have stalled")
	}

	if r.dropped.Load() == 0 {
		t.Fatal("expected dropped events to be counted")
	}
}

// TestReporterReportsDropsOnRecovery checks that a reporting outage is not silent:
// once delivery works again, the collector is told how many events were lost, as
// an error event so validation fails rather than seeing a clean run.
func TestReporterReportsDropsOnRecovery(t *testing.T) {
	client, server := net.Pipe()

	var mu sync.Mutex
	var got []trafficEvent
	var wg sync.WaitGroup
	wg.Add(1)
	go collectEvents(server, &got, &mu, &wg)

	// Start with dialing broken so nothing can be delivered.
	var dialMu sync.Mutex
	dialOk := false
	r := newTestReporter(t, 2, func() (net.Conn, error) {
		dialMu.Lock()
		defer dialMu.Unlock()
		if !dialOk {
			return nil, errors.New("results service unreachable")
		}
		return client, nil
	})

	// Overfill the queue so events are dropped and counted.
	for i := 0; i < 50; i++ {
		r.send(trafficEvent{Type: "dial", Status: "ok", ClientId: "target1"})
	}

	deadline := time.Now().Add(5 * time.Second)
	for r.dropped.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if r.dropped.Load() == 0 {
		t.Fatal("expected drops while the results service was unreachable")
	}

	// Restore delivery, then emit one more event to carry the drop report.
	dialMu.Lock()
	dialOk = true
	dialMu.Unlock()

	r.send(trafficEvent{Type: "dial", Status: "ok", ClientId: "target1"})
	r.stop()

	client.Close()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	var drop *trafficEvent
	for i := range got {
		if got[i].Type == "results-drop" {
			drop = &got[i]
			break
		}
	}
	if drop == nil {
		t.Fatalf("expected a results-drop event after recovery, got %+v", got)
	}
	if drop.Status != "error" {
		t.Errorf("drop report must be an error event so validation fails, got status %q", drop.Status)
	}
	if drop.ClientId != "driver1" {
		t.Errorf("drop report must be attributed to the driver, got %q", drop.ClientId)
	}
}

// TestReporterKeepsDropsRecordedDuringReport pins the reason the reported count is
// subtracted rather than the counter being cleared: send is called from the traffic
// loops, so drops can land while a report is on the wire. Clearing would discard
// them.
func TestReporterKeepsDropsRecordedDuringReport(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	var mu sync.Mutex
	var got []trafficEvent
	var wg sync.WaitGroup
	wg.Add(1)
	go collectEvents(server, &got, &mu, &wg)

	r := &reporter{
		clientId:            "driver1",
		events:              make(chan trafficEvent, 4),
		closeStartNotify:    make(chan struct{}),
		closeCompleteNotify: make(chan struct{}),
		writeTimeout:        100 * time.Millisecond,
	}

	// Simulate three further drops landing while the report for the first five is
	// being written, by bumping the counter from inside the dial the report uses.
	r.dialResults = func() (net.Conn, error) {
		r.dropped.Add(3)
		return client, nil
	}

	r.dropped.Store(5)
	r.deliver(trafficEvent{Type: "dial", Status: "ok", ClientId: "target1"})

	if got := r.dropped.Load(); got != 3 {
		t.Fatalf("drops recorded during the report must survive, want 3, got %d", got)
	}

	r.closeConn()
	client.Close()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 || got[0].Type != "results-drop" {
		t.Fatalf("expected a results-drop event first, got %+v", got)
	}
	if !strings.Contains(got[0].Error, "dropped 5 ") {
		t.Errorf("report must name the 5 it accounted for, got %q", got[0].Error)
	}
}

// TestReporterRetainsDropCountWhenReportFails checks that a drop report lost in
// transit does not take the count with it; the next attempt must still carry it.
func TestReporterRetainsDropCountWhenReportFails(t *testing.T) {
	r := &reporter{
		clientId:            "driver1",
		events:              make(chan trafficEvent, 4),
		closeStartNotify:    make(chan struct{}),
		closeCompleteNotify: make(chan struct{}),
		writeTimeout:        100 * time.Millisecond,
		dialResults: func() (net.Conn, error) {
			return nil, errors.New("results service unreachable")
		},
	}

	r.dropped.Store(7)
	// deliver is called on the sender goroutine; drive it directly so the
	// failure path is deterministic.
	r.deliver(trafficEvent{Type: "dial", Status: "ok", ClientId: "target1"})

	if got := r.dropped.Load(); got != 7 {
		t.Fatalf("drop count must survive a failed report, want 7, got %d", got)
	}
}
