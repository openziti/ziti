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

package controller

import (
	"bytes"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/config"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/transport"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"time"
)

type Config struct {
	Id      *identity.TokenId
	Network *network.Options
	Db      *db.Db
	Trace   struct {
		Handler *channel.TraceHandler
	}
	Profile struct {
		Memory struct {
			Path     string
			Interval time.Duration
		}
		CPU struct {
			Path string
		}
	}
	Ctrl struct {
		Listener transport.Address
		Options  *channel.Options
	}
	Mgmt struct {
		Listener transport.Address
		Options  *channel2.Options
	}
	Metrics      *metrics.Config
	HealthChecks struct {
		BoltCheck struct {
			Interval     time.Duration
			Timeout      time.Duration
			InitialDelay time.Duration
		}
	}
	src map[interface{}]interface{}
}

func (config *Config) Configure(sub config.Subconfig) error {
	return sub.LoadConfig(config.src)
}

func LoadConfig(path string) (*Config, error) {
	cfgBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfgmap := make(map[interface{}]interface{})
	if err = yaml.NewDecoder(bytes.NewReader(cfgBytes)).Decode(&cfgmap); err != nil {
		return nil, err
	}
	config.InjectEnv(cfgmap)
	if value, found := cfgmap["v"]; found {
		if value.(int) != 3 {
			panic("config version mismatch: see docs for information on config updates")
		}
	} else {
		panic("no config version: see docs for information on config")
	}

	var identityConfig *identity.Config

	if value, found := cfgmap["identity"]; found {
		subMap := value.(map[interface{}]interface{})
		identityConfig, err = identity.NewConfigFromMapWithPathContext(subMap, "identity")

		if err != nil {
			return nil, fmt.Errorf("could not parse root identity: %v", err)
		}
	} else {
		return nil, fmt.Errorf("identity section not found")
	}

	controllerConfig := &Config{
		Network: network.DefaultOptions(),
		src:     cfgmap,
	}

	if id, err := identity.LoadIdentity(*identityConfig); err != nil {
		return nil, fmt.Errorf("unable to load identity (%s)", err)
	} else {
		controllerConfig.Id = identity.NewIdentity(id)
	}

	if value, found := cfgmap["network"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if options, err := network.LoadOptions(submap); err == nil {
				controllerConfig.Network = options
			} else {
				return nil, fmt.Errorf("invalid 'network' stanza (%s)", err)
			}
		} else {
			pfxlog.Logger().Warn("invalid or empty 'network' stanza")
		}
	}

	if value, found := cfgmap["db"]; found {
		str, err := db.Open(value.(string))
		if err != nil {
			return nil, err
		}
		controllerConfig.Db = str
	} else {
		panic("controllerConfig must provide [db]")
	}

	if value, found := cfgmap["trace"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["path"]; found {
				handler, err := channel.NewTraceHandler(value.(string), controllerConfig.Id.Token)
				if err != nil {
					return nil, err
				}
				handler.AddDecoder(&channel.Decoder{})
				handler.AddDecoder(&ctrl_pb.Decoder{})
				handler.AddDecoder(&xgress.Decoder{})
				handler.AddDecoder(&mgmt_pb.Decoder{})
				controllerConfig.Trace.Handler = handler
			}
		}
	}

	if value, found := cfgmap["profile"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["memory"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := submap["path"]; found {
						controllerConfig.Profile.Memory.Path = value.(string)
					}
					if value, found := submap["intervalMs"]; found {
						controllerConfig.Profile.Memory.Interval = time.Duration(value.(int)) * time.Millisecond
					} else {
						controllerConfig.Profile.Memory.Interval = 15 * time.Second
					}
				}
			}
			if value, found := submap["cpu"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := submap["path"]; found {
						controllerConfig.Profile.CPU.Path = value.(string)
					}
				}
			}
		}
	}

	if value, found := cfgmap["ctrl"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["listener"]; found {
				listener, err := transport.ParseAddress(value.(string))
				if err != nil {
					return nil, err
				}
				controllerConfig.Ctrl.Listener = listener
			} else {
				panic("controllerConfig must provide [ctrl/listener]")
			}

			controllerConfig.Ctrl.Options = channel.DefaultOptions()
			if value, found := submap["options"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					options, err := channel.LoadOptions(submap)
					if err != nil {
						return nil, err
					}
					controllerConfig.Ctrl.Options = options
					if err := controllerConfig.Ctrl.Options.Validate(); err != nil {
						return nil, fmt.Errorf("error loading channel options for [ctrl/options] (%v)", err)
					}
				}
			}

			if controllerConfig.Trace.Handler != nil {
				controllerConfig.Ctrl.Options.PeekHandlers = append(controllerConfig.Ctrl.Options.PeekHandlers, controllerConfig.Trace.Handler)
			}
		} else {
			panic("controllerConfig [ctrl] section in unexpected format")
		}
	} else {
		panic("controllerConfig must provide [ctrl]")
	}

	if value, found := cfgmap["mgmt"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["listener"]; found {
				listener, err := transport.ParseAddress(value.(string))
				if err != nil {
					return nil, err
				}
				controllerConfig.Mgmt.Listener = listener
			} else {
				panic("controllerConfig must provide [mgmt/listener]")
			}

			controllerConfig.Mgmt.Options = channel2.DefaultOptions()
			if value, found := submap["options"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					controllerConfig.Mgmt.Options = channel2.LoadOptions(submap)
					if err := controllerConfig.Mgmt.Options.Validate(); err != nil {
						return nil, fmt.Errorf("error loading channel options for [mgmt/options] (%v)", err)
					}
				}
			}
		} else {
			panic("controllerConfig [mgmt] section in unexpected format")
		}
	} else {
		panic("controllerConfig must provide [mgmt]")
	}

	if value, found := cfgmap["metrics"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if metricsCfg, err := metrics.LoadConfig(submap); err == nil {
				controllerConfig.Metrics = metricsCfg
			} else {
				return nil, fmt.Errorf("error loading metrics controllerConfig (%s)", err)
			}
		} else {
			pfxlog.Logger().Warn("invalid or empty [metrics] stanza")
		}
	}

	controllerConfig.HealthChecks.BoltCheck.Interval = 30 * time.Second
	controllerConfig.HealthChecks.BoltCheck.Timeout = 20 * time.Second
	controllerConfig.HealthChecks.BoltCheck.InitialDelay = 30 * time.Second

	if value, found := cfgmap["healthChecks"]; found {
		if healthChecksMap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := healthChecksMap["boltCheck"]; found {
				if boltMap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := boltMap["interval"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							controllerConfig.HealthChecks.BoltCheck.Interval = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.bolt.interval value '%v", value)
						}
					}

					if value, found := boltMap["timeout"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							controllerConfig.HealthChecks.BoltCheck.Timeout = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.bolt.timeout value '%v", value)
						}
					}

					if value, found := boltMap["initialDelay"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							controllerConfig.HealthChecks.BoltCheck.InitialDelay = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.bolt.initialDelay value '%v", value)
						}
					}
				} else {
					pfxlog.Logger().Warn("invalid [healthChecks.bolt] stanza")
				}
			}
		} else {
			pfxlog.Logger().Warn("invalid [healthChecks] stanza")
		}
	}

	return controllerConfig, nil
}
