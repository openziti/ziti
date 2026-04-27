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

package oidc_auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCert creates a self-signed certificate with the given SAN URIs.
func testCert(t *testing.T, uris ...*url.URL) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         uris,
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(raw)
	require.NoError(t, err)
	return cert
}

// testCertFingerprint returns the SHA-1 fingerprint of a certificate in the same hex format
// used by the production code.
func testCertFingerprint(cert *x509.Certificate) string {
	return fmt.Sprintf("%x", sha1.Sum(cert.Raw))
}

func Test_verifyByFingerprint(t *testing.T) {
	certA := testCert(t)
	certB := testCert(t)
	fpA := testCertFingerprint(certA)
	fpB := testCertFingerprint(certB)

	t.Run("matching fingerprint passes", func(t *testing.T) {
		err := verifyByFingerprint(certA, []string{fpA})
		require.NoError(t, err)
	})

	t.Run("one of multiple allowed fingerprints matches", func(t *testing.T) {
		err := verifyByFingerprint(certB, []string{fpA, fpB})
		require.NoError(t, err)
	})

	t.Run("no matching fingerprint fails", func(t *testing.T) {
		err := verifyByFingerprint(certA, []string{fpB})
		require.Error(t, err)
		require.Contains(t, err.Error(), "fingerprint")
	})

	t.Run("nil leaf cert fails", func(t *testing.T) {
		err := verifyByFingerprint(nil, []string{fpA})
		require.Error(t, err)
	})
}

func Test_verifyBySpiffeId(t *testing.T) {
	const (
		identityId   = "id-001"
		apiSessionId = "as-002"
		certId       = "cert-003"
	)

	spiffeURL := &url.URL{
		Scheme: "spiffe",
		Host:   "example.trust.domain",
		Path:   fmt.Sprintf("identity/%s/apiSession/%s/apiSessionCertificate/%s", identityId, apiSessionId, certId),
	}

	t.Run("matching SPIFFE ID passes", func(t *testing.T) {
		cert := testCert(t, spiffeURL)
		err := verifyBySpiffeId(cert, apiSessionId, identityId)
		require.NoError(t, err)
	})

	t.Run("wrong apiSessionId fails", func(t *testing.T) {
		cert := testCert(t, spiffeURL)
		err := verifyBySpiffeId(cert, "wrong-session", identityId)
		require.Error(t, err)
		require.Contains(t, err.Error(), "SPIFFE")
	})

	t.Run("wrong identityId fails", func(t *testing.T) {
		cert := testCert(t, spiffeURL)
		err := verifyBySpiffeId(cert, apiSessionId, "wrong-identity")
		require.Error(t, err)
	})

	t.Run("non-spiffe URI fails", func(t *testing.T) {
		httpURL := &url.URL{
			Scheme: "https",
			Host:   "example.com",
			Path:   fmt.Sprintf("identity/%s/apiSession/%s/apiSessionCertificate/%s", identityId, apiSessionId, certId),
		}
		cert := testCert(t, httpURL)
		err := verifyBySpiffeId(cert, apiSessionId, identityId)
		require.Error(t, err)
	})

	t.Run("nil leaf cert passes (not applicable)", func(t *testing.T) {
		err := verifyBySpiffeId(nil, apiSessionId, identityId)
		require.NoError(t, err)
	})

	t.Run("empty apiSessionId passes (not applicable)", func(t *testing.T) {
		cert := testCert(t)
		err := verifyBySpiffeId(cert, "", identityId)
		require.NoError(t, err)
	})

	t.Run("cert with multiple URIs matches on spiffe", func(t *testing.T) {
		otherURL := &url.URL{Scheme: "https", Host: "example.com", Path: "/other"}
		cert := testCert(t, otherURL, spiffeURL)
		err := verifyBySpiffeId(cert, apiSessionId, identityId)
		require.NoError(t, err)
	})
}

func Test_verifyCertBinding(t *testing.T) {
	const (
		identityId   = "id-100"
		apiSessionId = "as-200"
	)

	spiffeURL := &url.URL{
		Scheme: "spiffe",
		Host:   "trust.domain",
		Path:   fmt.Sprintf("identity/%s/apiSession/%s/apiSessionCertificate/cert-300", identityId, apiSessionId),
	}

	certWithSpiffe := testCert(t, spiffeURL)
	certPlain := testCert(t)
	fpSpiffe := testCertFingerprint(certWithSpiffe)

	t.Run("z_cfs present uses strict fingerprint check", func(t *testing.T) {
		err := verifyCertBinding(certWithSpiffe, apiSessionId, identityId, []string{fpSpiffe})
		require.NoError(t, err)
	})

	t.Run("z_cfs present with wrong cert fails even if SPIFFE matches", func(t *testing.T) {
		err := verifyCertBinding(certPlain, apiSessionId, identityId, []string{fpSpiffe})
		require.Error(t, err)
	})

	t.Run("no z_cfs falls back to SPIFFE verification", func(t *testing.T) {
		err := verifyCertBinding(certWithSpiffe, apiSessionId, identityId, nil)
		require.NoError(t, err)
	})

	t.Run("no z_cfs and no SPIFFE match fails", func(t *testing.T) {
		err := verifyCertBinding(certPlain, apiSessionId, identityId, nil)
		require.Error(t, err)
	})

	t.Run("no z_cfs and nil leaf cert passes", func(t *testing.T) {
		err := verifyCertBinding(nil, apiSessionId, identityId, nil)
		require.NoError(t, err)
	})
}

func Test_tlsLeafCert(t *testing.T) {
	t.Run("nil request returns nil", func(t *testing.T) {
		require.Nil(t, tlsLeafCert(nil))
	})

	t.Run("request without TLS returns nil", func(t *testing.T) {
		r := &http.Request{}
		require.Nil(t, tlsLeafCert(r))
	})

	t.Run("request with empty peer certs returns nil", func(t *testing.T) {
		r := &http.Request{
			TLS: &tls.ConnectionState{},
		}
		require.Nil(t, tlsLeafCert(r))
	})

	t.Run("request with TLS returns leaf cert", func(t *testing.T) {
		leaf := testCert(t)
		intermediate := testCert(t)
		r := &http.Request{
			TLS: &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{leaf, intermediate},
			},
		}
		result := tlsLeafCert(r)
		require.Equal(t, leaf, result)
	})
}
