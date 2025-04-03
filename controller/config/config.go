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

package config

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/transport/v2"
	transporttls "github.com/openziti/transport/v2/tls"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/config"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/router/xgress"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"math"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultProfileMemoryInterval             = 15 * time.Second
	DefaultHealthChecksBoltCheckInterval     = 30 * time.Second
	DefaultHealthChecksBoltCheckTimeout      = 20 * time.Second
	DefaultHealthChecksBoltCheckInitialDelay = 30 * time.Second

	DefaultRaftCommandHandlerMaxQueueSize = 250

	// DefaultTlsHandshakeRateLimiterEnabled is whether the tls handshake rate limiter is enabled by default
	DefaultTlsHandshakeRateLimiterEnabled = false

	// TlsHandshakeRateLimiterMinSizeValue is the minimum size that can be configured for the tls handshake rate limiter
	// window range
	TlsHandshakeRateLimiterMinSizeValue = 5

	// TlsHandshakeRateLimiterMaxSizeValue is the maximum size that can be configured for the tls handshake rate limiter
	// window range
	TlsHandshakeRateLimiterMaxSizeValue = 10000

	// TlsHandshakeRateLimiterMetricOutstandingCount is the name of the metric tracking how many tasks are in process
	TlsHandshakeRateLimiterMetricOutstandingCount = "tls_handshake_limiter.in_process"

	// TlsHandshakeRateLimiterMetricCurrentWindowSize is the name of the metric tracking the current window size
	TlsHandshakeRateLimiterMetricCurrentWindowSize = "tls_handshake_limiter.window_size"

	// TlsHandshakeRateLimiterMetricWorkTimer is the name of the metric tracking how long successful tasks are taking to complete
	TlsHandshakeRateLimiterMetricWorkTimer = "tls_handshake_limiter.work_timer"

	// DefaultTlsHandshakeRateLimiterMaxWindow is the default max size for the tls handshake rate limiter
	DefaultTlsHandshakeRateLimiterMaxWindow = 1000

	DefaultRouterDataModelEnabled            = true
	DefaultRouterDataModelLogSize            = 10_000
	DefaultRouterDataModelListenerBufferSize = 1000

	DefaultRaftSnapshotInterval  = 2 * time.Minute
	DefaultRaftSnapshotThreshold = 500
	DefaultRaftTrailingLogs      = 500
)

type Config struct {
	Id                     *identity.TokenId
	SpiffeIdTrustDomain    *url.URL
	AdditionalTrustDomains []*url.URL

	Raft    *RaftConfig
	Network *NetworkConfig
	Edge    *EdgeConfig
	Db      boltz.Db
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
		Options  *CtrlOptions
	}
	HealthChecks struct {
		BoltCheck struct {
			Interval     time.Duration
			Timeout      time.Duration
			InitialDelay time.Duration
		}
	}
	RouterDataModel         common.RouterDataModelConfig
	CommandRateLimiter      command.RateLimiterConfig
	TlsHandshakeRateLimiter command.AdaptiveRateLimiterConfig
	Src                     map[interface{}]interface{}
}

func (self *Config) ToJson() (string, error) {
	jsonMap, err := config.ToJsonCompatibleMap(self.Src)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(jsonMap)
	return string(b), err
}

func (self *Config) IsRaftEnabled() bool {
	return self.Raft != nil
}

// CtrlOptions extends channel.Options to include support for additional, non-channel specific options
// (e.g. NewListener)
type CtrlOptions struct {
	*channel.Options
	NewListener            *transport.Address
	AdvertiseAddress       *transport.Address
	RouterHeartbeatOptions *channel.HeartbeatOptions
	PeerHeartbeatOptions   *channel.HeartbeatOptions
}

func (config *Config) Configure(sub config.Subconfig) error {
	return sub.LoadConfig(config.Src)
}

