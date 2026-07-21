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

package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const vTestTrustDomain = "spiffe://verify-test"

type vCertAndKey struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

var vSerial int64

func vNextSerial() *big.Int {
	vSerial++
	return big.NewInt(vSerial)
}

func vMkCA(t *testing.T, cn string, parent *vCertAndKey) *vCertAndKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          vNextSerial(),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	signParent, signKey := tmpl, key
	if parent != nil {
		signParent, signKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signParent, &key.PublicKey, signKey)
	require.NoError(t, err)
	c, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &vCertAndKey{cert: c, key: key}
}

// vMkLeaf builds an end-entity cert. signer==nil yields a leaf that does not chain to any CA. A nil
// ekus produces a cert with no ExtKeyUsage extension (unrestricted).
func vMkLeaf(t *testing.T, cn, spiffePath string, ekus []x509.ExtKeyUsage, signer *vCertAndKey) *vCertAndKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: vNextSerial(),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  ekus,
	}
	if spiffePath != "" {
		u, err := url.Parse(vTestTrustDomain + spiffePath)
		require.NoError(t, err)
		tmpl.URIs = []*url.URL{u}
	}
	signParent, signKey := tmpl, key
	if signer != nil {
		signParent, signKey = signer.cert, signer.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signParent, &key.PublicKey, signKey)
	require.NoError(t, err)
	c, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &vCertAndKey{cert: c, key: key}
}

func vPoolOf(certs ...*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return pool
}

// TestVerifyLeafCertChain_RejectsUnchainedLeaf verifies that a leaf which does not itself chain to a
// trusted CA is rejected even when another presented certificate does chain - i.e. verification is bound
// to the leaf, not to "some presented certificate verifies".
func TestVerifyLeafCertChain_RejectsUnchainedLeaf(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	roots := vPoolOf(root.cert, inter.cert)

	// A trust-anchored cert (chains to the pool) presented as an extra certificate.
	extra := vMkLeaf(t, "extra", "", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, inter)
	// The leaf itself is self-signed and does NOT chain to the pool.
	unchained := vMkLeaf(t, "unchained", "/identity/other", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil)

	_, err := VerifyLeafCertChain(roots, []*x509.Certificate{unchained.cert, extra.cert})
	req.Error(err, "leaf not chaining to the pool must be rejected even alongside a chaining extra cert")
}

func TestVerifyLeafCertChain_AcceptsLegitLeaf(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	roots := vPoolOf(root.cert, inter.cert)

	legit := vMkLeaf(t, "legit", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, inter)
	leaf, err := VerifyLeafCertChain(roots, []*x509.Certificate{legit.cert})
	req.NoError(err)
	req.Equal(legit.cert, leaf, "returns the verified leaf (certs[0]) for identity use")
}

func TestVerifyLeafCertChain_AcceptsPeerSuppliedIntermediate(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	legit := vMkLeaf(t, "legit", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, inter)

	// Pool holds ONLY the root; the intermediate must be supplied on the wire (certs[1:]).
	rootOnly := vPoolOf(root.cert)
	_, err := VerifyLeafCertChain(rootOnly, []*x509.Certificate{legit.cert})
	req.Error(err, "without the intermediate anywhere the leaf cannot be verified")
	_, err = VerifyLeafCertChain(rootOnly, []*x509.Certificate{legit.cert, inter.cert})
	req.NoError(err, "peer-supplied intermediate lets a valid peer verify")
}

// TestVerifyLeafCertChain_MultipleRoots covers a trust bundle with more than one self-signed root (as a
// quickstart deployment produces, concatenating a controller root and a signer root). A leaf chaining to
// either root must verify.
func TestVerifyLeafCertChain_MultipleRoots(t *testing.T) {
	req := require.New(t)
	ctrlRoot := vMkCA(t, "ctrl-root", nil)
	signerRoot := vMkCA(t, "signer-root", nil)
	signerInter := vMkCA(t, "signer-int", signerRoot)
	roots := vPoolOf(ctrlRoot.cert, signerRoot.cert)

	leaf := vMkLeaf(t, "router", "/identity/r1", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, signerInter)
	_, err := VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert, signerInter.cert})
	req.NoError(err, "a leaf chaining to one of several trusted roots must verify")
}

// TestVerifyLeafCertChain_IntermediateAsTrustAnchor covers a self-managed PKI that distributes a
// (non-self-signed) intermediate as the trust anchor without its root. Every certificate in the pool is
// a valid chain terminus, so a leaf chaining directly to that intermediate must verify.
func TestVerifyLeafCertChain_IntermediateAsTrustAnchor(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	leaf := vMkLeaf(t, "leaf", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, inter)

	interAnchored := vPoolOf(inter.cert) // only the intermediate distributed, no self-signed root
	_, err := VerifyLeafCertChain(interAnchored, []*x509.Certificate{leaf.cert})
	req.NoError(err, "an intermediate distributed as a trust anchor must be accepted as a chain terminus")
}

// TestVerifyLeafCertChain_ArbitraryEKU covers external PKIs whose certificates carry arbitrary or absent
// extended key usages. No EKU restriction is applied, so all of them verify as long as the chain is
// valid.
func TestVerifyLeafCertChain_ArbitraryEKU(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	roots := vPoolOf(root.cert, inter.cert)

	for _, ekus := range [][]x509.ExtKeyUsage{
		nil,
		{x509.ExtKeyUsageClientAuth},
		{x509.ExtKeyUsageServerAuth},
		{x509.ExtKeyUsageEmailProtection},
	} {
		leaf := vMkLeaf(t, "leaf", "/identity/real", ekus, inter)
		_, err := VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert})
		req.NoError(err, "leaf with EKU %v must be accepted", ekus)
	}
}

func TestVerifyLeafCertChain_EmptyInputs(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	roots := vPoolOf(root.cert)

	_, err := VerifyLeafCertChain(nil, []*x509.Certificate{root.cert})
	req.Error(err, "nil pool rejected")
	_, err = VerifyLeafCertChain(roots, nil)
	req.Error(err, "no certs rejected")
}
