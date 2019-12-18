/*
	Copyright 2019 Netfoundry, Inc.

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
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"math/big"
	"net"
	"net/url"
	"strings"
	"time"
)

type SignFunc func([]byte, *SigningOpts) ([]byte, error)

type CertPem struct {
	Cert *x509.Certificate
	Pem  []byte
}

type Signer interface {
	Sign([]byte, *SigningOpts) ([]byte, error)
	ToPem([]byte) ([]byte, error)
	SigningCertPEM() string
}

type SigningOpts struct {
	// Subject Alternate Name values.
	DNSNames       []string
	EmailAddresses []string
	IPAddresses    []net.IP
	URIs           []*url.URL
}

func (so *SigningOpts) Apply(c *x509.Certificate) {
	c.DNSNames = so.DNSNames
	c.EmailAddresses = so.EmailAddresses
	c.IPAddresses = so.IPAddresses
	c.URIs = so.URIs
}

type SerialGenerator interface {
	Generate() *big.Int
}

type DefaultSerialGenerator struct{}

func (DefaultSerialGenerator) Generate() *big.Int {
	//@todo this need to be better, this does not include negative numbers for 20bit values, nor is this managed
	r, _ := rand.Int(rand.Reader, big.NewInt(524287))

	return r
}

type ServerSigner struct {
	caCert          *x509.Certificate
	caKey           interface{}
	SerialGenerator SerialGenerator
}

func NewServerSigner(caCert *x509.Certificate, caKey interface{}) *ServerSigner {
	return &ServerSigner{
		caCert:          caCert,
		caKey:           caKey,
		SerialGenerator: &DefaultSerialGenerator{},
	}
}

func (s *ServerSigner) SigningCertPEM() string {
	b, err := s.ToPem(s.caCert.Raw)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to convert signing cert into PEM")
		return ""
	}

	return string(b)
}

func (s *ServerSigner) ToPem(raw []byte) ([]byte, error) {
	return cert2pem(raw)
}

func (s *ServerSigner) Sign(csrPem []byte, opts *SigningOpts) ([]byte, error) {
	csr, err := ParseCsr(csrPem)

	if err != nil {
		return nil, fmt.Errorf("unable to parse PEM bytes as CSR: %s", err)
	}

	if err = csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("CSR signature validation failed: %s", err)
	}

	// create client certificate template
	certTemplate := x509.Certificate{
		Signature:          csr.Signature,
		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		SerialNumber: s.SerialGenerator.Generate(),
		Issuer:       s.caCert.Subject,
		Subject:      csr.Subject,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().AddDate(1, 0, 0), //@todo make this an option?
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         false,
	}

	if opts != nil {
		opts.Apply(&certTemplate)
	}

	cert, err := x509.CreateCertificate(rand.Reader, &certTemplate, s.caCert, csr.PublicKey, s.caKey)

	if err != nil {
		return nil, fmt.Errorf("could not sign cert: %s", err)
	}

	return cert, nil
}

type ClientSigner struct {
	caCert          *x509.Certificate
	caKey           interface{}
	SerialGenerator SerialGenerator
}

func (s *ClientSigner) SigningCertPEM() string {
	return ""
}

func NewClientSigner(caCert *x509.Certificate, caKey interface{}) *ClientSigner {
	return &ClientSigner{
		caCert:          caCert,
		caKey:           caKey,
		SerialGenerator: &DefaultSerialGenerator{},
	}
}

func (s *ClientSigner) ToPem(raw []byte) ([]byte, error) {
	return cert2pem(raw)
}

func (s *ClientSigner) Sign(csrPem []byte, opts *SigningOpts) ([]byte, error) {
	csr, err := ParseCsr(csrPem)

	if err != nil {
		return nil, fmt.Errorf("unable to parse PEM bytes as CSR: %s", err)
	}

	if err = csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("CSR signature validation failed: %s", err)
	}

	// create client certificate template
	certTemplate := x509.Certificate{
		Signature: csr.Signature,

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		SerialNumber: s.SerialGenerator.Generate(),
		Issuer:       s.caCert.Subject,
		Subject:      csr.Subject,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:         false,
	}

	if opts != nil {
		opts.Apply(&certTemplate)
	}

	cert, err := x509.CreateCertificate(rand.Reader, &certTemplate, s.caCert, csr.PublicKey, s.caKey)

	if err != nil {
		return nil, fmt.Errorf("could not sign cert: %s", err)
	}

	return cert, nil
}

func cert2pem(raw []byte) ([]byte, error) {
	cert := bytes.NewBuffer(make([]byte, 0))

	err := pem.Encode(cert, &pem.Block{Type: "CERTIFICATE", Bytes: raw})

	if err != nil {
		return nil, fmt.Errorf("could not create pem encoding: %s", err)
	}

	return cert.Bytes(), nil
}

func ParseCsr(csrPem []byte) (*x509.CertificateRequest, error) {
	if len(csrPem) == 0 {
		return nil, errors.New("csrPem must not be null or empty")
	}

	pemBlock, remainder := pem.Decode(csrPem)
	if pemBlock == nil {
		return nil, fmt.Errorf("could not decode csrPem as PEM")
	}

	if remainder == nil || len(remainder) != 0 {
		return nil, fmt.Errorf("unexpected PEM blocks at end of CSR")
	}

	return x509.ParseCertificateRequest(pemBlock.Bytes)
}

func PemChain2Blocks(pemBuff string) ([]*pem.Block, error) {
	remainder := []byte(strings.TrimSpace(pemBuff))

	var b *pem.Block
	numBlock := 0

	var blocks []*pem.Block
	for len(remainder) > 0 {
		numBlock++
		b, remainder = pem.Decode(remainder)

		if b == nil {
			return nil, fmt.Errorf("could not parse block #%d", numBlock)
		}

		if b.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("block #%d is not a certificate", numBlock)
		}

		blocks = append(blocks, b)
	}

	return blocks, nil
}

func Blocks2Certs(blocks []*pem.Block) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	numCert := 0
	for _, b := range blocks {
		numCert++

		c, err := x509.ParseCertificate(b.Bytes)

		if err != nil {
			return nil, fmt.Errorf("could not parse block #%d as an x509 certificate", numCert)
		}

		certs = append(certs, c)
	}

	return certs, nil
}
