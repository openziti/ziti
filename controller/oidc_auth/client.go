package oidc_auth

import (
	"time"

	"github.com/zitadel/oidc/v2/pkg/oidc"
	"github.com/zitadel/oidc/v2/pkg/op"
)

// Client represents an OIDC Client and implements op.Client
type Client struct {
	id                             string
	secret                         string
	redirectURIs                   []string
	postLogoutRedirectURIs         []string
	applicationType                op.ApplicationType
	authMethod                     oidc.AuthMethod
	loginURL                       func(string) string
	responseTypes                  []oidc.ResponseType
	grantTypes                     []oidc.GrantType
	accessTokenType                op.AccessTokenType
	devMode                        bool
	idTokenUserinfoClaimsAssertion bool
	clockSkew                      time.Duration
	idTokenDuration                time.Duration
}

// GetID returns the clients id, implements op.Client
func (c *Client) GetID() string {
	return c.id
}

// RedirectURIs returns an array of valid redirect URIs, implements op.Client
func (c *Client) RedirectURIs() []string {
	return c.redirectURIs
}

// PostLogoutRedirectURIs returns an array of post logout redirect URIs, implements op.Client
func (c *Client) PostLogoutRedirectURIs() []string {
	return c.postLogoutRedirectURIs
}

// ApplicationType returns the application type (app, native, user agent), implements op.Client
func (c *Client) ApplicationType() op.ApplicationType {
	return c.applicationType
}

// AuthMethod returns the authentication method (client_secret_basic, client_secret_post, none, private_key_jwt), implements op.Client
func (c *Client) AuthMethod() oidc.AuthMethod {
	return c.authMethod
}

// ResponseTypes returns all allowed response types (code, id_token token, id_token), these must match with the allowed grant types, implements op.Client
func (c *Client) ResponseTypes() []oidc.ResponseType {
	return c.responseTypes
}

// GrantTypes returns all allowed grant types (authorization_code, refresh_token, urn:ietf:params:oauth:grant-type:jwt-bearer), implements op.Client
func (c *Client) GrantTypes() []oidc.GrantType {
	return c.grantTypes
}

// LoginURL returns the URL clients should be directed to for login based on authentication request id,
// implements op.Client
func (c *Client) LoginURL(id string) string {
	return c.loginURL(id)
}

// AccessTokenType returns the type of access token the client uses (Bearer (opaque) or JWT), implements op.Client
func (c *Client) AccessTokenType() op.AccessTokenType {
	return c.accessTokenType
}

// IDTokenLifetime returns the lifetime of the client's id_tokens
func (c *Client) IDTokenLifetime() time.Duration {
	return c.idTokenDuration
}

// DevMode enables the use of non-compliant configs such as redirect_uris, implements op.Client
func (c *Client) DevMode() bool {
	return c.devMode
}

// RestrictAdditionalIdTokenScopes allows specifying which custom scopes shall be asserted into the id_token, implements op.Client
func (c *Client) RestrictAdditionalIdTokenScopes() func(scopes []string) []string {
	return func(scopes []string) []string {
		return scopes
	}
}

// RestrictAdditionalAccessTokenScopes allows specifying which custom scopes shall be asserted into the JWT access_token, implements op.Client
func (c *Client) RestrictAdditionalAccessTokenScopes() func(scopes []string) []string {
	return func(scopes []string) []string {
		return scopes
	}
}

// IsScopeAllowed enables Client custom scopes validation, implements op.Client
// No custom scopes are currently supported.
func (c *Client) IsScopeAllowed(_ string) bool {
	return false
}

// IDTokenUserinfoClaimsAssertion allows specifying if claims of scope profile, email, phone and address are asserted into the id_token
// even if an access token if issued which violates the OIDC Core spec
// (5.4. Requesting Claims using Scope Values: https://openid.net/specs/openid-connect-core-1_0.html#ScopeClaims)
// some clients though require that e.g. email is always in the id_token when requested even if an access_token is issued, implements op.Client
func (c *Client) IDTokenUserinfoClaimsAssertion() bool {
	return c.idTokenUserinfoClaimsAssertion
}

// ClockSkew enables clients to instruct the OP to apply a clock skew on the various times and expirations
// (subtract from issued_at, add to expiration, ...), implements op.Client
func (c *Client) ClockSkew() time.Duration {
	return c.clockSkew
}

// NativeClient will create a client of type native, which will always use PKCE and allow the use of refresh tokens
func NativeClient(id string, redirectURIs, postlogoutURIs []string) *Client {
	return &Client{
		id:                             id,
		secret:                         "", //rely on PKCE
		redirectURIs:                   redirectURIs,
		postLogoutRedirectURIs:         postlogoutURIs,
		applicationType:                op.ApplicationTypeNative,
		authMethod:                     oidc.AuthMethodNone,
		responseTypes:                  []oidc.ResponseType{oidc.ResponseTypeCode},
		grantTypes:                     []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken},
		accessTokenType:                op.AccessTokenTypeJWT,
		devMode:                        false,
		idTokenUserinfoClaimsAssertion: false,
		clockSkew:                      0,
		idTokenDuration:                1 * time.Hour,
	}
}
