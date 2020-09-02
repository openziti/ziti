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

package xweb

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/openziti/foundation/identity/identity"
	"time"
)

// The root configuration options necessary to start numerous http.Server instances via WebListener's.
type Config struct {
	WebListeners          []*WebListener
	DefaultIdentityConfig *identity.IdentityConfig
	DefaultIdentity       identity.Identity

	DefaultIdentitySection string
	WebSection             string
	enabled                bool
}

// Parses a configuration map, looking for sections that define an identity.IdentityConfig and an array of WebListener's.
func (config *Config) Parse(configMap map[interface{}]interface{}) error {

	if config.DefaultIdentitySection == "" {
		return errors.New("identity section not specified for configuration")
	}

	if config.WebSection == "" {
		return errors.New("web section not specified for configuration")
	}

	//default identity config is the root identity
	if identityInterface, ok := configMap[config.DefaultIdentitySection]; ok {
		if identityMap, ok := identityInterface.(map[interface{}]interface{}); ok {
			if identityConfig, err := parseIdentityConfig(identityMap); err == nil {
				config.DefaultIdentityConfig = identityConfig
			} else {
				return fmt.Errorf("error parsing root identity section [%s] : %v", config.DefaultIdentitySection, err)
			}

		} else {
			return fmt.Errorf("root identity section [%s] must be a map", config.DefaultIdentitySection)
		}
	} else {
		return fmt.Errorf("root identity section [%s] must be defined", config.DefaultIdentitySection)
	}

	if webInterface, ok := configMap[config.WebSection]; ok {
		//treat section like an array of maps
		if webArrayInterface, ok := webInterface.([]interface{}); ok {
			for i, webInterface := range webArrayInterface {
				if webMap, ok := webInterface.(map[interface{}]interface{}); ok {
					webListener := &WebListener{
						DefaultIdentityConfig: config.DefaultIdentityConfig,
					}
					if err := webListener.Parse(webMap); err != nil {
						return fmt.Errorf("error parsing web configuration [%s] at index [%d]: %v", config.WebSection, i, err)
					}

					config.WebListeners = append(config.WebListeners, webListener)
				} else {
					return fmt.Errorf("error parsing web configuration [%s] at index [%d]: not a map", config.WebSection, i)
				}
			}
		} else {
			return fmt.Errorf("%s identity section [%s] must be a map", config.WebSection, config.DefaultIdentitySection)
		}
	}

	return nil
}

// Uses a WebHandlerFactoryRegistry to validate that all API bindings may be fulfilled. All other relevant Config values
// are also validated.
func (config *Config) Validate(registry WebHandlerFactoryRegistry) error {

	//validate default identity by loading
	if defaultIdentity, err := identity.LoadIdentity(*config.DefaultIdentityConfig); err == nil {
		config.DefaultIdentity = defaultIdentity
	} else {
		return fmt.Errorf("could not load root identity: %v", err)
	}

	//add default loaded identity to each web
	for _, webListener := range config.WebListeners {
		webListener.DefaultIdentity = config.DefaultIdentity
	}

	for i, web := range config.WebListeners {
		//validate attributes
		if err := web.Validate(registry); err != nil {
			return fmt.Errorf("could not validate web listener at %s[%d]: %v", config.WebSection, i, err)
		}
	}

	//enabled only after validation passes
	config.enabled = true

	return nil
}

// Whether this configuration should be considered "enabled". Set to true after Validate passes.
func (config *Config) Enabled() bool {
	return config.enabled
}

// The configuration that will eventually be used to create an xweb.Server (which in turn houses all of the components
// necessary to run multiple http.Server's).
type WebListener struct {
	Name       string
	APIs       []*API
	BindPoints []*BindPoint
	Options    Options

	IdentityConfig *identity.IdentityConfig
	Identity       identity.Identity

	DefaultIdentityConfig *identity.IdentityConfig
	DefaultIdentity       identity.Identity
}

