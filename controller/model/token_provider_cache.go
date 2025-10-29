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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-openapi/jsonpointer"
	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	nfPem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/jwks"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/db"
	cmap "github.com/orcaman/concurrent-map/v2"
	"go.etcd.io/bbolt"
)

// TokenIssuerCache maintains a cache of available JWT token issuers.
// It handles discovery, validation, and caching of public keys for token verification.
// Listens for database events to keep cached state synchronized.
type TokenIssuerCache struct {
	issuers cmap.ConcurrentMap[string, TokenIssuer]
	env     Env
}

// NewTokenIssuerCache creates a new TokenIssuerCache and loads existing issuers from the database.
// Registers listeners for issuer creation, update, and deletion events.
func NewTokenIssuerCache(env Env) *TokenIssuerCache {
	result := &TokenIssuerCache{
		env:     env,
		issuers: cmap.New[TokenIssuer](),
	}

	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(result.onExtJwtCreate, boltz.EntityCreatedAsync)
	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(result.onExtJwtUpdate, boltz.EntityUpdatedAsync)
	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(result.onExtJwtDelete, boltz.EntityDeletedAsync)

	result.loadExisting()

	return result
}

// onExtJwtCreate handles creation of new external JWT signers.
// Creates a TokenIssuerExtJwt, resolves keys from certificate or JWKS endpoint, and caches it.
func (a *TokenIssuerCache) onExtJwtCreate(signer *db.ExternalJwtSigner) {
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

	signerRec := &TokenIssuerExtJwt{
		externalJwtSigner: signer,
		jwksResolver:      &jwks.HttpResolver{},
		kidToPubKey:       map[string]IssuerPublicKey{},
	}

	if err := signerRec.Resolve(false); err != nil {
		logger.WithError(err).Error("could not resolve signer cert/jwks")
	}

	a.issuers.Set(*signer.Issuer, signerRec)

}

// onExtJwtUpdate handles updates to existing external JWT signers.
// Reloads the signer from database and refreshes the cache entry.
func (a *TokenIssuerCache) onExtJwtUpdate(signer *db.ExternalJwtSigner) {
	//read on update because patches can pass partial data
	err := a.env.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		signer, _, err = a.env.GetStores().ExternalJwtSigner.FindById(tx, signer.Id)
		return err
	})

	if err != nil {
		pfxlog.Logger().Errorf("error on external signature update for authentication module %T: could not read entity: %v", a, err)
	}

	a.onExtJwtCreate(signer)
}

// onExtJwtDelete handles deletion of external JWT signers.
// Removes the signer from the cache.
func (a *TokenIssuerCache) onExtJwtDelete(signer *db.ExternalJwtSigner) {
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

	a.issuers.Remove(*signer.Issuer)
}

// loadExisting loads all external JWT signers from the database during initialization.
func (a *TokenIssuerCache) loadExisting() {
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

			a.onExtJwtCreate(signer)
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().Errorf("error loading external jwt signerByIssuer: %v", err)
	}
}

// GetByIssuerString returns the TokenIssuer for the given issuer claim string.
// Returns nil if no issuer is found.
func (a *TokenIssuerCache) GetByIssuerString(issuer string) TokenIssuer {
	tokenIssuer, ok := a.issuers.Get(issuer)

	if !ok {
		return nil
	}

	return tokenIssuer
}

// GetById returns the TokenIssuer with the given ID.
// Returns nil if no issuer is found.
func (a *TokenIssuerCache) GetById(issuerId string) TokenIssuer {
	for _, issuer := range a.issuers.Items() {
		if issuer.Id() == issuerId {
			return issuer
		}
	}

	return nil
}

// GetIssuerStrings returns a list of all known issuer strings.
func (a *TokenIssuerCache) GetIssuerStrings() []string {
	return a.issuers.Keys()
}

