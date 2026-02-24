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
	"crypto/sha1"
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
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/db"
	cmap "github.com/orcaman/concurrent-map/v2"
	"go.etcd.io/bbolt"
)

var _ common.TokenIssuerCache = (*TokenIssuerCache)(nil)

// TokenIssuerCache maintains a cache of available JWT token issuers.
// It handles discovery, validation, and caching of public keys for token verification.
// Listens for database events to keep cached state synchronized.
type TokenIssuerCache struct {
	// extJwt.Issuer -> extJwt issuer
	externalIssuers cmap.ConcurrentMap[string, common.TokenIssuer]

	// controller.Id -> controller issuer, controllers are stored by id
	// as we are guarantee they have a KID and issuers may be variable
	// due to xweb API address binds
	controllerIssuers cmap.ConcurrentMap[string, common.TokenIssuer]

	env Env
}

// NewTokenIssuerCache creates a new TokenIssuerCache and loads existing issuers from the database.
// Registers listeners for issuer creation, update, and deletion events.
func NewTokenIssuerCache(env Env) *TokenIssuerCache {
	result := &TokenIssuerCache{
		env:               env,
		externalIssuers:   cmap.New[common.TokenIssuer](),
		controllerIssuers: cmap.New[common.TokenIssuer](),
	}

	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(result.onExtJwtCreate, boltz.EntityCreatedAsync)
	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(result.onExtJwtUpdate, boltz.EntityUpdatedAsync)
	env.GetStores().ExternalJwtSigner.AddEntityEventListenerF(result.onExtJwtDelete, boltz.EntityDeletedAsync)

	env.GetStores().Controller.AddEntityEventListenerF(result.onControllerCreate, boltz.EntityCreatedAsync)
	env.GetStores().Controller.AddEntityEventListenerF(result.onControllerUpdate, boltz.EntityUpdatedAsync)
	env.GetStores().Controller.AddEntityEventListenerF(result.onControllerDelete, boltz.EntityDeletedAsync)

	result.loadExisting()

	return result
}

func (a *TokenIssuerCache) onControllerCreate(controller *db.Controller) {
	logger := pfxlog.Logger().WithFields(map[string]interface{}{
		"id":   controller.Id,
		"name": controller.Name,
	})

	certs := nfPem.PemStringToCertificates(controller.CertPem)

	if len(certs) == 0 {
		logger.Error("could not add controller token issuer, no certificates found")
		return
	}

	cert := certs[0]

	controllerTokenIssuer := &ControllerTokenIssuer{
		controllerId:   controller.Id,
		controllerName: controller.Name,
		pubKey: common.IssuerPublicKey{
			PubKey: cert.PublicKey,
			Chain:  certs},
		kid:              fmt.Sprintf("%x", sha1.Sum(cert.Raw)),
		controllerIssuer: "",
	}
	a.controllerIssuers.Set(controller.Id, controllerTokenIssuer)
}

func (a *TokenIssuerCache) onControllerUpdate(controller *db.Controller) {
	//read on update because patches can pass partial data
	err := a.env.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		controller, _, err = a.env.GetStores().Controller.FindById(tx, controller.Id)
		return err
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error on controller update, could not update controller token issuer")
		return
	}

	a.onControllerCreate(controller)
}

func (a *TokenIssuerCache) onControllerDelete(controller *db.Controller) {
	a.controllerIssuers.Remove(controller.Id)
}

// addControllerIssuer creates a ControllerTokenIssuer from a model Controller and adds it to the cache.
func (a *TokenIssuerCache) addControllerIssuer(controller *Controller) {
	logger := pfxlog.Logger().WithFields(map[string]interface{}{
		"id":   controller.Id,
		"name": controller.Name,
	})

	certs := nfPem.PemStringToCertificates(controller.CertPem)

	if len(certs) == 0 {
		logger.Error("could not add controller token issuer, no certificates found")
		return
	}

	cert := certs[0]

	controllerTokenIssuer := &ControllerTokenIssuer{
		controllerId:   controller.Id,
		controllerName: controller.Name,
		pubKey: common.IssuerPublicKey{
			PubKey: cert.PublicKey,
			Chain:  certs,
		},
		kid:              fmt.Sprintf("%x", sha1.Sum(cert.Raw)),
		controllerIssuer: "",
	}
	a.controllerIssuers.Set(controller.Id, controllerTokenIssuer)
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
		logger.Error("could not add ext jwt token issuer, issuer is nil")
		return
	}

	signerRec := &TokenIssuerExtJwt{
		externalJwtSigner: signer,
		jwksResolver:      &jwks.HttpResolver{},
		kidToPubKey:       map[string]common.IssuerPublicKey{},
	}

	if err := signerRec.Resolve(false); err != nil {
		logger.WithError(err).Error("could not resolve ext jwt token issuer cert/jwks")
	}

	a.externalIssuers.Set(*signer.Issuer, signerRec)

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
		pfxlog.Logger().WithError(err).Error("error on external signature update, could not update ext jwt token issuer")
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
		logger.Error("could not remove ext jwt token issuer, issuer is nil")
		return
	}

	a.externalIssuers.Remove(*signer.Issuer)
}

