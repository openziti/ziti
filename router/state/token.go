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

package state

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/go-events"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/sirupsen/logrus"
)

// ApiSessionTokenType distinguishes between authentication mechanisms during the
// migration from legacy systems to JWT-based authentication.
type ApiSessionTokenType string

const (
	// ApiSessionTokenLegacyProtobuf represents full controller-synchronized sessions
	// with complete metadata transmitted via protobuf messages.
	ApiSessionTokenLegacyProtobuf ApiSessionTokenType = "legacyProtobuf"

	// ApiSessionTokenJwt represents modern self-contained JWT tokens with embedded
	// claims, eliminating the need for controller synchronization.
	ApiSessionTokenJwt ApiSessionTokenType = "JWT"

	// ApiSessionTokenLegacyTokenOnly represents minimal legacy tokens with only
	// the token string, lacking session metadata.
	ApiSessionTokenLegacyTokenOnly ApiSessionTokenType = "legacyTokenOnly"
)

// ApiSessionToken provides a unified interface for authentication tokens across
// Ziti's evolution from controller-synchronized sessions to self-contained JWTs.
//
// This abstraction enables routers to handle both legacy protobuf sessions and
// modern JWT tokens transparently, supporting rolling upgrades without service
// disruption. The embedded protobuf structure maintains compatibility with
// legacy controllers while JWT fields enable modern stateless authentication.
type ApiSessionToken struct {
	// Embedded protobuf session for legacy compatibility and convenience.
	// Controllers historically transmitted session state via this structure.
	*edge_ctrl_pb.ApiSession

	// JWT fields for modern stateless authentication
	JwtToken *jwt.Token
	Claims   *common.AccessClaims

	// Hash provides consistent token identification across all token types,
	// enabling efficient caching and deduplication.
	Hash string

	// ControllerId enforces controller affinity for legacy sessions in HA
	// deployments, where session state may not be replicated across controllers.
	ControllerId string

	Type ApiSessionTokenType
}

// NewApiSessionTokenFromJwt creates a unified token abstraction from a validated JWT,
// enabling modern stateless authentication while maintaining compatibility with
// legacy router code expecting protobuf session structures.
func NewApiSessionTokenFromJwt(jwtToken *jwt.Token, accessClaims *common.AccessClaims) (*ApiSessionToken, error) {
	identityId, err := accessClaims.GetSubject()

	if err != nil {
		return nil, fmt.Errorf("unable to get the api session identity from the JWT subject (%w)", err)
	}

	return &ApiSessionToken{
		ApiSession: &edge_ctrl_pb.ApiSession{
			Token:            jwtToken.Raw,
			CertFingerprints: accessClaims.CertFingerprints,
			Id:               accessClaims.ApiSessionId,
			IdentityId:       identityId,
		},
		JwtToken: jwtToken,
		Claims:   accessClaims,
		Hash:     accessClaims.JWTID,
		Type:     ApiSessionTokenJwt,
	}, nil
}

// NewApiSessionTokenFromProtobuf wraps controller-synchronized session data
// in the unified token abstraction, preserving the legacy synchronization model
// for backwards compatibility during network upgrades. Legacy API Sessions are
// tied to a specific controller.
func NewApiSessionTokenFromProtobuf(apiSessionBuf *edge_ctrl_pb.ApiSession, controllerId string) *ApiSessionToken {
	result := &ApiSessionToken{
		ApiSession:   apiSessionBuf,
		Hash:         logHash(apiSessionBuf.Token),
		Type:         ApiSessionTokenLegacyProtobuf,
		ControllerId: controllerId,
	}

	return result
}

// NewApiSessionTokenFromLegacyToken creates a minimal token abstraction for
// legacy scenarios where only the raw token string is available, filling
// unknown fields with placeholder values to maintain API consistency.
func NewApiSessionTokenFromLegacyToken(token string) *ApiSessionToken {
	return &ApiSessionToken{
		ApiSession: &edge_ctrl_pb.ApiSession{
			Token:            token,
			CertFingerprints: nil,
			Id:               "unknown",
			IdentityId:       "unknown",
		},
		Hash: logHash(token),
		Type: ApiSessionTokenLegacyTokenOnly,
	}
}

// UpdateToken refreshes JWT-specific fields during token renewal while
// preserving the token's identity and legacy compatibility structure.
// This enables seamless token rotation without breaking existing references.
func (a *ApiSessionToken) UpdateToken(new *ApiSessionToken) {
	a.JwtToken = new.JwtToken
	a.Claims = new.Claims
	a.Hash = new.Hash
}

