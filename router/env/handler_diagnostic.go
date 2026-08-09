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
	"runtime"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/stackdump"
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

	// slowHandlerStackDumpMaxSignatures caps how many distinct situations one process will write a file for.
	// Dumps under saturation repeat, so a limit on distinct ones keeps ten different pictures where a limit on
	// total dumps kept the same picture ten times. Repeats are still counted, they just cost a log line rather
	// than a file. A ceiling of this x slowHandlerStackDumpMaxBytes is what makes the disk cost knowable, and a
	// soak that saturated for days once filled a controller's volume without one.
	slowHandlerStackDumpMaxSignatures = 10

	// Stack snapshots start at the smaller size and grow until the snapshot fits, because a truncated dump
	// hides exactly what is wanted: with a fixed 1MB buffer, a controller holding thousands of goroutines cut
	// off before the one holding the lock the rest were queued behind.
	slowHandlerStackDumpInitialBytes = 1 << 20
	slowHandlerStackDumpMaxBytes     = 8 << 20

	// slowHandlerStackDumpMinGroup is how many goroutines must share a stack for it to count toward a dump's
	// signature. A convoy is many goroutines in one place; anything held by fewer is the incidental difference
	// between one moment and the next, and counting it would make every dump distinct.
	slowHandlerStackDumpMinGroup = 5

	// slowHandlerStackDumpDir is where each stack dump is written, one file
	// per dump, relative to the process's CWD. Falls back to logging the
	// stack inline at warn level if the directory cannot be written.
	slowHandlerStackDumpDir = "logs"
)

// stackRecorder captures the dumps, deduplicating them so repeats are counted rather than written again.
var stackRecorder = stackdump.NewRecorder(stackdump.Config{
	Dir:           slowHandlerStackDumpDir,
	FilePrefix:    "slow-handler",
	Interval:      slowHandlerStackDumpInterval,
	MaxSignatures: slowHandlerStackDumpMaxSignatures,
	InitialBytes:  slowHandlerStackDumpInitialBytes,
	MaxBytes:      slowHandlerStackDumpMaxBytes,
	MinGroup:      slowHandlerStackDumpMinGroup,
})

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
		disarm := armStackDumpWatchdog(contentType, ch)
		inner.HandleReceive(m, ch)
		disarm()
		recordHandlerDuration(contentType, ch, time.Since(start))
	})
}

// armStackDumpWatchdog schedules a stack snapshot for a handler that has not returned by the dump threshold,
// and returns the function that stands it down. The caller must always call that.
//
// The snapshot has to be taken while the handler is still in the handler, which is the whole point of taking
// one: dumping after it returns records the goroutine unwinding this diagnostic rather than whatever it was
// blocked on, and leaves the reason to be guessed at from the other goroutines. A timer per message is the cost
// of that, and this is temporary code on a path that already pays for a wall-clock measurement.
func armStackDumpWatchdog(contentType int32, ch channel.Channel) func() {
	timer := time.AfterFunc(slowHandlerStackDumpThreshold, func() {
		writeSlowHandlerStacks(contentType, ch)
	})
	return func() {
		timer.Stop()
	}
}

// writeSlowHandlerStacks snapshots every goroutine, or counts the snapshot against one already on file. Called
// from the watchdog, so the handler it is reporting on is still running.
func writeSlowHandlerStacks(contentType int32, ch channel.Channel) {
	log := pfxlog.Logger().
		WithField("channelId", ch.Id()).
		WithField("contentType", contentType).
		WithField("thresholdMs", slowHandlerStackDumpThreshold.Milliseconds()).
		WithField("goroutines", runtime.NumGoroutine())

	// Which handler failed to return is the one thing the dump cannot show, since its goroutine cannot be
	// picked out of the many others also inside wrapped handlers.
	result := stackRecorder.Capture(fmt.Sprintf("contentType=%d", contentType))
	if result.Signature != "" {
		log = log.WithField("stackSignature", result.Signature).
			WithField("stackBytes", result.Bytes).
			WithField("distinctSignatures", result.DistinctSignatures)
	}

	switch result.Outcome {
	case stackdump.RateLimited:
		log.Warn("handler still running past stack dump threshold (no dump, one was taken recently)")
	case stackdump.Repeat:
		log.WithField("occurrences", result.Occurrences).
			WithField("stackFile", result.Path).
			Warn("handler still running past stack dump threshold, in a situation already on file")
	case stackdump.SignatureLimit:
		log.Warn("handler still running past stack dump threshold in a new situation (no dump, this process has " +
			"written as many distinct situations as it is allowed)")
	case stackdump.WriteFailed:
		log.WithError(result.Err).
			WithField("stack", string(result.Stack)).
			Warn("handler still running past stack dump threshold; could not write stack file, logging inline")
	default:
		log.WithField("stackFile", result.Path).
			Warn("handler still running past stack dump threshold; goroutine stacks written to file")
	}
}

// recordHandlerDuration logs how long a handler took, once it has returned. Stack dumps are not taken here:
// see armStackDumpWatchdog.
func recordHandlerDuration(contentType int32, ch channel.Channel, d time.Duration) {
	if d < slowHandlerWarnThreshold {
		return
	}
	// Goroutine count is recorded because this measures wall time around a handler and so cannot tell work
	// from deschedule. A handler that only unmarshals and hands off cannot spend 100ms working, so a high
	// count alongside a large duration says the host is starved rather than the handler slow. Without it,
	// every duration here reads as if the handler earned it.
	pfxlog.Logger().
		WithField("channelId", ch.Id()).
		WithField("contentType", contentType).
		WithField("durationMs", d.Milliseconds()).
		WithField("goroutines", runtime.NumGoroutine()).
		Warn("slow channel handler")
}
