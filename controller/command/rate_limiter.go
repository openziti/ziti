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
	"github.com/google/uuid"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/pkg/errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MetricCommandLimiterCurrentQueuedCount = "command.limiter.queued_count"
	MetricCommandLimiterWorkTimer          = "command.limiter.work_timer"

	DefaultLimiterSize = 100
	MinLimiterSize     = 10

	DefaultAdaptiveRateLimiterEnabled       = true
	DefaultAdaptiveRateLimiterMinWindowSize = 5
	DefaultAdaptiveRateLimiterMaxWindowSize = 250
	DefaultAdaptiveRateLimiterTimeout       = 30 * time.Second
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

// An AdaptiveRateLimitTracker works similarly to an AdaptiveRateLimiter, except it just manages the rate
// limiting without actually running the work. Because it doesn't run the work itself, it has to account
// for the possibility that some work may never report as complete or failed. It thus has a configurable
// timeout at which point outstanding work will be marked as failed.
type AdaptiveRateLimitTracker interface {
	RunRateLimited() (RateLimitControl, error)
	RunRateLimitedF(f func(control RateLimitControl) error) error
	IsRateLimited() bool
}

type NoOpRateLimiter struct{}

func (self NoOpRateLimiter) RunRateLimited(f func() error) error {
	return f()
}

type NoOpAdaptiveRateLimiter struct{}

func (self NoOpAdaptiveRateLimiter) RunRateLimited(f func() error) (RateLimitControl, error) {
	return noOpRateLimitControl{}, f()
}

type NoOpAdaptiveRateLimitTracker struct{}

func (n NoOpAdaptiveRateLimitTracker) RunRateLimited() (RateLimitControl, error) {
	return noOpRateLimitControl{}, nil
}

func (n NoOpAdaptiveRateLimitTracker) RunRateLimitedF(f func(control RateLimitControl) error) error {
	return f(noOpRateLimitControl{})
}

func (n NoOpAdaptiveRateLimitTracker) IsRateLimited() bool {
	return false
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

	// Timeout - only used for AdaptiveRateLimitTracker, sets when a piece of outstanding work will be assumed to
	//           have failed if it hasn't been marked completed yet, so that work slots aren't lost
	Timeout time.Duration
}

func (self *AdaptiveRateLimiterConfig) SetDefaults() {
	self.Enabled = DefaultAdaptiveRateLimiterEnabled
	self.MinSize = DefaultAdaptiveRateLimiterMinWindowSize
	self.MaxSize = DefaultAdaptiveRateLimiterMaxWindowSize
	self.Timeout = DefaultAdaptiveRateLimiterTimeout
}

func LoadAdaptiveRateLimiterConfig(cfg *AdaptiveRateLimiterConfig, cfgmap map[interface{}]interface{}) error {
	if value, found := cfgmap["enabled"]; found {
		cfg.Enabled = strings.EqualFold("true", fmt.Sprintf("%v", value))
	}

	if value, found := cfgmap["maxSize"]; found {
		if intVal, ok := value.(int); ok {
			v := int64(intVal)
			cfg.MaxSize = uint32(v)
		} else {
			return errors.Errorf("invalid value %d for adaptive rate limiter max size, must be integer value", value)
		}
	}

	if value, found := cfgmap["minSize"]; found {
		if intVal, ok := value.(int); ok {
			v := int64(intVal)
			cfg.MinSize = uint32(v)
		} else {
			return errors.Errorf("invalid value %d for adaptive rate limiter min size, must be integer value", value)
		}
	}

	if cfg.MinSize < 1 {
		return errors.Errorf("invalid value %d for adaptive rate limiter min size, must be at least", cfg.MinSize)
	}

	if cfg.MinSize > cfg.MaxSize {
		return errors.Errorf("invalid values, %d, %d for adaptive rate limiter min size and max size, min must be <= max",
			cfg.MinSize, cfg.MaxSize)
	}

	if value, found := cfgmap["timeout"]; found {
		var err error
		if cfg.Timeout, err = time.ParseDuration(fmt.Sprintf("%v", value)); err != nil {
			return fmt.Errorf("invalid value %v for adaptive rate limiter timeout (%w)", value, err)
		}
	}

	return nil
}