// SelectCtrlCh implements controller affinity for legacy sessions while allowing
// JWT-based sessions to use any available controller, optimizing for both
// backwards compatibility and modern load distribution.
func (a *ApiSessionToken) SelectCtrlCh(ctrls env.NetworkControllers) channel.Channel {
	if a == nil {
		return nil
	}

	// Legacy sessions require specific controller affinity due to non-replicated state
	if a.ControllerId != "" {
		return ctrls.GetCtrlChannel(a.ControllerId)
	}

	// JWT sessions can use any controller due to their self-contained nature
	return ctrls.AnyCtrlChannel()
}

// SelectModelUpdateCtrlCh selects the appropriate controller channel for model
// updates, respecting legacy session affinity while leveraging dedicated model
// update channels for JWT sessions when available.
func (a *ApiSessionToken) SelectModelUpdateCtrlCh(ctrls env.NetworkControllers) channel.Channel {
	if a == nil {
		return nil
	}

	// Maintain controller affinity for legacy sessions
	if a.ControllerId != "" {
		return ctrls.GetCtrlChannel(a.ControllerId)
	}

	// Use specialized model update channel for optimal performance
	return ctrls.GetModelUpdateCtrlChannel()
}

// AddLoggingFields enriches log entries with structured token metadata,
// providing comprehensive debugging information while protecting sensitive
// token data through safe identifiers and hashes.
func (a *ApiSessionToken) AddLoggingFields(logger *logrus.Entry) *logrus.Entry {
	if a != nil {
		logger = logger.WithField("apiSessionToken", logrus.Fields{
			"tokenId":          a.TokenId(),
			"apiSessionId":     a.ApiSession.Id,
			"identityId":       a.IdentityId,
			"type":             a.Type,
			"certFingerprints": a.CertFingerprints,
		})
	}

	return logger
}

// RemovedEventName generates a unique event name for session removal notifications,
// enabling precise event handling and avoiding cross-session event interference.
func (a *ApiSessionToken) RemovedEventName() events.EventName {
	// using token hash here, ensure that both legacy variants use the same hashed value from
	// `token`. JWT tokens do not use this functionality.
	return events.EventName(EventRemovedApiSession + "-" + a.TokenId())
}

// TokenId returns a unique value per token that is also safe to log. Depending
// on the token, it may be a hash of a secret (legacy token types).
func (a *ApiSessionToken) TokenId() string {
	if a.Type == ApiSessionTokenJwt {
		return a.Claims.JWTID
	}

	return a.Hash
}

// Token returns the raw secret token value.
func (a *ApiSessionToken) Token() string {
	switch a.Type {
	case ApiSessionTokenJwt:
		return a.JwtToken.Raw
	case ApiSessionTokenLegacyProtobuf:
		return a.ApiSession.Token
	case ApiSessionTokenLegacyTokenOnly:
		return a.ApiSession.Token
	}

	return "unknown"
}

// IsOidc returns true if this API session token uses OIDC authentication
// JWT access tokens rather than legacy UUID tokens
func (a *ApiSessionToken) IsOidc() bool {
	return a.Type == ApiSessionTokenJwt
}

// IsLegacy returns true if this API session token uses legacy authentication
// formats (UUID-based tokens or protobuf sessions) rather than modern JWT tokens.
func (a *ApiSessionToken) IsLegacy() bool {
	if a.Type == ApiSessionTokenLegacyTokenOnly || a.Type == ApiSessionTokenLegacyProtobuf {
		return true
	}

	return false
}

// logHash creates a truncated SHA-256 hash of a token for safe logging.
// Returns a 27-character base64url-encoded string that uniquely identifies
// the token without exposing its actual value.
func logHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:20])
}

// ServiceSessionTokenType differentiates between service-level authentication
// mechanisms in the transition to JWT-based service access.
type ServiceSessionTokenType string

const (
	// ServiceSessionTypeJwt represents modern JWT-based service access tokens
	// with embedded authorization claims and service-specific permissions.
	ServiceSessionTypeJwt ServiceSessionTokenType = "JWT"

	// ServiceSessionTypeLegacyTokenOnly represents controller-generated service
	// session identifiers from the pre-JWT authentication system.
	ServiceSessionTypeLegacyTokenOnly ServiceSessionTokenType = "legacyTokenOnly"
)

