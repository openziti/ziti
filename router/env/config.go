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

package env

import (
	"bytes"
	"fmt"
	"github.com/openziti/transport/v2/tls"
	"github.com/openziti/ziti/controller/command"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/config"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/xgress"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	yaml3 "gopkg.in/yaml.v3"
)

const (

	// PathMapKey is used to store a loaded configuration file's source path
	PathMapKey = "@file"

	// CtrlMapKey is the string key for the ctrl section
	CtrlMapKey = "ctrl"

	// CtrlEndpointMapKey is the string key for the ctrl.endpoint section
	CtrlEndpointMapKey = "endpoint"

	// CtrlEndpointsMapKey is the string key for the ctrl.endpoints section
	CtrlEndpointsMapKey = "endpoints"

	// CtrlEndpointBindMapKey is the string key for the ctrl.bind section
	CtrlEndpointBindMapKey = "bind"

	// CtrlRateLimiterMinSizeValue is the minimum size that can be configured for the control channel rate limiter
	// window range
	CtrlRateLimiterMinSizeValue = 5

	// CtrlRateLimiterMaxSizeValue is the maximum size that can be configured for the control channel rate limiter
	// window range
	CtrlRateLimiterMaxSizeValue = 1000

	// CtrlRateLimiterMetricOutstandingCount is the name of the metric tracking how many tasks are in process
	CtrlRateLimiterMetricOutstandingCount = "ctrl_limiter.in_process"

	// CtrlRateLimiterMetricCurrentWindowSize is the name of the metric tracking the current window size
	CtrlRateLimiterMetricCurrentWindowSize = "ctrl_limiter.window_size"

	// CtrlRateLimiterMetricWorkTimer is the name of the metric tracking how long successful tasks are taking to complete
	CtrlRateLimiterMetricWorkTimer = "ctrl_limiter.work_timer"

	CtrlHaMapKey = "ha"

	ConnectEventsMapKey = "connectEvents"

	DefaultConnectEventsEnabled = true

	DefaultConnectEventsBatchInterval = 3 * time.Second
	MinConnectEventsBatchInterval     = 250 * time.Millisecond
	MaxConnectEventsBatchInterval     = 10 * time.Minute

	DefaultConnectEventsMaxQueuedEvents = 100_000
	MinConnectEventsMaxQueuedEvents     = 0
	MaxConnectEventsMaxQueuedEvents     = 10_000_000

	DefaultConnectEventsFullSyncInterval = 5 * time.Minute
	MinConnectEventsFullSyncInterval     = time.Second
	MaxConnectEventsFullSyncInterval     = 24 * time.Hour
)

// internalConfigKeys is used to distinguish internally defined configuration vs file configuration
var internalConfigKeys = []string{
	PathMapKey,
}

func LoadConfigMap(path string) (map[interface{}]interface{}, error) {
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfgmap := make(map[interface{}]interface{})
	if err = yaml.NewDecoder(bytes.NewReader(yamlBytes)).Decode(&cfgmap); err != nil {
		return nil, err
	}

	config.InjectEnv(cfgmap)

	cfgmap[PathMapKey] = path

	return cfgmap, nil
}

type Config struct {
	IdConfig       *identity.Config
	Id             *identity.TokenId
	EnableDebugOps bool
	Forwarder      *Options
	Trace          struct {
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
		InitialEndpoints      []*UpdatableAddress
		LocalBinding          string
		DefaultRequestTimeout time.Duration
		Options               *channel.Options
		EndpointsFile         string
		Heartbeats            HeartbeatOptions
		StartupTimeout        time.Duration
		RateLimit             command.AdaptiveRateLimiterConfig
	}
	Link struct {
		Listeners  []map[interface{}]interface{}
		Dialers    []map[interface{}]interface{}
		Heartbeats channel.HeartbeatOptions
	}
	Dialers   map[string]xgress.OptionsData
	Listeners []ListenerBinding
	Transport map[interface{}]interface{}
	Metrics   struct {
		ReportInterval        time.Duration
		IntervalAgeThreshold  time.Duration
		MessageQueueSize      int
		EventQueueSize        int
		EnableDataDelayMetric bool
	}
	HealthChecks struct {
		CtrlPingCheck struct {
			Interval     time.Duration
			Timeout      time.Duration
			InitialDelay time.Duration
		}
		LinkCheck struct {
			MinLinks     int
			Interval     time.Duration
			InitialDelay time.Duration
		}
	}
	ConnectEvents ConnectEventsConfig
	Proxy         *transport.ProxyConfiguration
	Plugins       []string
	Edge          *EdgeConfig
	Src           map[interface{}]interface{}
	path          string
}

