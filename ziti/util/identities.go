package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/openziti/edge/rest_management_api_client"
	fabric_rest_client "github.com/openziti/fabric/rest_client"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti/constants"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/pkg/errors"
	"gopkg.in/resty.v1"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type API string

const (
	FabricAPI API = "fabric"
	EdgeAPI   API = "edge"
)

type RestClientConfig struct {
	EdgeIdentities   map[string]*RestClientEdgeIdentity   `json:"edgeIdentities"`
	FabricIdentities map[string]*RestClientFabricIdentity `json:"fabricIdentities"`
	Default          string                               `json:"default"`
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
	NewRequest(client *resty.Client) *resty.Request
	IsReadOnly() bool
	GetBaseUrlForApi(api API) (string, error)
	NewEdgeManagementClient(clientOpts ClientOpts) (*rest_management_api_client.ZitiEdgeManagement, error)
	NewFabricManagementClient(clientOpts ClientOpts) (*fabric_rest_client.ZitiFabric, error)
	NewWsHeader() http.Header
}

func NewRequest(restClientIdentity RestClientIdentity, timeoutInSeconds int, verbose bool) (*resty.Request, error) {
	client, err := restClientIdentity.NewClient(time.Duration(timeoutInSeconds)*time.Second, verbose)
	if err != nil {
		return nil, err
	}
	return restClientIdentity.NewRequest(client).SetHeader("Content-Type", "application/json"), nil
}

type RestClientEdgeIdentity struct {
	Url       string `json:"url"`
	Username  string `json:"username"`
	Token     string `json:"token"`
	LoginTime string `json:"loginTime"`
	CaCert    string `json:"caCert,omitempty"`
	ReadOnly  bool   `json:"readOnly"`
}

func (self *RestClientEdgeIdentity) IsReadOnly() bool {
	return self.ReadOnly
}

func (self *RestClientEdgeIdentity) NewTlsClientConfig() (*tls.Config, error) {
	rootCaPool := x509.NewCertPool()

	rootPemData, err := os.ReadFile(self.CaCert)
	if err != nil {
		return nil, errors.Errorf("could not read session certificates [%s]: %v", self.CaCert, err)
	}

	rootCaPool.AppendCertsFromPEM(rootPemData)

	return &tls.Config{
		RootCAs: rootCaPool,
	}, nil
}

func (self *RestClientEdgeIdentity) NewClient(timeout time.Duration, verbose bool) (*resty.Client, error) {
	client := newClient()
	client.SetRootCertificate(self.CaCert)
	client.SetTimeout(timeout)
	client.SetDebug(verbose)
	return client, nil
}

func (self *RestClientEdgeIdentity) NewRequest(client *resty.Client) *resty.Request {
	r := client.R()
	r.SetHeader(constants.ZitiSession, self.Token)
	return r
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
		return u.Scheme + "://" + u.Host + "/fabric/v1", nil
	}
	return "", errors.Errorf("unsupport api %v", api)
}

