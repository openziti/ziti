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
	"github.com/golang-jwt/jwt/v5"
)

type Signer interface {
	Generate(jwt.Claims) (string, error)
	SigningMethod() jwt.SigningMethod
	KeyId() string
}

type SignerImpl struct {
	signingMethod jwt.SigningMethod
	issuer        string
	key           crypto.PrivateKey
	keyId         string
}

func New(sm jwt.SigningMethod, key crypto.PrivateKey, keyId string) *SignerImpl {
	return &SignerImpl{
		signingMethod: sm,
		key:           key,
		keyId:         keyId,
	}
}

func (j *SignerImpl) SigningMethod() jwt.SigningMethod {
	return j.signingMethod
}

func (j *SignerImpl) KeyId() string {
	return j.keyId
}

func (j *SignerImpl) Generate(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(j.signingMethod, claims)

	if j.keyId != "" {
		token.Header["kid"] = j.keyId
	}

	return token.SignedString(j.key)
}
