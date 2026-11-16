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

// TestVerifyLeafCertChain_AdditionalRoots covers a controller whose edge signing CA sits outside its own
// trust bundle: a router presents an enrollment certificate issued by that CA, so the leaf chains only
// once the signing bundle is supplied as an additional anchor.
func TestVerifyLeafCertChain_AdditionalRoots(t *testing.T) {
	req := require.New(t)
	ctrlRoot := vMkCA(t, "ctrl-root", nil)
	signingRoot := vMkCA(t, "signing-root", nil)
	signingInter := vMkCA(t, "signing-int", signingRoot)
	roots := vPoolOf(ctrlRoot.cert)
	leaf := vMkLeaf(t, "router", "/identity/r1", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, signingInter)

	_, err := VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert, signingInter.cert})
	req.Error(err, "without the signing bundle the leaf chains to nothing trusted")

	_, err = VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert, signingInter.cert}, signingRoot.cert)
	req.NoError(err, "the signing root supplied as an additional anchor lets the leaf verify")

	_, err = VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert}, signingInter.cert)
	req.NoError(err, "an intermediate from the signing bundle is a terminus like any other anchor")

	ctrlLeaf := vMkLeaf(t, "ctrl-signed", "/identity/r2", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, ctrlRoot)
	_, err = VerifyLeafCertChain(roots, []*x509.Certificate{ctrlLeaf.cert}, signingRoot.cert)
	req.NoError(err, "additional anchors do not displace the caller's own pool")
}

// TestVerifyLeafCertChain_AdditionalRootsDoNotMutateCallerPool guards the caller's pool, which an identity
// shares with the live tls.Configs it has handed out: anchors passed for one check must not become
// permanently trusted.
func TestVerifyLeafCertChain_AdditionalRootsDoNotMutateCallerPool(t *testing.T) {
	req := require.New(t)
	ctrlRoot := vMkCA(t, "ctrl-root", nil)
	signingRoot := vMkCA(t, "signing-root", nil)
	roots := vPoolOf(ctrlRoot.cert)
	leaf := vMkLeaf(t, "router", "/identity/r1", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, signingRoot)

	_, err := VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert}, signingRoot.cert)
	req.NoError(err)

	_, err = VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert})
	req.Error(err, "the signing root must not have been added to the caller's pool")
}

// TestVerifyClientLeafCertChain_RequiresClientAuth covers the client-certificate variant: a leaf whose
// extended key usage extension is present must permit client authentication, while a leaf without the
// extension (as issued by `ziti pki`) stays unrestricted.
func TestVerifyClientLeafCertChain_RequiresClientAuth(t *testing.T) {
	root := vMkCA(t, "root", nil)
	inter := vMkCA(t, "int", root)
	roots := vPoolOf(root.cert, inter.cert)

	for _, test := range []struct {
		name     string
		ekus     []x509.ExtKeyUsage
		accepted bool
	}{
		{name: "no eku extension", ekus: nil, accepted: true},
		{name: "clientAuth", ekus: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, accepted: true},
		{name: "clientAuth and serverAuth", ekus: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, accepted: true},
		{name: "anyExtendedKeyUsage", ekus: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, accepted: true},
		{name: "serverAuth only", ekus: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, accepted: false},
		{name: "emailProtection only", ekus: []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection}, accepted: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := require.New(t)
			leaf := vMkLeaf(t, "leaf", "/identity/real", test.ekus, inter)

			verified, err := VerifyClientLeafCertChain(roots, []*x509.Certificate{leaf.cert})
			if test.accepted {
				req.NoError(err)
				req.Same(leaf.cert, verified)
			} else {
				req.Error(err)
				req.Nil(verified)
			}

			_, err = VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert})
			req.NoError(err, "the role-agnostic check applies no EKU restriction")
		})
	}
}

