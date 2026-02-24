package common

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
)

// MaxBearerTokensProcessed represents the maximum number of bearer tokens that will be processed.
const MaxBearerTokensProcessed = 3

// TokenIssuer represents a JWT token issuer capable of verifying tokens.
// Implementations provide token verification, configuration queries, and claim extraction.
type TokenIssuer interface {
	// Id returns the unique identifier of this token issuer.
	Id() string
	// TypeName returns the type name of the token issuer (e.g., "externalJwtSigner").
	TypeName() string
	// Name returns the human-readable name of the token issuer.
	Name() string
	// IsEnabled returns true if this token issuer is enabled.
	IsEnabled() bool

	// PubKeyByKid returns the public key for the given key ID.
	PubKeyByKid(kid string) (IssuerPublicKey, bool)
	// Resolve loads the issuer's public keys from certificate or JWKS endpoint.
	// If force is true, refreshes cached keys even if already loaded.
	Resolve(force bool) error

	// ExpectedIssuer returns the issuer claim value that tokens from this issuer should contain.
	ExpectedIssuer() string
	// ExpectedAudience returns the audience claim value that tokens should contain.
	ExpectedAudience() string

	// AuthenticatorId returns the authenticator ID for this issuer.
	AuthenticatorId() string

	// EnrollmentAuthPolicyId returns the auth policy ID to apply to identities enrolled via this issuer.
	EnrollmentAuthPolicyId() string
	// EnrollmentAttributeClaimsSelector returns the JSON pointer path to the attributes claim.
	EnrollmentAttributeClaimsSelector() string
	// EnrollmentNameClaimSelector returns the JSON pointer path to the identity name claim.
	EnrollmentNameClaimSelector() string

	// IdentityIdClaimsSelector returns the JSON pointer path to the identity ID claim.
	IdentityIdClaimsSelector() string

	// UseExternalId returns true if the identity ID should be stored as an external ID.
	UseExternalId() bool

	// VerifyToken verifies and parses a JWT token, returning the verification result.
	VerifyToken(token string) *TokenVerificationResult

	// EnrollToCertEnabled returns true if enrollment to certificate is allowed.
	EnrollToCertEnabled() bool
	// EnrollToTokenEnabled returns true if enrollment to token is allowed.
	EnrollToTokenEnabled() bool

	GetKids() []string

	IsControllerTokenIssuer() bool
}

// TokenVerificationResult contains the result of JWT token verification.
// Includes the parsed token, extracted claims, and the issuer that verified it.
type TokenVerificationResult struct {
	Token                  *jwt.Token
	Claims                 map[string]any
	IdClaimSelector        string
	IdClaimValue           string
	AttributeClaimSelector string
	AttributeClaimValue    []string
	NameClaimSelector      string
	NameClaimValue         string
	Error                  error
}

// IsValid returns true if the JWT token signature is valid and no other errors have occurred
func (r *TokenVerificationResult) IsValid() bool {
	return r.Error == nil && r.Token != nil && r.Token.Valid
}

// TokenIsExpired returns true if the verification error wraps jwt.ErrTokenExpired, or if
// the token's exp claim is in the past. The exp-claim fallback handles cases where
// the error has been replaced with an application-level error after initial parsing.
func (r *TokenVerificationResult) TokenIsExpired() bool {
	if r.Error != nil {
		return errors.Is(r.Error, jwt.ErrTokenExpired)
	}

	if r.Token != nil && r.Token.Claims != nil {
		expTime, _ := r.Token.Claims.GetExpirationTime()

		if expTime != nil {
			return expTime.Before(time.Now())
		}
	}

	return false
}

// TokenSignatureIsInvalid returns true if the verification error wraps
// jwt.ErrTokenSignatureInvalid, meaning the token's cryptographic signature did not match
// the expected signing key.
func (r *TokenVerificationResult) TokenSignatureIsInvalid() bool {
	return r.Error != nil && errors.Is(r.Error, jwt.ErrTokenSignatureInvalid)
}

// IssuerPublicKey represents a public key and associated certificate chain.
type IssuerPublicKey struct {
	PubKey any                 // The public key used for signature verification
	Chain  []*x509.Certificate // Optional X.509 certificate chain
}

