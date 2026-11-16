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

	"github.com/stretchr/testify/require"
)

// newTestLeafWithEku creates a leaf certificate signed by the given CA with the given extended key
// usages and validity window. Nil ekus produces a certificate without an extended key usage extension.
func newTestLeafWithEku(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, ekus []x509.ExtKeyUsage, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, big.NewInt(int64(1)<<62))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  ekus,
	}

	certDer, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDer)
	require.NoError(t, err)

	return cert
}

// Test_AuthModuleCert_verifyClientCerts_RequiresClientAuth verifies that certificate authentication only
// accepts chains usable for client authentication: a leaf restricted to another purpose, such as a
// serverAuth-only server certificate, must not verify, while a leaf without an extended key usage
// extension remains accepted.
func Test_AuthModuleCert_verifyClientCerts_RequiresClientAuth(t *testing.T) {
	ca, caKey := newTestCA(t, "root")
	roots := x509.NewCertPool()
	roots.AddCert(ca)

	module := &AuthModuleCert{}

	validFrom := time.Now().Add(-time.Hour)
	validTo := time.Now().Add(time.Hour)

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
			leaf := newTestLeafWithEku(t, ca, caKey, test.ekus, validFrom, validTo)

			chains, err := module.verifyClientCerts([]*x509.Certificate{leaf}, roots)
			if test.accepted {
				req.NoError(err)
				req.NotEmpty(chains)
			} else {
				req.Error(err)
				req.Empty(chains)
			}
		})
	}

	t.Run("expired clientAuth leaf still verifies", func(t *testing.T) {
		req := require.New(t)
		leaf := newTestLeafWithEku(t, ca, caKey, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))

		chains, err := module.verifyClientCerts([]*x509.Certificate{leaf}, roots)
		req.NoError(err, "expiry is enforced later, according to the auth policy")
		req.NotEmpty(chains)
	})
}
