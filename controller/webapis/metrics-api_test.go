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

package webapis

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var mSerial int64

func mMkCert(t *testing.T, cn string, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	mSerial++
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(mSerial),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	c, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return c
}

func mScrapeRequest(peerCerts []*x509.Certificate) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if peerCerts != nil {
		r.TLS = &tls.ConnectionState{PeerCertificates: peerCerts}
	}
	return r
}

// Test_MetricsApi_ScrapeCertAuthorization covers the scrape-cert gate on the metrics endpoint. When a
// scrape cert is pinned, authorization must match the pinned cert against the presented LEAF only
// (PeerCertificates[0]), compare the full certificate, honor the validity window, and reject requests
// with no client certificate. Only the rejecting (401) paths are exercised here; they return before the
// handler consults the network.
func Test_MetricsApi_ScrapeCertAuthorization(t *testing.T) {
	now := time.Now()
	scrapeCert := mMkCert(t, "scrape", now.Add(-time.Hour), now.Add(time.Hour))

	handler := (&MetricsApiHandler{scrapeCert: scrapeCert}).newHandler()

	assertUnauthorized := func(t *testing.T, peerCerts []*x509.Certificate) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, mScrapeRequest(peerCerts))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	t.Run("no client certificate", func(t *testing.T) {
		assertUnauthorized(t, nil)
	})

	t.Run("presented leaf is not the scrape cert", func(t *testing.T) {
		other := mMkCert(t, "other", now.Add(-time.Hour), now.Add(time.Hour))
		assertUnauthorized(t, []*x509.Certificate{other})
	})

	t.Run("scrape cert presented only as a filler cert, not the leaf", func(t *testing.T) {
		attacker := mMkCert(t, "attacker", now.Add(-time.Hour), now.Add(time.Hour))
		// The pinned scrape cert is present in the chain, but the leaf (index 0) is the attacker's.
		assertUnauthorized(t, []*x509.Certificate{attacker, scrapeCert})
	})

	t.Run("expired scrape cert presented as the leaf", func(t *testing.T) {
		expired := mMkCert(t, "expired", now.Add(-2*time.Hour), now.Add(-time.Hour))
		expiredHandler := (&MetricsApiHandler{scrapeCert: expired}).newHandler()
		rec := httptest.NewRecorder()
		expiredHandler.ServeHTTP(rec, mScrapeRequest([]*x509.Certificate{expired}))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "a leaf outside its validity window must be rejected")
	})
}