func LoadConfig(path string) (*Config, error) {
	cfgBytes, err := os.ReadFile(path)
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

		if identityConfig.ServerCert == "" && identityConfig.ServerKey == "" {
			identityConfig.ServerCert = identityConfig.Cert
			identityConfig.ServerKey = identityConfig.Key
		}
	} else {
		return nil, fmt.Errorf("identity section not found")
	}

	controllerConfig := &Config{
		Network: DefaultNetworkConfig(),
		Src:     cfgmap,
	}

	if id, err := identity.LoadIdentity(*identityConfig); err != nil {
		return nil, fmt.Errorf("unable to load identity (%s)", err)
	} else {
		controllerConfig.Id = identity.NewIdentity(id)

		if err := controllerConfig.Id.WatchFiles(); err != nil {
			pfxlog.Logger().Warn("could not enable file watching on identity: %w", err)
		}
	}

	if value, found := cfgmap["network"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if options, err := LoadNetworkConfig(submap); err == nil {
				controllerConfig.Network = options
			} else {
				return nil, fmt.Errorf("invalid 'network' stanza (%s)", err)
			}
		} else {
			pfxlog.Logger().Warn("invalid or empty 'network' stanza")
		}
	}

	if value, found := cfgmap["cluster"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			controllerConfig.Raft = &RaftConfig{}

			controllerConfig.Raft.ElectionTimeout = 5 * time.Second
			controllerConfig.Raft.HeartbeatTimeout = 3 * time.Second
			controllerConfig.Raft.LeaderLeaseTimeout = 3 * time.Second
			controllerConfig.Raft.CommandHandlerOptions.MaxQueueSize = DefaultRaftCommandHandlerMaxQueueSize
			controllerConfig.Raft.WarnWhenLeaderlessFor = time.Minute

			controllerConfig.Raft.SnapshotInterval = DefaultRaftSnapshotInterval
			controllerConfig.Raft.SnapshotThreshold = DefaultRaftSnapshotThreshold
			controllerConfig.Raft.TrailingLogs = DefaultRaftTrailingLogs

			if value, found := submap["dataDir"]; found {
				controllerConfig.Raft.DataDir = value.(string)
			} else {
				return nil, errors.Errorf("cluster dataDir configuration missing")
			}

			if value, found := submap["snapshotInterval"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.SnapshotInterval = val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.snapshotInterval value '%v", value)
				}
			}

			if value, found := submap["commitTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.CommitTimeout = &val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.commitTimeout value '%v", value)
				}
			}

			if value, found := submap["electionTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.ElectionTimeout = val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.electionTimeout value '%v", value)
				}
			}

			if value, found := submap["heartbeatTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.HeartbeatTimeout = val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.heartbeatTimeout value '%v", value)
				}
			}

			if value, found := submap["leaderLeaseTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.LeaderLeaseTimeout = val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.leaderLeaseTimeout value '%v", value)
				}
			}

			if value, found := submap["snapshotThreshold"]; found {
				val := uint32(value.(int))
				controllerConfig.Raft.SnapshotThreshold = val
			}

			if value, found := submap["maxAppendEntries"]; found {
				val := uint32(value.(int))
				controllerConfig.Raft.MaxAppendEntries = &val
			}

			if value, found := submap["trailingLogs"]; found {
				val := uint32(value.(int))
				controllerConfig.Raft.TrailingLogs = val
			}

			if value, found := submap["logLevel"]; found {
				val := fmt.Sprintf("%v", value)
				if hclog.LevelFromString(val) == hclog.NoLevel {
					return nil, errors.Errorf("invalid value for raft.logLevel [%v]", val)
				}
				controllerConfig.Raft.LogLevel = &val
			}

			if value, found := submap["warnWhenLeaderlessFor"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					if val < 10*time.Second {
						pfxlog.Logger().Infof("invalid value %s for raft.warnWhenLeaderlessFor, must be >= 10s", val)
					} else {
						controllerConfig.Raft.WarnWhenLeaderlessFor = val
					}
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.warnWhenLeaderlessFor value '%v", value)
				}
			}

			if value, found := submap["logFile"]; found {
				val := fmt.Sprintf("%v", value)
				options := *hclog.DefaultOptions
				f, err := os.Create(val)
				if err != nil {
					return nil, errors.Wrapf(err, "unable to open raft log file [%v]", val)
				}
				options.Output = f
				if controllerConfig.Raft.LogLevel != nil {
					options.Level = hclog.LevelFromString(*controllerConfig.Raft.LogLevel)
				}
				controllerConfig.Raft.Logger = hclog.New(&options)
			}

			if value, found := cfgmap["commandHandler"]; found {
				if chSubMap, ok := value.(map[interface{}]interface{}); ok {
					if value, found := chSubMap["maxQueueSize"]; found {
						controllerConfig.Raft.CommandHandlerOptions.MaxQueueSize = uint16(value.(int))
					}
				} else {
					return nil, errors.New("invalid commandHandler value, should be map")
				}
			}
		} else {
			return nil, errors.Errorf("invalid cluster configuration")
		}
	} else if value, found := cfgmap["db"]; found {
		str, err := db.Open(value.(string))
		if err != nil {
			return nil, err
		}
		controllerConfig.Db = str
	} else {
		panic("controllerConfig must provide [db] or [cluster]")
	}

	//SPIFFE Trust Domain
	var spiffeId *url.URL
	if controllerConfig.Raft != nil {
		//HA setup, SPIFFE ID must come from certs
		var err error
		spiffeId, err = GetSpiffeIdFromIdentity(controllerConfig.Id.Identity)
		if err != nil {
			panic("error determining a trust domain from a SPIFFE id in the root identity for HA configuration, must have a spiffe:// URI SANs in the server certificate or along the signing CAs chain: " + err.Error())
		}

		if spiffeId == nil {
			panic("unable to determine a trust domain from a SPIFFE id in the root identity for HA configuration, must have a spiffe:// URI SANs in the server certificate or along the signing CAs chain")
		}
	} else {
		// Non-HA/legacy system, prefer SPIFFE id from certs, but fall back to configuration if necessary
		spiffeId, _ = GetSpiffeIdFromIdentity(controllerConfig.Id.Identity)

		if spiffeId == nil {
			//for non HA setups allow the trust domain to come from the configuration root value `trustDomain`
			if value, found := cfgmap["trustDomain"]; found {
				trustDomain, ok := value.(string)

				if !ok {
					panic(fmt.Sprintf("could not parse [trustDomain], expected a string got [%T]", value))
				}

				if trustDomain != "" {
					if !strings.HasPrefix("spiffe://", trustDomain) {
						trustDomain = "spiffe://" + trustDomain
					}

					spiffeId, err = url.Parse(trustDomain)

					if err != nil {
						panic("could not parse [trustDomain] when used in a SPIFFE id URI [" + trustDomain + "], please make sure it is a valid URI hostname: " + err.Error())
					}

					if spiffeId == nil {
						panic("could not parse [trustDomain] when used in a SPIFFE id URI [" + trustDomain + "]: spiffeId is nil and no error returned")
					}

					if spiffeId.Scheme != "spiffe" {
						panic("[trustDomain] does not have a spiffe scheme (spiffe://) has: " + spiffeId.Scheme)
					}
				}
			}
		}

		//default a generated trust domain and spiffe id from the sha1 of the root ca
		if spiffeId == nil {
			spiffeId, err = generateDefaultSpiffeId(controllerConfig.Id.Identity)

			if err != nil {
				panic("could not generate default trust domain: " + err.Error())
			}

			pfxlog.Logger().Warnf("this environment is using a default generated trust domain [%s], it is recommended that a trust domain is specified in configuration via URI SANs or the 'trustDomain' field", spiffeId.String())
			pfxlog.Logger().Warnf("this environment is using a default generated trust domain [%s], it is recommended that if network components have enrolled that the generated trust domain be added to the configuration field 'additionalTrustDomains' array when configuring a explicit trust domain", spiffeId.String())
		}
	}

	if spiffeId == nil {
		panic("unable to determine trust domain from SPIFFE id (spiffe:// URI SANs in server cert or signing CAs) or from configuration [trustDomain], controllers must have a trust domain")
	}

	if spiffeId.Hostname() == "" {
		panic("unable to determine trust domain from SPIFFE id: hostname was empty")
	}

	if controllerConfig.Raft != nil {
		if err = ValidateSpiffeId(controllerConfig.Id, spiffeId); err != nil {
			panic(err)
		}
	}

	//only preserve trust domain
	spiffeId.Path = ""
	controllerConfig.SpiffeIdTrustDomain = spiffeId

	if value, found := cfgmap["additionalTrustDomains"]; found {
		if valArr, ok := value.([]any); ok {
			var trustDomains []*url.URL
			for _, trustDomain := range valArr {
				if strTrustDomain, ok := trustDomain.(string); ok {

					if !strings.HasPrefix("spiffe://", strTrustDomain) {
						strTrustDomain = "spiffe://" + strTrustDomain
					}

					spiffeId, err = url.Parse(strTrustDomain)

					if err != nil {
						panic(fmt.Sprintf("invalid entry in 'additionalTrustDomains', could not be parsed as a URI: %v", trustDomain))
					}
					//only preserve trust domain
					spiffeId.Path = ""

					trustDomains = append(trustDomains, spiffeId)
				} else {
					panic(fmt.Sprintf("invalid entry in 'additionalTrustDomains' expected a string: %v", trustDomain))
				}
			}

			controllerConfig.AdditionalTrustDomains = trustDomains
		}
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
						controllerConfig.Profile.Memory.Interval = DefaultProfileMemoryInterval
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

			controllerConfig.Ctrl.Options = &CtrlOptions{
				Options:                channel.DefaultOptions(),
				PeerHeartbeatOptions:   channel.DefaultHeartbeatOptions(),
				RouterHeartbeatOptions: channel.DefaultHeartbeatOptions(),
			}

			if value, found := submap["options"]; found {
				if submap, ok := value.(map[interface{}]interface{}); ok {
					options, err := channel.LoadOptions(submap)
					if err != nil {
						return nil, err
					}

					controllerConfig.Ctrl.Options.Options = options

					if val, found := submap["newListener"]; found {
						if newListener, ok := val.(string); ok {
							if newListener != "" {
								if addr, err := transport.ParseAddress(newListener); err == nil {
									controllerConfig.Ctrl.Options.NewListener = &addr

									if err := verifyNewListenerInServerCert(controllerConfig, addr); err != nil {
										return nil, err
									}

								} else {
									return nil, fmt.Errorf("error loading newListener for [ctrl/options] (%v)", err)
								}
							}
						} else {
							return nil, errors.New("error loading newAddress for [ctrl/options] (must be a string)")
						}
					}

					if val, found := submap["advertiseAddress"]; found {
						if advertiseAddr, ok := val.(string); ok {
							if advertiseAddr != "" {
								addr, err := transport.ParseAddress(advertiseAddr)
								if err != nil {
									return nil, errors.Wrapf(err, "error parsing value '%v' for [ctrl/options/advertiseAddress]", advertiseAddr)
								}
								controllerConfig.Ctrl.Options.AdvertiseAddress = &addr
								if controllerConfig.Raft != nil {
									controllerConfig.Raft.AdvertiseAddress = addr
								}
							}
						} else {
							return nil, errors.New("error loading advertiseAddress for [ctrl/options] (must be a string)")
						}
					}

					if value, found := submap["routerHeartbeats"]; found {
						if submap, ok := value.(map[interface{}]interface{}); ok {
							options, err := channel.LoadHeartbeatOptions(submap)
							if err != nil {
								return nil, err
							}
							controllerConfig.Ctrl.Options.RouterHeartbeatOptions = options
						}
					}

					if value, found := submap["peerHeartbeats"]; found {
						if submap, ok := value.(map[interface{}]interface{}); ok {
							options, err := channel.LoadHeartbeatOptions(submap)
							if err != nil {
								return nil, err
							}
							controllerConfig.Ctrl.Options.PeerHeartbeatOptions = options
						}
					}

					if err := controllerConfig.Ctrl.Options.Validate(); err != nil {
						return nil, fmt.Errorf("error loading channel options for [ctrl/options] (%v)", err)
					}
				}
				if value != nil {
					m := value.(map[interface{}]interface{})
					a := strings.TrimPrefix(m["advertiseAddress"].(string), "tls:")
					v := controllerConfig.Id.ValidFor(strings.Split(a, ":")[0])
					if v != nil {
						pfxlog.Logger().Fatalf("provided value for ctrl/options/advertiseAddress is invalid (%v)", v)
					}
				}
			}
			if controllerConfig.Raft != nil && controllerConfig.Raft.AdvertiseAddress == nil {
				return nil, errors.New("[ctrl/options/advertiseAddress] is required when raft is enabled")
			}
		} else {
			panic("controllerConfig [ctrl] section in unexpected format")
		}
	} else {
		panic("controllerConfig must provide [ctrl]")
	}

	controllerConfig.HealthChecks.BoltCheck.Interval = DefaultHealthChecksBoltCheckInterval
	controllerConfig.HealthChecks.BoltCheck.Timeout = DefaultHealthChecksBoltCheckTimeout
	controllerConfig.HealthChecks.BoltCheck.InitialDelay = DefaultHealthChecksBoltCheckInitialDelay

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

	controllerConfig.CommandRateLimiter.Enabled = true
	controllerConfig.CommandRateLimiter.QueueSize = command.DefaultLimiterSize

	if value, found := cfgmap["commandRateLimiter"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["enabled"]; found {
				controllerConfig.CommandRateLimiter.Enabled = strings.EqualFold("true", fmt.Sprintf("%v", value))
			}

			if value, found := submap["maxQueued"]; found {
				if intVal, ok := value.(int); ok {
					v := int64(intVal)
					if v < command.MinLimiterSize {
						return nil, errors.Errorf("invalid value %v for commandRateLimiter, must be at least %v", value, command.MinLimiterSize)
					}
					if v > math.MaxUint32 {
						return nil, errors.Errorf("invalid value %v for commandRateLimiter, must be at most %v", value, int64(math.MaxUint32))
					}
					controllerConfig.CommandRateLimiter.QueueSize = uint32(v)
				} else {
					return nil, errors.Errorf("invalid value %v for commandRateLimiter, must be integer value", value)
				}
			}
		}
	}

	controllerConfig.TlsHandshakeRateLimiter.SetDefaults()
	controllerConfig.TlsHandshakeRateLimiter.Enabled = DefaultTlsHandshakeRateLimiterEnabled
	controllerConfig.TlsHandshakeRateLimiter.MaxSize = DefaultTlsHandshakeRateLimiterMaxWindow
	controllerConfig.TlsHandshakeRateLimiter.QueueSizeMetric = TlsHandshakeRateLimiterMetricOutstandingCount
	controllerConfig.TlsHandshakeRateLimiter.WindowSizeMetric = TlsHandshakeRateLimiterMetricCurrentWindowSize
	controllerConfig.TlsHandshakeRateLimiter.WorkTimerMetric = TlsHandshakeRateLimiterMetricWorkTimer

	if value, found := cfgmap["tls"]; found {
		if tlsMap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := tlsMap["handshakeTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					transporttls.SetSharedListenerHandshakeTimeout(val)
				} else {
					return nil, errors.Wrapf(err, "failed to parse tls.handshakeTimeout value '%v", value)
				}
			}
			if err = loadTlsHandshakeRateLimiterConfig(&controllerConfig.TlsHandshakeRateLimiter, tlsMap); err != nil {
				return nil, err
			}
		}
	}

	controllerConfig.RouterDataModel.Enabled = DefaultRouterDataModelEnabled
	controllerConfig.RouterDataModel.LogSize = DefaultRouterDataModelLogSize
	controllerConfig.RouterDataModel.ListenerBufferSize = DefaultRouterDataModelListenerBufferSize

	if value, found := cfgmap["routerDataModel"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["enabled"]; found {
				controllerConfig.RouterDataModel.Enabled = strings.EqualFold("true", fmt.Sprintf("%v", value))
			}

			if value, found := submap["logSize"]; found {
				if val, ok := value.(int); ok {
					if val < 0 {
						return nil, errors.Wrapf(err, "failed to parse routerDataModel.logSize, must be >= 0 %v", value)
					}
					controllerConfig.RouterDataModel.LogSize = uint64(val)
				} else {
					return nil, errors.Wrapf(err, "failed to parse routerDataModel.logSize, should be int not value %T", value)
				}
			}

			if value, found := submap["listenerBufferSize"]; found {
				if val, ok := value.(int); ok {
					if val < 0 {
						return nil, errors.Wrapf(err, "failed to parse routerDataModel.listenerBufferSize, must be >= 0 %v", value)
					}
					controllerConfig.RouterDataModel.ListenerBufferSize = uint(val)
				} else {
					return nil, errors.Wrapf(err, "failed to parse routerDataModel.listenerBufferSize, should be int not value %T", value)
				}
			}
		} else {
			return nil, errors.Errorf("invalid raft configuration")
		}
	}

	edgeConfig, err := LoadEdgeConfigFromMap(cfgmap)
	if err != nil {
		return nil, err
	}
	controllerConfig.Edge = edgeConfig

	return controllerConfig, nil
}