// pubKeyLookup is a callback for JWT parsing to resolve the public key for signature verification.
// Locates the token issuer, validates its configuration, and returns the corresponding public key.
func (a *TokenIssuerCache) pubKeyLookup(token *jwt.Token) (interface{}, error) {
	logger := pfxlog.Logger().Entry

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

	if kid == "" {
		logger.Error("kid is empty")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("kid", kid)

	claims := token.Claims.(jwt.MapClaims)
	if claims == nil {
		logger.Error("unknown signer, attempting to look up by claims, but claims were nil")
		return nil, apierror.NewInvalidAuth()
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		logger.WithError(err).Error("unknown signer, attempting to retrieve issuer claim from token failed")
		return nil, apierror.NewInvalidAuth()
	}

	if issuer == "" {
		logger.Error("unknown signer, attempting to look up by issue, but issuer is empty or not a string")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("issuer", issuer)

	tokenIssuer := a.GetByIssuerString(issuer)

	if tokenIssuer == nil {
		issuers := a.GetIssuerStrings()
		logger.WithField("knownIssuers", issuers).Error("issuer not found, issuers are bit-for-bit compared, they must match exactly")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("issuerType", tokenIssuer.TypeName()).WithField("tokenIssuerName", tokenIssuer.Name())

	if !tokenIssuer.IsEnabled() {
		logger.Error("token issuer is disabled")
		return nil, apierror.NewInvalidAuth()
	}

	audiences, err := token.Claims.GetAudience()

	if err != nil {
		logger.WithError(err).Error("could not retrieve audience values from token")
		return nil, apierror.NewInvalidAuth()
	}

	audienceFound := false
	for _, audience := range audiences {
		if audience == tokenIssuer.ExpectedAudience() {
			audienceFound = true
			break
		}
	}

	if !audienceFound {
		logger.Errorf("token audience does not match expected audience, expected %s, got %s", tokenIssuer.ExpectedAudience(), audiences)
		return nil, apierror.NewInvalidAuth()
	}

	_ = tokenIssuer.Resolve(false)
	key, ok := tokenIssuer.PubKeyByKid(kid)

	if !ok {
		if err := tokenIssuer.Resolve(true); err != nil {
			logger.WithError(err).Error("error attempting to resolve extJwtSigner certificate used for signing")
		}

		key, ok = tokenIssuer.PubKeyByKid(kid)

		if !ok {
			return nil, fmt.Errorf("kid [%s] not found for issuer [%s]", kid, issuer)
		}
	}

	claims[InternalTokenIssuerClaim] = tokenIssuer

	return key.PubKey, nil
}

// VerifyTokenByInspection verifies a JWT by examining its issuer claim to locate the appropriate issuer.
// Parses the token and validates signature, audience, and extracts identity/name/attribute claims.
// Returns the verification result containing extracted claims and the verifying issuer.
func (a *TokenIssuerCache) VerifyTokenByInspection(candidateToken string) (*TokenVerificationResult, error) {

	claims := jwt.MapClaims{}

	token, err := jwt.ParseWithClaims(candidateToken, claims, a.pubKeyLookup)

	if err != nil {
		return nil, fmt.Errorf("could not parse token: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("token is not valid")
	}

	tokenIssuer := claims[InternalTokenIssuerClaim].(TokenIssuer)

	if tokenIssuer == nil {
		return nil, errors.New("token issuer is nil")
	}

	if !tokenIssuer.IsEnabled() {
		return nil, errors.New("token issuer is disabled")
	}

	idClaimValue, err := resolveStringClaimSelector(claims, tokenIssuer.IdentityIdClaimsSelector())

	if err != nil {
		return nil, fmt.Errorf("could not resolve identity claim property %s: %w", tokenIssuer.IdentityIdClaimsSelector(), err)
	}

	attributeSelector := tokenIssuer.EnrollmentAttributeClaimsSelector()
	var attributeClaimValues []string
	if attributeSelector != "" {
		attributeClaimValues, err = resolveStringSliceClaimProperty(claims, attributeSelector)

		if err != nil {
			return nil, fmt.Errorf("could not resolve attribute claim property %s: %w", tokenIssuer.EnrollmentAttributeClaimsSelector(), err)
		}
	}

	if attributeClaimValues == nil {
		attributeClaimValues = []string{}
	}

	nameSelector := tokenIssuer.EnrollmentNameClaimSelector()
	nameClaimValue := ""

	if nameSelector != "" {
		nameClaimValue, err = resolveStringClaimSelector(claims, nameSelector)

		if err != nil {
			return nil, fmt.Errorf("could not resolve name claim property %s: %w", tokenIssuer.EnrollmentNameClaimSelector(), err)
		}
	}

	return &TokenVerificationResult{
		TokenIssuer:            tokenIssuer,
		Token:                  token,
		Claims:                 claims,
		IdClaimSelector:        tokenIssuer.IdentityIdClaimsSelector(),
		IdClaimValue:           idClaimValue,
		AttributeClaimSelector: tokenIssuer.EnrollmentAttributeClaimsSelector(),
		AttributeClaimValue:    attributeClaimValues,
		NameClaimSelector:      tokenIssuer.EnrollmentNameClaimSelector(),
		NameClaimValue:         nameClaimValue,
	}, nil
}

// TokenVerificationResult contains the result of JWT token verification.
// Includes the parsed token, extracted claims, and the issuer that verified it.
type TokenVerificationResult struct {
	TokenIssuer            TokenIssuer
	Token                  *jwt.Token
	Claims                 map[string]any
	IdClaimSelector        string
	IdClaimValue           string
	AttributeClaimSelector string
	AttributeClaimValue    []string
	NameClaimSelector      string
	NameClaimValue         string
}

// IsValid returns true if the JWT token signature is valid.
func (r *TokenVerificationResult) IsValid() bool {
	return r.Token.Valid
}

// TokenIssuer represents a JWT token issuer capable of verifying tokens.
// Implementations provide token verification, configuration queries, and claim extraction.
type TokenIssuer interface {
	// Id returns the unique identifier of this token issuer.
	Id() string
	// TypeName returns the type name of the token issuer (e.g., "externalJwtSigner").
	TypeName() string
	// Name returns the human-readable name of the token issuer.
	Name() string
	// IsEnabled returns true if this token issuer is enabled.
	IsEnabled() bool

	// PubKeyByKid returns the public key for the given key ID.
	PubKeyByKid(kid string) (IssuerPublicKey, bool)
	// Resolve loads the issuer's public keys from certificate or JWKS endpoint.
	// If force is true, refreshes cached keys even if already loaded.
	Resolve(force bool) error

	// ExpectedIssuer returns the issuer claim value that tokens from this issuer should contain.
	ExpectedIssuer() string
	// ExpectedAudience returns the audience claim value that tokens should contain.
	ExpectedAudience() string

	// AuthenticatorId returns the authenticator ID for this issuer.
	AuthenticatorId() string

	// EnrollmentAuthPolicyId returns the auth policy ID to apply to identities enrolled via this issuer.
	EnrollmentAuthPolicyId() string
	// EnrollmentAttributeClaimsSelector returns the JSON pointer path to the attributes claim.
	EnrollmentAttributeClaimsSelector() string
	// EnrollmentNameClaimSelector returns the JSON pointer path to the identity name claim.
	EnrollmentNameClaimSelector() string

	// IdentityIdClaimsSelector returns the JSON pointer path to the identity ID claim.
	IdentityIdClaimsSelector() string

	// UseExternalId returns true if the identity ID should be stored as an external ID.
	UseExternalId() bool

	// VerifyToken verifies and parses a JWT token, returning the verification result.
	VerifyToken(token string) (*TokenVerificationResult, error)

	// EnrollToCertEnabled returns true if enrollment to certificate is allowed.
	EnrollToCertEnabled() bool
	// EnrollToTokenEnabled returns true if enrollment to token is allowed.
	EnrollToTokenEnabled() bool
}

var _ TokenIssuer = (*TokenIssuerExtJwt)(nil)

// TokenIssuerExtJwt is a TokenIssuer implementation for external JWT signers.
// Supports verification via certificate PEM or JWKS endpoints.
type TokenIssuerExtJwt struct {
	sync.Mutex
	jwksLastRequest time.Time

	kidToPubKey       map[string]IssuerPublicKey
	jwksResponse      *jwks.Response
	externalJwtSigner *db.ExternalJwtSigner

	jwksResolver jwks.Resolver
}

// EnrollToCertEnabled returns true if this issuer allows enrollment to certificate.
func (r *TokenIssuerExtJwt) EnrollToCertEnabled() bool {
	return r.externalJwtSigner.EnrollToCertEnabled
}

// EnrollToTokenEnabled returns true if this issuer allows enrollment to token authentication.
func (r *TokenIssuerExtJwt) EnrollToTokenEnabled() bool {
	return r.externalJwtSigner.EnrollToTokenEnabled
}

// EnrollmentAttributeClaimsSelector returns the JSON pointer path to the attributes claim.
func (r *TokenIssuerExtJwt) EnrollmentAttributeClaimsSelector() string {
	return r.externalJwtSigner.EnrollAttributeClaimsSelector
}

// EnrollmentNameClaimSelector returns the JSON pointer path to the identity name claim.
func (r *TokenIssuerExtJwt) EnrollmentNameClaimSelector() string {
	return r.externalJwtSigner.EnrollNameClaimSelector
}

// EnrollmentAuthPolicyId returns the auth policy ID to apply to enrolled identities.
func (r *TokenIssuerExtJwt) EnrollmentAuthPolicyId() string {
	return r.externalJwtSigner.EnrollAuthPolicyId
}

// VerifyToken verifies a JWT using this issuer's configuration.
// Attempts resolution of keys if not already cached before verification.
func (r *TokenIssuerExtJwt) VerifyToken(token string) (*TokenVerificationResult, error) {
	err := r.Resolve(false)

	pfxlog.Logger().WithError(err).Warn("error during routine resolve of external jwt signer cert/jwks, attempting to verify the token with any cached keys")

	claims := jwt.MapClaims{}
	resultToken, err := jwt.ParseWithClaims(token, claims, r.keyFunc)

	if err != nil {
		return nil, err
	}

	if !resultToken.Valid {
		return nil, errors.New("token is not valid")
	}

	idClaimValue, err := resolveStringClaimSelector(claims, r.IdentityIdClaimsSelector())

	if err != nil {
		return nil, fmt.Errorf("could not resolve id claim property: %w", err)
	}

	return &TokenVerificationResult{
		TokenIssuer:     r,
		Token:           resultToken,
		Claims:          claims,
		IdClaimSelector: r.IdentityIdClaimsSelector(),
		IdClaimValue:    idClaimValue,
	}, nil
}

// resolveStringSliceClaimProperty extracts a string or string array from JWT claims using a JSON pointer.
// Returns a string slice even if the claim is a single string value.
func resolveStringSliceClaimProperty(claims jwt.MapClaims, property string) ([]string, error) {
	if property == "" {
		return nil, nil
	}

	if !strings.HasPrefix(property, "/") {
		property = "/" + property
	}

	jsonPointer, err := jsonpointer.New(property)

	if err != nil {
		return nil, fmt.Errorf("could not parse json pointer: %s: %w", property, err)
	}

	val, _, err := jsonPointer.Get(claims)

	if err != nil {
		return nil, fmt.Errorf("could not resolve json pointer: %s: %w", property, err)
	}

	strVal, ok := val.(string)

	if ok {
		if strVal != "" {
			return []string{strVal}, nil
		}
		return nil, nil
	}

	arrVals, ok := val.([]any)

	if !ok {
		return nil, fmt.Errorf("could not resolve json pointer: %s: value is not a string or an array of strings, got: %v", property, arrVals)
	}

	var attributes []string
	for i, arrVal := range arrVals {
		arrStrVal, ok := arrVal.(string)

		if !ok {
			return nil, fmt.Errorf("could not resolve json point %s as array of strings at index %d: %v: value is not a string", property, i, arrVal)
		}

		if arrStrVal != "" {
			attributes = append(attributes, arrStrVal)
		}
	}

	return attributes, nil
}

// resolveStringClaimSelector extracts a string value from JWT claims using a JSON pointer.
func resolveStringClaimSelector(claims jwt.MapClaims, property string) (string, error) {
	if property == "" {
		return "", nil
	}

	if !strings.HasPrefix(property, "/") {
		property = "/" + property
	}

	jsonPointer, err := jsonpointer.New(property)

	if err != nil {
		return "", fmt.Errorf("could not parse json pointer: %s: %w", property, err)
	}

	val, _, err := jsonPointer.Get(claims)

	if err != nil {
		return "", fmt.Errorf("could not resolve json pointer: %s: %w", property, err)
	}

	strVal, ok := val.(string)

	if !ok {
		return "", fmt.Errorf("could not resolve json pointer: %s: value is not a string", property)
	}

	return strVal, nil
}

// UseExternalId returns true if the identity ID should be stored as an external ID.
func (r *TokenIssuerExtJwt) UseExternalId() bool {
	return r.externalJwtSigner.UseExternalId
}

// IdentityIdClaimsSelector returns the JSON pointer path to the identity ID claim.
// Defaults to the standard identity ID claims selector if not configured.
func (r *TokenIssuerExtJwt) IdentityIdClaimsSelector() string {
	ret := stringz.OrEmpty(r.externalJwtSigner.IdentityIdClaimsSelector)

	if ret == "" {
		return db.DefaultIdentityIdClaimsSelector
	}

	return ret
}

// AuthenticatorId returns an authenticator ID specific to this token issuer.
func (r *TokenIssuerExtJwt) AuthenticatorId() string {
	return "extJwtId:" + r.externalJwtSigner.Id
}

// IsEnabled returns true if this token issuer is enabled.
func (r *TokenIssuerExtJwt) IsEnabled() bool {
	return r.externalJwtSigner.Enabled
}

// Name returns the human-readable name of this token issuer.
func (r *TokenIssuerExtJwt) Name() string {
	return r.externalJwtSigner.Name
}

// ExpectedIssuer returns the issuer claim value that tokens should contain.
func (r *TokenIssuerExtJwt) ExpectedIssuer() string {
	return stringz.OrEmpty(r.externalJwtSigner.Issuer)
}

// ExpectedAudience returns the audience claim value that tokens should contain.
func (r *TokenIssuerExtJwt) ExpectedAudience() string {
	return stringz.OrEmpty(r.externalJwtSigner.Audience)
}

// TypeName returns the type name of this token issuer.
func (r *TokenIssuerExtJwt) TypeName() string {
	return "externalJwtSigner"
}

// Id returns the unique identifier of this token issuer.
func (r *TokenIssuerExtJwt) Id() string {
	return r.externalJwtSigner.Id
}

// keyFunc is a JWT parsing callback that resolves the public key for signature verification.
func (r *TokenIssuerExtJwt) keyFunc(token *jwt.Token) (any, error) {
	kid, err := getJwtTokenKid(token)

	if err != nil {
		return nil, fmt.Errorf("could not determine public key to use: %w", err)
	}

	if kid == "" {
		return nil, fmt.Errorf("could not determine public key to use: kid is empty")
	}

	issuerPublicKey, ok := r.PubKeyByKid(kid)

	if !ok {
		return nil, fmt.Errorf("no public key found for kid %s", kid)
	}

	return issuerPublicKey.PubKey, nil
}

// PubKeyByKid returns the public key for the given key ID from the cache.
// Thread-safe using internal mutex.
func (r *TokenIssuerExtJwt) PubKeyByKid(kid string) (IssuerPublicKey, bool) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	key, ok := r.kidToPubKey[kid]

	return key, ok
}

// Resolve loads public keys from either the configured certificate PEM or JWKS endpoint.
// If force is true, refreshes keys even if already cached.
// Handles timeout caching for JWKS endpoint queries to avoid excessive network calls.
func (r *TokenIssuerExtJwt) Resolve(force bool) error {
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
		r.kidToPubKey = map[string]IssuerPublicKey{
			kid: {
				PubKey: certs[0].PublicKey,
				Chain:  certs,
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

				r.kidToPubKey[key.KeyId] = IssuerPublicKey{
					PubKey: certs[0].PublicKey,
					Chain:  certs,
				}
			} else {
				//else the key properties are the only way to construct the public key
				k, err := jwks.KeyToPublicKey(key)

				if err != nil {
					return err
				}

				r.kidToPubKey[key.KeyId] = IssuerPublicKey{
					PubKey: k,
				}
			}

		}

		r.jwksResponse = jwksResponse

		return nil
	}

	return errors.New("instructed to add external jwt signer that does not have a certificate PEM or JWKS endpoint")
}

// IssuerPublicKey represents a public key and associated certificate chain.
type IssuerPublicKey struct {
	PubKey any                 // The public key used for signature verification
	Chain  []*x509.Certificate // Optional X.509 certificate chain
}

// getJwtTokenKid extracts the key ID (kid) from a JWT token header.
func getJwtTokenKid(token *jwt.Token) (string, error) {
	if token.Header == nil {
		return "", errors.New("token header is nil")
	}

	kidVal, ok := token.Header["kid"]

	if !ok || kidVal == nil {
		return "", errors.New("token header kid value is nil or missing")
	}

	kid, ok := kidVal.(string)

	if !ok || kid == "" {
		return "", errors.New("token header kid value is empty or not a string")
	}

	return kid, nil

}
