package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/controller/env"
	fabric_rest_client "github.com/openziti/ziti/controller/rest_client"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/constants"
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
	EdgeIdentities map[string]*RestClientEdgeIdentity `json:"edgeIdentities"`
	Default        string                             `json:"default"`
	Layout         int                                `json:"layout"`
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
	Url           string `json:"url"`
	Username      string `json:"username"`
	Token         string `json:"token"`
	LoginTime     string `json:"loginTime"`
	CaCert        string `json:"caCert,omitempty"`
	ReadOnly      bool   `json:"readOnly"`
	NetworkIdFile string `json:"networkId"`
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
	client := NewClient()
	if ztFromEnv, ztFromEnvErr := ZitifiedTransportFromEnv(); ztFromEnvErr != nil {
		return nil, ztFromEnvErr
	} else {
		if ztFromEnv != nil {
			if verbose {
				client.Log.Printf("Using Ziti Transport from environment var: %s", constants.ZitiCliNetworkIdVarName)
			}
			client.GetClient().Transport = ztFromEnv
		} else {
			if ztFromFile, ztFromFileErr := NewZitifiedTransportFromFile(self.NetworkIdFile); ztFromFileErr != nil {
				// ignore any error around the networkId file
				if verbose {
					client.Log.Printf("Ziti Transport from cached file failed: %v", ztFromFileErr)
				}
			} else {
				if verbose {
					client.Log.Printf("Using Ziti Transport from cached file: %s", self.NetworkIdFile)
				}
				client.GetClient().Transport = ztFromFile
			}
		}
	}

	if self.CaCert != "" {
		client.SetRootCertificate(self.CaCert)
	}
	client.SetTimeout(timeout)
	client.SetDebug(verbose)
	return client, nil
}

func (self *RestClientEdgeIdentity) NewRequest(client *resty.Client) *resty.Request {
	r := client.R()
	r.SetHeader(env.ZitiSession, self.Token)
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
	return "", errors.Errorf("unsupported api %v", api)
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
	result.Set(env.ZitiSession, self.Token)
	return result
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
