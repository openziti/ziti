/*
	Copyright NetFoundry, Inc.

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
	"github.com/golang-jwt/jwt"
)

type Signer interface {
	Generate(string, string, jwt.MapClaims) (string, error)
}

type IdentitySigner struct {
	signingMethod jwt.SigningMethod
	issuer        string
	key           interface{}
}

func New(issuer string, sm jwt.SigningMethod, key interface{}) *IdentitySigner {
	return &IdentitySigner{
		signingMethod: sm,
		issuer:        issuer,
		key:           key,
	}
}

func (j *IdentitySigner) Generate(subj, jti string, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(j.signingMethod, claims)
	return token.SignedString(j.key)
}
