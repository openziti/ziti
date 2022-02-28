/*
	Copyright NetFoundry, Inc.

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
	"errors"
	"fmt"
	"time"
)

type Options struct {
	LatencyProbeInterval     time.Duration
	LatencyProbeTimeout      time.Duration
	XgressCloseCheckInterval time.Duration
	XgressDialDwellTime      time.Duration
	FaultTxInterval          time.Duration
	IdleTxInterval           time.Duration
	IdleCircuitTimeout       time.Duration
	XgressDial               WorkerPoolOptions
	LinkDial                 WorkerPoolOptions
}

type WorkerPoolOptions struct {
	QueueLength uint16
	WorkerCount uint16
}

func DefaultOptions() *Options {
	return &Options{
		LatencyProbeInterval:     DefaultLatencyProbeInterval,
		LatencyProbeTimeout:      DefaultLatencyProbeTimeout,
		XgressCloseCheckInterval: DefaultXgressCloseCheckInterval,
		XgressDialDwellTime:      DefaultXgressDialDwellTime,
		FaultTxInterval:          DefaultFaultTxInterval,
		IdleTxInterval:           DefaultIdleTxInterval,
		IdleCircuitTimeout:       DefaultIdleCircuitTimeout,
		XgressDial: WorkerPoolOptions{
			QueueLength: DefaultXgressDialWorkerQueueLength,
			WorkerCount: DefaultXgressDialWorkerCount,
		},
		LinkDial: WorkerPoolOptions{
			QueueLength: DefaultLinkDialQueueLength,
			WorkerCount: DefaultLinkDialWorkerCount,
		},
	}
}

func LoadOptions(src map[interface{}]interface{}) (*Options, error) {
	options := DefaultOptions()

	if value, found := src["latencyProbeInterval"]; found {
		if latencyProbeInterval, ok := value.(int); ok {
			options.LatencyProbeInterval = time.Duration(latencyProbeInterval) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'latencyProbeInterval'")
		}
	}

	if value, found := src["latencyProbeTimeout"]; found {
		if latencyProbeTimeout, ok := value.(int); ok {
			options.LatencyProbeTimeout = time.Duration(latencyProbeTimeout) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'latencyProbeTimeout'")
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

	if value, found := src["faultTxInterval"]; found {
		if val, ok := value.(int); ok {
			options.FaultTxInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'faultTxInterval'")
		}
	}

	if value, found := src["idleTxInterval"]; found {
		if val, ok := value.(int); ok {
			options.IdleTxInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'idleTxInterval'")
		}
	}

	if value, found := src["idleCircuitTimeout"]; found {
		if val, ok := value.(int); ok {
			options.IdleCircuitTimeout = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'idleCircuitTimeout'")
		}
	}

	if value, found := src["xgressDialQueueLength"]; found {
		if length, ok := value.(int); ok {
			if length < MinXgressDialWorkerQueueLength || length > MaxXgressDialWorkerQueueLength {
				return nil, errors.New(fmt.Sprintf("invalid value for 'xgressDialQueueLength', expected integer between %v and %v", MinXgressDialWorkerQueueLength, MaxXgressDialWorkerQueueLength))
			}
			options.XgressDial.QueueLength = uint16(length)
		} else {
			return nil, errors.New(fmt.Sprintf("invalid value for 'xgressDialQueueLength', expected integer between %v and %v", MinXgressDialWorkerQueueLength, MaxXgressDialWorkerQueueLength))
		}
	}

	if value, found := src["xgressDialWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers < MinXgressDialWorkerCount || workers > MaxXgressDialWorkerCount {
				return nil, errors.New(fmt.Sprintf("invalid value for 'xgressDialWorkerCount', expected integer between %v and %v", MinXgressDialWorkerCount, MaxXgressDialWorkerCount))
			}
			options.XgressDial.WorkerCount = uint16(workers)
		} else {
			return nil, errors.New(fmt.Sprintf("invalid value for 'xgressDialWorkerCount', expected integer between %v and %v", MinXgressDialWorkerCount, MaxXgressDialWorkerCount))
		}
	}

	if value, found := src["linkDialQueueLength"]; found {
		if length, ok := value.(int); ok {
			if length < MinLinkDialWorkerQueueLength || length > MaxLinkDialWorkerQueueLength {
				return nil, errors.New(fmt.Sprintf("invalid value for 'linkDialQueueLength', expected integer between %v and %v", MinLinkDialWorkerQueueLength, MaxLinkDialWorkerQueueLength))
			}
			options.LinkDial.QueueLength = uint16(length)
		} else {
			return nil, errors.New(fmt.Sprintf("invalid value for 'linkDialQueueLength', expected integer between %v and %v", MinLinkDialWorkerQueueLength, MaxLinkDialWorkerQueueLength))
		}
	}

	if value, found := src["linkDialWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers <= MinLinkDialWorkerCount || workers > MaxLinkDialWorkerCount {
				return nil, errors.New(fmt.Sprintf("invalid value for 'linkDialWorkerCount', expected integer between %v and %v", MinLinkDialWorkerCount, MaxLinkDialWorkerCount))
			}
			options.LinkDial.WorkerCount = uint16(workers)
		} else {
			return nil, errors.New(fmt.Sprintf("invalid value for 'linkDialWorkerCount', expected integer between %v and %v", MinLinkDialWorkerCount, MaxLinkDialWorkerCount))
		}
	}

	return options, nil
}
