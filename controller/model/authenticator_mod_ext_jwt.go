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
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/models"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var _ AuthProcessor = &AuthModuleExtJwt{}

const (
	AuthMethodExtJwt          = "ext-jwt"
	InternalTokenIssuerClaim  = "-internal-token-issuer"
	JwksQueryTimeout          = 1 * time.Second
	MaxCandidateJwtProcessing = 2
)

type AuthTokenVerificationResult struct {
	*TokenVerificationResult

	Error      error
	AuthPolicy *AuthPolicy
	Identity   *Identity
}

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

type AuthModuleExtJwt struct {
	BaseAuthenticator
}

func NewAuthModuleExtJwt(env Env) *AuthModuleExtJwt {
	ret := &AuthModuleExtJwt{
		BaseAuthenticator: BaseAuthenticator{
			method: AuthMethodExtJwt,
			env:    env,
		},
	}

	return ret
}

func (a *AuthModuleExtJwt) CanHandle(method string) bool {
	return method == a.method
}

func (a *AuthModuleExtJwt) Process(context AuthContext) (AuthResult, error) {
	return a.process(context)
}

func (a *AuthModuleExtJwt) ProcessSecondary(context AuthContext) (AuthResult, error) {
	return a.process(context)
}

type AuthResultIssuer struct {
	AuthResultBase
	Issuer TokenIssuer
}

func (a *AuthResultIssuer) IsSuccessful() bool {
	return a.Issuer != nil && a.Identity() != nil
}

func (a *AuthResultIssuer) AuthenticatorId() string {
	if a.Issuer == nil {
		return ""
	}

	return a.Issuer.AuthenticatorId()
}

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

// process attempts to locate a JWT and ExtJwtSigner that  verifies it. If context.GetPrimaryIdentity()==nil, this will be
// processed as a secondary authentication factor.
func (a *AuthModuleExtJwt) process(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", AuthMethodExtJwt)

	bundle := &AuthBundle{
		Identity: context.GetPrimaryIdentity(),
	}

	authType := "secondary"
	if bundle.Identity == nil {
		authType = "primary"
	}

	headers := context.GetHeaders()
	candidateTokens := headers.GetStrings(AuthorizationHeader)

	var verifyResults []*AuthTokenVerificationResult

	if len(candidateTokens) == 0 {
		reason := "encountered 0 candidate JWTs, verification cannot occur"
		failEvent := a.NewAuthEventFailure(context, bundle, reason)

		logger.Error(reason)
		a.DispatchEvent(failEvent)

		return nil, apierror.NewInvalidAuth()
	}

	for i, candidateToken := range candidateTokens {
		candidateToken = strings.TrimSpace(candidateToken)
		if !strings.HasPrefix(candidateToken, "Bearer ") {
			verifyResult := &AuthTokenVerificationResult{
				Error: errors.New("authorization header did not not start with Bearer"),
			}
			verifyResults = append(verifyResults, verifyResult)
			continue
		}

		candidateToken = strings.TrimPrefix(candidateToken, "Bearer ")

		verifyResult := a.verify(context, candidateToken)

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

			verifyResult.LogResult(logger, i)

			return result, nil
		} else {
			verifyResults = append(verifyResults, verifyResult)
		}
	}

	logger.Errorf("encountered %d candidate and all failed to validate for %s authentication, see the following log messages", len(candidateTokens), authType)
	for i, result := range verifyResults {
		result.LogResult(logger, i)
	}

	reason := fmt.Sprintf("encountered %d candidate JWTs and all failed to validate for %s authentication", len(verifyResults), authType)
	failEvent := a.NewAuthEventFailure(context, bundle, reason)
	a.DispatchEvent(failEvent)

	return nil, apierror.NewInvalidAuth()
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

func (a *AuthModuleExtJwt) verify(context AuthContext, jwtStr string) *AuthTokenVerificationResult {
	result := &AuthTokenVerificationResult{}

	targetIdentity := context.GetPrimaryIdentity()
	isPrimary := targetIdentity == nil

	var err error
	result.TokenVerificationResult, err = a.env.GetTokenIssuerCache().VerifyTokenByInspection(jwtStr)

	if err != nil {
		result.Error = fmt.Errorf("jwt failed validation: %w", err)
		return result
	}

	if !result.IsValid() {
		result.Error = errors.New("authorization failed, jwt did not pass verification")
		return result
	}

	var authPolicy *AuthPolicy

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

	if !isPrimary && targetIdentity.Id != result.Identity.Id {
		result.Error = fmt.Errorf("jwt mapped to identity [%s - %s], which does not match the current sessions identity [%s - %s]", result.Identity.Id, result.Identity.Name, targetIdentity.Id, targetIdentity.Name)
		return result
	}

	if result.Identity.Disabled {
		result.Error = fmt.Errorf("the identity [%s] is disabled, disabledAt %v, disabledUntil: %v", result.Identity.Id, result.Identity.DisabledAt, result.Identity.DisabledUntil)
		return result
	}

	if isPrimary {
		err = a.verifyAsPrimary(authPolicy, result.TokenIssuer)

		if err != nil {
			result.Error = fmt.Errorf("primary external jwt processing failed on authentication policy [%s]: %w", authPolicy.Id, err)
			return result
		}

	} else {
		if authPolicy.Secondary.RequiredExtJwtSigner == nil {
			result.Error = fmt.Errorf("secondary external jwt authentication on authentication policy [%s] is not configured", authPolicy.Id)
			return result
		}

		if result.TokenIssuer.Id() != *authPolicy.Secondary.RequiredExtJwtSigner {
			result.Error = fmt.Errorf("secondary external jwt authentication failed on authentication policy [%s]: the required ext-jwt signer [%s] did not match the validating id [%s]", authPolicy.Id, *authPolicy.Secondary.RequiredExtJwtSigner, result.TokenIssuer.Id())
			return result
		}
	}

	return result
}