// isSelfSigned checks if the given certificate is self-signed.
func isSelfSigned(cert *x509.Certificate) (bool, error) {
	// Check if the Issuer and Subject fields are equal
	if cert.Issuer.String() != cert.Subject.String() {
		return false, nil
	}

	// Attempt to verify the certificate's signature with its own public key
	err := cert.CheckSignatureFrom(cert)
	if err != nil {
		return false, err
	}

	return true, nil
}

func generateDefaultSpiffeId(id identity.Identity) (*url.URL, error) {
	rawCerts := id.Cert().Certificate
	certs := make([]*x509.Certificate, len(rawCerts))

	for i := range rawCerts {
		cert, err := x509.ParseCertificate(rawCerts[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate at index [%d]: %w", i, err)
		}
		certs[i] = cert
	}

	chain := id.CaPool().GetChain(id.Cert().Leaf, certs...)

	// chain is 0 or 1, no root possible
	if len(chain) <= 1 {
		return nil, fmt.Errorf("error generating default trust domain from root CA: no root CA detected after chain assembly from the root identity server cert and ca bundle")
	}

	candidateRoot := chain[len(chain)-1]

	if candidateRoot == nil {
		return nil, fmt.Errorf("encountered nil candidate root ca during default trust domain generation")
	}

	if !candidateRoot.IsCA {
		return nil, fmt.Errorf("candidate root CA is not flagged with the x509 CA flag")
	}

	if selfSigned, _ := isSelfSigned(candidateRoot); !selfSigned {
		return nil, errors.New("candidate root CA is not self signed")
	}

	rawHash := sha1.Sum(candidateRoot.Raw)

	fingerprint := fmt.Sprintf("%x", rawHash)
	idStr := "spiffe://" + fingerprint

	spiffeId, err := url.Parse(idStr)

	if err != nil {
		return nil, fmt.Errorf("could not parse generated SPIFFE id [%s] as a URI: %w", idStr, err)
	}

	return spiffeId, nil
}

// GetSpiffeIdFromIdentity will search an Identity for a trust domain encoded as a spiffe:// URI SAN starting
// from the server cert and up its signing chain. Each certificate must contain 0 or 1 spiffe:// URI SAN. The first
// SPIFFE id looking up the chain back to the root CA is returned. If no SPIFFE id is encountered, nil is returned.
// Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromIdentity(id identity.Identity) (*url.URL, error) {
	tlsCerts := id.ServerCert()

	spiffeId, err := GetSpiffeIdFromTlsCertChain(tlsCerts)

	if err != nil {
		return nil, fmt.Errorf("failed to acquire SPIFFE id from server certs: %w", err)
	}

	if spiffeId != nil {
		return spiffeId, nil
	}

	if len(tlsCerts) > 0 {
		chain := id.CaPool().GetChain(tlsCerts[0].Leaf)

		if len(chain) > 0 {
			spiffeId, _ = GetSpiffeIdFromCertChain(chain)
		}
	}

	if spiffeId == nil {
		return nil, errors.Errorf("SPIFFE id not found in identity")
	}

	return spiffeId, nil
}

