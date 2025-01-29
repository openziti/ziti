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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_client"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/foundation/v2/term"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	ziticobra "github.com/openziti/ziti/internal/cobra"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LoginOptions are the flags for login commands
type LoginOptions struct {
	api.Options
	Username      string
	Password      string
	Token         string
	CaCert        string
	ReadOnly      bool
	Yes           bool
	IgnoreConfig  bool
	ClientCert    string
	ClientKey     string
	ExtJwt        string
	File          string
	ControllerUrl string

	FileCertCreds *edge_apis.IdentityCredentials
}

const LoginFlagKey = "login"

func addLoginAnnotation(cmd *cobra.Command, flagName string) {
	_ = ziticobra.AddFlagAnnotation(cmd, flagName, LoginFlagKey, "true")
}

func AddLoginFlags(cmd *cobra.Command, options *LoginOptions) {
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.Username, "username", "u", "", "username to use for authenticating to the Ziti Edge Controller ")
	addLoginAnnotation(cmd, "username")
	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "password to use for authenticating to the Ziti Edge Controller, if -u is supplied and -p is not, a value will be prompted for")
	addLoginAnnotation(cmd, "password")
	cmd.Flags().StringVarP(&options.Token, "token", "t", "", "if an api token has already been acquired, it can be set in the config with this option. This will set the session to read only by default")
	addLoginAnnotation(cmd, "token")
	cmd.Flags().StringVarP(&options.CaCert, "ca", "", "", "additional root certificates used by the Ziti Edge Controller")
	addLoginAnnotation(cmd, "ca")
	cmd.Flags().BoolVar(&options.ReadOnly, "read-only", false, "marks this login as read-only. Note: this is not a guarantee that nothing can be changed on the server. Care should still be taken!")
	addLoginAnnotation(cmd, "read-only")
	cmd.Flags().BoolVarP(&options.Yes, "yes", "y", false, "If set, responds to prompts with yes. This will result in untrusted certs being accepted or updated.")
	addLoginAnnotation(cmd, "yes")
	cmd.Flags().BoolVar(&options.IgnoreConfig, "ignore-config", false, "If set, does not use values from nor write the config file. Required values not specified will be prompted for.")
	addLoginAnnotation(cmd, "ignore-config")
	cmd.Flags().StringVarP(&options.ClientCert, "client-cert", "c", "", "A certificate used to authenticate")
	addLoginAnnotation(cmd, "client-cert")
	cmd.Flags().StringVarP(&options.ClientKey, "client-key", "k", "", "The key to use with certificate authentication")
	addLoginAnnotation(cmd, "client-key")
	cmd.Flags().StringVarP(&options.ExtJwt, "ext-jwt", "e", "", "A file containing a JWT from an external provider to be used for authentication")
	addLoginAnnotation(cmd, "ext-jwt")
	cmd.Flags().StringVarP(&options.File, "file", "f", "", "An identity file to use for authentication")
	addLoginAnnotation(cmd, "file")

	options.AddCommonFlags(cmd)
}

// NewLoginCmd creates the command
func NewLoginCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &LoginOptions{
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
			if len(args) > 0 {
				options.ControllerUrl = args[0]
			}
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	AddLoginFlags(cmd, options)

	return cmd
}

func (o *LoginOptions) newHttpClient() *http.Client {
	if o.ControllerUrl != "" && o.Args == nil || len(o.Args) < 1 {
		o.Args = []string{o.ControllerUrl}
	}
	// any error indicates there are probably no saved credentials. look for login information and use those
	loginErr := o.Run()
	if loginErr != nil {
		pfxlog.Logger().Fatal(loginErr)
	}

	caPool := x509.NewCertPool()
	if _, cacertErr := os.Stat(o.CaCert); cacertErr == nil {
		rootPemData, err := os.ReadFile(o.CaCert)
		if err != nil {
			pfxlog.Logger().Fatalf("error reading CA cert [%s]", o.CaCert)
		}
		caPool.AppendCertsFromPEM(rootPemData)
	} else {
		pfxlog.Logger().Warnf("CA cert not found [%s]", o.CaCert)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caPool,
			},
		},
	}
}

// NewClientApiClient returns a new management client for use with the controller using the set of login material provided
func (o *LoginOptions) NewClientApiClient() (*rest_client_api_client.ZitiEdgeClient, error) {
	httpClient := o.newHttpClient()

	c, e := rest_util.NewEdgeClientClientWithToken(httpClient, o.ControllerUrl, o.Token)
	if e != nil {
		pfxlog.Logger().Fatal(e)
	}
	return c, nil
}

// NewMgmtClient returns a new management client for use with the controller using the set of login material provided
func (o *LoginOptions) NewMgmtClient() (*rest_management_api_client.ZitiEdgeManagement, error) {
	httpClient := o.newHttpClient()

	c, e := rest_util.NewEdgeManagementClientWithToken(httpClient, o.ControllerUrl, o.Token)
	if e != nil {
		pfxlog.Logger().Fatal(e)
	}
	return c, nil
}