// TokenIssuerCache is a read-only view over the set of known JWT token issuers. It is used
// during request processing to look up the appropriate issuer for a candidate bearer token
// without requiring callers to hold a reference to the full cache implementation.
type TokenIssuerCache interface {
	// GetByIssuerString returns the TokenIssuer whose expected issuer string matches exactly.
	GetByIssuerString(issuer string) TokenIssuer

	// GetById returns the TokenIssuer with the given unique identifier.
	GetById(issuerId string) TokenIssuer

	// GetIssuerStrings returns all registered issuer claim strings.
	GetIssuerStrings() []string

	// VerifyTokenByInspection parses and verifies a raw JWT string, returning the result and
	// the issuer that successfully verified it, or nil if no issuer claimed the token.
	VerifyTokenByInspection(candidateToken string) (*TokenVerificationResult, TokenIssuer)

	// IterateExternalIssuers calls f for each registered issuer. Iteration stops when f returns false.
	IterateExternalIssuers(f func(issuer TokenIssuer) bool)

	// GetIssuerByKid returns the TokenIssuer that owns the given key ID
	GetIssuerByKid(kid string) TokenIssuer
}

// SecurityToken is the result of verifying the primary security token presented on a request.
// It carries either a legacy zt-session string or a controller-issued OIDC bearer token,
// and is used throughout request processing to identify the authenticated session.
type SecurityToken struct {
	IsLegacy  bool               // true if zt-session, false if JWT
	ZtSession string             // non-empty for legacy tokens
	OidcToken *BearerTokenHeader // non-nil for JWT tokens
	Request   *http.Request      //todo: can remove?
}

// SecurityTokenCtx holds all security-token state for a single HTTP request. It parses
// Authorization and zt-Session headers lazily on first access, classifies each bearer token
// as either the primary API-session token or a secondary external token, and caches
// verification results so that subsequent callers pay no additional cost.
type SecurityTokenCtx struct {
	// ztSession holds the raw value of the zt-Session header, if present.
	ztSession string

	httpRequest *http.Request

	// rawAuthorizationHeaders contains every value of the incoming Authorization header.
	rawAuthorizationHeaders []string

	// unverifiedApiSessionBearerToken is the first controller-issued bearer token found.
	// Signature verification is deferred until GetVerifiedApiSessionToken is called.
	unverifiedApiSessionBearerToken *BearerTokenHeader

	// unverifiedExternalBearerTokens holds bearer tokens that are not the primary session
	// token (e.g., secondary ext-jwt tokens or tokens received alongside a zt-Session header).
	unverifiedExternalBearerTokens []*BearerTokenHeader

	fillOnce sync.Once

	hasProcessedHeaders bool
	tokenIssuerCache    TokenIssuerCache

	// Lazy verification for the primary (ziti) token
	apiSessionToken    *SecurityToken
	apiSessionTokenErr error

	processApiSessionTokenOnce sync.Once

	// Lazy verification for external tokens
	externalMu                 sync.Mutex
	bearerTokenCount           int
	bearerTokenParseErrorCount int
}

// BearerTokenHeader represents a single bearer token parsed from an Authorization header.
// It carries both the raw string and the result of any subsequent signature verification,
// allowing callers to inspect unverified claims (e.g., issuer, kid) before committing to
// a full cryptographic check.
type BearerTokenHeader struct {
	// Raw represents the raw token string w/o the "Bearer " prefix
	Raw string

	// UnverifiedToken is the raw unverified token. Use TokenVerificationResult.Token for verified tokens
	UnverifiedToken *jwt.Token

	// AccessClaims is populated for controller issued tokens only
	AccessClaims *AccessClaims

	// HeaderIndex is the original position within the incoming HTTP header
	HeaderIndex int

	TokenVerificationResult *TokenVerificationResult

	TokenIssuer TokenIssuer
}

