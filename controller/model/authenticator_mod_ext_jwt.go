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
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var _ AuthProcessor = &AuthModuleExtJwt{}

const (
	// AuthMethodExtJwt is the authentication method identifier for external JWT authentication.
	AuthMethodExtJwt = "ext-jwt"
	// InternalTokenIssuerClaim is the key for storing the token issuer in JWT claims during verification.
	InternalTokenIssuerClaim = "-internal-token-issuer"
	// JwksQueryTimeout is the duration to cache JWKS endpoint responses to reduce network calls.
	JwksQueryTimeout = 1 * time.Second
	// MaxCandidateJwtProcessing limits the number of candidate tokens to process during authentication.
	MaxCandidateJwtProcessing = 2
)

// AuthTokenVerificationResult extends TokenVerificationResult with authentication context.
// Includes error information, auth policy, and identity from authentication processing.
type AuthTokenVerificationResult struct {
	BearerToken *common.BearerTokenHeader
	AuthPolicy  *AuthPolicy
	Identity    *Identity

	Error error
}

// LogResult logs the authentication verification result with contextual fields.
// Logs issuer, policy, identity, and audiences when available.
func (r *AuthTokenVerificationResult) LogResult(logger *logrus.Entry) {
	headerIndex := -1

	if r.AuthPolicy != nil {
		logger = logger.WithField("authPolicyId", r.AuthPolicy.Id)
	}

	if r.Identity != nil {
		logger = logger.WithField("identityId", r.Identity.Id)
	}

	if r.BearerToken != nil {
		headerIndex = r.BearerToken.HeaderIndex

		if r.BearerToken.TokenIssuer != nil {
			logger = logger.WithField("tokenIssuerId", r.BearerToken.TokenIssuer.Id()).
				WithField("tokenIssuerType", r.BearerToken.TokenIssuer.TypeName()).
				WithField("issuer", r.BearerToken.TokenIssuer.ExpectedIssuer()).
				WithField("expectedAudience", r.BearerToken.TokenIssuer.ExpectedAudience())
		}

		if r.BearerToken.TokenVerificationResult != nil {
			if r.BearerToken.TokenVerificationResult.Token != nil && r.BearerToken.TokenVerificationResult.Claims != nil {
				audiences := r.BearerToken.Audience()
				logger = logger.WithField("tokenAudiences", audiences)
			}
		}
	}

	if r.Error == nil {
		logger.Debugf("validated candidate JWT at index %d", headerIndex)
	} else {
		logger.WithError(r.Error).Errorf("failed to validate candidate JWT at index %d", headerIndex)
	}
}

// AuthModuleExtJwt handles JWT authentication using external token issuers.
// Uses the token issuer cache to verify tokens and authenticate identities.
type AuthModuleExtJwt struct {
	BaseAuthenticator
}

// NewAuthModuleExtJwt creates a new AuthModuleExtJwt handler.
func NewAuthModuleExtJwt(env Env) *AuthModuleExtJwt {
	ret := &AuthModuleExtJwt{
		BaseAuthenticator: BaseAuthenticator{
			method: AuthMethodExtJwt,
			env:    env,
		},
	}

	return ret
}

// CanHandle returns true if the given authentication method is ext-jwt.
func (a *AuthModuleExtJwt) CanHandle(method string) bool {
	return method == a.method
}

// Process handles primary and secondary JWT authentication using external token issuers.
func (a *AuthModuleExtJwt) Process(context AuthContext) (AuthResult, error) {
	if context.GetPrimaryIdentity() == nil {
		return a.ProcessPrimary(context)
	}
	return a.ProcessSecondary(context)
}

