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

package apiproxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func requestWithPeerCerts(certs ...*x509.Certificate) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/edge/client/v1/authenticate", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: certs}
	return req
}

func Test_SetProxyHeaders_DropsClientSuppliedCertHeader(t *testing.T) {
	req := require.New(t)

	r := requestWithPeerCerts()
	r.Header.Set(clientCertHeader, "Zm9yZ2Vk")
	r.Header.Set(edgeRouterProxyRequest, "whenever")

	setProxyHeaders(r)

	req.Empty(r.Header.Values(clientCertHeader), "a client must not be able to supply its own chain")
	req.Len(r.Header.Values(edgeRouterProxyRequest), 1)
	req.NotEqual("whenever", r.Header.Get(edgeRouterProxyRequest))
}

func Test_SetProxyHeaders_ReplacesClientValuesWithHandshakeChain(t *testing.T) {
	req := require.New(t)

	leaf := &x509.Certificate{Raw: []byte{0x01, 0x02}}
	intermediate := &x509.Certificate{Raw: []byte{0x03}}

	r := requestWithPeerCerts(leaf, intermediate)
	r.Header.Add(clientCertHeader, "Zm9yZ2VkMQ==")
	r.Header.Add(clientCertHeader, "Zm9yZ2VkMg==")

	setProxyHeaders(r)

	values := r.Header.Values(clientCertHeader)
	req.Len(values, 1, "the chain goes out as exactly one value so the controller can attribute it")
	req.Equal(base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}), values[0])
}

func Test_SetProxyHeaders_NoPeerCerts(t *testing.T) {
	req := require.New(t)

	r := requestWithPeerCerts()
	r.Header.Set(clientCertHeader, "Zm9yZ2Vk")

	setProxyHeaders(r)

	req.Empty(r.Header.Values(clientCertHeader))
	req.NotEmpty(r.Header.Get(edgeRouterProxyRequest))
}

func Test_SetProxyHeaders_NoTlsState(t *testing.T) {
	req := require.New(t)

	r := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	r.TLS = nil
	r.Header.Set(clientCertHeader, "Zm9yZ2Vk")

	req.NotPanics(func() { setProxyHeaders(r) })
	req.Empty(r.Header.Values(clientCertHeader))
}
