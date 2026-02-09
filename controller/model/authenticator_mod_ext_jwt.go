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
	"strings"
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
	*TokenVerificationResult

	Error      error
	AuthPolicy *AuthPolicy
	Identity   *Identity
}

// LogResult logs the authentication verification result with contextual fields.
// Logs issuer, policy, identity, and audiences when available.
func (r *AuthTokenVerificationResult) LogResult(logger *logrus.Entry, index int) {
	if r.AuthPolicy != nil {
		logger = logger.WithField("authPolicyId", r.AuthPolicy.Id)
	}

	if r.Identity != nil {
		logger = logger.WithField("identityId", r.Identity.Id)
	}

	if r.TokenVerificationResult != nil {
		if r.TokenVerificationResult.TokenIssuer != nil {
			logger = logger.WithField("tokenIssuerId", r.TokenVerificationResult.TokenIssuer.Id()).
				WithField("tokenIssuerType", r.TokenVerificationResult.TokenIssuer.TypeName()).
				WithField("issuer", r.TokenIssuer.ExpectedIssuer()).
				WithField("expectedAudience", r.TokenIssuer.ExpectedAudience())
		}

		if r.TokenVerificationResult.Token != nil {
			if r.TokenVerificationResult.Token != nil && r.TokenVerificationResult.Claims != nil {
				audiences, _ := r.Token.Claims.GetAudience()
				logger = logger.WithField("tokenAudiences", audiences)
			}
		}
	}

	if r.Error == nil {
		logger.Debugf("validated candidate JWT at index %d", index)
	} else {
		logger.WithError(r.Error).Errorf("failed to validate candidate JWT at index %d", index)
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
	tokenIssuerCache.IterateIssuers(func(issuer TokenIssuer) bool {
		ids = append(ids, issuer.Id())
		issuers = append(issuers, issuer.ExpectedIssuer())
		return true
	})

	if len(ids) == 0 {
		return nil, errorz.NewUnauthorized()
	}

	securityTokenCtx := context.GetSecurityTokenCtx()
	candidateTokens := securityTokenCtx.UnverifiedExternalBearerTokens

	if len(candidateTokens) == 0 {
		reason := "encountered 0 candidate JWTs, verification cannot occur"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedPrimaryExtTokenMissing(ids, issuers)
	}

	var verifyResults []*AuthTokenVerificationResult
	var targetToken *common.BearerTokenHeader
	var targetTokenIssuer TokenIssuer
	var targetTokenHeaderIndex int

	for i, candidateToken := range candidateTokens {
		targetTokenHeaderIndex = i
		if candidateToken.Token.Claims == nil {
			continue
		}

		candidateIssuer, err := candidateToken.Claims.GetIssuer()

		if err != nil || candidateIssuer == "" {
			continue
		}

		tokenIssuer := tokenIssuerCache.GetByIssuerString(candidateIssuer)

		if tokenIssuer == nil {
			verifyResult := &AuthTokenVerificationResult{
				Error: errors.New("bearer token issuer did not match any enabled external jwt signers"),
			}
			verifyResults = append(verifyResults, verifyResult)
			continue
		}

		targetToken = candidateToken
		targetTokenIssuer = tokenIssuer
		break
	}

	if targetToken == nil {
		logger.Errorf("encountered %d candidate and all failed to validate for %s authentication, see the following log messages", len(candidateTokens), authType)
		for i, result := range verifyResults {
			result.LogResult(logger, i)
		}

		reason := fmt.Sprintf("encountered %d candidate JWTs and all failed to validate for %s authentication", len(verifyResults), authType)
		failEvent := a.NewAuthEventFailure(context, bundle, reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedPrimaryExtTokenMissing(ids, issuers)
	}

	//found a token and issuer
	tokenVerificationResult, err := targetTokenIssuer.VerifyToken(targetToken.Raw)

	if err != nil {
		verifyResult := &AuthTokenVerificationResult{
			Error: fmt.Errorf("failed to verify token: %w", err),
		}
		verifyResults = []*AuthTokenVerificationResult{verifyResult}

		//gross but we have zero type information here
		if strings.Contains(err.Error(), "token is expired") {
			return nil, errorz.NewUnauthorizedPrimaryExtTokenExpired(ids, issuers)
		}

		return nil, errorz.NewUnauthorizedPrimaryExtTokenInvalid(ids, issuers)
	}

	verifyResult := a.processTokenVerificationResult(tokenVerificationResult)

	if !verifyResult.IsValid() {
		verifyResult := &AuthTokenVerificationResult{
			Error: errors.New("token is not valid"),
		}
		verifyResults = []*AuthTokenVerificationResult{verifyResult}
		return nil, errorz.NewUnauthorizedPrimaryExtTokenInvalid(ids, issuers)
	}

	//success
	result := &AuthResultIssuer{
		AuthResultBase: AuthResultBase{
			authPolicy: verifyResult.AuthPolicy,
			identity:   verifyResult.Identity,
			env:        a.env,
		},
		Issuer: verifyResult.TokenIssuer,
	}

	bundle.Identity = verifyResult.Identity
	bundle.AuthPolicy = verifyResult.AuthPolicy
	bundle.TokenIssuer = verifyResult.TokenIssuer

	successEvent := a.NewAuthEventSuccess(context, bundle)
	a.DispatchEvent(successEvent)

	verifyResult.LogResult(logger, targetTokenHeaderIndex)

	return result, nil

}

// ProcessSecondary handles secondary JWT authentication using external token issuers.
func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	bundle := &AuthBundle{
		Identity: context.GetPrimaryIdentity(),
	}

	securityTokenCtx := context.GetSecurityTokenCtx()
	candidateTokens := securityTokenCtx.UnverifiedExternalBearerTokens

	var err error
	bundle.AuthPolicy, err = a.env.GetManagers().AuthPolicy.Read(bundle.Identity.AuthPolicyId)

	if err != nil {
		return nil, err
	}

	id := stringz.OrEmpty(bundle.AuthPolicy.Secondary.RequiredExtJwtSigner)
	if id == "" {
		return nil, errors.New("no secondary auth policy configured, expected one")
	}
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

	if len(candidateTokens) == 0 {
		reason := "encountered 0 candidate JWTs, verification cannot occur"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, errorz.NewUnauthorizedSecondaryExtTokenMissing(ids, issuers)
	}

	var targetToken *common.BearerTokenHeader
	for _, candidateToken := range candidateTokens {
		tokenIss := ""

		if candidateToken.Token.Claims == nil {
			continue
		}

		tokenIss, err = candidateToken.Claims.GetIssuer()

		if err != nil || tokenIss == "" {
			continue
		}

		if issuers[0] == tokenIss {
			targetToken = candidateToken
			break
		}
	}

	if targetToken == nil {
		reason := fmt.Sprintf("encountered %d candidate JWTs, but no token issuers matched the expected issuer %s for secondary verification", len(candidateTokens), issuers[0])
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)
		return nil, errorz.NewUnauthorizedSecondaryExtTokenMissing(ids, issuers)
	}

	verifyResult := a.verify(context, targetToken.Raw)

	if verifyResult.Error == nil {
		//success
		result := &AuthResultIssuer{
			AuthResultBase: AuthResultBase{
				authPolicy: verifyResult.AuthPolicy,
				identity:   verifyResult.Identity,
				env:        a.env,
			},
			Issuer: verifyResult.TokenIssuer,
		}

		bundle.Identity = verifyResult.Identity
		bundle.AuthPolicy = verifyResult.AuthPolicy
		bundle.TokenIssuer = verifyResult.TokenIssuer

		successEvent := a.NewAuthEventSuccess(context, bundle)
		a.DispatchEvent(successEvent)

		verifyResult.LogResult(logger, targetToken.HeaderIndex)

		return result, nil
	}

	reason := fmt.Errorf("bearer token at header index %d matched issuer but did not verify: %w", targetToken.HeaderIndex, verifyResult.Error)
	failEvent := a.NewAuthEventFailure(context, bundle, reason.Error())
	logger.Error(reason)
	a.DispatchEvent(failEvent)

	//gross but we have zero type information here
	if strings.Contains(verifyResult.Error.Error(), "token is expired") {
		return nil, errorz.NewUnauthorizedSecondaryExtTokenExpired(ids, issuers)
	}

	return nil, errorz.NewUnauthorizedSecondaryExtTokenInvalid(ids, issuers)
}

