/*
	(c) Copyright NetFoundry Inc.

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

package xlink_transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/stretchr/testify/require"
)

// caPoolIdentity is a minimal identity.Identity whose only useful method is CA. The link verification
// path consults nothing else on the identity, so the remaining interface methods are left to the
// embedded nil interface (never called on the paths exercised here).
type caPoolIdentity struct {
	identity.Identity
	roots *x509.CertPool
}

func (f *caPoolIdentity) CA() *x509.CertPool { return f.roots }

func ltPool(certs ...*x509.Certificate) *x509.CertPool {
	p := x509.NewCertPool()
	for _, c := range certs {
		p.AddCert(c)
	}
	return p
}

type ltCertAndKey struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

var ltSerial int64

func ltMkCert(t *testing.T, cn string, isCA bool, signer *ltCertAndKey) *ltCertAndKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ltSerial++
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(ltSerial),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
	}
	if isCA {
		tmpl.IsCA = true
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	signParent, signKey := tmpl, key
	if signer != nil {
		signParent, signKey = signer.cert, signer.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signParent, &key.PublicKey, signKey)
	require.NoError(t, err)
	c, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &ltCertAndKey{cert: c, key: key}
}

func ltHandler(roots *x509.CertPool, token string) *ConnectionHandler {
	return &ConnectionHandler{routerId: &identity.TokenId{Identity: &caPoolIdentity{roots: roots}, Token: token}}
}

// Test_ConnectionHandler_RejectsUntrustedDialer covers incoming link verification: the dialing router
// must present a leaf that chains to the CA. A self-signed leaf, alone or backed by a scraped
// CA-chained filler certificate, must be rejected - verification is bound to the leaf, not to "some
// presented certificate chains".
func Test_ConnectionHandler_RejectsUntrustedDialer(t *testing.T) {
	req := require.New(t)

	root := ltMkCert(t, "root", true, nil)
	inter := ltMkCert(t, "int", true, root)
	roots := ltPool(root.cert, inter.cert)
	handler := ltHandler(roots, "router1")

	legit := ltMkCert(t, "legit-router", false, inter)
	forged := ltMkCert(t, "forged", false, nil)

	hello := &channel.Hello{Headers: channel.Headers{}}

	req.Error(handler.HandleConnection(hello, nil), "no certificates must be rejected")
	req.Error(handler.HandleConnection(hello, []*x509.Certificate{forged.cert}),
		"self-signed leaf that does not chain to the CA must be rejected")
	req.Error(handler.HandleConnection(hello, []*x509.Certificate{forged.cert, legit.cert}),
		"self-signed leaf backed by a scraped CA-chained filler cert must be rejected")
}

func Test_ConnectionHandler_AcceptsTrustedDialer(t *testing.T) {
	req := require.New(t)

	root := ltMkCert(t, "root", true, nil)
	inter := ltMkCert(t, "int", true, root)
	roots := ltPool(root.cert, inter.cert)
	handler := ltHandler(roots, "router1")

	legit := ltMkCert(t, "legit-router", false, inter)
	req.NoError(handler.HandleConnection(&channel.Hello{Headers: channel.Headers{}}, []*x509.Certificate{legit.cert}),
		"a leaf chaining to the CA must be accepted")
}

// Test_ConnectionHandler_DialedRouterIdMismatch verifies the dial is rejected when it names a different
// target router, even if the presented certificate is otherwise valid.
func Test_ConnectionHandler_DialedRouterIdMismatch(t *testing.T) {
	req := require.New(t)

	root := ltMkCert(t, "root", true, nil)
	inter := ltMkCert(t, "int", true, root)
	roots := ltPool(root.cert, inter.cert)
	handler := ltHandler(roots, "router1")
	legit := ltMkCert(t, "legit-router", false, inter)

	mismatch := channel.Headers{}
	mismatch.PutStringHeader(LinkDialedRouterId, "some-other-router")
	req.Error(handler.HandleConnection(&channel.Hello{Headers: mismatch}, []*x509.Certificate{legit.cert}),
		"a link dial meant for a different router must be rejected")

	match := channel.Headers{}
	match.PutStringHeader(LinkDialedRouterId, "router1")
	req.NoError(handler.HandleConnection(&channel.Hello{Headers: match}, []*x509.Certificate{legit.cert}),
		"a matching dialed router id with a trusted cert must be accepted")
}
