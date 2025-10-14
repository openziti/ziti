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
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/jwtsigner"

	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/google/uuid"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

var (
	_ op.Storage                  = &HybridStorage{}
	_ op.ClientCredentialsStorage = &HybridStorage{}
)

const JwtTokenPrefix = "ey"

// Storage is a compound interface of op.Storage and custom storage functions
type Storage interface {
	op.Storage

	// Authenticate attempts to perform authentication on supplied credentials for all known authentication methods
	Authenticate(authCtx model.AuthContext, id string, configTypes []string) (*AuthRequest, error)

	// VerifyTotpForAuthRequest will verify the supplied code for the current authentication request's subject
	// A change context is required for the removal of one-time TOTP recovery codes
	VerifyTotp(ctx *change.Context, code string, id string) (*AuthRequest, error)

	// StartTotpEnrollment will attempt to create an MFA record for the current in scope identity
	// if it can. If an MFA record already exists, it will fail, and CompleteTotpEnrollment or
	// DeleteTotpEnrollment must be used.
	StartTotpEnrollment(ctx *change.Context, authRequestId string) (*model.Mfa, error)

	// CompleteTotpEnrollment will validate the supplied code against the current unverified MFA record for
	// the identity in scope. If MFA enrollment hasn't been started, StartTotpEnrollment should be used.
	CompleteTotpEnrollment(ctx *change.Context, authRequestId, code string) error

	// DeleteTotpEnrollment will delete a current MFA record for the identity in scope. If the MFA record
	// has been verified, a code must be supplied that passes verification. If the
	// record has not been verified, deletion will occur without verifying the code value.
	DeleteTotpEnrollment(ctx *change.Context, authRequestId string, code string) error

	// IsTokenRevoked will return true if a token has been removed.
	// TokenId may be a JWT token id or an identity id
	IsTokenRevoked(tokenId string) bool

	// AddClient adds an OIDC Client to the registry of valid clients.
	AddClient(client *Client)

	// GetAuthRequest returns an *AuthRequest by its id
	GetAuthRequest(id string) (*AuthRequest, error)

	// Managers returns the model managers used to interact with durable storage
	Managers() *model.Managers

	// UpdateSdkEnvInfo will attempt to take the identity in scope of an AuthRequest and update its durable SDK/ENV
	// info.
	UpdateSdkEnvInfo(request *AuthRequest) error
}

func NewRevocation(tokenId string, expiresAt time.Time) *model.Revocation {
	return &model.Revocation{
		BaseEntity: models.BaseEntity{
			Id: tokenId,
		},
		ExpiresAt: expiresAt,
	}
}

// HybridStorage implements the Storage interface
// Authentication requests are not synchronized with other controllers. Authentication must happen entirely
// with one controller. After id, access, and/or refresh tokens are acquired, they may be used at any controller.
// All token revocations are synchronized with other controllers.
type HybridStorage struct {
	env        model.Env
	signingKey key

	authRequests cmap.ConcurrentMap[string, *AuthRequest] //authRequest.Id -> authRequest
	codes        cmap.ConcurrentMap[string, string]       //code -> authRequest.Id

	clients cmap.ConcurrentMap[string, *Client]

	deviceCodes  cmap.ConcurrentMap[string, deviceAuthorizationEntry]
	userCodes    cmap.ConcurrentMap[string, string]
	serviceUsers cmap.ConcurrentMap[string, *Client]

	startOnce sync.Once
	config    *Config

	keys cmap.ConcurrentMap[string, *pubKey]
}

func (s *HybridStorage) UpdateSdkEnvInfo(request *AuthRequest) error {
	identity, err := s.env.GetManagers().Identity.Read(request.IdentityId)

	if err != nil {
		return err
	}

	changeCtx := NewChangeCtx()

	var envInfo *model.EnvInfo
	var sdkInfo *model.SdkInfo

	if request.EnvInfo != nil {
		envInfo = &model.EnvInfo{
			Arch:      request.EnvInfo.Arch,
			Os:        request.EnvInfo.Os,
			OsRelease: request.EnvInfo.OsRelease,
			OsVersion: request.EnvInfo.OsVersion,
			Domain:    request.EnvInfo.Domain,
			Hostname:  request.EnvInfo.Hostname,
		}
	}

	if request.SdkInfo != nil {
		sdkInfo = &model.SdkInfo{
			AppId:      request.SdkInfo.AppID,
			AppVersion: request.SdkInfo.AppVersion,
			Branch:     request.SdkInfo.Branch,
			Revision:   request.SdkInfo.Revision,
			Type:       request.SdkInfo.Type,
			Version:    request.SdkInfo.Version,
		}
	}

	return s.env.GetManagers().Identity.UpdateSdkEnvInfo(identity, envInfo, sdkInfo, changeCtx)
}

func (s *HybridStorage) Managers() *model.Managers {
	return s.env.GetManagers()
}

func (s *HybridStorage) StartTotpEnrollment(changeCtx *change.Context, authRequestId string) (*model.Mfa, error) {
	authRequest, err := s.GetAuthRequest(authRequestId)

	if err != nil {
		return nil, errorz.NewUnauthorized()
	}

	id, err := s.env.GetManagers().Mfa.CreateForIdentityId(authRequest.IdentityId, changeCtx)

	if err != nil {
		return nil, err
	}

	return s.env.GetManagers().Mfa.Read(id)
}