// Parses a configuration map to set all relavant WebListener values.
func (web *WebListener) Parse(webConfigMap map[interface{}]interface{}) error {
	//parse name, required, string
	if nameInterface, ok := webConfigMap["name"]; ok {
		if name, ok := nameInterface.(string); ok {
			web.Name = name
		} else {
			return errors.New("name is required to be a string")
		}
	} else {
		return errors.New("name is required")
	}

	//parse apis, require 1, objet, defer
	if apiInterface, ok := webConfigMap["apis"]; ok {
		if apiArrayInterfaces, ok := apiInterface.([]interface{}); ok {
			for i, apiInterface := range apiArrayInterfaces {
				if apiMap, ok := apiInterface.(map[interface{}]interface{}); ok {
					api := &API{}
					if err := api.Parse(apiMap); err != nil {
						return fmt.Errorf("error parsing api configuration at index [%d]: %v", i, err)
					}

					web.APIs = append(web.APIs, api)
				} else {
					return fmt.Errorf("error parsing api configuration at index [%d]: not a map", i)
				}
			}
		} else {
			return errors.New("api section must be an array")
		}
	} else {
		return errors.New("apis section is required")
	}

	//parse listen address
	if addressInterface, ok := webConfigMap["bindPoints"]; ok {
		if addressesArrayInterfaces, ok := addressInterface.([]interface{}); ok {
			for i, addressInterface := range addressesArrayInterfaces {
				if addressMap, ok := addressInterface.(map[interface{}]interface{}); ok {
					address := &BindPoint{}
					if err := address.Parse(addressMap); err != nil {
						return fmt.Errorf("error parsing address configuration at index [%d]: %v", i, err)
					}

					web.BindPoints = append(web.BindPoints, address)
				} else {
					return fmt.Errorf("error parsing address configuration at index [%d]: not a map", i)
				}
			}
		} else {
			return errors.New("addresses section must be an array")
		}
	} else {
		return errors.New("addresses section is required")
	}

	//parse identity
	if identityInterface, ok := webConfigMap["identity"]; ok {
		if identityMap, ok := identityInterface.(map[interface{}]interface{}); ok {
			if identityConfig, err := parseIdentityConfig(identityMap); err == nil {
				web.IdentityConfig = identityConfig
			} else {
				return fmt.Errorf("error parsing identity section: %v", err)
			}

		} else {
			return errors.New("identity section must be a map if defined")
		}

	} //no else, optional, will defer to router identity

	//parse options
	web.Options = Options{}
	web.Options.Default()

	if optionsInterface, ok := webConfigMap["options"]; ok {
		if optionMap, ok := optionsInterface.(map[interface{}]interface{}); ok {
			if err := web.Options.Parse(optionMap); err != nil {
				return fmt.Errorf("error parsing options section: %v", err)
			}
		} //no else, options are optional
	}

	return nil
}

// Validates all WebListener values
func (web *WebListener) Validate(registry WebHandlerFactoryRegistry) error {
	if web.Name == "" {
		return errors.New("name must not be empty")
	}

	if len(web.APIs) <= 0 {
		return errors.New("no APIs specified, must specify at least one")
	}

	for i, api := range web.APIs {
		if err := api.Validate(); err != nil {
			return fmt.Errorf("invalid API at index [%d]: %v", i, err)
		}

		//check if binding is valid
		if binding := registry.Get(api.Binding()); binding == nil {
			return fmt.Errorf("invalid API at index [%d]: invalid binding %s", i, api.Binding())
		}
	}

	if len(web.BindPoints) <= 0 {
		return errors.New("no addresses specified, must specify at lest one")
	}

	for i, address := range web.BindPoints {
		if err := address.Validate(); err != nil {
			return fmt.Errorf("invalid address at index [%d]: %v", i, err)
		}
	}

	//default identity config
	if web.IdentityConfig == nil {
		web.IdentityConfig = web.DefaultIdentityConfig
		web.Identity = web.DefaultIdentity
	}

	if web.Identity == nil {
		if web.IdentityConfig == nil {
			return errors.New("no identity specified")
		}

		if id, err := identity.LoadIdentity(*web.IdentityConfig); err == nil {
			web.Identity = id
		} else {
			return fmt.Errorf("failed to load identity: %v", err)
		}
	}

	if err := web.Options.TlsVersionOptions.Validate(); err != nil {
		return fmt.Errorf("invalid TLS version option: %v", err)
	}

	if err := web.Options.TimeoutOptions.Validate(); err != nil {
		return fmt.Errorf("invalid timeout option: %v", err)
	}

	return nil

}

// Represents some "api" or "site" by binding name. Each API configuration is used against a WebHandlerFactoryRegistry
// to locate the proper factory to generate a WebHandler. The options provided by this structure are parsed by the
// WebHandlerFactory and the behavior, valid keys, and valid values are not defined by xweb components, but by that
// WebHandlerFactory and it resulting WebHandler's.
type API struct {
	binding string
	options map[interface{}]interface{}
}

// Returns the string that uniquely identifies bo the WebHandlerFactory and resulting WebHandler's that will be attached
// to some WebListener and its resulting Server.
func (api *API) Binding() string {
	return api.binding
}