// VerifyAsApiSessionOidcToken verifies the instance as a controller bearer token
func (b *BearerTokenHeader) VerifyAsApiSessionOidcToken() error {
	if !b.IsApiSessionAccessToken() || b.TokenIssuer == nil || !b.TokenIssuer.IsControllerTokenIssuer() {
		return errorz.NewUnauthorizedOidcInvalid()
	}

	if b.TokenVerificationResult == nil {
		b.TokenVerificationResult = b.TokenIssuer.VerifyToken(b.Raw)
	}

	if b.IsValid() {
		return nil
	}

	if b.TokenVerificationResult.TokenIsExpired() {
		b.TokenVerificationResult.Error = errorz.NewUnauthorizedOidcExpired()
	} else if b.TokenVerificationResult.TokenSignatureIsInvalid() {
		b.TokenVerificationResult.Error = errorz.NewUnauthorizedOidcInvalid()
	}

	return b.TokenVerificationResult.Error
}

// Kid returns the key ID from the token's JOSE header, or an empty string if the token
// has not been parsed or does not carry a kid claim.
func (b *BearerTokenHeader) Kid() string {
	if b.UnverifiedToken == nil {
		return ""
	}

	kid := ""

	kidVal, kidOk := b.UnverifiedToken.Header["kid"]

	if kidOk {
		kid, _ = kidVal.(string)
	}

	return kid
}

// StandardClaims returns the jwt.Claims parsed from the unverified token, or nil if the
// token has not been parsed yet. These claims have not been cryptographically verified.
func (b *BearerTokenHeader) StandardClaims() jwt.Claims {
	if b.UnverifiedToken == nil {
		return nil
	}

	return b.UnverifiedToken.Claims
}

// Issuer returns the iss claim from the unverified token, or an empty string if unavailable.
// Used before signature verification to locate the matching TokenIssuer.
func (b *BearerTokenHeader) Issuer() string {
	if b.UnverifiedToken == nil {
		return ""
	}
	standardClaims := b.StandardClaims()

	if standardClaims == nil {
		return ""
	}

	issuer, err := standardClaims.GetIssuer()
	if err != nil {
		return ""
	}

	return issuer
}

// Audience returns the aud claim from the unverified token, or nil if unavailable.
func (b *BearerTokenHeader) Audience() []string {
	if b.UnverifiedToken == nil {
		return nil
	}
	standardClaims := b.StandardClaims()

	if standardClaims == nil {
		return nil
	}

	audience, err := standardClaims.GetAudience()
	if err != nil {
		return nil
	}

	return audience
}

// IsApiSessionAccessToken returns true when the bearer token was issued by a controller as
// an access token associated with a specific API session.
func (b *BearerTokenHeader) IsApiSessionAccessToken() bool {
	if b.AccessClaims == nil {
		return false
	}

	return b.AccessClaims.ApiSessionId != "" && b.AccessClaims.Type == TokenTypeAccess
}

// IsValid returns true if the token has been verified and the verification result indicates
// a valid signature with no errors. A nil verification result is treated as not valid.
func (b *BearerTokenHeader) IsValid() bool {
	if b == nil || b.TokenVerificationResult == nil {
		return false
	}

	return b.TokenVerificationResult.IsValid()
}

// GetSecurityTokenCtx retrieves the SecurityTokenCtx stored in the request context by
// NewSecurityTokenCtx/AddToRequest, or returns nil if none has been attached.
func GetSecurityTokenCtx(r *http.Request) *SecurityTokenCtx {
	val := r.Context().Value(SecurityTokenCtxKey)

	if val == nil {
		return nil
	}

	return val.(*SecurityTokenCtx)
} //todo: make sure no one is using this

// ContextKeySecurityCtx is a typed key for storing a SecurityCtx in a request context,
// preventing accidental collisions with plain string keys.
type ContextKeySecurityCtx string

// SecurityCtxKey is the context key under which the resolved SecurityCtx is stored after
// authentication has been completed for the request.
const SecurityCtxKey ContextKeySecurityCtx = "securityCtx"

// ContextKeySecurityTokenCtx is the key used to store the SecurityTokenCtx in the request context
type ContextKeySecurityTokenCtx string

// SecurityTokenCtxKey is the key used to store the SecurityTokenCtx in the request context
const SecurityTokenCtxKey ContextKeySecurityTokenCtx = "securityTokenCtx"

