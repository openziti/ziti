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
package model

import (
	"crypto/x509"
	"github.com/golang-jwt/jwt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/storage/boltz"
	nfPem "github.com/openziti/foundation/util/pem"
	cmap "github.com/orcaman/concurrent-map"
	"go.etcd.io/bbolt"
	"strings"
)

var _ AuthProcessor = &AuthModuleExtJwt{}

const AuthMethodExtJwt = "ext-jwt"

type AuthModuleExtJwt struct {
	env     Env
	method  string
	signers cmap.ConcurrentMap //map[fingerprint string]*signer
}

func NewAuthModuleExtJwt(env Env) *AuthModuleExtJwt {
	ret := &AuthModuleExtJwt{
		env:     env,
		method:  AuthMethodExtJwt,
		signers: cmap.New(),
	}

	env.GetStores().ExternalJwtSigner.AddListener(boltz.EventCreate, ret.onExternalSignerCreateOrUpdate)
	env.GetStores().ExternalJwtSigner.AddListener(boltz.EventDelete, ret.onExternalSignerDelete)

	ret.loadExistingSigners()

	return ret
}

type signerRecord struct {
	externalJwtSigner *persistence.ExternalJwtSigner
	cert              *x509.Certificate
}

func (a *AuthModuleExtJwt) CanHandle(method string) bool {
	return method == a.method
}
func (a *AuthModuleExtJwt) pubKeyLookup(token *jwt.Token) (interface{}, error) {
	fingerprintToSignerRectInterface := a.getKnownSignerRecords()

	kidVal, ok := token.Header["kid"]

	if !ok {
		return nil, apierror.NewInvalidAuth()
	}

	kid, ok := kidVal.(string)

	if !ok {
		return nil, apierror.NewInvalidAuth()
	}

	signerRecordInterface, ok := fingerprintToSignerRectInterface[kid]

	if !ok {
		return nil, apierror.NewInvalidAuth()
	}

	signerRecord := signerRecordInterface.(*signerRecord)

	if !signerRecord.externalJwtSigner.Enabled {
		return nil, apierror.NewInvalidAuth()
	}

	return signerRecord.cert.PublicKey, nil
}

func (a AuthModuleExtJwt) Process(context AuthContext) (identityId string, authenticatorId string, err error) {
	headers := map[string]interface{}{}

	for key, value := range context.GetHeaders() {
		headers[strings.ToLower(key)] = value
	}

	authHeaders := headers["authorization"].([]string)

	if len(authHeaders) != 1 {
		return "", "", apierror.NewInvalidAuth()
	}
	authHeader := authHeaders[0]

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", "", apierror.NewInvalidAuth()
	}

	jwtStr := strings.Replace(authHeader, "Bearer ", "", 1)

	jwtToken, err := jwt.Parse(jwtStr, a.pubKeyLookup)

	if err == nil && jwtToken.Valid {
		claims := jwtToken.Claims.(jwt.MapClaims)
		return claims["sub"].(string), AuthMethodExtJwt, nil
	}

	return "", "", apierror.NewInvalidAuth()
}

func (a *AuthModuleExtJwt) getKnownSignerRecords() map[string]interface{} {
	return a.signers.Items()
}

func (a *AuthModuleExtJwt) onExternalSignerCreateOrUpdate(args ...interface{}) {
	signer, ok := args[0].(*persistence.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: expected %T got %T", a, signer, args[0])
	}

	certs := nfPem.PemStringToCertificates(signer.CertPem)

	a.signers.Set(signer.Fingerprint, &signerRecord{
		externalJwtSigner: signer,
		cert:              certs[0],
	})
}

func (a *AuthModuleExtJwt) onExternalSignerDelete(args ...interface{}) {
	signer, ok := args[0].(*persistence.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: expected %T got %T", a, signer, args[0])
	}

	a.signers.Remove(signer.Fingerprint)
}

func (a *AuthModuleExtJwt) loadExistingSigners() {
	err := a.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		ids, _, err := a.env.GetStores().ExternalJwtSigner.QueryIds(tx, "")

		if err != nil {
			return err
		}

		for _, id := range ids {
			signer, err := a.env.GetStores().ExternalJwtSigner.LoadOneById(tx, id)
			if err != nil {
				return err
			}

			certs := nfPem.PemStringToCertificates(signer.CertPem)

			a.signers.Set(signer.Fingerprint, certs[0])
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().Errorf("error loading external jwt signers: %v", err)
	}
}
