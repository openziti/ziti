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
	"github.com/michaelquigley/pfxlog"
)

type Options struct {
	CycleSeconds uint32
	Smart        struct {
		RerouteFraction float32
		RerouteCap      uint32
	}
}

func DefaultOptions() *Options {
	options := &Options{
		CycleSeconds: 15,
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

	if value, found := src["smart"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["rerouteFraction"]; found {
				if rerouteFraction, ok := value.(float64); ok {
					options.Smart.RerouteFraction = float32(rerouteFraction)
				} else {
					pfxlog.Logger().Errorf("%p", value)
				}
			}

			if value, found := submap["rerouteCap"]; found {
				if rerouteCap, ok := value.(int); ok {
					options.Smart.RerouteCap = uint32(rerouteCap)
				} else {
					pfxlog.Logger().Errorf("%p", value)
				}
			}
		} else {
			pfxlog.Logger().Errorf("invalid or empty 'smart' stanza")
		}
	}

	return options, nil
}
