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
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_client"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/foundation/v2/term"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	ziticobra "github.com/openziti/ziti/internal/cobra"
	"github.com/openziti/ziti/internal/jwtutil"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	xterm "golang.org/x/term"
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
	ApiSession    edge_apis.ApiSession
	TotpCallback  func(strings chan string)

	client     http.Client
	transport  *http.Transport
	caPool     *x509.CertPool
	cachedId   *util.RestClientEdgeIdentity
	mgmtClient *edge_apis.ManagementApiClient
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
	cmd.Flags().StringVarP(&options.NetworkId, "network-identity", "n", "", "The identity to use to connect to the OpenZiti overlay")
	addLoginAnnotation(cmd, "network-identity")

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
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			if len(args) > 0 {
				options.ControllerUrl = args[0]
			}

			if options.extJwtFile != "" {
				auth, err := os.ReadFile(options.extJwtFile)
				if err != nil {
					return err
				}
				options.ExtJwtToken = string(auth)
			}
			return options.Run()
		},
		SuggestFor: []string{},
	}

	AddLoginFlags(cmd, options)

	return cmd
}

func (o *LoginOptions) newHttpClient(tryCachedCreds bool) (http.Client, error) {
	if o.ControllerUrl != "" && o.Args == nil || len(o.Args) < 1 {
		o.Args = []string{o.ControllerUrl}
	}

	if tryCachedCreds {
		// any error indicates there are probably no saved credentials. look for login information and use those
		cached := *o
		cached.PopulateFromCache()
		cached.IgnoreConfig = true // don't overwrite when trying to login
		loginErr := cached.Run()
		if loginErr != nil {
			return http.Client{}, loginErr
		}
	}
	t, cte := o.createHttpTransport()
	if cte != nil {
		return http.Client{}, cte
	}
	c := http.Client{
		Transport: t,
	}
	return c, nil
}

// NewClientApiClient returns a new management client for use with the controller using the set of login material provided
func (o *LoginOptions) NewClientApiClient() (*rest_client_api_client.ZitiEdgeClient, error) {
	nc, newClientErr := o.newHttpClient(true)
	if newClientErr != nil {
		return nil, newClientErr
	}

	return rest_util.NewEdgeClientClientWithToken(&nc, o.ControllerUrl, o.Token)
}

