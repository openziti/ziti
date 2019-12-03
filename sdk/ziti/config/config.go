/*
	Copyright 2019 Netfoundry, Inc.

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

package config

import (
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

type Config struct {
	ZtAPI    string                  `json:"ztAPI"`
	ID       identity.IdentityConfig `json:"id"`
}

func New(ztApi string, idConfig identity.IdentityConfig) *Config {
	return &Config{
		ZtAPI:    ztApi,
		ID:       idConfig,
	}
}

func NewFromFile(confFile string) (*Config, error) {
	conf, err := ioutil.ReadFile(confFile)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Config file (%s) is not found ", confFile))
	}

	c := Config{}
	err = json.Unmarshal(conf, &c)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to load ZT configuration (%s): %v", confFile, err))
	}

	return &c, nil
}
