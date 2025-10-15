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

package common

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

const (
	ClaimClientIdOpenZiti = "openziti"
	ClaimAudienceOpenZiti = "openziti"

	//ClaimLegacyNative - to remove after SDKs stop using this as a client id
	ClaimLegacyNative = "native"

	CustomClaimApiSessionId      = "z_asid"
	CustomClaimExternalId        = "z_eid"
	CustomClaimIsAdmin           = "z_ia"
	CustomClaimsConfigTypes      = "z_ct"
	CustomClaimsCertFingerprints = "z_cfs"
	CustomClaimsAuthenticatorId  = "z_authid"

	// CustomClaimsTokenType and other constants below may not appear as referenced, but are used in `json: ""` tags. Provided here for external use.
	CustomClaimsTokenType       = "z_t"
	CustomClaimServiceId        = "z_sid"
	CustomClaimIdentityId       = "z_iid"
	CustomClaimServiceType      = "z_st"
	CustomClaimRemoteAddress    = "z_ra"
	CustomClaimIsCertExtendable = "z_ice"
	CustomClaimImproperCert     = "z_iccc"
	CustomClaimIsLegacy         = "z_leg"

	DefaultAccessTokenDuration  = 30 * time.Minute
	DefaultIdTokenDuration      = 30 * time.Minute
	DefaultRefreshTokenDuration = 24 * time.Hour

	TokenTypeAccess        = "a"
	TokenTypeRefresh       = "r"
	TokenTypeServiceAccess = "s"
	TokenTypeTotp          = "t"

	ServiceSessionTypeBind = "Bind"
	ServiceSessionTypeDial = "Dial"
)

type CustomClaims struct {
	ApiSessionId            string              `json:"z_asid,omitempty"`
	ExternalId              string              `json:"z_eid,omitempty"`
	IsAdmin                 bool                `json:"z_ia,omitempty"`
	ConfigTypes             []string            `json:"z_ct,omitempty"`
	ApplicationId           string              `json:"z_aid,omitempty"`
	Type                    string              `json:"z_t"`
	CertFingerprints        []string            `json:"z_cfs"`
	Scopes                  []string            `json:"scopes,omitempty"`
	SdkInfo                 *rest_model.SdkInfo `json:"z_sdk"`
	EnvInfo                 *rest_model.EnvInfo `json:"z_env"`
	RemoteAddress           string              `json:"z_ra"`
	IsCertExtendable        bool                `json:"z_ice"`
	AuthenticatorId         string              `json:"z_authid,omitempty"`
	IsCertExtendRequested   bool                `json:"z_cer"`
	IsCertKeyRollRequested  bool                `json:"z_ckrr"`
	ImproperClientCertChain bool                `json:"z_iccc"`
}

func (c *CustomClaims) ToMap() (map[string]any, error) {
	out := map[string]any{}
	str, err := json.Marshal(c)

	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(str, &out)

	if err != nil {
		return nil, err
	}

	return out, nil
}

type RefreshClaims struct {
	oidc.IDTokenClaims
	CustomClaims
}

func (r *RefreshClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: r.TokenClaims.GetExpiration()}, nil
}

func (r *RefreshClaims) GetNotBefore() (*jwt.NumericDate, error) {
	notBefore := r.TokenClaims.NotBefore.AsTime()
	return &jwt.NumericDate{Time: notBefore}, nil
}

func (r *RefreshClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: r.TokenClaims.GetIssuedAt()}, nil
}

func (r *RefreshClaims) GetIssuer() (string, error) {
	return r.TokenClaims.Issuer, nil
}

func (r *RefreshClaims) GetSubject() (string, error) {
	return r.TokenClaims.Subject, nil
}

func (r *RefreshClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings(r.TokenClaims.Audience), nil
}

func (c *RefreshClaims) MarshalJSON() ([]byte, error) {
	var customBuf, idBuf []byte
	var err error

	if idBuf, err = json.Marshal(c.IDTokenClaims); err != nil {
		return nil, fmt.Errorf("refresh token oidc claims marhsalling error: %w", err)
	}

	if customBuf, err = json.Marshal(c.CustomClaims); err != nil {
		return nil, fmt.Errorf("refresh token custom claims marhsalling error: %w", err)
	}

	mergeMap := map[string]any{}

	_ = json.Unmarshal(idBuf, &mergeMap)
	_ = json.Unmarshal(customBuf, &mergeMap)

	return json.Marshal(mergeMap)
}

func (c *RefreshClaims) UnmarshalJSON(data []byte) error {
	var err error

	if err = json.Unmarshal(data, &c.IDTokenClaims); err != nil {
		return fmt.Errorf("refresh token oidc claims unmarhsalling error: %w", err)
	}

	if err = json.Unmarshal(data, &c.CustomClaims); err != nil {
		return fmt.Errorf("refresh token custom claims unmarhsalling error: %w", err)
	}
	return nil
}