// ProcessPrimary handles the first-factor phase of external JWT authentication. It retrieves
// all candidate external bearer tokens from the security token context, selects the first one
// that passes issuer, audience, and identity-claim checks, and returns an AuthResultIssuer on
// success. Specific error types are returned to indicate missing, expired, or invalid tokens.
func (a *AuthModuleExtJwt) ProcessPrimary(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	bundle := &AuthBundle{
		Identity: context.GetPrimaryIdentity(),
	}

	if bundle.Identity != nil {
		return nil, errors.New("primary identity already set, cannot process again")
	}

	authType := "primary"

	ids := make([]string, 0, 5)
	issuers := make([]string, 0, 5)

	tokenIssuerCache := a.env.GetTokenIssuerCache()
	tokenIssuerCache.IterateExternalIssuers(func(issuer common.TokenIssuer) bool {
		if issuer.IsEnabled() {
			ids = append(ids, issuer.Id())
			issuers = append(issuers, issuer.ExpectedIssuer())
		}

		return true
	})

	if len(ids) == 0 {
		return nil, errorz.NewUnauthorized()
	}

	securityTokenCtx := context.GetSecurityTokenCtx()
	externalTokens := securityTokenCtx.GetExternalTokens()

	if len(externalTokens) == 0 {
		reason := "encountered 0 candidate JWTs, verification cannot occur"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedPrimaryExtTokenMissing(ids, issuers)
	}

	var candidates []*common.BearerTokenHeader
	for _, externalToken := range externalTokens {
		if externalToken.TokenIssuer != nil && externalToken.TokenIssuer.IsEnabled() {
			candidates = append(candidates, externalToken)
		}
	}

	if len(candidates) == 0 {
		reason := fmt.Sprintf("encountered %d candidate JWTs, none of which were valid for %s authentication", len(externalTokens), authType)
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedPrimaryExtTokenMissing(ids, issuers)
	}

	var primaryResult *AuthTokenVerificationResult

	candidatesHaveExpired := false
	candidatesHaveInvalidSignature := false

	for _, candidate := range candidates {

		if !candidate.IsValid() {
			if candidate.TokenVerificationResult.TokenIsExpired() {
				candidatesHaveExpired = true
			} else {
				candidatesHaveInvalidSignature = true
			}
			continue
		}

		verifyResult := a.verifyTokenClaims(candidate)

		if verifyResult.Error != nil {
			continue
		}

		if verifyResult.Identity == nil {
			continue
		}

		if verifyResult.AuthPolicy == nil {
			continue
		}

		if !verifyResult.AuthPolicy.Primary.ExtJwt.Allowed {
			continue
		}

		if len(verifyResult.AuthPolicy.Primary.ExtJwt.AllowedExtJwtSigners) > 0 {
			if !stringz.Contains(verifyResult.AuthPolicy.Primary.ExtJwt.AllowedExtJwtSigners, candidate.TokenIssuer.Id()) {
				continue
			}
		}

		primaryResult = verifyResult
		break
	}

	if primaryResult == nil {
		reason := fmt.Sprintf("encountered %d candidate JWTs, none of which were valid for %s authentication", len(candidates), authType)
		failEvent := a.NewAuthEventFailure(context, bundle, reason)
		logger.Error(reason)
		a.DispatchEvent(failEvent)

		if candidatesHaveExpired {
			return nil, errorz.NewUnauthorizedPrimaryExtTokenExpired(ids, issuers)
		}

		if candidatesHaveInvalidSignature {
			return nil, errorz.NewUnauthorizedPrimaryExtTokenInvalid(ids, issuers)
		}

		return nil, errorz.NewUnauthorizedPrimaryExtTokenMissing(ids, issuers)
	}

	//success
	result := &AuthResultIssuer{
		AuthResultBase: AuthResultBase{
			authPolicy: primaryResult.AuthPolicy,
			identity:   primaryResult.Identity,
			env:        a.env,
		},
		Issuer: primaryResult.BearerToken.TokenIssuer,
	}

	bundle.Identity = primaryResult.Identity
	bundle.AuthPolicy = primaryResult.AuthPolicy
	bundle.TokenIssuer = primaryResult.BearerToken.TokenIssuer

	successEvent := a.NewAuthEventSuccess(context, bundle)
	a.DispatchEvent(successEvent)

	primaryResult.LogResult(logger)

	return result, nil

}

// ProcessSecondary handles secondary JWT authentication using external token issuers.
func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	bundle := &AuthBundle{
		Identity: context.GetPrimaryIdentity(),
	}

	if bundle.Identity == nil {
		return nil, errors.New("primary identity not set, cannot process again")
	}

	authPolicy, err := a.env.GetManagers().AuthPolicy.Read(bundle.Identity.AuthPolicyId)

	if err != nil {
		return nil, fmt.Errorf("could not read auth policy by id %s: %w", bundle.Identity.AuthPolicyId, err)
	}

	bundle.AuthPolicy = authPolicy

	id := stringz.OrEmpty(bundle.AuthPolicy.Secondary.RequiredExtJwtSigner)
	if id == "" {
		return nil, errors.New("no secondary auth policy configured, expected one")
	}

	securityTokenCtx := context.GetSecurityTokenCtx()
	candidateToken := securityTokenCtx.GetExternalTokenForExtJwtSigner(id)

	extJwt, err := a.env.GetManagers().ExternalJwtSigner.Read(id)

	if err != nil {
		return nil, fmt.Errorf("could not read required external jwt by id %s: %w", id, err)
	}

	ids := []string{
		id,
	}

	issuers := []string{
		stringz.OrEmpty(extJwt.Issuer),
	}

	if candidateToken == nil {
		reason := "encountered 0 candidate JWTs, verification cannot occur"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedSecondaryExtTokenMissing(ids, issuers)
	}

	if candidateToken.TokenVerificationResult == nil {
		reason := "encountered a candidate but no verification result was found"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedSecondaryExtTokenMissing(ids, issuers)
	}

	if candidateToken.TokenIssuer == nil {
		reason := "encountered a candidate but no token issuer was found"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedSecondaryExtTokenMissing(ids, issuers)
	}

	if !candidateToken.TokenVerificationResult.IsValid() {
		failEvent := a.NewAuthEventFailure(context, bundle, candidateToken.TokenVerificationResult.Error.Error())

		logger.WithError(candidateToken.TokenVerificationResult.Error).Error("token verification failed")
		a.DispatchEvent(failEvent)

		if candidateToken.TokenVerificationResult.TokenIsExpired() {
			return nil, errorz.NewUnauthorizedSecondaryExtTokenExpired(ids, issuers)
		}

		return nil, errorz.NewUnauthorizedSecondaryExtTokenInvalid(ids, issuers)
	}

	verifyResult := a.verifyTokenClaims(candidateToken)

	if verifyResult.Error != nil {
		failEvent := a.NewAuthEventFailure(context, bundle, verifyResult.Error.Error())

		logger.WithError(verifyResult.Error).Error("token claim verification failed")
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedSecondaryExtTokenInvalid(ids, issuers)
	}

	if verifyResult.Identity.Id != bundle.Identity.Id {
		failEvent := a.NewAuthEventFailure(context, bundle, "identity mismatch, the primary identity did not match the secondary")
		logger.Error("identity mismatch, the primary identity did not match the secondary")
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedSecondaryExtTokenInvalid(ids, issuers)
	}

	//success
	bundle.TokenIssuer = candidateToken.TokenIssuer

	result := &AuthResultIssuer{
		AuthResultBase: AuthResultBase{
			authPolicy: bundle.AuthPolicy,
			identity:   bundle.Identity,
			env:        a.env,
		},
		Issuer: candidateToken.TokenIssuer,
	}

	successEvent := a.NewAuthEventSuccess(context, bundle)
	a.DispatchEvent(successEvent)

	return result, nil

}

