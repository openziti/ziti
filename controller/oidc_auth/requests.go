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
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/controller/model"
	"time"

	"github.com/zitadel/oidc/v2/pkg/oidc"
)

// AuthRequest represents an OIDC authentication request and implements op.AuthRequest
type AuthRequest struct {
	oidc.AuthRequest
	Id                    string
	CreationDate          time.Time
	IdentityId            string
	AuthTime              time.Time
	ApiSessionId          string
	SecondaryTotpRequired bool
	SecondaryExtJwtSigner *model.ExternalJwtSigner
	ConfigTypes           []string
	Amr                   map[string]struct{}

	PeerCerts           []*x509.Certificate
	RequestedMethod     string
	BearerTokenDetected bool
	SdkInfo             *rest_model.SdkInfo
	EnvInfo             *rest_model.EnvInfo
	RemoteAddress       string
	IsCertExtendable    bool
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
		(a.SecondaryExtJwtSigner == nil || a.HasAmrExtJwtId(a.SecondaryExtJwtSigner.Id))
}

// HasAmr returns true if the supplied amr is present
func (a *AuthRequest) HasAmr(amr string) bool {
	_, found := a.Amr[amr]
	return found
}

func (a *AuthRequest) HasAmrExtJwtId(id string) bool {
	return a.HasAmr(AuthMethodSecondaryExtJwt + ":" + id)
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
	return []string{a.ClientID}
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
func (a *AuthRequest) GetScopes() []string {
	return a.Scopes
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
		prints = append(prints, fmt.Sprintf("%x", sha1.Sum(cert.Raw)))
	}

	return prints
}

func (a *AuthRequest) NeedsTotp() bool {
	return a.SecondaryTotpRequired && !a.HasAmr(AuthMethodSecondaryTotp)
}

func (a *AuthRequest) NeedsSecondaryExtJwt() bool {
	return a.SecondaryExtJwtSigner != nil && !a.HasAmrExtJwtId(a.SecondaryExtJwtSigner.Id)
}

func (a *AuthRequest) GetAuthQueries() []*rest_model.AuthQueryDetail {
	var authQueries []*rest_model.AuthQueryDetail

	if a.NeedsTotp() {
		provider := rest_model.MfaProvidersZiti
		authQueries = append(authQueries, &rest_model.AuthQueryDetail{
			Format:     rest_model.MfaFormatsNumeric,
			HTTPMethod: "POST",
			HTTPURL:    "./oidc/login/totp",
			MaxLength:  8,
			MinLength:  6,
			Provider:   &provider,
			TypeID:     rest_model.AuthQueryTypeTOTP,
		})
	}

	if a.NeedsSecondaryExtJwt() {
		provider := rest_model.MfaProvidersURL
		authQueries = append(authQueries, &rest_model.AuthQueryDetail{
			ClientID: stringz.OrEmpty(a.SecondaryExtJwtSigner.ClientId),
			HTTPURL:  stringz.OrEmpty(a.SecondaryExtJwtSigner.ExternalAuthUrl),
			Scopes:   a.SecondaryExtJwtSigner.Scopes,
			Provider: &provider,
			ID:       a.SecondaryExtJwtSigner.Id,
			TypeID:   rest_model.AuthQueryTypeEXTDashJWT,
		})
	}

	return authQueries
}

// RefreshTokenRequest is a wrapper around RefreshClaims to avoid collisions between go-jwt interface requirements and
// zitadel oidc interface names. Implements zitadel op.RefreshTokenRequest
type RefreshTokenRequest struct {
	common.RefreshClaims
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
