package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_management_api_client"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/controller/env"
	fabric_rest_client "github.com/openziti/ziti/v2/controller/rest_client"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/constants"
	"github.com/pkg/errors"
	"gopkg.in/resty.v1"
)

var zitiCliContextCollection *ziti.CtxCollection

func init() {
	zitiCliContextCollection = ziti.NewSdkCollection()
}

type API string

const (
	FabricAPI API = "fabric"
	EdgeAPI   API = "edge"
)

type RestClientConfig struct {
	EdgeIdentities    map[string]*RestClientEdgeIdentity `json:"edgeIdentities"`
	Default           string                             `json:"default"`
	Layout            int                                `json:"layout"`
	LayoutNoticeShown bool                               `json:"layoutNoticeShown,omitempty"`
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

type RestClientIdentity interface {
	NewTlsClientConfig() (*tls.Config, error)
	NewClient(timeout time.Duration, verbose bool) (*resty.Client, error)
	NewClientByTerminator(timeout time.Duration, verbose bool, terminator string) (*resty.Client, error)
	NewRequest(client *resty.Client) *resty.Request
	IsReadOnly() bool
	GetBaseUrlForApi(api API) (string, error)
	NewEdgeManagementClient(clientOpts ClientOpts) (*rest_management_api_client.ZitiEdgeManagement, error)
	NewFabricManagementClient(clientOpts ClientOpts) (*fabric_rest_client.ZitiFabric, error)
	NewWsHeader() http.Header
	NewZitiContext() (ziti.Context, error)
}

func NewRequest(restClientIdentity RestClientIdentity, timeoutInSeconds int, verbose bool) (*resty.Request, error) {
	return NewRequestByTerminator(restClientIdentity, timeoutInSeconds, verbose, "")
}

func NewRequestByTerminator(restClientIdentity RestClientIdentity, timeoutInSeconds int, verbose bool, terminator string) (*resty.Request, error) {
	client, err := restClientIdentity.NewClientByTerminator(time.Duration(timeoutInSeconds)*time.Second, verbose, terminator)
	if err != nil {
		return nil, err
	}
	return restClientIdentity.NewRequest(client).SetHeader("Content-Type", "application/json"), nil
}

type RestClientEdgeIdentity struct {
	Url           string                           `json:"url"`
	Username      string                           `json:"username"`
	Token         string                           `json:"token"`
	LoginTime     string                           `json:"loginTime"`
	CaCert        string                           `json:"caCert,omitempty"`
	ReadOnly      bool                             `json:"readOnly"`
	NetworkIdFile string                           `json:"networkId"`
	ApiSession    *edge_apis.ApiSessionJsonWrapper `json:"apiSession"`
}

func (self *RestClientEdgeIdentity) IsReadOnly() bool {
	return self.ReadOnly
}

func (self *RestClientEdgeIdentity) NewTlsClientConfig() (*tls.Config, error) {
	rootCaPool := x509.NewCertPool()

	if self.CaCert != "" {
		rootPemData, err := os.ReadFile(self.CaCert)
		if err != nil {
			return nil, errors.Errorf("could not read session certificates [%s]: %v", self.CaCert, err)
		}

		rootCaPool.AppendCertsFromPEM(rootPemData)
	} else {
		var err error
		rootCaPool, err = x509.SystemCertPool()
		if err != nil {
			return nil, errors.New("couldn't retrieve the SystemCertPool and no CaCert provided")
		}
	}

	return &tls.Config{
		RootCAs: rootCaPool,
	}, nil
}

func (self *RestClientEdgeIdentity) NewClient(timeout time.Duration, verbose bool) (*resty.Client, error) {
	return self.NewClientByTerminator(timeout, verbose, "")
}

func (self *RestClientEdgeIdentity) getHttpTransport(log *log.Logger, verbose bool, terminator string) (*http.Transport, error) {
	if ztFromEnv, ztFromEnvErr := ZitifiedTransportFromEnv(""); ztFromEnvErr != nil {
		return &http.Transport{}, ztFromEnvErr
	} else {
		if ztFromEnv != nil {
			if verbose {
				log.Printf("Using Ziti Transport from environment var: %s", constants.ZitiCliNetworkIdVarName)
			}
			return ztFromEnv, nil
		} else {
			if self.NetworkIdFile != "" {
				if ztFromFile, ztFromFileErr := NewZitifiedTransportFromFile(self.NetworkIdFile, terminator); ztFromFileErr != nil {
					// ignore any error around the networkId file
					if verbose {
						log.Printf("Ziti transport from cached file failed: %v", ztFromFileErr)
					}
				} else {
					if verbose {
						log.Printf("Using Ziti transport from cached file: %s", self.NetworkIdFile)
					}
					return ztFromFile, nil
				}
			} else {
				if verbose {
					log.Printf("Using default http transport")
				}
			}
		}
	}
	return &http.Transport{}, nil
}

func (self *RestClientEdgeIdentity) NewClientByTerminator(timeout time.Duration, verbose bool, terminator string) (*resty.Client, error) {
	client := NewClient()
	transport, err := self.getHttpTransport(client.Log, verbose, terminator)
	if err != nil {
		return nil, err
	}
	client.GetClient().Transport = transport
	if self.CaCert != "" {
		client.SetRootCertificate(self.CaCert)
	}
	client.SetTimeout(timeout)
	client.SetDebug(verbose)
	return client, nil
}

func (self *RestClientEdgeIdentity) NewRequest(client *resty.Client) *resty.Request {
	r := client.R()
	if self.ApiSession != nil && self.ApiSession.ApiSession != nil {
		switch self.ApiSession.ApiSession.GetType() {
		case edge_apis.ApiSessionTypeOidc:
			authHeader := "Bearer " + strings.TrimSpace(string(self.ApiSession.ApiSession.GetToken()))
			r.SetHeader("Authorization", authHeader)
		case edge_apis.ApiSessionTypeLegacy:
			r.SetHeader(env.ZitiSession, string(self.ApiSession.ApiSession.GetToken()))
		default:
			panic("unsupported api session type " + self.ApiSession.ApiSession.GetType())
		}
	} else {
		r.SetHeader(env.ZitiSession, self.Token)
	}
	return r
}

// oidcTokenLeeway skews the OIDC token expiration slightly before its true expiry so it is
// used on the border, requests won't fail 401s.
const oidcTokenLeeway = 30 * time.Second

// OidcAccessTokenExpired reports whether sess is an OIDC session whose access token is expired (or
// will expire within oidcTokenLeeway). Non-OIDC sessions always report false. An unreadable token is
// treated as expired so the caller refreshes rather than sending a token it cannot validate.
func OidcAccessTokenExpired(sess edge_apis.ApiSession) bool {
	oidcSess, ok := sess.(*edge_apis.ApiSessionOidc)
	if !ok {
		return false
	}
	claims, err := oidcSess.GetAccessClaims()
	if err != nil {
		return true
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return true
	}
	return time.Now().Add(oidcTokenLeeway).After(exp.Time)
}

// OidcRefreshTokenValid reports whether sess carries an OIDC refresh token that is present and not
// yet expired. A missing, unreadable, or expired refresh token returns false, indicating the user
// must authenticate again rather than refresh.
func OidcRefreshTokenValid(sess edge_apis.ApiSession) bool {
	oidcSess, ok := sess.(*edge_apis.ApiSessionOidc)
	if !ok || oidcSess.OidcTokens == nil || oidcSess.OidcTokens.RefreshToken == "" {
		return false
	}
	claims := &jwt.RegisteredClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(oidcSess.OidcTokens.RefreshToken, claims); err != nil {
		return false
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return false
	}
	return time.Now().Add(oidcTokenLeeway).Before(exp.Time)
}

// refreshOidcTokenIfExpired refreshes the cached OIDC access token when it has expired and the
// refresh token is still valid. It returns true when the session was refreshed so the caller can
// persist the updated config. Non-OIDC sessions and still-valid access tokens are no-ops. An expired
// refresh token, or a failed refresh, returns an error telling the user to log in again.
func (self *RestClientEdgeIdentity) refreshOidcTokenIfExpired() (bool, error) {
	if self.ApiSession == nil || self.ApiSession.ApiSession == nil {
		return false, nil
	}
	if !OidcAccessTokenExpired(self.ApiSession.ApiSession) {
		return false, nil
	}
	if !OidcRefreshTokenValid(self.ApiSession.ApiSession) {
		return false, errors.New("the cached access token has expired and the refresh token can no longer be used to re-authenticate, please login again")
	}

	ctrlUrl, err := url.Parse(self.Url)
	if err != nil {
		return false, errors.Wrapf(err, "could not parse controller url %v while refreshing token", self.Url)
	}

	tlsClientConfig, err := self.NewTlsClientConfig()
	if err != nil {
		return false, err
	}

	_, _ = fmt.Fprintln(os.Stderr, "Access token has expired, refreshing it using the cached refresh token...")

	mgmtClient := edge_apis.NewManagementApiClient([]*url.URL{ctrlUrl}, tlsClientConfig.RootCAs, nil)
	refreshed, refreshErr := mgmtClient.AuthenticateWithPreviousSession(&edge_apis.EmptyCredentials{}, self.ApiSession.ApiSession)
	if refreshErr != nil || refreshed == nil {
		return false, errors.Wrap(refreshErr, "failed to refresh the access token using the cached refresh token, please login again")
	}

	self.ApiSession = &edge_apis.ApiSessionJsonWrapper{ApiSession: refreshed}
	_, _ = fmt.Fprintln(os.Stderr, "Successfully refreshed the access token")
	return true, nil
}

func (self *RestClientEdgeIdentity) GetBaseUrlForApi(api API) (string, error) {
	if api == EdgeAPI {
		return self.Url, nil
	}
	if api == FabricAPI {
		u, err := url.Parse(self.Url)
		if err != nil {
			return "", err
		}
		if u.User != nil {
			return fmt.Sprintf("%s://%s@%s/fabric/v1", u.Scheme, u.User.Username(), u.Host), nil
		} else {
			return fmt.Sprintf("%s://%s/fabric/v1", u.Scheme, u.Host), nil
		}
	}
	return "", errors.Errorf("unsupported api %v", api)
}

func (self *RestClientEdgeIdentity) NewEdgeManagementClient(clientOpts ClientOpts) (*rest_management_api_client.ZitiEdgeManagement, error) {
	httpClient, err := self.newRestClientTransport(clientOpts)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(self.Url)
	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(parsedHost.Host, rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = self.newEdgeAuth()

	return rest_management_api_client.New(clientRuntime, nil), nil
}

func (self *RestClientEdgeIdentity) NewFabricManagementClient(clientOpts ClientOpts) (*fabric_rest_client.ZitiFabric, error) {
	httpClient, err := self.newRestClientTransport(clientOpts)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(self.Url)
	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(parsedHost.Host, fabric_rest_client.DefaultBasePath, fabric_rest_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = self.newEdgeAuth()

	return fabric_rest_client.New(clientRuntime, nil), nil
}

func (self *RestClientEdgeIdentity) NewWsHeader() http.Header {
	result := http.Header{}

	if self.ApiSession != nil && self.ApiSession.ApiSession != nil {
		if self.ApiSession.ApiSession.GetType() == edge_apis.ApiSessionTypeOidc {
			result.Set("Authorization", "Bearer "+strings.TrimSpace(string(self.ApiSession.ApiSession.GetToken())))
		} else {
			result.Set(env.ZitiSession, string(self.ApiSession.ApiSession.GetToken()))
		}
	} else if self.Token != "" {
		result.Set(env.ZitiSession, self.Token)
	} else {
		panic("no  authentication mechanism set")
	}
	return result
}

func (self *RestClientEdgeIdentity) NewZitiContext() (ziti.Context, error) {
	if self.NetworkIdFile != "" {
		data, err := os.ReadFile(self.NetworkIdFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read ziti identity file %s: %v", self.NetworkIdFile, err)
		}
		return NewZitifiedContextFromSlice(data)
	} else {
		data, err := ZitiConfigFromEnv()
		if err != nil {
			return nil, err
		}
		return NewZitifiedContextFromSlice(data)
	}
}

func LoadRestClientConfig() (*RestClientConfig, string, error) {
	config := &RestClientConfig{}

	cfgDir, err := ConfigDir()
	if err != nil {
		return nil, "", errors.Wrap(err, "couldn't get config dir while loading cli configuration")
	}

	configFile := filepath.Join(cfgDir, "ziti-cli.json")
	_, err = os.Stat(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			config.EdgeIdentities = map[string]*RestClientEdgeIdentity{}
			return config, configFile, nil
		}
		return nil, "", errors.Wrapf(err, "error while statting config file %v", configFile)
	}

	result, err := os.ReadFile(configFile)
	if err != nil {
		return nil, "", errors.Wrapf(err, "error while reading config file %v", configFile)
	}

	if err := json.Unmarshal(result, config); err != nil {
		return nil, "", errors.Wrapf(err, "error while parsing JSON config file %v", configFile)
	}

	if config.EdgeIdentities == nil {
		config.EdgeIdentities = map[string]*RestClientEdgeIdentity{}
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

	if err = os.MkdirAll(cfgDir, 0700); err != nil {
		return errors.Wrapf(err, "unable to create config dir %v", cfgDir)
	}

	configFile := filepath.Join(cfgDir, "ziti-cli.json")

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return errors.Wrap(err, "error while marshalling config to JSON")
	}

	err = os.WriteFile(configFile, data, 0600)
	if err != nil {
		return errors.Wrapf(err, "error while writing config file %v", configFile)
	}

	return nil
}

var selectedIdentity RestClientIdentity
var selectIdentityLock sync.Mutex

func ReloadConfig() {
	selectIdentityLock.Lock()
	defer selectIdentityLock.Unlock()

	selectedIdentity = nil
}

func LoadSelectedIdentity() (RestClientIdentity, error) {
	selectIdentityLock.Lock()
	defer selectIdentityLock.Unlock()

	if selectedIdentity == nil {
		config, configFile, err := LoadRestClientConfig()
		if err != nil {
			return nil, err
		}
		id := config.GetIdentity()
		clientIdentity, found := config.EdgeIdentities[id]
		if !found {
			if len(config.EdgeIdentities) == 0 {
				return nil, errors.New("no identities found in CLI config. Please log in using 'ziti edge login' command")
			} else {
				return nil, errors.Errorf("no identity '%v' found in CLI config %v. You can select an existing identity using 'ziti edge use <identity_name>'", id, configFile)
			}
		}

		// refresh an expired OIDC access token before any request uses it, persisting the new tokens
		if refreshed, err := clientIdentity.refreshOidcTokenIfExpired(); err != nil {
			return nil, err
		} else if refreshed {
			// the refresh already succeeded server side, so use the new token even if saving fails,
			// otherwise a write error would waste a single-use refresh token
			if persistErr := PersistRestClientConfig(config); persistErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "warning: could not save refreshed token to CLI config: %v\n", persistErr)
			}
		}

		selectedIdentity = clientIdentity
	}
	return selectedIdentity, nil
}