// AuthResultIssuer represents a successful JWT authentication result from an external token issuer.
type AuthResultIssuer struct {
	AuthResultBase
	Issuer TokenIssuer
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

func (a *AuthModuleExtJwt) verifyAsPrimary(authPolicy *AuthPolicy, tokenIssuer TokenIssuer) error {
	if !authPolicy.Primary.ExtJwt.Allowed {
		return errors.New("primary external jwt authentication on auth policy is disabled")
	}

	if len(authPolicy.Primary.ExtJwt.AllowedExtJwtSigners) == 0 {
		//allow any valid JWT
		return nil
	}

	for _, allowedId := range authPolicy.Primary.ExtJwt.AllowedExtJwtSigners {
		if allowedId == tokenIssuer.Id() {
			return nil
		}
	}

	return fmt.Errorf("allowed issuers does not contain the external jwt id: %s, expected one of: %v", tokenIssuer.Id(), authPolicy.Primary.ExtJwt.AllowedExtJwtSigners)
}

func (a *AuthModuleExtJwt) processTokenVerificationResult(tokenResult *TokenVerificationResult) *AuthTokenVerificationResult {
	result := &AuthTokenVerificationResult{
		TokenVerificationResult: tokenResult,
	}

	var authPolicy *AuthPolicy
	var err error

	claimIdLookupMethod := ""
	if result.TokenIssuer.UseExternalId() {
		claimIdLookupMethod = "external id"
		authPolicy, result.Identity, err = getAuthPolicyByExternalId(a.env, AuthMethodExtJwt, "", result.IdClaimValue)
	} else {
		claimIdLookupMethod = "identity id"
		authPolicy, result.Identity, err = getAuthPolicyByIdentityId(a.env, AuthMethodExtJwt, "", result.IdClaimValue)
	}

	if err != nil {
		result.Error = fmt.Errorf("error during authentication policy and identity lookup by claims type [%s] and claim id [%s]: %w", claimIdLookupMethod, result.IdClaimValue, err)
		return result
	}

	if authPolicy == nil {
		result.Error = fmt.Errorf("no authentication policy found for claims type [%s] and claim id [%s]: %w", claimIdLookupMethod, result.IdClaimValue, err)
		return result
	}

	result.AuthPolicy = authPolicy

	if result.Identity == nil {
		result.Error = fmt.Errorf("no identity found for claims type [%s] and claim id [%s]: %w", claimIdLookupMethod, result.IdClaimValue, err)
		return result
	}

	if result.Identity.Disabled {
		result.Error = fmt.Errorf("the identity [%s] is disabled, disabledAt %v, disabledUntil: %v", result.Identity.Id, result.Identity.DisabledAt, result.Identity.DisabledUntil)
		return result
	}

	return result
}

func (a *AuthModuleExtJwt) verify(context AuthContext, jwtStr string) *AuthTokenVerificationResult {

	targetIdentity := context.GetPrimaryIdentity()
	isPrimary := targetIdentity == nil

	tokenVerificationResult, err := a.env.GetTokenIssuerCache().VerifyTokenByInspection(jwtStr)

	if err != nil {
		result := &AuthTokenVerificationResult{}
		result.Error = fmt.Errorf("failed to verify token: %w", err)
		return result
	}

	authTokenResult := a.processTokenVerificationResult(tokenVerificationResult)

	if authTokenResult.Error != nil {
		return authTokenResult
	}

	if isPrimary {
		err = a.verifyAsPrimary(authTokenResult.AuthPolicy, authTokenResult.TokenIssuer)

		if err != nil {
			authTokenResult.Error = fmt.Errorf("primary external jwt processing failed on authentication policy [%s]: %w", authTokenResult.AuthPolicy.Id, err)
			return authTokenResult
		}

	} else {
		if authTokenResult.AuthPolicy.Secondary.RequiredExtJwtSigner == nil {
			authTokenResult.Error = fmt.Errorf("secondary external jwt authentication on authentication policy [%s] is not configured", authTokenResult.AuthPolicy.Id)
			return authTokenResult
		}

		if authTokenResult.TokenIssuer.Id() != *authTokenResult.AuthPolicy.Secondary.RequiredExtJwtSigner {
			authTokenResult.Error = fmt.Errorf("secondary external jwt authentication failed on authentication policy [%s]: the required ext-jwt signer [%s] did not match the validating id [%s]", authTokenResult.AuthPolicy.Id, *authTokenResult.AuthPolicy.Secondary.RequiredExtJwtSigner, authTokenResult.TokenIssuer.Id())
			return authTokenResult
		}
	}

	return authTokenResult
}
