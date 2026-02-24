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

package response

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/permissions"
)

const (
	IdPropertyName    = "id"
	SubIdPropertyName = "subId"
)

// RequestContext carries all state for a single inbound API request as it flows through
// middleware and route handlers. It provides access to the raw HTTP request and response
// writer, the resolved security context, routing metadata (entity type, action, IDs),
// and helpers for building audit change records.
type RequestContext struct {
	Responder

	// Id is the unique identifier generated for this request, used in logging and tracing.
	Id string

	ResponseWriter http.ResponseWriter
	Request        *http.Request

	Api        permissions.Api
	EntityType string
	Action     permissions.Action

	entityId    string
	entitySubId string
	Body        []byte
	StartTime   time.Time

	// SecurityCtx holds the resolved authentication and authorization state for the request.
	SecurityCtx SecurityCtx
}

// GetSecurityTokenCtx returns the raw token context from the resolved security context,
// providing access to bearer tokens and their verification state.
func (rc *RequestContext) GetSecurityTokenCtx() *common.SecurityTokenCtx {
	return rc.SecurityCtx.GetSecurityTokenCtx()
}

// HasJwtSecurityToken returns true if the request context has a valid JWT security token
func (rc *RequestContext) HasJwtSecurityToken() bool {
	if rc.GetSecurityTokenCtx() == nil {
		return false
	}
	verified, err := rc.GetSecurityTokenCtx().GetVerifiedApiSessionToken()
	return err == nil && verified != nil && !verified.IsLegacy
}

// HasLegacySecurityToken returns true if the request context has a legacy zt-session security token
func (rc *RequestContext) HasLegacySecurityToken() bool {
	if rc.GetSecurityTokenCtx() == nil {
		return false
	}
	verified, err := rc.GetSecurityTokenCtx().GetVerifiedApiSessionToken()
	return err == nil && verified != nil && verified.IsLegacy
}

func (rc *RequestContext) GetApi() permissions.Api {
	return rc.Api
}

func (rc *RequestContext) HasPermission(s string) bool {
	_, hasPermission := rc.SecurityCtx.GetPermissions()[s]
	return hasPermission
}

func (rc *RequestContext) GetEntityType() string {
	return rc.EntityType
}

func (rc *RequestContext) GetEntityAction() string {
	if rc.EntityType != "" && string(rc.Action) != "" {
		return fmt.Sprintf("%s.%s", rc.EntityType, rc.Action)
	}
	return ""
}

func (rc *RequestContext) GetAction() permissions.Action {
	return rc.Action
}

func (rc *RequestContext) InitPermissionsContext(api permissions.Api, entityType string, action permissions.Action) {
	rc.Api = api
	rc.EntityType = entityType
	rc.Action = action
}

func (rc *RequestContext) GetId() string {
	return rc.Id
}

func (rc *RequestContext) GetBody() []byte {
	return rc.Body
}

func (rc *RequestContext) GetRequest() *http.Request {
	return rc.Request
}

func (rc *RequestContext) GetResponseWriter() http.ResponseWriter {
	return rc.ResponseWriter
}

type EventLogger interface {
	Log(actorType, actorId, eventType, entityType, entityId, formatString string, formatData []string, data map[interface{}]interface{})
}

func (rc *RequestContext) SetEntityId(id string) {
	rc.entityId = id
}

func (rc *RequestContext) SetEntitySubId(id string) {
	rc.entitySubId = id
}

func (rc *RequestContext) GetEntityId() (string, error) {
	if rc.entityId == "" {
		return "", errors.New("id not found")
	}
	return rc.entityId, nil
}

func (rc *RequestContext) GetEntitySubId() (string, error) {
	if rc.entitySubId == "" {
		return "", errors.New("subId not found")
	}

	return rc.entitySubId, nil
}

func (rc *RequestContext) NewChangeContext() *change.Context {
	identity, _ := rc.SecurityCtx.GetIdentity()
	return rc.NewChangeContextForIdentity(identity)
}

// NewChangeContextForIdentity builds a change.Context attributed to the given identity rather
// than the session's own identity. This is the primary implementation used by NewChangeContext
// and can be called directly when an administrator is acting on behalf of another identity.
func (rc *RequestContext) NewChangeContextForIdentity(identity *model.Identity) *change.Context {
	changeCtx := change.New().SetSourceType(change.SourceTypeRest).
		SetSourceAuth("edge").
		SetSourceMethod(rc.GetRequest().Method).
		SetSourceLocal(rc.GetRequest().Host).
		SetSourceRemote(rc.GetRequest().RemoteAddr)

	if identity != nil {
		changeCtx.SetChangeAuthorType(change.AuthorTypeIdentity).
			SetChangeAuthorId(identity.Id).
			SetChangeAuthorName(identity.Name)
	} else {
		changeCtx.SetChangeAuthorType(change.AuthorTypeUnattributed)
	}

	if rc.Request.Form.Has("traceId") {
		changeCtx.SetTraceId(rc.Request.Form.Get("traceId"))
	}
	return changeCtx
}

// SecurityCtx is the interface that RequestContext uses to access per-request authentication
// and authorization state. The concrete implementation (env.SecurityCtx) resolves identity,
// API session, auth policy, MFA queries, and permission sets lazily and caches results.
// Implementations must be safe for concurrent use within a single request lifetime.
type SecurityCtx interface {
	// GetSecurityTokenCtx returns the raw token context carrying bearer token state.
	GetSecurityTokenCtx() *common.SecurityTokenCtx
	// GetIdentity resolves and returns the identity for the session. When a masquerade
	// is active, the masquerade identity is returned instead.
	GetIdentity() (*model.Identity, error)
	// GetAuthPolicy resolves and returns the auth policy for the session's identity.
	GetAuthPolicy() (*model.AuthPolicy, error)
	// GetApiSession resolves and returns the API session for the request.
	GetApiSession() (*model.ApiSession, error)

	// GetTotp resolves and returns the TOTP MFA configuration for the session's identity.
	GetTotp() (*model.Mfa, error)

	// GetApiSessionWithoutResolve returns the API session only if already resolved.
	GetApiSessionWithoutResolve() (*model.ApiSession, error)
	// GetMfaAuthQueriesWithoutResolve returns MFA challenges only if already resolved.
	GetMfaAuthQueriesWithoutResolve() []*rest_model.AuthQueryDetail
	// GetMfaErrorWithoutResolve returns the MFA error only if already resolved.
	GetMfaErrorWithoutResolve() error

	// IsPartiallyAuthed returns true when primary auth succeeded but secondary auth is pending.
	IsPartiallyAuthed() bool
	// IsFullyAuthed returns true when both primary and secondary auth are satisfied.
	IsFullyAuthed() bool
	// GetPermissions returns the resolved permission set for the session.
	GetPermissions() map[string]struct{}
	// AddToRequest attaches this SecurityCtx to the request's context.
	AddToRequest(r *http.Request)
	// GetError returns the error from primary session resolution, if any.
	GetError() error

	// GetMfaAuthQueries resolves and returns outstanding MFA challenges.
	GetMfaAuthQueries() []*rest_model.AuthQueryDetail
	// GetMfaError resolves and returns any secondary-auth failure.
	GetMfaError() error

	// MasqueradeAsIdentity allows an admin to act as another identity for this request.
	MasqueradeAsIdentity(identity *model.Identity) error
	// EndMasquerade clears any active masquerade.
	EndMasquerade()
}