// NewSecurityTokenCtx creates a SecurityTokenCtx for the given request and token issuer cache.
// Header parsing is deferred until the first access of token data.
func NewSecurityTokenCtx(r *http.Request, tokenIssuerCache TokenIssuerCache) (*SecurityTokenCtx, error) {
	result := &SecurityTokenCtx{
		tokenIssuerCache: tokenIssuerCache,
		httpRequest:      r,
	}

	return result, nil
}

// GetBearerTokenCount returns the number of bearer tokens found in the Authorization headers,
// up to MaxBearerTokensProcessed. Headers are parsed on the first call.
func (s *SecurityTokenCtx) GetBearerTokenCount() (int, error) {
	err := s.processHeaders()

	if err != nil {
		return 0, err
	}

	return s.bearerTokenCount, nil
}

// GetInvalidBearerTokenCount returns the number of bearer tokens that could not be parsed
// (e.g., malformed JWTs). Valid tokens that fail signature verification are not counted here.
func (s *SecurityTokenCtx) GetInvalidBearerTokenCount() (int, error) {
	err := s.processHeaders()

	if err != nil {
		return 0, err
	}

	return s.bearerTokenParseErrorCount, nil
}

// AddToRequest stores this SecurityTokenCtx in the request's context under SecurityTokenCtxKey
// so that downstream handlers can retrieve it with GetSecurityTokenCtx.
func (s *SecurityTokenCtx) AddToRequest(r *http.Request) {
	*r = *r.WithContext(context.WithValue(r.Context(), SecurityTokenCtxKey, s))
}

// IsZtSession returns true if the request carried a legacy zt-Session header. It triggers
// header processing if not already done.
func (s *SecurityTokenCtx) IsZtSession() bool {
	s.ProcessApiSessionToken()
	return s.ztSession != ""
}

// GetVerifiedApiSessionToken lazily verifies and returns the primary ziti token (zt-session or controller JWT).
// The result is cached for the lifetime of the request.
func (s *SecurityTokenCtx) GetVerifiedApiSessionToken() (*SecurityToken, error) {
	s.ProcessApiSessionToken()

	return s.apiSessionToken, s.apiSessionTokenErr
}

// GetExternalTokenForExtJwtSigner returns the first verified external bearer token whose
// TokenIssuer ID matches extJwtSignerId, or nil if no matching token was presented.
func (s *SecurityTokenCtx) GetExternalTokenForExtJwtSigner(extJwtSignerId string) *BearerTokenHeader {
	candidates := s.GetExternalTokens()

	for _, candidate := range candidates {
		if candidate.TokenVerificationResult != nil && candidate.TokenIssuer != nil && candidate.TokenIssuer.Id() == extJwtSignerId {
			return candidate
		}
	}

	return nil
}

// GetExternalTokens lazily verifies and returns all external bearer tokens from the request.
// Tokens that have already been verified are returned as-is. Unverified tokens are verified
// against their associated TokenIssuer on first access. Results are cached for the lifetime
// of the request.
func (s *SecurityTokenCtx) GetExternalTokens() []*BearerTokenHeader {
	s.externalMu.Lock()
	defer s.externalMu.Unlock()

	s.ProcessApiSessionToken()

	if !s.hasProcessedHeaders {
		return nil
	}

	var results []*BearerTokenHeader

	for _, externalBearerToken := range s.unverifiedExternalBearerTokens {
		//can't processed
		if externalBearerToken == nil || externalBearerToken.Raw == "" {
			continue
		}

		//already processed
		if externalBearerToken.TokenVerificationResult != nil {
			results = append(results, externalBearerToken)
			continue
		}

		if externalBearerToken.TokenIssuer == nil {
			externalBearerToken.TokenVerificationResult = &TokenVerificationResult{Error: errors.New("no token issuer found")}
			results = append(results, externalBearerToken)
			continue
		}

		externalBearerToken.TokenVerificationResult = externalBearerToken.TokenIssuer.VerifyToken(externalBearerToken.Raw)

		results = append(results, externalBearerToken)
	}

	return results
}