func (s *HybridStorage) DeleteTotpEnrollment(changeCtx *change.Context, authRequestId string, code string) error {
	authRequest, err := s.GetAuthRequest(authRequestId)

	if err != nil {
		return errorz.NewUnauthorized()
	}

	mfaDetail, err := s.env.GetManagers().Mfa.ReadOneByIdentityId(authRequest.IdentityId)

	if err != nil {
		return err
	}

	if mfaDetail == nil {
		return errorz.NewNotFound()
	}

	if mfaDetail.IsVerified {
		ok, err := s.env.GetManagers().Mfa.Verify(mfaDetail, code, changeCtx)
		if err != nil {
			return err
		}

		if !ok {
			return apierror.NewInvalidMfaTokenError()
		}

		//ok fall through
	}

	return s.env.GetManagers().Mfa.Delete(mfaDetail.Id, changeCtx)
}

func (s *HybridStorage) CompleteTotpEnrollment(changeCtx *change.Context, authRequestId, code string) error {
	authRequest, err := s.GetAuthRequest(authRequestId)

	if err != nil {
		return errorz.NewUnauthorized()
	}

	return s.env.GetManagers().Mfa.CompleteTotpEnrollment(authRequest.IdentityId, code, changeCtx)
}

func (s *HybridStorage) AddClient(client *Client) {
	s.clients.Set(client.id, client)
}

var _ Storage = &HybridStorage{}

func NewStorage(rootSigner *jwtsigner.TlsJwtSigner, config *Config, env model.Env) *HybridStorage {
	store := &HybridStorage{
		env: env,
		signingKey: key{
			id:         rootSigner.KeyId(),
			algorithm:  jose.SignatureAlgorithm(rootSigner.SigningMethod().Alg()),
			privateKey: rootSigner.TlsCerts.PrivateKey,
			publicKey:  rootSigner.TlsCerts.Leaf.PublicKey,
		},
		authRequests: cmap.New[*AuthRequest](),
		codes:        cmap.New[string](),
		clients:      cmap.New[*Client](),
		deviceCodes:  cmap.New[deviceAuthorizationEntry](),
		userCodes:    cmap.New[string](),
		serviceUsers: cmap.New[*Client](),
		config:       config,
		keys:         cmap.New[*pubKey](),
	}

	store.start()
	return store
}

// start will run Clean every 10 seconds
func (s *HybridStorage) start() {
	s.startOnce.Do(func() {
		closeNotify := s.env.GetCloseNotifyChannel()
		ticker := time.NewTicker(10 * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					s.Clean()
				case <-closeNotify:
					ticker.Stop()
					return
				}
			}
		}()
	})
}

// Clean removes abandoned auth requests and associated data
func (s *HybridStorage) Clean() {
	var deleteKeys []string
	oldest := time.Now().Add(-10 * time.Minute)
	s.authRequests.IterCb(func(key string, v *AuthRequest) {
		if v.CreationDate.Before(oldest) {
			deleteKeys = append(deleteKeys, key)
		}
	})

	for _, key := range deleteKeys {
		s.authRequests.Remove(key)
	}

	//find associated codes and remove
	var deleteCodes []string

	s.codes.IterCb(func(code string, authId string) {
		if stringz.Contains(deleteKeys, authId) {
			deleteCodes = append(deleteCodes, code)
		}
	})

	for _, deleteCode := range deleteCodes {
		s.codes.Remove(deleteCode)
	}
}

// Authenticate will verify supplied credentials and update the primary authentication status of an AuthRequest
func (s *HybridStorage) Authenticate(authCtx model.AuthContext, id string, configTypes []string) (*AuthRequest, error) {
	authRequest, ok := s.authRequests.Get(id)

	if !ok {
		return nil, fmt.Errorf("request not found")
	}

	result, err := s.env.GetManagers().Authenticator.Authorize(authCtx)

	if err != nil {
		return nil, err
	}
	if !result.IsSuccessful() {
		return nil, apierror.NewInvalidAuth()
	}

	authRequest.IdentityId = result.Identity().Id
	authRequest.AddAmr(authCtx.GetMethod())

	authenticator := result.Authenticator()

	if authenticator != nil {
		if certAuth := authenticator.ToCert(); certAuth != nil {
			authRequest.IsCertExtendRequested = certAuth.IsExtendRequested
			authRequest.IsCertKeyRollRequested = certAuth.IsKeyRollRequested
		}
	}

	configTypeIds := s.env.GetManagers().ConfigType.MapConfigTypeNamesToIds(configTypes, authRequest.IdentityId)

	for configId := range configTypeIds {
		authRequest.ConfigTypes = append(authRequest.ConfigTypes, configId)
	}

	mfa, err := s.env.GetManagers().Mfa.ReadOneByIdentityId(authRequest.IdentityId)

	if err != nil {
		return nil, err
	}

	authRequest.IsTotpEnrolled = mfa != nil && mfa.IsVerified
	authRequest.SecondaryTotpRequired = authRequest.IsTotpEnrolled || result.AuthPolicy().Secondary.RequireTotp

	extJwtSignerId := stringz.OrEmpty(result.AuthPolicy().Secondary.RequiredExtJwtSigner)

	if extJwtSignerId != "" {
		authRequest.SecondaryExtJwtSigner, err = s.env.GetManagers().ExternalJwtSigner.Read(extJwtSignerId)

		if err != nil {
			return nil, err
		}
	}

	if authCtx.GetMethod() == AuthMethodCert {
		if len(authRequest.PeerCerts) == 0 {
			authRequest.PeerCerts = authCtx.GetCerts()
		}
		certAuth := result.Authenticator().ToCert()

		if certAuth != nil {
			authRequest.IsCertExtendable = certAuth.IsIssuedByNetwork
			authRequest.IsCertExtendable = true
			authRequest.IsCertKeyRollRequested = certAuth.IsKeyRollRequested
			authRequest.ImproperClientCertChain = result.ImproperClientCertChain()
		}

	}

	authRequest.AuthenticatorId = result.AuthenticatorId()

	return authRequest, nil
}

