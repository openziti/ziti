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

package edgerouter

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

const (
	DefaultHeartbeatIntervalSeconds   = 60
	MinHeartbeatIntervalSeconds       = 10
	MaxHeartbeatIntervalSeconds       = 60
	DefaultSessionValidateChunkSize   = 1000
	DefaultSessionValidateMinInterval = "250ms"
	DefaultSessionValidateMaxInterval = "1500ms"

	FlagsCfgMapKey = "@flags"
)

type Config struct {
	Enabled                    bool
	ApiProxy                   ApiProxy
	EdgeListeners              []*edge_ctrl_pb.Listener
	Csr                        Csr
	HeartbeatIntervalSeconds   int
	SessionValidateChunkSize   uint32
	SessionValidateMinInterval time.Duration
	SessionValidateMaxInterval time.Duration
	Tcfg                       transport.Configuration
	ForceExtendEnrollment      bool

	RouterConfig             *router.Config
	EnrollmentIdentityConfig *identity.Config
}

type Csr struct {
	Sans               *Sans  `yaml:"sans"`
	Country            string `yaml:"country"`
	Locality           string `yaml:"locality"`
	Organization       string `yaml:"organization"`
	OrganizationalUnit string `yaml:"organizationalUnit"`
	Province           string `yaml:"province"`
}

type ApiProxy struct {
	Enabled  bool
	Listener string
	Upstream string
}

func NewConfig(routerConfig *router.Config) *Config {
	return &Config{
		RouterConfig: routerConfig,
	}
}

func (config *Config) LoadConfig(configMap map[interface{}]interface{}) error {
	//enrollment config loading is more lax on where the CSR section lives (i.e. under edge: or at the root level)

	if val, ok := configMap["edge"]; ok && val != nil {
		var edgeConfigMap map[interface{}]interface{}
		config.Enabled = true
		if edgeConfigMap, ok = val.(map[interface{}]interface{}); !ok {
			return fmt.Errorf("expected map as edge configuration")
		}

		if err := config.loadCsr(edgeConfigMap, "edge"); err != nil {
			return err
		}
	} else if val, ok := configMap["csr"]; ok && val != nil {
		if err := config.loadCsr(configMap, ""); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("expected enrollment CSR section")
	}

	return config.ensureIdentity(configMap)
}

func (config *Config) LoadConfigFromMap(configMap map[interface{}]interface{}) error {
	var err error
	config.Enabled = false

	if val, ok := configMap[FlagsCfgMapKey]; ok {
		if flags, ok := val.(map[string]*pflag.Flag); ok {
			if flag, ok := flags["extend"]; ok {
				config.ForceExtendEnrollment = flag.Value.String() == "true"
			}
		}
	}

	var edgeConfigMap map[interface{}]interface{} = nil

	if val, ok := configMap["edge"]; ok && val != nil {
		config.Enabled = true
		if edgeConfigMap, ok = val.(map[interface{}]interface{}); !ok {
			return fmt.Errorf("expected map as edge configuration")
		}
	}

	if err := config.ensureIdentity(configMap); err != nil {
		return err
	}

	if val, found := edgeConfigMap["heartbeatIntervalSeconds"]; found {
		config.HeartbeatIntervalSeconds = val.(int)
	}

	if config.HeartbeatIntervalSeconds > DefaultHeartbeatIntervalSeconds || config.HeartbeatIntervalSeconds < MinHeartbeatIntervalSeconds {
		pfxlog.Logger().Warnf("Invalid heartbeat interval [%v] (min: %v, max: %v), setting to default [%v]", config.HeartbeatIntervalSeconds, MaxHeartbeatIntervalSeconds, MinHeartbeatIntervalSeconds, DefaultHeartbeatIntervalSeconds)
		config.HeartbeatIntervalSeconds = DefaultHeartbeatIntervalSeconds
	}

	config.SessionValidateChunkSize = DefaultSessionValidateChunkSize
	if val, found := edgeConfigMap["sessionValidateChunkSize"]; found {
		config.SessionValidateChunkSize = uint32(val.(int))
	}

	sessionValidateMinInterval := DefaultSessionValidateMinInterval
	if val, found := edgeConfigMap["sessionValidateMinInterval"]; found {
		sessionValidateMinInterval = val.(string)
	}

	config.SessionValidateMinInterval, err = time.ParseDuration(sessionValidateMinInterval)
	if err != nil {
		return errors.Wrap(err, "invalid duration value for sessionValidateMinInterval")
	}

	sessionValidateMaxInterval := DefaultSessionValidateMaxInterval
	if val, found := edgeConfigMap["sessionValidateMaxInterval"]; found {
		sessionValidateMaxInterval = val.(string)
	}

	config.SessionValidateMaxInterval, err = time.ParseDuration(sessionValidateMaxInterval)
	if err != nil {
		return errors.Wrap(err, "invalid duration value for sessionValidateMaxInterval")
	}

	if err = config.loadApiProxy(edgeConfigMap); err != nil {
		return err
	}

	if err = config.loadCsr(edgeConfigMap, "edge"); err != nil {
		return err
	}

	if err = config.loadListener(configMap); err != nil {
		return err
	}

	if err = config.loadTransportConfig(configMap); err != nil {
		return err
	}

	return nil
}

