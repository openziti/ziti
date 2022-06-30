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

package test

import (
	"github.com/openziti/foundation/identity/identity"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"time"
)

const (
	Hostname = "localhost"
)

func mkCert(cn string, dns []string) (*rsa.PrivateKey, *x509.Certificate) {
	// key, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)

	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(169),
		SignatureAlgorithm: x509.SHA256WithRSA,
		DNSNames:              dns,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
	}
	return key, cert
}

func CreateTestIdentity() (identity.Identity, error) {
	// setup
	key, cert := mkCert("Test Name", []string{Hostname})

	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, key.Public(), key)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	cfg := identity.Config{
		Key:        "pem:" + string(keyPEM),
		Cert:       "pem:" + string(certPEM),
		ServerCert: "pem:" + string(certPEM),
		CA:         "pem:" + string(certPEM),
	}

	return identity.LoadIdentity(cfg)
}
