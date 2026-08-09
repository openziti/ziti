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

// Package diagnostics holds opt-in instrumentation for investigating a running controller or router.
// Everything here is off unless configured on, and is intended to be turned on for a test run or an
// investigation rather than left on in production.
package diagnostics

import (
	"fmt"
	"runtime"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/stackdump"
)

// Defaults for SlowHandlerConfig. They suit finding a wedged handler; an investigation into lock
// contention will want a lower threshold, and one into a rare event a longer interval and more signatures.
const (
	DefaultSlowHandlerWarnThreshold      = 100 * time.Millisecond
	DefaultSlowHandlerStackDumpThreshold = 500 * time.Millisecond
	DefaultSlowHandlerStackDumpInterval  = 10 * time.Minute
	DefaultSlowHandlerMaxSignatures      = 10
	DefaultSlowHandlerMinGroup           = 5
	DefaultSlowHandlerInitialBytes       = 1 << 20
	DefaultSlowHandlerMaxBytes           = 8 << 20
	DefaultSlowHandlerDir                = "logs"
)

// SlowHandlerConfig configures slow channel handler detection. The zero value is disabled; use
// DefaultSlowHandlerConfig and override from there.
type SlowHandlerConfig struct {
	// Enabled decides whether handlers are wrapped at all. When false nothing is installed, so a message
	// costs exactly what it did before.
	Enabled bool

	// WarnThreshold is how long a handler may take before its duration is logged, once it has returned.
	WarnThreshold time.Duration

	// StackDumpThreshold is how long a handler may run before every goroutine's stack is captured, while
	// it is still running. Lower this to catch lock contention; the default finds handlers that are stuck.
	StackDumpThreshold time.Duration

	// StackDumpInterval bounds how often a dump is taken, process-wide. runtime.Stack over every goroutine
	// briefly stops the world and produces hundreds of KB, so this is what keeps a saturated process from
	// dumping continuously.
	StackDumpInterval time.Duration

	// MaxSignatures caps how many distinct situations one process writes a file for. Dumps under saturation
	// repeat, so limiting distinct ones keeps several different pictures where limiting the total kept the
	// same picture several times. Repeats are still counted, they just cost a log line. MaxSignatures times
	// MaxBytes is the ceiling on disk used.
	MaxSignatures int

	// MinGroup is how many goroutines must share a stack for it to count toward a dump's signature. A
	// convoy is many goroutines in one place; fewer is the incidental difference between two moments, and
	// counting it would make every dump distinct.
	MinGroup int

	// InitialBytes and MaxBytes bound the snapshot buffer, which grows from the first until the dump fits.
	// A truncated dump hides what is wanted: a process holding thousands of goroutines can be cut off
	// before the one holding the lock the rest are queued behind.
	InitialBytes int
	MaxBytes     int

	// Dir is where dumps are written, one file each, relative to the process's working directory. If it
	// cannot be written the stack is logged inline instead.
	Dir string
}

// DefaultSlowHandlerConfig returns the defaults, disabled.
func DefaultSlowHandlerConfig() SlowHandlerConfig {
	return SlowHandlerConfig{
		Enabled:            false,
		WarnThreshold:      DefaultSlowHandlerWarnThreshold,
		StackDumpThreshold: DefaultSlowHandlerStackDumpThreshold,
		StackDumpInterval:  DefaultSlowHandlerStackDumpInterval,
		MaxSignatures:      DefaultSlowHandlerMaxSignatures,
		MinGroup:           DefaultSlowHandlerMinGroup,
		InitialBytes:       DefaultSlowHandlerInitialBytes,
		MaxBytes:           DefaultSlowHandlerMaxBytes,
		Dir:                DefaultSlowHandlerDir,
	}
}

// SlowHandlerDetector wraps channel receive handlers so slow ones are reported. A nil detector wraps
// nothing, so a disabled configuration needs no branch at the call site.
type SlowHandlerDetector struct {
	cfg      SlowHandlerConfig
	recorder *stackdump.Recorder
	label    string
}

