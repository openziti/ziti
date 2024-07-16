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

package model

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	nfPem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/jwks"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/db"
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
	JwksQueryTimeout    = 1 * time.Second
)

type AuthModuleExtJwt struct {
	env     Env
	method  string
	signers cmap.ConcurrentMap[string, *signerRecord]
}

func NewAuthModuleExtJwt(env Env) *AuthModuleExtJwt {
	ret := &AuthModuleExtJwt{
		env:     env,
		method:  AuthMethodExtJwt,
		signers: cmap.New[*signerRecord](),
	}

	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(ret.addSigner, boltz.EntityCreatedAsync)
	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(ret.onExternalSignerUpdate, boltz.EntityUpdatedAsync)
	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(ret.onExternalSignerDelete, boltz.EntityDeletedAsync)

	ret.loadExistingSigners()

	return ret
}

type pubKey struct {
	pubKey any
	chain  []*x509.Certificate
}

type signerRecord struct {
	sync.Mutex
	jwksLastRequest time.Time

	kidToPubKey       map[string]pubKey
	jwksResponse      *jwks.Response
	externalJwtSigner *db.ExternalJwtSigner

	jwksResolver jwks.Resolver
}

func (r *signerRecord) Resolve(force bool) error {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	if r.externalJwtSigner.CertPem != nil {
		if len(r.kidToPubKey) != 0 && !force {
			return nil
		}

		certs := nfPem.PemStringToCertificates(*r.externalJwtSigner.CertPem)

		if len(certs) == 0 {
			return errors.New("could not add signer, PEM did not parse to any certificates")
		}

		kid := ""

		if r.externalJwtSigner.Kid == nil {
			kid = nfPem.FingerprintFromCertificate(certs[0])
			pfxlog.Logger().WithField("id", r.externalJwtSigner.Id).WithField("name", r.externalJwtSigner.Name).Warnf("external jwt signer does not have a kid, generated: %s", kid)
		} else {
			kid = *r.externalJwtSigner.Kid
		}

		// first cert only
		r.kidToPubKey = map[string]pubKey{
			kid: {
				pubKey: certs[0].PublicKey,
				chain:  certs,
			},
		}

		return nil

	} else if r.externalJwtSigner.JwksEndpoint != nil {
		if len(r.kidToPubKey) != 0 && !force {
			return nil
		}

		if !r.jwksLastRequest.IsZero() && time.Since(r.jwksLastRequest) < JwksQueryTimeout {
			return nil
		}

		r.jwksLastRequest = time.Now()

		jwksResponse, _, err := r.jwksResolver.Get(*r.externalJwtSigner.JwksEndpoint)

		if err != nil {
			return fmt.Errorf("could not resolve jwks endpoint: %v", err)
		}

		for _, key := range jwksResponse.Keys {
			//if we have an x509chain the first must be the signing key
			if len(key.X509Chain) != 0 {
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

				r.kidToPubKey[key.KeyId] = pubKey{
					pubKey: certs[0].PublicKey,
					chain:  certs,
				}
			} else {
				//else the key properties are the only way to construct the public key
				k, err := jwks.KeyToPublicKey(key)

				if err != nil {
					return err
				}

				r.kidToPubKey[key.KeyId] = pubKey{
					pubKey: k,
				}
			}

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
		issuers := a.signers.Keys()
		logger.WithField("knownIssuers", issuers).Error("issuer not found, issuers are bit-for-bit compared, they must match exactly")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("extJwtSignerId", signerRecord.externalJwtSigner.Id).WithField("extJwtSignerName", signerRecord.externalJwtSigner.Name)

	if !signerRecord.externalJwtSigner.Enabled {
		logger.Error("external jwt is disabled")
		return nil, apierror.NewInvalidAuth()
	}

	key, ok := signerRecord.kidToPubKey[kid]

	if !ok {
		if err := signerRecord.Resolve(true); err != nil {
			logger.WithError(err).Error("error attempting to resolve extJwtSigner certificate used for signing")
		}

		key, ok = signerRecord.kidToPubKey[kid]

		if !ok {
			return nil, fmt.Errorf("kid [%s] not found for issuer [%s]", kid, issuer)
		}
	}

	claims[ExtJwtInternalClaim] = signerRecord.externalJwtSigner

	return key.pubKey, nil
}

func (a *AuthModuleExtJwt) Process(context AuthContext) (AuthResult, error) {
	return a.process(context, true)
}

func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (AuthResult, error) {
	return a.process(context, false)
}

type AuthResultJwt struct {
	AuthResultBase
	externalJwtSigner *db.ExternalJwtSigner
}

func (a *AuthResultJwt) IsSuccessful() bool {
	return a.externalJwtSigner != nil && a.AuthResultBase.identityId != ""
}

func (a *AuthResultJwt) AuthenticatorId() string {
	if a.externalJwtSigner == nil {
		return ""
	}

	return "extJwtId:" + a.externalJwtSigner.Id
}

func (a *AuthModuleExtJwt) process(context AuthContext, isPrimary bool) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	jwtStr, err := a.getJwtFromAuthHeader(context)

	if err != nil {
		logger.WithError(err).Error("error attempting to obtain JWT from authentication header")
		return nil, apierror.NewInvalidAuth()
	}

	//pubKeyLookup also handles extJwtSigner.enabled checking
	jwtToken, err := jwt.Parse(jwtStr, a.pubKeyLookup)

	if err != nil {
		logger.WithError(err).Error("authorization failed, jwt did not verify due to error")
		return nil, apierror.NewInvalidAuth()
	}

	if !jwtToken.Valid {
		logger.Error("authorization failed, jwt did not pass signature verification")
		return nil, apierror.NewInvalidAuth()
	}

	mapClaims := jwtToken.Claims.(jwt.MapClaims)
	extJwt := mapClaims[ExtJwtInternalClaim].(*db.ExternalJwtSigner)

	if extJwt == nil {
		logger.Error("no external jwt signer found for internal claims")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("extJwtSignerId", extJwt.Id).
		WithField("issuer", stringz.OrEmpty(extJwt.Issuer)).
		WithField("audience", stringz.OrEmpty(extJwt.Audience))

	err = a.verifyIssuer(extJwt.Issuer, mapClaims)

	if err != nil {
		logger.WithError(err).Error("issuer validation failed")
	}

	err = a.verifyAudience(extJwt.Audience, mapClaims)

	if err != nil {
		logger.WithError(err).Error("audience validation failed")
		return nil, apierror.NewInvalidAuth()
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

	if identity.Disabled {
		logger.
			WithField("disabledAt", identity.DisabledAt).
			WithField("disabledUntil", identity.DisabledUntil).
			Error("authentication failed, identity is disabled")
		return nil, apierror.NewInvalidAuth()
	}

	if isPrimary {
		err := a.verifyAsPrimary(authPolicy, extJwt)

		if err != nil {
			logger.WithError(err).Error("primary external jwt processing failed")
			return nil, apierror.NewInvalidAuth()
		}

	} else {
		if authPolicy.Secondary.RequiredExtJwtSigner == nil {
			logger.Error("secondary external jwt authentication on auth policy is not configured")
			return nil, apierror.NewInvalidAuth()
		}

		if extJwt.Id != *authPolicy.Secondary.RequiredExtJwtSigner {
			logger.WithField("requiredExtJwtId", *authPolicy.Secondary.RequiredExtJwtSigner).
				WithField("tokenExtJwtId", extJwt.Id).
				Error("secondary external jwt authentication failed because the token did not match the required ext-jwt signer")
			return nil, apierror.NewInvalidAuth()
		}
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
		externalJwtSigner: extJwt,
	}

	return result, nil
}

func (a *AuthModuleExtJwt) onExternalSignerCreate(args ...interface{}) {
	signer, ok := args[0].(*db.ExternalJwtSigner)

	if !ok {
		pfxlog.Logger().Errorf("error on external signature create for authentication module %T: expected %T got %T", a, signer, args[0])
		return
	}

	a.addSigner(signer)
}

func (a *AuthModuleExtJwt) onExternalSignerUpdate(signer *db.ExternalJwtSigner) {
	//read on update because patches can pass partial data
	err := a.env.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		signer, _, err = a.env.GetStores().ExternalJwtSigner.FindById(tx, signer.Id)
		return err
	})

	if err != nil {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: could not read entity: %v", a, err)
	}

	a.addSigner(signer)
}

func (a *AuthModuleExtJwt) addSigner(signer *db.ExternalJwtSigner) {
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
		kidToPubKey:       map[string]pubKey{},
	}

	if err := signerRec.Resolve(false); err != nil {
		logger.WithError(err).Error("could not resolve signer cert/jwks")
	}

	a.signers.Set(*signer.Issuer, signerRec)

}