func (config *Config) CurrentCtrlAddress() string {
	return config.Ctrl.InitialEndpoints[0].String()
}

func (config *Config) Configure(sub config.Subconfig) error {
	return sub.LoadConfig(config.Src)
}

const (
	TimeFormatYear    = "2006"
	TimeFormatMonth   = "01"
	TimeFormatDay     = "02"
	TimeFormatHour    = "15"
	TimeFormatMinute  = "04"
	TimeFormatSeconds = "05"
	TimestampFormat   = TimeFormatYear + TimeFormatMonth + TimeFormatDay + TimeFormatHour + TimeFormatMinute + TimeFormatSeconds

	DefaultLinkHeartbeatSendInterval = 10 * time.Second
	DefaultLinkUnresponsiveTimeout   = time.Minute
)

// CreateBackup will attempt to use the current path value to create a backup of
// the file on disk. The resulting file path is returned.
func (config *Config) CreateBackup() (string, error) {
	source, err := os.Open(config.path)
	if err != nil {
		return "", fmt.Errorf("could not open path %s: %v", config.path, err)
	}
	defer func() { _ = source.Close() }()

	destPath := config.path + ".backup." + time.Now().Format(TimestampFormat)
	destination, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("could not create backup file: %v", err)
	}
	defer func() { _ = destination.Close() }()

	if _, err := io.Copy(destination, source); err != nil {
		return "", fmt.Errorf("could not copy to backup file: %v", err)
	}

	return destPath, nil
}

func deepCopyMap(src map[interface{}]interface{}, dest map[interface{}]interface{}) {
	for key, value := range src {
		switch src[key].(type) {
		case map[interface{}]interface{}:
			dest[key] = map[interface{}]interface{}{}
			deepCopyMap(src[key].(map[interface{}]interface{}), dest[key].(map[interface{}]interface{}))
		default:
			dest[key] = value
		}
	}
}

// cleanSrcCopy returns a copy of the current src map[interface{}]interface{} without internal
// keys like @FlagsCfgMapKey and @PathMapKey.
func (config *Config) cleanSrcCopy() map[interface{}]interface{} {
	out := map[interface{}]interface{}{}
	deepCopyMap(config.Src, out)

	for _, internalKey := range internalConfigKeys {
		delete(out, internalKey)
	}

	return out
}

// Save attempts to take the current config's src attribute and Save it as
// yaml to the path value.
func (config *Config) Save() error {
	if config.path == "" {
		return errors.New("no path provided in configuration, cannot save")
	}

	if _, err := os.Stat(config.path); err != nil {
		return fmt.Errorf("invalid path %s: %v", config.path, err)
	}

	outSrc := config.cleanSrcCopy()
	out, err := yaml.Marshal(outSrc)

	if err != nil {
		return err
	}

	file, err := os.Create(config.path)

	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	_, err = file.Write(out)

	return err
}

// UpdatableAddress allows a single address to be passed to multiple channel implementations and be centrally updated
// in a thread safe manner.
type UpdatableAddress struct {
	wrapped concurrenz.AtomicValue[transport.Address]
}

// UpdatableAddress implements transport.Address
var _ transport.Address = &UpdatableAddress{}

// NewUpdatableAddress create a new *UpdatableAddress which implements transport.Address and allow
// thread safe updating of the internal address
func NewUpdatableAddress(address transport.Address) *UpdatableAddress {
	ret := &UpdatableAddress{}
	ret.wrapped.Store(address)
	return ret
}

// Listen implements transport.Address.Listen
func (c *UpdatableAddress) Listen(name string, i *identity.TokenId, acceptF func(transport.Conn), tcfg transport.Configuration) (io.Closer, error) {
	return c.getWrapped().Listen(name, i, acceptF, tcfg)
}

