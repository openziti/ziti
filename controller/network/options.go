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
	"github.com/pkg/errors"
	"math"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	DefaultNetworkOptionsCycleSeconds            = 60
	DefaultNetworkOptionsRouteTimeout            = 10 * time.Second
	DefaultNetworkOptionsCreateCircuitRetries    = 2
	DefaultNetworkOptionsCtrlChanLatencyInterval = 10 * time.Second
	DefaultNetworkOptionsPendingLinkTimeout      = 10 * time.Second
	DefaultNetworkOptionsMinRouterCost           = 10
	DefaultNetworkOptionsRouterConnectChurnLimit = time.Minute
	DefaultNetworkOptionsSmartRerouteFraction    = 0.02
	DefaultNetworkOptionsSmartRerouteCap         = 4
)

type Options struct {
	CycleSeconds uint32
	Smart        struct {
		RerouteFraction float32
		RerouteCap      uint32
	}
	RouteTimeout            time.Duration
	CreateCircuitRetries    uint32
	CtrlChanLatencyInterval time.Duration
	PendingLinkTimeout      time.Duration
	MinRouterCost           uint16
	RouterConnectChurnLimit time.Duration
}

func DefaultOptions() *Options {
	options := &Options{
		CycleSeconds:            DefaultNetworkOptionsCycleSeconds,
		RouteTimeout:            DefaultNetworkOptionsRouteTimeout,
		CreateCircuitRetries:    DefaultNetworkOptionsCreateCircuitRetries,
		CtrlChanLatencyInterval: DefaultNetworkOptionsCtrlChanLatencyInterval,
		PendingLinkTimeout:      DefaultNetworkOptionsPendingLinkTimeout,
		MinRouterCost:           DefaultNetworkOptionsMinRouterCost,
		RouterConnectChurnLimit: DefaultNetworkOptionsRouterConnectChurnLimit,
	}
	options.Smart.RerouteFraction = DefaultNetworkOptionsSmartRerouteFraction
	options.Smart.RerouteCap = DefaultNetworkOptionsSmartRerouteCap
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

	if value, found := src["createCircuitRetries"]; found {
		if createCircuitRetries, ok := value.(int); ok {
			if createCircuitRetries < 0 {
				return nil, errors.New("invalid uint32 value for 'createCircuitRetries'")
			}
			options.CreateCircuitRetries = uint32(createCircuitRetries)
		} else {
			return nil, errors.New("invalid value for 'createCircuitRetries'")
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

	if value, found := src["pendingLinkTimeoutSeconds"]; found {
		if pendingLinkTimeoutSeconds, ok := value.(int); ok {
			options.PendingLinkTimeout = time.Duration(pendingLinkTimeoutSeconds) * time.Second
		} else {
			return nil, errors.New("invalid value for 'pendingLinkTimeoutSeconds'")
		}
	}

	if value, found := src["minRouterCost"]; found {
		if minRouterCost, ok := value.(int); ok {
			if minRouterCost < 0 || minRouterCost > math.MaxUint16 {
				logrus.Fatalf("invalid network.minRouterCost value. Must be between 0 and %v", math.MaxUint16)
			}
			options.MinRouterCost = uint16(minRouterCost)
		} else {
			logrus.Fatalf("invalid network.minRouterCost value. Must be between number between 0 and %v", math.MaxUint16)
		}
	}

	if value, found := src["routerConnectChurnLimit"]; found {
		if routerConnectChurnLimitStr, ok := value.(string); ok {
			val, err := time.ParseDuration(routerConnectChurnLimitStr)
			if err != nil {
				return nil, errors.Wrap(err, "invalid value for 'routerConnectChurnLimit'")
			}
			options.RouterConnectChurnLimit = val
		} else {
			return nil, errors.New("invalid value for 'routerConnectChurnLimit'")
		}
	}

	return options, nil
}
