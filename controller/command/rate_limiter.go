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

package command

import (
	"fmt"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MetricCommandLimiterCurrentQueuedCount = "command.limiter.queued_count"
	MetricCommandLimiterWorkTimer          = "command.limiter.work_timer"

	DefaultLimiterSize = 100
	MinLimiterSize     = 10
)

type RateLimiterConfig struct {
	Enabled   bool
	QueueSize uint32
}

func NewRateLimiter(config RateLimiterConfig, registry metrics.Registry, closeNotify <-chan struct{}) RateLimiter {
	if !config.Enabled {
		return NoOpRateLimiter{}
	}

	if config.QueueSize < MinLimiterSize {
		config.QueueSize = MinLimiterSize
	}

	result := &DefaultRateLimiter{
		queue:       make(chan *rateLimitedWork, config.QueueSize),
		closeNotify: closeNotify,
		workRate:    registry.Timer(MetricCommandLimiterWorkTimer),
	}

	if existing := registry.GetGauge(MetricCommandLimiterCurrentQueuedCount); existing != nil {
		existing.Dispose()
	}

	registry.FuncGauge(MetricCommandLimiterCurrentQueuedCount, func() int64 {
		return int64(result.currentSize.Load())
	})

	go result.run()

	return result
}

// A RateLimiter allows running arbitrary, sequential operations with a limiter, so that only N operations
// can be queued to run at any given time. If the system is too busy, the rate limiter will return
// an ApiError indicating that the server is too busy
type RateLimiter interface {
	RunRateLimited(func() error) error
}

// An AdaptiveRateLimiter allows running arbitrary, sequential operations with a limiter, so that only N operations
// can be queued to run at any given time. If the system is too busy, the rate limiter will return
// an ApiError indicating that the server is too busy.
//
// The rate limiter returns a RateLimitControl, allow the calling code to indicate if the operation finished in
// time. If operations are timing out before the results are available, the rate limiter should allow fewer
// operations in, as they will likely time out before the results can be used.
//
// The rate limiter doesn't have a set queue size, it has a window which can grow and shrink. When
// a timeout is signaled, using the RateLimitControl, it shrinks the window based on queue position
// of the timed out operation. For example, if an operation was queued at position 200, but the times
// out, we assume that we need to limit the queue size to something less than 200 for now.
//
// The limiter will also reject already queued operations if the window size changes and the operation
// was queued at a position larger than the current window size.
//
// The window size will slowly grow back towards the max as successes are noted in the RateLimitControl.
type AdaptiveRateLimiter interface {
	RunRateLimited(f func() error) (RateLimitControl, error)
}

type NoOpRateLimiter struct{}

func (self NoOpRateLimiter) RunRateLimited(f func() error) error {
	return f()
}

type NoOpAdaptiveRateLimiter struct{}

func (self NoOpAdaptiveRateLimiter) RunRateLimited(f func() error) (RateLimitControl, error) {
	return noOpRateLimitControl{}, f()
}

type rateLimitedWork struct {
	wrapped func() error
	result  chan error
}

type DefaultRateLimiter struct {
	currentSize atomic.Int32
	queue       chan *rateLimitedWork
	closeNotify <-chan struct{}
	workRate    metrics.Timer
}

func (self *DefaultRateLimiter) RunRateLimited(f func() error) error {
	work := &rateLimitedWork{
		wrapped: f,
		result:  make(chan error, 1),
	}
	select {
	case self.queue <- work:
		self.currentSize.Add(1)
		select {
		case result := <-work.result:
			return result
		case <-self.closeNotify:
			return errors.New("rate limiter shutting down")
		}
	case <-self.closeNotify:
		return errors.New("rate limiter shutting down")
	default:
		return apierror.NewTooManyUpdatesError()
	}
}

func (self *DefaultRateLimiter) run() {
	defer self.workRate.Dispose()

	for {
		select {
		case work := <-self.queue:
			self.currentSize.Add(-1)
			startTime := time.Now()
			result := work.wrapped()
			self.workRate.UpdateSince(startTime)
			if result != nil {
				work.result <- result
			}
			close(work.result)
		case <-self.closeNotify:
			return
		}
	}
}

// AdaptiveRateLimiterConfig contains configuration values used to create a new AdaptiveRateLimiter
type AdaptiveRateLimiterConfig struct {
	// Enabled - if false, a no-op rate limiter will be created, which doesn't enforce any rate limiting
	Enabled bool

	// MaxSize - the maximum window size to allow
	MaxSize uint32

	// MinSize - the smallest window size to allow
	MinSize uint32

	// WorkTimerMetric - the name of the timer metric for timing how long operations take to execute
	WorkTimerMetric string

	// QueueSize - the name of the gauge metric showing the current number of operations queued
	QueueSizeMetric string

	// WindowSizeMetric - the name of the metric show the current window size
	WindowSizeMetric string
}