// MustListen implements transport.Address.MustListen
func (c *UpdatableAddress) MustListen(name string, i *identity.TokenId, acceptF func(transport.Conn), tcfg transport.Configuration) io.Closer {
	return c.getWrapped().MustListen(name, i, acceptF, tcfg)
}

// String implements transport.Address.String
func (c *UpdatableAddress) String() string {
	return c.getWrapped().String()
}

// Type implements transport.Address.Type
func (c *UpdatableAddress) Type() string {
	return c.getWrapped().Type()
}

// Dial implements transport.Address.Dial
func (c *UpdatableAddress) Dial(name string, i *identity.TokenId, timeout time.Duration, tcfg transport.Configuration) (transport.Conn, error) {
	return c.getWrapped().Dial(name, i, timeout, tcfg)
}

func (c *UpdatableAddress) DialWithLocalBinding(name string, binding string, i *identity.TokenId, timeout time.Duration, tcfg transport.Configuration) (transport.Conn, error) {
	return c.getWrapped().DialWithLocalBinding(name, binding, i, timeout, tcfg)
}

// getWrapped loads the current transport.Address
func (c *UpdatableAddress) getWrapped() transport.Address {
	return c.wrapped.Load()
}

// Store updates the address currently used by this configuration instance
func (c *UpdatableAddress) Store(address transport.Address) {
	c.wrapped.Store(address)
}

// MarshalYAML handles serialization for the YAML format
func (c *UpdatableAddress) MarshalYAML() (interface{}, error) {
	return c.String(), nil
}

// UnmarshalYAML handled deserialization for the YAML format
func (c *UpdatableAddress) UnmarshalYAML(value *yaml3.Node) error {
	var yamlAddress string
	err := value.Decode(&yamlAddress)
	if err != nil {
		return err
	}

	addr, err := transport.ParseAddress(yamlAddress)
	if err != nil {
		return err
	}
	c.Store(addr)

	return nil
}

func LoadConfig(path string) (*Config, error) {
	return LoadConfigWithOptions(path, true)
}

