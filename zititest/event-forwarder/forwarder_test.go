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
	"errors"
	"net"
	"testing"
	"time"
)

// TestForwardTimesOutAndReconnects is the core of the fix: a destination that
// accepts a connection but never reads must not park forward forever. The write
// has to hit its deadline, the dead connection has to be discarded, and the loop
// has to move on to a redial.
func TestForwardTimesOutAndReconnects(t *testing.T) {
	// net.Pipe is unbuffered and nothing reads the far side, so the write blocks
	// until its deadline expires.
	stalled, peer := net.Pipe()
	defer peer.Close()

	f := &zitiForwarder{
		done:         make(chan struct{}),
		writeTimeout: 100 * time.Millisecond,
		conn:         stalled,
	}

	// The redial ends the test deterministically: reaching it proves the stalled
	// write timed out and was reset, and closing done makes ensureConnected give up
	// rather than retrying for real.
	redialed := false
	f.dial = func() (net.Conn, error) {
		redialed = true
		close(f.done)
		return nil, errors.New("destination unreachable")
	}

	start := time.Now()
	err := f.forward("event")
	elapsed := time.Since(start)

	if !errors.Is(err, errShutdown) {
		t.Fatalf("expected errShutdown once done closed, got %v", err)
	}
	if !redialed {
		t.Fatal("stalled write did not lead to a redial; forward would have parked")
	}
	if f.conn != nil {
		t.Error("timed-out connection must be discarded, not reused")
	}
	if elapsed > 5*time.Second {
		t.Errorf("forward took %s; the write deadline was not applied", elapsed)
	}
}

// TestForwardStopsWhenDoneClosed checks that shutdown is observed on every attempt,
// not only on the dial path. A destination that accepts connections but never reads
// would otherwise keep the loop spinning.
func TestForwardStopsWhenDoneClosed(t *testing.T) {
	f := &zitiForwarder{
		done:         make(chan struct{}),
		writeTimeout: 100 * time.Millisecond,
	}
	f.dial = func() (net.Conn, error) {
		t.Error("forward must not dial after done is closed")
		return nil, errors.New("unreachable")
	}
	close(f.done)

	if err := f.forward("event"); !errors.Is(err, errShutdown) {
		t.Fatalf("expected errShutdown, got %v", err)
	}
}

// TestForwardWritesLine covers the happy path, including that a successful write
// keeps the connection for the next line.
func TestForwardWritesLine(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	f := &zitiForwarder{
		done:         make(chan struct{}),
		writeTimeout: 5 * time.Second,
		conn:         client,
	}
	f.dial = func() (net.Conn, error) {
		t.Error("forward must reuse the live connection rather than redialing")
		return nil, errors.New("unreachable")
	}

	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(server)
		if scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	if err := f.forward("hello"); err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	select {
	case got := <-lines:
		if got != "hello" {
			t.Errorf("expected %q, got %q", "hello", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("line never reached the destination")
	}

	if f.conn == nil {
		t.Error("a successful write must keep the connection")
	}
}
