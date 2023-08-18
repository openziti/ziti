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

package network

import (
	"github.com/pkg/errors"
	"math"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	DefaultOptionsCreateCircuitRetries      = 2
	DefaultOptionsCycleSeconds              = 60
	DefaultOptionsEnableLegacyLinkMgmt      = true
	DefaultOptionsInitialLinkLatency        = 65 * time.Second
	DefaultOptionsPendingLinkTimeout        = 10 * time.Second
	DefaultOptionsMetricsReportInterval     = time.Minute
	DefaultOptionsMinRouterCost             = 10
	DefaultOptionsRouterConnectChurnLimit   = time.Minute
	DefaultOptionsRouterMessagingMaxWorkers = 100
	DefaultOptionsRouterMessagingQueueSize  = 100
	DefaultOptionsRouteTimeout              = 10 * time.Second

	DefaultOptionsSmartRerouteCap          = 4
	DefaultOptionsSmartRerouteFraction     = 0.02
	DefaultOptionsSmartRerouteMinCostDelta = 15

	OptionsRouterCommMaxQueueSize = 1_000_000
	OptionsRouterCommMaxWorkers   = 10_000
)

type Options struct {
	CreateCircuitRetries    uint32
	CycleSeconds            uint32
	EnableLegacyLinkMgmt    bool
	InitialLinkLatency      time.Duration
	IntervalAgeThreshold    time.Duration
	MetricsReportInterval   time.Duration
	MinRouterCost           uint16
	PendingLinkTimeout      time.Duration
	RouteTimeout            time.Duration
	RouterConnectChurnLimit time.Duration
	RouterComm              struct {
		QueueSize  uint32
		MaxWorkers uint32
	}
	Smart struct {
		RerouteFraction float32
		RerouteCap      uint32
		MinCostDelta    uint32
	}
}

func DefaultOptions() *Options {
	options := &Options{
		CreateCircuitRetries:  DefaultOptionsCreateCircuitRetries,
		CycleSeconds:          DefaultOptionsCycleSeconds,
		EnableLegacyLinkMgmt:  DefaultOptionsEnableLegacyLinkMgmt,
		InitialLinkLatency:    DefaultOptionsInitialLinkLatency,
		MetricsReportInterval: DefaultOptionsMetricsReportInterval,
		MinRouterCost:         DefaultOptionsMinRouterCost,
		PendingLinkTimeout:    DefaultOptionsPendingLinkTimeout,
		RouterComm: struct {
			QueueSize  uint32
			MaxWorkers uint32
		}{
			QueueSize:  DefaultOptionsRouterMessagingQueueSize,
			MaxWorkers: DefaultOptionsRouterMessagingMaxWorkers,
		},
		RouterConnectChurnLimit: DefaultOptionsRouterConnectChurnLimit,
		RouteTimeout:            DefaultOptionsRouteTimeout,
		Smart: struct {
			RerouteFraction float32
			RerouteCap      uint32
			MinCostDelta    uint32
		}{
			RerouteFraction: DefaultOptionsSmartRerouteFraction,
			RerouteCap:      DefaultOptionsSmartRerouteCap,
			MinCostDelta:    DefaultOptionsSmartRerouteMinCostDelta,
		},
	}
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

	if value, found := src["smart"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["rerouteFraction"]; found {
				if rerouteFraction, ok := value.(float64); ok {
					options.Smart.RerouteFraction = float32(rerouteFraction)
				} else {
					return nil, errors.New("invalid value for 'rerouteFraction'")
				}
			}

			if value, found := submap["rerouteCap"]; found {
				if rerouteCap, ok := value.(int); ok {
					options.Smart.RerouteCap = uint32(rerouteCap)
				} else {
					return nil, errors.New("invalid value for 'rerouteCap'")
				}
			}

			if value, found := submap["minCostDelta"]; found {
				if minCostDelta, ok := value.(int); ok {
					options.Smart.MinCostDelta = uint32(minCostDelta)
				} else {
					return nil, errors.New("invalid value for 'minCostDelta'")
				}
			}
		} else {
			logrus.Errorf("invalid or empty 'smart' stanza")
		}
	}

	if value, found := src["routerMessaging"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["queueSize"]; found {
				if queueSize, ok := value.(int); ok {
					if queueSize < 0 {
						return nil, errors.New("invalid value for 'routerMessaging.queueSize', must be greater than or equal to 0")
					}
					if queueSize > OptionsRouterCommMaxQueueSize {
						return nil, errors.Errorf("invalid value for 'routerMessaging.queueSize', must be less than or equal to %v", OptionsRouterCommMaxQueueSize)
					}
					options.RouterComm.QueueSize = uint32(queueSize)
				} else {
					return nil, errors.New("invalid value for 'routerMessaging.queueSize'")
				}
			}

			if value, found := submap["maxWorkers"]; found {
				if maxWorkers, ok := value.(int); ok {
					if maxWorkers < 1 {
						return nil, errors.New("invalid value for 'routerMessaging.maxWorkers', must be greater than 0")
					}
					if maxWorkers > OptionsRouterCommMaxWorkers {
						return nil, errors.Errorf("invalid value for 'routerMessaging.maxWorkers', must be less than or equal to %v", OptionsRouterCommMaxWorkers)
					}

					options.RouterComm.MaxWorkers = uint32(maxWorkers)
				} else {
					return nil, errors.New("invalid value for 'routerMessaging.maxWorkers'")
				}
			}
		} else {
			logrus.Errorf("invalid 'routerMessaging' stanza")
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

	if value, found := src["initialLinkLatency"]; found {
		if initialLinkLatencyStr, ok := value.(string); ok {
			val, err := time.ParseDuration(initialLinkLatencyStr)
			if err != nil {
				return nil, errors.Wrap(err, "invalid value for 'initialLinkLatency'")
			}
			options.InitialLinkLatency = val
		} else {
			return nil, errors.New("invalid value for 'initialLinkLatency'")
		}
	}

	if value, found := src["metricsReportInterval"]; found {
		if sval, ok := value.(string); ok {
			val, err := time.ParseDuration(sval)
			if err != nil {
				return nil, errors.Wrap(err, "invalid value for 'metricsReportInterval'")
			}
			options.MetricsReportInterval = val
		} else {
			return nil, errors.New("invalid value for 'metricsReportInterval'")
		}
	}

	if value, found := src["intervalAgeThreshold"]; found {
		if sval, ok := value.(string); ok {
			val, err := time.ParseDuration(sval)
			if err != nil {
				return nil, errors.Wrap(err, "invalid value for 'intervalAgeThreshold'")
			}
			options.IntervalAgeThreshold = val
		} else {
			return nil, errors.New("invalid value for 'intervalAgeThreshold'")
		}
	}

	if value, found := src["enableLegacyLinkMgmt"]; found {
		if bval, ok := value.(bool); ok {
			options.EnableLegacyLinkMgmt = bval
		} else {
			return nil, errors.New("invalid value for 'enableLegacyLinkMgmt'")
		}
	}

	return options, nil
}