// AuthResultIssuer represents a successful JWT authentication result from an external token issuer.
type AuthResultIssuer struct {
	AuthResultBase
	Issuer common.TokenIssuer
}

// IsSuccessful returns true if the token issuer and identity are both present.
func (a *AuthResultIssuer) IsSuccessful() bool {
	return a.Issuer != nil && a.Identity() != nil
}

// AuthenticatorId returns the authenticator ID from the token issuer.
func (a *AuthResultIssuer) AuthenticatorId() string {
	if a.Issuer == nil {
		return ""
	}

	return a.Issuer.AuthenticatorId()
}

// Authenticator returns an Authenticator instance for this JWT authentication.
func (a *AuthResultIssuer) Authenticator() *Authenticator {
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

// verifyTokenClaims validates the issuer and audience claims of a pre-parsed bearer token,
// then resolves the associated identity and auth policy using the configured claim selectors.
// It does not perform signature verification — that is expected to have been done by the
// SecurityTokenCtx. An AuthTokenVerificationResult with a non-nil Error indicates failure.
func (a *AuthModuleExtJwt) verifyTokenClaims(token *common.BearerTokenHeader) *AuthTokenVerificationResult {
	result := &AuthTokenVerificationResult{
		BearerToken: token,
	}

	issuer := token.Issuer()

	if issuer == "" {
		result.Error = errors.New("token claims did not contain an issuer")
		return result
	}

	if issuer != token.TokenIssuer.ExpectedIssuer() {
		result.Error = fmt.Errorf("token issuer [%s] does not match expected issuer [%s]", token.TokenIssuer.Id(), token.TokenIssuer.ExpectedIssuer())
		return result
	}

	audience := token.Audience()

	if len(audience) == 0 {
		result.Error = errors.New("token claims did not contain an audience")
		return result
	}

	if !stringz.Contains(audience, token.TokenIssuer.ExpectedAudience()) {
		result.Error = fmt.Errorf("token audience [%s] does not match expected audience [%s]", audience, token.TokenIssuer.ExpectedAudience())
		return result
	}

	var authPolicy *AuthPolicy
	var err error

	claimIdLookupMethod := ""

	if token.TokenIssuer.UseExternalId() {
		claimIdLookupMethod = "external id"
		authPolicy, result.Identity, err = getAuthPolicyByExternalId(a.env, AuthMethodExtJwt, "", token.TokenVerificationResult.IdClaimValue)
	} else {
		claimIdLookupMethod = "identity id"
		authPolicy, result.Identity, err = getAuthPolicyByIdentityId(a.env, AuthMethodExtJwt, "", token.TokenVerificationResult.IdClaimValue)
	}

	if err != nil {
		result.Error = fmt.Errorf("error during authentication policy and identity lookup by claims type [%s] and claim id [%s]: %w", claimIdLookupMethod, token.TokenVerificationResult.IdClaimValue, err)
		return result
	}

	if authPolicy == nil {
		result.Error = fmt.Errorf("no authentication policy found for claims type [%s] and claim id [%s]: %w", claimIdLookupMethod, token.TokenVerificationResult.IdClaimValue, err)
		return result
	}

	result.AuthPolicy = authPolicy

	if result.Identity == nil {
		result.Error = fmt.Errorf("no identity found for claims type [%s] and claim id [%s]: %w", claimIdLookupMethod, token.TokenVerificationResult.IdClaimValue, err)
		return result
	}

	if result.Identity.Disabled {
		result.Error = fmt.Errorf("the identity [%s] is disabled, disabledAt %v, disabledUntil: %v", result.Identity.Id, result.Identity.DisabledAt, result.Identity.DisabledUntil)
		return result
	}

	return result
}
