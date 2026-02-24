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

package env

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/permissions"
)

// SecurityCtx resolves and caches the full authentication context for a single HTTP request.
// Starting from the raw token data in a SecurityTokenCtx, it looks up the associated API
// session, identity, auth policy, MFA state, and permission set — each at most once.
// It also supports administrator identity masquerading for privileged operations.
type SecurityCtx struct {
	securityTokenCtx *common.SecurityTokenCtx
	env              model.Env

	resolveApiSessionOnce   sync.Once
	resolvedApiSessionError error

	resolveMfaOnce         sync.Once
	resolvedMfaError       error
	resolvedMfaAuthQueries []*rest_model.AuthQueryDetail

	resolvePermissionsOnce sync.Once
	resolvedPermissions    map[string]struct{}

	resolveVerifiedApiSessionToken *common.SecurityToken
	resolvedApiSession             *model.ApiSession
	resolvedIdentity               *model.Identity
	resolvedAuthPolicy             *model.AuthPolicy

	masqueradeIdentity *model.Identity
	resolvedTotp       *model.Mfa
	resolvedTotpError  error
}

// NewSecurityCtx creates a SecurityCtx that will resolve authentication details from
// securityTokenCtx using the managers and stores available through env.
func NewSecurityCtx(securityTokenCtx *common.SecurityTokenCtx, env model.Env) *SecurityCtx {
	return &SecurityCtx{securityTokenCtx: securityTokenCtx, env: env, resolvedPermissions: map[string]struct{}{}}
}

// GetError returns the error encountered while resolving the API session, or nil if the
// session resolved successfully.
func (ctx *SecurityCtx) GetError() error {
	return ctx.resolvedApiSessionError
}

// GetSecurityTokenCtx returns the underlying token context that holds the raw bearer tokens
// and their pre-parsed issuer associations.
func (ctx *SecurityCtx) GetSecurityTokenCtx() *common.SecurityTokenCtx {
	return ctx.securityTokenCtx
}

// GetIdentity triggers full resolution of the authentication context and returns the
// identity associated with the session. When an administrator has called MasqueradeAsIdentity,
// the masquerade identity is returned instead of the session's own identity.
func (ctx *SecurityCtx) GetIdentity() (*model.Identity, error) {
	ctx.resolve()

	if ctx.masqueradeIdentity != nil {
		return ctx.masqueradeIdentity, nil
	}

	return ctx.resolvedIdentity, ctx.resolvedApiSessionError
}

// GetAuthPolicy triggers resolution and returns the auth policy governing the session's identity.
func (ctx *SecurityCtx) GetAuthPolicy() (*model.AuthPolicy, error) {
	ctx.resolve()
	return ctx.resolvedAuthPolicy, ctx.resolvedApiSessionError
}

// GetApiSession triggers resolution and returns the API session for the request.
func (ctx *SecurityCtx) GetApiSession() (*model.ApiSession, error) {
	ctx.resolve()
	return ctx.resolvedApiSession, ctx.resolvedApiSessionError
}

// GetTotp triggers resolution and returns the TOTP MFA configuration for the session's identity.
func (ctx *SecurityCtx) GetTotp() (*model.Mfa, error) {
	ctx.resolve()
	return ctx.resolvedTotp, ctx.resolvedTotpError
}

// GetApiSessionWithoutResolve returns the API session if it has already been resolved,
// without triggering resolution. Useful for response header helpers that run after
// the primary handler has already resolved the session.
func (ctx *SecurityCtx) GetApiSessionWithoutResolve() (*model.ApiSession, error) {
	return ctx.resolvedApiSession, ctx.resolvedApiSessionError
}

// GetMfaAuthQueriesWithoutResolve returns any outstanding MFA auth queries without triggering
// resolution.
func (ctx *SecurityCtx) GetMfaAuthQueriesWithoutResolve() []*rest_model.AuthQueryDetail {
	return ctx.resolvedMfaAuthQueries
}

// GetMfaErrorWithoutResolve returns the MFA error if secondary authentication checks have
// already run, without triggering resolution.
func (ctx *SecurityCtx) GetMfaErrorWithoutResolve() error {
	return ctx.resolvedMfaError
}