// IsTokenRevoked returns true or false if a token has been revoked
func (s *HybridStorage) IsTokenRevoked(tokenId string) bool {
	revocation, _ := s.env.GetManagers().Revocation.Read(tokenId)

	return revocation != nil
}

// VerifyTotp will update and return the AuthRequest associated with `id`
func (s *HybridStorage) VerifyTotp(ctx *change.Context, code string, id string) (*AuthRequest, error) {
	code = strings.TrimSpace(code)
	id = strings.TrimSpace(id)

	if len(code) > 13 || len(id) > 40 {
		return nil, errors.New("invalid input")
	}

	if len(code) == 0 {
		return nil, errors.New("code is required")
	}

	if len(id) == 0 {
		return nil, errors.New("invalid request")
	}

	authRequest, ok := s.authRequests.Get(id)

	if !ok {
		return nil, errors.New("request not found")
	}

	if len(authRequest.Amr) == 0 {
		return nil, errors.New("request not authorized")
	}

	totp, err := s.env.GetManagers().Mfa.ReadOneByIdentityId(authRequest.IdentityId)

	if err != nil {
		return nil, errors.New("could not read totp status")
	}

	if totp == nil {
		return nil, errors.New("totp not found")
	}

	ok, _ = s.env.GetManagers().Mfa.Verify(totp, code, ctx)

	if !ok {
		return nil, apierror.NewInvalidMfaTokenError()
	}

	authRequest.AddAmr(AuthMethodSecondaryTotp)

	return authRequest, nil
}

// CreateAuthRequest creates a new AuthRequest based on an incoming request, implements the op.Storage interface
func (s *HybridStorage) CreateAuthRequest(ctx context.Context, authReq *oidc.AuthRequest, identityId string) (op.AuthRequest, error) {
	httpRequest, err := HttpRequestFromContext(ctx)

	if httpRequest == nil || err != nil {
		return nil, oidc.ErrServerError()
	}

	request := &AuthRequest{
		AuthRequest:   *authReq,
		CreationDate:  time.Now(),
		IdentityId:    identityId,
		ApiSessionId:  uuid.NewString(),
		RemoteAddress: httpRequest.RemoteAddr,
	}

	request.PeerCerts = httpRequest.TLS.PeerCertificates

	for _, authHeader := range httpRequest.Header.Values("authorize") {
		if strings.HasPrefix(authHeader, "Bearer ") {
			request.BearerTokenDetected = true
			break
		}
	}

	request.RequestedMethod = httpRequest.URL.Query().Get("method")

	configTypeNames := httpRequest.URL.Query()["configTypes"]

	configTypeIds := s.env.GetManagers().ConfigType.MapConfigTypeNamesToIds(configTypeNames, identityId)

	for configId := range configTypeIds {
		request.ConfigTypes = append(request.ConfigTypes, configId)
	}

	if len(authReq.Prompt) == 1 && authReq.Prompt[0] == "none" {
		return nil, oidc.ErrLoginRequired()
	}

	request.Id = uuid.NewString()

	s.authRequests.Set(request.Id, request)

	return request, nil
}

// AuthRequestByID implements the op.Storage interface
func (s *HybridStorage) AuthRequestByID(_ context.Context, id string) (op.AuthRequest, error) {
	return s.GetAuthRequest(id)
}

// GetAuthRequest returns an AuthRequest by id
func (s *HybridStorage) GetAuthRequest(id string) (*AuthRequest, error) {
	request, ok := s.authRequests.Get(id)
	if !ok {
		return nil, fmt.Errorf("request not found")
	}
	return request, nil
}

// AuthRequestByCode implements the op.Storage interface
func (s *HybridStorage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	requestID, ok := func() (string, bool) {
		requestID, ok := s.codes.Get(code)
		return requestID, ok
	}()
	if !ok {
		return nil, fmt.Errorf("code invalid or expired")
	}
	return s.AuthRequestByID(ctx, requestID)
}

// SaveAuthCode implements the op.Storage interface
func (s *HybridStorage) SaveAuthCode(_ context.Context, id string, code string) error {
	s.codes.Set(code, id)
	return nil
}

// DeleteAuthRequest implements the op.Storage interface
func (s *HybridStorage) DeleteAuthRequest(_ context.Context, id string) error {
	s.authRequests.Remove(id)

	var toRemove []string
	s.codes.IterCb(func(key string, v string) {
		if v == id {
			toRemove = append(toRemove, key)
		}
	})

	for _, mapKey := range toRemove {
		s.codes.Remove(mapKey)
	}

	return nil
}