// Returns the options associated with thsi API binding.
func (api *API) Options() map[interface{}]interface{} {
	return api.options
}

// Parses the configuration map for an API.
func (api *API) Parse(apiConfigMap map[interface{}]interface{}) error {
	if bindingInterface, ok := apiConfigMap["binding"]; ok {
		if binding, ok := bindingInterface.(string); ok {
			api.binding = binding
		} else {
			return errors.New("binding must be a string")
		}
	} else {
		return errors.New("binding is required")
	}

	if optionsInterface, ok := apiConfigMap["options"]; ok {
		if optionsMap, ok := optionsInterface.(map[interface{}]interface{}); ok {
			api.options = optionsMap //leave to bindings to interpret further
		} else {
			return errors.New("options if declared must be a map")
		}
	} //no else optional

	return nil
}

// Validates this configuration object.
func (api *API) Validate() error {
	if api.Binding() == "" {
		return errors.New("binding must be specified")
	}

	return nil
}

// Represents the interface:port address of where a http.Server should listen for a WebListener and the public
// address that should be used to address it.
type BindPoint struct {
	InterfaceAddress string // <interface>:<port>
	Address          string //<ip/host>:<port>
}

// Parses the configuration map for a BindPoint.
func (bindPoint *BindPoint) Parse(config map[interface{}]interface{}) error {
	if interfaceVal, ok := config["interface"]; ok {
		if address, ok := interfaceVal.(string); ok {
			bindPoint.InterfaceAddress = address
		} else {
			return fmt.Errorf("could not use value for address, not a string")
		}
	}

	if interfaceVal, ok := config["address"]; ok {
		if address, ok := interfaceVal.(string); ok {
			bindPoint.Address = address
		} else {
			return errors.New("could not use value for address, not a string")
		}
	}

	return nil
}

// Validates this configuration object.
func (bindPoint *BindPoint) Validate() error {
	if bindPoint.InterfaceAddress == "" {
		return errors.New("value for address must be provided")
	}

	if bindPoint.Address == "" {
		return errors.New("value for address must be provided")
	}

	return nil
}

// The WebListener shared options configuration struct.
type Options struct {
	TimeoutOptions
	TlsVersionOptions
}

// Defaults all necessary values
func (options *Options) Default() {
	options.TimeoutOptions.Default()
	options.TlsVersionOptions.Default()
}

// Parses a configuariont map
func (options *Options) Parse(optionsMap map[interface{}]interface{}) error {
	if err := options.TimeoutOptions.Parse(optionsMap); err != nil {
		return fmt.Errorf("error parsing options: %v", err)
	}

	if err := options.TlsVersionOptions.Parse(optionsMap); err != nil {
		return fmt.Errorf("error parsing options: %v", err)
	}

	return nil
}

//Represents http timeout options
type TimeoutOptions struct {
	ReadTimeout  time.Duration
	IdleTimeout  time.Duration
	WriteTimeout time.Duration
}

// Defaults all http. timeout options
func (timeoutOptions *TimeoutOptions) Default() {
	timeoutOptions.WriteTimeout = time.Second * 10
	timeoutOptions.ReadTimeout = time.Second * 5
	timeoutOptions.IdleTimeout = time.Second * 5
}

// Parses a config map
func (timeoutOptions *TimeoutOptions) Parse(config map[interface{}]interface{}) error {
	if interfaceVal, ok := config["readTimeoutMs"]; ok {
		if readTimeoutMs, ok := interfaceVal.(int); ok {
			timeoutOptions.ReadTimeout = time.Duration(readTimeoutMs) * time.Millisecond
		} else {
			return errors.New("could not use value for readTimeoutMs, not an integer")
		}
	}

	if interfaceVal, ok := config["idleTimeoutMs"]; ok {
		if idleTimeoutMs, ok := interfaceVal.(int); ok {
			timeoutOptions.IdleTimeout = time.Duration(idleTimeoutMs) * time.Millisecond
		} else {
			return errors.New("could not use value for idleTimeoutMs, not an integer")
		}
	}

	if interfaceVal, ok := config["writeTimeoutMs"]; ok {
		if writeTimeoutMs, ok := interfaceVal.(int); ok {
			timeoutOptions.WriteTimeout = time.Duration(writeTimeoutMs) * time.Millisecond
		} else {
			return errors.New("could not use value for writeTimeoutMs, not an integer")
		}
	}

	return nil
}

