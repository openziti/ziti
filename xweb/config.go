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

const (
	MinTLSVersion = tls.VersionTLS12
	MaxTLSVersion = tls.VersionTLS13

	DefaultHttpWriteTimeout = time.Second * 10
	DefaultHttpReadTimeout  = time.Second * 5
	DefaultHttpIdleTimeout  = time.Second * 5
)

// TlsVersionMap is a map of configuration strings to TLS version identifiers
var TlsVersionMap = map[string]int{
	"TLS1.0": tls.VersionTLS10,
	"TLS1.1": tls.VersionTLS11,
	"TLS1.2": tls.VersionTLS12,
	"TLS1.3": tls.VersionTLS13,
}

// ReverseTlsVersionMap is a map of TLS version identifiers to configuration strings
var ReverseTlsVersionMap = map[int]string{
	tls.VersionTLS10: "TLS1.0",
	tls.VersionTLS11: "TLS1.1",
	tls.VersionTLS12: "TLS1.2",
	tls.VersionTLS13: "TLS1.3",
}

// Config is the root configuration options necessary to start numerous http.Server instances via WebListener's.
type Config struct {
	SourceConfig map[interface{}]interface{}

	WebListeners []*WebListener
	WebSection   string

	DefaultIdentity        identity.Identity
	DefaultIdentitySection string

	//used for loading/validation logic, use DefaultIdentity.Config() for runtime
	defaultIdentityConfig *identity.Config

	enabled bool
}

