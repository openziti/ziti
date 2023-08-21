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

package forwarder

import (
	"github.com/pkg/errors"
	"time"
)

const (
	DefaultXgressCloseCheckInterval    = 5 * time.Second
	DefaultXgressDialDwellTime         = 0
	DefaultFaultTxInterval             = 15 * time.Second
	DefaultIdleTxInterval              = 60 * time.Second
	DefaultIdleCircuitTimeout          = 60 * time.Second
	DefaultXgressDialWorkerQueueLength = 1000
	MinXgressDialWorkerQueueLength     = 1
	MaxXgressDialWorkerQueueLength     = 10000
	DefaultXgressDialWorkerCount       = 128
	MinXgressDialWorkerCount           = 1
	MaxXgressDialWorkerCount           = 10000

	DefaultLinkDialQueueLength   = 1000
	MinLinkDialWorkerQueueLength = 1
	MaxLinkDialWorkerQueueLength = 10000
	DefaultLinkDialWorkerCount   = 32
	MinLinkDialWorkerCount       = 1
	MaxLinkDialWorkerCount       = 10000

	DefaultRateLimiterQueueLength   = 5000
	MinRateLimiterWorkerQueueLength = 1
	MaxRateLimiterWorkerQueueLength = 50000
	DefaultRateLimiterWorkerCount   = 5
	MinRateLimiterWorkerCount       = 1
	MaxRateLimiterWorkerCount       = 10000
)

type Options struct {
	FaultTxInterval          time.Duration
	IdleCircuitTimeout       time.Duration
	IdleTxInterval           time.Duration
	LinkDial                 WorkerPoolOptions
	RateLimiter              WorkerPoolOptions
	XgressCloseCheckInterval time.Duration
	XgressDial               WorkerPoolOptions
	XgressDialDwellTime      time.Duration
}

type WorkerPoolOptions struct {
	QueueLength uint16
	WorkerCount uint16
}

func DefaultOptions() *Options {
	return &Options{
		FaultTxInterval:    DefaultFaultTxInterval,
		IdleCircuitTimeout: DefaultIdleCircuitTimeout,
		IdleTxInterval:     DefaultIdleTxInterval,
		LinkDial: WorkerPoolOptions{
			QueueLength: DefaultLinkDialQueueLength,
			WorkerCount: DefaultLinkDialWorkerCount,
		},
		RateLimiter: WorkerPoolOptions{
			QueueLength: DefaultRateLimiterQueueLength,
			WorkerCount: DefaultRateLimiterWorkerCount,
		},
		XgressCloseCheckInterval: DefaultXgressCloseCheckInterval,
		XgressDial: WorkerPoolOptions{
			QueueLength: DefaultXgressDialWorkerQueueLength,
			WorkerCount: DefaultXgressDialWorkerCount,
		},
		XgressDialDwellTime: DefaultXgressDialDwellTime,
	}
}

