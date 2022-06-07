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
	"encoding/base64"
	"fmt"
	"github.com/golang-jwt/jwt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	nfPem "github.com/openziti/foundation/util/pem"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/jwks"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
	"sync"
	"time"
)

var _ AuthProcessor = &AuthModuleExtJwt{}

const (
	AuthMethodExtJwt    = "ext-jwt"
	ExtJwtInternalClaim = "-internal-ext-jwt"
)

type AuthModuleExtJwt struct {
	env     Env
	method  string
	signers cmap.ConcurrentMap[*signerRecord]
}

func NewAuthModuleExtJwt(env Env) *AuthModuleExtJwt {
	ret := &AuthModuleExtJwt{
		env:     env,
		method:  AuthMethodExtJwt,
		signers: cmap.New[*signerRecord](),
	}

	env.GetStores().ExternalJwtSigner.AddListener(boltz.EventCreate, ret.onExternalSignerCreate)
	env.GetStores().ExternalJwtSigner.AddListener(boltz.EventUpdate, ret.onExternalSignerUpdate)
	env.GetStores().ExternalJwtSigner.AddListener(boltz.EventDelete, ret.onExternalSignerDelete)

	ret.loadExistingSigners()

	return ret
}

type signerRecord struct {
	sync.Mutex
	jwksLastRequest time.Time

	kidToCertificate  map[string]*x509.Certificate
	jwksResponse      *jwks.Response
	externalJwtSigner *persistence.ExternalJwtSigner

	jwksResolver jwks.Resolver
}

func (r *signerRecord) Resolve(force bool) error {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	if r.externalJwtSigner.CertPem != nil {
		if len(r.kidToCertificate) != 0 && !force {
			return nil
		}

		certs := nfPem.PemStringToCertificates(*r.externalJwtSigner.CertPem)

		if len(certs) == 0 {
			return errors.New("could not add signer, PEM did not parse to any certificates")
		}

		// first cert only
		r.kidToCertificate = map[string]*x509.Certificate{
			*r.externalJwtSigner.Kid: certs[0],
		}

		return nil

	} else if r.externalJwtSigner.JwksEndpoint != nil {
		if len(r.kidToCertificate) != 0 && !force {
			return nil
		}

		if !r.jwksLastRequest.IsZero() && time.Now().Sub(r.jwksLastRequest) < time.Second*5 {
			return nil
		}

		r.jwksLastRequest = time.Now()

		jwksResponse, _, err := r.jwksResolver.Get(*r.externalJwtSigner.JwksEndpoint)

		if err != nil {
			return fmt.Errorf("could not resolve jwks endpoint: %v", err)
		}

		for _, key := range jwksResponse.Keys {
			if len(key.X509Chain) == 0 {
				return errors.New("could not parse JWKS keys, x509 chain was empty")
			}

			x509Der, err := base64.StdEncoding.DecodeString(key.X509Chain[0])

			if err != nil {
				return fmt.Errorf("could not parse JWKS keys: %v", err)
			}

			certs, err := x509.ParseCertificates(x509Der)

			if err != nil {
				return fmt.Errorf("could not parse JWKS DER as x509: %v", err)
			}

			if len(certs) == 0 {
				return fmt.Errorf("no ceritficates parsed")
			}

			r.kidToCertificate[key.KeyId] = certs[0]
		}

		r.jwksResponse = jwksResponse

		return nil
	}

	return errors.New("instructed to add external jwt signer that does not have a certificate PEM or JWKS endpoint")
}

