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

package handler_ctrl

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
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// meshChannelType mirrors controller/raft/mesh.ChannelTypeMesh without importing that package.
const meshChannelType = "ctrl.mesh"

func helloWithType(chType string) *channel.Hello {
	h := channel.Headers{}
	if chType != "" {
		h.PutStringHeader(channel.TypeHeader, chType)
	}
	return &channel.Hello{Headers: h}
}

// Test_ConnectHandler_isSeparatelyValidated covers the rule that only channel types dispatched to a
// dedicated, self-validating acceptor are skipped; every other type - including unrecognized types and
// the mesh type on a non-clustered controller - routes to the router control acceptor and must be
// validated here.
func Test_ConnectHandler_isSeparatelyValidated(t *testing.T) {
	// Clustered controller: only the mesh type has a dedicated acceptor.
	clustered := &ConnectHandler{separatelyValidatedTypes: map[string]struct{}{meshChannelType: {}}}
	require.True(t, clustered.isSeparatelyValidated(helloWithType(meshChannelType)), "mesh is validated by its own acceptor")
	require.False(t, clustered.isSeparatelyValidated(helloWithType(ctrlchan.ChannelTypeDefault)), "router ctrl type is validated here")
	require.False(t, clustered.isSeparatelyValidated(helloWithType(ctrlchan.ChannelTypeHighPriority)))
	require.False(t, clustered.isSeparatelyValidated(helloWithType("bogus")), "unrecognized types route to the router acceptor and must be validated")
	require.False(t, clustered.isSeparatelyValidated(helloWithType("")), "legacy (no type) connections are validated here")

	// Non-clustered controller: no dedicated acceptors, so even a mesh-typed connection routes to the
	// router acceptor and must be validated.
	standalone := &ConnectHandler{separatelyValidatedTypes: map[string]struct{}{}}
	require.False(t, standalone.isSeparatelyValidated(helloWithType(meshChannelType)), "mesh on a non-clustered controller must be validated")
	require.False(t, standalone.isSeparatelyValidated(helloWithType(ctrlchan.ChannelTypeDefault)))
}

func Test_isFirstCtrlConnection(t *testing.T) {
	// Legacy / non-grouped dial: no grouped header -> treated as a new channel.
	require.True(t, isFirstCtrlConnection(&channel.Hello{Headers: channel.Headers{}}))

	grouped := channel.Headers{}
	grouped.PutBoolHeader(channel.IsGroupedHeader, true)
	grouped.PutBoolHeader(channel.IsFirstGroupConnection, true)
	require.True(t, isFirstCtrlConnection(&channel.Hello{Headers: grouped}), "grouped first connection")

	additional := channel.Headers{}
	additional.PutBoolHeader(channel.IsGroupedHeader, true)
	require.False(t, isFirstCtrlConnection(&channel.Hello{Headers: additional}), "additional underlay (no first flag)")

	notFirst := channel.Headers{}
	notFirst.PutBoolHeader(channel.IsGroupedHeader, true)
	notFirst.PutBoolHeader(channel.IsFirstGroupConnection, false)
	require.False(t, isFirstCtrlConnection(&channel.Hello{Headers: notFirst}), "additional underlay (first=false)")
}

// caPoolIdentity is a minimal identity.Identity whose only useful method is CA. The certificate
// verification path of HandleConnection consults nothing else, so the remaining interface methods are
// left to the embedded nil interface (never called on the paths exercised here).
type caPoolIdentity struct {
	identity.Identity
	roots *x509.CertPool
}

func (f *caPoolIdentity) CA() *x509.CertPool { return f.roots }

type ctCertAndKey struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

var ctSerial int64

func ctMkCert(t *testing.T, cn string, isCA bool, signer *ctCertAndKey) *ctCertAndKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ctSerial++
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(ctSerial),
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
	return &ctCertAndKey{cert: c, key: key}
}

// Test_ConnectHandler_HandleConnection_RejectsUntrustedLeaf covers the router control-channel
// verification: a connection dispatched to the router acceptor must present a leaf that chains to the
// controller CA. In particular, presenting a self-signed leaf followed by a legitimate router's public
// certificate as filler must be rejected - the check is bound to the leaf, not to "some presented cert
// chains". These paths fail during certificate verification, before any network state is consulted.
func Test_ConnectHandler_HandleConnection_RejectsUntrustedLeaf(t *testing.T) {
	req := require.New(t)

	root := ctMkCert(t, "root", true, nil)
	inter := ctMkCert(t, "int", true, root)
	roots := x509.NewCertPool()
	roots.AddCert(root.cert)
	roots.AddCert(inter.cert)

	handler := &ConnectHandler{
		identity:                 &caPoolIdentity{roots: roots},
		separatelyValidatedTypes: map[string]struct{}{},
	}

	// A legitimate, CA-chained certificate an attacker could scrape off the wire and present as filler.
	legit := ctMkCert(t, "legit-router", false, inter)
	// The attacker's own self-signed leaf - it does not chain to the CA.
	forged := ctMkCert(t, "forged", false, nil)

	hello := &channel.Hello{IdToken: "router1", Headers: channel.Headers{}}

	req.Error(handler.HandleConnection(hello, nil), "no certificates must be rejected")
	req.Error(handler.HandleConnection(hello, []*x509.Certificate{forged.cert}),
		"self-signed leaf that does not chain to the CA must be rejected")
	req.Error(handler.HandleConnection(hello, []*x509.Certificate{forged.cert, legit.cert}),
		"self-signed leaf backed by a scraped CA-chained filler cert must be rejected")
}

// Test_ConnectHandler_HandleConnection_SkipsSeparatelyValidated verifies that connections whose type is
// handled by a separate acceptor (e.g. the raft mesh on a clustered controller) are not validated here,
// even when the presented certificate would fail the router control-channel check.
func Test_ConnectHandler_HandleConnection_SkipsSeparatelyValidated(t *testing.T) {
	req := require.New(t)

	forged := ctMkCert(t, "forged", false, nil)
	handler := &ConnectHandler{
		separatelyValidatedTypes: map[string]struct{}{meshChannelType: {}},
	}

	req.NoError(handler.HandleConnection(helloWithType(meshChannelType), []*x509.Certificate{forged.cert}),
		"mesh-typed connection is validated by its own acceptor and must be skipped here")
}

// Test_withinChurnLimit pins the admission policy that decides whether an already-connected router's
// channel may be displaced by a new connection.
//
// It is the only thing rate-limiting displacement. Network.ConnectRouter always displaces an occupant it
// does not recognise, so without this a spurious first-connection hello would tear down a healthy control
// channel and force the router to redial. Uniqueness itself is guaranteed under the per-router lock in
// ConnectRouter, not here, so this check exists purely to protect a working connection from churn.
func Test_withinChurnLimit(t *testing.T) {
	connectedAt := func(d time.Duration) *model.Router {
		r := &model.Router{ConnectTime: time.Now().Add(-d)}
		r.Id = "r1"
		return r
	}

	tests := []struct {
		name       string
		since      time.Duration
		churnLimit time.Duration
		protected  bool
	}{
		{"a connection just established is protected", 0, time.Minute, true},
		{"still protected part way through the window", 30 * time.Second, time.Minute, true},
		{"displaceable once the window has passed", 2 * time.Minute, time.Minute, false},
		// A zero limit is a supported setting and means "always allow takeover", which is what the option
		// existed to make configurable in the first place.
		{"a zero limit protects nothing", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.protected, withinChurnLimit(connectedAt(tt.since), tt.churnLimit))
		})
	}
}