func LoadOptions(src map[interface{}]interface{}) (*Options, error) {
	options := DefaultOptions()

	if value, found := src["faultTxInterval"]; found {
		if val, ok := value.(int); ok {
			options.FaultTxInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'faultTxInterval'")
		}
	}

	if value, found := src["idleCircuitTimeout"]; found {
		if val, ok := value.(int); ok {
			options.IdleCircuitTimeout = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'idleCircuitTimeout'")
		}
	}

	if value, found := src["idleTxInterval"]; found {
		if val, ok := value.(int); ok {
			options.IdleTxInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'idleTxInterval'")
		}
	}

	if value, found := src["linkDialQueueLength"]; found {
		if length, ok := value.(int); ok {
			if length < MinLinkDialWorkerQueueLength || length > MaxLinkDialWorkerQueueLength {
				return nil, errors.Errorf("invalid value for 'linkDialQueueLength', expected integer between %v and %v", MinLinkDialWorkerQueueLength, MaxLinkDialWorkerQueueLength)
			}
			options.LinkDial.QueueLength = uint16(length)
		} else {
			return nil, errors.Errorf("invalid value for 'linkDialQueueLength', expected integer between %v and %v", MinLinkDialWorkerQueueLength, MaxLinkDialWorkerQueueLength)
		}
	}

	if value, found := src["linkDialWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers <= MinLinkDialWorkerCount || workers > MaxLinkDialWorkerCount {
				return nil, errors.Errorf("invalid value for 'linkDialWorkerCount', expected integer between %v and %v", MinLinkDialWorkerCount, MaxLinkDialWorkerCount)
			}
			options.LinkDial.WorkerCount = uint16(workers)
		} else {
			return nil, errors.Errorf("invalid value for 'linkDialWorkerCount', expected integer between %v and %v", MinLinkDialWorkerCount, MaxLinkDialWorkerCount)
		}
	}

	if value, found := src["rateLimitedQueueLength"]; found {
		if length, ok := value.(int); ok {
			if length < MinRateLimiterWorkerQueueLength || length > MaxRateLimiterWorkerQueueLength {
				return nil, errors.Errorf("invalid value for 'rateLimitedQueueLength', expected integer between %v and %v", MinRateLimiterWorkerQueueLength, MaxRateLimiterWorkerQueueLength)
			}
			options.RateLimiter.QueueLength = uint16(length)
		} else {
			return nil, errors.Errorf("invalid value for 'rateLimitedQueueLength', expected integer between %v and %v", MinRateLimiterWorkerQueueLength, MaxRateLimiterWorkerQueueLength)
		}
	}

	if value, found := src["rateLimitedWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers <= MinRateLimiterWorkerCount || workers > MaxRateLimiterWorkerCount {
				return nil, errors.Errorf("invalid value for 'rateLimitedWorkerCount', expected integer between %v and %v", MinRateLimiterWorkerCount, MaxRateLimiterWorkerCount)
			}
			options.RateLimiter.WorkerCount = uint16(workers)
		} else {
			return nil, errors.Errorf("invalid value for 'rateLimitedWorkerCount', expected integer between %v and %v", MinRateLimiterWorkerCount, MaxRateLimiterWorkerCount)
		}
	}

	if value, found := src["xgressCloseCheckInterval"]; found {
		if val, ok := value.(int); ok {
			options.XgressCloseCheckInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'latencyProbeInterval'")
		}
	}

	if value, found := src["xgressDialDwellTime"]; found {
		if v, ok := value.(int); ok {
			options.XgressDialDwellTime = time.Duration(v) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'xgressDialDwellTime'")
		}
	}

	if value, found := src["xgressDialQueueLength"]; found {
		if length, ok := value.(int); ok {
			if length < MinXgressDialWorkerQueueLength || length > MaxXgressDialWorkerQueueLength {
				return nil, errors.Errorf("invalid value for 'xgressDialQueueLength', expected integer between %v and %v", MinXgressDialWorkerQueueLength, MaxXgressDialWorkerQueueLength)
			}
			options.XgressDial.QueueLength = uint16(length)
		} else {
			return nil, errors.Errorf("invalid value for 'xgressDialQueueLength', expected integer between %v and %v", MinXgressDialWorkerQueueLength, MaxXgressDialWorkerQueueLength)
		}
	}

	if value, found := src["xgressDialWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers < MinXgressDialWorkerCount || workers > MaxXgressDialWorkerCount {
				return nil, errors.Errorf("invalid value for 'xgressDialWorkerCount', expected integer between %v and %v", MinXgressDialWorkerCount, MaxXgressDialWorkerCount)
			}
			options.XgressDial.WorkerCount = uint16(workers)
		} else {
			return nil, errors.Errorf("invalid value for 'xgressDialWorkerCount', expected integer between %v and %v", MinXgressDialWorkerCount, MaxXgressDialWorkerCount)
		}
	}

	return options, nil
}
