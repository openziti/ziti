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
	"bytes"
	"crypto/x509"
	"slices"

	"github.com/openziti/identity"
)

// Intermediates returns the CA certificates in certs that are not self-signed roots, in input order.
// Leaf certificates are skipped.
func Intermediates(certs []*x509.Certificate) []*x509.Certificate {
	var result []*x509.Certificate
	for _, c := range certs {
		if c.IsCA && !identity.IsRootCa(c) {
			result = append(result, c)
		}
	}
	return result
}

// IsRootCa reports whether c is a self-signed CA certificate.
func IsRootCa(c *x509.Certificate) bool {
	return identity.IsRootCa(c)
}

// UniqueSortedDer returns the DER encodings of certs with duplicates removed, in byte order, so that
// two callers holding the same certificates in any order produce identical output.
func UniqueSortedDer(certs []*x509.Certificate) [][]byte {
	seen := map[string]struct{}{}
	var result [][]byte
	for _, c := range certs {
		key := string(c.Raw)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, c.Raw)
	}
	slices.SortFunc(result, bytes.Compare)
	return result
}