// loadExisting loads all external JWT signers and controllers during initialization.
func (a *TokenIssuerCache) loadExisting() {
	err := a.env.GetDb().View(func(tx *bbolt.Tx) error {
		extJwtIds, _, err := a.env.GetStores().ExternalJwtSigner.QueryIds(tx, "")

		if err != nil {
			return err
		}

		for _, id := range extJwtIds {
			signer, err := a.env.GetStores().ExternalJwtSigner.LoadById(tx, id)
			if err != nil {
				pfxlog.Logger().WithError(err).WithField("extJwtId", id).Error("error loading external jwt as token issuer")
				continue
			}

			a.onExtJwtCreate(signer)
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error loading existing external jwt token issuers")
	}

	controllers, err := a.env.GetManagers().Controller.ReadAll()
	if err != nil {
		pfxlog.Logger().WithError(err).Error("error loading controllers as token issuers")
		return
	}

	for _, controller := range controllers {
		a.addControllerIssuer(controller)
	}
}

// GetByIssuerString returns the TokenIssuer for the given issuer claim string.
// Returns nil if no issuer is found.
func (a *TokenIssuerCache) GetByIssuerString(issuer string) common.TokenIssuer {
	tokenIssuer, ok := a.externalIssuers.Get(issuer)

	if !ok {
		return nil
	}

	return tokenIssuer
}

// GetById returns the TokenIssuer with the given ID.
// Returns nil if no issuer is found.
func (a *TokenIssuerCache) GetById(issuerId string) common.TokenIssuer {
	for _, issuer := range a.externalIssuers.Items() {
		if issuer.Id() == issuerId {
			return issuer
		}
	}

	return nil
}

// GetIssuerStrings returns a list of all known issuer strings.
func (a *TokenIssuerCache) GetIssuerStrings() []string {
	return a.externalIssuers.Keys()
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

		tokenIssuer = a.GetIssuerByKid(kid)

		if tokenIssuer == nil {
			issuers := a.GetIssuerStrings()
			kids := a.GetKids()
			logger.WithField("knownIssuers", issuers).WithField("kids", kids).Error("issuer not found, kid not found, issuers and kids are compared and must match")
			return nil, apierror.NewInvalidAuth()
		}

		if !tokenIssuer.IsControllerTokenIssuer() {
			logger.Error("kid matched an external jwt signer but issuer string did not match, rejecting to prevent cross-issuer token acceptance")
			return nil, apierror.NewInvalidAuth()
		}
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
func (a *TokenIssuerCache) VerifyTokenByInspection(candidateToken string) (*common.TokenVerificationResult, common.TokenIssuer) {

	claims := jwt.MapClaims{}

	token, err := jwt.ParseWithClaims(candidateToken, claims, a.pubKeyLookup)

	result := &common.TokenVerificationResult{
		Token:  token,
		Claims: claims,
		Error:  err,
	}

	if !result.IsValid() {
		return result, nil
	}

	tokenIssuer := claims[InternalTokenIssuerClaim].(common.TokenIssuer)

	if tokenIssuer == nil {
		result.Error = errors.New("token issuer not found inside of parsed claims")
		return result, nil
	}

	if !tokenIssuer.IsEnabled() {
		result.Error = errors.New("token issuer is disabled")
		return result, tokenIssuer
	}

	result.IdClaimValue, err = resolveStringClaimSelector(claims, tokenIssuer.IdentityIdClaimsSelector())

	if err != nil {
		result.Error = fmt.Errorf("could not resolve identity claim property %s: %w", tokenIssuer.IdentityIdClaimsSelector(), err)
		return result, tokenIssuer
	}

	result.AttributeClaimSelector = tokenIssuer.EnrollmentAttributeClaimsSelector()

	if result.AttributeClaimSelector != "" {
		result.AttributeClaimValue, err = resolveStringSliceClaimProperty(claims, result.AttributeClaimSelector)

		if err != nil {
			result.Error = fmt.Errorf("could not resolve attribute claim property %s: %w", tokenIssuer.EnrollmentAttributeClaimsSelector(), err)
			return result, tokenIssuer
		}
	}

	if result.AttributeClaimValue == nil {
		result.AttributeClaimValue = []string{}
	}

	result.NameClaimSelector = tokenIssuer.EnrollmentNameClaimSelector()

	if result.NameClaimSelector != "" {
		result.NameClaimValue, err = resolveStringClaimSelector(claims, result.NameClaimSelector)

		if err != nil {
			result.Error = fmt.Errorf("could not resolve name claim property %s: %w", tokenIssuer.EnrollmentNameClaimSelector(), err)
			return result, tokenIssuer
		}
	}

	return result, tokenIssuer
}

// IterateExternalIssuers calls f for each registered external JWT issuer in an unspecified order.
// Iteration stops early when f returns false. Controller issuers are not included.
func (a *TokenIssuerCache) IterateExternalIssuers(f func(issuer common.TokenIssuer) bool) {
	keepGoing := true
	a.externalIssuers.IterCb(func(key string, v common.TokenIssuer) {
		if keepGoing {
			keepGoing = f(v)
		}
	})
}

// IterateControllerIssuers calls f for each registered controller JWT issuer in an unspecified order.
// Iteration stops early when f returns false. ExtJwt issuers are not included.
func (a *TokenIssuerCache) IterateControllerIssuers(f func(issuer common.TokenIssuer) bool) {
	keepGoing := true
	a.controllerIssuers.IterCb(func(key string, v common.TokenIssuer) {
		if keepGoing {
			keepGoing = f(v)
		}
	})
}

// GetIssuerByKid searches both external JWT signers and controller issuers for the one
// that owns the given key ID. Returns nil if no issuer claims that kid.
func (a *TokenIssuerCache) GetIssuerByKid(kid string) common.TokenIssuer {
	for _, issuer := range a.externalIssuers.Items() {
		if pubKey, ok := issuer.PubKeyByKid(kid); ok && pubKey.PubKey != nil {
			return issuer
		}
	}

	for _, controller := range a.controllerIssuers.Items() {
		if pubKey, ok := controller.PubKeyByKid(kid); ok && pubKey.PubKey != nil {
			return controller
		}
	}

	return nil
}

// GetKids returns a deduplicated list of all key IDs known across both external JWT signers
// and controller issuers. Used for diagnostic logging when a token cannot be matched.
func (a *TokenIssuerCache) GetKids() []string {
	kidMap := map[string]struct{}{}

	for _, issuer := range a.externalIssuers.Items() {
		issuerKids := issuer.GetKids()

		for _, kid := range issuerKids {
			kidMap[kid] = struct{}{}
		}
	}

	for _, controller := range a.controllerIssuers.Items() {
		controllerKids := controller.GetKids()

		for _, kid := range controllerKids {
			kidMap[kid] = struct{}{}
		}
	}

	kids := make([]string, 0, len(kidMap))
	for kid := range kidMap {
		kids = append(kids, kid)
	}

	return kids
}

var _ common.TokenIssuer = (*TokenIssuerExtJwt)(nil)

// TokenIssuerExtJwt is a TokenIssuer implementation for external JWT signers.
// Supports verification via certificate PEM or JWKS endpoints.
type TokenIssuerExtJwt struct {
	sync.Mutex
	jwksLastRequest time.Time

	kidToPubKey       map[string]common.IssuerPublicKey
	jwksResponse      *jwks.Response
	externalJwtSigner *db.ExternalJwtSigner

	jwksResolver jwks.Resolver
}

// GetKids returns all key IDs currently cached for this external JWT signer, triggering
// a non-forced Resolve to populate the cache if it is empty.
func (r *TokenIssuerExtJwt) GetKids() []string {
	_ = r.Resolve(false)

	kids := make([]string, 0, len(r.kidToPubKey))

	for kid := range r.kidToPubKey {
		kids = append(kids, kid)
	}

	return kids
}

// IsControllerTokenIssuer returns false because this issuer represents an external signer,
// not a controller-issued token.
func (r *TokenIssuerExtJwt) IsControllerTokenIssuer() bool {
	return false
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
func (r *TokenIssuerExtJwt) VerifyToken(token string) *common.TokenVerificationResult {
	err := r.Resolve(false)

	if err != nil {
		pfxlog.Logger().WithError(err).Warn("error during routine resolve of external jwt signer cert/jwks, attempting to verify the token with any cached keys")
	}

	claims := jwt.MapClaims{}
	resultToken, err := jwt.ParseWithClaims(token, claims, r.keyFunc)

	result := &common.TokenVerificationResult{
		Error:           err,
		Token:           resultToken,
		Claims:          claims,
		IdClaimSelector: r.IdentityIdClaimsSelector(),
	}

	if resultToken != nil && !resultToken.Valid && result.Error == nil {
		result.Error = errors.New("token invalid for an unspecified reason")
	}

	if !result.IsValid() {
		return result
	}

	result.IdClaimValue, result.Error = resolveStringClaimSelector(claims, r.IdentityIdClaimsSelector())

	if result.Error != nil {
		result.Error = fmt.Errorf("could not resolve identity claim property %s: %w", r.IdentityIdClaimsSelector(), result.Error)
	}

	return result
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
func (r *TokenIssuerExtJwt) PubKeyByKid(kid string) (common.IssuerPublicKey, bool) {
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
		r.kidToPubKey = map[string]common.IssuerPublicKey{
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

				r.kidToPubKey[key.KeyId] = common.IssuerPublicKey{
					PubKey: certs[0].PublicKey,
					Chain:  certs,
				}
			} else {
				//else the key properties are the only way to construct the public key
				k, err := jwks.KeyToPublicKey(key)

				if err != nil {
					return err
				}

				r.kidToPubKey[key.KeyId] = common.IssuerPublicKey{
					PubKey: k,
				}
			}

		}

		r.jwksResponse = jwksResponse

		return nil
	}

	return errors.New("instructed to add external jwt signer that does not have a certificate PEM or JWKS endpoint")
}

var _ common.TokenIssuer = (*ControllerTokenIssuer)(nil)

// ControllerTokenIssuer implements TokenIssuer for controller-issued JWTs. Each running
// controller is represented by one instance keyed by the SHA-1 fingerprint of its TLS
// certificate (the key ID). It is always considered enabled and does not support enrollment (toCert/toToken enrollment).
type ControllerTokenIssuer struct {
	controllerId     string
	controllerName   string
	pubKey           common.IssuerPublicKey
	kid              string
	controllerIssuer string
}

// GetKids returns the single key ID derived from the controller's TLS certificate fingerprint.
func (o *ControllerTokenIssuer) GetKids() []string {
	return []string{o.kid}
}

// IsControllerTokenIssuer returns true, distinguishing this issuer from external JWT signers.
func (o *ControllerTokenIssuer) IsControllerTokenIssuer() bool {
	return true
}

func (o *ControllerTokenIssuer) Id() string {
	return o.controllerId
}

func (o *ControllerTokenIssuer) TypeName() string {
	return "openZitiController"
}

func (o *ControllerTokenIssuer) Name() string {
	return o.controllerName
}

func (o *ControllerTokenIssuer) IsEnabled() bool {
	return true
}

func (o *ControllerTokenIssuer) PubKeyByKid(kid string) (common.IssuerPublicKey, bool) {
	if kid == o.kid {
		return o.pubKey, true
	}

	return common.IssuerPublicKey{}, false
}

func (o *ControllerTokenIssuer) Resolve(_ bool) error {
	return nil
}

func (o *ControllerTokenIssuer) ExpectedIssuer() string {
	return o.controllerIssuer
}

func (o *ControllerTokenIssuer) ExpectedAudience() string {
	return "openziti"
}

func (o *ControllerTokenIssuer) AuthenticatorId() string {
	return "internal"
}

func (o *ControllerTokenIssuer) EnrollmentAuthPolicyId() string {
	return ""
}

func (o *ControllerTokenIssuer) EnrollmentAttributeClaimsSelector() string {
	return ""
}

func (o *ControllerTokenIssuer) EnrollmentNameClaimSelector() string {
	return "name"
}

func (o *ControllerTokenIssuer) IdentityIdClaimsSelector() string {
	return "sub"
}

func (o *ControllerTokenIssuer) UseExternalId() bool {
	return false
}

func (o *ControllerTokenIssuer) VerifyToken(token string) *common.TokenVerificationResult {
	claims := jwt.MapClaims{}

	resultToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {

		kid := ""

		if token.Header != nil {
			kid, _ = token.Header["kid"].(string)
		}

		if kid == "" {
			kid = "<not set>"
		}

		key, ok := o.PubKeyByKid(kid)

		if !ok || key.PubKey == nil {
			return nil, errors.New("pubkey not found by kid " + kid)
		}

		return key.PubKey, nil
	})

	result := &common.TokenVerificationResult{
		Token:           resultToken,
		Claims:          claims,
		IdClaimSelector: o.IdentityIdClaimsSelector(),
		Error:           err,
	}

	if resultToken != nil && !resultToken.Valid && result.Error == nil {
		result.Error = errors.New("token invalid for an unspecified reason")
	}

	if !result.IsValid() {
		return result
	}

	result.IdClaimValue, _ = claims["sub"].(string)
	result.IdClaimValue = strings.TrimSpace(result.IdClaimValue)

	if result.IdClaimValue == "" {
		result.Error = errors.New("no sub claim found in token")
	}

	return result
}

func (o *ControllerTokenIssuer) EnrollToCertEnabled() bool {
	return false
}

func (o *ControllerTokenIssuer) EnrollToTokenEnabled() bool {
	return false
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