func (config *Config) loadApiProxy(edgeConfigMap map[interface{}]interface{}) error {
	config.ApiProxy = ApiProxy{}

	if value, found := edgeConfigMap["apiProxy"]; found {
		submap := value.(map[interface{}]interface{})

		if submap == nil {
			config.ApiProxy.Enabled = false
			return nil
		}

		if value, found := submap["listener"]; found {
			config.ApiProxy.Listener = value.(string)

			if config.ApiProxy.Listener == "" {
				return errors.New("required value [edge.apiProxy.listener] is expected to be a string")
			}
		} else {
			return errors.New("required value [edge.apiProxy.listener] is missing")
		}

		if value, found := submap["upstream"]; found {
			config.ApiProxy.Upstream = value.(string)

			if config.ApiProxy.Upstream == "" {
				return errors.New("required value [edge.apiProxy.upstream] is expected to be a string")
			}
		} else {
			return errors.New("required value [edge.apiProxy.upstream] is missing")
		}

		config.ApiProxy.Enabled = true
	} else {
		config.ApiProxy.Enabled = false
	}

	return nil
}

func (config *Config) loadListener(rootConfigMap map[interface{}]interface{}) error {
	subArray := rootConfigMap["listeners"]

	listeners, ok := subArray.([]interface{})
	if !ok || listeners == nil {
		return errors.New("section [listeners] is required to be an array")
	}

	for i, value := range listeners {
		submap := value.(map[interface{}]interface{})

		if submap == nil {
			return fmt.Errorf("value [listeners[%d]] is not a map", i)
		}

		if value, found := submap["binding"]; found {
			binding := value.(string)

			if binding == common.EdgeBinding {

				if value, found := submap["address"]; found {
					address := value.(string)
					if address == "" {
						return fmt.Errorf("required value [listeners[%d].address] for edge binding was not a string or was not found", i)
					}
					_, err := transport.ParseAddress(address)
					if err != nil {
						return fmt.Errorf("required value [listeners[%d].address] for edge binding was not a valid address", i)
					}

					edgeListener, err := parseEdgeListenerOptions(i, address, submap)

					if err != nil {
						return fmt.Errorf("error parsing edge listener[%d]: %v", i, err)
					}

					config.EdgeListeners = append(config.EdgeListeners, edgeListener)

				} else {
					return errors.New("required value [listeners.edge.address] was not found")
				}
			}
		}
	}

	if len(config.EdgeListeners) == 0 {
		return errors.New("required binding [edge] not found in [listeners], at least one edge binding is required")
	}

	return nil
}

func parseEdgeListenerOptions(index int, address string, edgeListenerMap map[interface{}]interface{}) (*edge_ctrl_pb.Listener, error) {
	/*
	   address: ws:0.0.0.0:443
	   options:
	     advertise: mattermost-wss.production.netfoundry.io:443
	*/
	addressParts := strings.Split(address, ":")
	addressPort, err := strconv.Atoi(addressParts[2])

	if err != nil {
		return nil, fmt.Errorf("port number for [listeners[%d].address] for edge binding could not be parssed", index)
	}

	if optionsValue, found := edgeListenerMap["options"]; found {
		options := optionsValue.(map[interface{}]interface{})

		if options == nil {
			return nil, fmt.Errorf("required section [listeners[%d].options] for edge binding is not a map", index)
		}

		if advertiseVal, found := options["advertise"]; found {
			advertise := advertiseVal.(string)

			if advertise == "" {
				return nil, fmt.Errorf("required value [listeners[%d].options.advertise] for edge binding was not a string or was not found", index)
			}

			advertiseParts := strings.Split(advertise, ":")
			advertisePort, err := strconv.Atoi(advertiseParts[1])

			if err != nil {
				return nil, fmt.Errorf("port number for [listeners[%d].options.advertise] for edge binding could not be parssed", index)
			}

			if advertisePort != addressPort {
				pfxlog.Logger().Infof("advertised port [%d] in [listeners[%d].options.advertise] does not match the listening port [%d] in [listeners[%d].address].", index, advertisePort, index, addressPort)
			}

			return &edge_ctrl_pb.Listener{
				Advertise: &edge_ctrl_pb.Address{
					Value:    advertise,
					Protocol: addressParts[0],
					Hostname: advertiseParts[0],
					Port:     int32(advertisePort),
				},
				Address: &edge_ctrl_pb.Address{
					Value:    address,
					Protocol: addressParts[0],
					Hostname: addressParts[1],
					Port:     int32(addressPort),
				},
			}, nil
		} else {
			return nil, fmt.Errorf("required value [listeners[%d].options.advertise]for edge binding was not found", index)
		}

	} else {
		return nil, fmt.Errorf("required value [listeners[%d].options] for edge binding not found", index)
	}
}