func (self *RestClientEdgeIdentity) NewEdgeManagementClient(clientOpts ClientOpts) (*rest_management_api_client.ZitiEdgeManagement, error) {
	httpClient, err := newRestClientTransport(clientOpts, self)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(self.Url)
	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(parsedHost.Host, rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = &EdgeManagementAuth{
		Token: self.Token,
	}

	return rest_management_api_client.New(clientRuntime, nil), nil
}

func (self *RestClientEdgeIdentity) NewFabricManagementClient(clientOpts ClientOpts) (*fabric_rest_client.ZitiFabric, error) {
	httpClient, err := newRestClientTransport(clientOpts, self)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(self.Url)
	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(parsedHost.Host, fabric_rest_client.DefaultBasePath, fabric_rest_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = &EdgeManagementAuth{
		Token: self.Token,
	}

	return fabric_rest_client.New(clientRuntime, nil), nil
}

func (self *RestClientEdgeIdentity) NewWsHeader() http.Header {
	result := http.Header{}
	result.Set(constants.ZitiSession, self.Token)
	return result
}

type RestClientFabricIdentity struct {
	Url        string `json:"url"`
	CaCert     string `json:"caCert,omitempty"`
	ClientCert string `json:"clientCert,omitempty"`
	ClientKey  string `json:"clientKey,omitempty"`
	ReadOnly   bool   `json:"readOnly"`
}

func (self *RestClientFabricIdentity) NewTlsClientConfig() (*tls.Config, error) {
	id, err := identity.LoadClientIdentity(self.ClientCert, self.ClientKey, self.CaCert)
	if err != nil {
		return nil, errors.Wrap(err, "unable to load identity")
	}
	return id.ClientTLSConfig(), nil
}

func (self *RestClientFabricIdentity) NewClient(timeout time.Duration, verbose bool) (*resty.Client, error) {
	id, err := identity.LoadClientIdentity(self.ClientCert, self.ClientKey, self.CaCert)
	if err != nil {
		return nil, errors.Wrap(err, "unable to load identity")
	}
	client := newClient()
	client.SetTLSClientConfig(id.ClientTLSConfig())
	client.SetTimeout(timeout)
	client.SetDebug(verbose)
	return client, nil
}

func (self *RestClientFabricIdentity) NewRequest(client *resty.Client) *resty.Request {
	return client.R()
}

func (self *RestClientFabricIdentity) GetBaseUrlForApi(api API) (string, error) {
	if api == FabricAPI {
		u, err := url.Parse(self.Url)
		if err != nil {
			return "", err
		}
		return u.Scheme + "://" + u.Host + "/fabric/v1", nil
	}
	return "", errors.Errorf("unsupport api %v", api)
}

func (self *RestClientFabricIdentity) IsReadOnly() bool {
	return self.ReadOnly
}

func (self *RestClientFabricIdentity) NewEdgeManagementClient(ClientOpts) (*rest_management_api_client.ZitiEdgeManagement, error) {
	return nil, errors.New("fabric identities cannot be used to connect to the edge management API")
}

func (self *RestClientFabricIdentity) NewFabricManagementClient(clientOpts ClientOpts) (*fabric_rest_client.ZitiFabric, error) {
	httpClient, err := newRestClientTransport(clientOpts, self)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(self.Url)
	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(parsedHost.Host, fabric_rest_client.DefaultBasePath, fabric_rest_client.DefaultSchemes, httpClient)

	return fabric_rest_client.New(clientRuntime, nil), nil
}

func (self *RestClientFabricIdentity) NewWsHeader() http.Header {
	return http.Header{}
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
			config.FabricIdentities = map[string]*RestClientFabricIdentity{}
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

	if config.FabricIdentities == nil {
		config.FabricIdentities = map[string]*RestClientFabricIdentity{}
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

	err = os.WriteFile(configFile, data, 0600)
	if err != nil {
		return errors.Wrapf(err, "error while writing config file %v", configFile)
	}

	return nil
}

var selectedIdentity RestClientIdentity

func LoadSelectedIdentity() (RestClientIdentity, error) {
	if selectedIdentity == nil {
		config, configFile, err := LoadRestClientConfig()
		if err != nil {
			return nil, err
		}
		id := config.GetIdentity()
		clientIdentity, found := config.EdgeIdentities[id]
		if !found {
			return nil, errors.Errorf("no identity '%v' found in cli config %v", id, configFile)
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

func LoadSelectedIdentityForApi(api API) (RestClientIdentity, error) {
	if api == EdgeAPI {
		return LoadSelectedIdentity()
	}

	if api == FabricAPI {
		if selectedIdentity == nil {
			config, configFile, err := LoadRestClientConfig()
			if err != nil {
				return nil, err
			}
			id := config.GetIdentity()
			var clientIdentity RestClientIdentity
			var found bool
			clientIdentity, found = config.EdgeIdentities[id]
			if !found {
				clientIdentity, found = config.FabricIdentities[id]
				if !found {
					return nil, errors.Errorf("no identity '%v' found in cli config %v", id, configFile)
				}
			}
			selectedIdentity = clientIdentity
		}
		return selectedIdentity, nil
	}
	return nil, errors.Errorf("unsupported API: '%v'", api)
}

func LoadSelectedRWIdentityForApi(api API) (RestClientIdentity, error) {
	id, err := LoadSelectedIdentityForApi(api)
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

func newRestClientTransport(clientOpts ClientOpts, clientIdentity RestClientIdentity) (*http.Client, error) {
	httpClientTransport := &edgeTransport{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 10 * time.Second,
			}).DialContext,

			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       10 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		ResponseFunc: newRestClientResponseF(clientOpts),
		RequestFunc:  newRestClientRequestF(clientOpts, clientIdentity.IsReadOnly()),
	}

	tlsClientConfig, err := clientIdentity.NewTlsClientConfig()
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
