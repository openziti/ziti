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

package subcmd

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"

	"github.com/netfoundry/ziti-foundation/transport"
	"bytes"
)

type Config struct {
	MgmtGwListenAddress string `yaml:"listenAddress"`
	MgmtGwCertPath      string `yaml:"certPath"`
	MgmtGwKeyPath       string `yaml:"keyPath"`

	MgmtAddr       string `yaml:"mgmt"`
	mgmtAddress    transport.Address
	MgmtCertPath   string `yaml:"mgmtCertPath"`
	MgmtKeyPath    string `yaml:"mgmtKeyPath"`
	MgmtCaCertPath string `yaml:"mgmtCaCertPath"`
}

func loadConfig(path string) (*Config, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.NewDecoder(bytes.NewBuffer(buf)).Decode(config)
	if err != nil {
		return nil, err
	}

	if config.MgmtGwListenAddress == "" {
		return nil, errors.New("configuration missing [listenAddress]")
	}

	if config.MgmtAddr == "" {
		return nil, errors.New("configuration missing [mgmt]")
	}
	config.mgmtAddress, err = transport.ParseAddress(config.MgmtAddr)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cannot parse [mgmt] (%s)", err))
	}

	if config.MgmtCertPath == "" {
		return nil, errors.New("configuration missing [mgmtCertPath]")
	}

	if config.MgmtKeyPath == "" {
		return nil, errors.New("configuration missing [mgmtKeyPath]")
	}

	if config.MgmtCaCertPath == "" {
		return nil, errors.New("configuration missing [mgmtCaCertPath]")
	}

	return config, nil
}
