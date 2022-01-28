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
	"net"
	"strconv"
	"strings"
)

// BindPoint represents the interface:port address of where a http.Server should listen for a WebListener and the public
// address that should be used to address it.
type BindPoint struct {
	InterfaceAddress string // <interface>:<port>
	Address          string //<ip/host>:<port>
	NewAddress       string //<ip/host>:<port> sent out as a header for clients to alternatively swap to (ip -> hostname moves)
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

	if interfaceVal, ok := config["newAddress"]; ok {
		if address, ok := interfaceVal.(string); ok {
			bindPoint.NewAddress = address
		} else {
			return errors.New("could not use value for newAddress, not a string")
		}
	}

	return nil
}

// Validate this configuration object.
func (bindPoint *BindPoint) Validate() error {

	// required
	if err := validateHostPort(bindPoint.InterfaceAddress); err != nil {
		return fmt.Errorf("invalid interface address [%s]: %v", bindPoint.InterfaceAddress, err)
	}

	// required
	if err := validateHostPort(bindPoint.Address); err != nil {
		return fmt.Errorf("invalid advertise address [%s]: %v", bindPoint.Address, err)
	}

	//optional
	if bindPoint.NewAddress != "" {
		if err := validateHostPort(bindPoint.NewAddress); err != nil {
			return fmt.Errorf("invalid new address [%s]: %v", bindPoint.NewAddress, err)
		}
	}

	return nil
}

func validateHostPort(address string) error {
	address = strings.TrimSpace(address)

	if address == "" {
		return errors.New("must not be an empty string or unspecified")
	}

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		return errors.Errorf("could not split host and port: %v", err)
	}

	if host == "" {
		return errors.New("host must be specified")
	}

	if port == "" {
		return errors.New("port must be specified")
	}

	if port, err := strconv.ParseInt(port, 10, 32); err != nil {
		return errors.New("invalid port, must be a integer")
	} else if port < 1 || port > 65535 {
		return errors.New("invalid port, must 1-65535")
	}

	return nil
}