// TestVerifyClientLeafCertChain_EnrollmentIssuedCerts verifies the certificates the controller issues
// during enrollment: the client certificate is accepted as a client certificate, the serverAuth-only
// server certificate issued alongside it is not.
func TestVerifyClientLeafCertChain_EnrollmentIssuedCerts(t *testing.T) {
	req := require.New(t)
	signingCa := vMkCA(t, "signing-ca", nil)
	roots := vPoolOf(signingCa.cert)

	csrKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	req.NoError(err)
	csrDer, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "router1"},
	}, csrKey)
	req.NoError(err)
	csr, err := x509.ParseCertificateRequest(csrDer)
	req.NoError(err)

	clientDer, err := NewClientSigner(signingCa.cert, signingCa.key).SignCsr(csr, nil)
	req.NoError(err)
	clientCert, err := x509.ParseCertificate(clientDer)
	req.NoError(err)

	serverDer, err := NewServerSigner(signingCa.cert, signingCa.key).SignCsr(csr, nil)
	req.NoError(err)
	serverCert, err := x509.ParseCertificate(serverDer)
	req.NoError(err)

	_, err = VerifyClientLeafCertChain(roots, []*x509.Certificate{clientCert})
	req.NoError(err, "an enrollment-issued client certificate must be accepted")

	_, err = VerifyClientLeafCertChain(roots, []*x509.Certificate{serverCert})
	req.Error(err, "an enrollment-issued server certificate must not be accepted as a client certificate")
}

// TestVerifyClientLeafCertChain_IssuerEKU pins how an extended key usage extension on an issuing CA is
// treated: as with crypto/tls client certificate verification, a CA that restricts its usages must
// permit clientAuth for the leaves it issues to be accepted as client certificates.
func TestVerifyClientLeafCertChain_IssuerEKU(t *testing.T) {
	req := require.New(t)
	root := vMkCA(t, "root", nil)
	roots := vPoolOf(root.cert)

	mkRestrictedInter := func(cn string, ekus []x509.ExtKeyUsage) *vCertAndKey {
		interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		req.NoError(err)
		der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
			SerialNumber:          vNextSerial(),
			Subject:               pkix.Name{CommonName: cn},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			ExtKeyUsage:           ekus,
			BasicConstraintsValid: true,
		}, root.cert, &interKey.PublicKey, root.key)
		req.NoError(err)
		c, err := x509.ParseCertificate(der)
		req.NoError(err)
		return &vCertAndKey{cert: c, key: interKey}
	}

	clientInter := mkRestrictedInter("client-int", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
	leaf := vMkLeaf(t, "leaf", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, clientInter)
	_, err := VerifyClientLeafCertChain(roots, []*x509.Certificate{leaf.cert, clientInter.cert})
	req.NoError(err, "an issuer that permits clientAuth is accepted")

	serverInter := mkRestrictedInter("server-int", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	leaf = vMkLeaf(t, "leaf", "/identity/real", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, serverInter)
	_, err = VerifyClientLeafCertChain(roots, []*x509.Certificate{leaf.cert, serverInter.cert})
	req.Error(err, "an issuer restricted to serverAuth cannot issue client certificates")

	_, err = VerifyLeafCertChain(roots, []*x509.Certificate{leaf.cert, serverInter.cert})
	req.NoError(err, "the role-agnostic check applies no EKU restriction")
}

// TestVerifyClientLeafCertChain_AdditionalRoots verifies the client variant honors additional trust
// anchors the same way VerifyLeafCertChain does, without lifting the clientAuth requirement.
func TestVerifyClientLeafCertChain_AdditionalRoots(t *testing.T) {
	req := require.New(t)
	ctrlRoot := vMkCA(t, "ctrl-root", nil)
	signingRoot := vMkCA(t, "signing-root", nil)
	roots := vPoolOf(ctrlRoot.cert)

	clientLeaf := vMkLeaf(t, "router", "/identity/r1", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, signingRoot)
	serverLeaf := vMkLeaf(t, "router", "/identity/r1", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, signingRoot)

	_, err := VerifyClientLeafCertChain(roots, []*x509.Certificate{clientLeaf.cert})
	req.Error(err, "without the signing root the leaf chains to nothing trusted")

	_, err = VerifyClientLeafCertChain(roots, []*x509.Certificate{clientLeaf.cert}, signingRoot.cert)
	req.NoError(err)

	_, err = VerifyClientLeafCertChain(roots, []*x509.Certificate{serverLeaf.cert}, signingRoot.cert)
	req.Error(err, "a serverAuth-only leaf is rejected even when it chains to an additional anchor")

	_, err = VerifyClientLeafCertChain(roots, []*x509.Certificate{clientLeaf.cert})
	req.Error(err, "the signing root must not have been added to the caller's pool")
}
