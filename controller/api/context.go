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

package api

import (
	"context"
	"fmt"
	"github.com/openziti/fabric/controller/change"
	"github.com/pkg/errors"
	"net/http"
)

type RequestContextImpl struct {
	Responder
	Id             string
	ResponseWriter http.ResponseWriter
	Request        *http.Request
	entityId       string
	entitySubId    string
	Body           []byte
}

const (
	IdPropertyName    = "id"
	SubIdPropertyName = "subId"
)

func (rc *RequestContextImpl) GetId() string {
	return rc.Id
}

func (rc *RequestContextImpl) GetBody() []byte {
	return rc.Body
}

func (rc *RequestContextImpl) GetRequest() *http.Request {
	return rc.Request
}

func (rc *RequestContextImpl) GetResponseWriter() http.ResponseWriter {
	return rc.ResponseWriter
}

func (rc *RequestContextImpl) SetEntityId(id string) {
	rc.entityId = id
}

func (rc *RequestContextImpl) SetEntitySubId(id string) {
	rc.entitySubId = id
}

func (rc *RequestContextImpl) GetEntityId() (string, error) {
	if rc.entityId == "" {
		return "", errors.New("id not found")
	}
	return rc.entityId, nil
}

func (rc *RequestContextImpl) GetEntitySubId() (string, error) {
	if rc.entitySubId == "" {
		return "", errors.New("subId not found")
	}

	return rc.entitySubId, nil
}

func (rc *RequestContextImpl) NewChangeContext() *change.Context {
	changeCtx := change.New().SetSourceType(change.SourceTypeRest).
		SetSourceAuth("fabric").
		SetSourceMethod(rc.GetRequest().Method).
		SetSourceLocal(rc.GetRequest().Host).
		SetSourceRemote(rc.GetRequest().RemoteAddr)

	changeCtx.SetChangeAuthorType(change.AuthorTypeCert)

	if rc.Request.TLS != nil {
		for _, cert := range rc.Request.TLS.PeerCertificates {
			if !cert.IsCA {
				changeCtx.SetChangeAuthorId(cert.Subject.CommonName)
			}
		}
	}

	return changeCtx
}

// ContextKey is used a custom type to avoid accidental context key collisions
type ContextKey string

const ZitiContextKey = ContextKey("context")

func AddRequestContextToHttpContext(r *http.Request, rc RequestContext) {
	ctx := context.WithValue(r.Context(), ZitiContextKey, rc)
	*r = *r.WithContext(ctx)
}

func GetRequestContextFromHttpContext(r *http.Request) (RequestContext, error) {
	val := r.Context().Value(ZitiContextKey)
	if val == nil {
		return nil, fmt.Errorf("value for key %s no found in context", ZitiContextKey)
	}

	requestContext := val.(RequestContext)

	if requestContext == nil {
		return nil, fmt.Errorf("value for key %s is not a request context", ZitiContextKey)
	}

	return requestContext, nil
}