func LoadConfigWithOptions(path string, loadIdentity bool) (*Config, error) {
	cfgmap, err := LoadConfigMap(path)

	if err != nil {
		return nil, err
	}

	if value, found := cfgmap["v"]; found {
		if value.(int) != 3 {
			panic("config version mismatch: see docs for information on config updates")
		}
	} else {
		panic("config version mismatch: no configuration version specified")
	}

	cfg := &Config{Src: cfgmap}

	identityConfig, err := LoadIdentityConfigFromMap(cfgmap)

	if err != nil {
		return nil, fmt.Errorf("unable to load identity: %v", err)
	}

	cfg.IdConfig = identityConfig

	if loadIdentity {
		if id, err := identity.LoadIdentity(*identityConfig); err != nil {
			return nil, fmt.Errorf("unable to load identity (%w)", err)
		} else {
			cfg.Id = identity.NewIdentity(id)

			if err := cfg.Id.WatchFiles(); err != nil {
				pfxlog.Logger().Warn("could not enable file watching on identity: %w", err)
			}
		}
	}

	if value, found := cfgmap[PathMapKey]; found {
		cfg.path = value.(string)
	}

	if value, found := cfgmap["enableDebugOps"]; found {
		if bVal, ok := value.(bool); ok {
			cfg.EnableDebugOps = bVal
		}
	}

	cfg.Forwarder = DefaultOptions()
	if value, found := cfgmap["forwarder"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if options, err := LoadOptions(submap); err == nil {
				cfg.Forwarder = options
			} else {
				return nil, fmt.Errorf("invalid 'forwarder' stanza (%w)", err)
			}
		} else {
			pfxlog.Logger().Warn("invalid or empty 'forwarder' stanza")
		}
	}

	if value, found := cfgmap["trace"]; found {
		submap := value.(map[interface{}]interface{})
		if value, found := submap["path"]; found {
			handler, err := channel.NewTraceHandler(value.(string), cfg.Id.Token)
			if err != nil {
				return nil, err
			}
			handler.AddDecoder(channel.Decoder{})
			handler.AddDecoder(xgress.Decoder{})
			handler.AddDecoder(ctrl_pb.Decoder{})
			cfg.Trace.Handler = handler
		}
	}

	if value, found := cfgmap["profile"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["memory"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := submap["path"]; found {
						cfg.Profile.Memory.Path = value.(string)
					}
					if value, found := submap["intervalMs"]; found {
						cfg.Profile.Memory.Interval = time.Duration(value.(int)) * time.Millisecond
					} else {
						cfg.Profile.Memory.Interval = 15 * time.Second
					}
				}
			}
			if value, found := submap["cpu"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := submap["path"]; found {
						cfg.Profile.CPU.Path = value.(string)
					}
				}
			}
		}
	}

	cfg.Ctrl.DefaultRequestTimeout = 5 * time.Second
	cfg.Ctrl.Options = channel.DefaultOptions()
	cfg.Ctrl.Heartbeats = *NewDefaultHeartbeatOptions()
	cfg.Ctrl.StartupTimeout = 30 * time.Second

	if value, found := cfgmap[CtrlMapKey]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap[CtrlEndpointBindMapKey]; found {
				_, err := transport.ResolveInterface(value.(string))
				if err != nil {
					return nil, fmt.Errorf("cannot parse [ctrl/bind] (%s)", err)
				}
				cfg.Ctrl.LocalBinding = value.(string)
			}

			if value, found := submap[CtrlEndpointMapKey]; found {
				address, err := transport.ParseAddress(value.(string))
				if err != nil {
					return nil, fmt.Errorf("cannot parse [ctrl/endpoint] (%s)", err)
				}
				cfg.Ctrl.InitialEndpoints = append(cfg.Ctrl.InitialEndpoints, NewUpdatableAddress(address))
			} else if value, found := submap[CtrlEndpointsMapKey]; found {
				if addresses, ok := value.([]interface{}); ok {
					for idx, value := range addresses {
						addressStr, ok := value.(string)
						if !ok {
							return nil, errors.Errorf("[ctrl/endpoints] value at position %v is not a string. value: %v", idx+1, value)
						}
						address, err := transport.ParseAddress(addressStr)
						if err != nil {
							return nil, errors.Wrapf(err, "cannot parse [ctrl/endpoints] value at position %v", idx+1)
						}
						cfg.Ctrl.InitialEndpoints = append(cfg.Ctrl.InitialEndpoints, NewUpdatableAddress(address))
					}
				} else {
					return nil, errors.New("cannot parse [ctrl/endpoints], must be list")
				}
			}

			if value, found := submap["options"]; found {
				if optionsMap, ok := value.(map[interface{}]interface{}); ok {
					options, err := channel.LoadOptions(optionsMap)
					if err != nil {
						return nil, errors.Wrap(err, "unable to load control channel options")
					}
					cfg.Ctrl.Options = options
					if err := cfg.Ctrl.Options.Validate(); err != nil {
						return nil, fmt.Errorf("error loading channel options for [ctrl/options] (%v)", err)
					}
				}

				if value, found := submap["heartbeats"]; found {
					if submap, ok := value.(map[interface{}]interface{}); ok {
						options, err := channel.LoadHeartbeatOptions(submap)
						if err != nil {
							return nil, err
						}
						heartbeats, err := NewHeartbeatOptions(options)
						if err != nil {
							return nil, err
						}
						cfg.Ctrl.Heartbeats = *heartbeats
					}
				}
			}
			if value, found := submap["defaultRequestTimeout"]; found {
				var err error
				if cfg.Ctrl.DefaultRequestTimeout, err = time.ParseDuration(value.(string)); err != nil {
					return nil, errors.Wrap(err, "invalid value for ctrl.defaultRequestTimeout")
				}
			}
			if value, found := submap["startupTimeout"]; found {
				var err error
				if cfg.Ctrl.StartupTimeout, err = time.ParseDuration(value.(string)); err != nil {
					return nil, errors.Wrap(err, "invalid value for ctrl.startupTimeout")
				}
			}
			if value, found := submap["endpointsFile"]; found {
				cfg.Ctrl.EndpointsFile = value.(string)
			} else {
				cfg.Ctrl.EndpointsFile = filepath.Join(filepath.Dir(cfg.path), "endpoints.yml")
			}
			if err = cfg.loadCtrlRateLimiterConfig(submap); err != nil {
				return nil, err
			}
		}
	}

	cfg.Link.Heartbeats = *channel.DefaultHeartbeatOptions()
	cfg.Link.Heartbeats.SendInterval = DefaultLinkHeartbeatSendInterval
	cfg.Link.Heartbeats.CloseUnresponsiveTimeout = DefaultLinkUnresponsiveTimeout

	if value, found := cfgmap["link"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["listeners"]; found {
				if subarr, ok := value.([]interface{}); ok {
					for _, value := range subarr {
						if lmap, ok := value.(map[interface{}]interface{}); ok {
							if value, found := lmap["binding"]; found {
								if _, ok := value.(string); ok {
									cfg.Link.Listeners = append(cfg.Link.Listeners, lmap)
								} else {
									return nil, fmt.Errorf("[link/listeners] must provide string [binding] (%v)", lmap)
								}
							} else {
								return nil, fmt.Errorf("[link/listeners] must provide [binding] (%v)", lmap)
							}
						} else {
							return nil, fmt.Errorf("[link/listeners] must express a map (%v)", value)
						}
					}
				} else {
					return nil, fmt.Errorf("[link/listenrs] must provide at least one listener (%v)", value)
				}
			}
			if value, found := submap["dialers"]; found {
				if subarr, ok := value.([]interface{}); ok {
					for _, value := range subarr {
						if lmap, ok := value.(map[interface{}]interface{}); ok {
							if value, found := lmap["binding"]; found {
								if _, ok := value.(string); ok {
									cfg.Link.Dialers = append(cfg.Link.Dialers, lmap)
								} else {
									return nil, fmt.Errorf("[link/dialers] must provide string [binding] (%v)", lmap)
								}
							} else {
								return nil, fmt.Errorf("[link/dialers] must provide [binding] (%v)", lmap)
							}
						} else {
							return nil, fmt.Errorf("[link/dialers] must express a map (%v)", value)
						}
					}
				} else {
					return nil, fmt.Errorf("[link/dialers] must provide at least one dialer (%v)", value)
				}
			}

			if value, found := submap["heartbeats"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					options, err := channel.LoadHeartbeatOptions(submap)
					if err != nil {
						return nil, err
					}
					cfg.Link.Heartbeats = *options
				}
			}
		}
	}

	if value, found := cfgmap["dialers"]; found {
		if subarr, ok := value.([]interface{}); ok {
			for _, value := range subarr {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := submap["binding"]; found {
						binding := value.(string)
						if cfg.Dialers == nil {
							cfg.Dialers = make(map[string]xgress.OptionsData)
						}
						cfg.Dialers[binding] = submap
					} else {
						return nil, fmt.Errorf("[dialer] must provide [binding] (%v)", submap)
					}
				}
			}
		}
	}

	if value, found := cfgmap["listeners"]; found {
		if subarr, ok := value.([]interface{}); ok {
			for _, value := range subarr {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					binding, found := submap["binding"].(string)
					if !found {
						return nil, fmt.Errorf("[listener] must provide [binding] (%v)", submap)
					}
					cfg.Listeners = append(cfg.Listeners, ListenerBinding{Name: binding, Options: submap})
				}
			}
		}
	}

	if value, found := cfgmap["transport"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			cfg.Transport = submap
		}
	}

	cfg.Metrics.ReportInterval = time.Minute
	cfg.Metrics.MessageQueueSize = 10
	cfg.Metrics.EventQueueSize = 256

	if value, found := cfgmap["metrics"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["reportInterval"]; found {
				var err error
				if cfg.Metrics.ReportInterval, err = time.ParseDuration(value.(string)); err != nil {
					return nil, errors.Wrap(err, "invalid value for metrics.reportInterval")
				}
			}
			if value, found := submap["intervalAgeThreshold"]; found {
				var err error
				if cfg.Metrics.IntervalAgeThreshold, err = time.ParseDuration(value.(string)); err != nil {
					return nil, errors.Wrap(err, "invalid value for metrics.intervalAgeThreshold")
				}
			}
			if value, found := submap["messageQueueSize"]; found {
				if intVal, ok := value.(int); ok {
					cfg.Metrics.MessageQueueSize = intVal
				} else {
					return nil, errors.New("invalid value for metrics.messageQueueSize")
				}
			}
			if value, found := submap["eventQueueSize"]; found {
				if intVal, ok := value.(int); ok {
					cfg.Metrics.EventQueueSize = intVal
				} else {
					return nil, errors.New("invalid value for metrics.eventQueueSize")
				}
			}
			if value, found := submap["enableDataDelayMetric"]; found {
				cfg.Metrics.EnableDataDelayMetric = strings.EqualFold("true", fmt.Sprintf("%v", value))
			}
		}
	}

	cfg.HealthChecks.CtrlPingCheck.Interval = 30 * time.Second
	cfg.HealthChecks.CtrlPingCheck.Timeout = 15 * time.Second
	cfg.HealthChecks.CtrlPingCheck.InitialDelay = 15 * time.Second
	cfg.HealthChecks.LinkCheck.Interval = 5 * time.Second
	cfg.HealthChecks.LinkCheck.InitialDelay = 1 * time.Second
	cfg.HealthChecks.LinkCheck.MinLinks = 0

	if value, found := cfgmap["healthChecks"]; found {
		if healthChecksMap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := healthChecksMap["ctrlPingCheck"]; found {
				if boltMap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := boltMap["interval"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							cfg.HealthChecks.CtrlPingCheck.Interval = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.ctrlPingCheck.interval value '%v", value)
						}
					}

					if value, found := boltMap["timeout"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							cfg.HealthChecks.CtrlPingCheck.Timeout = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.ctrlPingCheck.timeout value '%v", value)
						}
					}

					if value, found := boltMap["initialDelay"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							cfg.HealthChecks.CtrlPingCheck.InitialDelay = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.ctrlPingCheck.initialDelay value '%v", value)
						}
					}
				} else {
					pfxlog.Logger().Warn("invalid [healthChecks.ctrlPingCheck] stanza")
				}
			}
			if value, found := healthChecksMap["linkCheck"]; found {
				if checkMap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := checkMap["interval"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							cfg.HealthChecks.LinkCheck.Interval = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.linkCheck.interval value '%v", value)
						}
					}
					if value, found := checkMap["minLinks"]; found {
						if val, ok := value.(int); ok {
							cfg.HealthChecks.LinkCheck.MinLinks = val
						} else {
							return nil, errors.Wrapf(err, "invalid value [%v] for healthChecks.linkCheck.minLinks", value)
						}
					}

					if value, found := checkMap["initialDelay"]; found {
						if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
							cfg.HealthChecks.LinkCheck.InitialDelay = val
						} else {
							return nil, errors.Wrapf(err, "failed to parse healthChecks.linkCheck.initialDelay value '%v", value)
						}
					}

				} else {
					pfxlog.Logger().Warn("invalid [healthChecks.linkCheck] stanza")
				}
			}
		} else {
			pfxlog.Logger().Warn("invalid [healthChecks] stanza")
		}
	}

	if value, found := cfgmap["plugins"]; found {
		if list, ok := value.([]interface{}); ok {
			for _, v := range list {
				if plugin, ok := v.(string); ok {
					cfg.Plugins = append(cfg.Plugins, plugin)
				} else {
					pfxlog.Logger().Warnf("invalid plugin value: '%v'", plugin)
				}
			}
		} else {
			pfxlog.Logger().Warn("invalid plugins value")
		}
	}

	if value, found := cfgmap[transport.KeyProxy]; found {
		if proxyMap, ok := value.(map[interface{}]interface{}); ok {
			proxyConfig, err := transport.LoadProxyConfiguration(proxyMap)
			if err != nil {
				return nil, err
			}
			cfg.Proxy = proxyConfig
		} else {
			pfxlog.Logger().Warn("invalid proxy configuration, must be map")
		}
	}

	if value, found := cfgmap["tls"]; found {
		if tlsMap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := tlsMap["handshakeTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					tls.SetSharedListenerHandshakeTimeout(val)
				} else {
					return nil, errors.Wrapf(err, "failed to parse tls.handshakeTimeout value '%v", value)
				}
			}
		}
	}

	cfg.ConnectEvents.Enabled = DefaultConnectEventsEnabled
	cfg.ConnectEvents.BatchInterval = DefaultConnectEventsBatchInterval
	cfg.ConnectEvents.FullSyncInterval = DefaultConnectEventsFullSyncInterval
	cfg.ConnectEvents.MaxQueuedEvents = DefaultConnectEventsMaxQueuedEvents

	if value, found := cfgmap[ConnectEventsMapKey]; found {
		if connectEvents, ok := value.(map[interface{}]interface{}); !ok {
			pfxlog.Logger().Warn("invalid xgress edge listener options value: connectEvents value should be map")
		} else {
			if value, found := connectEvents["enabled"]; found {
				enabled := strings.EqualFold("true", fmt.Sprintf("%v", value))
				cfg.ConnectEvents.Enabled = enabled
			}

			if value, found := connectEvents["batchInterval"]; found {
				if strVal, ok := value.(string); ok {
					interval, err := time.ParseDuration(strVal)
					if err != nil {
						pfxlog.Logger().WithError(err).Warn("invalid value: connectEvents.batchInterval value should be a valid duration")
					} else {
						cfg.ConnectEvents.BatchInterval = interval
					}
				} else {
					pfxlog.Logger().Warn("invalid value: connectEvents.batchInterval value should be a string representing a duration")
				}
			}

			if value, found := connectEvents["fullSyncInterval"]; found {
				if strVal, ok := value.(string); ok {
					interval, err := time.ParseDuration(strVal)
					if err != nil {
						pfxlog.Logger().WithError(err).Warn("invalid value: connectEvents.fullSyncInterval value should be a valid duration")
					} else {
						cfg.ConnectEvents.FullSyncInterval = interval
					}
				} else {
					pfxlog.Logger().Warn("invalid value: connectEvents.fullSyncInterval value should be a string representing a duration")
				}
			}

			if value, found := connectEvents["maxQueuedEvents"]; found {
				if intVal, ok := value.(int); ok {
					cfg.ConnectEvents.MaxQueuedEvents = int64(intVal)
				} else {
					pfxlog.Logger().Warn("invalid value: connectEvents.fullSyncInterval should be a positive integer value")
				}
			}
		}
	}

	if cfg.ConnectEvents.BatchInterval < MinConnectEventsBatchInterval {
		pfxlog.Logger().Warnf("connectEvents.batchInterval less than allowed minimum of %s", MinConnectEventsBatchInterval.String())
		cfg.ConnectEvents.BatchInterval = MinConnectEventsBatchInterval
	}

	if cfg.ConnectEvents.BatchInterval > MaxConnectEventsBatchInterval {
		pfxlog.Logger().Warnf("connectEvents.batchInterval greater than allowed maximum of %s", MaxConnectEventsBatchInterval.String())
		cfg.ConnectEvents.BatchInterval = MaxConnectEventsBatchInterval
	}

	if cfg.ConnectEvents.FullSyncInterval < MinConnectEventsBatchInterval {
		pfxlog.Logger().Warnf("connectEvents.fullSyncInterval less than allowed minimum of %s", MinConnectEventsFullSyncInterval.String())
		cfg.ConnectEvents.FullSyncInterval = MinConnectEventsFullSyncInterval
	}

	if cfg.ConnectEvents.FullSyncInterval > MaxConnectEventsBatchInterval {
		pfxlog.Logger().Warnf("connectEvents.fullSyncInterval greater than allowed maximum of %s", MaxConnectEventsFullSyncInterval.String())
		cfg.ConnectEvents.FullSyncInterval = MaxConnectEventsFullSyncInterval
	}

	if cfg.ConnectEvents.MaxQueuedEvents < MinConnectEventsMaxQueuedEvents {
		pfxlog.Logger().Warnf("connectEvents.maxQueuedEvents less than allowed minimum of %d", MinConnectEventsMaxQueuedEvents)
		cfg.ConnectEvents.MaxQueuedEvents = MinConnectEventsMaxQueuedEvents
	}

	if cfg.ConnectEvents.MaxQueuedEvents > MaxConnectEventsMaxQueuedEvents {
		pfxlog.Logger().Warnf("connectEvents.maxQueuedEvents greater than allowed maximum of %d", MaxConnectEventsMaxQueuedEvents)
		cfg.ConnectEvents.MaxQueuedEvents = MaxConnectEventsMaxQueuedEvents
	}

	cfg.Edge = NewEdgeConfig(cfg)
	if err = cfg.Edge.LoadEdgeConfigFromMap(cfgmap, loadIdentity); err != nil {
		return nil, err
	}

	if loadIdentity {
		var errs []error
		for _, c := range cfg.Link.Listeners {
			a := c["advertise"]
			if a != nil {
				// should start with tls: or dtls:
				parts := strings.Split(a.(string), ":")
				if parts[0] == "tls" || parts[0] == "dtls" {
					addy := parts[1]
					e := cfg.Id.ValidFor(addy)
					if e != nil {
						errs = append(errs, fmt.Errorf("invalid link.listeners.advertise: %s, error: %v", a.(string), e))
					}
				}
			}
		}

		for _, c := range cfg.Listeners {
			opts := c.Options["options"]
			if opts != nil {
				optOpts := opts.(map[interface{}]interface{})
				o := optOpts["advertise"]
				if o != nil {
					// should start with tls:
					addy := strings.TrimPrefix(o.(string), "tls:")
					addy = strings.Split(addy, ":")[0]
					e := cfg.Id.ValidFor(addy)
					if e != nil {
						errs = append(errs, fmt.Errorf("invalid listeners.binding.advertise: %s, error: %v", o.(string), e))
					}
				}
			}
		}

		if len(errs) > 0 {
			pfxlog.Logger().Fatalf("one or more advertise addresses are invalid: %v", errs)
		}
	}
	return cfg, nil
}

