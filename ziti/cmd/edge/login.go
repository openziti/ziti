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

package edge

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/foundation/v2/term"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// loginOptions are the flags for login commands
type loginOptions struct {
	api.Options
	Username     string
	Password     string
	Token        string
	CaCert       string
	ReadOnly     bool
	Yes          bool
	IgnoreConfig bool
}

// newLoginCmd creates the command
func newLoginCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &loginOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "login my.controller.hostname[:port]/path",
		Short: "logs into a Ziti Edge Controller instance",
		Long:  `login allows the ziti command to establish a session with a Ziti Edge Controller, allowing more commands to be run against the controller.`,
		Args:  cobra.RangeArgs(0, 1),
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
	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "password to use for authenticating to the Ziti Edge Controller, if -u is supplied and -p is not, a value will be prompted for")
	cmd.Flags().StringVarP(&options.Token, "token", "t", "", "if an api token has already been acquired, it can be set in the config with this option. This will set the session to read only by default")
	cmd.Flags().StringVarP(&options.CaCert, "cert", "c", "", "additional root certificates used by the Ziti Edge Controller")
	cmd.Flags().BoolVar(&options.ReadOnly, "read-only", false, "marks this login as read-only. Note: this is not a guarantee that nothing can be changed on the server. Care should still be taken!")
	cmd.Flags().BoolVarP(&options.Yes, "yes", "y", false, "If set, responds to prompts with yes. This will result in untrusted certs being accepted or updated.")
	cmd.Flags().BoolVar(&options.IgnoreConfig, "ignore-config", false, "If set, does not use value from the config file for hostname or username. Values must be entered or will be prompted for.")
	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements this command
func (o *loginOptions) Run() error {
	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	id := config.GetIdentity()

	var host string
	if len(o.Args) == 0 {
		if defaultId := config.EdgeIdentities[id]; defaultId != nil && !o.IgnoreConfig {
			host = defaultId.Url
			o.Printf("Using controller url: %v from identity '%v' in config file: %v\n", host, id, configFile)
		} else {
			if host, err = term.Prompt("Enter controller host[:port] (default localhost:1280): "); err != nil {
				return err
			}
			if host == "" {
				host = "localhost:1280"
			}
		}
	} else {
		host = o.Args[0]
	}

	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	ctrlUrl, err := url.Parse(host)
	if err != nil {
		return errors.Wrap(err, "invalid controller URL")
	}

	host = ctrlUrl.Scheme + "://" + ctrlUrl.Host

	if err = o.ConfigureCerts(host, ctrlUrl); err != nil {
		return err
	}

	if certAbs, err := filepath.Abs(o.CaCert); err == nil {
		o.CaCert = certAbs
	}

	if ctrlUrl.Path == "" {
		host = util.EdgeControllerGetManagementApiBasePath(host, o.CaCert)
	} else {
		host = host + ctrlUrl.Path
	}

	if o.Token != "" && !o.Cmd.Flag("read-only").Changed {
		o.ReadOnly = true
		o.Println("NOTE: When using --token the saved identity will be marked as read-only unless --read-only=false is provided")
	}

	if o.Token == "" {
		for o.Username == "" {
			if defaultId := config.EdgeIdentities[id]; defaultId != nil && defaultId.Username != "" && !o.IgnoreConfig {
				o.Username = defaultId.Username
				o.Printf("Using username: %v from identity '%v' in config file: %v\n", o.Username, id, configFile)
			} else if o.Username, err = term.Prompt("Enter username: "); err != nil {
				return err
			}
		}

		if o.Password == "" {
			if o.Password, err = term.PromptPassword("Enter password: ", false); err != nil {
				return err
			}
		}

		container := gabs.New()
		_, _ = container.SetP(o.Username, "username")
		_, _ = container.SetP(o.Password, "password")

		body := container.String()

		jsonParsed, err := util.EdgeControllerLogin(host, o.CaCert, body, o.Out, o.OutputJSONResponse, o.Options.Timeout, o.Options.Verbose)

		if err != nil {
			return err
		}

		if !jsonParsed.ExistsP("data.token") {
			return fmt.Errorf("no session token returned from login request to %v. Received: %v", host, jsonParsed.String())
		}

		var ok bool
		o.Token, ok = jsonParsed.Path("data.token").Data().(string)

		if !ok {
			return fmt.Errorf("session token returned from login request to %v is not in the expected format. Received: %v", host, jsonParsed.String())
		}

		if !o.OutputJSONResponse {
			o.Printf("Token: %v\n", o.Token)
		}
	}

	absCertPath, err := filepath.Abs(o.CaCert)
	if err == nil {
		o.CaCert = absCertPath
	}

	loginIdentity := &util.RestClientEdgeIdentity{
		Url:       host,
		Username:  o.Username,
		Token:     o.Token,
		LoginTime: time.Now().Format(time.RFC3339),
		CaCert:    o.CaCert,
		ReadOnly:  o.ReadOnly,
	}

	o.Printf("Saving identity '%v' to %v\n", id, configFile)
	config.EdgeIdentities[id] = loginIdentity

	err = util.PersistRestClientConfig(config)

	return err
}

