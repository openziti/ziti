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
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	transporttls "github.com/openziti/transport/v2/tls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLinkId(t *testing.T) {
	t.Run("prefers header when present", func(t *testing.T) {
		headers := map[int32][]byte{
			LinkHeaderLinkId: []byte("the-link-id"),
		}
		assert.Equal(t, "the-link-id", resolveLinkId(headers, "fallback-id"))
	})

	t.Run("falls back to channel id when header absent", func(t *testing.T) {
		headers := map[int32][]byte{
			LinkHeaderRouterId: []byte("router-1"),
		}
		assert.Equal(t, "fallback-id", resolveLinkId(headers, "fallback-id"))
	})

	t.Run("falls back to channel id when header empty", func(t *testing.T) {
		headers := map[int32][]byte{
			LinkHeaderLinkId: []byte(""),
		}
		assert.Equal(t, "fallback-id", resolveLinkId(headers, "fallback-id"))
	})

	t.Run("falls back to channel id when headers nil", func(t *testing.T) {
		assert.Equal(t, "fallback-id", resolveLinkId(nil, "fallback-id"))
	})
}

// testPki generates an in-memory CA and issues leaf certificates, so the link id
// tests can run over tls with real certificates in play.
type testPki struct {
	caKey  *ecdsa.PrivateKey
	caCert *x509.Certificate
	caPem  string
	serial int64
}

func newTestPki(t *testing.T) *testPki {
	req := require.New(t)

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	req.NoError(err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "link-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	req.NoError(err)

	caCert, err := x509.ParseCertificate(der)
	req.NoError(err)

	return &testPki{
		caKey:  caKey,
		caCert: caCert,
		caPem:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		serial: 1,
	}
}

// newIdentity issues a leaf certificate for the given common name and returns a
// TokenId for it, with the token defaulted to the common name.
func (self *testPki) newIdentity(t *testing.T, cn string) *identity.TokenId {
	req := require.New(t)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	req.NoError(err)

	self.serial++
	template := &x509.Certificate{
		SerialNumber: big.NewInt(self.serial),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, self.caCert, &key.PublicKey, self.caKey)
	req.NoError(err)

	keyDer, err := x509.MarshalECPrivateKey(key)
	req.NoError(err)

	certPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPem := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}))

	id, err := identity.LoadIdentity(identity.Config{
		Key:        "pem:" + keyPem,
		Cert:       "pem:" + certPem,
		ServerCert: "pem:" + certPem,
		CA:         "pem:" + self.caPem,
	})
	req.NoError(err)

	return identity.NewIdentity(id)
}

// TestLinkIdHeaderPreferredOverChannelId runs a real hello exchange over tls,
// mirroring how links dial: the dialer uses its router identity with the token
// swapped to the link id. It verifies that with certificates in play the channel
// id still comes from the hello token, not the cert, and that the link id header
// is preferred over the channel id when the two differ.
func TestLinkIdHeaderPreferredOverChannelId(t *testing.T) {
	transport.AddAddressParser(transporttls.AddressParser{})
	req := require.New(t)

	pki := newTestPki(t)
	listenerId := pki.newIdentity(t, "listening-router")
	dialingRouterId := pki.newIdentity(t, "dialing-router")

	// grab a free port for the test listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	req.NoError(err)
	port := l.Addr().(*net.TCPAddr).Port
	req.NoError(l.Close())

	addr, err := transport.ParseAddress(fmt.Sprintf("tls:127.0.0.1:%d", port))
	req.NoError(err)

	type acceptResult struct {
		channelId  string
		linkId     string
		peerCertCN string
	}
	results := make(chan acceptResult, 1)

	acceptF := func(underlay channel.Underlay) {
		_, err := channel.NewChannelWithUnderlay("test-listen", underlay, channel.BindHandlerF(func(binding channel.Binding) error {
			ch := binding.GetChannel()
			result := acceptResult{
				channelId: ch.Id(),
				linkId:    resolveLinkId(ch.Underlay().Headers(), ch.Id()),
			}
			if certs := ch.Certificates(); len(certs) > 0 {
				result.peerCertCN = certs[0].Subject.CommonName
			}
			results <- result
			return nil
		}), channel.DefaultOptions())
		assert.NoError(t, err)
	}

	listenerConfig := channel.ListenerConfig{
		ConnectOptions: channel.DefaultConnectOptions(),
	}
	listener, err := channel.NewClassicListenerF(listenerId, addr, listenerConfig, acceptF)
	req.NoError(err)
	defer func() { _ = listener.Close() }()

	// mirror the xlink dialer: the real router identity with the token swapped to
	// the link id (the current link-id-as-channel-id behavior), and a different
	// value in the link id header, proving the listener prefers the header
	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity: dialingRouterId.ShallowCloneWithNewToken("token-link-id"),
		Endpoint: addr,
		Headers: map[int32][]byte{
			LinkHeaderLinkId: []byte("header-link-id"),
		},
	})

	// dial the way dialMulti does: the grouped-dial headers get overlaid onto the
	// link headers in the hello, sharing one header keyspace. This catches link
	// header keys colliding with the channel library's own header keys (e.g.
	// channel.TypeHeader overwriting the link id header).
	firstDialHeaders := channel.Headers{}
	firstDialHeaders.PutBoolHeader(channel.IsFirstGroupConnection, true)
	firstDialHeaders.PutBoolHeader(channel.IsGroupedHeader, true)
	firstDialHeaders.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)

	underlay, err := dialer.CreateWithHeaders(5*time.Second, firstDialHeaders)
	req.NoError(err)

	ch, err := channel.NewChannelWithUnderlay("test-dial", underlay, channel.BindHandlerF(func(channel.Binding) error {
		return nil
	}), channel.DefaultOptions())
	req.NoError(err)
	defer func() { _ = ch.Close() }()

	// dial side: the channel id is the listening router's identity
	req.Equal("listening-router", ch.Id())

	select {
	case result := <-results:
		// the client cert was presented, and its common name is the router id,
		// yet the channel id still comes from the hello token
		req.Equal("dialing-router", result.peerCertCN)
		req.Equal("token-link-id", result.channelId)
		// the link id header wins when it differs from the channel id
		req.Equal("header-link-id", result.linkId)
	case <-time.After(5 * time.Second):
		req.Fail("timed out waiting for link connection")
	}
}
