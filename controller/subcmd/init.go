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

package subcmd

import (
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/server"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/util/term"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
)

const (
	minPasswordLength = 5
	maxPasswordLength = 100
	minUsernameLength = 4
	maxUsernameLength = 100
)

func AddCommands(root *cobra.Command, versionProvider common.VersionProvider) {
	root.AddCommand(NewEdgeCmd(versionProvider))
}

func NewEdgeCmd(versionProvider common.VersionProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edge",
		Short: "used to perform various edge related functionality",
		Long:  "used to perform various edge related functionality",
	}

	cmd.AddCommand(NewEdgeInitializeCmd(versionProvider))

	return cmd
}

type edgeInitializeOptions struct {
	password string
	username string
	name     string
}

func NewEdgeInitializeCmd(versionProvider common.VersionProvider) *cobra.Command {
	options := &edgeInitializeOptions{}

	cmd := &cobra.Command{
		Use:     "init <config> [-p]",
		Aliases: []string{"initialize"},
		Example: "ziti-controller edge init controller.yml -u admin -p o93wjh5n",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("config file not specified: ziti-controller edge init <config>")
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctrl := configureController(args[0], versionProvider)

			if options.username == "" {
				options.username = promptUsername()
			}

			if options.password == "" {
				options.password = promptPassword()
			}

			if len(options.name) < 4 {
				pfxlog.Logger().Fatal("Name must be 4 or more characters")
			}

			if len(options.name) > 100 {
				pfxlog.Logger().Fatal("Name must be 100 or less characters")
			}

			if err := validateUsernameLength(options.username); err != nil {
				pfxlog.Logger().Fatal(err)
			}

			if err := validatePasswordLength(options.password); err != nil {
				pfxlog.Logger().Fatal(err)
			}

			if err := ctrl.AppEnv.Managers.Identity.InitializeDefaultAdmin(options.username, options.password, options.name); err != nil {
				pfxlog.Logger().Fatal(err)
			}
			pfxlog.Logger().Info("Ziti Edge initialization complete")
		},
	}

	cmd.Flags().StringVarP(&options.password, "password", "p", "", "the admin password value to initialize with, prompted for if not supplied")
	cmd.Flags().StringVarP(&options.username, "username", "u", "", "the admin username value to initialize with, prompted for if not supplied")
	cmd.Flags().StringVarP(&options.name, "name", "n", "Default Admin", "the admin display name to initialize with, defaults to 'Default Admin'")

	return cmd
}

func validateUsernameLength(username string) error {
	length := len(username)

	if length > maxUsernameLength {
		return errors.New("username must be " + strconv.Itoa(maxUsernameLength) + " or less characters")
	}

	if length < minUsernameLength {
		return errors.New("username must be " + strconv.Itoa(minUsernameLength) + " or more characters")
	}

	return nil
}

func validatePasswordLength(password string) error {
	length := len(password)

	if length > maxPasswordLength {
		return errors.New("password must be " + strconv.Itoa(maxPasswordLength) + " or less characters")
	}

	if length < minPasswordLength {
		return errors.New("password must be " + strconv.Itoa(minPasswordLength) + " or more characters")
	}

	return nil
}

func configureController(configPath string, versionProvider common.VersionProvider) *server.Controller {
	config, err := controller.LoadConfig(configPath)

	if err != nil {
		pfxlog.Logger().WithError(err).Fatalf("could not read configuration file [%s]", configPath)
	}

	var fabricController *controller.Controller
	if fabricController, err = controller.NewController(config, versionProvider); err != nil {
		panic(err)
	}

	edgeController, err := server.NewController(config, fabricController)

	if err != nil {
		panic(err)
	}

	edgeController.SetHostController(fabricController)
	edgeController.Initialize()

	return edgeController
}

func promptUsername() string {
	username := ""
	for username == "" {
		var err error
		username, err = term.Prompt("Provide an admin username: ")

		if err != nil {
			pfxlog.Logger().WithError(err).Fatal("could not request or did not receive the username from shell")
		}

		username = strings.TrimSpace(username)

		if err = validateUsernameLength(username); err != nil {
			username = ""
			println(err.Error())
		}
	}

	return username
}

func promptPassword() string {
	var err error
	var password string

	for password == "" {
		password, err = term.PromptPassword("Enter the admin password: ", false)

		if err != nil {
			pfxlog.Logger().WithError(err).Fatal("could not request or did not receive the password from shell")
		}

		password = strings.TrimSpace(password)

		if err = validatePasswordLength(password); err != nil {
			password = ""
			println(err.Error())
			continue
		}

		confirmedPassword, err := term.PromptPassword("Confirm the admin password: ", false)

		if err != nil {
			pfxlog.Logger().WithError(err).Fatal("could not request or did not confirmed the password from shell")
		}

		if confirmedPassword != password {
			pfxlog.Logger().Fatal("passwords did not match")
		}
	}

	return password
}