func ValidateSpiffeId(id *identity.TokenId, spiffeId *url.URL) error {
	if !strings.HasPrefix(spiffeId.Path, "/controller/") {
		return fmt.Errorf("invalid SPIFFE id path: %s, should have /controller/ prefix", spiffeId.Path)
	}
	idInSpiffeId := strings.TrimPrefix(spiffeId.Path, "/controller/")
	if idInSpiffeId != id.Token {
		return fmt.Errorf("spiffe id '%s', does not match subject identifier '%s'", id.Token, idInSpiffeId)
	}
	return nil
}

// GetSpiffeIdFromCertChain cycles through a slice of certificates that goes from leaf up CAs. Each certificate
// must contain 0 or 1 spiffe:// URI SAN. The first encountered SPIFFE id looking up the chain back to the root CA is returned.
// If no SPIFFE id is encountered, nil is returned. Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromCertChain(certs []*x509.Certificate) (*url.URL, error) {
	var spiffeId *url.URL
	for _, cert := range certs {
		var err error
		spiffeId, err = GetSpiffeIdFromCert(cert)

		if err != nil {
			return nil, fmt.Errorf("failed to determine SPIFFE ID from x509 certificate chain: %w", err)
		}

		if spiffeId != nil {
			return spiffeId, nil
		}
	}

	return nil, errors.New("failed to determine SPIFFE ID, no spiffe:// URI SANs found in x509 certificate chain")
}