type ServiceAccessClaims struct {
	jwt.RegisteredClaims

	// ApiSessionId is the id of the parent api session
	ApiSessionId string `json:"z_asid"`

	// IdentityId is the id of the associated identity
	IdentityId string `json:"z_iid"`

	// TokenType denotes the overall token type, which is a token that denotes access to a service for dial/bind
	TokenType string `json:"z_t"`

	// Type is either "Dial" or "Bind"
	Type string `json:"z_st"`

	// IsLegacy denotes that this token was generated from a legacy-authenticated API Session and thus has durable
	// storage associated with it. This alters how systems interact with the token and ensures legacy considerations
	// are taken into account (storage cleanup, token value interpretation, etc.)
	IsLegacy bool `json:"z_leg"`
}

func (c *ServiceAccessClaims) HasAudience(targetAud string) bool {
	for _, aud := range c.Audience {
		if aud == targetAud {
			return true
		}
	}
	return false
}

type AccessClaims struct {
	oidc.AccessTokenClaims
	CustomClaims
}

func (r *AccessClaims) ConfigTypesAsMap() map[string]struct{} {
	result := map[string]struct{}{}

	for _, configType := range r.ConfigTypes {
		result[configType] = struct{}{}
	}

	return result
}

func (r *AccessClaims) UnmarshalJSON(raw []byte) error {
	err := json.Unmarshal(raw, &r.AccessTokenClaims)
	if err != nil {
		return err
	}

	err = json.Unmarshal(raw, &r.CustomClaims)

	return err
}

func (r *AccessClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: r.TokenClaims.GetExpiration()}, nil
}

func (r *AccessClaims) GetNotBefore() (*jwt.NumericDate, error) {
	notBefore := r.TokenClaims.NotBefore.AsTime()
	return &jwt.NumericDate{Time: notBefore}, nil
}

func (r *AccessClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: r.TokenClaims.GetIssuedAt()}, nil
}

func (r *AccessClaims) GetIssuer() (string, error) {
	return r.TokenClaims.Issuer, nil
}

func (r *AccessClaims) GetSubject() (string, error) {
	return r.TokenClaims.Subject, nil
}

func (r *AccessClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings(r.TokenClaims.Audience), nil
}

func (c *AccessClaims) TotpComplete() bool {
	for _, amr := range c.AuthenticationMethodsReferences {
		if amr == "totp" {
			return true
		}
	}

	return false
}

func (c *AccessClaims) HasAudience(targetAud string) bool {
	for _, aud := range c.Audience {
		if aud == targetAud {
			return true
		}
	}
	return false
}

type IdTokenClaims struct {
	oidc.IDTokenClaims
	CustomClaims
}

func (r *IdTokenClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: r.TokenClaims.GetExpiration()}, nil
}

func (r *IdTokenClaims) GetNotBefore() (*jwt.NumericDate, error) {
	notBefore := r.TokenClaims.NotBefore.AsTime()
	return &jwt.NumericDate{Time: notBefore}, nil
}

func (r *IdTokenClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: r.TokenClaims.GetIssuedAt()}, nil
}

func (r *IdTokenClaims) GetIssuer() (string, error) {
	return r.TokenClaims.Issuer, nil
}

func (r *IdTokenClaims) GetSubject() (string, error) {
	return r.TokenClaims.Subject, nil
}

func (r *IdTokenClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings(r.TokenClaims.Audience), nil
}

func (c *IdTokenClaims) TotpComplete() bool {
	for _, amr := range c.AuthenticationMethodsReferences {
		if amr == "totp" {
			return true
		}
	}

	return false
}

// TotpClaims is a set of claims used to define TOTP JWT tokens that signify the last time a client
// has successfully performed a TOTP code submission. They have no expiration date, but are tied to
// and API Session via the ApiSessionId/z_asid claim. A valid, unexpired Api Session token is required
// to be used in conjunction with a TOTP token - making the TOTP token scoped to the API Session's expiration and
// validity.
//
// As these are expected for HA systems only, they do require OIDC authentication to be issued and can
// be obtained via the /[edge|management]/v1/tokens/totp endpoint. Ensure your current API Session bearer
// token is valid and is set in the authorization header of the request.
//
// Claims:
//   - z_asid: the id of the api session that the token is scoped to
//   - z_t: the type of token, always "t"
//   - sub: the identity id of the identity that the token is scoped to
//   - iss: the controller that issued the token
//   - issued_at: the time the token was issued, also indicated when the TOTP code was submitted and verified
type TotpClaims struct {
	jwt.RegisteredClaims
	ApiSessionId string `json:"z_asid,omitempty"`
	Type         string `json:"z_t"`
}

func (t *TotpClaims) HasAudience(targetAud string) bool {
	for _, aud := range t.Audience {
		if aud == targetAud {
			return true
		}
	}
	return false
}
