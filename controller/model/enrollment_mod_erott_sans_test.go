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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/cert"
	"github.com/stretchr/testify/require"
)

func mustParseUrls(t *testing.T, raw ...string) []*url.URL {
	t.Helper()

	var result []*url.URL
	for _, r := range raw {
		parsed, err := url.Parse(r)
		require.NoError(t, err)
		result = append(result, parsed)
	}

	return result
}

func urlStrings(urls []*url.URL) []string {
	var result []string
	for _, u := range urls {
		result = append(result, u.String())
	}
	return result
}

func Test_DropSpiffeIds(t *testing.T) {
	req := require.New(t)

	uris := mustParseUrls(t,
		"https://example.test/ok",
		"spiffe://trust-domain/identity/someone-else",
		"SPIFFE://trust-domain/identity/someone-else",
		"urn:example:keep",
	)

	result := dropSpiffeIds(uris)

	req.Equal([]string{"https://example.test/ok", "urn:example:keep"}, urlStrings(result))
}

func Test_DropSpiffeIds_NilEntries(t *testing.T) {
	req := require.New(t)

	req.Nil(dropSpiffeIds(nil))
	req.Nil(dropSpiffeIds([]*url.URL{nil}))
}

// signerEnvStub supplies the one Env method the CSR processing touches.
type signerEnvStub struct {
	*TestContext
	signer cert.Signer
}

func (self *signerEnvStub) GetControlClientCsrSigner() cert.Signer {
	return self.signer
}

func (self *signerEnvStub) GetApiServerCsrSigner() cert.Signer {
	return self.signer
}

// newTestSigner builds a self-signed CA and a client signer using it.
func newTestSigner(t *testing.T) cert.Signer {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert.NewClientSigner(caCert, caKey)
}

// newTestCsrPem writes a CSR asking for the given common name and URI SANs.
func newTestCsrPem(t *testing.T, commonName string, uris []*url.URL) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: commonName},
		URIs:     uris,
		DNSNames: []string{"router.example.test"},
	}, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func Test_ProcessClientCsrPem_DoesNotSignSpiffeIdFromCsr(t *testing.T) {
	req := require.New(t)

	routerId := "router-1"
	module := &EnrollModuleEr{
		env:                  &signerEnvStub{signer: newTestSigner(t)},
		method:               MethodEnrollEdgeRouterOtt,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	// The CSR asks for a SPIFFE id naming another identity. Signing it would produce a
	// network-issued certificate that routers and the controller accept as that identity.
	csrPem := newTestCsrPem(t, routerId, mustParseUrls(t, "spiffe://trust-domain/identity/victim"))

	raw, err := module.ProcessClientCsrPem(csrPem, routerId)
	req.NoError(err)

	signed, err := x509.ParseCertificate(raw)
	req.NoError(err)

	req.Empty(signed.URIs, "a SPIFFE id from a router CSR must not end up in the signed certificate")
	req.Equal([]string{"router.example.test"}, signed.DNSNames, "the router still names where it can be reached")
}

func Test_ProcessServerCsrPem_DoesNotSignSpiffeIdFromCsr(t *testing.T) {
	req := require.New(t)

	module := &EnrollModuleEr{
		env:                  &signerEnvStub{signer: newTestSigner(t)},
		method:               MethodEnrollEdgeRouterOtt,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	csrPem := newTestCsrPem(t, "router-1", mustParseUrls(t, "spiffe://trust-domain/identity/victim"))

	raw, err := module.ProcessServerCsrPem(csrPem)
	req.NoError(err)

	signed, err := x509.ParseCertificate(raw)
	req.NoError(err)

	req.Empty(signed.URIs)
	req.Equal([]string{"router.example.test"}, signed.DNSNames)
}
