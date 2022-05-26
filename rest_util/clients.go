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

// Package rest_util provides helper functions to generate a client for the Ziti Edge REST APIs. It is a meat and
// potato API that is meant to be consumed by higher level implementations (e.g. CLIs).
//
// The main entry functions are:
// - NewEdgeManagementClientWithToken()
// - NewEdgeManagementClientWithUpdb()
// - NewEdgeManagementClientWithCert()
// - NewEdgeManagementClientWithAuthenticator()
// - NewEdgeClientClientWithToken()
// - NewEdgeClientClientWithUpdb()
// - NewEdgeClientClientWithCert()
// - NewEdgeClientClientWithAuthenticator()
//
// `updb` and `cert` are supported with specific helper functions. Any authentication method not supported explicitly
// can use the ***Authenticator helper functions to implement other authentication methods.
//
// An example(s) is provided in the `examples` directory.
package rest_util

import (
	"crypto"
	"crypto/x509"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/openziti/edge/rest_client_api_client"
	"github.com/openziti/edge/rest_management_api_client"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
)

// NewEdgeManagementClientWithToken will generate a new rest_management_api_client.ZitiEdgeManagement client based
// upon a provided http.Client, controller address, and an API Session token that has been previously obtained.
func NewEdgeManagementClientWithToken(httpClient *http.Client, apiAddress string, apiSessionToken string) (*rest_management_api_client.ZitiEdgeManagement, error) {
	ctrlUrl, err := url.Parse(apiAddress)

	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(ctrlUrl.Host, rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = &ZitiTokenAuth{
		Token: apiSessionToken,
	}

	return rest_management_api_client.New(clientRuntime, nil), nil
}

// NewEdgeManagementClientWithUpdb will generate a new rest_management_api_client.ZitiEdgeManagement client based
// upon a provided http.Client, controller address, and will authenticate via username/password database (updb)
// to obtain an API Session token.
func NewEdgeManagementClientWithUpdb(username, password string, apiAddress string, rootCas *x509.CertPool) (*rest_management_api_client.ZitiEdgeManagement, error) {
	auth := NewAuthenticatorUpdb(username, password)
	auth.RootCas = rootCas
	return NewEdgeManagementClientWithAuthenticator(auth, apiAddress)
}

// NewEdgeManagementClientWithCert will generate a new rest_management_api_client.ZitiEdgeManagement client based
// upon a provided http.Client, controller address, and will authenticate via client certificate to obtain
// an API Session token.
func NewEdgeManagementClientWithCert(cert *x509.Certificate, privateKey crypto.PrivateKey, apiAddress string, rootCas *x509.CertPool) (*rest_management_api_client.ZitiEdgeManagement, error) {
	auth := NewAuthenticatorCert(cert, privateKey)
	auth.RootCas = rootCas
	return NewEdgeManagementClientWithAuthenticator(auth, apiAddress)
}

// NewEdgeManagementClientWithAuthenticator will generate a new rest_management_api_client.ZitiEdgeManagement client based
// upon a provided http.Client, controller address, and will authenticate with the provided Authenticator to obtain
// an API Session token.
func NewEdgeManagementClientWithAuthenticator(authenticator Authenticator, apiAddress string) (*rest_management_api_client.ZitiEdgeManagement, error) {
	ctrlUrl, err := url.Parse(apiAddress)

	if err != nil {
		return nil, err
	}

	apiSession, err := authenticator.Authenticate(ctrlUrl)

	if err != nil {
		return nil, err
	}

	if apiSession.Token == nil || *apiSession.Token == "" {
		return nil, errors.New("api session token was empty")
	}

	httpClient, err := authenticator.BuildHttpClient()

	return NewEdgeManagementClientWithToken(httpClient, apiAddress, *apiSession.Token)
}

// NewEdgeClientClientWithToken will generate a new rest_client_api_client.ZitiEdgeClient client based
// upon a provided http.Client, controller address, and an API Session token that has been previously obtained.
func NewEdgeClientClientWithToken(httpClient *http.Client, apiAddress string, apiSessionToken string) (*rest_client_api_client.ZitiEdgeClient, error) {
	ctrlUrl, err := url.Parse(apiAddress)

	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(ctrlUrl.Host, rest_client_api_client.DefaultBasePath, rest_client_api_client.DefaultSchemes, httpClient)

	clientRuntime.DefaultAuthentication = &ZitiTokenAuth{
		Token: apiSessionToken,
	}

	return rest_client_api_client.New(clientRuntime, nil), nil
}

// NewEdgeClientClientWithUpdb will generate a new rest_client_api_client.ZitiEdgeClient client based
// upon a provided http.Client, controller address, and will authenticate via username/password database (updb)
// to obtain an API Session token.
func NewEdgeClientClientWithUpdb(username, password string, apiAddress string, rootCas *x509.CertPool) (*rest_client_api_client.ZitiEdgeClient, error) {
	auth := NewAuthenticatorUpdb(username, password)
	auth.RootCas = rootCas
	return NewEdgeClientClientWithAuthenticator(auth, apiAddress)
}

// NewEdgeClientClientWithCert will generate a new rest_client_api_client.ZitiEdgeClient client based
// upon a provided http.Client, controller address, and will authenticate via client certificate to obtain
// an API Session token.
func NewEdgeClientClientWithCert(cert *x509.Certificate, privateKey crypto.PrivateKey, apiAddress string, rootCas *x509.CertPool) (*rest_client_api_client.ZitiEdgeClient, error) {
	auth := NewAuthenticatorCert(cert, privateKey)
	auth.RootCas = rootCas
	return NewEdgeClientClientWithAuthenticator(auth, apiAddress)
}

// NewEdgeClientClientWithAuthenticator will generate a new rest_client_api_client.ZitiEdgeClient client based
// upon a provided http.Client, controller address, and will authenticate with the provided Authenticator to obtain
// an API Session token.
func NewEdgeClientClientWithAuthenticator(authenticator Authenticator, apiAddress string) (*rest_client_api_client.ZitiEdgeClient, error) {
	ctrlUrl, err := url.Parse(apiAddress)

	if err != nil {
		return nil, err
	}

	apiSession, err := authenticator.Authenticate(ctrlUrl)

	if err != nil {
		return nil, err
	}

	if apiSession.Token == nil || *apiSession.Token == "" {
		return nil, errors.New("api session token was empty")
	}

	httpClient, err := authenticator.BuildHttpClient()

	return NewEdgeClientClientWithToken(httpClient, apiAddress, *apiSession.Token)
}