func (o *loginOptions) ConfigureCerts(host string, ctrlUrl *url.URL) error {
	isServerTrusted, err := util.IsServerTrusted(host)
	if err != nil {
		return err
	}

	if !isServerTrusted && o.CaCert == "" {
		wellKnownCerts, certs, err := util.GetWellKnownCerts(host)
		if err != nil {
			return errors.Wrapf(err, "unable to retrieve server certificate authority from %v", host)
		}

		certsTrusted, err := util.AreCertsTrusted(host, wellKnownCerts)
		if err != nil {
			return err
		}
		if !certsTrusted {
			return errors.New("server supplied certs not trusted by server, unable to continue")
		}

		savedCerts, certFile, err := util.ReadCert(ctrlUrl.Hostname())
		if err != nil {
			return err
		}

		if savedCerts != nil {
			o.CaCert = certFile
			if !util.AreCertsSame(o, wellKnownCerts, savedCerts) {
				o.Printf("WARNING: server supplied certificate authority doesn't match cached certs at %v\n", certFile)
				replace := o.Yes
				if !replace {
					if replace, err = o.askYesNo("Replace cached certs [Y/N]: "); err != nil {
						return err
					}
				}
				if replace {
					_, err = util.WriteCert(o, ctrlUrl.Hostname(), wellKnownCerts)
					if err != nil {
						return err
					}
				}
			}
		} else {
			o.Printf("Untrusted certificate authority retrieved from server\n")
			o.Println("Verified that server supplied certificates are trusted by server")
			o.Printf("Server supplied %v certificates\n", len(certs))
			importCerts := o.Yes
			if !importCerts {
				if importCerts, err = o.askYesNo("Trust server provided certificate authority [Y/N]: "); err != nil {
					return err
				}
			}
			if importCerts {
				o.CaCert, err = util.WriteCert(o, ctrlUrl.Hostname(), wellKnownCerts)
				if err != nil {
					return err
				}
			} else {
				o.Println("WARNING: no certificate authority provided for server, continuing but login will likely fail")
			}
		}
	} else if isServerTrusted && o.CaCert != "" {
		override, err := o.askYesNo("Server certificate authority is already trusted. Are you sure you want to provide an additional CA [Y/N]: ")
		if err != nil {
			return err
		}
		if !override {
			o.CaCert = ""
		}
	}

	return nil
}

func (o *loginOptions) askYesNo(prompt string) (bool, error) {
	filter := &yesNoFilter{}
	if _, err := o.ask(prompt, filter.Accept); err != nil {
		return false, err
	}
	return filter.result, nil
}

func (o *loginOptions) ask(prompt string, f func(string) bool) (string, error) {
	for {
		val, err := term.Prompt(prompt)
		if err != nil {
			return "", err
		}
		val = strings.TrimSpace(val)
		if f(val) {
			return val, nil
		}
		o.Printf("Invalid input: %v\n", val)
	}
}

type yesNoFilter struct {
	result bool
}

func (self *yesNoFilter) Accept(s string) bool {
	if strings.EqualFold("y", s) || strings.EqualFold("yes", s) {
		self.result = true
		return true
	}

	if strings.EqualFold("n", s) || strings.EqualFold("no", s) {
		self.result = false
		return true
	}

	return false
}
