/*
	Copyright NetFoundry Inc.

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

package cmd

import (
	"fmt"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/viper"
	"gopkg.in/AlecAivazis/survey.v1"
	"os"
	"path/filepath"
)

var ZITI_COMPONENTS = []string{
	c.ZITI,
	c.ZITI_CONTROLLER,
	c.ZITI_PROX_C,
	c.ZITI_ROUTER,
	c.ZITI_TUNNEL,
	c.ZITI_EDGE_TUNNEL,
}

func (o *CommonOptions) GetZitiComponent(p string) (string, error) {
	if p == "" {
		prompt := &survey.Select{
			Message: "Ziti Component",
			Options: ZITI_COMPONENTS,
			Default: c.ZITI,
			Help:    "Choose the Ziti component for which you want to do config initialization",
		}

		survey.AskOne(prompt, &p, nil)
	}
	return p, nil
}

func (o *CommonOptions) createInitialZitiConfig() error {

	zitiConfigDir, err := util.ZitiAppConfigDir(c.ZITI)
	if err != nil {
		return err
	}

	_, err = util.EnvironmentsDir()
	if err != nil {
		return err
	}

	fileName := filepath.Join(zitiConfigDir, c.CONFIGFILENAME) + ".json"

	_, err = os.Stat(fileName)
	if err == nil {
		return fmt.Errorf("config file (%s) already exists", fileName)
	}

	cfgfile, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	if err := cfgfile.Close(); err != nil {
		return err
	}

	viper.SetConfigType("json")
	viper.SetConfigName(c.CONFIGFILENAME)
	viper.AddConfigPath(zitiConfigDir)

	// Set some default values
	viper.SetDefault("bin", util.HomeDir()+"/Repos/nf/ziti/bin")

	// Capture AWS creds
	awsAccessKey, err := util.PickPassword("AWS_ACCESS_KEY_ID:")
	if err != nil {
		return err
	}
	viper.SetDefault("AWS_ACCESS_KEY_ID", awsAccessKey)
	awsSecretAccessKey, err := util.PickPassword("AWS_SECRET_ACCESS_KEY:")
	if err != nil {
		return err
	}
	viper.SetDefault("AWS_SECRET_ACCESS_KEY", awsSecretAccessKey)

	err = viper.WriteConfig()
	if err != nil {
		return err
	}

	return nil
}

func (o *CommonOptions) createInitialControllerConfig() error {

	configDir, err := util.ConfigDir()
	if err != nil {
		return err
	}

	controllerConfigDir, err := util.ZitiAppConfigDir(c.ZITI_CONTROLLER)
	if err != nil {
		return err
	}

	fileName := filepath.Join(controllerConfigDir, c.CONFIGFILENAME) + ".json"

	_, err = os.Stat(fileName)
	if err == nil {
		return fmt.Errorf("config file (%s) already exists", fileName)
	}

	cfgfile, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	if err := cfgfile.Close(); err != nil {
		return err
	}

	viper.SetConfigType("json")
	viper.SetConfigName(c.CONFIGFILENAME)
	viper.AddConfigPath(controllerConfigDir)

	/*
		{
			"identity": {
				"cert": 		"etc/ca/intermediate/certs/ctrl-client.cert.pem",
				"server_cert": 	"etc/ca/intermediate/certs/ctrl-server.cert.pem",
				"key": 			"etc/ca/intermediate/private/ctrl.key.pem",
				"ca": 			"etc/ca/intermediate/certs/ca-chain.cert.pem"
			},

			"ctrlListener": "quic:0.0.0.0:6262",
			"mgmtListener": "tls:0.0.0.0:10000",
			"dbPath": "bin/ctrl.db"
		}
	*/

	// Set some default values
	viper.SetDefault("identity.cert", configDir+"/etc/ca/intermediate/certs/ctrl-client.cert.pem")
	viper.SetDefault("identity.server_cert", configDir+"/etc/ca/intermediate/certs/ctrl-server.cert.pem")
	viper.SetDefault("identity.key", configDir+"/etc/ca/intermediate/private/ctrl.key.pem")
	viper.SetDefault("identity.ca", configDir+"/etc/ca/intermediate/certs/ca-chain.cert.pem")
	viper.SetDefault("ctrlListener", "quic:0.0.0.0:6262")
	viper.SetDefault("mgmtListener", "tls:0.0.0.0:10000")
	viper.SetDefault("dbPath", "ctrl.db")

	err = viper.WriteConfig()
	if err != nil {
		return err
	}

	return nil
}
