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
	"github.com/openziti/channel"
	"github.com/openziti/foundation/transport"
	"github.com/pkg/errors"
	"reflect"
)

func loadListenerConfig(data map[interface{}]interface{}) (*listenerConfig, error) {
	config := &listenerConfig{}

	if value, found := data["bind"]; found {
		if addressString, ok := value.(string); ok {
			if address, err := transport.ParseAddress(addressString); err == nil {
				config.bind = address
				config.advertise = address
				config.linkProtocol = address.Type()
			} else {
				return nil, fmt.Errorf("error parsing 'bind' address in listener config (%w)", err)
			}
		} else {
			return nil, fmt.Errorf("invalid 'bind' address in listener config (%s)", reflect.TypeOf(value))
		}
	} else {
		return nil, fmt.Errorf("missing 'bind' address in listener config")
	}

	if value, found := data["advertise"]; found {
		if addressString, ok := value.(string); ok {
			if address, err := transport.ParseAddress(addressString); err == nil {
				config.advertise = address
			} else {
				return nil, fmt.Errorf("error parsing 'advertise' address in listener config")
			}
		} else {
			return nil, fmt.Errorf("invalid 'advertise' address in listener config (%s)", reflect.TypeOf(value))
		}
	}

	if value, found := data["costTags"]; found {
		if costTags, ok := value.([]interface{}); ok {
			stringTags := make([]string, len(costTags))
			for i, tag := range costTags {
				stringTags[i] = fmt.Sprint(tag)
			}
			config.linkCostTags = stringTags
		} else {
			return nil, fmt.Errorf("invalid 'costTags' address in listener config (%s)", reflect.TypeOf(value))
		}
	}

	if value, found := data["options"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			options, err := channel.LoadOptions(submap)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse link listener options")
			}
			config.options = options
		} else {
			return nil, fmt.Errorf("invalid 'options' in listener config (%s)", reflect.TypeOf(value))
		}
	} else {
		config.options = channel.DefaultOptions()
	}

	return config, nil
}

type listenerConfig struct {
	bind         transport.Address
	advertise    transport.Address
	linkProtocol string
	linkCostTags []string
	options      *channel.Options
}

func loadDialerConfig(data map[interface{}]interface{}) (*dialerConfig, error) {
	config := &dialerConfig{split: true}

	if value, found := data["split"]; found {
		if split, ok := value.(bool); ok {
			config.split = split
		} else {
			return nil, errors.Errorf("invalid 'split' flag in dialer config (%s)", reflect.TypeOf(value))
		}
	}

	if value, found := data["options"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			options, err := channel.LoadOptions(submap)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse link dialer options")
			}
			config.options = options
		} else {
			return nil, fmt.Errorf("invalid 'options' in dialer config (%s)", reflect.TypeOf(value))
		}
	}

	return config, nil
}

type dialerConfig struct {
	split   bool
	options *channel.Options
}
