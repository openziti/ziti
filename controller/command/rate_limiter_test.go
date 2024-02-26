//go:build perf

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
	"errors"
	"fmt"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/stretchr/testify/assert"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_AdaptiveRateLimiter(t *testing.T) {
	cfg := AdaptiveRateLimiterConfig{
		Enabled:          true,
		MaxSize:          250,
		MinSize:          5,
		WorkTimerMetric:  "workTime",
		QueueSizeMetric:  "queueSize",
		WindowSizeMetric: "windowSize",
	}

	registry := metrics.NewRegistry("test", nil)
	closeNotify := make(chan struct{})
	limiter := NewAdaptiveRateLimiter(cfg, registry, closeNotify).(*adaptiveRateLimiter)

	var queueFull atomic.Uint32
	var timedOut atomic.Uint32
	var completed atomic.Uint32

	countdown := &sync.WaitGroup{}

	logStats := func() {
		fmt.Printf("queueFulls: %v\n", queueFull.Load())
		fmt.Printf("timedOut: %v\n", timedOut.Load())
		fmt.Printf("completed: %v\n", completed.Load())
		fmt.Printf("queueSize: %v\n", limiter.currentSize.Load())
		fmt.Printf("windowSize: %v\n", limiter.currentWindow.Load())
	}

	go func() {
		for {
			select {
			case <-closeNotify:
				return
			case <-time.After(time.Second):
				logStats()
			}
		}
	}()

	for i := 0; i < 300; i++ {
		countdown.Add(1)

		go func() {
			defer countdown.Done()
			count := 0
			for count < 1000 {
				start := time.Now()
				ctrl, err := limiter.RunRateLimited(func() error {
					time.Sleep(25 * time.Millisecond)
					return nil
				})

				if err == nil {
					elapsed := time.Since(start)
					if elapsed > time.Second*5 {
						timedOut.Add(1)
						ctrl.Backoff()
					} else {
						count++
						completed.Add(1)
						ctrl.Success()
					}
				} else {
					apiError := &errorz.ApiError{}
					if errors.As(err, &apiError) && apiError.Code == apierror.ServerTooManyRequestsCode {
						queueFull.Add(1)
					} else {
						panic(err)
					}
				}
			}
		}()
	}

	countdown.Wait()
	close(closeNotify)
	logStats()
}

func Test_AdaptiveRateLimiterTracker(t *testing.T) {
	cfg := AdaptiveRateLimiterConfig{
		Enabled:          true,
		MaxSize:          250,
		MinSize:          5,
		WorkTimerMetric:  "workTime",
		QueueSizeMetric:  "queueSize",
		WindowSizeMetric: "windowSize",
		Timeout:          time.Second,
	}

	registry := metrics.NewRegistry("test", nil)
	closeNotify := make(chan struct{})
	limiter := NewAdaptiveRateLimitTracker(cfg, registry, closeNotify).(*adaptiveRateLimitTracker)

	var queueFull atomic.Uint32
	var timedOut atomic.Uint32
	var completed atomic.Uint32

	countdown := &sync.WaitGroup{}

	logStats := func() {
		fmt.Printf("queueFulls: %v\n", queueFull.Load())
		fmt.Printf("timedOut: %v\n", timedOut.Load())
		fmt.Printf("completed: %v\n", completed.Load())
		fmt.Printf("queueSize: %v\n", limiter.currentSize.Load())
		fmt.Printf("windowSize: %v\n", limiter.currentWindow.Load())
	}

	go func() {
		for {
			select {
			case <-closeNotify:
				return
			case <-time.After(time.Second):
				logStats()
			}
		}
	}()

	sem := concurrenz.NewSemaphore(25)

	for i := 0; i < 300; i++ {
		countdown.Add(1)

		go func() {
			defer countdown.Done()
			count := 0
			for count < 1000 {
				// start := time.Now()
				err := limiter.RunRateLimitedF(func(control RateLimitControl) error {
					if sem.TryAcquire() {
						time.Sleep(25 * time.Millisecond)
						control.Success()
						sem.Release()
						completed.Add(1)
					} else {
						time.Sleep(5 * time.Millisecond)
						control.Backoff()
						timedOut.Add(1)
					}
					return nil
				})

				if err != nil {
					apiError := &errorz.ApiError{}
					if errors.As(err, &apiError) && apiError.Code == apierror.ServerTooManyRequestsCode {
						queueFull.Add(1)
						time.Sleep(time.Millisecond)
					} else {
						panic(err)
					}
				} else {
					count++
				}
			}
		}()
	}

	countdown.Wait()
	close(closeNotify)
	logStats()
}

func Test_AuthFlood(t *testing.T) {
	countdown := &sync.WaitGroup{}

	var complete atomic.Int32
	var errCount atomic.Int32

	for i := 0; i < 100; i++ {
		countdown.Add(1)
		idx := i
		go func() {
			defer countdown.Done()

			ctx, err := ziti.NewContextFromFile("/home/plorenz/work/demo/zcat.json")
			if err != nil {
				panic(err)
			}

			ctxImpl := ctx.(*ziti.ContextImpl)

			for j := 0; j < 10; j++ {
				for {
					_, err = ctxImpl.CtrlClt.Authenticate()
					if err == nil {
						break
					} else {
						errCount.Add(1)
					}
				}
				done := complete.Add(1)
				fmt.Printf("%v done!, competed: %v, errs: %v\n", idx, done, errCount.Load())
			}
		}()
	}

	countdown.Wait()
}

func Test_WasApiRateLimited(t *testing.T) {
	err := errors.New("hello")
	assert.False(t, WasRateLimited(err))

	err = apierror.NewTooManyUpdatesError()
	assert.True(t, WasRateLimited(err))
}
