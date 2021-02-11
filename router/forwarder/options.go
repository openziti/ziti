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
	"time"
)

type Options struct {
	LatencyProbeInterval     time.Duration
	XgressCloseCheckInterval time.Duration
	FaultTxInterval          time.Duration
	XgressDial               WorkerPoolOptions
	LinkDial                 WorkerPoolOptions
}

type WorkerPoolOptions struct {
	QueueLength uint16
	WorkerCount uint16
}

func DefaultOptions() *Options {
	return &Options{
		LatencyProbeInterval:     10 * time.Second,
		XgressCloseCheckInterval: 5 * time.Second,
		FaultTxInterval:          15 * time.Second,
		XgressDial: WorkerPoolOptions{
			QueueLength: 1000,
			WorkerCount: 10,
		},
		LinkDial: WorkerPoolOptions{
			QueueLength: 1000,
			WorkerCount: 10,
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

	if value, found := src["xgressCloseCheckInterval"]; found {
		if val, ok := value.(int); ok {
			options.XgressCloseCheckInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'latencyProbeInterval'")
		}
	}

	if value, found := src["faultTxInterval"]; found {
		if val, ok := value.(int); ok {
			options.FaultTxInterval = time.Duration(val) * time.Millisecond
		} else {
			return nil, errors.New("invalid value for 'faultTxInterval")
		}
	}

	if value, found := src["xgressDialQueueLength "]; found {
		if length, ok := value.(int); ok {
			if length <= 0 || length > 10000 {
				return nil, errors.New("invalid value for 'xgressDialQueueLength ', expected integer between 1 and 1000")
			}
			options.XgressDial.QueueLength = uint16(length)
		} else {
			return nil, errors.New("invalid value for 'xgressDialQueueLength ', expected integer between 1 and 1000")
		}
	}

	if value, found := src["xgressDialWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers <= 0 || workers > 10000 {
				return nil, errors.New("invalid value for 'xgressDialWorkerCount', expected integer between 1 and 1000")
			}
			options.XgressDial.WorkerCount = uint16(workers)
		} else {
			return nil, errors.New("invalid value for 'xgressDialWorkerCount', expected integer between 1 and 1000")
		}
	}

	if value, found := src["linkDialQueueLength"]; found {
		if length, ok := value.(int); ok {
			if length <= 0 || length > 10000 {
				return nil, errors.New("invalid value for 'linkDialQueueLength', expected integer between 1 and 1000")
			}
			options.LinkDial.QueueLength = uint16(length)
		} else {
			return nil, errors.New("invalid value for 'linkDialQueueLength', expected integer between 1 and 1000")
		}
	}

	if value, found := src["linkDialWorkerCount"]; found {
		if workers, ok := value.(int); ok {
			if workers <= 0 || workers > 10000 {
				return nil, errors.New("invalid value for 'linkDialWorkerCount', expected integer between 10 and 1000")
			}
			options.LinkDial.WorkerCount = uint16(workers)
		} else {
			return nil, errors.New("invalid value for 'linkDialWorkerCount', expected integer between 10 and 1000")
		}
	}

	return options, nil
}
