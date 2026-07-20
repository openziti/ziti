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
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/openziti/identity"
)

// VerifyClientCertChain verifies that the presented leaf certificate (certs[0]) chains to a trusted
// root in the given CA pool, and returns that verified leaf. Only certs[0] is verified: it is the
// certificate whose private key the TLS handshake proved the peer holds, and it is the certificate a
// caller derives peer identity from. Any remaining certs[1:] are treated only as candidate
// intermediates (added alongside the pool's intermediates), never as independent grounds for
// acceptance.
//
// Client-authentication extended key usage is required. A certificate with no ExtKeyUsage extension
// is unrestricted and passes (the form ziti pki produces); a leaf whose EKU excludes client
// authentication is rejected. Callers that must accept server-auth-only client identities should
// verify with an explicit {ClientAuth, ServerAuth} usage set instead.
func VerifyClientCertChain(pool *identity.CaPool, certs []*x509.Certificate) (*x509.Certificate, error) {
	if pool == nil {
		return nil, errors.New("no ca pool provided")
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates presented")
	}

	intermediates := pool.IntermediatesAsStdPool()
	for _, intermediate := range certs[1:] {
		intermediates.AddCert(intermediate)
	}

	if _, err := certs[0].Verify(x509.VerifyOptions{
		Roots:         pool.RootsAsStdPool(),
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return nil, fmt.Errorf("leaf certificate not trusted: %w", err)
	}

	return certs[0], nil
}
