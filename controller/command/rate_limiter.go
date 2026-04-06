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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/pkg/errors"
	gometrics "github.com/rcrowley/go-metrics"
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

	DefaultAdaptiveRateLimiterSuccessThreshold      = 0.9
	DefaultAdaptiveRateLimiterIncreaseFactor        = 1.02
	DefaultAdaptiveRateLimiterDecreaseFactor        = 0.9
	DefaultAdaptiveRateLimiterIncreaseCheckInterval = 10
	DefaultAdaptiveRateLimiterDecreaseCheckInterval = 10
)

// RateLimiterConfig contains configuration values used to create a new DefaultRateLimiter
type RateLimiterConfig struct {
	Enabled   bool
	QueueSize uint32
}

// NewRateLimiter creates a new rate limiter using the given configuration. If the configuration has
// Enabled set to false, a NoOpRateLimiter will be returned
func NewRateLimiter(config RateLimiterConfig, registry metrics.Registry, closeNotify <-chan struct{}) rate.RateLimiter {
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
		config:      config,
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

// NoOpRateLimiter is a rate limiter that doesn't enforce any rate limiting
type NoOpRateLimiter struct{}

func (self NoOpRateLimiter) RunRateLimited(f func() error) error {
	return f()
}

func (self NoOpRateLimiter) GetQueueFillPct() float64 {
	return 0
}

// NoOpAdaptiveRateLimiter is an adaptive rate limiter that doesn't enforce any rate limiting
type NoOpAdaptiveRateLimiter struct{}

func (self NoOpAdaptiveRateLimiter) RunRateLimited(f func() error) (rate.RateLimitControl, error) {
	return rate.NoOpRateLimitControl(), f()
}

// NoOpAdaptiveRateLimitTracker is an adaptive rate limit tracker that doesn't enforce any rate limiting
type NoOpAdaptiveRateLimitTracker struct{}

func (n NoOpAdaptiveRateLimitTracker) RunRateLimited(string) (rate.RateLimitControl, error) {
	return rate.NoOpRateLimitControl(), nil
}

func (n NoOpAdaptiveRateLimitTracker) RunRateLimitedF(_ string, f func(control rate.RateLimitControl) error) error {
	return f(rate.NoOpRateLimitControl())
}

func (n NoOpAdaptiveRateLimitTracker) IsRateLimited() bool {
	return false
}

type rateLimitedWork struct {
	wrapped func() error
	result  chan error
}

// DefaultRateLimiter implements rate.RateLimiter using a fixed-size buffered channel as a work queue
type DefaultRateLimiter struct {
	currentSize atomic.Int32
	queue       chan *rateLimitedWork
	closeNotify <-chan struct{}
	workRate    metrics.Timer
	config      RateLimiterConfig
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

func (self *DefaultRateLimiter) GetQueueFillPct() float64 {
	return float64(self.currentSize.Load()) / float64(self.config.QueueSize)
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

// SetDefaults sets the default values for the AdaptiveRateLimiterConfig
func (self *AdaptiveRateLimiterConfig) SetDefaults() {
	self.Enabled = DefaultAdaptiveRateLimiterEnabled
	self.MinSize = DefaultAdaptiveRateLimiterMinWindowSize
	self.MaxSize = DefaultAdaptiveRateLimiterMaxWindowSize
	self.Timeout = DefaultAdaptiveRateLimiterTimeout
}

// Load reads the configuration values from the given config map
func (cfg *AdaptiveRateLimiterConfig) Load(cfgmap map[interface{}]interface{}) error {
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

// NewAdaptiveRateLimiter creates a new adaptive rate limiter using the given configuration. If the
// configuration has Enabled set to false, a NoOpAdaptiveRateLimiter will be returned
func NewAdaptiveRateLimiter(config AdaptiveRateLimiterConfig, registry metrics.Registry, closeNotify <-chan struct{}) rate.AdaptiveRateLimiter {
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

func (self *adaptiveRateLimiter) RunRateLimited(f func() error) (rate.RateLimitControl, error) {
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
		return rate.NoOpRateLimitControl(), apierror.NewTooManyUpdatesError()
	}

	work.queuePosition = queuePosition

	defer self.currentSize.Add(-1)

	select {
	case self.queue <- work:
		select {
		case result := <-work.result:
			return rateLimitControl{limiter: self, queuePosition: work.queuePosition}, result
		case <-self.closeNotify:
			return rate.NoOpRateLimitControl(), errors.New("rate limiter shutting down")
		}
	case <-self.closeNotify:
		return rate.NoOpRateLimitControl(), errors.New("rate limiter shutting down")
	default:
		return rate.NoOpRateLimitControl(), apierror.NewTooManyUpdatesError()
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

// WasRateLimited returns true if the given error indicates that a request was rejected due to rate limiting
func WasRateLimited(err error) bool {
	var apiErr *errorz.ApiError
	if errors.As(err, &apiErr) {
		return apiErr.Code == apierror.ServerTooManyRequestsCode
	}
	return false
}

// AdaptiveRateLimitTrackerConfig contains configuration values used to create a new AdaptiveRateLimitTracker
type AdaptiveRateLimitTrackerConfig struct {
	AdaptiveRateLimiterConfig

	// SuccessThreshold - the success rate threshold above which the window size will be increased and
	//                    below which the window size will be decreased
	SuccessThreshold float64

	// IncreaseFactor - the multiplier applied to the current window size when increasing it
	IncreaseFactor float64

	// DecreaseFactor - the multiplier applied to the current window size when decreasing it
	DecreaseFactor float64

	// IncreaseCheckInterval - the number of successes between window size increase checks
	IncreaseCheckInterval int

	// DecreaseCheckInterval - the number of backoffs between window size decrease checks
	DecreaseCheckInterval int
}

// SetDefaults sets the default values for the AdaptiveRateLimitTrackerConfig
func (self *AdaptiveRateLimitTrackerConfig) SetDefaults() {
	self.AdaptiveRateLimiterConfig.SetDefaults()
	self.SuccessThreshold = DefaultAdaptiveRateLimiterSuccessThreshold
	self.IncreaseFactor = DefaultAdaptiveRateLimiterIncreaseFactor
	self.DecreaseFactor = DefaultAdaptiveRateLimiterDecreaseFactor
	self.IncreaseCheckInterval = DefaultAdaptiveRateLimiterIncreaseCheckInterval
	self.DecreaseCheckInterval = DefaultAdaptiveRateLimiterDecreaseCheckInterval
}

// Load reads the configuration values from the given config map
func (cfg *AdaptiveRateLimitTrackerConfig) Load(cfgmap map[interface{}]interface{}) error {
	if err := cfg.AdaptiveRateLimiterConfig.Load(cfgmap); err != nil {
		return err
	}

	if value, found := cfgmap["successThreshold"]; found {
		if v, ok := value.(float64); ok {
			cfg.SuccessThreshold = v
		} else {
			return errors.Errorf("invalid value %v for adaptive rate limiter success threshold, must be floating point value", value)
		}
	}

	if cfg.SuccessThreshold > 1 {
		return errors.Errorf("invalid value %f for adaptive rate limiter success threshold, must be between 0 and 1", cfg.SuccessThreshold)
	}

	if cfg.SuccessThreshold < 0 {
		return errors.Errorf("invalid value %f for adaptive rate limiter success threshold, must be between 0 and 1", cfg.SuccessThreshold)
	}

	if value, found := cfgmap["increaseFactor"]; found {
		if v, ok := value.(float64); ok {
			cfg.IncreaseFactor = v
		} else {
			return errors.Errorf("invalid value %v for adaptive rate limiter increaseFactor, must be floating point value", value)
		}
	}

	if cfg.IncreaseFactor < 1 {
		return errors.Errorf("invalid value %f for adaptive rate limiter increaseFactor, must be greater than 1, usually less than 2", cfg.IncreaseFactor)
	}

	if value, found := cfgmap["decreaseFactor"]; found {
		if v, ok := value.(float64); ok {
			cfg.DecreaseFactor = v
		} else {
			return errors.Errorf("invalid value %v for adaptive rate limiter decreaseFactor, must be floating point value", value)
		}
	}

	if cfg.DecreaseFactor <= 0 || cfg.DecreaseFactor >= 1 {
		return errors.Errorf("invalid value %f for adaptive rate limiter decreaseFactor, must be between 0 and 1", cfg.DecreaseFactor)
	}

	if value, found := cfgmap["increaseCheckInterval"]; found {
		if intVal, ok := value.(int); ok {
			cfg.IncreaseCheckInterval = intVal
		} else {
			return errors.Errorf("invalid value %v for adaptive rate limiter increaseCheckInterval, must be integer value", value)
		}
	}

	if cfg.IncreaseCheckInterval < 1 {
		return errors.Errorf("invalid value %d for adaptive rate limiter increaseCheckInterval, must be at least 1", cfg.IncreaseCheckInterval)
	}

	if value, found := cfgmap["decreaseCheckInterval"]; found {
		if intVal, ok := value.(int); ok {
			cfg.DecreaseCheckInterval = intVal
		} else {
			return errors.Errorf("invalid value %v for adaptive rate limiter decreaseCheckInterval, must be integer value", value)
		}
	}

	if cfg.DecreaseCheckInterval < 1 {
		return errors.Errorf("invalid value %d for adaptive rate limiter decreaseCheckInterval, must be at least 1", cfg.DecreaseCheckInterval)
	}

	return nil
}

// NewAdaptiveRateLimitTracker creates a new adaptive rate limit tracker using the given configuration.
// If the configuration has Enabled set to false, a NoOpAdaptiveRateLimitTracker will be returned.
// Unlike the AdaptiveRateLimiter, the tracker does not execute work directly. Instead it tracks
// outstanding work and adjusts the window size based on the success rate of completed work.
func NewAdaptiveRateLimitTracker(config AdaptiveRateLimitTrackerConfig, registry metrics.Registry, closeNotify <-chan struct{}) rate.AdaptiveRateLimitTracker {
	if !config.Enabled {
		return NoOpAdaptiveRateLimitTracker{}
	}

	result := &adaptiveRateLimitTracker{
		minWindow:             int32(config.MinSize),
		maxWindow:             int32(config.MaxSize),
		successThreshold:      config.SuccessThreshold,
		increaseFactor:        config.IncreaseFactor,
		decreaseFactor:        config.DecreaseFactor,
		increaseCheckInterval: uint32(config.IncreaseCheckInterval),
		decreaseCheckInterval: uint32(config.DecreaseCheckInterval),
		timeout:               config.Timeout,
		workRate:              registry.Timer(config.WorkTimerMetric),
		outstandingWork:       map[string]*adaptiveRateLimitTrackerWork{},
		closeNotify:           closeNotify,
		successRate:           gometrics.NewHistogram(gometrics.NewExpDecaySample(128, 0.5)),
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

// adaptiveRateLimitTracker manages a sliding concurrency window to control the rate of outstanding work.
// It does not execute work directly. Callers acquire a slot via RunRateLimited and later report the
// outcome (Success, Backoff, or Failed) through the returned RateLimitControl.
//
// The window size adjusts between minWindow and maxWindow based on an exponentially decaying success
// rate histogram. Every increaseCheckInterval successes, if the success rate exceeds the configured
// threshold the window is grown by increaseFactor. Every decreaseCheckInterval backoffs, if the
// success rate is below the threshold the window is shrunk by decreaseFactor. A background goroutine
// expires work that has not been completed within the configured timeout, treating it as a backoff.
type adaptiveRateLimitTracker struct {
	currentWindow         atomic.Int32
	minWindow             int32
	maxWindow             int32
	successThreshold      float64
	increaseFactor        float64
	decreaseFactor        float64
	increaseCheckInterval uint32
	decreaseCheckInterval uint32

	timeout        time.Duration
	lock           sync.Mutex
	successCounter atomic.Uint32
	backoffCounter atomic.Uint32

	currentSize     atomic.Int32
	workRate        metrics.Timer
	outstandingWork map[string]*adaptiveRateLimitTrackerWork
	closeNotify     <-chan struct{}
	successRate     gometrics.Histogram
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
	self.successRate.Update(1)

	if self.currentWindow.Load() >= self.maxWindow {
		return
	}

	if self.successCounter.Add(1)%self.increaseCheckInterval == 0 && self.successRate.Mean() > self.successThreshold {
		current := self.currentWindow.Load()
		nextWindow := int32(float64(current) * self.increaseFactor)
		if nextWindow == current {
			nextWindow++
		}

		if nextWindow > self.maxWindow {
			nextWindow = self.maxWindow
		}
		self.updateWindowSize(nextWindow)
	}
}

func (self *adaptiveRateLimitTracker) backoff(work *adaptiveRateLimitTrackerWork) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.currentSize.Add(-1)
	delete(self.outstandingWork, work.id)

	self.successRate.Update(0)

	if self.currentWindow.Load() <= self.minWindow {
		return
	}

	if self.backoffCounter.Add(1)%self.decreaseCheckInterval == 0 && self.successRate.Mean() < self.successThreshold {
		current := self.currentWindow.Load()
		nextWindow := int32(float64(current) * self.decreaseFactor)

		if nextWindow == current {
			nextWindow--
		}

		if nextWindow < self.minWindow {
			nextWindow = self.minWindow
		}
		self.updateWindowSize(nextWindow)
	}
}

func (self *adaptiveRateLimitTracker) updateWindowSize(nextWindow int32) {
	pfxlog.Logger().WithField("queueSize", self.currentSize.Load()).
		WithField("currentWindowSize", self.currentWindow.Load()).
		WithField("nextWindowSize", nextWindow).
		WithField("successRate", self.successRate.Mean()).
		Debug("window size updated")
	self.currentWindow.Store(nextWindow)
}

func (self *adaptiveRateLimitTracker) complete(work *adaptiveRateLimitTrackerWork) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.currentSize.Add(-1)
	delete(self.outstandingWork, work.id)
}

func (self *adaptiveRateLimitTracker) RunRateLimited(label string) (rate.RateLimitControl, error) {
	self.lock.Lock()
	defer self.lock.Unlock()
	queuePosition := self.currentSize.Add(1)
	if queuePosition > self.currentWindow.Load() {
		self.currentSize.Add(-1)
		return rate.NoOpRateLimitControl(), apierror.NewTooManyUpdatesError()
	}

	work := &adaptiveRateLimitTrackerWork{
		id:            uuid.NewString(),
		limiter:       self,
		queuePosition: queuePosition,
		createTime:    time.Now(),
		label:         label,
	}

	self.outstandingWork[work.id] = work

	return work, nil
}

func (self *adaptiveRateLimitTracker) RunRateLimitedF(label string, f func(control rate.RateLimitControl) error) error {
	ctrl, err := self.RunRateLimited(label)
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
		pfxlog.Logger().WithField("label", work.label).
			WithField("duration", time.Since(work.createTime)).
			Info("rate limit work expired")
		work.Backoff()
	}
}

type adaptiveRateLimitTrackerWork struct {
	id            string
	limiter       *adaptiveRateLimitTracker
	queuePosition int32
	createTime    time.Time
	completed     atomic.Bool
	label         string
}

func (self *adaptiveRateLimitTrackerWork) Success() {
	if self.completed.CompareAndSwap(false, true) {
		pfxlog.Logger().WithField("label", self.label).
			WithField("duration", time.Since(self.createTime)).
			WithField("currentSize", self.limiter.currentSize.Load()).
			WithField("currentWindow", self.limiter.currentWindow.Load()).
			Debug("success")
		self.limiter.success(self)
	}
}

func (self *adaptiveRateLimitTrackerWork) Backoff() {
	if self.completed.CompareAndSwap(false, true) {
		pfxlog.Logger().WithField("label", self.label).
			WithField("duration", time.Since(self.createTime)).
			WithField("currentSize", self.limiter.currentSize.Load()).
			WithField("currentWindow", self.limiter.currentWindow.Load()).
			Debug("backoff")
		self.limiter.backoff(self)
	}
}

func (self *adaptiveRateLimitTrackerWork) Failed() {
	if self.completed.CompareAndSwap(false, true) {
		pfxlog.Logger().WithField("label", self.label).
			WithField("duration", time.Since(self.createTime)).
			WithField("currentSize", self.limiter.currentSize.Load()).
			WithField("currentWindow", self.limiter.currentWindow.Load()).
			Debug("failed")
		self.limiter.complete(self)
	}
}
