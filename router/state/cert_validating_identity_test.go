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

package state

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

// stubServerIdentity provides just enough of identity.Identity to exercise ServerTLSConfig. The
// embedded nil interface panics if anything else is called, which keeps the stub honest.
type stubServerIdentity struct {
	identity.Identity
	cfg *tls.Config
}

func (self *stubServerIdentity) ServerTLSConfig() *tls.Config {
	return self.cfg
}

func (self *stubServerIdentity) CA() *x509.CertPool {
	return nil
}

func newTestValidatingIdentity(clientAuth tls.ClientAuthType) *tls.Config {
	wrapped := &certValidatingIdentity{
		Identity: &stubServerIdentity{
			cfg: &tls.Config{ClientAuth: clientAuth},
		},
	}
	return wrapped.ServerTLSConfig()
}

func TestCertValidatingIdentityAllowsNoCertWhenNotRequired(t *testing.T) {
	// A listener that doesn't ask for a client cert must not be failed for not receiving one.
	// The wss transport does this on its outer TLS layer and requires the client cert on the
	// inner TLS layer established over the websocket.
	for _, clientAuth := range []tls.ClientAuthType{tls.NoClientCert, tls.RequestClientCert, tls.VerifyClientCertIfGiven} {
		t.Run(clientAuth.String(), func(t *testing.T) {
			cfg := newTestValidatingIdentity(clientAuth)
			require.NotNil(t, cfg.VerifyConnection)
			require.NoError(t, cfg.VerifyConnection(tls.ConnectionState{}))
		})
	}
}

func TestCertValidatingIdentityRejectsNoCertWhenRequired(t *testing.T) {
	for _, clientAuth := range []tls.ClientAuthType{tls.RequireAnyClientCert, tls.RequireAndVerifyClientCert} {
		t.Run(clientAuth.String(), func(t *testing.T) {
			cfg := newTestValidatingIdentity(clientAuth)
			require.NotNil(t, cfg.VerifyConnection)
			require.ErrorContains(t, cfg.VerifyConnection(tls.ConnectionState{}), "no client certificate presented")
		})
	}
}

// TestCertValidatingIdentityHonorsLateClientAuthChange covers the ordering that transport listeners
// rely on: they take the config from ServerTLSConfig and only then decide whether the layer needs a
// client certificate.
func TestCertValidatingIdentityHonorsLateClientAuthChange(t *testing.T) {
	cfg := newTestValidatingIdentity(tls.RequireAnyClientCert)
	require.ErrorContains(t, cfg.VerifyConnection(tls.ConnectionState{}), "no client certificate presented")

	cfg.ClientAuth = tls.NoClientCert
	require.NoError(t, cfg.VerifyConnection(tls.ConnectionState{}))

	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	require.ErrorContains(t, cfg.VerifyConnection(tls.ConnectionState{}), "no client certificate presented")
}

func TestCertValidatingIdentityNilServerConfig(t *testing.T) {
	wrapped := &certValidatingIdentity{Identity: &stubServerIdentity{}}
	require.Nil(t, wrapped.ServerTLSConfig())
}

// issueLeafWithEku issues a leaf certificate from the CA with the given extended key usages. Nil ekus
// produces a certificate without an extended key usage extension.
func issueLeafWithEku(t *testing.T, ca *testCa, ekus []x509.ExtKeyUsage) *x509.Certificate {
	t.Helper()

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, big.NewInt(int64(1)<<62))
	require.NoError(t, err)

	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  ekus,
	}, ca.cert, &leafKey.PublicKey, ca.key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert
}

// TestCertValidatingIdentityRequiresClientAuthEku verifies that the edge listener only accepts client
// certificates usable for client authentication: a leaf that chains to a published trust anchor but is
// restricted to another purpose, such as a serverAuth-only server certificate, is rejected, while a
// leaf without an extended key usage extension remains accepted.
func TestCertValidatingIdentityRequiresClientAuthEku(t *testing.T) {
	signingCa, err := newTestRootCa("edge-signing-root")
	require.NoError(t, err)

	mgr := newCertOriginTestManager(nil)
	publishPublicKey(mgr, "signing-root", signingCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
		edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation,
	})

	wrapped := &certValidatingIdentity{
		Identity: &stubServerIdentity{
			cfg: &tls.Config{ClientAuth: tls.RequireAnyClientCert},
		},
		stateManager: mgr,
	}
	cfg := wrapped.ServerTLSConfig()
	require.NotNil(t, cfg.VerifyConnection)

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
			leaf := issueLeafWithEku(t, signingCa, test.ekus)
			err := cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}})
			if test.accepted {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "client certificate not issued by a trusted CA")
			}
		})
	}
}
