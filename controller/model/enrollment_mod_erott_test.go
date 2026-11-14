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
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_routerCertSigningOpts(t *testing.T) {
	mustParse := func(uri string) *url.URL {
		parsed, err := url.Parse(uri)
		require.NoError(t, err)
		return parsed
	}

	csr := &x509.CertificateRequest{
		DNSNames:       []string{"router.example.com"},
		EmailAddresses: []string{"router@example.com"},
		IPAddresses:    []net.IP{net.ParseIP("192.0.2.1")},
		URIs: []*url.URL{
			mustParse("spiffe://trust-domain/controller/ctrl1"),
			mustParse("ziti://router/made/up/example"),
			mustParse("SPIFFE://trust-domain/identity/id1"),
		},
	}

	opts := routerCertSigningOpts(csr)

	require.Equal(t, csr.DNSNames, opts.DNSNames)
	require.Equal(t, csr.EmailAddresses, opts.EmailAddresses)
	require.Equal(t, csr.IPAddresses, opts.IPAddresses)
	require.Len(t, opts.URIs, 1, "SPIFFE ids must not be taken from the CSR")
	require.Equal(t, "ziti://router/made/up/example", opts.URIs[0].String())
}