func (c *Config) loadCtrlRateLimiterConfig(cfgmap map[interface{}]interface{}) error {
	rateLimitConfig := &c.Ctrl.RateLimit
	rateLimitConfig.SetDefaults()
	rateLimitConfig.MinSize = 1
	rateLimitConfig.QueueSizeMetric = CtrlRateLimiterMetricOutstandingCount
	rateLimitConfig.WindowSizeMetric = CtrlRateLimiterMetricCurrentWindowSize
	rateLimitConfig.WorkTimerMetric = CtrlRateLimiterMetricWorkTimer

	if value, found := cfgmap["rateLimiter"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if err := command.LoadAdaptiveRateLimiterConfig(rateLimitConfig, submap); err != nil {
				return err
			}
			if rateLimitConfig.MaxSize < CtrlRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for ctrl.rateLimiter.maxSize, must be at least %v",
					rateLimitConfig.MaxSize, CtrlRateLimiterMinSizeValue)
			}
			if rateLimitConfig.MaxSize > CtrlRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for ctrl.rateLimiter.maxSize, must be at most %v",
					rateLimitConfig.MaxSize, CtrlRateLimiterMaxSizeValue)
			}

			if rateLimitConfig.MinSize < CtrlRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for ctrl.rateLimiter.minSize, must be at least %v",
					rateLimitConfig.MinSize, CtrlRateLimiterMinSizeValue)
			}
			if rateLimitConfig.MinSize > CtrlRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for ctrl.rateLimiter.minSize, must be at most %v",
					rateLimitConfig.MinSize, CtrlRateLimiterMaxSizeValue)
			}
		} else {
			return errors.Errorf("invalid type for ctrl.rateLimiter, should be map instead of %T", value)
		}
	}

	return nil
}

func (c *Config) SaveControllerEndpoints(endpoints []string) error {
	endpointsFile := c.Ctrl.EndpointsFile

	configData := map[string]interface{}{
		"endpoints": endpoints,
	}

	if data, err := yaml.Marshal(configData); err != nil {
		return fmt.Errorf("unable to marshal updated controller endpoints to yaml (%w)", err)
	} else if err = os.WriteFile(endpointsFile, data, 0600); err != nil {
		return fmt.Errorf("unable to write updated controller endpoints to file '%s' (%w)", c.Ctrl.EndpointsFile, err)
	}
	return nil
}

func LoadIdentityConfigFromMap(cfgmap map[interface{}]interface{}) (*identity.Config, error) {
	if value, found := cfgmap["identity"]; found {
		subMap := value.(map[interface{}]interface{})
		return identity.NewConfigFromMapWithPathContext(subMap, "identity")
	}

	return nil, fmt.Errorf("identity section not found")
}

type ListenerBinding struct {
	Name    string
	Options xgress.OptionsData
}
