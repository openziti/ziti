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

const (
	AuthMethodExtJwt    = "ext-jwt"
	ExtJwtInternalClaim = "-internal-ext-jwt"
)

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
		pfxlog.Logger().Error("missing kid")
		return nil, apierror.NewInvalidAuth()
	}

	kid, ok := kidVal.(string)

	if !ok {
		pfxlog.Logger().Error("kid is not a string")
		return nil, apierror.NewInvalidAuth()
	}

	signerRecordInterface, ok := fingerprintToSignerRectInterface[kid]

	if !ok {
		pfxlog.Logger().Error("unknown kid")
		return nil, apierror.NewInvalidAuth()
	}

	signerRecord := signerRecordInterface.(*signerRecord)

	if !signerRecord.externalJwtSigner.Enabled {
		pfxlog.Logger().WithField("externalJwtId", signerRecord.externalJwtSigner.Id).Error("external jwt is disabled")
		return nil, apierror.NewInvalidAuth()
	}
	mapClaims := token.Claims.(jwt.MapClaims)
	mapClaims[ExtJwtInternalClaim] = signerRecord.externalJwtSigner

	return signerRecord.cert.PublicKey, nil
}

func (a AuthModuleExtJwt) Process(context AuthContext) (identityId, externalId, authenticatorId string, err error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	headers := map[string]interface{}{}

	for key, value := range context.GetHeaders() {
		headers[strings.ToLower(key)] = value
	}

	authHeaders := headers["authorization"].([]string)

	if len(authHeaders) != 1 {
		logger.Error("no authorization header found")
		return "", "", "", apierror.NewInvalidAuth()
	}
	authHeader := authHeaders[0]

	if !strings.HasPrefix(authHeader, "Bearer ") {
		logger.Error("authorization header missing Bearer prefix")
		return "", "", "", apierror.NewInvalidAuth()
	}

	jwtStr := strings.Replace(authHeader, "Bearer ", "", 1)

	jwtToken, err := jwt.Parse(jwtStr, a.pubKeyLookup)

	if err == nil && jwtToken.Valid {
		claimsProperty := "sub"
		mapClaims := jwtToken.Claims.(jwt.MapClaims)

		extJwt := mapClaims[ExtJwtInternalClaim].(*persistence.ExternalJwtSigner)

		if extJwt == nil {
			logger.Error("no external jwt signer found for internal claims")
			return "", "", "", apierror.NewInvalidAuth()
		}

		logger = logger.WithField("externalJwtSignerId", extJwt.Id)

		if extJwt.ClaimsProperty != nil {
			claimsProperty = *extJwt.ClaimsProperty
		}

		logger = logger.WithField("claimsProperty", claimsProperty)

		identityIdInterface, ok := mapClaims[claimsProperty]

		if !ok {
			logger.Error("claims property on external jwt signer not found in claims")
			return "", "", "", apierror.NewInvalidAuth()
		}

		identityId, ok := identityIdInterface.(string)

		if !ok || identityId == "" {
			logger.Error("expected claims id was not a string or was empty")
			return "", "", "", apierror.NewInvalidAuth()
		}

		var authPolicy *AuthPolicy

		var identity *Identity
		if extJwt.UseExternalId {
			authPolicy, identity, err = getAuthPolicyByExternalId(a.env, AuthMethodExtJwt, "", identityId)
		} else {
			authPolicy, identity, err = getAuthPolicyByIdentityId(a.env, AuthMethodExtJwt, "", identityId)
		}

		if err != nil {
			logger.WithError(err).Error("encountered unhandled error during authentication")
			return "", "", "", apierror.NewInvalidAuth()
		}

		if authPolicy == nil {
			logger.WithError(err).Error("encountered unhandled nil auth policy during authentication")
			return "", "", "", apierror.NewInvalidAuth()
		}

		if identity.Disabled {
			logger.
				WithField("disabledAt", identity.DisabledAt).
				WithField("disabledUntil", identity.DisabledUntil).
				Error("authentication failed, identity is disabled")
			return "", "", "", apierror.NewInvalidAuth()
		}

		if !authPolicy.Primary.ExtJwt.Allowed {
			logger.Error("external jwt authentication on auth policy is disabled")
			return "", "", "", apierror.NewInvalidAuth()
		}

		if len(authPolicy.Primary.ExtJwt.AllowedExtJwtSigners) > 0 {
			found := false
			for _, allowedId := range authPolicy.Primary.ExtJwt.AllowedExtJwtSigners {
				if allowedId == extJwt.Id {
					found = true
					break
				}
			}

			if !found {
				logger.
					WithField("allowedSigners", authPolicy.Primary.ExtJwt.AllowedExtJwtSigners).
					Error("auth policy does not allow specified signer")
				return "", "", "", apierror.NewInvalidAuth()
			}
		}

		if extJwt.UseExternalId {
			return "", identityId, AuthMethodExtJwt, nil
		}
		return identityId, "", AuthMethodExtJwt, nil
	}

	logger.Error("authorization failed, jwt did not verify")
	return "", "", "", apierror.NewInvalidAuth()
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
