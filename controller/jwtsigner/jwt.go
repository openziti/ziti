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

package jwtsigner

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Signer provides JWT signing capabilities with configurable signing methods and key identification.
// It abstracts the process of creating signed JWT tokens from claims.
type Signer interface {
	Generate(jwt.Claims) (string, error)
	SigningMethod() jwt.SigningMethod
	KeyId() string
}

// SignerImpl is the concrete implementation of the Signer interface.
// It holds the signing method, private key, and key identifier needed for JWT signing.
type SignerImpl struct {
	signingMethod jwt.SigningMethod
	key           crypto.PrivateKey
	keyId         string
}

// New creates a new SignerImpl with the specified signing method, private key, and key ID.
// The key ID is used for JWT key identification in multi-key scenarios.
func New(sm jwt.SigningMethod, key crypto.PrivateKey, keyId string) *SignerImpl {
	return &SignerImpl{
		signingMethod: sm,
		key:           key,
		keyId:         keyId,
	}
}

// SigningMethod returns the JWT signing method configured for this signer.
func (j *SignerImpl) SigningMethod() jwt.SigningMethod {
	return j.signingMethod
}

// KeyId returns the key identifier associated with this signer.
// This is used in the JWT header 'kid' field for key identification.
func (j *SignerImpl) KeyId() string {
	return j.keyId
}

// Generate creates a signed JWT token from the provided claims.
// If a key ID is configured, it adds the 'kid' header to the token.
// Returns the signed JWT string or an error if signing fails.
func (j *SignerImpl) Generate(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(j.signingMethod, claims)

	if j.keyId != "" {
		token.Header["kid"] = j.keyId
	}

	return token.SignedString(j.key)
}

// TlsJwtSigner combines a JWT signer with its associated TLS certificate.
// It wraps a jwtsigner.Signer and stores the certificate that was used to
// create the signer, enabling JWT signing operations with certificate-based
// key identification (kid).
type TlsJwtSigner struct {
	Signer
	TlsCerts *tls.Certificate
}

// Set configures the TlsJwtSigner with a new TLS certificate.
// It updates the stored certificate, determines the appropriate JWT signing method
// based on the certificate's key type, generates a key ID (kid) from the certificate's
// SHA1 hash, and creates a new JWT signer with these parameters.
func (c *TlsJwtSigner) Set(cert *tls.Certificate) error {
	c.TlsCerts = cert
	signingMethod, err := GetJwtSigningMethod(cert)

	if err != nil {
		return err
	}
	kid := fmt.Sprintf("%x", sha1.Sum(cert.Certificate[0]))
	c.Signer = New(signingMethod, c.TlsCerts.PrivateKey, kid)

	return nil
}

// GetJwtSigningMethod determines the appropriate JWT signing method based on the
// certificate's public key type and parameters.
// For ECDSA keys, it selects ES256, ES384, or ES512 based on the curve bit size.
// For RSA keys, it defaults to RS256.
// Panics if the certificate has an unsupported key type or ECDSA curve size.
func GetJwtSigningMethod(cert *tls.Certificate) (jwt.SigningMethod, error) {

	var sm jwt.SigningMethod

	if cert.Leaf == nil {
		if len(cert.Certificate) == 0 {
			return nil, fmt.Errorf("no certificates found")
		}
		var err error
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])

		if err != nil {
			return nil, fmt.Errorf("could not parse first certificate as leaf: %w", err)
		}
	}

	switch cert.Leaf.PublicKey.(type) {
	case *ecdsa.PublicKey:
		key := cert.Leaf.PublicKey.(*ecdsa.PublicKey)
		switch key.Params().BitSize {
		case jwt.SigningMethodES256.CurveBits:
			sm = jwt.SigningMethodES256
		case jwt.SigningMethodES384.CurveBits:
			sm = jwt.SigningMethodES384
		case jwt.SigningMethodES512.CurveBits:
			sm = jwt.SigningMethodES512
		default:

			return nil, fmt.Errorf("unsupported EC key size: %d", key.Params().BitSize)
		}

	case *rsa.PublicKey:
		sm = jwt.SigningMethodRS256
	default:
		return nil, errors.New("unknown certificate type, unable to determine signing method")
	}

	return sm, nil
}