// Run implements this command
func (o *LoginOptions) Run() error {
	var host string

	config, configFile, cfgErr := util.LoadRestClientConfig()
	if cfgErr != nil {
		return cfgErr
	}

	if o.File != "" {
		cfg, err := ziti.NewConfigFromFile(o.File)

		if err != nil {
			return fmt.Errorf("could not read file %s: %w", o.File, err)
		}

		if !o.IgnoreConfig {
			idCredentials := edge_apis.NewIdentityCredentialsFromConfig(cfg.ID)
			o.FileCertCreds = idCredentials
		}
		ztAPI := cfg.ZtAPI

		// override with the first HA client API URL if defined
		if len(cfg.ZtAPIs) > 0 {
			ztAPI = cfg.ZtAPIs[0]
		}

		parsedZtAPI, err := url.Parse(ztAPI)
		if err != nil {
			return fmt.Errorf("could not parse ztAPI '%s' as a URL", ztAPI)
		}

		host = parsedZtAPI.Host
	}

	id := config.GetIdentity()

	if host == "" {
		if o.ControllerUrl == "" {
			if defaultId := config.EdgeIdentities[id]; defaultId != nil && !o.IgnoreConfig {
				host = defaultId.Url
				o.Printf("Using controller url: %v from identity '%v' in config file: %v\n", host, id, configFile)
			} else {
				var err error
				if host, err = term.Prompt("Enter controller host[:port] (default localhost:1280): "); err != nil {
					return err
				}
				if host == "" {
					host = "localhost:1280"
				}
			}
		} else {
			host = o.ControllerUrl
		}
	}

	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	ctrlUrl, urlParseErr := url.Parse(host)
	if urlParseErr != nil {
		return errors.Wrap(urlParseErr, "invalid controller URL")
	}

	host = ctrlUrl.Scheme + "://" + ctrlUrl.Host

	if err := o.ConfigureCerts(host, ctrlUrl); err != nil {
		return err
	}

	if o.CaCert != "" {
		if certAbs, err := filepath.Abs(o.CaCert); err == nil {
			o.CaCert = certAbs
		}
	}

	if ctrlUrl.Path == "" {
		if o.FileCertCreds != nil && o.FileCertCreds.CaPool != nil {
			host = util.EdgeControllerGetManagementApiBasePathWithPool(host, o.FileCertCreds.CaPool)
		} else {
			host = util.EdgeControllerGetManagementApiBasePath(host, o.CaCert)
		}
	} else {
		host = host + ctrlUrl.Path
	}

	if o.Token != "" && o.Cmd != nil && !o.Cmd.Flag("read-only").Changed {
		o.ReadOnly = true
		o.Println("NOTE: When using --token the saved identity will be marked as read-only unless --read-only=false is provided")
	}

	body := "{}"
	if o.Token == "" && o.ClientCert == "" && o.ExtJwt == "" && o.FileCertCreds == nil {
		for o.Username == "" {
			var err error
			if defaultId := config.EdgeIdentities[id]; defaultId != nil && defaultId.Username != "" && !o.IgnoreConfig {
				o.Username = defaultId.Username
				o.Printf("Using username: %v from identity '%v' in config file: %v\n", o.Username, id, configFile)
			} else if o.Username, err = term.Prompt("Enter username: "); err != nil {
				return err
			}
		}

		if o.Password == "" {
			var err error
			if o.Password, err = term.PromptPassword("Enter password: ", false); err != nil {
				return err
			}
		}

		container := gabs.New()
		_, _ = container.SetP(o.Username, "username")
		_, _ = container.SetP(o.Password, "password")

		body = container.String()
	}

	if o.Token == "" {
		jsonParsed, err := login(o, host, body)

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

	o.ControllerUrl = host
	if !o.IgnoreConfig {
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

		return util.PersistRestClientConfig(config)
	}
	return nil
}

func (o *LoginOptions) ConfigureCerts(host string, ctrlUrl *url.URL) error {
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

func (o *LoginOptions) askYesNo(prompt string) (bool, error) {
	filter := &yesNoFilter{}
	if _, err := o.ask(prompt, filter.Accept); err != nil {
		return false, err
	}
	return filter.result, nil
}

func (o *LoginOptions) ask(prompt string, f func(string) bool) (string, error) {
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

// EdgeControllerLogin will authenticate to the given Edge Controller
func login(o *LoginOptions, url string, authentication string) (*gabs.Container, error) {
	client := util.NewClient()
	cert := o.CaCert
	out := o.Out
	logJSON := o.OutputJSONResponse
	timeout := o.Timeout
	verbose := o.Verbose
	method := "password"
	if cert != "" {
		client.SetRootCertificate(cert)
	}
	authHeader := ""
	if o.ExtJwt != "" {
		auth, err := os.ReadFile(o.ExtJwt)
		if err != nil {
			return nil, fmt.Errorf("couldn't load jwt file at %s: %v", o.ExtJwt, err)
		}
		method = "ext-jwt"
		authHeader = "Bearer " + strings.TrimSpace(string(auth))
		client.SetHeader("Authorization", authHeader)
	} else {
		if o.ClientCert != "" {
			clientCert, err := tls.LoadX509KeyPair(o.ClientCert, o.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("can't load client certificate: %s with key %s: %v", o.ClientCert, o.ClientKey, err)
			}
			client.SetCertificates(clientCert)
			method = "cert"
		} else if o.FileCertCreds != nil {
			tlsCert := o.FileCertCreds.TlsCerts()[0]
			client.SetCertificates(tlsCert)
			method = "cert"
		}
	}

	resp, err := client.
		SetTimeout(time.Duration(timeout)*time.Second).
		SetDebug(verbose).
		R().
		SetQueryParam("method", method).
		SetHeader("Content-Type", "application/json").
		SetBody(authentication).
		Post(url + "/authenticate")

	if err != nil {
		return nil, fmt.Errorf("unable to authenticate to %v. Error: %v", url, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unable to authenticate to %v. Status code: %v, Server returned: %v", url, resp.Status(), util.PrettyPrintResponse(resp))
	}

	if logJSON {
		util.OutputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())
	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", url, resp.String())
	}

	return jsonParsed, nil
}
