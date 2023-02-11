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

package controller

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/config"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/raft"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/identity"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"os"
	"strings"
	"time"
)

const (
	DefaultProfileMemoryInterval             = 15 * time.Second
	DefaultHealthChecksBoltCheckInterval     = 30 * time.Second
	DefaultHealthChecksBoltCheckTimeout      = 20 * time.Second
	DefaultHealthChecksBoltCheckInitialDelay = 30 * time.Second

	DefaultRaftCommandHandlerMaxQueueSize = 1000
	DefaultRaftCommandHandlerMaxWorkers   = 10
)

type Config struct {
	Id      *identity.TokenId
	Raft    *raft.Config
	Network *network.Options
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
	src map[interface{}]interface{}
}

// CtrlOptions extends channel.Options to include support for additional, non-channel specific options
// (e.g. NewListener)
type CtrlOptions struct {
	*channel.Options
	NewListener      *transport.Address
	AdvertiseAddress *transport.Address
}

func (config *Config) Configure(sub config.Subconfig) error {
	return sub.LoadConfig(config.src)
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

	if value, found := cfgmap["raft"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			controllerConfig.Raft = &raft.Config{}
			controllerConfig.Raft.CommandHandlerOptions.MaxQueueSize = DefaultRaftCommandHandlerMaxQueueSize
			controllerConfig.Raft.CommandHandlerOptions.MaxWorkers = DefaultRaftCommandHandlerMaxWorkers

			if value, found := submap["dataDir"]; found {
				controllerConfig.Raft.DataDir = value.(string)
			} else {
				return nil, errors.Errorf("raft dataDir configuration missing")
			}
			if value, found := submap["minClusterSize"]; found {
				controllerConfig.Raft.MinClusterSize = uint32(value.(int))
			}
			if value, found := submap["bootstrapMembers"]; found {
				if lst, ok := value.([]interface{}); ok {
					for idx, val := range lst {
						if member, ok := val.(string); ok {
							controllerConfig.Raft.BootstrapMembers = append(controllerConfig.Raft.BootstrapMembers, member)
						} else {
							return nil, errors.Errorf("invalid bootstrapMembers value '%v'at index %v, should be array", idx, val)
						}
					}
				} else {
					return nil, errors.New("invalid bootstrapMembers value, should be array")
				}
			}

			if value, found := submap["snapshotInterval"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.SnapshotInterval = &val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.snapshotInterval value '%v", value)
				}
			}

			if value, found := submap["heartbeatTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.HeartbeatTimeout = &val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.heartbeatTimeout value '%v", value)
				}
			}

			if value, found := submap["leaderLeaseTimeout"]; found {
				if val, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
					controllerConfig.Raft.LeaderLeaseTimeout = &val
				} else {
					return nil, errors.Wrapf(err, "failed to parse raft.leaderLeaseTimeout value '%v", value)
				}
			}

			if value, found := submap["snapshotThreshold"]; found {
				val := uint32(value.(int))
				controllerConfig.Raft.SnapshotThreshold = &val
			}

			if value, found := submap["maxAppendEntries"]; found {
				val := uint32(value.(int))
				controllerConfig.Raft.MaxAppendEntries = &val
			}

			if value, found := submap["trailingLogs"]; found {
				val := uint32(value.(int))
				controllerConfig.Raft.TrailingLogs = &val
			}

			if value, found := submap["logLevel"]; found {
				val := fmt.Sprintf("%v", value)
				if hclog.LevelFromString(val) == hclog.NoLevel {
					return nil, errors.Errorf("invalid value for raft.logLevel [%v]", val)
				}
				controllerConfig.Raft.LogLevel = &val
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
					if value, found := chSubMap["maxWorkers"]; found {
						controllerConfig.Raft.CommandHandlerOptions.MaxWorkers = uint16(value.(int))
					}
				} else {
					return nil, errors.New("invalid commandHandler value, should be map")
				}
			}
		} else {
			return nil, errors.Errorf("invalid raft configuration")
		}
	} else if value, found := cfgmap["db"]; found {
		str, err := db.Open(value.(string))
		if err != nil {
			return nil, err
		}
		controllerConfig.Db = str
	} else {
		panic("controllerConfig must provide [db] or [raft]")
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
				Options: channel.DefaultOptions(),
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

					if err := controllerConfig.Ctrl.Options.Validate(); err != nil {
						return nil, fmt.Errorf("error loading channel options for [ctrl/options] (%v)", err)
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

	return controllerConfig, nil
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
