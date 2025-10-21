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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	xterm "golang.org/x/term"
	"gopkg.in/resty.v1"
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
	extJwtFile    string
	ExtJwtToken   string
	File          string
	ControllerUrl string
	ServiceName   string
	NetworkId     string

	FileCertCreds *edge_apis.IdentityCredentials
	client        http.Client
}

func (options *LoginOptions) GetClient() http.Client {
	return options.client
}
func (options *LoginOptions) SetClient(c http.Client) {
	options.client = c
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
	cmd.Flags().StringVarP(&options.extJwtFile, "ext-jwt", "e", "", "A file containing a JWT from an external provider to be used for authentication")
	addLoginAnnotation(cmd, "ext-jwt")
	cmd.Flags().StringVarP(&options.File, "file", "f", "", "An identity file to use for authentication")
	addLoginAnnotation(cmd, "file")
	cmd.Flags().StringVarP(&options.ServiceName, "service", "s", "", "The service name to use. When set the file will be used to create a zitified connection")
	addLoginAnnotation(cmd, "service")
	cmd.Flags().StringVarP(&options.NetworkId, "networkIdentity", "n", "", "The identity to use to connect to the OpenZiti overlay")
	addLoginAnnotation(cmd, "networkIdentity")

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

			if options.extJwtFile != "" {
				auth, err := os.ReadFile(options.extJwtFile)
				if err != nil {
					pfxlog.Logger().Fatal(err)
				}
				options.ExtJwtToken = string(auth)
			}
			cmdhelper.CheckErr(options.Run())
		},
		SuggestFor: []string{},
	}

	AddLoginFlags(cmd, options)

	return cmd
}

func (o *LoginOptions) newHttpClient(tryCachedCreds bool) *http.Client {
	if o.ControllerUrl != "" && o.Args == nil || len(o.Args) < 1 {
		o.Args = []string{o.ControllerUrl}
	}

	if tryCachedCreds {
		// any error indicates there are probably no saved credentials. look for login information and use those
		o.PopulateFromCache()
		loginErr := o.Run()
		if loginErr != nil {
			pfxlog.Logger().Fatal(loginErr)
		}
	}

	caPool := x509.NewCertPool()
	if o.CaCert != "" {
		if _, cacertErr := os.Stat(o.CaCert); cacertErr == nil {
			rootPemData, err := os.ReadFile(o.CaCert)
			if err != nil {
				pfxlog.Logger().Fatalf("error reading CA cert [%s]", o.CaCert)
			}
			caPool.AppendCertsFromPEM(rootPemData)
		} else {
			pfxlog.Logger().Warnf("CA cert not found [%s]", o.CaCert)
		}
	}

	transportToUse := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caPool,
		},
	}

	t, cte := o.createHttpTransport()
	if cte != nil {
		return &http.Client{
			Transport: transportToUse,
		}
	} else {
		hc := &http.Client{}
		if t != nil {
			t.TLSClientConfig.RootCAs = caPool
			transportToUse = t
		}

		hc.Transport = transportToUse
		return hc
	}
}

// NewClientApiClient returns a new management client for use with the controller using the set of login material provided
func (o *LoginOptions) NewClientApiClient() (*rest_client_api_client.ZitiEdgeClient, error) {
	httpClient := o.newHttpClient(true)

	c, e := rest_util.NewEdgeClientClientWithToken(httpClient, o.ControllerUrl, o.Token)
	if e != nil {
		pfxlog.Logger().Fatal(e)
	}
	return c, nil
}

// NewMgmtClient returns a new management client for use with the controller using the set of login material provided
func (o *LoginOptions) NewMgmtClient() (*rest_management_api_client.ZitiEdgeManagement, error) {
	httpClient := o.newHttpClient(true)

	c, e := rest_util.NewEdgeManagementClientWithToken(httpClient, o.ControllerUrl, o.Token)
	if e != nil {
		pfxlog.Logger().Fatal(e)
	}
	return c, nil
}

func createZitifiedHttpClient(cfg *ziti.Config) (*http.Client, error) {
	zitiContext, err := ziti.NewContext(cfg)
	if err != nil {
		panic(err)
	}

	zitiContexts := ziti.NewSdkCollection()
	zitiContexts.Add(zitiContext)

	zitiTransport := http.DefaultTransport.(*http.Transport).Clone() // copy default transport
	zitiTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := zitiContexts.NewDialer()
		return dialer.Dial(network, addr)
	}

	_, se := zitiContext.GetServices()
	if se != nil {
		return nil, se
	}
	return &http.Client{Transport: zitiTransport}, nil
	//return &http.Client{Transport: createZitifiedHttpTransport(zitiContexts)}, nil
}

