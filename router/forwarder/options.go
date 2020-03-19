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
	LatencyProbeInterval time.Duration
}

func DefaultOptions() *Options {
	return &Options{LatencyProbeInterval: time.Duration(10) * time.Second}
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

	return options, nil
}