func (a *AuthModuleExtJwt) onExternalSignerDelete(signer *db.ExternalJwtSigner) {
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
	err := a.env.GetDb().View(func(tx *bbolt.Tx) error {
		ids, _, err := a.env.GetStores().ExternalJwtSigner.QueryIds(tx, "")

		if err != nil {
			return err
		}

		for _, id := range ids {
			signer, err := a.env.GetStores().ExternalJwtSigner.LoadById(tx, id)
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

func (a *AuthModuleExtJwt) verifyAudience(expectedAudience *string, mapClaims jwt.MapClaims) error {

	if expectedAudience == nil {
		return errors.New("audience on ext-jwt signer is nil, audience is required")
	}

	audValues := mapClaims["aud"]

	if audValues == nil {
		return errors.New("audience field is missing or is empty in JWT")
	}

	audSlice, ok := audValues.([]string)

	if !ok {
		audISlice, ok := audValues.([]interface{})

		if ok {
			audSlice = []string{}
			for _, aud := range audISlice {
				audSlice = append(audSlice, aud.(string))
			}
		} else {
			audString, ok := audValues.(string)

			if !ok {
				return errors.New("jwt audience field is not a string or an array of strings")
			}

			audSlice = []string{audString}
		}
	}

	for _, validAud := range audSlice {
		if validAud == *expectedAudience {
			return nil
		}
	}

	return errors.New("audience value is invalid, no audiences matched the expected audience")
}

func (a *AuthModuleExtJwt) verifyIssuer(expectedIssuer *string, mapClaims jwt.MapClaims) error {

	if expectedIssuer == nil {
		return errors.New("no issuer found for external jwt signer, issuer is required")
	}

	issuer := ""
	if issuerVal, ok := mapClaims["iss"]; ok {
		issuer, ok = issuerVal.(string)
		if !ok {
			return fmt.Errorf("issuer in claims was not a string, got %T", issuerVal)
		}
	}

	if *expectedIssuer == issuer {
		return nil
	}

	return fmt.Errorf("invalid issuer, got: %s", issuer)
}

func (a *AuthModuleExtJwt) getJwtFromAuthHeader(context AuthContext) (string, error) {
	headers := map[string]interface{}{}

	for key, value := range context.GetHeaders() {
		headers[strings.ToLower(key)] = value
	}

	var authHeaders []string

	if authHeaderVal, ok := headers["authorization"]; ok {
		authHeaders = authHeaderVal.([]string)
	}

	if len(authHeaders) != 1 {
		return "", fmt.Errorf("wrong number of authorization headers found got %d expected 1", len(authHeaders))
	}

	if !strings.HasPrefix(authHeaders[0], "Bearer ") {
		return "", errors.New("invalid authorization header, missing Bearer prefix")
	}

	jwtStr := strings.Replace(authHeaders[0], "Bearer ", "", 1)
	jwtStr = strings.TrimSpace(jwtStr)

	if len(jwtStr) == 0 {
		return "", errors.New("invalid authorization header, jwt after Bearer prefix is empty")
	}

	return jwtStr, nil
}

func (a *AuthModuleExtJwt) verifyAsPrimary(authPolicy *AuthPolicy, extJwt *db.ExternalJwtSigner) error {
	if !authPolicy.Primary.ExtJwt.Allowed {
		return errors.New("primary external jwt authentication on auth policy is disabled")
	}

	if len(authPolicy.Primary.ExtJwt.AllowedExtJwtSigners) == 0 {
		//allow any valid JWT
		return nil
	}

	for _, allowedId := range authPolicy.Primary.ExtJwt.AllowedExtJwtSigners {
		if allowedId == extJwt.Id {
			return nil
		}
	}

	return fmt.Errorf("allowed signers does not contain the external jwt id: %s, expected one of: %v", extJwt.Id, authPolicy.Primary.ExtJwt.AllowedExtJwtSigners)
}