func (a *AuthModuleExtJwt) CanHandle(method string) bool {
	return method == a.method
}
func (a *AuthModuleExtJwt) pubKeyLookup(token *jwt.Token) (interface{}, error) {
	logger := pfxlog.Logger().WithField("method", a.method)

	kidVal, ok := token.Header["kid"]

	if !ok {
		logger.Error("missing kid")
		return nil, apierror.NewInvalidAuth()
	}

	kid, ok := kidVal.(string)

	if !ok {
		logger.Error("kid is not a string")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("kid", kid)

	claims := token.Claims.(jwt.MapClaims)
	if claims == nil {
		logger.Error("unknown signer, attempting to look up by claims, but claims were nil")
		return nil, apierror.NewInvalidAuth()
	}

	issVal, ok := claims["iss"]

	if !ok {
		logger.Error("unknown signer, attempting to look up by issue, but issuer is missing")
		return nil, apierror.NewInvalidAuth()
	}

	issuer := issVal.(string)

	if issuer == "" {
		logger.Error("unknown signer, attempting to look up by issue, but issuer is empty or not a string")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("issuer", issuer)

	signerRecord, ok := a.signers.Get(issuer)

	if !ok {
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("extJwtSignerId", signerRecord.externalJwtSigner.Id).WithField("extJwtSignerName", signerRecord.externalJwtSigner.Name)

	if !signerRecord.externalJwtSigner.Enabled {
		logger.Error("external jwt is disabled")
		return nil, apierror.NewInvalidAuth()
	}

	cert, ok := signerRecord.kidToCertificate[kid]

	if !ok {
		if err := signerRecord.Resolve(false); err != nil {
			logger.WithError(err).Error("error attempting to resolve extJwtSigner certificate used for signing")
		}
	}

	cert, ok = signerRecord.kidToCertificate[kid]

	if !ok {
		return nil, fmt.Errorf("kid [%s] not found for issuer [%s]", kid, issuer)
	}

	claims[ExtJwtInternalClaim] = signerRecord.externalJwtSigner

	return cert.PublicKey, nil
}

func (a *AuthModuleExtJwt) Process(context AuthContext) (AuthResult, error) {
	return a.process(context, true)
}

func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (AuthResult, error) {
	return a.process(context, false)
}

type AuthResultJwt struct {
	AuthResultBase
	externalJwtSignerId string
	externalJwtSigner   *persistence.ExternalJwtSigner
}

func (a *AuthResultJwt) IsSuccessful() bool {
	return a.externalJwtSignerId != "" && a.AuthResultBase.identityId != ""
}

func (a *AuthModuleExtJwt) process(context AuthContext, isPrimary bool) (AuthResult, error) {
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
		return nil, apierror.NewInvalidAuth()
	}
	authHeader := authHeaders[0]

	if !strings.HasPrefix(authHeader, "Bearer ") {
		logger.Error("authorization header missing Bearer prefix")
		return nil, apierror.NewInvalidAuth()
	}

	jwtStr := strings.Replace(authHeader, "Bearer ", "", 1)

	//pubKeyLookup also handles extJwtSigner.enabled checking
	jwtToken, err := jwt.Parse(jwtStr, a.pubKeyLookup)

	if err == nil && jwtToken.Valid {
		mapClaims := jwtToken.Claims.(jwt.MapClaims)
		extJwt := mapClaims[ExtJwtInternalClaim].(*persistence.ExternalJwtSigner)

		if extJwt == nil {
			logger.Error("no external jwt signer found for internal claims")
			return nil, apierror.NewInvalidAuth()
		}

		logger = logger.WithField("externalJwtSignerId", extJwt.Id)

		issuer := ""
		if issuerVal, ok := mapClaims["iss"]; ok {
			issuer, ok = issuerVal.(string)
			if !ok {
				logger.Error("issuer in claims was not a string")
				return nil, apierror.NewInvalidAuth()
			}
		}

		logger = logger.WithField("claimsIssuer", issuer)

		if extJwt.Issuer != nil && *extJwt.Issuer != issuer {
			logger.WithField("expectedIssuer", *extJwt.Issuer).Error("invalid issuer")
			return nil, apierror.NewInvalidAuth()
		}

		if extJwt.Audience != nil {
			audValues := mapClaims["aud"]

			if audValues == nil {
				logger.WithField("audience", audValues).Error("audience is missing")
				return nil, apierror.NewInvalidAuth()
			}

			audSlice, ok := audValues.([]string)

			if !ok {
				audString, ok := audValues.(string)

				if !ok {
					logger.WithField("audience", audValues).Error("audience is not a string or array of strings")
					return nil, apierror.NewInvalidAuth()
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
				return nil, apierror.NewInvalidAuth()
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
			return nil, apierror.NewInvalidAuth()
		}

		claimsId, ok := identityIdInterface.(string)

		if !ok || claimsId == "" {
			logger.Error("expected claims id was not a string or was empty")
			return nil, apierror.NewInvalidAuth()
		}

		var authPolicy *AuthPolicy

		var identity *Identity
		if extJwt.UseExternalId {
			authPolicy, identity, err = getAuthPolicyByExternalId(a.env, AuthMethodExtJwt, "", claimsId)
		} else {
			authPolicy, identity, err = getAuthPolicyByIdentityId(a.env, AuthMethodExtJwt, "", claimsId)
		}

		if err != nil {
			logger.WithError(err).Error("encountered unhandled error during authentication")
			return nil, apierror.NewInvalidAuth()
		}

		if authPolicy == nil {
			logger.WithError(err).Error("encountered unhandled nil auth policy during authentication")
			return nil, apierror.NewInvalidAuth()
		}

		if identity == nil {
			logger.WithError(err).Error("encountered unhandled nil identity during authentication")
			return nil, apierror.NewInvalidAuth()
		}

		externalJwtSignerId := ""
		if identity.Disabled {
			logger.
				WithField("disabledAt", identity.DisabledAt).
				WithField("disabledUntil", identity.DisabledUntil).
				Error("authentication failed, identity is disabled")
			return nil, apierror.NewInvalidAuth()
		}

		if !authPolicy.Primary.ExtJwt.Allowed {
			logger.Error("external jwt authentication on auth policy is disabled")
			return nil, apierror.NewInvalidAuth()
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
					return nil, apierror.NewInvalidAuth()
				}
			} else {
				return nil, apierror.NewInvalidAuth()
			}
		} else if authPolicy.Secondary.RequiredExtJwtSigner != nil {
			if extJwt.Id != *authPolicy.Secondary.RequiredExtJwtSigner {
				return nil, apierror.NewInvalidAuth()
			}

			externalJwtSignerId = extJwt.Id
		}

		result := &AuthResultJwt{
			AuthResultBase: AuthResultBase{
				authPolicyId: authPolicy.Id,
				authPolicy:   authPolicy,
				identity:     identity,
				identityId:   identity.Id,
				externalId:   stringz.OrEmpty(identity.ExternalId),
				env:          a.env,
			},
			externalJwtSignerId: externalJwtSignerId,
			externalJwtSigner:   extJwt,
		}

		return result, nil
	}

	logger.Error("authorization failed, jwt did not verify")
	return nil, apierror.NewInvalidAuth()
}

func (a *AuthModuleExtJwt) onExternalSignerCreate(args ...interface{}) {
	signer, ok := args[0].(*persistence.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature create for authentication module %T: expected %T got %T", a, signer, args[0])
		return
	}

	a.addSigner(signer)
}

func (a *AuthModuleExtJwt) onExternalSignerUpdate(args ...interface{}) {
	signer, ok := args[0].(*persistence.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: expected %T got %T", a, signer, args[0])
		return
	}

	//read on update because patches can pass partial data
	err := a.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		signer, err = a.env.GetStores().ExternalJwtSigner.LoadOneById(tx, signer.Id)

		return err
	})

	if err != nil {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: could not read entity: %v", a, err)
	}

	a.addSigner(signer)
}

