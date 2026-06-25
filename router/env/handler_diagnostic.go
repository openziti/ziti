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

// TEMPORARY DIAGNOSTIC CODE
//
// This file exists to help identify why ctrl-channel send back-pressure is
// causing the controller's pool.router.messaging p99 to hit the 1s send
// timeout (see link-bugs.md #2). It wraps every channel receive handler with
// a timer and logs slow handlers on the router rx path.
//
// Remove this file (and the WithSlowHandlerDiagnostic call sites in
// router/accepter.go and router/env/ctrls.go) once we have a root cause for
// the back-pressure and have either fixed it or have proper production
// instrumentation in its place.

package env

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
)

const (
	// slowHandlerWarnThreshold logs a warn line with content type and duration.
	// Most ctrl-channel handlers should complete in microseconds; >100ms is
	// either real work blocking us (DB write, lock contention) or a sign that
	// the read goroutine is starved.
	slowHandlerWarnThreshold = 100 * time.Millisecond

	// slowHandlerStackDumpThreshold also dumps all goroutine stacks. Catches
	// cases where another goroutine is holding a lock our handler needs, or
	// where GC has the world stopped for an extended window.
	slowHandlerStackDumpThreshold = 500 * time.Millisecond

	// slowHandlerStackDumpInterval bounds how often we dump stacks globally.
	// runtime.Stack(buf, true) snapshots every goroutine; under load that's
	// hundreds of KB of log output and a brief stop-the-world. Once per 10
	// minutes is enough to characterize a wedged handler without flooding.
	slowHandlerStackDumpInterval = 10 * time.Minute

	// slowHandlerStackDumpDir is where each stack dump is written, one file
	// per dump, relative to the process's CWD. Falls back to logging the
	// stack inline at warn level if the directory cannot be written.
	slowHandlerStackDumpDir = "logs"
)

// lastStackDumpNanos holds time.Now().UnixNano() of the most recent stack
// dump. CompareAndSwap is used to claim the next dump slot atomically.
var lastStackDumpNanos atomic.Int64

// WithSlowHandlerDiagnostic wraps a BindHandler so every receive handler it
// registers gets a timing decorator. See file header for removal criteria.
func WithSlowHandlerDiagnostic(inner channel.BindHandler) channel.BindHandler {
	return channel.BindHandlerF(func(binding channel.Binding) error {
		return inner.BindChannel(&slowHandlerBinding{Binding: binding})
	})
}

// slowHandlerBinding decorates a channel.Binding so AddReceiveHandler /
// AddReceiveHandlerF install a timing-wrapped version of the handler. Other
// Binding methods are forwarded unchanged. Typed handlers registered via
// channel.AddReceiveHandlers route through AddReceiveHandler too, so they pick
// up the same timing wrapper.
type slowHandlerBinding struct {
	channel.Binding
}

func (b *slowHandlerBinding) AddReceiveHandler(contentType int32, h channel.ReceiveHandler) {
	b.Binding.AddReceiveHandler(contentType, wrapHandler(contentType, h))
}

func (b *slowHandlerBinding) AddReceiveHandlerF(contentType int32, h channel.ReceiveHandlerF) {
	b.Binding.AddReceiveHandler(contentType, wrapHandler(contentType, h))
}

func wrapHandler(contentType int32, inner channel.ReceiveHandler) channel.ReceiveHandler {
	return channel.ReceiveHandlerF(func(m *channel.Message, ch channel.Channel) {
		start := time.Now()
		inner.HandleReceive(m, ch)
		recordHandlerDuration(contentType, ch, time.Since(start))
	})
}

func recordHandlerDuration(contentType int32, ch channel.Channel, d time.Duration) {
	if d < slowHandlerWarnThreshold {
		return
	}
	log := pfxlog.Logger().
		WithField("channelId", ch.Id()).
		WithField("contentType", contentType).
		WithField("durationMs", d.Milliseconds())
	if d < slowHandlerStackDumpThreshold {
		log.Warn("slow channel handler")
		return
	}
	if !claimStackDumpSlot() {
		log.Warn("slow channel handler (stack dump suppressed by rate limit)")
		return
	}
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	path, writeErr := writeStackDump(buf[:n])
	if writeErr != nil {
		log.WithError(writeErr).
			WithField("stack", string(buf[:n])).
			Warn("slow channel handler; could not write stack file, logging inline")
		return
	}
	log.WithField("stackFile", path).Warn("slow channel handler; goroutine stacks written to file")
}

// writeStackDump writes the goroutine stack snapshot to a per-dump file under
// slowHandlerStackDumpDir, named with a sortable timestamp. Returns the path
// on success.
func writeStackDump(stack []byte) (string, error) {
	if err := os.MkdirAll(slowHandlerStackDumpDir, 0o755); err != nil {
		return "", err
	}
	// Filename-safe timestamp: 2026-05-16T20-30-45.123Z (millisecond
	// precision, ':' replaced with '-' since some tooling dislikes ':' in
	// filenames).
	ts := time.Now().UTC().Format("2006-01-02T15-04-05.000Z")
	name := fmt.Sprintf("slow-handler-%s.stack", ts)
	path := filepath.Join(slowHandlerStackDumpDir, name)
	if err := os.WriteFile(path, stack, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// claimStackDumpSlot returns true if the caller is allowed to emit a stack
// dump now. Uses CompareAndSwap on the last-dump timestamp so simultaneous
// slow handlers across goroutines do not all dump.
func claimStackDumpSlot() bool {
	now := time.Now().UnixNano()
	prev := lastStackDumpNanos.Load()
	if now-prev < int64(slowHandlerStackDumpInterval) {
		return false
	}
	return lastStackDumpNanos.CompareAndSwap(prev, now)
}
