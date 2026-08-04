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
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/openziti/identity"
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
