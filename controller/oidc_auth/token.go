package oidc_auth

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/zitadel/oidc/v2/pkg/oidc"
	"time"
)

const (
	ClaimAudienceOpenZiti = "openziti"

	CustomClaimApiSessionId      = "z_asid"
	CustomClaimExternalId        = "z_eid"
	CustomClaimIsAdmin           = "z_ia"
	CustomClaimsConfigTypes      = "z_ct"
	CustomClaimsCertFingerprints = "z_cfs"

	DefaultAccessTokenDuration  = 30 * time.Minute
	DefaultIdTokenDuration      = 30 * time.Minute
	DefaultRefreshTokenDuration = 24 * time.Hour

	TokenTypeAccess  = "a"
	TokenTypeRefresh = "r"
)

type CustomClaims struct {
	ApiSessionId     string   `json:"z_asid,omitempty"`
	ExternalId       string   `json:"z_eid,omitempty"`
	IsAdmin          bool     `json:"z_ia,omitempty"`
	ConfigTypes      []string `json:"z_ct,omitempty"`
	ApplicationId    string   `json:"z_aid,omitempty"`
	Type             string   `json:"z_t"`
	CertFingerprints []string `json:"z_cfs"`
	Scopes           []string `json:"scopes,omitempty"`
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
	return r.TokenClaims.Issuer, nil
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

type AccessClaims struct {
	oidc.AccessTokenClaims
	CustomClaims
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
	return r.TokenClaims.Issuer, nil
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

type IdTokenClaims struct {
	oidc.IDTokenClaims
	CustomClaims
}

func (c *IdTokenClaims) TotpComplete() bool {
	for _, amr := range c.AuthenticationMethodsReferences {
		if amr == "totp" {
			return true
		}
	}

	return false
}