// GetSpiffeIdFromTlsCertChain will search a tls certificate chain for a trust domain encoded as a spiffe:// URI SAN.
// Each certificate must contain 0 or 1 spiffe:// URI SAN. The first SPIFFE id looking up the chain is returned. If
// no SPIFFE id is encountered, nil is returned. Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromTlsCertChain(tlsCerts []*tls.Certificate) (*url.URL, error) {
	for _, tlsCert := range tlsCerts {
		for i, rawCert := range tlsCert.Certificate {
			cert, err := x509.ParseCertificate(rawCert)

			if err != nil {
				return nil, fmt.Errorf("failed to parse TLS cert at index [%d]: %w", i, err)
			}

			spiffeId, err := GetSpiffeIdFromCert(cert)

			if err != nil {
				return nil, fmt.Errorf("failed to determine SPIFFE ID from TLS cert at index [%d]: %w", i, err)
			}

			if spiffeId != nil {
				return spiffeId, nil
			}
		}
	}

	return nil, nil
}

// GetSpiffeIdFromCert will search a x509 certificate for a trust domain encoded as a spiffe:// URI SAN.
// Each certificate must contain 0 or 1 spiffe:// URI SAN. The first SPIFFE id looking up the chain is returned. If
// no SPIFFE id is encountered, nil is returned. Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromCert(cert *x509.Certificate) (*url.URL, error) {
	var spiffeId *url.URL
	for _, uriSan := range cert.URIs {
		if uriSan.Scheme == "spiffe" {
			if spiffeId != nil {
				return nil, fmt.Errorf("multiple URI SAN spiffe:// ids encountered, must only have one, encountered at least two: [%s] and [%s]", spiffeId.String(), uriSan.String())
			}
			spiffeId = uriSan
		}
	}

	return spiffeId, nil
}