// GetVerifiedApiSessionToken triggers resolution and returns the verified primary security token
// (either a legacy zt-session or an OIDC bearer token) along with any session-level error.
func (ctx *SecurityCtx) GetVerifiedApiSessionToken() (*common.SecurityToken, error) {
	ctx.resolve()
	return ctx.resolveVerifiedApiSessionToken, ctx.resolvedApiSessionError
}

// GetMfaAuthQueries triggers resolution and returns the list of outstanding MFA challenges
// that the identity must complete before gaining full access.
func (ctx *SecurityCtx) GetMfaAuthQueries() []*rest_model.AuthQueryDetail {
	ctx.resolve()
	return ctx.resolvedMfaAuthQueries
}

// GetMfaError triggers resolution and returns any error encountered while evaluating
// secondary MFA requirements (e.g., a missing or expired ext-JWT secondary token).
func (ctx *SecurityCtx) GetMfaError() error {
	ctx.resolve()
	return ctx.resolvedMfaError
}

// MasqueradeAsIdentity allows an authenticated administrator to act as another identity
// for the duration of the request. Subsequent calls to GetIdentity will return the given
// identity rather than the one derived from the session token. Returns an error if the
// caller is not authenticated or does not hold admin privileges.
func (ctx *SecurityCtx) MasqueradeAsIdentity(identity *model.Identity) error {
	ctx.resolve()

	originalIdentity := ctx.resolvedIdentity

	if originalIdentity == nil {
		return errors.New("cannot masquerade as identity when not authenticated")
	}

	if !originalIdentity.IsAdmin && !originalIdentity.IsDefaultAdmin {
		return errors.New("only administrators can masquerade as other identities")
	}

	if !ctx.isFullyAuthed() {
		return errors.New("cannot masquerade as identity until fully authenticated")
	}

	ctx.masqueradeIdentity = identity
	return nil
}

// EndMasquerade clears any active identity masquerade, restoring GetIdentity to return
// the identity associated with the session token.
func (ctx *SecurityCtx) EndMasquerade() {
	ctx.masqueradeIdentity = nil
}

// IsPartiallyAuthed returns true when the primary authentication (session token) succeeded
// but at least one secondary factor (TOTP or ext-JWT) is still outstanding.
func (ctx *SecurityCtx) IsPartiallyAuthed() bool {
	ctx.resolve()

	return ctx.isPartiallyAuthed()
}

func (ctx *SecurityCtx) isPartiallyAuthed() bool {
	primaryAuthOk := ctx.resolvedApiSession != nil && ctx.resolvedApiSessionError == nil
	secondaryAuthOk := len(ctx.resolvedMfaAuthQueries) == 0 && ctx.resolvedMfaError == nil

	return primaryAuthOk && !secondaryAuthOk
}

// IsFullyAuthed returns true when both primary and all secondary authentication factors
// have been satisfied.
func (ctx *SecurityCtx) IsFullyAuthed() bool {
	ctx.resolve()

	return ctx.isFullyAuthed()
}

func (ctx *SecurityCtx) isFullyAuthed() bool {
	primaryAuthOk := ctx.resolvedApiSession != nil && ctx.resolvedApiSessionError == nil
	secondaryAuthOk := len(ctx.resolvedMfaAuthQueries) == 0 && ctx.resolvedMfaError == nil

	return primaryAuthOk && secondaryAuthOk
}

func (ctx *SecurityCtx) setApiSessionError(err error) {
	ctx.resolvedApiSessionError = err
}

