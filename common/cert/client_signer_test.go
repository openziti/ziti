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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// csSignedCsr returns a CSR signed by a freshly generated key.
func csSignedCsr(commonName string) (*x509.CertificateRequest, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: commonName},
	}, key)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificateRequest(der)
}

// csSigningCa returns a self-signed CA certificate and its key.
func csSigningCa(commonName string) (*x509.Certificate, crypto.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}

	return caCert, key, nil
}

func TestClientSigner_SignCsr_DefaultsToClientAuth(t *testing.T) {
	req := require.New(t)

	caCert, caKey, err := csSigningCa("default-eku-ca")
	req.NoError(err)

	csr, err := csSignedCsr("default-eku-leaf")
	req.NoError(err)

	signer := NewClientSigner(caCert, caKey)

	raw, err := signer.SignCsr(csr, nil)

	req.NoError(err)
	signed, err := x509.ParseCertificate(raw)
	req.NoError(err)
	req.Equal([]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, signed.ExtKeyUsage,
		"a client signer left at its default must issue clientAuth alone")
}

func TestClientSigner_SignCsr_IssuesConfiguredExtKeyUsage(t *testing.T) {
	req := require.New(t)

	caCert, caKey, err := csSigningCa("both-eku-ca")
	req.NoError(err)

	csr, err := csSignedCsr("both-eku-leaf")
	req.NoError(err)

	signer := NewClientSigner(caCert, caKey)
	signer.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}

	raw, err := signer.SignCsr(csr, nil)

	req.NoError(err)
	signed, err := x509.ParseCertificate(raw)
	req.NoError(err)
	req.ElementsMatch([]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, signed.ExtKeyUsage,
		"a client signer must issue the extended key usages it is configured with")
}