func LoadSelectedRWIdentity() (RestClientIdentity, error) {
	id, err := LoadSelectedIdentity()
	if err != nil {
		return nil, err
	}
	if id.IsReadOnly() {
		return nil, errors.New("this login is marked read-only, only GET operations are allowed")
	}
	return id, nil
}

func newRestClientResponseF(clientOpts ClientOpts) func(*http.Response, error) {
	return func(resp *http.Response, err error) {
		if clientOpts.OutputResponseJson() {
			if resp == nil || resp.Body == nil {
				_, _ = fmt.Fprint(clientOpts.OutputWriter(), "<empty response body>\n")
				return
			}

			resp.Body = io.NopCloser(resp.Body)
			bodyContent, err := io.ReadAll(resp.Body)
			if err != nil {
				_, _ = fmt.Fprintf(clientOpts.ErrOutputWriter(), "could not read response body: %v", err)
				return
			}
			bodyStr := string(bodyContent)
			_, _ = fmt.Fprint(clientOpts.OutputWriter(), bodyStr, "\n")
		}
	}
}

func newRestClientRequestF(clientOpts ClientOpts, readOnly bool) func(*http.Request) error {
	return func(request *http.Request) error {
		if readOnly && !strings.EqualFold(request.Method, "get") {
			return errors.New("this login is marked read-only, only GET operations are allowed")
		}
		if clientOpts.OutputRequestJson() {
			if request == nil || request.Body == nil {
				_, _ = fmt.Fprint(clientOpts.OutputWriter(), "<empty request body>\n")
				return nil
			}

			body, err := request.GetBody()
			if err == nil {
				_, _ = fmt.Fprintf(clientOpts.ErrOutputWriter(), "could not copy request body: %v", err)
				return nil
			}
			bodyContent, err := io.ReadAll(body)
			if err != nil {
				bodyStr := string(bodyContent)
				_, _ = fmt.Fprint(clientOpts.OutputWriter(), bodyStr, "\n")
				return nil
			}
		}
		return nil
	}
}