// CreateAccessToken implements the op.Storage interface
func (s *HybridStorage) CreateAccessToken(ctx context.Context, request op.TokenRequest) (string, time.Time, error) {
	accessTokenId, accessClaims, err := s.createAccessToken(ctx, request)

	if err != nil {
		return "", time.Time{}, err
	}

	ts, err := TokenStateFromContext(ctx)

	if err != nil {
		return "", time.Time{}, err
	}

	ts.AccessClaims = accessClaims

	return accessTokenId, accessClaims.Expiration.AsTime(), nil
}

// createAccessToken converts an op.TokenRequest into an access token
func (s *HybridStorage) createAccessToken(ctx context.Context, request op.TokenRequest) (string, *common.AccessClaims, error) {
	now := time.Now()

	claims := &common.AccessClaims{
		AccessTokenClaims: oidc.AccessTokenClaims{
			TokenClaims: oidc.TokenClaims{
				JWTID:      uuid.NewString(),
				Issuer:     op.IssuerFromContext(ctx),
				Subject:    request.GetSubject(),
				Audience:   request.GetAudience(),
				Expiration: oidc.Time(now.Add(s.config.AccessTokenDuration).Unix()),
				IssuedAt:   oidc.Time(now.Unix()),
				AuthTime:   oidc.Time(now.Unix()),
				NotBefore:  oidc.Time(now.Unix()),
			},
		},
		CustomClaims: common.CustomClaims{},
	}

	var eventType = "unhandled"

	switch req := request.(type) {
	case *AuthRequest:
		eventType = event.ApiSessionEventTypeCreated
		claims.CustomClaims.ApiSessionId = req.ApiSessionId
		claims.CustomClaims.ApplicationId = req.ClientID
		claims.CustomClaims.ConfigTypes = req.ConfigTypes
		claims.AuthenticationMethodsReferences = req.GetAMR()
		claims.CustomClaims.CertFingerprints = req.GetCertFingerprints()
		claims.CustomClaims.EnvInfo = req.EnvInfo
		claims.CustomClaims.SdkInfo = req.SdkInfo
		claims.CustomClaims.RemoteAddress = req.RemoteAddress
		claims.CustomClaims.IsCertExtendable = req.IsCertExtendable
		claims.CustomClaims.IsCertExtendRequested = req.IsCertExtendRequested
		claims.CustomClaims.IsCertKeyRollRequested = req.IsCertKeyRollRequested
		claims.CustomClaims.ImproperClientCertChain = req.ImproperClientCertChain
		claims.CustomClaims.AuthenticatorId = req.AuthenticatorId
		claims.AuthTime = oidc.Time(req.AuthTime.Unix())
		claims.AccessTokenClaims.AuthenticationMethodsReferences = req.GetAMR()
		claims.ClientID = req.ClientID
	case *RefreshTokenRequest:
		eventType = event.ApiSessionEventTypeRefreshed
		claims.CustomClaims = req.CustomClaims
		claims.AuthTime = req.AuthTime
		claims.AccessTokenClaims.AuthenticationMethodsReferences = req.GetAMR()
		claims.ClientID = req.ClientID
	case op.TokenExchangeRequest:
		eventType = event.ApiSessionEventTypeExchanged
		mapClaims := req.GetExchangeSubjectTokenClaims()
		subjectClaims := &common.AccessClaims{}
		if mapClaims != nil {
			jsonStr, err := json.Marshal(mapClaims)

			if err != nil {
				return "", nil, err
			}
			err = json.Unmarshal(jsonStr, subjectClaims)

			if err != nil {
				return "", nil, err
			}
		} else {
			var err error
			subjectTokenStr := req.GetExchangeSubjectTokenIDOrToken()
			_, subjectClaims, err = s.parseAccessToken(subjectTokenStr)

			if err != nil {
				return "", nil, err
			}

			if subjectClaims.CustomClaims.Type != common.TokenTypeAccess && subjectClaims.CustomClaims.Type != common.TokenTypeRefresh {
				return "", nil, fmt.Errorf("invalid token type: %s", claims.CustomClaims.Type)
			}

			claims.Audience = subjectClaims.Audience
			claims.AccessTokenClaims.Scopes = subjectClaims.CustomClaims.Scopes
		}
		claims.CustomClaims = subjectClaims.CustomClaims
		claims.AccessTokenClaims.AuthenticationMethodsReferences = req.GetAMR()
		claims.ClientID = req.GetClientID()
	}

	claims.AccessTokenClaims.Scopes = request.GetScopes()
	claims.CustomClaims.Scopes = request.GetScopes()
	claims.CustomClaims.Type = common.TokenTypeAccess

	identity, err := s.env.GetManagers().Identity.Read(request.GetSubject())

	if err != nil {
		return "", nil, err
	}

	if identity == nil {
		return "", nil, fmt.Errorf("identity not found: %s", request.GetSubject())
	}

	claims.CustomClaims.IsAdmin = identity.IsAdmin
	claims.CustomClaims.ExternalId = stringz.OrEmpty(identity.ExternalId)

	ipAddr := ""
	if httpRequest, _ := HttpRequestFromContext(ctx); httpRequest != nil {
		ipAddr = httpRequest.RemoteAddr
	}

	evt := &event.ApiSessionEvent{
		Namespace:  event.ApiSessionEventNS,
		EventType:  eventType,
		EventSrcId: s.env.GetId(),
		Id:         claims.ApiSessionId,
		Type:       event.ApiSessionTypeJwt,
		Timestamp:  time.Now(),
		IdentityId: identity.Id,
		IpAddress:  ipAddr,
	}
	s.env.GetEventDispatcher().AcceptApiSessionEvent(evt)

	return claims.JWTID, claims, nil
}

