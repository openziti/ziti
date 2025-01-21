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
	"github.com/openziti/ziti/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"strings"
	"sync"
	"time"
)

var _ AuthProcessor = &AuthModuleExtJwt{}

const (
	AuthMethodExtJwt          = "ext-jwt"
	ExtJwtInternalClaim       = "-internal-ext-jwt"
	JwksQueryTimeout          = 1 * time.Second
	MaxCandidateJwtProcessing = 2
)

type candidateResult struct {
	Error error

	ExpectedIssuer          string
	EncounteredIssuer       string
	ExpectedAudience        string
	EncounteredAudiences    []string
	EncounteredExtJwtSigner *db.ExternalJwtSigner
	ExpectedExtJwtSignerIds []string

	AuthPolicy *AuthPolicy
	Identity   *Identity

	IdClaimProperty string
	IdClaimsValue   string
}

func (r *candidateResult) LogResult(logger *logrus.Entry, index int) {
	if r.AuthPolicy != nil {
		logger = logger.WithField("authPolicyId", r.AuthPolicy.Id)
	}

	if r.Identity != nil {
		logger = logger.WithField("identityId", r.Identity.Id)
	}

	if r.EncounteredExtJwtSigner != nil {
		logger = logger.WithField("extJwtSignerId", r.EncounteredExtJwtSigner.Id)
	}

	logger = logger.WithField("issuer", r.ExpectedIssuer)
	logger = logger.WithField("audience", r.ExpectedAudience)

	if r.Error == nil {
		logger.Debugf("validated candidate JWT at index %d", index)
	} else {
		logger.WithError(r.Error).Errorf("failed to validate candidate JWT at index %d", index)
	}

}

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

func (r *signerRecord) PubKeyByKid(kid string) (pubKey, bool) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	key, ok := r.kidToPubKey[kid]

	return key, ok
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
		if (!r.jwksLastRequest.IsZero() && time.Since(r.jwksLastRequest) < JwksQueryTimeout) && !force {
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
				// x5c is the only attribute with padding according to
				// RFC 7517 Section-4.7 "x5c" (X.509 Certificate Chain) Parameter
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

	key, ok := signerRecord.PubKeyByKid(kid)

	if !ok {
		if err := signerRecord.Resolve(false); err != nil {
			logger.WithError(err).Error("error attempting to resolve extJwtSigner certificate used for signing")
		}

		key, ok = signerRecord.PubKeyByKid(kid)

		if !ok {
			return nil, fmt.Errorf("kid [%s] not found for issuer [%s]", kid, issuer)
		}
	}

	claims[ExtJwtInternalClaim] = signerRecord.externalJwtSigner

	return key.pubKey, nil
}

func (a *AuthModuleExtJwt) Process(context AuthContext) (AuthResult, error) {
	return a.process(context)
}

func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (AuthResult, error) {
	return a.process(context)
}

type AuthResultJwt struct {
	AuthResultBase
	externalJwtSigner *db.ExternalJwtSigner
}

func (a *AuthResultJwt) IsSuccessful() bool {
	return a.externalJwtSigner != nil && a.Identity() != nil
}

func (a *AuthResultJwt) AuthenticatorId() string {
	if a.externalJwtSigner == nil {
		return ""
	}

	return "extJwtId:" + a.externalJwtSigner.Id
}

func (a *AuthResultJwt) Authenticator() *Authenticator {
	authenticator := &Authenticator{
		BaseEntity: models.BaseEntity{
			Id:        AuthMethodExtJwt,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			IsSystem:  true,
		},
		Method: AuthMethodExtJwt,
	}

	if a.identity != nil {
		authenticator.IdentityId = a.identity.Id
	}

	return authenticator
}