func loadTlsHandshakeRateLimiterConfig(rateLimitConfig *command.AdaptiveRateLimiterConfig, cfgmap map[interface{}]interface{}) error {
	if value, found := cfgmap["rateLimiter"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if err := command.LoadAdaptiveRateLimiterConfig(rateLimitConfig, submap); err != nil {
				return err
			}
			if rateLimitConfig.MaxSize < TlsHandshakeRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for tls.rateLimiter.maxSize, must be at least %v",
					rateLimitConfig.MaxSize, TlsHandshakeRateLimiterMinSizeValue)
			}
			if rateLimitConfig.MaxSize > TlsHandshakeRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for tls.rateLimiter.maxSize, must be at most %v",
					rateLimitConfig.MaxSize, TlsHandshakeRateLimiterMaxSizeValue)
			}

			if rateLimitConfig.MinSize < TlsHandshakeRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for tls.rateLimiter.minSize, must be at least %v",
					rateLimitConfig.MinSize, TlsHandshakeRateLimiterMinSizeValue)
			}
			if rateLimitConfig.MinSize > TlsHandshakeRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for tls.rateLimiter.minSize, must be at most %v",
					rateLimitConfig.MinSize, TlsHandshakeRateLimiterMaxSizeValue)
			}
		} else {
			return errors.Errorf("invalid type for tls.rateLimiter, should be map instead of %T", value)
		}
	}

	return nil
}

