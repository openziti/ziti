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

package handler_link

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/stretchr/testify/require"
)

// Test_leafFingerprint covers the fingerprint used to verify an incoming link's router: it must come
// only from the leaf certificate the handshake proved (certs[0]). The central case is that a victim
// router's certificate presented as filler elsewhere in the chain must not contribute a fingerprint,
// so it cannot be matched against the victim's enrolled fingerprint to impersonate the victim.
func Test_leafFingerprint(t *testing.T) {
	cert := func(cn string) *x509.Certificate {
		return &x509.Certificate{Subject: pkix.Name{CommonName: cn}, Raw: []byte("der-" + cn)}
	}

	t.Run("returns the leaf fingerprint", func(t *testing.T) {
		req := require.New(t)
		leaf := cert("router-A")

		fingerprint, err := leafFingerprint([]*x509.Certificate{leaf})
		req.NoError(err)
		req.Equal(nfpem.FingerprintFromCertificate(leaf), fingerprint)
	})

	t.Run("ignores filler certs in the rest of the chain", func(t *testing.T) {
		req := require.New(t)
		leaf := cert("router-A")
		filler := cert("router-D")

		fingerprint, err := leafFingerprint([]*x509.Certificate{leaf, filler})
		req.NoError(err)
		req.Equal(nfpem.FingerprintFromCertificate(leaf), fingerprint)
		req.NotEqual(nfpem.FingerprintFromCertificate(filler), fingerprint,
			"a victim cert presented as filler must not contribute a fingerprint")
	})

	t.Run("rejects when no certificates are presented", func(t *testing.T) {
		req := require.New(t)
		_, err := leafFingerprint(nil)
		req.Error(err)
	})
}