func (ctx *SecurityCtx) resolveMfa() {
	if ctx.securityTokenCtx == nil {
		return
	}

	ctx.resolveMfaOnce.Do(func() {
		if ctx.resolvedApiSessionError != nil {
			return
		}

		if ctx.resolvedAuthPolicy == nil {
			return
		}

		totpRequired := ctx.resolvedApiSession.TotpRequired || ctx.resolvedAuthPolicy.Secondary.RequireTotp

		if totpRequired {
			if !ctx.resolvedApiSession.TotpComplete {
				totpAuthQuery := NewAuthQueryZitiTotp()

				if ctx.resolvedIdentity != nil {
					ctx.resolvedTotp, ctx.resolvedTotpError = ctx.env.GetManagers().Mfa.ReadOneByIdentityId(ctx.resolvedIdentity.Id)

					if ctx.resolvedTotp != nil && ctx.resolvedTotp.IsVerified {
						totpAuthQuery.IsTotpEnrolled = true
					}
				}

				ctx.resolvedMfaAuthQueries = append(ctx.resolvedMfaAuthQueries, totpAuthQuery)
			}
		}

		if ctx.resolvedAuthPolicy.Secondary.RequiredExtJwtSigner != nil {
			requireExtJwtSigner, err := ctx.env.GetManagers().ExternalJwtSigner.Read(*ctx.resolvedAuthPolicy.Secondary.RequiredExtJwtSigner)

			if err != nil {
				ctx.resolvedMfaError = fmt.Errorf("error reading required external JWT signer: %w", err)
				return
			}

			if requireExtJwtSigner == nil {
				ctx.resolvedMfaError = fmt.Errorf("required external JWT signer id %s not found", *ctx.resolvedAuthPolicy.Secondary.RequiredExtJwtSigner)
				return
			}

			verifiedExternalToken := ctx.securityTokenCtx.GetExternalTokenForExtJwtSigner(requireExtJwtSigner.Id)

			if verifiedExternalToken == nil {
				ctx.resolvedMfaError = errorz.NewUnauthorizedSecondaryExtTokenMissing([]string{requireExtJwtSigner.Id}, []string{*requireExtJwtSigner.Issuer})
				ctx.resolvedMfaAuthQueries = append(ctx.resolvedMfaAuthQueries, NewAuthQueryExtJwt(requireExtJwtSigner))
			} else if !verifiedExternalToken.IsValid() {
				ctx.resolvedMfaAuthQueries = append(ctx.resolvedMfaAuthQueries, NewAuthQueryExtJwt(requireExtJwtSigner))

				if verifiedExternalToken.TokenVerificationResult.TokenIsExpired() {
					ctx.resolvedMfaError = errorz.NewUnauthorizedSecondaryExtTokenExpired([]string{requireExtJwtSigner.Id}, []string{*requireExtJwtSigner.Issuer})
				} else {
					ctx.resolvedMfaError = errorz.NewUnauthorizedSecondaryExtTokenInvalid([]string{requireExtJwtSigner.Id}, []string{*requireExtJwtSigner.Issuer})
				}
			}
		}
	})
}

func (ctx *SecurityCtx) resolve() {
	if ctx.securityTokenCtx == nil {
		return
	}

	ctx.resolveApiSessionOnce.Do(func() {
		verifiedApiSessionToken, err := ctx.securityTokenCtx.GetVerifiedApiSessionToken()

		if err != nil {
			ctx.setApiSessionError(err)
			return
		}

		if verifiedApiSessionToken == nil {
			ctx.setApiSessionError(errorz.NewUnauthorizedTokensMissing())
			return
		}

		ctx.resolveVerifiedApiSessionToken = verifiedApiSessionToken

		if verifiedApiSessionToken.IsLegacy {
			ctx.resolveZtSession(verifiedApiSessionToken)
		} else {
			ctx.resolveOidcSession(verifiedApiSessionToken)
		}

		if ctx.resolvedApiSessionError != nil {
			ctx.setApiSessionError(ctx.resolvedApiSessionError)
			return
		}

		if ctx.resolvedApiSession == nil {
			return
		}

		ctx.resolveMfa()
		ctx.resolvePermissions()
	})
}

func (ctx *SecurityCtx) resolveZtSession(securityToken *common.SecurityToken) {
	if ctx.securityTokenCtx == nil {
		return
	}

	apiSession, err := ctx.env.GetManagers().ApiSession.ReadByToken(securityToken.ZtSession)

	if err != nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}

	if apiSession == nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}

	if apiSession.IdentityId == "" {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}

	identity, err := ctx.env.GetManagers().Identity.Read(apiSession.IdentityId)

	if err != nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}
	if identity == nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}

	authPolicy, err := ctx.env.GetManagers().AuthPolicy.Read(identity.AuthPolicyId)

	if err != nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}
	if authPolicy == nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedZtSessionInvalid())
		return
	}

	ctx.resolvedIdentity = identity
	ctx.resolvedAuthPolicy = authPolicy
	ctx.resolvedApiSession = apiSession
}

