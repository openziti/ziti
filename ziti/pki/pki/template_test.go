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

package pki

import (
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/ziti/pki/store"
)

func TestServerLeafHasServerAuthEKU(t *testing.T) {
	cert := signLeaf(t, "server", false)
	if !hasExtKeyUsage(cert, x509.ExtKeyUsageServerAuth) || hasExtKeyUsage(cert, x509.ExtKeyUsageClientAuth) {
		t.Fatalf("server ExtKeyUsage = %v, want [serverAuth]", cert.ExtKeyUsage)
	}
}

func TestClientLeafHasClientAuthEKU(t *testing.T) {
	cert := signLeaf(t, "client", true)
	if !hasExtKeyUsage(cert, x509.ExtKeyUsageClientAuth) || hasExtKeyUsage(cert, x509.ExtKeyUsageServerAuth) {
		t.Fatalf("client ExtKeyUsage = %v, want [clientAuth]", cert.ExtKeyUsage)
	}
}

func signLeaf(t *testing.T, name string, client bool) *x509.Certificate {
	t.Helper()
	z := &ZitiPKI{Store: &store.Local{Root: t.TempDir()}}
	if err := z.Sign(nil, &Request{
		Name:              "ca",
		PrivateKeyOptions: &EcPrivateKeyOptions{Curve: elliptic.P256()},
		Template: &x509.Certificate{
			Subject:  pkix.Name{CommonName: "test-ca"},
			IsCA:     true,
			NotAfter: time.Now().Add(24 * time.Hour),
		},
	}); err != nil {
		t.Fatalf("sign ca: %v", err)
	}
	ca, err := z.GetCA("ca")
	if err != nil {
		t.Fatalf("get ca: %v", err)
	}
	if len(ca.Cert.ExtKeyUsage) != 0 {
		t.Fatalf("CA ExtKeyUsage = %v, want none", ca.Cert.ExtKeyUsage)
	}
	if err := z.Sign(ca, &Request{
		Name:                name,
		IsClientCertificate: client,
		PrivateKeyOptions:   &EcPrivateKeyOptions{Curve: elliptic.P256()},
		Template: &x509.Certificate{
			Subject:  pkix.Name{CommonName: name},
			DNSNames: []string{"localhost"},
			NotAfter: time.Now().Add(24 * time.Hour),
		},
	}); err != nil {
		t.Fatalf("sign %s: %v", name, err)
	}
	bundle, err := z.GetBundle("ca", name)
	if err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	return bundle.Cert
}

func hasExtKeyUsage(cert *x509.Certificate, usage x509.ExtKeyUsage) bool {
	for _, u := range cert.ExtKeyUsage {
		if u == usage {
			return true
		}
	}
	return false
}