// CreateAccessAndRefreshTokens implements the op.Storage interface
func (s *HybridStorage) CreateAccessAndRefreshTokens(ctx context.Context, request op.TokenRequest, currentRefreshToken string) (accessTokenID string, newRefreshToken string, expiration time.Time, err error) {
	accessTokenId, accessClaims, err := s.createAccessToken(ctx, request)

	if err != nil {
		return "", "", time.Time{}, err
	}

	tokenState, err := TokenStateFromContext(ctx)

	if err != nil {
		return "", "", time.Time{}, err
	}

	tokenState.AccessClaims = accessClaims

	if currentRefreshToken == "" {
		refreshToken, refreshClaims, err := s.createRefreshClaims(accessClaims)
		tokenState.RefreshClaims = refreshClaims
		if err != nil {
			return "", "", time.Time{}, err
		}

		return accessTokenId, refreshToken, accessClaims.Expiration.AsTime(), nil
	}

	refreshToken, refreshClaims, err := s.renewRefreshToken(currentRefreshToken)
	tokenState.RefreshClaims = refreshClaims

	if err != nil {
		return "", "", time.Time{}, err
	}

	return accessTokenId, refreshToken, accessClaims.Expiration.AsTime(), nil
}

// parseRefreshToken parses a JWT refresh token
func (s *HybridStorage) parseRefreshToken(tokenStr string) (*jwt.Token, *common.RefreshClaims, error) {
	refreshClaims := &common.RefreshClaims{}
	parsedToken, err := jwt.ParseWithClaims(tokenStr, refreshClaims, s.env.JwtSignerKeyFunc)

	if err != nil || parsedToken == nil {
		return nil, nil, fmt.Errorf("failed to parse token")
	}

	if !parsedToken.Valid {
		return nil, nil, fmt.Errorf("invalid refresh_token")
	}

	if refreshClaims.Type != common.TokenTypeRefresh {
		return nil, nil, errors.New("invalid token type")
	}

	return parsedToken, refreshClaims, nil
}

// parseAccessToken parses a JWT access token
func (s *HybridStorage) parseAccessToken(tokenStr string) (*jwt.Token, *common.AccessClaims, error) {
	accessClaims := &common.AccessClaims{}
	parsedToken, err := jwt.ParseWithClaims(tokenStr, accessClaims, s.env.JwtSignerKeyFunc)

	if err != nil || parsedToken == nil {
		return nil, nil, fmt.Errorf("failed to parse token")
	}

	if !parsedToken.Valid {
		return nil, nil, fmt.Errorf("invalid refresh_token")
	}

	return parsedToken, accessClaims, nil
}

// TokenRequestByRefreshToken implements the op.Storage interface
func (s *HybridStorage) TokenRequestByRefreshToken(_ context.Context, refreshToken string) (op.RefreshTokenRequest, error) {
	_, token, err := s.parseRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}
	return &RefreshTokenRequest{*token}, err
}

// TerminateSession implements the op.Storage interface
func (s *HybridStorage) TerminateSession(_ context.Context, identityId string, clientID string) error {
	now := time.Now()
	return s.saveRevocation(NewRevocation(identityId+","+clientID, now.Add(s.config.MaxTokenDuration())))
}

// GetRefreshTokenInfo implements the op.Storage interface
func (s *HybridStorage) GetRefreshTokenInfo(_ context.Context, _ string, token string) (identityId string, tokenID string, err error) {
	_, refreshClaims, err := s.parseRefreshToken(token)

	if err != nil {
		return "", "", op.ErrInvalidRefreshToken
	}
	return refreshClaims.Subject, token, nil
}

// RevokeToken implements the op.Storage interface
func (s *HybridStorage) RevokeToken(_ context.Context, tokenIDOrToken string, _ string, _ string) *oidc.Error {
	if strings.HasPrefix(tokenIDOrToken, JwtTokenPrefix) {
		_, claims, err := s.parseRefreshToken(tokenIDOrToken)

		if err != nil {
			return nil //not a valid token ignore
		}
		revocation := NewRevocation(claims.JWTID, claims.Expiration.AsTime())
		if err := s.saveRevocation(revocation); err != nil {
			return oidc.ErrServerError()
		}

	}

	revocation := NewRevocation(tokenIDOrToken, time.Now().Add(s.config.MaxTokenDuration()))
	if err := s.saveRevocation(revocation); err != nil {
		return oidc.ErrServerError()
	}

	return nil
}

// SigningKey implements the op.Storage interface
func (s *HybridStorage) SigningKey(_ context.Context) (op.SigningKey, error) {
	return &s.signingKey, nil
}

// SignatureAlgorithms implements the op.Storage interface
func (s *HybridStorage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{s.signingKey.Algorithm()}, nil
}

