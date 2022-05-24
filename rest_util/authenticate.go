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

package rest_util

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	openapiclient "github.com/go-openapi/runtime/client"
	"github.com/openziti/edge/rest_management_api_client"
	"github.com/openziti/edge/rest_management_api_client/authentication"
	"github.com/openziti/edge/rest_model"
	"net/http"
	"net/url"
)

// Authenticator is an interface that facilitates obtaining an API Session.
type Authenticator interface {
	//Authenticate issues an authentication HTTP requests to the designated controller. The method and operation
	// of this authentication request is determined by the implementor.
	Authenticate(controllerAddress *url.URL) (*rest_model.CurrentAPISessionDetail, error)

	//BuildHttpClient returns a http.Client to use for an API client. This specifically allows
	//client certificate authentication to be configured in the http.Client's transport/tls.Config
	BuildHttpClient() (*http.Client, error)
}

// HttpClientFunc allows an external HttpClient to be created and used.
type HttpClientFunc func(tlsClientConfig *tls.Config) (*http.Client, error)

// TlsConfigFunc allows the tls.Config to be modified before use.
type TlsConfigFunc func() (*tls.Config, error)

// AuthenticatorBase provides embeddable shared capabilities for all
// authenticators.
type AuthenticatorBase struct {
	ConfigTypes    rest_model.ConfigTypes
	EnvInfo        *rest_model.EnvInfo
	SdkInfo        *rest_model.SdkInfo
	HttpClientFunc HttpClientFunc
	TlsConfigFunc  TlsConfigFunc
	RootCas        *x509.CertPool
}

// BuildHttpClientWithModifyTls builds a new http.Client with the provided HttpClientFunc and TlsConfigFunc.
// If not set, default NewHttpClientWithTlsConfig and NewTlsConfig will be used.
func (a *AuthenticatorBase) BuildHttpClientWithModifyTls(modifyTls func(*tls.Config)) (*http.Client, error) {
	if a.HttpClientFunc == nil {
		a.HttpClientFunc = NewHttpClientWithTlsConfig
	}

	if a.TlsConfigFunc == nil {
		a.TlsConfigFunc = NewTlsConfig
	}

	tlsConfig, err := a.TlsConfigFunc()
	tlsConfig.RootCAs = a.RootCas

	if modifyTls != nil {
		modifyTls(tlsConfig)
	}

	if err != nil {
		return nil, err
	}

	httpClient, err := a.HttpClientFunc(tlsConfig)

	if err != nil {
		return nil, err
	}

	return httpClient, err
}

var _ Authenticator = &AuthenticatorUpdb{}

// AuthenticatorUpdb is an implementation of Authenticator that can fulfill username/password authentication
// requests.
type AuthenticatorUpdb struct {
	AuthenticatorBase
	Username string
	Password string
}

func NewAuthenticatorUpdb(username, password string) *AuthenticatorUpdb {
	return &AuthenticatorUpdb{
		Username: username,
		Password: password,
	}
}

func (a *AuthenticatorUpdb) BuildHttpClient() (*http.Client, error) {
	return a.BuildHttpClientWithModifyTls(nil)
}

func (a *AuthenticatorUpdb) Params() *authentication.AuthenticateParams {
	return &authentication.AuthenticateParams{
		Auth: &rest_model.Authenticate{
			ConfigTypes: a.ConfigTypes,
			EnvInfo:     a.EnvInfo,
			SdkInfo:     a.SdkInfo,
			Username:    rest_model.Username(a.Username),
			Password:    rest_model.Password(a.Password),
		},
		Method:  "password",
		Context: context.Background(),
	}
}