// NewSlowHandlerDetector returns a detector for the given config, or nil if it is disabled. label names
// the process side in dump filenames, so a controller's and a router's do not collide.
func NewSlowHandlerDetector(label string, cfg SlowHandlerConfig) *SlowHandlerDetector {
	if !cfg.Enabled {
		return nil
	}
	return &SlowHandlerDetector{
		cfg:   cfg,
		label: label,
		recorder: stackdump.NewRecorder(stackdump.Config{
			Dir:           cfg.Dir,
			FilePrefix:    label + "-slow-handler",
			Interval:      cfg.StackDumpInterval,
			MaxSignatures: cfg.MaxSignatures,
			InitialBytes:  cfg.InitialBytes,
			MaxBytes:      cfg.MaxBytes,
			MinGroup:      cfg.MinGroup,
		}),
	}
}

// Wrap decorates inner so every receive handler it registers is timed. A nil detector returns inner
// unchanged, installing nothing.
func (self *SlowHandlerDetector) Wrap(inner channel.BindHandler) channel.BindHandler {
	if self == nil {
		return inner
	}
	return channel.BindHandlerF(func(binding channel.Binding) error {
		return inner.BindChannel(&slowHandlerBinding{Binding: binding, detector: self})
	})
}

// slowHandlerBinding decorates a channel.Binding so AddReceiveHandler and AddReceiveHandlerF install a
// timed version of the handler. Other Binding methods are forwarded unchanged. Typed handlers registered
// via channel.AddReceiveHandlers route through AddReceiveHandler, so they are timed too.
type slowHandlerBinding struct {
	channel.Binding
	detector *SlowHandlerDetector
}

func (self *slowHandlerBinding) AddReceiveHandler(contentType int32, h channel.ReceiveHandler) {
	self.Binding.AddReceiveHandler(contentType, self.detector.wrapHandler(contentType, h))
}

func (self *slowHandlerBinding) AddReceiveHandlerF(contentType int32, h channel.ReceiveHandlerF) {
	self.Binding.AddReceiveHandler(contentType, self.detector.wrapHandler(contentType, h))
}

func (self *SlowHandlerDetector) wrapHandler(contentType int32, inner channel.ReceiveHandler) channel.ReceiveHandler {
	return channel.ReceiveHandlerF(func(m *channel.Message, ch channel.Channel) {
		start := time.Now()
		disarm := self.armStackDumpWatchdog(contentType, ch)
		inner.HandleReceive(m, ch)
		disarm()
		self.recordHandlerDuration(contentType, ch, time.Since(start))
	})
}

// armStackDumpWatchdog schedules a stack snapshot for a handler that has not returned by the dump
// threshold, and returns the function that stands it down. The caller must always call that.
//
// The snapshot has to be taken while the handler is still in the handler, which is the whole point of
// taking one: dumping after it returns records the goroutine unwinding this rather than whatever it was
// blocked on. A timer per message is the cost of that, which is why this is off by default.
func (self *SlowHandlerDetector) armStackDumpWatchdog(contentType int32, ch channel.Channel) func() {
	timer := time.AfterFunc(self.cfg.StackDumpThreshold, func() {
		self.writeSlowHandlerStacks(contentType, ch)
	})
	return func() {
		timer.Stop()
	}
}

// writeSlowHandlerStacks snapshots every goroutine, or counts the snapshot against one already on file.
// Called from the watchdog, so the handler it reports on is still running.
func (self *SlowHandlerDetector) writeSlowHandlerStacks(contentType int32, ch channel.Channel) {
	log := pfxlog.Logger().
		WithField("channelId", ch.Id()).
		WithField("contentType", contentType).
		WithField("thresholdMs", self.cfg.StackDumpThreshold.Milliseconds()).
		WithField("goroutines", runtime.NumGoroutine())

	// Which handler failed to return is the one thing the dump cannot show, since its goroutine cannot be
	// picked out of the many others also inside wrapped handlers.
	result := self.recorder.Capture(fmt.Sprintf("contentType=%d", contentType))
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

// recordHandlerDuration logs how long a handler took, once it has returned. Stack dumps are not taken
// here: see armStackDumpWatchdog.
func (self *SlowHandlerDetector) recordHandlerDuration(contentType int32, ch channel.Channel, d time.Duration) {
	if d < self.cfg.WarnThreshold {
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
