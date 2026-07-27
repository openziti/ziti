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

package mesh

// End-to-end validation of the mesh peer certificate check over a real TCP+TLS socket, using the
// production controller TLS config (identity.ServerTLSConfig). The ctrl listener uses
// RequireAnyClientCert and does not verify the client chain, so the handshake completes for any
// presented client leaf; peer admission is enforced afterward by the direction-aware cert-chain check
// against the node CA pool. Unit-level coverage of the shared check lives in common/cert.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/stretchr/testify/require"
)

const testTrustDomain = "spiffe://mesh-cert-test"

type certAndKey struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

var testSerial int64

func nextTestSerial() *big.Int {
	testSerial++
	return big.NewInt(testSerial)
}

func mkTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return k
}

func signTestCert(t *testing.T, tmpl, parent *x509.Certificate, pub *ecdsa.PublicKey, signerKey *ecdsa.PrivateKey) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signerKey)
	require.NoError(t, err)
	c, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return c
}

func mkCA(t *testing.T, cn string, parent *certAndKey) *certAndKey {
	t.Helper()
	key := mkTestKey(t)
	tmpl := &x509.Certificate{
		SerialNumber:          nextTestSerial(),
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
	return &certAndKey{cert: signTestCert(t, tmpl, signParent, &key.PublicKey, signKey), key: key}
}

func mkLeafWithKey(t *testing.T, cn, spiffePath string, ekus []x509.ExtKeyUsage, signer *certAndKey, key *ecdsa.PrivateKey) *certAndKey {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: nextTestSerial(),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  ekus,
	}
	if spiffePath != "" {
		u, err := url.Parse(testTrustDomain + spiffePath)
		require.NoError(t, err)
		tmpl.URIs = []*url.URL{u}
	}
	signParent, signKey := tmpl, key
	if signer != nil {
		signParent, signKey = signer.cert, signer.key
	}
	return &certAndKey{cert: signTestCert(t, tmpl, signParent, &key.PublicKey, signKey), key: key}
}

func mkLeaf(t *testing.T, cn, spiffePath string, ekus []x509.ExtKeyUsage, signer *certAndKey) *certAndKey {
	return mkLeafWithKey(t, cn, spiffePath, ekus, signer, mkTestKey(t))
}

func pemOfCerts(t *testing.T, certs ...*x509.Certificate) string {
	t.Helper()
	var b strings.Builder
	for _, c := range certs {
		require.NoError(t, pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
	}
	return b.String()
}

func pemOfKey(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, pem.Encode(&b, &pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return b.String()
}

// Test_MeshPeerCert_LiveHandshake stands up a real TLS listener with the production node identity,
// then connects as both a rogue and a legitimate peer. The rogue presents a self-signed identity leaf
// plus the node's own (scraped) server cert as an extra certificate; the handshake completes, but the
// mesh check must reject it. The legitimate peer presents a CA-signed client leaf and must be accepted.
func Test_MeshPeerCert_LiveHandshake(t *testing.T) {
	req := require.New(t)

	root := mkCA(t, "root", nil)
	inter := mkCA(t, "int", root)

	// Real node identity: one key backs the client and server certs (LoadIdentity uses the default
	// key for the server cert when no server_key is configured).
	nodeKey := mkTestKey(t)
	clientCert := mkLeafWithKey(t, "node-client", "/controller/real-id",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, inter, nodeKey)
	serverCert := mkLeafWithKey(t, "node-server", "",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, inter, nodeKey)

	id, err := identity.LoadIdentity(identity.Config{
		Key:        "pem:" + pemOfKey(t, nodeKey),
		Cert:       "pem:" + pemOfCerts(t, clientCert.cert, inter.cert),
		ServerCert: "pem:" + pemOfCerts(t, serverCert.cert, inter.cert),
		CA:         "pem:" + pemOfCerts(t, root.cert, inter.cert),
	})
	req.NoError(err)

	serverCfg := id.ServerTLSConfig()
	req.NotNil(serverCfg)
	req.Equal(tls.RequireAnyClientCert, serverCfg.ClientAuth)

	rawLn, err := net.Listen("tcp", "127.0.0.1:0")
	req.NoError(err)
	defer func() { _ = rawLn.Close() }()
	ln := tls.NewListener(rawLn, serverCfg)
	addr := ln.Addr().String()

	type acceptResult struct {
		peer []*x509.Certificate
		err  error
	}
	results := make(chan acceptResult, 3)
	go func() {
		for i := 0; i < 3; i++ {
			c, aerr := ln.Accept()
			if aerr != nil {
				results <- acceptResult{err: aerr}
				continue
			}
			tc := c.(*tls.Conn)
			_ = tc.SetDeadline(time.Now().Add(10 * time.Second))
			if herr := tc.Handshake(); herr != nil {
				results <- acceptResult{err: herr}
				_ = tc.Close()
				continue
			}
			results <- acceptResult{peer: tc.ConnectionState().PeerCertificates}
			_ = tc.Close()
		}
	}()

	ca := id.CA()

	// Scrape the node's own server certificate from the listener (a throwaway client cert satisfies
	// RequireAnyClientCert). This is the "extra" certificate the rogue peer will present.
	throwaway := mkLeaf(t, "throwaway", "", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil)
	scrapeConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{{Certificate: [][]byte{throwaway.cert.Raw}, PrivateKey: throwaway.key, Leaf: throwaway.cert}},
	})
	req.NoError(err)
	serverPresented := scrapeConn.ConnectionState().PeerCertificates
	_ = scrapeConn.Close()
	req.NotEmpty(serverPresented)
	extra := serverPresented[0]
	req.Equal("node-server", extra.Subject.CommonName)
	<-results

	// The scraped server certificate is server-auth-only, yet it chains to the node CA. Verification is
	// EKU-agnostic and anchors on the node's full trusted-CA pool, so it is accepted - this is the
	// certificate an outbound dial would present as its leaf.
	_, err = cert.VerifyLeafCertChain(ca, serverPresented)
	req.NoError(err, "a server-auth leaf that chains to the node CA is accepted")

	// Rogue peer: self-signed identity leaf + the scraped extra cert. Handshake completes; the mesh
	// check must reject it because the identity leaf (certs[0]) does not chain to the CA.
	rogue := mkLeaf(t, "rogue", "/controller/victim-id", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil)
	rogueConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{{Certificate: [][]byte{rogue.cert.Raw, extra.Raw}, PrivateKey: rogue.key, Leaf: rogue.cert}},
	})
	req.NoError(err, "handshake completes under RequireAnyClientCert even for a self-signed leaf")
	_ = rogueConn.Close()
	rogueRes := <-results
	req.NoError(rogueRes.err)
	_, err = cert.VerifyLeafCertChain(ca, rogueRes.peer)
	req.Error(err, "mesh check rejects a self-signed identity leaf backed by a scraped extra cert")

	// Legitimate peer: CA-signed client leaf. Handshake completes and the mesh check accepts it.
	legitConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{{Certificate: [][]byte{clientCert.cert.Raw, inter.cert.Raw}, PrivateKey: nodeKey, Leaf: clientCert.cert}},
	})
	req.NoError(err)
	_ = legitConn.Close()
	legitRes := <-results
	req.NoError(legitRes.err)
	_, err = cert.VerifyLeafCertChain(ca, legitRes.peer)
	req.NoError(err, "mesh check accepts a legitimate CA-signed peer leaf")
}
