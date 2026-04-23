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
	"math/big"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// newTestCA creates a self-signed CA certificate and key for testing.
func newTestCA(t *testing.T, cn string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	certDer, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := x509.ParseCertificate(certDer)
	if err != nil {
		t.Fatal(err)
	}

	return cert, key
}

// newTestLeaf creates a leaf certificate signed by the given CA.
func newTestLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDer, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := x509.ParseCertificate(certDer)
	if err != nil {
		t.Fatal(err)
	}

	return cert
}

func newTestTrustCache(t *testing.T, firstPartyCa *x509.Certificate, thirdPartyCa *x509.Certificate) *TrustCache {
	t.Helper()

	tc := &TrustCache{}

	if firstPartyCa != nil {
		tc.staticFirstPartyRootPool = x509.NewCertPool()
		tc.staticFirstPartyRootPool.AddCert(firstPartyCa)
		tc.staticFirstPartyTrustAnchorPool = x509.NewCertPool()
		tc.staticFirstPartyTrustAnchorPool.AddCert(firstPartyCa)
	}

	if thirdPartyCa != nil {
		tc.thirdPartyTrustAnchorPool = x509.NewCertPool()
		tc.thirdPartyTrustAnchorPool.AddCert(thirdPartyCa)
	}

	tc.certOriginCache = ttlcache.New[string, CertOrigin](
		ttlcache.WithTTL[string, CertOrigin](certVerifyCacheTTL),
	)

	return tc
}

func TestTrustCache_VerifyClientCert_FirstParty(t *testing.T) {
	ca, caKey := newTestCA(t, "first-party-ca")
	leaf := newTestLeaf(t, ca, caKey, "client")
	tc := newTestTrustCache(t, ca, nil)

	result := tc.VerifyClientCert([]*x509.Certificate{leaf}, false)
	if result != CertOriginFirstParty {
		t.Errorf("expected CertOriginFirstParty, got %v", result)
	}
}

func TestTrustCache_VerifyClientCert_ThirdParty(t *testing.T) {
	firstPartyCa, _ := newTestCA(t, "first-party-ca")
	thirdPartyCa, thirdPartyKey := newTestCA(t, "third-party-ca")
	leaf := newTestLeaf(t, thirdPartyCa, thirdPartyKey, "client")
	tc := newTestTrustCache(t, firstPartyCa, thirdPartyCa)

	result := tc.VerifyClientCert([]*x509.Certificate{leaf}, false)
	if result != CertOriginThirdParty {
		t.Errorf("expected CertOriginThirdParty, got %v", result)
	}
}

func TestTrustCache_VerifyClientCert_Untrusted(t *testing.T) {
	firstPartyCa, _ := newTestCA(t, "first-party-ca")
	untrustedCa, untrustedKey := newTestCA(t, "untrusted-ca")
	leaf := newTestLeaf(t, untrustedCa, untrustedKey, "client")
	tc := newTestTrustCache(t, firstPartyCa, nil)

	result := tc.VerifyClientCert([]*x509.Certificate{leaf}, false)
	if result != CertOriginUntrusted {
		t.Errorf("expected CertOriginUntrusted, got %v", result)
	}
}

func TestTrustCache_VerifyClientCert_NoCerts(t *testing.T) {
	tc := newTestTrustCache(t, nil, nil)

	result := tc.VerifyClientCert(nil, false)
	if result != CertOriginUntrusted {
		t.Errorf("expected CertOriginUntrusted for nil certs, got %v", result)
	}

	result = tc.VerifyClientCert([]*x509.Certificate{}, false)
	if result != CertOriginUntrusted {
		t.Errorf("expected CertOriginUntrusted for empty certs, got %v", result)
	}
}

func TestTrustCache_VerifyClientCertCached(t *testing.T) {
	ca, caKey := newTestCA(t, "first-party-ca")
	leaf := newTestLeaf(t, ca, caKey, "client")
	tc := newTestTrustCache(t, ca, nil)

	fingerprint := "test-fingerprint"

	// First call: cache miss, should verify
	result := tc.VerifyClientCertCached(fingerprint, []*x509.Certificate{leaf}, false)
	if result != CertOriginFirstParty {
		t.Errorf("expected CertOriginFirstParty, got %v", result)
	}

	// Verify it's cached
	item := tc.certOriginCache.Get(fingerprint)
	if item == nil {
		t.Fatal("expected cache entry to exist")
	}
	if item.Value() != CertOriginFirstParty {
		t.Errorf("expected cached CertOriginFirstParty, got %v", item.Value())
	}

	// Second call: should use cache. We verify by removing pools and checking it still works.
	tc.staticFirstPartyRootPool = nil
	tc.staticFirstPartyTrustAnchorPool = nil
	result = tc.VerifyClientCertCached(fingerprint, []*x509.Certificate{leaf}, false)
	if result != CertOriginFirstParty {
		t.Errorf("expected CertOriginFirstParty from cache, got %v", result)
	}
}

func TestTrustCache_VerifyClientCertCached_UntrustedNotCached(t *testing.T) {
	firstPartyCa, _ := newTestCA(t, "first-party-ca")
	untrustedCa, untrustedKey := newTestCA(t, "untrusted-ca")
	leaf := newTestLeaf(t, untrustedCa, untrustedKey, "client")
	tc := newTestTrustCache(t, firstPartyCa, nil)

	fingerprint := "untrusted-fingerprint"

	result := tc.VerifyClientCertCached(fingerprint, []*x509.Certificate{leaf}, false)
	if result != CertOriginUntrusted {
		t.Errorf("expected CertOriginUntrusted, got %v", result)
	}

	// Untrusted results should NOT be cached
	item := tc.certOriginCache.Get(fingerprint)
	if item != nil {
		t.Error("untrusted results should not be cached")
	}
}
