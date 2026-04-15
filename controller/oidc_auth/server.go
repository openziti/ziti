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

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// openZitiDiscoveryConfiguration extends the standard OIDC discovery response with
// a vendor-specific "openziti_endpoints" field that advertises OpenZiti's custom
// login and MFA endpoints. This allows SDKs to discover endpoint URLs at runtime
// instead of hardcoding paths.
type openZitiDiscoveryConfiguration struct {
	*oidc.DiscoveryConfiguration
	OpenZitiEndpoints openZitiEndpoints `json:"openziti_endpoints"`
}

// openZitiEndpoints contains the URLs for OpenZiti-specific OIDC endpoints.
type openZitiEndpoints struct {
	// Password is the URL for username/password authentication.
	Password string `json:"password"`

	// Cert is the URL for client certificate authentication.
	Cert string `json:"cert"`

	// ExtJwt is the URL for external JWT authentication.
	ExtJwt string `json:"ext_jwt"`

	// Totp is the URL where a TOTP code is submitted for MFA verification.
	Totp string `json:"totp"`

	// TotpEnroll is the URL for starting (POST) or deleting (DELETE) TOTP enrollment.
	TotpEnroll string `json:"totp_enroll"`

	// TotpEnrollVerify is the URL for verifying a TOTP enrollment code.
	TotpEnrollVerify string `json:"totp_enroll_verify"`

	// AuthQueries is the URL for retrieving pending authentication queries.
	AuthQueries string `json:"auth_queries"`
}

// server embeds op.LegacyServer and overrides methods where the library's
// helper functions re-wrap storage errors with empty descriptions, discarding
// the error_description that storage set. The overrides call storage directly
// and pass errors through so that WriteError serializes the original description.
type server struct {
	*op.LegacyServer
}

var _ op.ExtendedLegacyServer = (*server)(nil)

func newServer(provider op.OpenIDProvider, endpoints op.Endpoints) *server {
	return &server{
		LegacyServer: op.NewLegacyServer(provider, endpoints),
	}
}

// Discovery returns the OpenID Provider Configuration with OpenZiti-specific endpoint
// extensions. It builds the standard OIDC discovery configuration, then wraps it with
// vendor-specific fields under "openziti_endpoints".
func (s *server) Discovery(ctx context.Context, r *op.Request[struct{}]) (*op.Response, error) {
	config := op.CreateDiscoveryConfig(ctx, s.Provider(), s.Provider().Storage())
	issuer := op.IssuerFromContext(ctx)

	return op.NewResponse(&openZitiDiscoveryConfiguration{
		DiscoveryConfiguration: config,
		OpenZitiEndpoints: openZitiEndpoints{
			Password:         issuer + "/login/password",
			Cert:             issuer + "/login/cert",
			ExtJwt:           issuer + "/login/ext-jwt",
			Totp:             issuer + "/login/totp",
			TotpEnroll:       issuer + "/login/totp/enroll",
			TotpEnrollVerify: issuer + "/login/totp/enroll/verify",
			AuthQueries:      issuer + "/login/auth-queries",
		},
	}), nil
}

// VerifyClient authenticates the client for a token request. It overrides
// LegacyServer.VerifyClient to pass through storage errors from
// GetClientByClientID without re-wrapping them in a new oidc.ErrInvalidClient
// that has an empty description.
func (s *server) VerifyClient(ctx context.Context, r *op.Request[op.ClientCredentials]) (op.Client, error) {
	if oidc.GrantType(r.Form.Get("grant_type")) == oidc.GrantTypeClientCredentials {
		storage, ok := s.Provider().Storage().(op.ClientCredentialsStorage)
		if !ok {
			return nil, oidc.ErrUnsupportedGrantType().WithDescription("client_credentials grant not supported")
		}
		return storage.ClientCredentials(ctx, r.Data.ClientID, r.Data.ClientSecret)
	}

	if r.Data.ClientAssertionType == oidc.ClientAssertionTypeJWTAssertion {
		jwtExchanger, ok := s.Provider().(op.JWTAuthorizationGrantExchanger)
		if !ok || !s.Provider().AuthMethodPrivateKeyJWTSupported() {
			return nil, oidc.ErrInvalidClient().WithDescription("auth_method private_key_jwt not supported")
		}
		return op.AuthorizePrivateJWTKey(ctx, r.Data.ClientAssertion, jwtExchanger)
	}

	client, err := s.Provider().Storage().GetClientByClientID(ctx, r.Data.ClientID)
	if err != nil {
		return nil, err
	}

	switch client.AuthMethod() {
	case oidc.AuthMethodNone:
		return client, nil
	case oidc.AuthMethodPrivateKeyJWT:
		return nil, oidc.ErrInvalidClient().WithDescription("private_key_jwt not allowed for this client")
	case oidc.AuthMethodPost:
		if !s.Provider().AuthMethodPostSupported() {
			return nil, oidc.ErrInvalidClient().WithDescription("auth_method post not supported")
		}
	}

	err = op.AuthorizeClientIDSecret(ctx, r.Data.ClientID, r.Data.ClientSecret, s.Provider().Storage())
	if err != nil {
		return nil, err
	}

	return client, nil
}

// RefreshToken handles refresh token requests. It overrides
// LegacyServer.RefreshToken to call storage.TokenRequestByRefreshToken
// directly instead of through the library's RefreshTokenRequestByRefreshToken
// helper, which re-wraps errors in a new oidc.ErrInvalidGrant with an empty
// description.
func (s *server) RefreshToken(ctx context.Context, r *op.ClientRequest[oidc.RefreshTokenRequest]) (*op.Response, error) {
	if !s.Provider().GrantTypeRefreshTokenSupported() {
		return nil, oidc.ErrInvalidRequest().WithDescription("grant_type refresh_token not supported")
	}

	request, err := s.Provider().Storage().TokenRequestByRefreshToken(ctx, r.Data.RefreshToken)
	if err != nil {
		return nil, err
	}

	if r.Client.GetID() != request.GetClientID() {
		return nil, oidc.ErrInvalidGrant().WithDescription("client_id does not match original grant")
	}

	if err = op.ValidateRefreshTokenScopes(r.Data.Scopes, request); err != nil {
		return nil, err
	}

	resp, err := op.CreateTokenResponse(ctx, request, r.Client, s.Provider(), true, "", r.Data.RefreshToken)
	if err != nil {
		return nil, err
	}

	return op.NewResponse(resp), nil
}
