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

package xlink_transport

import (
	"fmt"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/transport"
)

func loadConfig(data map[interface{}]interface{}) (*config, error) {
	c := &config{}

	if value, found := data["listener"]; found {
		address, err := transport.ParseAddress(value.(string))
		if err != nil {
			return nil, fmt.Errorf("cannot parse listener address (%w)", err)
		}
		c.listener = address
		c.advertise = address
	} else {
		return nil, fmt.Errorf("required 'listener' configuration missing")
	}

	if value, found := data["advertise"]; found {
		address, err := transport.ParseAddress(value.(string))
		if err != nil {
			return nil, fmt.Errorf("cannot parse advertise address (%w)", err)
		}
		c.advertise = address
	}

	if value, found := data["options"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			c.options = channel2.LoadOptions(submap)
		}
	}

	return c, nil
}

type config struct {
	listener  transport.Address
	advertise transport.Address
	options   *channel2.Options
}
