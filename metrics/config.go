/*
	Copyright 2019 NetFoundry, Inc.

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

package metrics

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
)

type Config struct {
	handlers map[Handler]Handler
}

func LoadConfig(srcmap map[interface{}]interface{}) (*Config, error) {
	cfg :=  &Config{handlers: make(map[Handler]Handler)}

	for k, v := range srcmap {
		if name, ok := k.(string); ok {
			switch name {
			case string(HandlerTypeInfluxDB):
				if submap, ok := v.(map[interface{}]interface{}); ok {
					if influxCfg, err := LoadInfluxConfig(submap); err == nil {
						if influxHandler, err := NewInfluxDBMetricsHandler(influxCfg); err == nil {
							cfg.handlers[influxHandler] = influxHandler
							pfxlog.Logger().Infof("added influx handler")
						} else {
							return nil, fmt.Errorf("error creating influx handler (%s)", err)
						}
					}
				} else {
					return nil, errors.New("invalid influx stanza")
				}
			}
		}
	}

	return cfg, nil
}