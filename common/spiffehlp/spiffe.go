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

package spiffehlp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/pkg/errors"
	"net/url"
)

// GetSpiffeIdFromCertChain cycles through a slice of certificates that goes from leaf up CAs. Each certificate
// must contain 0 or 1 spiffe:// URI SAN. The first encountered SPIFFE id looking up the chain back to the root CA is returned.
// If no SPIFFE id is encountered, nil is returned. Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromCertChain(certs []*x509.Certificate) (*url.URL, error) {
	var spiffeId *url.URL
	for _, cert := range certs {
		var err error
		spiffeId, err = GetSpiffeIdFromCert(cert)

		if err != nil {
			return nil, fmt.Errorf("failed to determine SPIFFE ID from x509 certificate chain: %w", err)
		}

		if spiffeId != nil {
			return spiffeId, nil
		}
	}

	return nil, errors.New("failed to determine SPIFFE ID, no spiffe:// URI SANs found in x509 certificate chain")
}

// GetSpiffeIdFromTlsCertChain will search a tls certificate chain for a trust domain encoded as a spiffe:// URI SAN.
// Each certificate must contain 0 or 1 spiffe:// URI SAN. The first SPIFFE id looking up the chain is returned. If
// no SPIFFE id is encountered, nil is returned. Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromTlsCertChain(tlsCerts []*tls.Certificate) (*url.URL, error) {
	for _, tlsCert := range tlsCerts {
		for i, rawCert := range tlsCert.Certificate {
			cert, err := x509.ParseCertificate(rawCert)

			if err != nil {
				return nil, fmt.Errorf("failed to parse TLS cert at index [%d]: %w", i, err)
			}

			spiffeId, err := GetSpiffeIdFromCert(cert)

			if err != nil {
				return nil, fmt.Errorf("failed to determine SPIFFE ID from TLS cert at index [%d]: %w", i, err)
			}

			if spiffeId != nil {
				return spiffeId, nil
			}
		}
	}

	return nil, nil
}

// GetSpiffeIdFromCert will search a x509 certificate for a trust domain encoded as a spiffe:// URI SAN.
// Each certificate must contain 0 or 1 spiffe:// URI SAN. The first SPIFFE id looking up the chain is returned. If
// no SPIFFE id is encountered, nil is returned. Errors are returned for parsing and processing errors only.
func GetSpiffeIdFromCert(cert *x509.Certificate) (*url.URL, error) {
	var spiffeId *url.URL
	for _, uriSan := range cert.URIs {
		if uriSan.Scheme == "spiffe" {
			if spiffeId != nil {
				return nil, fmt.Errorf("multiple URI SAN spiffe:// ids encountered, must only have one, encountered at least two: [%s] and [%s]", spiffeId.String(), uriSan.String())
			}
			spiffeId = uriSan
		}
	}

	return spiffeId, nil
}
