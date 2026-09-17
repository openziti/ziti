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

package model

import (
	"crypto/x509"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// proxiedAuthContext builds the auth context the way NewAuthContextHttp does, with header
// names in the canonical HTTP form and values as string slices.
func proxiedAuthContext(peerCerts []*x509.Certificate, header http.Header) AuthContext {
	headers := Headers{}
	for name, values := range header {
		headers.Set(name, values)
	}

	return &AuthContextHttp{
		Method:  "cert",
		Certs:   peerCerts,
		Headers: headers,
	}
}

func Test_GetClientCerts_ProxyHeaderIsMatchedCaseInsensitively(t *testing.T) {
	req := require.New(t)

	module := &AuthModuleCert{}
	peerCert := &x509.Certificate{Raw: []byte{0x01}}

	header := http.Header{}
	header.Set(EdgeRouterProxyRequest, "now")

	// No chain came with the proxy marker, so the proxied path refuses. Getting the peer certs
	// back instead would mean the marker was never seen.
	certs, err := module.getClientCerts(proxiedAuthContext([]*x509.Certificate{peerCert}, header))

	req.Error(err)
	req.Nil(certs)
}

func Test_GetClientCerts_NoProxyHeaderUsesPeerCerts(t *testing.T) {
	req := require.New(t)

	module := &AuthModuleCert{}
	peerCert := &x509.Certificate{Raw: []byte{0x01}}

	certs, err := module.getClientCerts(proxiedAuthContext([]*x509.Certificate{peerCert}, http.Header{}))

	req.NoError(err)
	req.Equal([]*x509.Certificate{peerCert}, certs)
}

func Test_GetProxiedClientCerts_RefusesMoreThanOneChainValue(t *testing.T) {
	req := require.New(t)

	module := &AuthModuleCert{}
	peerCert := &x509.Certificate{Raw: []byte{0x01}}

	header := http.Header{}
	header.Set(EdgeRouterProxyRequest, "now")
	header.Add(ClientCertHeader, "Zm9yZ2Vk")
	header.Add(ClientCertHeader, "cm91dGVy")

	// A router that strips client values sends exactly one. Two means the request came through
	// something that does not, and the router's chain cannot be told from the client's.
	certs, err := module.getProxiedClientCerts(proxiedAuthContext([]*x509.Certificate{peerCert}, header))

	req.Error(err)
	req.Nil(certs)
}

func Test_GetProxiedClientCerts_NoChainValue(t *testing.T) {
	req := require.New(t)

	module := &AuthModuleCert{}
	peerCert := &x509.Certificate{Raw: []byte{0x01}}

	header := http.Header{}
	header.Set(EdgeRouterProxyRequest, "now")

	certs, err := module.getProxiedClientCerts(proxiedAuthContext([]*x509.Certificate{peerCert}, header))

	req.Error(err)
	req.Nil(certs)
}

func Test_GetProxiedClientCerts_NoPeerCerts(t *testing.T) {
	req := require.New(t)

	module := &AuthModuleCert{}

	header := http.Header{}
	header.Set(EdgeRouterProxyRequest, "now")
	header.Set(ClientCertHeader, "Zm9yZ2Vk")

	certs, err := module.getProxiedClientCerts(proxiedAuthContext(nil, header))

	req.Error(err)
	req.Nil(certs)
}