func NewAdaptiveRateLimiter(config AdaptiveRateLimiterConfig, registry metrics.Registry, closeNotify <-chan struct{}) AdaptiveRateLimiter {
	if !config.Enabled {
		return NoOpAdaptiveRateLimiter{}
	}

	result := &adaptiveRateLimiter{
		minWindow:   int32(config.MinSize),
		maxWindow:   int32(config.MaxSize),
		queue:       make(chan *adaptiveRateLimitedWork, config.MaxSize),
		closeNotify: closeNotify,
		workRate:    registry.Timer(config.WorkTimerMetric),
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

func (self *adaptiveRateLimiter) backoff(queuePosition int32) {
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
	// Success indicats the operation was a success
	Success()

	// Backoff indicates that we need to backoff
	Backoff()

	// Failed indicates the operation was not a success, but a backoff isn't required
	Failed()
}

type rateLimitControl struct {
	limiter       *adaptiveRateLimiter
	queuePosition int32
}

func (r rateLimitControl) Success() {
	r.limiter.success()
}

func (r rateLimitControl) Backoff() {
	r.limiter.backoff(r.queuePosition)
}

func (r rateLimitControl) Failed() {
	// no-op for this type
}

func NoOpRateLimitControl() RateLimitControl {
	return noOpRateLimitControl{}
}

type noOpRateLimitControl struct{}

func (noOpRateLimitControl) Success() {}

func (noOpRateLimitControl) Backoff() {}

func (noOpRateLimitControl) Failed() {}

func WasRateLimited(err error) bool {
	var apiErr *errorz.ApiError
	if errors.As(err, &apiErr) {
		return apiErr.Code == apierror.ServerTooManyRequestsCode
	}
	return false
}

func NewAdaptiveRateLimitTracker(config AdaptiveRateLimiterConfig, registry metrics.Registry, closeNotify <-chan struct{}) AdaptiveRateLimitTracker {
	if !config.Enabled {
		return NoOpAdaptiveRateLimitTracker{}
	}

	result := &adaptiveRateLimitTracker{
		minWindow:       int32(config.MinSize),
		maxWindow:       int32(config.MaxSize),
		timeout:         config.Timeout,
		workRate:        registry.Timer(config.WorkTimerMetric),
		outstandingWork: map[string]*adaptiveRateLimitTrackerWork{},
		closeNotify:     closeNotify,
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

type adaptiveRateLimitTracker struct {
	currentWindow  atomic.Int32
	minWindow      int32
	maxWindow      int32
	timeout        time.Duration
	lock           sync.Mutex
	successCounter atomic.Uint32

	currentSize     atomic.Int32
	workRate        metrics.Timer
	outstandingWork map[string]*adaptiveRateLimitTrackerWork
	closeNotify     <-chan struct{}
}

func (self *adaptiveRateLimitTracker) IsRateLimited() bool {
	return self.currentSize.Load() >= self.currentWindow.Load()
}

func (self *adaptiveRateLimitTracker) success(work *adaptiveRateLimitTrackerWork) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.currentSize.Add(-1)
	delete(self.outstandingWork, work.id)
	self.workRate.UpdateSince(work.createTime)
	if self.currentWindow.Load() >= self.maxWindow {
		return
	}

	if self.successCounter.Add(1)%10 == 0 {
		if nextVal := self.currentWindow.Add(1); nextVal > self.maxWindow {
			self.currentWindow.Store(self.maxWindow)
		}
	}
}

func (self *adaptiveRateLimitTracker) backoff(work *adaptiveRateLimitTrackerWork) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.currentSize.Add(-1)
	delete(self.outstandingWork, work.id)

	if self.currentWindow.Load() <= self.minWindow {
		return
	}

	current := self.currentWindow.Load()
	nextWindow := work.queuePosition - 10
	if nextWindow < current {
		if nextWindow < self.minWindow {
			nextWindow = self.minWindow
		}
		self.currentWindow.Store(nextWindow)
	}
}

func (self *adaptiveRateLimitTracker) complete(work *adaptiveRateLimitTrackerWork) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.currentSize.Add(-1)
	delete(self.outstandingWork, work.id)
}

func (self *adaptiveRateLimitTracker) RunRateLimited() (RateLimitControl, error) {
	self.lock.Lock()
	defer self.lock.Unlock()
	queuePosition := self.currentSize.Add(1)
	if queuePosition > self.currentWindow.Load() {
		self.currentSize.Add(-1)
		return noOpRateLimitControl{}, apierror.NewTooManyUpdatesError()
	}

	work := &adaptiveRateLimitTrackerWork{
		id:            uuid.NewString(),
		limiter:       self,
		queuePosition: queuePosition,
		createTime:    time.Now(),
	}

	return work, nil
}

func (self *adaptiveRateLimitTracker) RunRateLimitedF(f func(control RateLimitControl) error) error {
	ctrl, err := self.RunRateLimited()
	if err != nil {
		return err
	}
	return f(ctrl)
}

func (self *adaptiveRateLimitTracker) run() {
	defer self.workRate.Dispose()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			self.cleanExpired()
		case <-self.closeNotify:
			return
		}
	}
}

func (self *adaptiveRateLimitTracker) cleanExpired() {
	self.lock.Lock()

	var toRemove []*adaptiveRateLimitTrackerWork

	for _, v := range self.outstandingWork {
		if time.Since(v.createTime) > self.timeout {
			toRemove = append(toRemove, v)
		}
	}

	self.lock.Unlock()

	for _, work := range toRemove {
		work.Backoff()
	}
}

type adaptiveRateLimitTrackerWork struct {
	id            string
	limiter       *adaptiveRateLimitTracker
	queuePosition int32
	createTime    time.Time
	completed     atomic.Bool
}

func (self *adaptiveRateLimitTrackerWork) Success() {
	if self.completed.CompareAndSwap(false, true) {
		self.limiter.success(self)
	}
}

func (self *adaptiveRateLimitTrackerWork) Backoff() {
	if self.completed.CompareAndSwap(false, true) {
		self.limiter.backoff(self)
	}
}

func (self *adaptiveRateLimitTrackerWork) Failed() {
	if self.completed.CompareAndSwap(false, true) {
		self.limiter.complete(self)
	}
}
