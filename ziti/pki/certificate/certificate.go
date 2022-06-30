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

package certificate

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
)

// Bundle represents a pair of private key and certificate.
type Bundle struct {
	Name string
	Key  *rsa.PrivateKey
	Cert *x509.Certificate
}

// Raw returns the raw bytes for the private key and certificate.
func (b *Bundle) Raw() ([]byte, []byte) {
	return x509.MarshalPKCS1PrivateKey(b.Key), b.Cert.Raw
}

// RawToBundle creates a bundle from the name and bytes given for a private key
// and a certificate.
func RawToBundle(name string, key []byte, cert []byte) (*Bundle, error) {
	k, err := x509.ParsePKCS1PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed parsing private key: %v", err)
	}
	c, err := x509.ParseCertificate(cert)
	if err != nil {
		return nil, fmt.Errorf("failed parsing certificate: %v", err)
	}
	return &Bundle{Name: name, Key: k, Cert: c}, nil
}

// State represents a certificate state (Valid, Expired, Revoked).
type State int

// Certificate states.
const (
	Valid State = iota
	Revoked
	Expired
)