func (self *RestClientEdgeIdentity) newRestClientTransport(clientOpts ClientOpts) (*http.Client, error) {
	u, err := url.Parse(self.Url)
	if err != nil {
		return nil, err
	}
	at := ""
	if u.User != nil {
		at = u.User.Username()
	}

	t, e := self.getHttpTransport(nil, false, at)
	if e != nil {
		return nil, e
	}

	t.Proxy = http.ProxyFromEnvironment
	t.ForceAttemptHTTP2 = true
	t.MaxIdleConns = 10
	t.IdleConnTimeout = 10 * time.Second
	t.TLSHandshakeTimeout = 10 * time.Second
	t.ExpectContinueTimeout = 1 * time.Second

	httpClientTransport := &edgeTransport{
		Transport:    t,
		ResponseFunc: newRestClientResponseF(clientOpts),
		RequestFunc:  newRestClientRequestF(clientOpts, self.IsReadOnly()),
	}

	tlsClientConfig, err := self.NewTlsClientConfig()
	if err != nil {
		return nil, err
	}

	httpClientTransport.TLSClientConfig = tlsClientConfig

	httpClient := &http.Client{
		Transport: httpClientTransport,
		Timeout:   10 * time.Second,
	}
	return httpClient, nil
}

func (self *RestClientEdgeIdentity) newEdgeAuth() EdgeManagementAuth {
	ea := EdgeManagementAuth{}

	if self.ApiSession != nil && self.ApiSession.ApiSession != nil {
		if self.ApiSession.ApiSession.GetType() == edge_apis.ApiSessionTypeOidc {
			ea.BearerToken = string(self.ApiSession.ApiSession.GetToken())
		} else {
			ea.LegacyToken = string(self.ApiSession.ApiSession.GetToken())
		}
	} else if self.Token != "" {
		ea.LegacyToken = self.Token
	} else {
		panic("no  authentication mechanism set")
	}

	return ea
}
