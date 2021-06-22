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
	"github.com/pkg/errors"
)

// BindPoint represents the interface:port address of where a http.Server should listen for a WebListener and the public
// address that should be used to address it.
type BindPoint struct {
	InterfaceAddress string // <interface>:<port>
	Address          string //<ip/host>:<port>
}

// Parse the configuration map for a BindPoint.
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

// Validate this configuration object.
func (bindPoint *BindPoint) Validate() error {
	if bindPoint.InterfaceAddress == "" {
		return errors.New("value for address must be provided")
	}

	if bindPoint.Address == "" {
		return errors.New("value for address must be provided")
	}

	return nil
}
