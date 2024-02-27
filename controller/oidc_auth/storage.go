package oidc_auth

import (
	"context"
	"crypto"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/google/uuid"
	"gopkg.in/square/go-jose.v2"

	"github.com/zitadel/oidc/v2/pkg/oidc"
	"github.com/zitadel/oidc/v2/pkg/op"
)

var (
	_ op.Storage                  = &HybridStorage{}
	_ op.ClientCredentialsStorage = &HybridStorage{}
)

const JwtTokenPrefix = "eY"

// Storage is a compound interface of op.Storage and custom storage functions
type Storage interface {
	op.Storage

	// Authenticate attempts to perform authentication on supplied credentials for all known authentication methods
	Authenticate(authCtx model.AuthContext, id string, configTypes []string) (*AuthRequest, error)

	// VerifyTotp will verify the supplied code for the current authentication request's subject
	// A change context is required for the removal of one-time TOTP recovery codes
	VerifyTotp(ctx *change.Context, code string, id string) (*AuthRequest, error)

	// IsTokenRevoked will return true if a token has been removed.
	// TokenId may be a JWT token id or an identity id
	IsTokenRevoked(tokenId string) bool

	// AddClient adds an OIDC Client to the registry of valid clients.
	AddClient(client *Client)

	// GetAuthRequest returns an *AuthRequest by its id
	GetAuthRequest(id string) (*AuthRequest, error)
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
	codes        cmap.ConcurrentMap[string, string]       //code->authRequest.Id

	clients cmap.ConcurrentMap[string, *Client]

	deviceCodes  cmap.ConcurrentMap[string, deviceAuthorizationEntry]
	userCodes    cmap.ConcurrentMap[string, string]
	serviceUsers cmap.ConcurrentMap[string, *Client]

	startOnce sync.Once
	config    *Config

	keys cmap.ConcurrentMap[string, *pubKey]
}

func (s *HybridStorage) AddClient(client *Client) {
	s.clients.Set(client.id, client)
}

var _ Storage = &HybridStorage{}

func NewStorage(kid string, publicKey crypto.PublicKey, privateKey crypto.PrivateKey, singingMethod jwt.SigningMethod, config *Config, env model.Env) *HybridStorage {
	store := &HybridStorage{
		config:       config,
		authRequests: cmap.New[*AuthRequest](),
		codes:        cmap.New[string](),
		clients:      cmap.New[*Client](),
		env:          env,
		signingKey: key{
			id:         kid,
			algorithm:  jose.SignatureAlgorithm(singingMethod.Alg()),
			privateKey: privateKey,
			publicKey:  publicKey,
		},
		deviceCodes:  cmap.New[deviceAuthorizationEntry](),
		userCodes:    cmap.New[string](),
		serviceUsers: cmap.New[*Client](),
		keys:         cmap.New[*pubKey](),
	}

	store.start()
	return store
}

