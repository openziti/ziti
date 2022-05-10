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

import "github.com/pkg/errors"

// API represents some "api" or "site" by binding name. Each API configuration is used against a WebHandlerFactoryRegistry
// to locate the proper factory to generate a WebHandler. The options provided by this structure are parsed by the
// WebHandlerFactory and the behavior, valid keys, and valid values are not defined by xweb components, but by that
// WebHandlerFactory and its resulting WebHandler's.
type API struct {
	binding string
	options map[interface{}]interface{}
}

// Binding returns the string that uniquely identifies bo the WebHandlerFactory and resulting WebHandler instances that
// will be attached to some WebListener and its resulting Server.
func (api *API) Binding() string {
	return api.binding
}

// Options returns the options associated with this API binding.
func (api *API) Options() map[interface{}]interface{} {
	return api.options
}

// Parse the configuration map for an API.
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

// Validate this configuration object.
func (api *API) Validate() error {
	if api.Binding() == "" {
		return errors.New("binding must be specified")
	}

	return nil
}