func (a *AuthenticatorUpdb) Authenticate(controllerAddress *url.URL) (*rest_model.CurrentAPISessionDetail, error) {
	httpClient, err := a.BuildHttpClientWithModifyTls(nil)

	if err != nil {
		return nil, err
	}

	clientRuntime := openapiclient.NewWithClient(controllerAddress.Host, rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	client := rest_management_api_client.New(clientRuntime, nil)

	params := a.Params()

	resp, err := client.Authentication.Authenticate(params)

	if err != nil {
		return nil, err
	}

	if resp.GetPayload() == nil {
		return nil, fmt.Errorf("error, nil payload: %v", resp.Error())
	}

	return resp.GetPayload().Data, nil
}

var _ Authenticator = &AuthenticatorCert{}

// AuthenticatorCert is an implementation of Authenticator that can fulfill client certificate authentication
// requests.
type AuthenticatorCert struct {
	AuthenticatorBase
	Certificate *x509.Certificate
	PrivateKey  crypto.PrivateKey
}

func NewAuthenticatorCert(cert *x509.Certificate, privateKey crypto.PrivateKey) *AuthenticatorCert {
	return &AuthenticatorCert{
		Certificate: cert,
		PrivateKey:  privateKey,
	}
}

func (a *AuthenticatorCert) BuildHttpClient() (*http.Client, error) {
	return a.BuildHttpClientWithModifyTls(func(config *tls.Config) {
		config.Certificates = []tls.Certificate{
			{
				PrivateKey: a.PrivateKey,
				Leaf:       a.Certificate,
			},
		}
	})
}

func (a *AuthenticatorCert) Authenticate(controllerAddress *url.URL) (*rest_model.CurrentAPISessionDetail, error) {
	httpClient, err := a.BuildHttpClient()
	if err != nil {
		return nil, err
	}

	clientRuntime := openapiclient.NewWithClient(controllerAddress.Host, rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	client := rest_management_api_client.New(clientRuntime, nil)

	params := a.Params()

	resp, err := client.Authentication.Authenticate(params)

	if err != nil {
		return nil, err
	}

	if resp.GetPayload() == nil {
		return nil, fmt.Errorf("error, nil payload: %v", resp.Error())
	}

	return resp.GetPayload().Data, nil
}

func (a *AuthenticatorCert) Params() *authentication.AuthenticateParams {
	return &authentication.AuthenticateParams{
		Auth: &rest_model.Authenticate{
			ConfigTypes: a.ConfigTypes,
			EnvInfo:     a.EnvInfo,
			SdkInfo:     a.SdkInfo,
		},
		Method:  "cert'",
		Context: context.Background(),
	}
}

type AuthenticatorAuthHeader struct {
	AuthenticatorBase
	Token string
}

func NewAuthenticatorAuthHeader(token string) *AuthenticatorAuthHeader {
	return &AuthenticatorAuthHeader{
		Token: token,
	}
}

func (a *AuthenticatorAuthHeader) Params() *authentication.AuthenticateParams {
	return &authentication.AuthenticateParams{
		Auth: &rest_model.Authenticate{
			ConfigTypes: a.ConfigTypes,
			EnvInfo:     a.EnvInfo,
			SdkInfo:     a.SdkInfo,
		},
		Method:  "jwt",
		Context: context.Background(),
	}
}

func (a *AuthenticatorAuthHeader) Authenticate(controllerAddress *url.URL) (*rest_model.CurrentAPISessionDetail, error) {
	httpClient, err := a.BuildHttpClientWithModifyTls(nil)

	if err != nil {
		return nil, err
	}

	clientRuntime := openapiclient.NewWithClient(controllerAddress.Host, rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = &HeaderAuth{
		HeaderName:  "Authorization",
		HeaderValue: a.Token,
	}

	client := rest_management_api_client.New(clientRuntime, nil)

	params := a.Params()

	resp, err := client.Authentication.Authenticate(params)

	if err != nil {
		return nil, err
	}

	if resp.GetPayload() == nil {
		return nil, fmt.Errorf("error, nil payload: %v", resp.Error())
	}

	return resp.GetPayload().Data, nil
}
