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

package oidc_auth

import (
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// verifyCertBinding checks that the TLS leaf certificate is bound to the token claims.
// Only the leaf certificate (index 0 of PeerCertificates) is examined; intermediates
// are unverified public certs used only for chain building and are not relevant here.
//
// If certFingerprints (z_cfs) is non-empty, the leaf cert fingerprint must match at least
// one entry. This is a strict check and fails if no match is found.
//
// If certFingerprints is empty, falls back to SPIFFE ID verification: the leaf cert's
// SAN URIs must contain a SPIFFE ID referencing the apiSessionId.
//
// Returns nil if verification passes or is not applicable (no leaf cert and no z_cfs).
func verifyCertBinding(leafCert *x509.Certificate, apiSessionId, identityId string, certFingerprints []string) error {
	if len(certFingerprints) > 0 {
		return verifyByFingerprint(leafCert, certFingerprints)
	}
	return verifyBySpiffeId(leafCert, apiSessionId, identityId)
}

// verifyByFingerprint performs a strict check: the leaf cert's SHA-1 fingerprint
// must appear in the allowed fingerprints list.
func verifyByFingerprint(leafCert *x509.Certificate, certFingerprints []string) error {
	if leafCert == nil {
		return oidc.ErrAccessDenied().WithDescription("peer certificate fingerprint does not match token")
	}

	fp := fmt.Sprintf("%x", sha1.Sum(leafCert.Raw))

	for _, allowed := range certFingerprints {
		if fp == allowed {
			return nil
		}
	}

	return oidc.ErrAccessDenied().WithDescription("peer certificate fingerprint does not match token")
}

// verifyBySpiffeId checks whether the leaf cert has a SPIFFE ID SAN URI that references the
// expected identity and API session. The SPIFFE path format is:
// identity/{identityId}/apiSession/{apiSessionId}/apiSessionCertificate/{certId}
func verifyBySpiffeId(leafCert *x509.Certificate, apiSessionId, identityId string) error {
	if apiSessionId == "" {
		return nil
	}

	if leafCert == nil {
		return nil
	}

	identitySegment := "identity/" + identityId + "/"
	sessionSegment := "apiSession/" + apiSessionId + "/"

	for _, uri := range leafCert.URIs {
		if uri.Scheme != "spiffe" {
			continue
		}
		if strings.Contains(uri.Path, identitySegment) && strings.Contains(uri.Path, sessionSegment) {
			return nil
		}
	}

	return oidc.ErrAccessDenied().WithDescription("peer certificate SPIFFE ID does not match API session")
}

// tlsLeafCert returns the leaf TLS peer certificate from the HTTP request, or nil if unavailable.
func tlsLeafCert(r *http.Request) *x509.Certificate {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}
	return r.TLS.PeerCertificates[0]
}
