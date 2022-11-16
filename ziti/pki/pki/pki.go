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

// Package pki provides helpers to manage a Public Key Infrastructure.
package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/openziti/ziti/ziti/pki/certificate"
	"github.com/openziti/ziti/ziti/pki/store"
	"time"

	"github.com/openziti/identity/certtools"
)

const (
	defaultPrivateKeySize = 4096
)

// Signing errors.
var (
	ErrCannotSelfSignNonCA = errors.New("cannot self sign non CA request")
	ErrMaxPathLenReached   = errors.New("max path len reached")
)

// Request is a struct for providing configuration to
// GenerateCertificate when actioning a certification generation request.
type Request struct {
	Name                string
	KeyName             string
	IsClientCertificate bool
	PrivateKeySize      int
	Template            *x509.Certificate
}
type CSRRequest struct {
	Name                string
	IsClientCertificate bool
	PrivateKey          rsa.PrivateKey
	Template            *x509.CertificateRequest
}

// ZitiPKI wraps helpers to handle a Public Key Infrastructure.
type ZitiPKI struct {
	Store store.Store
}

// GetCA fetches and returns the named Certificate Authority bundle
// from the store.
func (e *ZitiPKI) GetCA(name string) (*certificate.Bundle, error) {
	return e.GetBundle(name, name)
}

// GetBundle fetches and returns a certificate bundle from the store.
func (e *ZitiPKI) GetBundle(caName, name string) (*certificate.Bundle, error) {
	k, c, err := e.Store.Fetch(caName, name)
	if err != nil {
		return nil, fmt.Errorf("failed fetching bundle %v within CA %v: %v", name, caName, err)
	}

	return certificate.RawToBundle(name, k, c)
}

// GetPrivateKey fetches and returns a private key from the store.
func (e *ZitiPKI) GetPrivateKey(caname string, keyname string) (crypto.PrivateKey, error) {
	k, err := e.Store.FetchKeyBytes(caname, keyname)
	if err != nil {
		return nil, fmt.Errorf("failed fetching bundle %v within CA %v: %v", caname, keyname, err)
	}
	return certtools.LoadPrivateKey(k)
}

// Sign signs a generated certificate bundle based on the given request with
// the given signer.
func (e *ZitiPKI) Sign(signer *certificate.Bundle, req *Request) error {
	if !req.Template.IsCA && signer == nil {
		return ErrCannotSelfSignNonCA
	}
	if req.Template.IsCA && signer != nil && signer.Cert.MaxPathLen == 0 {
		return ErrMaxPathLenReached
	}

	var err error
	var privateKey *rsa.PrivateKey

	if req.KeyName == "" {
		if req.PrivateKeySize == 0 {
			req.PrivateKeySize = defaultPrivateKeySize
		}
		privateKey, err = rsa.GenerateKey(rand.Reader, req.PrivateKeySize)
		if err != nil {
			return fmt.Errorf("failed generating private key: %v", err)
		}
	} else {
		pk, err := e.GetPrivateKey(signer.Name, req.KeyName)
		if err != nil {
			return fmt.Errorf("failed fetching private key: %v", err)
		}
		privateKey = pk.(*rsa.PrivateKey)
	}
	publicKey := privateKey.Public()

	if err := defaultTemplate(req, publicKey); err != nil {
		return fmt.Errorf("failed updating generation request: %v", err)
	}

	if req.Template.IsCA {
		var intermediateCA bool
		if signer != nil {
			intermediateCA = true
			if signer.Cert.MaxPathLen > 0 {
				req.Template.MaxPathLen = signer.Cert.MaxPathLen - 1
			}
		}
		if err := caTemplate(req, intermediateCA); err != nil {
			return fmt.Errorf("failed updating generation request for CA: %v", err)
		}
		if !intermediateCA {
			// Use the generated certificate template and private key (self-signing).
			signer = &certificate.Bundle{Name: req.Name, Cert: req.Template, Key: privateKey}
		}
	} else {
		nonCATemplate(req)
	}

	rawCert, err := x509.CreateCertificate(rand.Reader, req.Template, signer.Cert, publicKey, signer.Key)
	if err != nil {
		return fmt.Errorf("failed creating and signing certificate: %v", err)
	}

	if err := e.Store.Add(signer.Name, req.Name, req.Template.IsCA, x509.MarshalPKCS1PrivateKey(privateKey), rawCert); err != nil {
		return fmt.Errorf("failed saving generated bundle: %v", err)
	}
	return nil
}

// Chain will...
func (e *ZitiPKI) Chain(signer *certificate.Bundle, req *Request) error {
	if err := e.Store.Chain(signer.Name, req.Name); err != nil {
		return fmt.Errorf("failed saving generated chain: %v", err)
	}
	return nil
}

// Generate and store a private key
func (e *ZitiPKI) GeneratePrivateKey(signer *certificate.Bundle, req *Request) error {
	if req.PrivateKeySize == 0 {
		req.PrivateKeySize = defaultPrivateKeySize
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, req.PrivateKeySize)
	if err != nil {
		return fmt.Errorf("failed generating private key: %v", err)
	}
	if err := e.Store.AddKey(signer.Name, req.KeyName, x509.MarshalPKCS1PrivateKey(privateKey)); err != nil {
		return fmt.Errorf("failed saving generated key: %v", err)
	}
	return nil
}

// Revoke revokes the given certificate from the store.
func (e *ZitiPKI) Revoke(caName string, cert *x509.Certificate) error {
	if err := e.Store.Update(caName, cert.SerialNumber, certificate.Revoked); err != nil {
		return fmt.Errorf("failed revoking certificate: %v", err)
	}
	return nil
}

// CRL builds a CRL for a given CA based on the revoked certs.
func (e *ZitiPKI) CRL(caName string, expire time.Time) ([]byte, error) {
	revoked, err := e.Store.Revoked(caName)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving revoked certificates for %v: %v", caName, err)
	}
	ca, err := e.GetCA(caName)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving CA bundle %v: %v", caName, err)
	}

	crl, err := ca.Cert.CreateCRL(rand.Reader, ca.Key, revoked, time.Now(), expire)
	if err != nil {
		return nil, fmt.Errorf("failed creating crl for %v: %v", caName, err)
	}
	return crl, nil
}

// CSR generates a csr certificate
func (e *ZitiPKI) CSR(caname string, bundleName string, csrTemplate x509.CertificateRequest, privateKey crypto.PrivateKey) error {
	csrCertificate, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		return err
	}
	if rsaK, ok := privateKey.(*rsa.PrivateKey); ok {
		if err := e.Store.AddCSR(caname, bundleName, false, x509.MarshalPKCS1PrivateKey(rsaK), csrCertificate); err != nil {
			return fmt.Errorf("failed saving generated CSR: %v", err)
		}
	} else if ecK, ok := privateKey.(*ecdsa.PrivateKey); ok {
		der, _ := x509.MarshalECPrivateKey(ecK)

		if err := e.Store.AddCSR(caname, bundleName, false, der, csrCertificate); err != nil {
			return fmt.Errorf("failed saving generated CSR: %v", err)
		}
	} else {
		return fmt.Errorf("Unsupported key type")
	}
	return nil
}