// Run implements this command
func (o *LoginOptions) Run() error {
	var host string

	config, configFile, cfgErr := util.LoadRestClientConfig()
	if cfgErr != nil {
		return cfgErr
	}

	/*
		// before -- working
		var httpClient *http.Client
		if o.NetworkId != "" {
			cfg, ce := ziti.NewConfigFromFile(o.NetworkId)
			if ce != nil {
				return ce
			}
			cfg.ConfigTypes = append(cfg.ConfigTypes, "all")
			c, cze := createZitifiedHttpClient(cfg)
			if cze != nil {
				return cze
			}
			o.SetClient(*c)
			httpClient = c
		} else {
			httpClient = &http.Client{}
		}
	*/

	httpClient := o.newHttpClient(false)
	o.SetClient(*httpClient)

	if o.File != "" {
		cfg, err := ziti.NewConfigFromFile(o.File)
		if err != nil {
			return fmt.Errorf("could not read file %s: %w", o.File, err)
		}

		idCredentials := edge_apis.NewIdentityCredentialsFromConfig(cfg.ID)
		o.FileCertCreds = idCredentials

		ztAPI := cfg.ZtAPI

		// override with the first HA client API URL if defined
		if len(cfg.ZtAPIs) > 0 {
			ztAPI = cfg.ZtAPIs[0]
		}

		parsedZtAPI, err := url.Parse(ztAPI)
		if err != nil {
			return fmt.Errorf("could not parse ztAPI '%s' as a URL", ztAPI)
		}

		if o.ControllerUrl == "" {
			host = parsedZtAPI.Host
		} else {
			host = o.ControllerUrl
		}
	}

	id := config.GetIdentity()

	if host == "" {
		if o.ControllerUrl == "" {
			if cachedCliConfig := config.EdgeIdentities[id]; cachedCliConfig != nil && !o.IgnoreConfig {
				host = cachedCliConfig.Url
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
			host = util.EdgeControllerGetManagementApiBasePathWithPool(host, o.FileCertCreds.CaPool, httpClient)
		} else {
			host = util.EdgeControllerGetManagementApiBasePath(host, o.CaCert, httpClient)
		}
	} else {
		host = host + ctrlUrl.Path
	}

	if o.Token != "" && o.Cmd != nil && !o.Cmd.Flag("read-only").Changed {
		o.ReadOnly = true
		o.Println("NOTE: When using --token the saved identity will be marked as read-only unless --read-only=false is provided")
	}

	body := "{}"
	if o.Token == "" && o.ClientCert == "" && o.ExtJwtToken == "" && o.FileCertCreds == nil {
		for o.Username == "" {
			if xterm.IsTerminal(int(os.Stdin.Fd())) {
				var err error
				if cachedCliConfig := config.EdgeIdentities[id]; cachedCliConfig != nil && cachedCliConfig.Username != "" && !o.IgnoreConfig {
					o.Username = cachedCliConfig.Username
					o.Printf("Using username: %v from identity '%v' in config file: %v\n", o.Username, id, configFile)
				} else if o.Username, err = term.Prompt("Enter username: "); err != nil {
					return err
				}
			} else {
				return errors.New("username required but not provided")
			}
		}

		if o.Password == "" {
			var err error
			if xterm.IsTerminal(int(os.Stdin.Fd())) {
				if o.Password, err = term.PromptPassword("Enter password: ", false); err != nil {
					return err
				}
			} else {
				return errors.New("password required but not provided")
			}
		}

		container := gabs.New()
		_, _ = container.SetP(o.Username, "username")
		_, _ = container.SetP(o.Password, "password")

		body = container.String()
	}

	if o.Token == "" {
		hc := o.GetClient()
		jsonParsed, err := login(o, host, body, &hc)

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
			Url:           host,
			Username:      o.Username,
			Token:         o.Token,
			LoginTime:     time.Now().Format(time.RFC3339),
			CaCert:        o.CaCert,
			ReadOnly:      o.ReadOnly,
			NetworkIdFile: o.NetworkId,
		}
		o.Printf("Saving identity '%v' to %v\n", id, configFile)
		config.EdgeIdentities[id] = loginIdentity

		return util.PersistRestClientConfig(config)
	}
	return nil
}

func (o *LoginOptions) ConfigureCerts(host string, ctrlUrl *url.URL) error {
	httpClient := o.GetClient()
	isServerTrusted, err := util.IsServerTrusted(host, &httpClient)
	if err != nil {
		return err
	}

	if !isServerTrusted && o.CaCert == "" {
		wellKnownCerts, certs, err := util.GetWellKnownCerts(host, httpClient)
		if err != nil {
			return errors.Wrapf(err, "unable to retrieve server certificate authority from %v", host)
		}

		certsTrusted, err := util.AreCertsTrusted(host, wellKnownCerts, httpClient)
		if err != nil {
			return err
		}
		if !certsTrusted {
			return errors.New("server supplied certs not trusted by server, unable to continue")
		}

		savedCerts, certFile, err := util.ReadCert(ctrlUrl)
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
					_, err = util.WriteCert(o, ctrlUrl, wellKnownCerts)
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
				o.CaCert, err = util.WriteCert(o, ctrlUrl, wellKnownCerts)
				if err != nil {
					return err
				}
			} else {
				o.Println("WARNING: no certificate authority provided for server, continuing but login will likely fail")
			}
		}
	}

	return nil
}

