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

package edge_controller

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/foundation/util/term"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"
	"strings"
)

// loginOptions are the flags for login commands
type loginOptions struct {
	commonOptions
	Username string
	Password string
	Cert     string
}

// newLoginCmd creates the command
func newLoginCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &loginOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "login my.controller.hostname[:port]",
		Short: "logs into a Ziti Edge Controller instance",
		Long:  `login allows the ziti command to establish a session with a Ziti Edge Controller, allowing more commands to be run against the controller.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.Username, "username", "u", "", "username to use for authenticating to the Ziti Edge Controller ")
	if err := cmd.MarkFlagRequired("username"); err != nil {
		panic(err)
	}

	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "password to use for authenticating to the Ziti Edge Controller, if -u is supplied and -p is not, a value will be prompted for")
	cmd.Flags().StringVarP(&options.Cert, "cert", "c", "", "additional root certificates used by the Ziti Edge Controller")
	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements this command
func (o *loginOptions) Run() error {
	host := o.Args[0]

	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	if o.Username != "" && o.Password == "" {
		var err error
		if o.Password, err = term.PromptPassword("Enter password: ", false); err != nil {
			return err
		}
	}
	container := gabs.New()
	_, _ = container.SetP(o.Username, "username")
	_, _ = container.SetP(o.Password, "password")

	body := container.String()

	jsonParsed, err := util.EdgeControllerLogin(host, o.Cert, body, o.Out, o.OutputJSONResponse, o.commonOptions.Timeout, o.commonOptions.Verbose)

	if err != nil {
		return err
	}

	if !jsonParsed.ExistsP("data.token") {
		return fmt.Errorf("no session token returned from login request to %v. Received: %v", host, jsonParsed.String())
	}

	token, ok := jsonParsed.Path("data.token").Data().(string)

	if !ok {
		return fmt.Errorf("session token returned from login request to %v is not in the expected format. Received: %v", host, jsonParsed.String())
	}

	if !o.OutputJSONResponse {
		fmt.Printf("Token: %v\n", token)
	}

	absCertPath, err := filepath.Abs(o.Cert)
	if err == nil {
		o.Cert = absCertPath
	}

	session := &util.Session{
		Host:  host,
		Token: token,
		Cert:  o.Cert,
	}

	err = session.Persist()

	return err
}
