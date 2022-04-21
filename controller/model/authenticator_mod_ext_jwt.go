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
	nfPem "github.com/openziti/foundation/util/pem"
	"github.com/openziti/storage/boltz"
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
	signers cmap.ConcurrentMap //map[kid string]*signer
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
	kidToSignerRectInterface := a.getKnownSignerRecords()

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

	signerRecordInterface, ok := kidToSignerRectInterface[kid]

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

func (a *AuthModuleExtJwt) Process(context AuthContext) (identityId, externalId, authenticatorId string, err error) {
	return a.process(context, true)
}

func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (identityId, externalId, authenticatorId string, err error) {
	return a.process(context, false)
}

func (a *AuthModuleExtJwt) process(context AuthContext, isPrimary bool) (identityId, externalId, authenticatorId string, err error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	headers := map[string]interface{}{}

	for key, value := range context.GetHeaders() {
		headers[strings.ToLower(key)] = value
	}

	var authHeaders []string

	if authHeaderVal, ok := headers["authorization"]; ok {
		authHeaders = authHeaderVal.([]string)
	}

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

	//pubKeyLookup also handles extJwtSigner.enabled checking
	jwtToken, err := jwt.Parse(jwtStr, a.pubKeyLookup)

	if err == nil && jwtToken.Valid {
		mapClaims := jwtToken.Claims.(jwt.MapClaims)
		extJwt := mapClaims[ExtJwtInternalClaim].(*persistence.ExternalJwtSigner)

		if extJwt == nil {
			logger.Error("no external jwt signer found for internal claims")
			return "", "", "", apierror.NewInvalidAuth()
		}

		logger = logger.WithField("externalJwtSignerId", extJwt.Id)

		issuer := ""
		if issuerVal, ok := mapClaims["iss"]; ok {
			issuer, ok = issuerVal.(string)
			if !ok {
				logger.Error("issuer in claims was not a string")
				return "", "", "", apierror.NewInvalidAuth()
			}
		}

		logger = logger.WithField("claimsIssuer", issuer)

		if extJwt.Issuer != nil && *extJwt.Issuer != issuer {
			logger.WithField("expectedIssuer", *extJwt.Issuer).Error("invalid issuer")
			return "", "", "", apierror.NewInvalidAuth()
		}

		if extJwt.Audience != nil {
			audValues := mapClaims["aud"]

			if audValues == nil {
				logger.WithField("audience", audValues).Error("audience is missing")
				return "", "", "", apierror.NewInvalidAuth()
			}

			audSlice, ok := audValues.([]string)

			if !ok {
				audString, ok := audValues.(string)

				if !ok {
					logger.WithField("audience", audValues).Error("audience is not a string or array of strings")
					return "", "", "", apierror.NewInvalidAuth()
				}

				audSlice = []string{audString}
			}

			found := false
			for _, validAud := range audSlice {
				if validAud == *extJwt.Audience {
					found = true
					break
				}
			}

			if !found {
				logger.WithField("expectedAudience", *extJwt.Audience).WithField("claimsAudiences", audSlice).Error("invalid audience")
				return "", "", "", apierror.NewInvalidAuth()
			}
		}

		idClaimProperty := "sub"
		if extJwt.ClaimsProperty != nil {
			idClaimProperty = *extJwt.ClaimsProperty
		}

		logger = logger.WithField("idClaimProperty", idClaimProperty)

		identityIdInterface, ok := mapClaims[idClaimProperty]

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
		externalJwtSignerId := ""
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

		if isPrimary {
			if len(authPolicy.Primary.ExtJwt.AllowedExtJwtSigners) > 0 {
				found := false
				for _, allowedId := range authPolicy.Primary.ExtJwt.AllowedExtJwtSigners {
					if allowedId == extJwt.Id {
						externalJwtSignerId = allowedId
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
		} else if authPolicy.Secondary.RequiredExtJwtSigner != nil {
			if extJwt.Id != *authPolicy.Secondary.RequiredExtJwtSigner {
				return "", "", "", apierror.NewInvalidAuth()
			}

			externalJwtSignerId = extJwt.Id
		}

		if extJwt.UseExternalId {
			return "", identityId, externalJwtSignerId, nil
		}
		return identityId, "", externalJwtSignerId, nil
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

	a.addSigner(signer)
}

func (a *AuthModuleExtJwt) addSigner(signer *persistence.ExternalJwtSigner) {
	certs := nfPem.PemStringToCertificates(signer.CertPem)

	a.signers.Set(signer.Kid, &signerRecord{
		externalJwtSigner: signer,
		cert:              certs[0],
	})
}

func (a *AuthModuleExtJwt) onExternalSignerDelete(args ...interface{}) {
	signer, ok := args[0].(*persistence.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: expected %T got %T", a, signer, args[0])
	}

	a.signers.Remove(signer.Kid)
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

			a.addSigner(signer)
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().Errorf("error loading external jwt signers: %v", err)
	}
}