func (self *AdaptiveRateLimiterConfig) Validate() error {
	if !self.Enabled {
		return nil
	}

	if self.MinSize < 1 {
		return errors.New("adaptive rate limiter min size is 1")
	}
	if self.MinSize > self.MaxSize {
		return fmt.Errorf("adaptive rate limiter min size must be <- max size. min: %v, max: %v", self.MinSize, self.MaxSize)
	}
	return nil
}

func NewAdaptiveRateLimiter(config AdaptiveRateLimiterConfig, registry metrics.Registry, closeNotify <-chan struct{}) AdaptiveRateLimiter {
	if !config.Enabled {
		return NoOpAdaptiveRateLimiter{}
	}

	result := &adaptiveRateLimiter{
		currentWindow: atomic.Int32{},
		minWindow:     int32(config.MinSize),
		maxWindow:     int32(config.MaxSize),
		queue:         make(chan *adaptiveRateLimitedWork, config.MaxSize),
		closeNotify:   closeNotify,
		workRate:      registry.Timer(config.WorkTimerMetric),
	}

	if existing := registry.GetGauge(config.QueueSizeMetric); existing != nil {
		existing.Dispose()
	}

	registry.FuncGauge(config.QueueSizeMetric, func() int64 {
		return int64(result.currentSize.Load())
	})

	if existing := registry.GetGauge(config.WindowSizeMetric); existing != nil {
		existing.Dispose()
	}

	registry.FuncGauge(config.WindowSizeMetric, func() int64 {
		return int64(result.currentWindow.Load())
	})

	result.currentWindow.Store(int32(config.MaxSize))

	go result.run()

	return result
}

type adaptiveRateLimitedWork struct {
	queuePosition int32
	wrapped       func() error
	result        chan error
}

type adaptiveRateLimiter struct {
	currentWindow  atomic.Int32
	minWindow      int32
	maxWindow      int32
	lock           sync.Mutex
	successCounter atomic.Uint32

	currentSize atomic.Int32
	queue       chan *adaptiveRateLimitedWork
	closeNotify <-chan struct{}
	workRate    metrics.Timer
}

func (self *adaptiveRateLimiter) success() {
	if self.currentWindow.Load() >= self.maxWindow {
		return
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	if self.successCounter.Add(1)%10 == 0 {
		if nextVal := self.currentWindow.Add(1); nextVal > self.maxWindow {
			self.currentWindow.Store(self.maxWindow)
		}
	}
}

func (self *adaptiveRateLimiter) failure(queuePosition int32) {
	if self.currentWindow.Load() <= self.minWindow {
		return
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	current := self.currentWindow.Load()
	nextWindow := queuePosition - 10
	if nextWindow < current {
		if nextWindow < self.minWindow {
			nextWindow = self.minWindow
		}
		self.currentWindow.Store(nextWindow)
	}
}

func (self *adaptiveRateLimiter) RunRateLimited(f func() error) (RateLimitControl, error) {
	work := &adaptiveRateLimitedWork{
		wrapped: f,
		result:  make(chan error, 1),
	}

	self.lock.Lock()
	queuePosition := self.currentSize.Add(1)
	hasRoom := queuePosition <= self.currentWindow.Load()
	if !hasRoom {
		self.currentSize.Add(-1)
	}
	self.lock.Unlock()

	if !hasRoom {
		return noOpRateLimitControl{}, apierror.NewTooManyUpdatesError()
	}

	work.queuePosition = queuePosition

	defer self.currentSize.Add(-1)

	select {
	case self.queue <- work:
		select {
		case result := <-work.result:
			return rateLimitControl{limiter: self, queuePosition: work.queuePosition}, result
		case <-self.closeNotify:
			return noOpRateLimitControl{}, errors.New("rate limiter shutting down")
		}
	case <-self.closeNotify:
		return noOpRateLimitControl{}, errors.New("rate limiter shutting down")
	default:
		return noOpRateLimitControl{}, apierror.NewTooManyUpdatesError()
	}
}

func (self *adaptiveRateLimiter) run() {
	defer self.workRate.Dispose()

	for {
		select {
		case work := <-self.queue:

			// if we're likely to discard the work because things have been timing out,
			// skip it, and return an error instead
			if work.queuePosition > self.currentWindow.Load()+10 {
				work.result <- apierror.NewTooManyUpdatesError()
				close(work.result)
				continue
			}

			startTime := time.Now()
			result := work.wrapped()
			self.workRate.UpdateSince(startTime)
			if result != nil {
				work.result <- result
			}
			close(work.result)
		case <-self.closeNotify:
			return
		}
	}
}

type RateLimitControl interface {
	Success()
	Timeout()
}

type rateLimitControl struct {
	limiter       *adaptiveRateLimiter
	queuePosition int32
}

func (r rateLimitControl) Success() {
	r.limiter.success()
}

func (r rateLimitControl) Timeout() {
	r.limiter.failure(r.queuePosition)
}

type noOpRateLimitControl struct{}

func (noOpRateLimitControl) Success() {}

func (noOpRateLimitControl) Timeout() {}