// start will run Clean every 10 seconds
func (s *HybridStorage) start() {
	s.startOnce.Do(func() {
		closeNotify := s.env.GetHostController().GetCloseNotifyChannel()
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

	authRequest.IdentityId = result.IdentityId()
	authRequest.AddAmr(authCtx.GetMethod())
	authRequest.ConfigTypes = append(authRequest.ConfigTypes, configTypes...)

	mfa, err := s.env.GetManagers().Mfa.ReadOneByIdentityId(authRequest.IdentityId)

	if err != nil {
		return nil, err
	}

	authRequest.SecondaryTotpRequired = mfa != nil && mfa.IsVerified

	if authCtx.GetMethod() == AuthMethodCert {
		if len(authRequest.PeerCerts) == 0 {
			authRequest.PeerCerts = authCtx.GetCerts()
		}
	}

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
		AuthRequest:  *authReq,
		CreationDate: time.Now(),
		IdentityId:   identityId,
		ApiSessionId: uuid.NewString(),
	}

	request.PeerCerts = httpRequest.TLS.PeerCertificates

	for _, authHeader := range httpRequest.Header.Values("authorize") {
		if strings.HasPrefix(authHeader, "Bearer ") {
			request.BearerTokenDetected = true
			break
		}
	}

	request.RequestedMethod = httpRequest.URL.Query().Get("method")

	request.ConfigTypes = httpRequest.URL.Query()["configTypes"]

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
	for entry := range s.codes.IterBuffered() {
		if entry.Val == id {
			s.codes.Remove(entry.Key)
		}
	}

	return nil
}

// CreateAccessToken implements the op.Storage interface
func (s *HybridStorage) CreateAccessToken(_ context.Context, request op.TokenRequest) (string, time.Time, error) {
	tokenId, claims, err := s.createAccessToken(request)

	if err != nil {
		return "", time.Time{}, err
	}

	return tokenId, claims.Expiration.AsTime(), nil
}

// createAccessToken converts an op.TokenRequest into an access token
func (s *HybridStorage) createAccessToken(request op.TokenRequest) (string, *AccessClaims, error) {
	var applicationID string
	var apiSessionID string
	var amr []string
	var configTypes []string
	var certsFingerprints []string

	now := time.Now()
	authTime := oidc.Time(now.Unix())

	switch req := request.(type) {
	case *AuthRequest:
		apiSessionID = req.ApiSessionId
		applicationID = req.ClientID
		configTypes = req.ConfigTypes
		amr = req.GetAMR()
		certsFingerprints = req.GetCertFingerprints()
	case *RefreshTokenRequest:
		applicationID = req.ApplicationId
		amr = req.AuthenticationMethodsReferences
		authTime = req.AuthTime
		configTypes = req.ConfigTypes
		certsFingerprints = req.GetCertFingerprints()

	case op.TokenExchangeRequest:
		applicationID = req.GetClientID()
		claims := req.GetExchangeSubjectTokenClaims()
		amr = req.GetAMR()

		if claims != nil {
			val, ok := claims[CustomClaimApiSessionId]

			if !ok {
				return "", nil, errors.New("invalid token exchange request claims: no api session id")
			}

			apiSessionID = val.(string)

			val, ok = claims[CustomClaimsConfigTypes]

			if ok {
				configTypes = val.([]string)
			}

			val, ok = claims[CustomClaimsCertFingerprints]

			if ok {
				certsFingerprints = val.([]string)
			}
		}
	}

	identity, err := s.env.GetManagers().Identity.Read(request.GetSubject())

	if err != nil {
		return "", nil, err
	}

	claims := &AccessClaims{
		AccessTokenClaims: oidc.AccessTokenClaims{
			TokenClaims: oidc.TokenClaims{
				JWTID:                           uuid.NewString(),
				Issuer:                          s.config.Issuer,
				Subject:                         request.GetSubject(),
				Audience:                        []string{ClaimAudienceOpenZiti},
				Expiration:                      oidc.Time(now.Add(s.config.AccessTokenDuration).Unix()),
				IssuedAt:                        oidc.Time(now.Unix()),
				AuthTime:                        authTime,
				NotBefore:                       oidc.Time(now.Unix()),
				AuthenticationMethodsReferences: amr,
				ClientID:                        applicationID,
			},
		},
		CustomClaims: CustomClaims{
			ApiSessionId:     apiSessionID,
			ExternalId:       stringz.OrEmpty(identity.ExternalId),
			IsAdmin:          identity.IsAdmin,
			ConfigTypes:      configTypes,
			ApplicationId:    applicationID,
			Scopes:           request.GetScopes(),
			Type:             TokenTypeAccess,
			CertFingerprints: certsFingerprints,
		},
	}

	if err != nil {
		return "", nil, err
	}

	return claims.JWTID, claims, nil
}

// CreateAccessAndRefreshTokens implements the op.Storage interface
func (s *HybridStorage) CreateAccessAndRefreshTokens(_ context.Context, request op.TokenRequest, currentRefreshToken string) (accessTokenID string, newRefreshToken string, expiration time.Time, err error) {
	accessTokenId, accessClaims, err := s.createAccessToken(request)

	if err != nil {
		return "", "", time.Time{}, err
	}

	if currentRefreshToken == "" {
		refreshToken, _, err := s.createRefreshClaims(accessClaims)
		if err != nil {
			return "", "", time.Time{}, err
		}

		return accessTokenId, refreshToken, accessClaims.Expiration.AsTime(), nil
	}

	refreshToken, _, err := s.renewRefreshToken(currentRefreshToken)
	if err != nil {
		return "", "", time.Time{}, err
	}

	return accessTokenId, refreshToken, accessClaims.Expiration.AsTime(), nil
}

// parseRefreshToken parses a JWT refresh token
func (s *HybridStorage) parseRefreshToken(tokenStr string) (*jwt.Token, *RefreshClaims, error) {
	refreshClaims := &RefreshClaims{}
	parsedToken, err := jwt.ParseWithClaims(tokenStr, refreshClaims, s.env.JwtSignerKeyFunc)

	if err != nil || parsedToken == nil {
		return nil, nil, fmt.Errorf("failed to parse token")
	}

	if !parsedToken.Valid {
		return nil, nil, fmt.Errorf("invalid refresh_token")
	}

	if refreshClaims.Type != TokenTypeRefresh {
		return nil, nil, errors.New("invalid token type")
	}

	return parsedToken, refreshClaims, nil
}

// parseAccessToken parses a JWT access token
func (s *HybridStorage) parseAccessToken(tokenStr string) (*jwt.Token, *AccessClaims, error) {
	accessClaims := &AccessClaims{}
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
	signers := s.env.GetHostController().GetPeerSigners()

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

	if claims.Type != TokenTypeAccess {
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

func (s *HybridStorage) getPrivateClaims(_ context.Context, identityId, _ string, scopes []string) (claims map[string]interface{}, err error) {
	identity, err := s.env.GetManagers().Identity.Read(identityId)

	if err != nil {
		return nil, err
	}

	claims = appendClaim(claims, CustomClaimIsAdmin, identity.IsAdmin)
	claims = appendClaim(claims, CustomClaimExternalId, identity.ExternalId)

	for _, scope := range scopes {
		if strings.HasPrefix(scope, ScopeApiSessionId) {
			apiSessionId := scope[len(ScopeApiSessionId):]
			claims = appendClaim(claims, CustomClaimApiSessionId, apiSessionId)
		}
	}

	return claims, nil
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
		if scope == oidc.ScopeOpenID {
			allowedScopes = append(allowedScopes, scope)
		}
	}
	return allowedScopes, nil
}

// Health implements the op.Storage interface
func (s *HybridStorage) Health(_ context.Context) error {
	return nil
}

func (s *HybridStorage) createRefreshClaims(accessClaims *AccessClaims) (string, *RefreshClaims, error) {
	claims := &RefreshClaims{
		IDTokenClaims: oidc.IDTokenClaims{
			TokenClaims: accessClaims.TokenClaims,
			NotBefore:   accessClaims.NotBefore,
		},
		CustomClaims: accessClaims.CustomClaims,
	}

	claims.Expiration = oidc.Time(time.Now().Add(s.config.RefreshTokenDuration).Unix())
	claims.Type = TokenTypeRefresh

	token, _ := s.env.GetJwtSigner().Generate("", "", claims)

	return token, claims, nil
}

func (s *HybridStorage) saveRevocation(revocation *model.Revocation) error {
	return s.env.GetManagers().Revocation.Create(revocation, change.New())
}

func (s *HybridStorage) renewRefreshToken(currentRefreshToken string) (string, *RefreshClaims, error) {
	_, refreshClaims, err := s.parseRefreshToken(currentRefreshToken)

	if err != nil {
		return "", nil, fmt.Errorf("invalid refresh token")
	}

	if err = s.saveRevocation(NewRevocation(refreshClaims.JWTID, refreshClaims.Expiration.AsTime())); err != nil {
		return "", nil, err
	}

	newRefreshClaims := &RefreshClaims{
		IDTokenClaims: refreshClaims.IDTokenClaims,
		CustomClaims:  refreshClaims.CustomClaims,
	}
	now := time.Now()
	newRefreshClaims.JWTID = uuid.NewString()
	newRefreshClaims.IssuedAt = oidc.Time(now.Unix())
	newRefreshClaims.NotBefore = oidc.Time(now.Unix())
	newRefreshClaims.Expiration = oidc.Time(now.Add(s.config.RefreshTokenDuration).Unix())

	token, _ := s.env.GetJwtSigner().Generate("", "", newRefreshClaims)

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
				userInfo.AppendClaims(CustomClaimExternalId, identity.ExternalId)
			}

			userInfo.AppendClaims(CustomClaimIsAdmin, identity.IsAdmin)
		}
	}
	return nil
}

// ValidateTokenExchangeRequest implements the op.TokenExchangeStorage interface
func (s *HybridStorage) ValidateTokenExchangeRequest(_ context.Context, request op.TokenExchangeRequest) error {
	if request.GetRequestedTokenType() == "" {
		request.SetRequestedTokenType(oidc.RefreshTokenType)
	}

	// Just an example, some use cases might need this use case
	if request.GetExchangeSubjectTokenType() == oidc.IDTokenType && request.GetRequestedTokenType() == oidc.RefreshTokenType {
		return errors.New("exchanging id_token to refresh_token is not supported")
	}

	allowedScopes := make([]string, 0)
	for _, scope := range request.GetScopes() {
		if scope == oidc.ScopeAddress {
			continue
		}

		allowedScopes = append(allowedScopes, scope)
	}

	request.SetCurrentScopes(allowedScopes)

	return nil
}

func (s *HybridStorage) CreateTokenExchangeRequest(_ context.Context, _ op.TokenExchangeRequest) error {
	return fmt.Errorf("unsupported")
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