func (a *AuthModuleExtJwt) addSigner(signer *persistence.ExternalJwtSigner) {
	logger := pfxlog.Logger().WithFields(map[string]interface{}{
		"id":           signer.Id,
		"name":         signer.Name,
		"hasCertPem":   signer.CertPem != nil,
		"jwksEndpoint": signer.JwksEndpoint,
	})

	if signer.Issuer == nil {
		logger.Error("could not add signer, issuer is nil")
		return
	}

	signerRec := &signerRecord{
		externalJwtSigner: signer,
		jwksResolver:      &jwks.HttpResolver{},
		kidToCertificate:  map[string]*x509.Certificate{},
	}

	if err := signerRec.Resolve(false); err != nil {
		logger.WithError(err).Error("could not resolve signer cert/jwks")
	}

	a.signers.Set(*signer.Issuer, signerRec)

}

func (a *AuthModuleExtJwt) onExternalSignerDelete(args ...interface{}) {
	signer, ok := args[0].(*persistence.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: expected %T got %T", a, signer, args[0])
		return
	}

	logger := pfxlog.Logger().WithFields(map[string]interface{}{
		"id":           signer.Id,
		"name":         signer.Name,
		"hasCertPem":   signer.CertPem != nil,
		"jwksEndpoint": signer.JwksEndpoint,
	})

	if signer.Issuer == nil {
		logger.Error("could not add signer, issuer is nil")
		return
	}

	a.signers.Remove(*signer.Issuer)
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
		pfxlog.Logger().Errorf("error loading external jwt signerByIssuer: %v", err)
	}
}
