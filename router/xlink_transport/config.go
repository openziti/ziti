/*
	(c) Copyright NetFoundry Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/link"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"reflect"
	"time"
)

const (
	MinRetryInterval = 10 * time.Millisecond
	MaxRetryInterval = 24 * time.Hour

	MinRetryBackoffFactor = 1
	MaxRetryBackoffFactor = 100

	DefaultHealthyMinRetryInterval   = 5 * time.Second
	DefaultHealthyMaxRetryInterval   = 5 * time.Minute
	DefaultHealthyRetryBackoffFactor = 1.5

	DefaultUnhealthyMinRetryInterval   = time.Minute
	DefaultUnhealthyMaxRetryInterval   = time.Hour
	DefaultUnhealthyRetryBackoffFactor = 10
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

	if value, found := data["bindInterface"]; found {
		if addressString, ok := value.(string); ok {
			config.bindInterface = addressString
		} else {
			return nil, fmt.Errorf("invalid 'bindInterface' address in listener config (%T)", value)
		}
	} else {
		if addr, ok := config.bind.(transport.HostPortAddress); ok {
			intf, err := transport.ResolveInterface(addr.Hostname())
			if err != nil {
				pfxlog.Logger().WithError(err).WithField("addr", addr.String()).Warn("unable to get interface for address")
			} else {
				pfxlog.Logger().Infof("found interface %v for bind address %v", intf.Name, addr.String())
				config.bindInterface = intf.Name
			}
		}
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
			for _, tag := range costTags {
				config.linkCostTags = append(config.linkCostTags, fmt.Sprint(tag))
			}
		} else {
			return nil, fmt.Errorf("invalid 'costTags' value in listener config (%s)", reflect.TypeOf(value))
		}
	}

	if value, found := data["groups"]; found {
		if group, ok := value.(string); ok {
			config.groups = append(config.groups, group)
		} else if groups, ok := value.([]interface{}); ok {
			for _, group := range groups {
				config.groups = append(config.groups, fmt.Sprint(group))
			}
		} else {
			return nil, fmt.Errorf("invalid 'groups' value in listener config (%s)", reflect.TypeOf(value))
		}
	}

	if len(config.groups) == 0 {
		config.groups = append(config.groups, link.GroupDefault)
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
	bind          transport.Address
	advertise     transport.Address
	bindInterface string
	linkProtocol  string
	linkCostTags  []string
	groups        []string
	options       *channel.Options
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

	if value, found := data["bind"]; found {
		logrus.Debugf("Parsing dialer bind config")
		if addressString, ok := value.(string); ok {
			_, err := transport.ResolveInterface(addressString)
			if err != nil {
				return nil, errors.Errorf("invalid 'bind' address in dialer config (%s)", err)
			}
			config.localBinding = addressString
			logrus.Debugf("Using local bind address %s", config.localBinding)
		} else {
			return nil, fmt.Errorf("invalid 'bind' address in dialer config (%s)", reflect.TypeOf(value))
		}
	}

	if value, found := data["groups"]; found {
		if group, ok := value.(string); ok {
			config.groups = append(config.groups, group)
		} else if groups, ok := value.([]interface{}); ok {
			for _, group := range groups {
				config.groups = append(config.groups, fmt.Sprint(group))
			}
		} else {
			return nil, fmt.Errorf("invalid 'groups' value in listener config (%s)", reflect.TypeOf(value))
		}
	}

	if len(config.groups) == 0 {
		config.groups = append(config.groups, link.GroupDefault)
	}

	config.healthyBackoffConfig = &backoffConfig{
		minRetryInterval:   DefaultHealthyMinRetryInterval,
		maxRetryInterval:   DefaultHealthyMaxRetryInterval,
		retryBackoffFactor: DefaultHealthyRetryBackoffFactor,
	}

	config.unhealthyBackoffConfig = &backoffConfig{
		minRetryInterval:   DefaultUnhealthyMinRetryInterval,
		maxRetryInterval:   DefaultUnhealthyMaxRetryInterval,
		retryBackoffFactor: DefaultUnhealthyRetryBackoffFactor,
	}

	if value, found := data["healthyDialBackoff"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if err := config.healthyBackoffConfig.load(submap); err != nil {
				return nil, errors.Wrap(err, "failed to parse healthyDialBackoff config")
			}
		} else {
			return nil, fmt.Errorf("invalid 'healthyDialBackoff' in dialer config (%s)", reflect.TypeOf(value))
		}
	}

	if value, found := data["unhealthyDialBackoff"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if err := config.unhealthyBackoffConfig.load(submap); err != nil {
				return nil, errors.Wrap(err, "failed to parse unhealthyDialBackoff config")
			}
		} else {
			return nil, fmt.Errorf("invalid 'healthyDialBackoff' in dialer config (%s)", reflect.TypeOf(value))
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

type backoffConfig struct {
	minRetryInterval   time.Duration
	maxRetryInterval   time.Duration
	retryBackoffFactor float64
}

func (self *backoffConfig) GetMinRetryInterval() time.Duration {
	return self.minRetryInterval
}

func (self *backoffConfig) GetMaxRetryInterval() time.Duration {
	return self.maxRetryInterval
}

func (self *backoffConfig) GetRetryBackoffFactor() float64 {
	return self.retryBackoffFactor
}

func (self *backoffConfig) load(data map[interface{}]interface{}) error {
	if value, found := data["retryBackoffFactor"]; found {
		if floatValue, ok := value.(float64); ok {
			self.retryBackoffFactor = floatValue
		} else if intValue, ok := value.(int); ok {
			self.retryBackoffFactor = float64(intValue)
		} else {
			return errors.Errorf("invalid (non-numeric) value for retryBackoffFactor: %v", value)
		}

		if self.retryBackoffFactor < MinRetryBackoffFactor {
			return errors.Errorf("retryBackoffFactor of %v is lower than minimum value of %v", self.retryBackoffFactor, MinRetryBackoffFactor)
		}
		if self.retryBackoffFactor > MaxRetryBackoffFactor {
			return errors.Errorf("retryBackoffFactor of %v is larger than maximum value of %v", self.retryBackoffFactor, MaxRetryBackoffFactor)
		}
	}

	if value, found := data["minRetryInterval"]; found {
		if strVal, ok := value.(string); ok {
			if d, err := time.ParseDuration(strVal); err == nil {
				self.minRetryInterval = d
			} else {
				return errors.Wrapf(err, "invalid value for minRetryInterval: %v", value)
			}
		} else {
			return errors.Errorf("invalid (non-string) value for minRetryInterval: %v", value)
		}

		if self.minRetryInterval < MinRetryInterval {
			return errors.Errorf("minRetryInterval of %v is lower than minimum value of %v", self.minRetryInterval, MinRetryInterval)
		}
		if self.minRetryInterval > MaxRetryInterval {
			return errors.Errorf("minRetryInterval of %v is larger than maximum value of %v", self.minRetryInterval, MaxRetryInterval)
		}
	}

	if value, found := data["maxRetryInterval"]; found {
		if strVal, ok := value.(string); ok {
			if d, err := time.ParseDuration(strVal); err == nil {
				self.maxRetryInterval = d
			} else {
				return errors.Wrapf(err, "invalid value for maxRetryInterval: %v", value)
			}
		} else {
			return errors.Errorf("invalid (non-string) value for maxRetryInterval: %v", value)
		}

		if self.maxRetryInterval < MinRetryInterval {
			return errors.Errorf("maxRetryInterval of %v is lower than minimum value of %v", self.maxRetryInterval, MinRetryInterval)
		}
		if self.maxRetryInterval > MaxRetryInterval {
			return errors.Errorf("maxRetryInterval of %v is larger than maximum value of %v", self.maxRetryInterval, MaxRetryInterval)
		}
	}

	if self.minRetryInterval > self.maxRetryInterval {
		return errors.Errorf("minRetryInterval of %v is larger than maxRetryInterval value of %v", self.minRetryInterval, self.maxRetryInterval)
	}

	return nil
}

type dialerConfig struct {
	split                  bool
	localBinding           string
	groups                 []string
	options                *channel.Options
	healthyBackoffConfig   *backoffConfig
	unhealthyBackoffConfig *backoffConfig
}
