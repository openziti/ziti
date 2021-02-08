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

package network

import (
	"errors"
	"github.com/sirupsen/logrus"
	"time"
)

type Options struct {
	CycleSeconds uint32
	Smart        struct {
		RerouteFraction float32
		RerouteCap      uint32
	}
	RouteTimeout            time.Duration
	CtrlChanLatencyInterval time.Duration
}

func DefaultOptions() *Options {
	options := &Options{
		CycleSeconds:            60,
		RouteTimeout:            10 * time.Second,
		CtrlChanLatencyInterval: 10 * time.Second,
	}
	options.Smart.RerouteFraction = 0.02
	options.Smart.RerouteCap = 4
	return options
}

func LoadOptions(src map[interface{}]interface{}) (*Options, error) {
	options := DefaultOptions()

	if value, found := src["cycleSeconds"]; found {
		if cycleSeconds, ok := value.(int); ok {
			options.CycleSeconds = uint32(cycleSeconds)
		} else {
			return nil, errors.New("invalid value for 'cycleSeconds'")
		}
	}

	if value, found := src["routeTimeoutSeconds"]; found {
		if routeTimeoutSeconds, ok := value.(int); ok {
			options.RouteTimeout = time.Duration(routeTimeoutSeconds) * time.Second
		} else {
			return nil, errors.New("invalid value for 'routeTimeoutSeconds'")
		}
	}

	if value, found := src["ctrlChanLatencyIntervalSeconds"]; found {
		if val, ok := value.(int); ok {
			options.CtrlChanLatencyInterval = time.Duration(val) * time.Second
		} else {
			return nil, errors.New("invalid value for 'ctrlChanLatencyIntervalSeconds'")
		}
	}

	if value, found := src["smart"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["rerouteFraction"]; found {
				if rerouteFraction, ok := value.(float64); ok {
					options.Smart.RerouteFraction = float32(rerouteFraction)
				} else {
					logrus.Errorf("%p", value)
				}
			}

			if value, found := submap["rerouteCap"]; found {
				if rerouteCap, ok := value.(int); ok {
					options.Smart.RerouteCap = uint32(rerouteCap)
				} else {
					logrus.Errorf("%p", value)
				}
			}
		} else {
			logrus.Errorf("invalid or empty 'smart' stanza")
		}
	}

	return options, nil
}