// processHeaders parses the zt-Session and Authorization headers exactly once.
// It classifies each bearer token as the primary API-session token or a secondary external
// token and pre-associates each with its matching TokenIssuer from the cache.
func (s *SecurityTokenCtx) processHeaders() error {
	if s.httpRequest == nil {
		return errors.New("request is nil")
	}

	s.fillOnce.Do(func() {
		defer func() { s.hasProcessedHeaders = true }()

		s.ztSession = strings.TrimSpace(s.httpRequest.Header.Get("zt-Session"))

		s.rawAuthorizationHeaders = s.httpRequest.Header.Values("Authorization")

		jwtParser := jwt.NewParser()
		for i, authorizationHeader := range s.rawAuthorizationHeaders {
			authorizationHeader = strings.TrimSpace(authorizationHeader)

			if strings.HasPrefix(authorizationHeader, "Bearer ") {

				if s.bearerTokenCount >= MaxBearerTokensProcessed {
					break
				}

				s.bearerTokenCount++
				rawToken := authorizationHeader[7:]
				claims := &AccessClaims{}
				unverifiedToken, _, err := jwtParser.ParseUnverified(rawToken, claims)

				bearerToken := &BearerTokenHeader{
					Raw:             rawToken,
					UnverifiedToken: unverifiedToken,
					AccessClaims:    claims,
					HeaderIndex:     i,
				}

				if err != nil {
					s.bearerTokenParseErrorCount++
					bearerToken.TokenVerificationResult = &TokenVerificationResult{Error: err}
					continue
				}

				kid := bearerToken.Kid()

				if kid != "" {
					bearerToken.TokenIssuer = s.tokenIssuerCache.GetIssuerByKid(kid)
				}

				if bearerToken.TokenIssuer == nil {
					issuer := bearerToken.Issuer()
					if issuer != "" {
						bearerToken.TokenIssuer = s.tokenIssuerCache.GetByIssuerString(issuer)
					}
				}

				if bearerToken.TokenIssuer == nil {
					bearerToken.TokenVerificationResult = &TokenVerificationResult{Error: errorz.NewUnauthorizedOidcInvalid()}
				}

				if s.ztSession != "" {
					// if we have a legacy header, everything is a secondary
					s.unverifiedExternalBearerTokens = append(s.unverifiedExternalBearerTokens, bearerToken)
				} else if s.unverifiedApiSessionBearerToken == nil && bearerToken.TokenIssuer != nil && bearerToken.TokenIssuer.IsControllerTokenIssuer() && bearerToken.IsApiSessionAccessToken() {
					s.unverifiedApiSessionBearerToken = bearerToken
				} else {
					//all other tokens are secondary or invalid
					s.unverifiedExternalBearerTokens = append(s.unverifiedExternalBearerTokens, bearerToken)
				}
			}
		}

	})

	return nil
}

// ProcessApiSessionToken parses headers (if needed) and then verifies the primary API-session token
// exactly once. Callers that only need the external tokens can call GetExternalTokens directly;
// this method is focused on establishing the session-level authentication result.
func (s *SecurityTokenCtx) ProcessApiSessionToken() {
	err := s.processHeaders()

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("error processing security token headers")
		return
	}

	if !s.hasProcessedHeaders {
		return
	}

	s.processApiSessionTokenOnce.Do(func() {
		if s.ztSession != "" {
			s.apiSessionToken = &SecurityToken{
				IsLegacy:  true,
				ZtSession: s.ztSession,
				Request:   s.httpRequest,
			}
			return

		} else if s.unverifiedApiSessionBearerToken != nil {
			err = s.unverifiedApiSessionBearerToken.VerifyAsApiSessionOidcToken()

			if err == nil {
				s.apiSessionToken = &SecurityToken{
					IsLegacy:  false,
					OidcToken: s.unverifiedApiSessionBearerToken,
					Request:   s.httpRequest,
				}
			} else {
				s.apiSessionTokenErr = err
			}
		} else {
			s.apiSessionToken = nil

			if s.bearerTokenParseErrorCount == 0 {
				s.apiSessionTokenErr = errorz.NewUnauthorizedTokensMissing()
			} else {
				s.apiSessionTokenErr = errorz.NewUnauthorizedOidcInvalid()
			}

		}

	})
}
