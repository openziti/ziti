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
)

// VerifyLeafCertChain verifies that the presented leaf certificate (certs[0]) chains to a trusted
// certificate in the given pool, and returns that verified leaf. Only certs[0] is verified: it is the
// certificate whose private key the TLS handshake proved the peer holds, and it is the certificate a
// caller derives peer identity from. Any remaining certs[1:] are treated only as candidate
// intermediates supplied by the peer, never as independent grounds for acceptance - so a peer cannot be
// admitted by presenting its own leaf alongside some other certificate that happens to chain.
//
// roots is the verifying node's full trusted-CA pool (identity.CA()). Every certificate in it is a valid
// chain terminus, whether a self-signed root or an intermediate distributed as a trust anchor, matching
// how the node's TLS configuration establishes trust. No extended-key-usage restriction is applied:
// certificates issued by an external PKI with arbitrary or absent EKUs are accepted as long as the leaf
// chains to a trusted CA.
func VerifyLeafCertChain(roots *x509.CertPool, certs []*x509.Certificate) (*x509.Certificate, error) {
	if roots == nil {
		return nil, errors.New("no ca pool provided")
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates presented")
	}

	intermediates := x509.NewCertPool()
	for _, intermediate := range certs[1:] {
		intermediates.AddCert(intermediate)
	}

	if _, err := certs[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, fmt.Errorf("leaf certificate not trusted: %w", err)
	}

	return certs[0], nil
}
