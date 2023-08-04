package oidc_auth

import (
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/zitadel/oidc/v2/pkg/oidc"
)

const (
	ScopeTokenId      = "tid-"
	ScopeApiSessionId = "asid-"
)

// AuthRequest represents an OIDC authentication request and implements op.AuthRequest
type AuthRequest struct {
	oidc.AuthRequest
	Id                      string
	CreationDate            time.Time
	IdentityId              string
	AuthTime                time.Time
	ApiSessionId            string
	SecondaryTotpRequired   bool
	SecondaryExtJwtRequired bool
	SecondaryExtJwtId       string
	ConfigTypes             []string
	Amr                     map[string]struct{}

	PeerCerts           []*x509.Certificate
	RequestedMethod     string
	BearerTokenDetected bool
}

// GetID returns an AuthRequest's ID and implements op.AuthRequest
func (a *AuthRequest) GetID() string {
	return a.Id
}

// GetACR returns the authentication class reference provided by client and implements oidc.AuthRequest
// All ACRs are currently ignored.
func (a *AuthRequest) GetACR() string {
	return ""
}

// GetAMR returns the authentication method references the authentication has undergone and implements op.AuthRequest
func (a *AuthRequest) GetAMR() []string {
	result := make([]string, len(a.Amr))
	i := 0
	for k := range a.Amr {
		result[i] = k
		i = i + 1
	}
	return result
}

// HasFullAuth returns true if an authentication request has passed all primary and secondary authentications.
func (a *AuthRequest) HasFullAuth() bool {
	return a.HasPrimaryAuth() && a.HasSecondaryAuth()
}

// HasPrimaryAuth returns true if a primary authentication mechanism has been passed.
func (a *AuthRequest) HasPrimaryAuth() bool {
	return a.HasAmr(AuthMethodCert) || a.HasAmr(AuthMethodPassword) || a.HasAmr(AuthMethodExtJwt)
}

// HasSecondaryAuth returns true if all applicable secondary authentications have been passed
func (a *AuthRequest) HasSecondaryAuth() bool {
	return (!a.SecondaryTotpRequired || a.HasAmr(AuthMethodSecondaryTotp)) &&
		(!a.SecondaryExtJwtRequired || a.HasAmr(AuthMethodSecondaryExtJwt))
}

// HasAmr returns true if the supplied amr is present
func (a *AuthRequest) HasAmr(amr string) bool {
	_, found := a.Amr[amr]
	return found
}

// AddAmr adds the supplied amr
func (a *AuthRequest) AddAmr(amr string) {
	if a.Amr == nil {
		a.Amr = map[string]struct{}{}
	}
	a.Amr[amr] = struct{}{}
}

// GetAudience returns all current audience targets and implements op.AuthRequest
func (a *AuthRequest) GetAudience() []string {
	return []string{a.ClientID, ClaimAudienceOpenZiti}
}

// GetAuthTime returns the time at which authentication has occurred and implements op.AuthRequest
func (a *AuthRequest) GetAuthTime() time.Time {
	return a.AuthTime
}

// GetClientID returns the client id requested and implements op.AuthRequest
func (a *AuthRequest) GetClientID() string {
	return a.ClientID
}

// GetCodeChallenge returns the rp supplied code change and implements op.AuthRequest
func (a *AuthRequest) GetCodeChallenge() *oidc.CodeChallenge {
	return &oidc.CodeChallenge{
		Challenge: a.CodeChallenge,
		Method:    a.CodeChallengeMethod,
	}
}

// GetNonce returns the rp supplied nonce and implements op.AuthRequest
func (a *AuthRequest) GetNonce() string {
	return a.Nonce
}

// GetRedirectURI returns the rp supplied redirect target and implements op.AuthRequest
func (a *AuthRequest) GetRedirectURI() string {
	return a.RedirectURI
}

// GetResponseType returns the rp supplied response type and implements op.AuthRequest
func (a *AuthRequest) GetResponseType() oidc.ResponseType {
	return a.ResponseType
}

// GetResponseMode is not supported and all tokens are turned via query string and implements op.AuthRequest
func (a *AuthRequest) GetResponseMode() oidc.ResponseMode {
	return ""
}

// GetScopes returns the current scopes and implements op.AuthRequest
// Scopes are also used to transport custom claims into access tokens.
// The zitadel oidc framework does not provide a method for accessing the request object during JWT signing time,
// and any claims supplied are overwritten.
func (a *AuthRequest) GetScopes() []string {
	result := append(a.Scopes, ScopeApiSessionId+a.ApiSessionId)
	result = append(result, ScopeTokenId+a.Id)
	return result
}

// GetState returns the rp provided state and implements op.AuthRequest
func (a *AuthRequest) GetState() string {
	return a.State
}

// GetSubject returns the target subject and implements op.AuthRequest
func (a *AuthRequest) GetSubject() string {
	return a.IdentityId
}

// Done returns true once authentication has been completed and implements op.AuthRequest
func (a *AuthRequest) Done() bool {
	return a.HasFullAuth()
}

func (a *AuthRequest) GetCertFingerprints() []string {
	var prints []string

	for _, cert := range a.PeerCerts {
		prints = append(prints, fmt.Sprintf("%s", sha1.Sum(cert.Raw)))
	}

	return prints
}

// RefreshTokenRequest is a wrapper around RefreshClaims to avoid collisions between go-jwt interface requirements and
// zitadel oidc interface names. Implements zitadel op.RefreshTokenRequest
type RefreshTokenRequest struct {
	RefreshClaims
}

// GetAMR implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) GetAMR() []string {
	return r.AuthenticationMethodsReferences
}

// GetAudience implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) GetAudience() []string {
	return r.Audience
}

// GetAuthTime implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) GetAuthTime() time.Time {
	return r.AuthTime.AsTime()
}

// GetClientID implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) GetClientID() string {
	return r.ClientID
}

// GetScopes implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) GetScopes() []string {
	return r.Scopes
}

// GetSubject implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) GetSubject() string {
	return r.Subject
}

// SetCurrentScopes implements op.RefreshTokenRequest
func (r *RefreshTokenRequest) SetCurrentScopes(scopes []string) {
	r.Scopes = scopes
}

func (r *RefreshTokenRequest) GetCertFingerprints() []string {
	return r.CertFingerprints
}