// Run implements this command
func (o *LoginOptions) Run() error {
	var host string

	config, configFile, cfgErr := util.LoadRestClientConfig()
	if cfgErr != nil {
		return cfgErr
	}

	httpClient, newClientErr := o.newHttpClient(false)
	if newClientErr != nil {
		return newClientErr
	}
	o.client = httpClient

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

	host = addHttpsIfNeeded(host)

	ctrlUrl, urlParseErr := url.Parse(host)
	if urlParseErr != nil {
		return errors.New("invalid controller URL supplied")
	}

	if ctrlUrl.Host == "" {
		return errors.New("invalid controller URL supplied")
	}

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
			host = util.EdgeControllerGetManagementApiBasePathWithPool(host, o.FileCertCreds.CaPool, &httpClient)
		} else {
			host = util.EdgeControllerGetManagementApiBasePath(host, o.CaCert, &httpClient)
		}
		hostUrl, _ := url.Parse(host)
		o.ControllerUrl = o.ControllerUrl + hostUrl.Path
	}

	if o.Token != "" && o.Cmd != nil && !o.Cmd.Flag("read-only").Changed {
		o.ReadOnly = true
		o.Println("NOTE: When using --token the saved identity will be marked as read-only unless --read-only=false is provided")
	}

	dontHaveApiSession := !(o.ApiSession != nil && len(o.ApiSession.GetToken()) != 0)
	if dontHaveApiSession && o.Token == "" && o.ClientCert == "" && o.ExtJwtToken == "" && o.FileCertCreds == nil {
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
	}

	caPool, caPoolErr := o.GetCaPool()
	if caPoolErr != nil {
		return caPoolErr
	} else {
		o.caPool = caPool
	}

	t, e := o.createHttpTransport()
	if e != nil {
		return e
	} else {
		o.transport = t
		nc, ncErr := o.newHttpClient(false)
		if ncErr != nil {
			return ncErr
		}
		o.client = nc
	}

	o.ControllerUrl = host

	if o.Token == "" || dontHaveApiSession {
		// if no token or api session, need to log in
		adminClient, newMgmtClientErr := o.NewManagementClient(false)
		if newMgmtClientErr != nil {
			return newMgmtClientErr
		}
		s, le := o.Login()
		if le != nil {
			return le
		}
		if s == nil {
			return fmt.Errorf("failed to login")
		} else {
			o.ApiSession = s
		}
		o.mgmtClient = adminClient
	}

	var sess *edge_apis.ApiSessionJsonWrapper
	if o.ApiSession != nil {
		sess = &edge_apis.ApiSessionJsonWrapper{
			ApiSession: o.ApiSession,
		}
	}
	if !o.IgnoreConfig {
		loginIdentity := &util.RestClientEdgeIdentity{
			Url:           o.ControllerUrl,
			Username:      o.Username,
			Token:         "", // --use-api-session--
			LoginTime:     time.Now().Format(time.RFC3339),
			CaCert:        o.CaCert,
			ReadOnly:      o.ReadOnly,
			NetworkIdFile: o.NetworkId,
			ApiSession:    sess,
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

func (o *LoginOptions) terminatorId() string {
	if o.ControllerUrl != "" {
		o.ControllerUrl = addHttpsIfNeeded(o.ControllerUrl)
	}
	curl, curle := url.Parse(o.ControllerUrl)
	if curle != nil {
		o.Printf("unable to parse controller url [%s]\n", o.ControllerUrl)
		return ""
	}
	return curl.User.Username()
}

func (o *LoginOptions) createHttpTransport() (*http.Transport, error) {
	// if cli param supplied - use it first
	if o.NetworkId != "" {
		t, e := util.NewZitifiedTransportFromFile(o.NetworkId, o.terminatorId())
		o.transport = t
		return t, e
	}

	// if env var set - use it
	if zt, zte := util.ZitifiedTransportFromEnv(o.terminatorId()); zte != nil {
		o.Printf("NetworkId found by env var [%s] but failed: %v\n", constants.ZitiCliNetworkIdVarName, zte)
		return nil, zte
	} else {
		if zt != nil {
			o.Printf("NetworkId found by env var [%s], zitified transport enabled\n", constants.ZitiCliNetworkIdVarName)
			o.NetworkId = ""
			o.transport = zt
			return zt, nil
		}
	}

	caPool, caErr := o.GetCaPool()
	if caErr != nil {
		return nil, caErr
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caPool,
		},
	}, nil
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
	if o.ControllerUrl == "" {
		o.ControllerUrl = cachedCliConfig.Url
	}
	if o.ControllerUrl != "" {
		o.ControllerUrl = addHttpsIfNeeded(o.ControllerUrl)
	}
	if o.Username == "" {
		o.Username = cachedCliConfig.Username
	}
	if o.Token == "" {
		o.Token = cachedCliConfig.Token
	}
	if o.NetworkId == "" {
		o.NetworkId = cachedCliConfig.NetworkIdFile
	}
	if o.CaCert == "" {
		o.CaCert = cachedCliConfig.CaCert
	}
	if o.ApiSession == nil {
		if cachedCliConfig.ApiSession != nil {
			o.ApiSession = cachedCliConfig.ApiSession.ApiSession
		}
	}
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

func (o *LoginOptions) GetCaPool() (*x509.CertPool, error) {
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
	return caPool, nil
}

func (o *LoginOptions) MergeUnsetFrom(cached LoginOptions) {
	if o == nil {
		return
	}

	if o.Username == "" {
		o.Username = cached.Username
	}
	if o.Password == "" {
		o.Password = cached.Password
	}
	if o.Token == "" {
		o.Token = cached.Token
	}
	if o.CaCert == "" {
		o.CaCert = cached.CaCert
	}
	if o.ClientCert == "" {
		o.ClientCert = cached.ClientCert
	}
	if o.ClientKey == "" {
		o.ClientKey = cached.ClientKey
	}
	if o.ServiceName == "" {
		o.ServiceName = cached.ServiceName
	}
	if o.extJwtFile == "" {
		o.extJwtFile = cached.extJwtFile
	}
	if o.ExtJwtToken == "" {
		o.ExtJwtToken = cached.ExtJwtToken
	}
	if o.File == "" {
		o.File = cached.File
	}
	if o.ControllerUrl == "" {
		o.ControllerUrl = cached.ControllerUrl
	}
	if o.NetworkId == "" {
		o.NetworkId = cached.NetworkId
	}
	if o.FileCertCreds == nil {
		o.FileCertCreds = cached.FileCertCreds
	}
	if o.ApiSession == nil {
		o.ApiSession = cached.ApiSession
	}
	if o.transport == nil {
		o.transport = cached.transport
	}
	if o.caPool == nil {
		o.caPool = cached.caPool
	}
	if o.cachedId == nil {
		o.cachedId = cached.cachedId
	}
	if o.mgmtClient == nil {
		o.mgmtClient = cached.mgmtClient
	}
	if o.TotpCallback == nil {
		o.TotpCallback = cached.TotpCallback
	}
	o.client = cached.client
}

func (o *LoginOptions) NewManagementClient(useCachedCreds bool) (*edge_apis.ManagementApiClient, error) {
	if useCachedCreds {
		o.PopulateFromCache()
		// any error indicates there are probably no saved credentials. look for login information and use those
		cached := *o
		cached.IgnoreConfig = true // don't overwrite when trying to login
		loginErr := cached.Run()
		if loginErr != nil {
			return nil, loginErr
		}
		o.MergeUnsetFrom(cached)
		_, sessErr := o.mgmtClient.AuthenticateWithPreviousSession(&edge_apis.EmptyCredentials{}, o.ApiSession)
		if sessErr != nil {
			return nil, sessErr
		}
		return o.mgmtClient, nil
	}

	o.ControllerUrl = addHttpsIfNeeded(o.ControllerUrl)
	ctrlUrl, _ := url.Parse(o.ControllerUrl)
	if ctrlUrl.Path == "" {
		resolvedUrl := util.EdgeControllerGetManagementApiBasePath(o.ControllerUrl, o.CaCert, &o.client)
		hostUrl, _ := url.Parse(resolvedUrl)
		o.ControllerUrl = o.ControllerUrl + hostUrl.Path
	}
	transport := &edge_apis.TlsAwareHttpTransport{Transport: o.transport}
	o.client.Transport = transport

	o.mgmtClient = edge_apis.NewManagementApiClientWithConfig(&edge_apis.ApiClientConfig{
		ApiUrls:          []*url.URL{ctrlUrl},
		CaPool:           o.caPool,
		TotpCodeProvider: edge_apis.NewTotpCodeProviderFromChStringFunc(o.TotpCallback),
		Components: &edge_apis.Components{
			HttpClient:        &o.client,
			TlsAwareTransport: transport,
			CaPool:            o.caPool,
		},
	})

	o.mgmtClient.SetAllowOidcDynamicallyEnabled(true)

	return o.mgmtClient, nil
}

func (o *LoginOptions) Login() (edge_apis.ApiSession, error) {
	var authCreds edge_apis.Credentials
	if o.Token != "" {
		if jwtutil.IsJwt(o.Token) {
			return edge_apis.NewApiSessionOidc(o.Token, ""), nil
		} else {
			return edge_apis.NewApiSessionLegacy(o.Token), nil
		}
	} else if o.ApiSession != nil {
		return o.ApiSession, nil
	} else if o.Username != "" && o.Password != "" {
		authCreds = edge_apis.NewUpdbCredentials(o.Username, o.Password)
	} else if o.ClientCert != "" || o.ClientKey != "" {
		var key crypto.PrivateKey
		if keyPEM, err := os.ReadFile(o.ClientKey); err != nil {
			return nil, fmt.Errorf("failed to read key: %w", err)
		} else {
			keyBlock, _ := pem.Decode(keyPEM)
			k, _ := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
			key = k
		}

		var cert *x509.Certificate
		if certPEM, err := os.ReadFile(o.ClientCert); err != nil {
			return nil, fmt.Errorf("failed to read cert: %w", err)
		} else {
			block, _ := pem.Decode(certPEM)
			if block == nil {
				return nil, fmt.Errorf("invalid cert pem")
			}
			c, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse cert: %w", err)
			}
			cert = c
		}

		authCreds = edge_apis.NewCertCredentials([]*x509.Certificate{cert}, key)
	} else if o.File != "" {
		cfg, err := ziti.NewConfigFromFile(o.File)
		if err != nil {
			return nil, err
		}
		authCreds = edge_apis.NewIdentityCredentialsFromConfig(cfg.ID)
	} else if o.extJwtFile != "" {
		jwt, jwtErr := os.ReadFile(o.extJwtFile)
		if jwtErr != nil {
			return nil, fmt.Errorf("failed to read jwt file: %w", jwtErr)
		}
		authCreds = edge_apis.NewJwtCredentials(string(jwt))
	}

	s, err := o.mgmtClient.Authenticate(authCreds, nil)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("authentication failed for some reason but no error")
	}

	return s, nil
}

// EffectiveUrl will take all inputs and return the expected url of the controller or prompt the user to enter the url
// to use if insufficient information exists in the inputs provided
func (o *LoginOptions) EffectiveUrl() (string, error) {
	// if provided use the url provided
	if o.ControllerUrl != "" {
		return addHttpsIfNeeded(o.ControllerUrl), nil
	}

	// if using file-based auth and --file is provided, look into the file for the url
	if o.File != "" {
		return o.UrlFromFile()
	}

	// if a cached id exists - use the url from that
	if o.cachedId != nil {
		return addHttpsIfNeeded(o.cachedId.Url), nil
	}

	return "", nil
}

func addHttpsIfNeeded(host string) string {
	if host != "" && !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}
	return host
}

func (o *LoginOptions) UrlFromFile() (string, error) {
	cfg, err := ziti.NewConfigFromFile(o.File)
	if err != nil {
		return "", fmt.Errorf("could not read file %s: %w", o.File, err)
	}

	if o.FileCertCreds == nil {
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
		return "", fmt.Errorf("could not parse ztAPI '%s' as a URL", ztAPI)
	}

	return addHttpsIfNeeded(parsedZtAPI.Host), nil
}