// KeySet implements the op.Storage interface
func (s *HybridStorage) KeySet(_ context.Context) ([]op.Key, error) {
	signers := s.env.GetPeerSigners()

	for _, cert := range signers {
		kid := fmt.Sprintf("%s", sha1.Sum(cert.Raw))

		if _, found := s.keys.Get(kid); found {
			continue
		}

		if newKey := newKeyFromCert(cert, kid); newKey != nil {
			s.keys.Set(kid, newKey)
		} else {
			pfxlog.Logger().
				WithField("issuer", cert.Issuer).
				WithField("subject", cert.Subject).
				WithField("kid", fmt.Sprintf("%x", sha1.Sum(cert.Raw))).
				WithField("publicKeyType", fmt.Sprintf("%T", cert.PublicKey)).
				Error("could not convert cert to JWKS key, unknown signing method")
		}
	}

	var result []op.Key

	//this controller
	if !s.IsTokenRevoked(s.signingKey.id) {
		result = append(result, &pubKey{key: s.signingKey})
	}

	//peer controllers
	s.keys.IterCb(func(kid string, key *pubKey) {
		if !s.IsTokenRevoked(kid) {
			result = append(result, key)
		}
	})

	return result, nil
}

// GetClientByClientID implements the op.Storage interface
func (s *HybridStorage) GetClientByClientID(_ context.Context, clientID string) (op.Client, error) {
	client, ok := s.clients.Get(clientID)
	if !ok {
		return nil, fmt.Errorf("client not found")
	}
	return client, nil
}

// AuthorizeClientIDSecret implements the op.Storage interface
func (s *HybridStorage) AuthorizeClientIDSecret(_ context.Context, clientID, clientSecret string) error {
	client, ok := s.clients.Get(clientID)
	if !ok {
		return fmt.Errorf("client not found")
	}

	//this isn't used and is plain text comparison
	if client.secret != clientSecret {
		return fmt.Errorf("invalid secret")
	}
	return nil
}

// SetUserinfoFromScopes implements the op.Storage interface.
func (s *HybridStorage) SetUserinfoFromScopes(_ context.Context, _ *oidc.UserInfo, _, _ string, _ []string) error {
	return nil
}

// SetUserinfoFromRequest implements the op.CanSetUserinfoFromRequest interface.
func (s *HybridStorage) SetUserinfoFromRequest(_ context.Context, userinfo *oidc.UserInfo, token op.IDTokenRequest, scopes []string) error {
	return s.setInfo(userinfo, token.GetSubject(), scopes)
}

// SetUserinfoFromToken implements the op.Storage interface
func (s *HybridStorage) SetUserinfoFromToken(ctx context.Context, userinfo *oidc.UserInfo, tokenID, subject, _ string) error {
	httpRequest, err := HttpRequestFromContext(ctx)

	if s.IsTokenRevoked(tokenID) {
		return errors.New("token is revoked")
	}

	if err != nil {
		return err
	}

	accessStr, err := getAccessToken(httpRequest)

	if err != nil {
		return err
	}

	_, claims, err := s.parseAccessToken(accessStr)

	if err != nil {
		return err
	}

	if claims.Type != common.TokenTypeAccess {
		return errors.New("token is invalid type")
	}

	return s.setInfo(userinfo, subject, nil)
}

// SetIntrospectionFromToken implements the op.Storage interface
func (s *HybridStorage) SetIntrospectionFromToken(_ context.Context, _ *oidc.IntrospectionResponse, _, _, _ string) error {
	return fmt.Errorf("unsupported")
}

// GetPrivateClaimsFromScopes implements the op.Storage interface
func (s *HybridStorage) GetPrivateClaimsFromScopes(ctx context.Context, identityId, clientID string, scopes []string) (claims map[string]interface{}, err error) {
	return s.getPrivateClaims(ctx, identityId, clientID, scopes)
}

func (s *HybridStorage) getPrivateClaims(ctx context.Context, _, _ string, _ []string) (claims map[string]interface{}, err error) {
	tokenState, err := TokenStateFromContext(ctx)

	if err != nil {
		return nil, err
	}

	tsClaims, err := tokenState.AccessClaims.CustomClaims.ToMap()

	return tsClaims, err
}

// GetKeyByIDAndClientID implements the op.Storage interface
func (s *HybridStorage) GetKeyByIDAndClientID(_ context.Context, keyID, _ string) (*jose.JSONWebKey, error) {
	targetKey, found := s.keys.Get(keyID)

	if !found {
		return nil, errors.New("key not found")
	}

	return &jose.JSONWebKey{
		KeyID: keyID,
		Use:   "sig",
		Key:   targetKey,
	}, nil
}

// ValidateJWTProfileScopes implements the op.Storage interface
func (s *HybridStorage) ValidateJWTProfileScopes(_ context.Context, _ string, scopes []string) ([]string, error) {
	allowedScopes := make([]string, 0)
	for _, scope := range scopes {
		if scope == oidc.ScopeOpenID || scope == oidc.ScopeOfflineAccess {
			allowedScopes = append(allowedScopes, scope)
		}
	}
	return allowedScopes, nil
}

// Health implements the op.Storage interface
func (s *HybridStorage) Health(_ context.Context) error {
	return nil
}