// verifyNewListenerInServerCert verifies that the hostname (ip/dns) for addr is present as an IP/DNS SAN in the first
// certificate provided in the controller's identity server certificates. This is to avoid scenarios where
// newListener propagated to routers who will never be able to verify the controller's certificates due to SAN issues.
func verifyNewListenerInServerCert(controllerConfig *Config, addr transport.Address) error {
	addrSplits := strings.Split(addr.String(), ":")
	if len(addrSplits) < 3 {
		return errors.New("could not determine newListener's host value, expected at least three segments")
	}

	host := addrSplits[1]

	serverCerts := controllerConfig.Id.Identity.ServerCert()

	if len(serverCerts) == 0 {
		return errors.New("could not verify newListener value, server certificate for identity contains no certificates")
	}

	hostFound := false
	for _, serverCert := range serverCerts {
		for _, dnsName := range serverCert.Leaf.DNSNames {
			if dnsName == host {
				hostFound = true
				break
			}
		}

		if hostFound {
			break
		}

		if !hostFound {
			for _, ipAddresses := range serverCert.Leaf.IPAddresses {
				if host == ipAddresses.String() {
					hostFound = true
					break
				}
			}
		}

		if hostFound {
			break
		}
	}

	if !hostFound {
		return fmt.Errorf("could not find newListener [%s] host value [%s] in first certificate for controller identity", addr.String(), host)
	}

	return nil
}

type CertValidatingIdentity struct {
	identity.Identity
}

func (self *CertValidatingIdentity) ClientTLSConfig() *tls.Config {
	cfg := self.Identity.ClientTLSConfig()
	cfg.VerifyConnection = self.VerifyConnection
	return cfg
}

func (self *CertValidatingIdentity) ServerTLSConfig() *tls.Config {
	cfg := self.Identity.ServerTLSConfig()
	cfg.VerifyConnection = self.VerifyConnection
	return cfg
}

func (self *CertValidatingIdentity) VerifyConnection(state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("no peer certificates provided")
	}
	log := pfxlog.Logger()
	for _, cert := range state.PeerCertificates {
		log.Infof("cert provided: CN: %v IsCA: %v", cert.Subject.CommonName, cert.IsCA)
	}

	options := x509.VerifyOptions{
		Roots:         self.Identity.CA(),
		Intermediates: x509.NewCertPool(),
	}

	for _, cert := range state.PeerCertificates[1:] {
		options.Intermediates.AddCert(cert)
	}

	result, err := state.PeerCertificates[0].Verify(options)

	if err != nil {
		pfxlog.Logger().WithError(err).Error("got error validating cert")
		return err
	}

	log.Infof("got result: %v", result)
	return nil
}
