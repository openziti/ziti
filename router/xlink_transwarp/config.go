/*
	(c) Copyright NetFoundry, Inc.

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

package xlink_transwarp

import (
	"fmt"
	"net"
	"reflect"
)

func loadListenerConfig(data map[interface{}]interface{}) (*listenerConfig, error) {
	config := &listenerConfig{}

	if value, found := data["bind"]; found {
		if addressString, ok := value.(string); ok {
			if address, err := net.ResolveUDPAddr("udp", addressString); err == nil {
				config.bindAddress = address
			} else {
				return nil, fmt.Errorf("error parsing 'bind' address (%w)", err)
			}
		} else {
			return nil, fmt.Errorf("invalid 'bind' address in listener config (%s)", reflect.TypeOf(value))
		}
	} else {
		return nil, fmt.Errorf("missing 'bind' address in listener config")
	}

	if value, found := data["advertise"]; found {
		if addressString, ok := value.(string); ok {
			if address, err := net.ResolveUDPAddr("udp", addressString); err == nil {
				config.advertiseAddress = address
			} else {
				return nil, fmt.Errorf("error parsing 'advertise' address in listener config (%w)", err)
			}
		} else {
			return nil, fmt.Errorf("invalid 'advertise' address in listener config (%s)", reflect.TypeOf(value))
		}
	} else {
		return nil, fmt.Errorf("missing 'advertise' address in listener config")
	}

	return config, nil
}

type listenerConfig struct {
	bindAddress      *net.UDPAddr
	advertiseAddress *net.UDPAddr
}

func loadDialerConfig(data map[interface{}]interface{}) (*dialerConfig, error) {
	config := &dialerConfig{}

	if value, found := data["bind"]; found {
		if addressString, ok := value.(string); ok {
			if address, err := net.ResolveUDPAddr("udp", addressString); err == nil {
				config.bindAddress = address
			} else {
				return nil, fmt.Errorf("error parsing 'bind' address in dialer config (%w)", err)
			}
		} else {
			return nil, fmt.Errorf("invalid 'bind' address in dialer config (%s)", reflect.TypeOf(value))
		}
	} else {
		return nil, fmt.Errorf("missing 'bind' address in dialer config")
	}

	return config, nil
}

type dialerConfig struct {
	bindAddress *net.UDPAddr
}