func (s *HybridStorage) createRefreshClaims(accessClaims *common.AccessClaims) (string, *common.RefreshClaims, error) {
	claims := &common.RefreshClaims{
		IDTokenClaims: oidc.IDTokenClaims{
			TokenClaims: accessClaims.TokenClaims,
			NotBefore:   accessClaims.NotBefore,
		},
		CustomClaims: accessClaims.CustomClaims,
	}

	claims.Expiration = oidc.Time(time.Now().Add(s.config.RefreshTokenDuration).Unix())
	claims.Type = common.TokenTypeRefresh

	token, _ := s.env.GetRootTlsJwtSigner().Generate(claims)

	return token, claims, nil
}

func (s *HybridStorage) saveRevocation(revocation *model.Revocation) error {
	return s.env.GetManagers().Revocation.Create(revocation, change.New())
}

func (s *HybridStorage) renewRefreshToken(currentRefreshToken string) (string, *common.RefreshClaims, error) {
	_, refreshClaims, err := s.parseRefreshToken(currentRefreshToken)

	if err != nil {
		return "", nil, fmt.Errorf("invalid refresh token")
	}

	if err = s.saveRevocation(NewRevocation(refreshClaims.JWTID, refreshClaims.Expiration.AsTime())); err != nil {
		return "", nil, err
	}

	newRefreshClaims := &common.RefreshClaims{
		IDTokenClaims: refreshClaims.IDTokenClaims,
		CustomClaims:  refreshClaims.CustomClaims,
	}
	now := time.Now()
	newRefreshClaims.JWTID = uuid.NewString()
	newRefreshClaims.IssuedAt = oidc.Time(now.Unix())
	newRefreshClaims.NotBefore = oidc.Time(now.Unix())
	newRefreshClaims.Expiration = oidc.Time(now.Add(s.config.RefreshTokenDuration).Unix())

	token, _ := s.env.GetRootTlsJwtSigner().Generate(newRefreshClaims)

	return token, newRefreshClaims, err
}

func (s *HybridStorage) setInfo(userInfo *oidc.UserInfo, identityId string, scopes []string) (err error) {
	identity, err := s.env.GetManagers().Identity.Read(identityId)

	if err != nil {
		return err
	}

	if identity == nil {
		return fmt.Errorf("user not found")
	}

	for _, scope := range scopes {
		switch scope {
		case oidc.ScopeOpenID:
			userInfo.Subject = identity.Id
			userInfo.Name = identity.Name
			if identity.ExternalId != nil && *identity.ExternalId != "" {
				userInfo.AppendClaims(common.CustomClaimExternalId, identity.ExternalId)
			}

			userInfo.AppendClaims(common.CustomClaimIsAdmin, identity.IsAdmin)
		}
	}
	return nil
}

func tokenTypeToName(oidcType oidc.TokenType) string {
	switch oidcType {
	case oidc.AccessTokenType:
		return "access_token"
	case oidc.IDTokenType:
		return "id_token"
	case oidc.RefreshTokenType:
		return "refresh_token"
	}

	return "unknown_token"

}

// ValidateTokenExchangeRequest implements the op.TokenExchangeStorage interface
func (s *HybridStorage) ValidateTokenExchangeRequest(_ context.Context, request op.TokenExchangeRequest) error {
	if request.GetRequestedTokenType() == "" {
		request.SetRequestedTokenType(oidc.RefreshTokenType)
	}

	requestedType := request.GetExchangeSubjectTokenType()
	proofType := request.GetExchangeSubjectTokenType()

	switch proofType {
	case oidc.AccessTokenType:
		if requestedType != oidc.AccessTokenType {
			return fmt.Errorf("exchanging %s for %s is not supported", tokenTypeToName(proofType), tokenTypeToName(requestedType))
		}
	case oidc.IDTokenType:
		return fmt.Errorf("exchanging %s for any token type is not supported", tokenTypeToName(proofType))
	case oidc.RefreshTokenType:
		if requestedType != oidc.AccessTokenType && requestedType != oidc.RefreshTokenType {
			return fmt.Errorf("exchanging %s for %s is not supported", tokenTypeToName(proofType), tokenTypeToName(requestedType))
		}
	default:
		return fmt.Errorf("exchange subject type (%s) is not supported", proofType)
	}

	for _, aud := range request.GetAudience() {
		if aud != common.ClaimAudienceOpenZiti && aud != common.ClaimLegacyNative {
			return fmt.Errorf("invalid audience requested [%s]", aud)
		}
	}

	scopes := request.GetScopes()

	if len(scopes) == 1 && scopes[0] == "" {
		//no scopes supplied, this is okay, do nothing, fill with defaults
	} else {
		for _, scope := range request.GetScopes() {
			if scope != oidc.ScopeOfflineAccess && scope != oidc.ScopeOpenID {
				return fmt.Errorf("invalid scope requested [%s]", scope)
			}
		}
	}

	return nil
}

func (s *HybridStorage) CreateTokenExchangeRequest(_ context.Context, _ op.TokenExchangeRequest) error {
	return nil
}

// GetPrivateClaimsFromTokenExchangeRequest implements the op.TokenExchangeStorage interface
func (s *HybridStorage) GetPrivateClaimsFromTokenExchangeRequest(ctx context.Context, request op.TokenExchangeRequest) (claims map[string]interface{}, err error) {
	claims, err = s.getPrivateClaims(ctx, "", request.GetClientID(), request.GetScopes())
	if err != nil {
		return nil, err
	}

	for k, v := range s.getTokenExchangeClaims(ctx, request) {
		claims = appendClaim(claims, k, v)
	}

	return claims, nil
}

