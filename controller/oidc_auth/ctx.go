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

package oidc_auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// contextKey is a private type used to restrict context value access
type contextKey string

// contextKeyHttpRequest is the key value to retrieve the current http.Request from a context
const contextKeyHttpRequest contextKey = "oidc_request"
const contextKeyTokenState contextKey = "oidc_token_state"

// NewChangeCtx creates a change.Context scoped to oidc_auth package
func NewChangeCtx() *change.Context {
	ctx := change.New()

	ctx.SetSourceType(SourceTypeOidc).
		SetChangeAuthorType(change.AuthorTypeController)

	return ctx
}

// NewHttpChangeCtx creates a change.Context scoped to oidc_auth package and supplied http.Request
func NewHttpChangeCtx(r *http.Request) *change.Context {
	ctx := NewChangeCtx()

	ctx.SetSourceLocal(r.Host).
		SetSourceRemote(r.RemoteAddr).
		SetSourceMethod(r.Method)

	return ctx
}

// TokenState carries per-request token state through the OIDC context. It is created at the
// start of each OIDC HTTP request (see provider.go) and is shared between the zitadel library
// callbacks and our server method overrides.
//
// The zitadel library's token creation flow works in two phases:
//  1. Storage callbacks (CreateAccessToken / CreateAccessAndRefreshTokens) build the claim data
//     and store it on AccessClaims / RefreshClaims.
//  2. Library functions (CreateJWT, CreateIDToken) read back those claims via
//     GetPrivateClaimsFromScopes and SetUserinfoFromRequest to produce the final JWTs.
//
// TokenState bridges these phases. It also carries CSR data submitted at the token endpoint
// through to createAccessToken (where the signing happens), and carries the resulting
// certificate PEM back out to the server method override (where it is added to the JSON
// response).
type TokenState struct {
	// AccessClaims holds the custom claims produced by createAccessToken. Read by
	// getPrivateClaims to populate the access token JWT.
	AccessClaims *common.AccessClaims

	// RefreshClaims holds the refresh token claims produced alongside the access token.
	RefreshClaims *common.RefreshClaims

	// CsrPem holds an optional CSR submitted as a form parameter on the token endpoint
	// (refresh or token exchange). Read by createAccessToken to sign and produce a
	// session-bound certificate. For the initial code exchange path, the CSR is carried
	// on the AuthRequest instead.
	CsrPem string

	// SessionCertPem holds the PEM certificate chain produced by signing a CSR. Set by
	// createAccessToken and read by the server method overrides (CodeExchange, RefreshToken,
	// TokenExchange) to include as a top-level "session_cert" field in the token endpoint
	// JSON response.
	SessionCertPem string
}

func TokenStateFromContext(ctx context.Context) (*TokenState, error) {
	val := ctx.Value(contextKeyTokenState)

	if val == nil {
		srvErr := oidc.ErrServerError()
		srvErr.Description = "token state context was nil"
		return nil, srvErr
	}

	tokenState := val.(*TokenState)

	if tokenState == nil {
		srvErr := oidc.ErrServerError()
		srvErr.Description = fmt.Sprintf("could not cast token state context value from %T to %T", val, tokenState)
		return nil, srvErr
	}

	return tokenState, nil
}

// HttpRequestFromContext returns the initiating http.Request for the current OIDC context
func HttpRequestFromContext(ctx context.Context) (*http.Request, error) {
	httpVal := ctx.Value(contextKeyHttpRequest)

	if httpVal == nil {
		return nil, oidc.ErrServerError()
	}

	httpRequest := httpVal.(*http.Request)

	if httpRequest == nil {
		return nil, oidc.ErrServerError()
	}

	return httpRequest, nil
}