// process attempts to locate a JWT and ExtJwtSigner that  verifies it. If context.GetPrimaryIdentity()==nil, this will be
// processed as a secondary authentication factor.
func (a *AuthModuleExtJwt) process(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	isPrimary := context.GetPrimaryIdentity() == nil

	authType := "secondary"
	if isPrimary {
		authType = "primary"
	}

	candidates := a.getJwtsFromAuthHeader(context)

	var verifyResults []*candidateResult

	if len(candidates) == 0 {
		logger.Error("encountered 0 candidate JWTs, verification cannot occur")
		return nil, apierror.NewInvalidAuth()
	}

	for i, candidate := range candidates {
		verifyResult := a.verifyCandidate(context, isPrimary, candidate)

		if verifyResult.Error == nil {
			//success
			result := &AuthResultJwt{
				AuthResultBase: AuthResultBase{
					authPolicy: verifyResult.AuthPolicy,
					identity:   verifyResult.Identity,
					env:        a.env,
				},
				externalJwtSigner: verifyResult.EncounteredExtJwtSigner,
			}

			verifyResult.LogResult(logger, i)

			return result, nil
		} else {
			verifyResults = append(verifyResults, verifyResult)
		}
	}

	logger.Errorf("encountered %d candidate JWTs and all failed to validate for %s authentication, see the following log messages", len(verifyResults), authType)
	for i, result := range verifyResults {
		result.LogResult(logger, i)
	}

	return nil, apierror.NewInvalidAuth()
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

func (a *AuthModuleExtJwt) getJwtsFromAuthHeader(context AuthContext) []string {
	headers := map[string]interface{}{}

	for key, value := range context.GetHeaders() {
		headers[strings.ToLower(key)] = value
	}

	var authHeaders []string

	if authHeaderVal, ok := headers["authorization"]; ok {
		authHeaders = authHeaderVal.([]string)
	}

	var candidates []string
	for _, authHeader := range authHeaders {
		if strings.HasPrefix(authHeader, "Bearer ") {
			jwtStr := strings.Replace(authHeader, "Bearer ", "", 1)
			jwtStr = strings.TrimSpace(jwtStr)

			if jwtStr != "" {
				candidates = append(candidates, jwtStr)
			}
		}
	}

	if len(candidates) > MaxCandidateJwtProcessing {
		candidates = candidates[:MaxCandidateJwtProcessing]
	}

	return candidates
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

func (a *AuthModuleExtJwt) verifyCandidate(context AuthContext, isPrimary bool, jwtStr string) *candidateResult {
	result := &candidateResult{}

	targetIdentity := context.GetPrimaryIdentity()

	//pubKeyLookup also handles extJwtSigner.enabled checking
	jwtToken, err := jwt.Parse(jwtStr, a.pubKeyLookup)

	if err != nil {
		result.Error = fmt.Errorf("jwt failed to parse: %w", err)
		return result
	}

	if !jwtToken.Valid {
		result.Error = errors.New("authorization failed, jwt did not pass signature verification")
		return result
	}

	mapClaims := jwtToken.Claims.(jwt.MapClaims)
	extJwt := mapClaims[ExtJwtInternalClaim].(*db.ExternalJwtSigner)

	if extJwt == nil {
		result.Error = errors.New("no external jwt signer found for internal claims")
		return result
	}

	result.EncounteredExtJwtSigner = extJwt
	result.ExpectedIssuer = stringz.OrEmpty(extJwt.Issuer)
	result.ExpectedAudience = stringz.OrEmpty(extJwt.Audience)
	result.EncounteredIssuer, _ = mapClaims.GetIssuer()
	result.EncounteredAudiences, _ = mapClaims.GetAudience()

	err = a.verifyIssuer(extJwt.Issuer, mapClaims)

	if err != nil {
		result.Error = fmt.Errorf("issuer validation failed: %w", err)
		return result
	}

	err = a.verifyAudience(extJwt.Audience, mapClaims)

	if err != nil {
		result.Error = fmt.Errorf("audience validation failed: %w", err)
		return result
	}

	idClaimProperty := "sub"
	if extJwt.ClaimsProperty != nil {
		idClaimProperty = *extJwt.ClaimsProperty
	}

	result.IdClaimProperty = idClaimProperty

	identityIdInterface, ok := mapClaims[idClaimProperty]

	if !ok {
		result.Error = fmt.Errorf("claims property [%s] was not found in the claims", idClaimProperty)
		return result
	}

	claimId, ok := identityIdInterface.(string)

	if !ok || claimId == "" {
		result.Error = fmt.Errorf("claims property [%s] was not a string or was empty: %v", idClaimProperty, identityIdInterface)
		return result
	}

	result.IdClaimsValue = claimId

	var authPolicy *AuthPolicy

	var identity *Identity
	claimIdLookupMethod := ""
	if extJwt.UseExternalId {
		claimIdLookupMethod = "external id"
		authPolicy, identity, err = getAuthPolicyByExternalId(a.env, AuthMethodExtJwt, "", claimId)
	} else {
		claimIdLookupMethod = "identity id"
		authPolicy, identity, err = getAuthPolicyByIdentityId(a.env, AuthMethodExtJwt, "", claimId)
	}

	if err != nil {
		result.Error = fmt.Errorf("error during authentication policy and identity lookup by claims type [%s] and claimd id [%s]: %w", claimIdLookupMethod, claimId, err)
		return result
	}

	if authPolicy == nil {
		result.Error = fmt.Errorf("no authentication policy found for claims type [%s] and claimd id [%s]: %w", claimIdLookupMethod, claimId, err)
		return result
	}

	result.AuthPolicy = authPolicy

	if identity == nil {
		result.Error = fmt.Errorf("no identity found for claims type [%s] and claimd id [%s]: %w", claimIdLookupMethod, claimId, err)
		return result
	}

	if targetIdentity != nil && targetIdentity.Id != identity.Id {
		result.Error = fmt.Errorf("jwt mapped to identity [%s - %s], which does not match the current sessions identity [%s - %s]", identity.Id, identity.Name, targetIdentity.Id, targetIdentity.Name)
		return result
	}

	result.Identity = identity

	if identity.Disabled {
		result.Error = fmt.Errorf("the identity [%s] is disabled, disabledAt %v, disabledUntil: %v", identity.Id, identity.DisabledAt, identity.DisabledUntil)
		return result
	}

	if isPrimary {
		err = a.verifyAsPrimary(authPolicy, extJwt)

		if len(authPolicy.Primary.ExtJwt.AllowedExtJwtSigners) == 0 {
			result.ExpectedExtJwtSignerIds = []string{"<any>"}
		} else {
			result.ExpectedExtJwtSignerIds = authPolicy.Primary.ExtJwt.AllowedExtJwtSigners
		}

		if err != nil {
			result.Error = fmt.Errorf("primary external jwt processing failed on authentication policy [%s]: %w", authPolicy.Id, err)
			return result
		}

	} else {
		if authPolicy.Secondary.RequiredExtJwtSigner == nil {
			result.Error = fmt.Errorf("secondary external jwt authentication on authentication policy [%s] is not configured", authPolicy.Id)
			return result
		}

		result.ExpectedExtJwtSignerIds = []string{*authPolicy.Secondary.RequiredExtJwtSigner}

		if extJwt.Id != *authPolicy.Secondary.RequiredExtJwtSigner {
			result.Error = fmt.Errorf("secondary external jwt authentication failed on authentication policy [%s]: the required ext-jwt signer [%s] did not match the validating id [%s]", authPolicy.Id, *authPolicy.Secondary.RequiredExtJwtSigner, extJwt.Id)
			return result
		}
	}

	return result
}