func (config *Config) loadCsr(configMap map[interface{}]interface{}, pathPrefix string) error {
	config.Csr = Csr{}

	if configMap == nil {
		return fmt.Errorf("nil config map")
	}

	if pathPrefix != "" {
		pathPrefix = pathPrefix + "."
	}

	if value, found := configMap["csr"]; found {
		submap := value.(map[interface{}]interface{})

		if submap == nil {
			return fmt.Errorf("required section [%scsr] is not a map", pathPrefix)
		}

		if err := mapstructure.Decode(submap, &config.Csr); err != nil {
			return fmt.Errorf("failed to load [%scsr]: %s", pathPrefix, err)
		}

	} else {
		return fmt.Errorf("required section [%scsr] not found", pathPrefix)
	}

	for _, uristr := range config.Csr.Sans.UriAddresses {
		parsedUrl, err := url.Parse(uristr)
		if err != nil {
			return fmt.Errorf("invalid SAN URI encountered in configuration file: %s", uristr)
		}
		config.Csr.Sans.UriAddressesParsed = append(config.Csr.Sans.UriAddressesParsed, parsedUrl)
	}

	for _, ipstr := range config.Csr.Sans.IpAddresses {
		ip := net.ParseIP(ipstr)
		if ip == nil {
			return fmt.Errorf("invalid SAN IP address encountered in configuration file: %s", ipstr)
		}
		config.Csr.Sans.IpAddressesParsed = append(config.Csr.Sans.IpAddressesParsed, ip)
	}

	return nil
}

func (config *Config) ensureIdentity(rootConfigMap map[interface{}]interface{}) error {
	if config.RouterConfig == nil {
		config.RouterConfig = &router.Config{}
	}

	//if we already have an Id (loaded by the fabric router) use that as is to avoid
	//duplicating the identity and causing issues w/ tls.Config cert updates
	if config.RouterConfig.Id != nil {
		return nil
	}

	idConfig, err := router.LoadIdentityConfigFromMap(rootConfigMap)

	if err != nil {
		return err
	}

	id, err := identity.LoadIdentity(*idConfig)

	if err != nil {
		return err
	}

	config.RouterConfig.Id = identity.NewIdentity(id)

	return nil
}

func (config *Config) loadTransportConfig(rootConfigMap map[interface{}]interface{}) error {
	if val, ok := rootConfigMap["transport"]; ok && val != nil {
		var tcfg map[interface{}]interface{}
		if tcfg, ok = val.(map[interface{}]interface{}); !ok {
			return fmt.Errorf("expected map as transport configuration")
		}
		config.Tcfg = tcfg
	}

	return nil
}

// LoadConfigFromMapForEnrollment loads a minimal subset of the router configuration to allow for enrollment.
// This process should be used to load edge enabled routers as well as non-edge routers.
func (config *Config) LoadConfigFromMapForEnrollment(cfgmap map[interface{}]interface{}) interface{} {
	var err error
	config.EnrollmentIdentityConfig, err = router.LoadIdentityConfigFromMap(cfgmap)

	if err != nil {
		return err
	}

	edgeVal := cfgmap["edge"]

	if edgeVal != nil {
		if err := config.loadCsr(cfgmap["edge"].(map[interface{}]interface{}), "edge"); err != nil {
			pfxlog.Logger().Warnf("could not load [edge.csr]: %v", err)
		} else {
			return nil
		}
	}

	//try loading the root csr
	if rootErr := config.loadCsr(cfgmap, ""); rootErr != nil {
		pfxlog.Logger().Warnf("could not load [csr]: %v", rootErr)

	} else {
		return nil
	}

	return fmt.Errorf("could not load [edge.csr] nor [csr] sections, see warnings")
}
