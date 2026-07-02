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

package tests

import (
	"crypto/x509"
	"net/http"
	"net/url"
	"strings"

	httptransport "github.com/go-openapi/runtime/client"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	fabricRestClient "github.com/openziti/ziti/v2/controller/rest_client"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"
)

// RestClients provides typed access to a controller's edge management and fabric
// REST APIs for integration tests. It is the tests-package counterpart to the
// zitirest shim, kept here so zitirest need not be exposed as a public API. Edge
// access is backed by sdk-golang's edge_apis client; the fabric REST client is
// constructed locally because sdk-golang has no equivalent.
type RestClients struct {
	Edge   *edge_apis.ZitiEdgeManagement
	Fabric *fabricRestClient.ZitiFabric
}

// newRestClients builds edge management and fabric REST clients for the given
// controller host, authenticated with a pre-acquired legacy session token. The
// token comes from the test's existing management API login, so no additional
// authentication flow is run here.
func newRestClients(host string, token string) (*RestClients, error) {
	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	caPool, err := loadWellKnownCertPool(host)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(host)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse host URL '%v'", host)
	}

	apiUrl := &url.URL{
		Scheme: parsedHost.Scheme,
		Host:   parsedHost.Host,
		Path:   "/edge/management/v1",
	}

	mgmt := edge_apis.NewManagementApiClient([]*url.URL{apiUrl}, caPool, nil)

	// The token was obtained via the legacy management API login, so install it
	// as a legacy (zt-session) api session rather than running an OIDC flow.
	mgmt.SetUseOidc(false)
	var session edge_apis.ApiSession = edge_apis.NewApiSessionLegacy(token)
	mgmt.ApiSession.Store(&session)

	fabricRuntime := httptransport.NewWithClient(parsedHost.Host,
		fabricRestClient.DefaultBasePath, fabricRestClient.DefaultSchemes, mgmt.HttpClient)
	fabricRuntime.DefaultAuthentication = mgmt
	fabric := fabricRestClient.New(fabricRuntime, nil)

	return &RestClients{
		Edge:   mgmt.API,
		Fabric: fabric,
	}, nil
}

// loadWellKnownCertPool fetches the controller's well-known certs and verifies the
// controller serves them itself before returning a CertPool, preserving the trust
// check the zitirest clients performed rather than blindly trusting the well-known
// endpoint.
func loadWellKnownCertPool(host string) (*x509.CertPool, error) {
	wellKnownCerts, _, err := util.GetWellKnownCerts(host, http.Client{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to retrieve server certificate authority from %v", host)
	}

	trusted, err := util.AreCertsTrusted(host, wellKnownCerts, http.Client{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to verify well known certs for host %v", host)
	}
	if !trusted {
		return nil, errors.New("server supplied certs not trusted by server, unable to continue")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(wellKnownCerts) {
		return nil, errors.New("failed to append well-known certs to pool")
	}
	return pool, nil
}
