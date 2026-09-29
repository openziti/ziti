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

package handler_peer_ctrl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newCertWithUri(t *testing.T, uri string) *x509.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	parsed, err := url.Parse(uri)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{parsed},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func Test_checkSentBy(t *testing.T) {
	t.Run("accepts the named member", func(t *testing.T) {
		certs := []*x509.Certificate{newCertWithUri(t, "spiffe://ziti.test/controller/ctrl2")}
		require.NoError(t, checkSentBy(certs, "ctrl2"))
	})

	t.Run("refuses a different controller", func(t *testing.T) {
		certs := []*x509.Certificate{newCertWithUri(t, "spiffe://ziti.test/controller/ctrl9")}
		require.ErrorContains(t, checkSentBy(certs, "ctrl2"), "request for member ctrl2 was sent by controller ctrl9")
	})

	t.Run("refuses a cert without a controller id", func(t *testing.T) {
		certs := []*x509.Certificate{newCertWithUri(t, "spiffe://ziti.test/router/ctrl2")}
		require.ErrorContains(t, checkSentBy(certs, "ctrl2"), "unable to identify sending controller")
	})

	t.Run("refuses a connection without certs", func(t *testing.T) {
		require.ErrorContains(t, checkSentBy(nil, "ctrl2"), "unable to identify sending controller")
	})
}