// SetUserinfoFromTokenExchangeRequest implements the op.TokenExchangeStorage interface
func (s *HybridStorage) SetUserinfoFromTokenExchangeRequest(ctx context.Context, userinfo *oidc.UserInfo, request op.TokenExchangeRequest) error {
	err := s.setInfo(userinfo, request.GetSubject(), request.GetScopes())
	if err != nil {
		return err
	}

	for k, v := range s.getTokenExchangeClaims(ctx, request) {
		userinfo.AppendClaims(k, v)
	}

	return nil
}

func (s *HybridStorage) getTokenExchangeClaims(_ context.Context, _ op.TokenExchangeRequest) (claims map[string]interface{}) {
	return claims
}

func appendClaim(claims map[string]interface{}, claim string, value interface{}) map[string]interface{} {
	if claims == nil {
		claims = make(map[string]interface{})
	}
	claims[claim] = value
	return claims
}

type deviceAuthorizationEntry struct {
	deviceCode string
	userCode   string
	state      *op.DeviceAuthorizationState
}

// StoreDeviceAuthorization implements op.DeviceAuthorizationStorage
func (s *HybridStorage) StoreDeviceAuthorization(_ context.Context, clientID, deviceCode, userCode string, expires time.Time, scopes []string) error {

	if _, ok := s.clients.Get(clientID); !ok {
		return errors.New("client not found")
	}

	if _, ok := s.userCodes.Get(userCode); ok {
		return op.ErrDuplicateUserCode
	}

	entry := deviceAuthorizationEntry{
		deviceCode: deviceCode,
		userCode:   userCode,
		state: &op.DeviceAuthorizationState{
			ClientID: clientID,
			Scopes:   scopes,
			Expires:  expires,
		},
	}

	s.deviceCodes.Set(deviceCode, entry)
	s.userCodes.Set(userCode, deviceCode)

	return nil
}

// GetDeviceAuthorizatonState implements op.DeviceAuthorizationStorage
func (s *HybridStorage) GetDeviceAuthorizatonState(ctx context.Context, clientID, deviceCode string) (*op.DeviceAuthorizationState, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	entry, ok := s.deviceCodes.Get(deviceCode)
	if !ok || entry.state.ClientID != clientID {
		return nil, errors.New("device code not found for client") // is there a standard not found error in the framework?
	}

	return entry.state, nil
}

// GetDeviceAuthorizationByUserCode implements op.DeviceAuthorizationStorage
func (s *HybridStorage) GetDeviceAuthorizationByUserCode(_ context.Context, userCode string) (*op.DeviceAuthorizationState, error) {
	code, ok := s.userCodes.Get(userCode)

	if !ok {
		return nil, errors.New("user code not found")
	}

	entry, ok := s.deviceCodes.Get(code)
	if !ok {
		return nil, errors.New("user code not found")
	}

	return entry.state, nil
}

// CompleteDeviceAuthorization implements op.DeviceAuthorizationStorage
func (s *HybridStorage) CompleteDeviceAuthorization(_ context.Context, userCode, subject string) error {
	code, ok := s.userCodes.Get(userCode)

	if !ok {
		return errors.New("user code not found")
	}

	entry, ok := s.deviceCodes.Get(code)

	if !ok {
		return errors.New("user code not found")
	}

	entry.state.Subject = subject
	entry.state.Done = true
	return nil
}

// DenyDeviceAuthorization implements op.DeviceAuthorizationStorage
func (s *HybridStorage) DenyDeviceAuthorization(_ context.Context, userCode string) error {
	code, ok := s.userCodes.Get(userCode)

	if !ok {
		return errors.New("device code not found")
	}

	authEntry, ok := s.deviceCodes.Get(code)

	if !ok {
		return errors.New("device auth entry not found")
	}

	authEntry.state.Denied = true
	return nil
}

// AuthRequestDone is used by testing and is not required to implement op.Storage
func (s *HybridStorage) AuthRequestDone(id string) error {
	if req, ok := s.authRequests.Get(id); ok {
		if req.HasFullAuth() {
			return nil
		}
		return errors.New("additional authentication interactions are required")
	}

	return errors.New("request not found")
}

// ClientCredentials implements op.ClientCredentialsStorage
func (s *HybridStorage) ClientCredentials(_ context.Context, clientID, clientSecret string) (op.Client, error) {
	client, ok := s.serviceUsers.Get(clientID)

	if !ok {
		return nil, errors.New("wrong service user or password")
	}

	if client.secret != clientSecret {
		return nil, errors.New("wrong service user or password")
	}

	return client, nil
}

// ClientCredentialsTokenRequest implements op.ClientCredentialsStorage
func (s *HybridStorage) ClientCredentialsTokenRequest(_ context.Context, clientID string, scopes []string) (op.TokenRequest, error) {
	client, ok := s.serviceUsers.Get(clientID)
	if !ok {
		return nil, errors.New("wrong service user or password")
	}

	return &oidc.JWTTokenRequest{
		Subject:  client.id,
		Audience: []string{clientID},
		Scopes:   scopes,
	}, nil
}

func getAccessToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("authorization")
	if authHeader == "" {
		return "", errors.New("no auth header")
	}
	parts := strings.Split(authHeader, "Bearer ")
	if len(parts) != 2 {
		return "", errors.New("invalid auth header")
	}
	return parts[1], nil
}
