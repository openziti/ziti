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

	"github.com/openziti/identity"
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

// TestVerifyClientCertChain_RejectsUnchainedLeaf verifies that a leaf which does not itself chain to
// the CA is rejected even when another presented certificate does chain - i.e. verification is bound
// to the leaf, not to "some presented certificate verifies".
func TestVerifyClientCertChain_RejectsUnchainedLeaf(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	pool := identity.NewCaPool([]*x509.Certificate{root.cert, inter.cert})

	// A trust-domain-signed cert (chains to the CA) presented as an extra certificate.
	extra := vMkLeaf(t, "extra", "", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, inter)
	// The leaf itself is self-signed and does NOT chain to the CA.
	unchained := vMkLeaf(t, "unchained", "/identity/other", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil)

	_, err := VerifyClientCertChain(pool, []*x509.Certificate{unchained.cert, extra.cert})
	req.Error(err, "leaf not chaining to the CA must be rejected even alongside a chaining extra cert")
}

func TestVerifyClientCertChain_AcceptsLegitLeaf(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	pool := identity.NewCaPool([]*x509.Certificate{root.cert, inter.cert})

	legit := vMkLeaf(t, "legit", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, inter)
	leaf, err := VerifyClientCertChain(pool, []*x509.Certificate{legit.cert})
	req.NoError(err)
	req.Equal(legit.cert, leaf, "returns the verified leaf (certs[0]) for identity use")
}

func TestVerifyClientCertChain_AcceptsPeerSuppliedIntermediate(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	legit := vMkLeaf(t, "legit", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, inter)

	// Pool holds ONLY the root; the intermediate must be supplied on the wire (certs[1:]).
	rootOnly := identity.NewCaPool([]*x509.Certificate{root.cert})
	_, err := VerifyClientCertChain(rootOnly, []*x509.Certificate{legit.cert})
	req.Error(err, "without the intermediate anywhere the leaf cannot be verified")
	_, err = VerifyClientCertChain(rootOnly, []*x509.Certificate{legit.cert, inter.cert})
	req.NoError(err, "peer-supplied intermediate lets a valid peer verify")
}

func TestVerifyClientCertChain_EKUCompat(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	pool := identity.NewCaPool([]*x509.Certificate{root.cert, inter.cert})

	// No ExtKeyUsage extension (the form ziti pki produces) is unrestricted -> accepted.
	noEKU := vMkLeaf(t, "ziti-pki", "/identity/real", nil, inter)
	_, err := VerifyClientCertChain(pool, []*x509.Certificate{noEKU.cert})
	req.NoError(err, "no-EKU leaf accepted (ziti pki backward compat)")

	// EKU present but excludes client auth (server-auth only) -> rejected.
	serverOnly := vMkLeaf(t, "server-only", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, inter)
	_, err = VerifyClientCertChain(pool, []*x509.Certificate{serverOnly.cert})
	req.Error(err, "server-auth-only leaf rejected by the client-auth requirement")
}

func TestVerifyClientCertChain_EmptyInputs(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	pool := identity.NewCaPool([]*x509.Certificate{root.cert})

	_, err := VerifyClientCertChain(nil, []*x509.Certificate{root.cert})
	req.Error(err, "nil pool rejected")
	_, err = VerifyClientCertChain(pool, nil)
	req.Error(err, "no certs rejected")
}

// TestVerifyCertChain_DirectionAwareEKU verifies that the client-auth and server-auth variants each
// enforce their own extended key usage. An external PKI may issue separate client-auth and server-auth
// certificates; verifying with the wrong direction (e.g. requiring client auth of a peer's server
// certificate on an outbound connection) would reject a legitimate peer. A leaf carrying both usages,
// or none at all, satisfies either variant.
func TestVerifyCertChain_DirectionAwareEKU(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	pool := identity.NewCaPool([]*x509.Certificate{root.cert, inter.cert})

	clientOnly := vMkLeaf(t, "client-only", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, inter)
	serverOnly := vMkLeaf(t, "server-only", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, inter)
	both := vMkLeaf(t, "both", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, inter)
	none := vMkLeaf(t, "none", "/identity/real", nil, inter)

	_, err := VerifyClientCertChain(pool, []*x509.Certificate{clientOnly.cert})
	req.NoError(err, "client-auth leaf accepted for inbound direction")
	_, err = VerifyClientCertChain(pool, []*x509.Certificate{serverOnly.cert})
	req.Error(err, "server-auth-only leaf rejected for inbound direction")

	_, err = VerifyServerCertChain(pool, []*x509.Certificate{serverOnly.cert})
	req.NoError(err, "server-auth leaf accepted for outbound direction")
	_, err = VerifyServerCertChain(pool, []*x509.Certificate{clientOnly.cert})
	req.Error(err, "client-auth-only leaf rejected for outbound direction")

	// A leaf carrying both usages, or none, satisfies either direction (the common ziti pki case).
	for _, c := range []*x509.Certificate{both.cert, none.cert} {
		_, err = VerifyClientCertChain(pool, []*x509.Certificate{c})
		req.NoError(err)
		_, err = VerifyServerCertChain(pool, []*x509.Certificate{c})
		req.NoError(err)
	}
}
