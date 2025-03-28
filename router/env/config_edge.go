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
	"github.com/pkg/errors"
)

const (
	DefaultHeartbeatIntervalSeconds   = 60
	MinHeartbeatIntervalSeconds       = 10
	MaxHeartbeatIntervalSeconds       = 60
	DefaultSessionValidateChunkSize   = 1000
	DefaultSessionValidateMinInterval = "250ms"
	DefaultSessionValidateMaxInterval = "1500ms"
)

type EdgeConfig struct {
	Enabled                    bool
	ApiProxy                   ApiProxy
	EdgeListeners              []*edge_ctrl_pb.Listener
	Csr                        *Csr
	HeartbeatIntervalSeconds   int
	SessionValidateChunkSize   uint32
	SessionValidateMinInterval time.Duration
	SessionValidateMaxInterval time.Duration
	Tcfg                       transport.Configuration
	ForceExtendEnrollment      bool

	RouterConfig *Config

	Db             string
	DbSaveInterval time.Duration
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

type Memory struct {
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"intervalMs"`
}

type Sans struct {
	DnsAddresses       []string `yaml:"dns" mapstructure:"dns"`
	IpAddresses        []string `yaml:"ip" mapstructure:"ip"`
	IpAddressesParsed  []net.IP
	EmailAddresses     []string `yaml:"email" mapstructure:"email"`
	UriAddresses       []string `yaml:"uri" mapstructure:"uri"`
	UriAddressesParsed []*url.URL
}

func NewEdgeConfig(routerConfig *Config) *EdgeConfig {
	return &EdgeConfig{
		RouterConfig: routerConfig,
	}
}

func (config *EdgeConfig) LoadEdgeConfigFromMap(configMap map[interface{}]interface{}, loadIdentity bool) error {
	var err error
	config.Enabled = false

	var defaultDbLocation = "./db.proto.gzip"

	if value, found := configMap[PathMapKey]; found {
		configPath := value.(string)
		configPath = strings.TrimSpace(configPath)
		defaultDbLocation = configPath + ".proto.gzip"
	} else {
		pfxlog.Logger().Warnf("the db property was not set, using default for cached data model: %s", config.Db)
	}

	config.Db = defaultDbLocation
	config.DbSaveInterval = 30 * time.Second

	if err = config.loadCsr(configMap); err != nil {
		return err
	}

	var edgeConfigMap map[interface{}]interface{}

	if val, ok := configMap["edge"]; ok && val != nil {
		config.Enabled = true
		if edgeConfigMap, ok = val.(map[interface{}]interface{}); !ok {
			return fmt.Errorf("expected map as edge configuration")
		}
	} else {
		return nil
	}

	if loadIdentity {
		if err := config.ensureIdentity(configMap); err != nil {
			return err
		}
	}

	if val, found := edgeConfigMap["db"]; found {
		config.Db = val.(string)
	}

	if config.Db == "" {
		config.Db = defaultDbLocation
		pfxlog.Logger().Warnf("the db property was not set, using default for cached data model: %s", config.Db)
	}

	if val, found := edgeConfigMap["dbSaveIntervalSeconds"]; found {
		seconds := val.(int)
		config.DbSaveInterval = time.Duration(seconds) * time.Second
	}

	if config.DbSaveInterval < 30*time.Second {
		config.DbSaveInterval = 30 * time.Second
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

	if err = config.loadEdgeListener(configMap); err != nil {
		return err
	}

	if err = config.loadTransportConfig(configMap); err != nil {
		return err
	}

	return nil
}

func (config *EdgeConfig) loadApiProxy(edgeConfigMap map[interface{}]interface{}) error {
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

func (config *EdgeConfig) loadEdgeListener(rootConfigMap map[interface{}]interface{}) error {
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

// loadCsr search for a root `csr` path or an `edge.csr` path for a CSR definition. The root path is preferred.
func (config *EdgeConfig) loadCsr(rootConfigMap map[interface{}]interface{}) error {
	csrI, ok := rootConfigMap["csr"]
	csrPath := "csr"

	if !ok {
		edgeI, ok := rootConfigMap["edge"]

		if !ok {
			return fmt.Errorf("root [csr] not found, root [edge] not found to check for [edge.csr], could not load any CSR")
		}

		edgeMap, ok := edgeI.(map[interface{}]interface{})

		if !ok {
			return fmt.Errorf("root [csr] not found, root [edge] found but was not a map/object")
		}

		csrI, ok = edgeMap["csr"]

		if !ok {
			return fmt.Errorf("root [csr] not found, root [edge] found but [edge.csr] is not defined, could not load any CSR")
		}

		csrPath = "edge.csr"
	}

	if csrI == nil {
		return fmt.Errorf("root [csr] not found, [edge.csr] not found, could not locate any CSR definition")
	}

	csrMap, ok := csrI.(map[interface{}]interface{})

	if !ok {
		return fmt.Errorf("could not load csr [%s] was found but is not an object/map", csrPath)
	}

	csr, err := config.parseCsr(csrMap, csrPath)

	if err != nil {
		return err
	}

	pfxlog.Logger().Infof("loaded csr info from configuration file at path [%s]", csrPath)

	config.Csr = csr

	return nil
}

// parseCsr parses the given map as a CSR definition. Error messages are based on the path provided.
func (config *EdgeConfig) parseCsr(csrMap map[interface{}]interface{}, path string) (*Csr, error) {
	targetCsr := &Csr{}

	if csrMap == nil {
		return nil, fmt.Errorf("nil map")
	}

	if err := mapstructure.Decode(csrMap, targetCsr); err != nil {
		return nil, fmt.Errorf("failed to load [%s]: %s", path, err)
	}

	for _, uristr := range targetCsr.Sans.UriAddresses {
		parsedUrl, err := url.Parse(uristr)
		if err != nil {
			return nil, fmt.Errorf("invalid SAN URI encountered in configuration file: %s", uristr)
		}
		targetCsr.Sans.UriAddressesParsed = append(targetCsr.Sans.UriAddressesParsed, parsedUrl)
	}

	for _, ipstr := range targetCsr.Sans.IpAddresses {
		ip := net.ParseIP(ipstr)
		if ip == nil {
			return nil, fmt.Errorf("invalid SAN IP address encountered in configuration file: %s", ipstr)
		}
		targetCsr.Sans.IpAddressesParsed = append(targetCsr.Sans.IpAddressesParsed, ip)
	}

	return targetCsr, nil
}

func (config *EdgeConfig) ensureIdentity(rootConfigMap map[interface{}]interface{}) error {
	if config.RouterConfig == nil {
		config.RouterConfig = &Config{}
	}

	//if we already have an Id (loaded by the fabric router) use that as is to avoid
	//duplicating the identity and causing issues w/ tls.Config cert updates
	if config.RouterConfig.Id != nil {
		return nil
	}

	idConfig, err := LoadIdentityConfigFromMap(rootConfigMap)

	if err != nil {
		return err
	}

	id, err := identity.LoadIdentity(*idConfig)

	if err != nil {
		return err
	}

	config.RouterConfig.Id = identity.NewIdentity(id)

	if err := config.RouterConfig.Id.WatchFiles(); err != nil {
		pfxlog.Logger().Warn("could not enable file watching on edge router identity: %w", err)
	}

	return nil
}

func (config *EdgeConfig) loadTransportConfig(rootConfigMap map[interface{}]interface{}) error {
	if val, ok := rootConfigMap["transport"]; ok && val != nil {
		config.Tcfg = make(transport.Configuration)
		if tcfg, ok := val.(map[interface{}]interface{}); ok {
			for k, v := range tcfg {
				config.Tcfg[k] = v
			}
		} else {
			return fmt.Errorf("expected map as transport configuration")
		}
	}

	return nil
}