// Validtes all settings
func (timeoutOptions *TimeoutOptions) Validate() error {
	if timeoutOptions.WriteTimeout <= 0 {
		return fmt.Errorf("value [%s] for writeTimeout too low, must be positive", timeoutOptions.WriteTimeout.String())
	}

	if timeoutOptions.ReadTimeout <= 0 {
		return fmt.Errorf("value [%s] for readTimeout too low, must be positive", timeoutOptions.ReadTimeout.String())
	}

	if timeoutOptions.IdleTimeout <= 0 {
		return fmt.Errorf("value [%s] for idleTimeout too low, must be positive", timeoutOptions.IdleTimeout.String())
	}

	return nil
}

// Represents TLS version options
type TlsVersionOptions struct {
	MinTLSVersion    int
	minTLSVersionStr string

	MaxTLSVersion    int
	maxTLSVersionStr string
}

// A map of configuration strings to TLS version identifiers
var tlsVersionMap = map[string]int{
	"TLS1.0": tls.VersionTLS10,
	"TLS1.1": tls.VersionTLS11,
	"TLS1.2": tls.VersionTLS12,
	"TLS1.3": tls.VersionTLS13,
}

// Defaults TLS versions
func (tlsVersionOptions *TlsVersionOptions) Default() {
	tlsVersionOptions.MinTLSVersion = tls.VersionTLS12
	tlsVersionOptions.MaxTLSVersion = tls.VersionTLS13
}

// Parses a config map
func (tlsVersionOptions *TlsVersionOptions) Parse(config map[interface{}]interface{}) error {
	if interfaceVal, ok := config["minTLSVersion"]; ok {
		var ok bool
		if tlsVersionOptions.minTLSVersionStr, ok = interfaceVal.(string); ok {
			if minTLSVersion, ok := tlsVersionMap[tlsVersionOptions.minTLSVersionStr]; ok {
				tlsVersionOptions.MinTLSVersion = minTLSVersion
			} else {
				return fmt.Errorf("could not use value for minTLSVersion, invalid value [%s]", tlsVersionOptions.minTLSVersionStr)
			}
		} else {
			return errors.New("could not use value for minTLSVersion, not an string")
		}
	}

	if interfaceVal, ok := config["maxTLSVersion"]; ok {
		var ok bool
		if tlsVersionOptions.maxTLSVersionStr, ok = interfaceVal.(string); ok {
			if maxTLSVersion, ok := tlsVersionMap[tlsVersionOptions.maxTLSVersionStr]; ok {
				tlsVersionOptions.MaxTLSVersion = maxTLSVersion
			} else {
				return fmt.Errorf("could not use value for maxTLSVersion, invalid value [%s]", tlsVersionOptions.maxTLSVersionStr)
			}
		} else {
			return errors.New("could not use value for maxTLSVersion, not an string")
		}
	}

	return nil
}

// Validates the configuration values
func (tlsVersionOptions *TlsVersionOptions) Validate() error {
	if tlsVersionOptions.MinTLSVersion > tlsVersionOptions.MaxTLSVersion {
		return fmt.Errorf("minTLSVersion [%s] must be less than or equal to maxTLSVersion [%s]", tlsVersionOptions.minTLSVersionStr, tlsVersionOptions.maxTLSVersionStr)
	}

	return nil
}

func parseIdentityConfig(identityMap map[interface{}]interface{}) (*identity.IdentityConfig, error) {
	idConfig := &identity.IdentityConfig{}

	if certInterface, ok := identityMap["cert"]; ok {
		if cert, ok := certInterface.(string); ok {
			idConfig.Cert = cert
		} else {
			return nil, errors.New("error parsing identity: cert must be a string")
		}
	} else {
		return nil, errors.New("error parsing identity: cert required")
	}

	if serverCertInterface, ok := identityMap["server_cert"]; ok {
		if serverCert, ok := serverCertInterface.(string); ok {
			idConfig.ServerCert = serverCert
		} else {
			return nil, errors.New("error parsing identity: server_cert must be a string")
		}
	} else {
		return nil, errors.New("error parsing identity: server_cert required")
	}

	if keyInterface, ok := identityMap["key"]; ok {
		if key, ok := keyInterface.(string); ok {
			idConfig.Key = key
		} else {
			return nil, errors.New("error parsing identity: key must be a string")
		}
	} else {
		return nil, errors.New("error parsing identity: key required")
	}

	if caInterface, ok := identityMap["ca"]; ok {
		if ca, ok := caInterface.(string); ok {
			idConfig.CA = ca
		} else {
			return nil, errors.New("error parsing identity: ca must be a string")
		}
	} else {
		return nil, errors.New("error parsing identity: ca required")
	}

	return idConfig, nil
}
