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
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/pkg/errors"
	"sync/atomic"
	"time"
)

const (
	MetricLimiterCurrentQueuedCount = "command.limiter.queued_count"
	MetricLimiterWorkTimer          = "command.limiter.work_timer"

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
		workRate:    registry.Timer(MetricLimiterWorkTimer),
	}

	if existing := registry.GetGauge(MetricLimiterCurrentQueuedCount); existing != nil {
		existing.Dispose()
	}

	registry.FuncGauge(MetricLimiterCurrentQueuedCount, func() int64 {
		return int64(result.currentSize.Load())
	})

	go result.run()

	return result
}

type RateLimiter interface {
	RunRateLimited(func() error) error
}

type NoOpRateLimiter struct{}

func (self NoOpRateLimiter) RunRateLimited(f func() error) error {
	return f()
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
