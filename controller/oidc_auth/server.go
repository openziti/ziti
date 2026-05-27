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