// Parse parses a configuration map, looking for sections that define an identity.Config and an array of WebListener's.
func (config *Config) Parse(configMap map[interface{}]interface{}) error {
	config.SourceConfig = configMap

	if config.DefaultIdentity == nil && config.DefaultIdentitySection == "" {
		return errors.New("identity section not specified for configuration, must be specified if a default identity is not provided")
	}

	if config.WebSection == "" {
		return errors.New("web section not specified for configuration")
	}

	//default identity config is the root identity
	if config.DefaultIdentity == nil {
		if identityInterface, ok := configMap[config.DefaultIdentitySection]; ok {
			if identityMap, ok := identityInterface.(map[interface{}]interface{}); ok {
				if identityConfig, err := parseIdentityConfig(identityMap, config.DefaultIdentitySection); err == nil {
					config.defaultIdentityConfig = identityConfig
				} else {
					return fmt.Errorf("error parsing root identity section [%s] : %v", config.DefaultIdentitySection, err)
				}

			} else {
				return fmt.Errorf("root identity section [%s] must be a map", config.DefaultIdentitySection)
			}
		} else {
			return fmt.Errorf("root identity section [%s] must be defined", config.DefaultIdentitySection)
		}
	} else {
		config.defaultIdentityConfig = config.DefaultIdentity.GetConfig()
	}

	if webInterface, ok := configMap[config.WebSection]; ok {
		//treat section like an array of maps
		if webArrayInterface, ok := webInterface.([]interface{}); ok {
			for i, webInterface := range webArrayInterface {
				if webMap, ok := webInterface.(map[interface{}]interface{}); ok {
					webListener := &WebListener{
						DefaultIdentity: config.DefaultIdentity,
					}
					if err := webListener.Parse(webMap, config.WebSection); err != nil {
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

// Validate uses a WebHandlerFactoryRegistry to validate that all API bindings may be fulfilled. All other relevant
// Config values are also validated.
func (config *Config) Validate(registry WebHandlerFactoryRegistry) error {

	if config.DefaultIdentity == nil {
		//validate default identity by loading
		if defaultIdentity, err := identity.LoadIdentity(*config.defaultIdentityConfig); err == nil {
			config.DefaultIdentity = defaultIdentity
		} else {
			return fmt.Errorf("could not load default identity: %v", err)
		}

		//add default loaded identity to each web
		for _, webListener := range config.WebListeners {
			webListener.DefaultIdentity = config.DefaultIdentity
		}
	}

	presentApis := map[string]WebHandlerFactory{}

	for i, webListener := range config.WebListeners {
		//validate attributes
		if err := webListener.Validate(registry); err != nil {
			return fmt.Errorf("could not validate web listener at %s[%d]: %v", config.WebSection, i, err)
		}

		for _, api := range webListener.APIs {
			presentApis[api.Binding()] = registry.Get(api.Binding())
		}
	}

	for presentApiBinding, presentApiFactory := range presentApis {
		if err := presentApiFactory.Validate(config); err != nil {
			return fmt.Errorf("error validating API binding %s: %v", presentApiBinding, err)
		}
	}

	//enabled only after validation passes
	config.enabled = true

	return nil
}

// Enabled returns true/false on whether this configuration should be considered "enabled". Set to true after
// Validate passes.
func (config *Config) Enabled() bool {
	return config.enabled
}

// Options is the shared options for a WebListener.
type Options struct {
	TimeoutOptions
	TlsVersionOptions
}

// Default provides defaults for all necessary values
func (options *Options) Default() {
	options.TimeoutOptions.Default()
	options.TlsVersionOptions.Default()
}

// Parse parses a configuration map
func (options *Options) Parse(optionsMap map[interface{}]interface{}) error {
	if err := options.TimeoutOptions.Parse(optionsMap); err != nil {
		return fmt.Errorf("error parsing options: %v", err)
	}

	if err := options.TlsVersionOptions.Parse(optionsMap); err != nil {
		return fmt.Errorf("error parsing options: %v", err)
	}

	return nil
}

// TimeoutOptions represents http timeout options
type TimeoutOptions struct {
	ReadTimeout  time.Duration
	IdleTimeout  time.Duration
	WriteTimeout time.Duration
}

// Default defaults all HTTP timeout options
func (timeoutOptions *TimeoutOptions) Default() {
	timeoutOptions.WriteTimeout = DefaultHttpWriteTimeout
	timeoutOptions.ReadTimeout = DefaultHttpReadTimeout
	timeoutOptions.IdleTimeout = DefaultHttpIdleTimeout
}

// Parse parses a config map
func (timeoutOptions *TimeoutOptions) Parse(config map[interface{}]interface{}) error {
	if interfaceVal, ok := config["readTimeout"]; ok {
		if readTimeoutStr, ok := interfaceVal.(string); ok {
			if readTimeout, err := time.ParseDuration(readTimeoutStr); err == nil {
				timeoutOptions.ReadTimeout = readTimeout
			} else {
				return fmt.Errorf("could not parse readTimeout %s as a duration (e.g. 1m): %v", readTimeoutStr, err)
			}
		} else {
			return errors.New("could not use value for readTimeout, not a string")
		}
	}

	if interfaceVal, ok := config["idleTimeout"]; ok {
		if idleTimeoutStr, ok := interfaceVal.(string); ok {
			if idleTimeout, err := time.ParseDuration(idleTimeoutStr); err == nil {
				timeoutOptions.IdleTimeout = idleTimeout
			} else {
				return fmt.Errorf("could not parse idleTimeout %s as a duration (e.g. 1m): %v", idleTimeoutStr, err)
			}
		} else {
			return errors.New("could not use value for idleTimeout, not a string")
		}
	}

	if interfaceVal, ok := config["writeTimeout"]; ok {
		if writeTimeoutStr, ok := interfaceVal.(string); ok {
			if writeTimeout, err := time.ParseDuration(writeTimeoutStr); err == nil {
				timeoutOptions.WriteTimeout = writeTimeout
			} else {
				return fmt.Errorf("could not parse writeTimeout %s as a duration (e.g. 1m): %v", writeTimeoutStr, err)
			}
		} else {
			return errors.New("could not use value for writeTimeout, not a string")
		}
	}

	return nil
}

// Validate validates all settings and return nil or an error
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

// TlsVersionOptions represents TLS version options
type TlsVersionOptions struct {
	MinTLSVersion    int
	minTLSVersionStr string

	MaxTLSVersion    int
	maxTLSVersionStr string
}

// Default defaults TLS versions
func (tlsVersionOptions *TlsVersionOptions) Default() {
	tlsVersionOptions.MinTLSVersion = MinTLSVersion
	tlsVersionOptions.MaxTLSVersion = MaxTLSVersion
}

// Parse parses a config map
func (tlsVersionOptions *TlsVersionOptions) Parse(config map[interface{}]interface{}) error {
	if interfaceVal, ok := config["minTLSVersion"]; ok {
		var ok bool
		if tlsVersionOptions.minTLSVersionStr, ok = interfaceVal.(string); ok {
			if minTLSVersion, ok := TlsVersionMap[tlsVersionOptions.minTLSVersionStr]; ok {
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
			if maxTLSVersion, ok := TlsVersionMap[tlsVersionOptions.maxTLSVersionStr]; ok {
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

// Validate validates the configuration values and returns nil or error
func (tlsVersionOptions *TlsVersionOptions) Validate() error {
	if tlsVersionOptions.MinTLSVersion > tlsVersionOptions.MaxTLSVersion {
		return fmt.Errorf("minTLSVersion [%s] must be less than or equal to maxTLSVersion [%s]", tlsVersionOptions.minTLSVersionStr, tlsVersionOptions.maxTLSVersionStr)
	}

	return nil
}

func parseIdentityConfig(identityMap map[interface{}]interface{}, pathContext string) (*identity.Config, error) {
	idConfig, err := identity.NewConfigFromMap(identityMap)

	if err = idConfig.ValidateWithPathContext(pathContext); err != nil {
		return nil, fmt.Errorf("error parsing identity: %v", err)
	}

	return idConfig, nil
}