// ServiceSessionToken represents authorization for specific service access within
// an API session, bridging legacy controller-managed service sessions with
// modern JWT-based service access tokens.
//
// Service sessions provide fine-grained authorization beyond the API session
// level, enabling per-service access control and audit trails. This abstraction
// maintains consistency between legacy controller-synchronized sessions and
// self-contained JWT service tokens.
type ServiceSessionToken struct {
	// ServiceId identifies the authorized service
	ServiceId string

	// ApiSessionToken provides the parent API session context
	ApiSessionToken *ApiSessionToken

	// JWT fields for modern service-specific authorization
	JwtToken *jwt.Token
	Claims   *common.ServiceAccessClaims
	Hash     string
	Type     ServiceSessionTokenType
}

// NewServiceSessionToken creates a service session from a validated JWT and
// parent API session, enforcing the hierarchical relationship between API
// sessions and service access while validating claim consistency.
func NewServiceSessionToken(jwtToken *jwt.Token, serviceAccessClaims *common.ServiceAccessClaims, apiSessionToken *ApiSessionToken) (*ServiceSessionToken, error) {
	serviceId, err := jwtToken.Claims.GetSubject()

	if err != nil {
		return nil, fmt.Errorf("unable to get the service id from the JWT subject (%w)", err)
	}

	if apiSessionToken == nil {
		return nil, fmt.Errorf("api session token is required")
	}

	if apiSessionToken.Id != serviceAccessClaims.ApiSessionId {
		return nil, fmt.Errorf("api session id (%s) does not match service session api session id (%s)", apiSessionToken.Id, serviceAccessClaims.ApiSessionId)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(jwtToken.Raw)))

	return &ServiceSessionToken{
		ServiceId:       serviceId,
		ApiSessionToken: apiSessionToken,
		JwtToken:        jwtToken,
		Claims:          serviceAccessClaims,
		Hash:            hash,
		Type:            ServiceSessionTypeJwt,
	}, nil
}

// NewServiceSessionTokenFromId creates a minimal service session abstraction
// for legacy controller-managed sessions where only the session ID is available,
// maintaining API compatibility with minimal metadata.
func NewServiceSessionTokenFromId(id string) *ServiceSessionToken {
	return &ServiceSessionToken{
		ServiceId:       "unknown",
		ApiSessionToken: nil,
		JwtToken:        nil,
		Claims: &common.ServiceAccessClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "unknown",
				ID:      id,
			},
			ApiSessionId: "unknown",
			IdentityId:   "unknown",
			TokenType:    "from_ctrl",
			Type:         "from_ctrl",
			IsLegacy:     true,
		},
		Type: ServiceSessionTypeLegacyTokenOnly,
	}
}

// AddLoggingFields enriches log entries with service session metadata,
// including hierarchical API session context for comprehensive audit trails
// and debugging across the service authorization stack.
func (s *ServiceSessionToken) AddLoggingFields(logger *logrus.Entry) *logrus.Entry {
	if s != nil {
		logger = logger.WithField("serviceSessionToken", logrus.Fields{
			"serviceId":    s.ServiceId,
			"apiSessionId": s.Claims.ApiSessionId,
			"identityId":   s.Claims.IdentityId,
			"tokenId":      s.Claims.ID,
			"type":         s.Type,
		})

		// Include parent API session context
		if s.ApiSessionToken != nil {
			logger = s.ApiSessionToken.AddLoggingFields(logger)
		}
	}

	return logger
}

// IsLegacyApiSession returns true if this service session is associated with
// a legacy API session token, or an error if no API session is linked.
func (s *ServiceSessionToken) IsLegacyApiSession() (bool, error) {
	if s.ApiSessionToken == nil {
		return false, fmt.Errorf("not linked to an api session token")
	}
	return s.ApiSessionToken.IsLegacy(), nil
}

// RemovedEventName generates a unique event name for service session removal,
// enabling precise cleanup and notification handling at the service level.
func (s *ServiceSessionToken) RemovedEventName() events.EventName {
	return events.EventName(EventRemovedEdgeSession + "-" + s.Claims.ID)
}

// TokenId returns the id that uniquely identifies this token.
func (s *ServiceSessionToken) TokenId() string {
	return s.Claims.ID
}

// Token returns the service session token value. For JWT-based sessions,
// this is the raw JWT for legacy tokens it is the id.
func (s *ServiceSessionToken) Token() string {
	if s.Type == ServiceSessionTypeJwt {
		return s.JwtToken.Raw
	}
	return s.TokenId()
}
