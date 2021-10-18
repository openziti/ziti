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
	"fmt"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
)

// WebListener is the configuration that will eventually be used to create an xweb.Server (which in turn houses all
// the components necessary to run multiple http.Server's).
type WebListener struct {
	Name       string
	APIs       []*API
	BindPoints []*BindPoint
	Options    Options

	DefaultIdentity identity.Identity
	Identity        identity.Identity
}

// Parse parses a configuration map to set all relevant WebListener values.
func (web *WebListener) Parse(webConfigMap map[interface{}]interface{}, pathContext string) error {
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
			if identityConfig, err := parseIdentityConfig(identityMap, pathContext + ".identity"); err == nil {
				web.Identity, err = identity.LoadIdentity(*identityConfig)
				if err != nil {
					return fmt.Errorf("error loading identity: %v", err)
				}
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

// Validate all WebListener values
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

	if web.Identity == nil {
		if web.DefaultIdentity == nil {
			return errors.New("no default identity specified and no identity specified")
		}

		web.Identity = web.DefaultIdentity
	}

	if err := web.Options.TlsVersionOptions.Validate(); err != nil {
		return fmt.Errorf("invalid TLS version option: %v", err)
	}

	if err := web.Options.TimeoutOptions.Validate(); err != nil {
		return fmt.Errorf("invalid timeout option: %v", err)
	}

	return nil

}