func (o *LoginOptions) askYesNo(prompt string) (bool, error) {
	if o.Yes {
		return true, nil
	}
	filter := &yesNoFilter{}
	if _, err := o.ask(prompt, filter.Accept); err != nil {
		return false, err
	}
	return filter.result, nil
}

func (o *LoginOptions) ask(prompt string, f func(string) bool) (string, error) {
	if o.Yes {
		return "yes", nil
	}

	if !xterm.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("Cannot accept certs - no terminal")
	}

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
func login(o *LoginOptions, url string, authentication string, httpClient *http.Client) (*gabs.Container, error) {
	var client *resty.Client
	if httpClient != nil {
		client = util.NewClientWithClient(httpClient)
	} else {
		client = util.NewClient()
	}
	cert := o.CaCert
	out := o.Out
	logJSON := o.OutputJSONResponse
	timeout := o.Timeout
	verbose := o.Verbose
	method := "password"
	if cert != "" {
		client.SetRootCertificate(cert)
		transport := client.GetClient().Transport.(*http.Transport)
		transport.CloseIdleConnections() // make sure any idle connections are closed so the client hello happens WITH certs
	}
	authHeader := ""
	if o.ExtJwtToken != "" {
		method = "ext-jwt"
		authHeader = "Bearer " + strings.TrimSpace(o.ExtJwtToken)
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

func createZitifiedHttpTransport(ctxCollection *ziti.CtxCollection) *http.Transport {
	zitiTransport := http.DefaultTransport.(*http.Transport).Clone() // copy default transport
	zitiTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := ctxCollection.NewDialerWithFallback(context.Background(), &net.Dialer{})
		return dialer.Dial(network, addr)
	}
	return zitiTransport
}

func (o *LoginOptions) createHttpTransport() (*http.Transport, error) {
	// if cli param supplied - use it
	if o.NetworkId != "" {
		return o.createHttpTransportFromFile()
	}

	// if env var set - use it
	if zt, zte := util.ZitifiedTransportFromEnv(); zte != nil {
		return nil, zte
	} else {
		if zt != nil {
			o.Printf("NetworkId found by env var [%s], zitified transport enabled\n", constants.ZitiCliNetworkIdVarName)
			o.NetworkId = ""
			return zt, nil
		}
	}

	// if cached id exists use it
	config, _, cfgErr := util.LoadRestClientConfig()
	if cfgErr != nil {
		return nil, cfgErr
	}
	id := config.GetIdentity()
	cachedCliConfig := config.EdgeIdentities[id]

	if cachedCliConfig != nil {
		// -- if cached id has networkId referenced - use it
		if cachedCliConfig.NetworkIdFile != "" {
			o.NetworkId = cachedCliConfig.NetworkIdFile
			return o.createHttpTransportFromFile()
		}
	}
	return nil, nil
}

func (o *LoginOptions) createHttpTransportFromFile() (*http.Transport, error) {
	cfg, ce := ziti.NewConfigFromFile(o.NetworkId)
	if ce != nil {
		return nil, ce
	}
	cfg.ConfigTypes = append(cfg.ConfigTypes, "all")

	zc, zce := ziti.NewContext(cfg)
	if zce != nil {
		return nil, zce
	}
	sdkCollection := ziti.NewSdkCollection()
	sdkCollection.Add(zc)
	_, se := zc.GetServices() // loads all the services
	if se != nil {
		return nil, fmt.Errorf("failed to get ziti services: %v", se)
	}
	return createZitifiedHttpTransport(sdkCollection), nil
}

func (o *LoginOptions) PopulateFromCache() {
	config, _, cfgErr := util.LoadRestClientConfig()
	if cfgErr != nil {
		return
	}

	id := config.GetIdentity()
	cachedCliConfig := config.EdgeIdentities[id]
	if cachedCliConfig == nil {
		return
	}

	o.ControllerUrl = cachedCliConfig.Url
	o.Username = cachedCliConfig.Username
	o.Token = cachedCliConfig.Token
	o.NetworkId = cachedCliConfig.NetworkIdFile
	o.CaCert = cachedCliConfig.CaCert
}

func NewFromCache(out io.Writer, eout io.Writer) LoginOptions {
	o := LoginOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: eout,
			},
		},
	}
	o.PopulateFromCache()
	return o
}

func TryCachedCredsLogin(out io.Writer, eout io.Writer) (LoginOptions, error) {
	o := NewFromCache(out, eout)
	err := o.Run()
	if err != nil {
		return o, err
	} else {
		return o, nil
	}
}
