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
package webapis

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/openziti/identity"
	"github.com/openziti/xweb/v2"
	"github.com/stretchr/testify/require"
	"math/big"
	"net"
	"testing"
	"time"
)

func Test_getPossibleIssuers(t *testing.T) {
	req := require.New(t)

	caKey, caCertTemplate := mkCaCert("Parent CA")
	parentDer, err := x509.CreateCertificate(rand.Reader, caCertTemplate, caCertTemplate, caKey.Public(), caKey)
	req.NoError(err)

	childKey1, childCert1 := mkClientCert("Test Child 1")
	childCert1Der, err := x509.CreateCertificate(rand.Reader, childCert1, caCertTemplate, childKey1.Public(), caKey)
	req.NoError(err)

	childKey2, childCert2 := mkServerCert("Test Child 2", []string{"client2.netfoundry.io"}, []net.IP{net.ParseIP("127.0.0.1")})
	childCert2Der, err := x509.CreateCertificate(rand.Reader, childCert2, caCertTemplate, childKey2.Public(), caKey)
	req.NoError(err)

	childKey3, childCert3 := mkServerCert("Test Child 3", []string{"client3.netfoundry.io"}, []net.IP{net.ParseIP("10.8.0.1")})
	childCert3Der, err := x509.CreateCertificate(rand.Reader, childCert3, caCertTemplate, childKey3.Public(), caKey)
	req.NoError(err)

	childKey4, childCert4 := mkServerCert("Test Child 4", []string{"client4.netfoundry.io"}, []net.IP{net.ParseIP("192.168.0.1")})
	childCert4Der, err := x509.CreateCertificate(rand.Reader, childCert4, caCertTemplate, childKey4.Public(), caKey)
	req.NoError(err)

	childKey1Der, _ := x509.MarshalECPrivateKey(childKey1.(*ecdsa.PrivateKey))
	childKey1Pem := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: childKey1Der,
	}

	childKey2Der, _ := x509.MarshalECPrivateKey(childKey2.(*ecdsa.PrivateKey))
	childKey2Pem := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: childKey2Der,
	}

	childKey3Der, _ := x509.MarshalECPrivateKey(childKey3.(*ecdsa.PrivateKey))
	childKey3Pem := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: childKey3Der,
	}

	childKey4Der, _ := x509.MarshalECPrivateKey(childKey4.(*ecdsa.PrivateKey))
	childKey4Pem := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: childKey4Der,
	}

	parentPem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: parentDer,
	}

	childCert1Pem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: childCert1Der,
	}

	childCert2Pem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: childCert2Der,
	}

	childCert3Pem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: childCert3Der,
	}

	childCert4Pem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: childCert4Der,
	}

	cfg := identity.Config{
		Key:        "pem:" + string(pem.EncodeToMemory(childKey1Pem)),
		Cert:       "pem:" + string(pem.EncodeToMemory(childCert1Pem)) + string(pem.EncodeToMemory(parentPem)),
		ServerKey:  "pem:" + string(pem.EncodeToMemory(childKey2Pem)),
		ServerCert: "pem:" + string(pem.EncodeToMemory(childCert2Pem)) + string(pem.EncodeToMemory(parentPem)),
		AltServerCerts: []identity.ServerPair{
			{
				ServerKey:  "pem:" + string(pem.EncodeToMemory(childKey3Pem)),
				ServerCert: "pem:" + string(pem.EncodeToMemory(childCert3Pem)) + string(pem.EncodeToMemory(parentPem)),
			},
			{
				ServerKey:  "pem:" + string(pem.EncodeToMemory(childKey4Pem)),
				ServerCert: "pem:" + string(pem.EncodeToMemory(childCert4Pem)) + string(pem.EncodeToMemory(parentPem)),
			},
		},
	}

	id, err := identity.LoadIdentity(cfg)
	req.NoError(err)

	t.Run("receives the proper issuers", func(t *testing.T) {
		req := require.New(t)
		const (
			bindPoint1Address = "test1.example.com:1234"
			bindPoint2Address = "test2.example.com:443"
		)

		bindPoints := []*xweb.BindPointConfig{
			{
				Address: bindPoint1Address,
			},
			{
				Address: bindPoint2Address,
			},
		}

		issuers := getPossibleIssuers(id, bindPoints)

		req.Len(issuers, 21)
		req.Contains(issuers, "test1.example.com:1234")
		req.Contains(issuers, "test2.example.com:443")
		req.Contains(issuers, "test2.example.com")

		req.Contains(issuers, "client2.netfoundry.io:1234")
		req.Contains(issuers, "client2.netfoundry.io:443")
		req.Contains(issuers, "client2.netfoundry.io")

		req.Contains(issuers, "client3.netfoundry.io:1234")
		req.Contains(issuers, "client3.netfoundry.io:443")
		req.Contains(issuers, "client3.netfoundry.io")

		req.Contains(issuers, "client4.netfoundry.io:1234")
		req.Contains(issuers, "client4.netfoundry.io:443")
		req.Contains(issuers, "client4.netfoundry.io")

		req.Contains(issuers, "127.0.0.1:1234")
		req.Contains(issuers, "127.0.0.1:443")
		req.Contains(issuers, "127.0.0.1")

		req.Contains(issuers, "10.8.0.1:1234")
		req.Contains(issuers, "10.8.0.1:443")
		req.Contains(issuers, "10.8.0.1")

		req.Contains(issuers, "192.168.0.1:1234")
		req.Contains(issuers, "192.168.0.1:443")
		req.Contains(issuers, "192.168.0.1")

	})
}

// helpers

var testSerial = int64(0)

func mkCaCert(cn string) (crypto.Signer, *x509.Certificate) {
	testSerial++

	key, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(testSerial),
		Subject: pkix.Name{
			Organization:       []string{"OpenZiti Identity Tests"},
			OrganizationalUnit: []string{"CA Certs"},
			CommonName:         cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{}, // CAs typically don’t need ExtKeyUsage
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            2,
		MaxPathLenZero:        false,
	}

	return key, cert
}

func mkServerCert(cn string, dns []string, ips []net.IP) (crypto.Signer, *x509.Certificate) {
	testSerial++

	key, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(testSerial),
		Subject: pkix.Name{
			Organization:       []string{"OpenZiti Identity Tests"},
			OrganizationalUnit: []string{"Server Certs"},
			CommonName:         cn,
		},
		DNSNames:              dns,
		IPAddresses:           ips,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	return key, cert
}

func mkClientCert(cn string) (crypto.Signer, *x509.Certificate) {
	key, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(testSerial),
		Subject: pkix.Name{
			Organization:       []string{"OpenZiti Identity Tests"},
			OrganizationalUnit: []string{"Client Certs"},
			CommonName:         cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	return key, cert
}