func (ctx *SecurityCtx) resolveOidcSession(securityToken *common.SecurityToken) {
	if securityToken == nil || securityToken.OidcToken == nil || securityToken.OidcToken.AccessClaims == nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedOidcInvalid())
		return
	}

	claims := securityToken.OidcToken.AccessClaims

	identity, err := ctx.env.GetManagers().Identity.Read(claims.Subject)

	if err != nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedOidcInvalid())
		return
	}
	if identity == nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedOidcInvalid())
		return
	}

	authPolicy, err := ctx.env.GetManagers().AuthPolicy.Read(identity.AuthPolicyId)

	if err != nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedOidcInvalid())
		return
	}
	if authPolicy == nil {
		ctx.setApiSessionError(errorz.NewUnauthorizedOidcInvalid())
		return
	}

	ctx.resolvedIdentity = identity
	ctx.resolvedAuthPolicy = authPolicy

	configTypes := map[string]struct{}{}

	for _, configType := range claims.ConfigTypes {
		configTypes[configType] = struct{}{}
	}

	ctx.resolvedApiSession = &model.ApiSession{
		BaseEntity: models.BaseEntity{
			Id:        claims.ApiSessionId,
			CreatedAt: claims.IssuedAt.AsTime(),
			UpdatedAt: claims.IssuedAt.AsTime(),
			IsSystem:  false,
		},
		Token:                   ctx.resolveVerifiedApiSessionToken.OidcToken.Raw,
		IdentityId:              claims.Subject,
		Identity:                identity,
		IPAddress:               securityToken.Request.RemoteAddr,
		ConfigTypes:             configTypes,
		TotpComplete:            claims.TotpComplete(),
		TotpRequired:            false,
		ExpiresAt:               claims.Expiration.AsTime(),
		ExpirationDuration:      time.Until(claims.Expiration.AsTime()),
		LastActivityAt:          time.Now(),
		AuthenticatorId:         claims.AuthenticatorId,
		IsCertExtendable:        claims.IsCertExtendable,
		IsCertExtendRequested:   claims.IsCertExtendRequested,
		IsCertKeyRollRequested:  claims.IsCertKeyRollRequested,
		ImproperClientCertChain: claims.ImproperClientCertChain,
	}
}

func (ctx *SecurityCtx) resolvePermissions() {
	if ctx.resolvedApiSession == nil || ctx.resolvedIdentity == nil {
		return
	}

	ctx.resolvePermissionsOnce.Do(func() {
		if ctx.isFullyAuthed() {
			ctx.resolvedPermissions[permissions.AuthenticatedPermission] = struct{}{}

			if ctx.resolvedIdentity.IsAdmin || ctx.resolvedIdentity.IsDefaultAdmin {
				ctx.resolvedPermissions[permissions.AdminPermission] = struct{}{}
			}
		} else if ctx.isPartiallyAuthed() {
			ctx.resolvedPermissions[permissions.PartiallyAuthenticatePermission] = struct{}{}
		}

		for _, permission := range ctx.resolvedIdentity.Permissions {
			ctx.resolvedPermissions[permission] = struct{}{}
		}
	})
}

// GetPermissions returns the set of permission strings granted to the session, such as
// "authenticated", "partiallyAuthenticated", and "admin". The map is populated during
// resolution and is safe to read after any of the Get* methods have been called.
func (ctx *SecurityCtx) GetPermissions() map[string]struct{} {
	ctx.resolve()
	return ctx.resolvedPermissions
}

// AddToRequest stores this SecurityCtx in the request's context under common.SecurityCtxKey
// so that route handlers can retrieve it without needing to re-resolve authentication.
func (ctx *SecurityCtx) AddToRequest(r *http.Request) {
	*r = *r.WithContext(context.WithValue(r.Context(), common.SecurityCtxKey, ctx))
}
