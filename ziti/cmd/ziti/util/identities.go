package util

import (
	"encoding/json"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
)

type RestClientConfig struct {
	Identities map[string]*RestClientIdentity `json:"identities"`
	Default    string                         `json:"default"`
}

func (self *RestClientConfig) GetIdentity() string {
	if common.CliIdentity != "" {
		return common.CliIdentity
	}
	if self.Default != "" {
		return self.Default
	}
	return "default"
}

type RestClientIdentity struct {
	Url       string `json:"url"`
	Username  string `json:"username"`
	Token     string `json:"token"`
	LoginTime string `json:"loginTime"`
	Cert      string `json:"cert,omitempty"`
	ReadOnly  bool   `json:"readOnly"`
}

func (self *RestClientIdentity) GetCert() string {
	return self.Cert
}

func (self *RestClientIdentity) GetToken() string {
	return self.Token
}

func (self *RestClientIdentity) GetBaseUrl() string {
	return self.Url
}

func LoadRestClientConfig() (*RestClientConfig, string, error) {
	config := &RestClientConfig{
		Identities: map[string]*RestClientIdentity{},
	}

	cfgDir, err := ConfigDir()
	if err != nil {
		return nil, "", errors.Wrap(err, "couldn't get config dir while loading cli configuration")
	}
	configFile := filepath.Join(cfgDir, "ziti-cli.json")
	_, err = os.Stat(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return config, configFile, nil
		}
		return nil, "", errors.Wrapf(err, "error while statting config file %v", configFile)
	}
	result, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, "", errors.Wrapf(err, "error while reading config file %v", configFile)
	}

	if err := json.Unmarshal(result, config); err != nil {
		return nil, "", errors.Wrapf(err, "error while parsing JSON config file %v", configFile)
	}

	return config, configFile, nil
}

func PersistRestClientConfig(config *RestClientConfig) error {
	if config.Default == "" {
		config.Default = "default"
	}

	cfgDir, err := ConfigDir()
	if err != nil {
		return errors.Wrap(err, "couldn't get config dir while persisting cli configuration")
	}
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return errors.Wrapf(err, "unable to create config dir %v", cfgDir)
	}

	configFile := filepath.Join(cfgDir, "ziti-cli.json")

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return errors.Wrap(err, "error while marshalling config to JSON")
	}

	err = ioutil.WriteFile(configFile, data, 0600)
	if err != nil {
		return errors.Wrapf(err, "error while writing config file %v", configFile)
	}

	return nil
}

var selectedIdentity *RestClientIdentity

func LoadSelectedIdentity() (*RestClientIdentity, error) {
	if selectedIdentity == nil {
		config, configFile, err := LoadRestClientConfig()
		if err != nil {
			return nil, err
		}
		id := config.GetIdentity()
		clientIdentity, found := config.Identities[id]
		if !found {
			return nil, errors.Errorf("no identity '%v' found in cli config %v", id, configFile)
		}
		selectedIdentity = clientIdentity
	}
	return selectedIdentity, nil
}

func LoadSelectedRWIdentity() (*RestClientIdentity, error) {
	id, err := LoadSelectedIdentity()
	if err != nil {
		return nil, err
	}
	if id.ReadOnly {
		return nil, errors.New("this login is marked read-only, only GET operations are allowed")
	}
	return id, nil
}
