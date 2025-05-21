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

package model

import (
	"crypto/x509"
	"encoding/json"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/event"
	"net/http"
	"time"
)

type AuthResult interface {
	AuthenticatorId() string
	SessionCerts() []*x509.Certificate
	Identity() *Identity
	Authenticator() *Authenticator
	AuthPolicy() *AuthPolicy
	IsSuccessful() bool
	ImproperClientCertChain() bool
}

type AuthProcessor interface {
	CanHandle(method string) bool
	Process(context AuthContext) (AuthResult, error)
}

type AuthRegistry interface {
	Add(method AuthProcessor)
	GetByMethod(method string) AuthProcessor
}

type AuthProcessorRegistryImpl struct {
	processors []AuthProcessor
}

func (registry *AuthProcessorRegistryImpl) Add(processor AuthProcessor) {
	registry.processors = append(registry.processors, processor)
}

func (registry *AuthProcessorRegistryImpl) GetByMethod(method string) AuthProcessor {
	for _, processor := range registry.processors {
		if processor.CanHandle(method) {
			return processor
		}
	}
	return nil
}

type AuthContext interface {
	GetMethod() string
	GetData() map[string]interface{}
	GetCerts() []*x509.Certificate
	GetHeaders() map[string]interface{}
	GetChangeContext() *change.Context
	GetRemoteAddr() string

	// GetPrimaryIdentity returns the current in context identity, which should be nil for primary and filled for secondary
	GetPrimaryIdentity() *Identity

	// SetPrimaryIdentity sets the identity already verified by a primary authentication method, used during secondary methods
	SetPrimaryIdentity(*Identity)
}

type AuthContextHttp struct {
	Method          string
	Data            map[string]interface{}
	Certs           []*x509.Certificate
	Headers         map[string]interface{}
	ChangeContext   *change.Context
	PrimaryIdentity *Identity
	RemoteAddr      string
}

func NewAuthContextHttp(request *http.Request, method string, data interface{}, ctx *change.Context) AuthContext {
	//TODO: this is a giant hack to not deal w/ removing the AuthContext layer
	sigh, _ := json.Marshal(data)
	mapData := map[string]interface{}{}
	_ = json.Unmarshal(sigh, &mapData)

	headers := map[string]interface{}{}
	for h, v := range request.Header {
		headers[h] = v
	}

	return &AuthContextHttp{
		Method:        method,
		Data:          mapData,
		Certs:         request.TLS.PeerCertificates,
		Headers:       headers,
		ChangeContext: ctx,
		RemoteAddr:    request.RemoteAddr,
	}
}

func (context *AuthContextHttp) GetMethod() string {
	return context.Method
}

func (context *AuthContextHttp) GetData() map[string]interface{} {
	return context.Data
}

func (context *AuthContextHttp) GetHeaders() map[string]interface{} {
	return context.Headers
}

func (context *AuthContextHttp) GetCerts() []*x509.Certificate {
	return context.Certs
}

func (context *AuthContextHttp) GetChangeContext() *change.Context {
	return context.ChangeContext
}

func (context *AuthContextHttp) GetPrimaryIdentity() *Identity { return context.PrimaryIdentity }

func (context *AuthContextHttp) SetPrimaryIdentity(primaryIdentity *Identity) {
	context.PrimaryIdentity = primaryIdentity
}

func (context *AuthContextHttp) GetRemoteAddr() string {
	return context.RemoteAddr
}

func (context *AuthContextHttp) SetRemoteAddr(addr string) {
	context.RemoteAddr = addr
}

var _ AuthResult = &AuthResultBase{}

type AuthResultBase struct {
	identity                *Identity
	authenticatorId         string
	authenticator           *Authenticator
	sessionCerts            []*x509.Certificate
	authPolicy              *AuthPolicy
	improperClientCertChain bool
	env                     Env
}

func (a *AuthResultBase) AuthenticatorId() string {
	return a.authenticatorId
}

func (a *AuthResultBase) SessionCerts() []*x509.Certificate {
	return a.sessionCerts
}

func (a *AuthResultBase) Identity() *Identity {
	return a.identity
}

func (a *AuthResultBase) Authenticator() *Authenticator {
	if a.authenticator == nil {
		a.authenticator, _ = a.env.GetManagers().Authenticator.Read(a.authenticatorId)
	}
	return a.authenticator
}

func (a *AuthResultBase) ImproperClientCertChain() bool {
	return a.improperClientCertChain
}

func (a *AuthResultBase) AuthPolicy() *AuthPolicy {
	return a.authPolicy
}

func (a *AuthResultBase) IsSuccessful() bool {
	return a.identity != nil
}

type AuthBundle struct {
	Authenticator           *Authenticator
	Identity                *Identity
	AuthPolicy              *AuthPolicy
	ExternalJwtSigner       *ExternalJwtSigner
	ImproperClientCertChain bool
}

func (a *AuthBundle) Apply(event *event.AuthenticationEvent) {
	if a.Authenticator != nil {
		event.AuthenticatorId = a.Authenticator.Id

		// set in scenarios where the identity is not fetched explicitly
		event.IdentityId = a.Authenticator.IdentityId
	}

	if a.Identity != nil {
		event.IdentityId = a.Identity.Id
	}

	if a.AuthPolicy != nil {
		event.AuthPolicyId = a.AuthPolicy.Id
	}

	if a.ExternalJwtSigner != nil {
		event.ExternalJwtSignerId = a.ExternalJwtSigner.Id
	}

	event.ImproperClientCertChain = a.ImproperClientCertChain
}

type BaseAuthenticator struct {
	method string
	env    Env
}

func (a *BaseAuthenticator) NewAuthEventFailure(authCtx AuthContext, bundle *AuthBundle, reason string) *event.AuthenticationEvent {
	result := &event.AuthenticationEvent{
		Namespace:     event.AuthenticationEventNS,
		EventSrcId:    a.env.GetId(),
		Timestamp:     time.Now(),
		EventType:     event.AuthenticationEventTypeFail,
		Method:        a.method,
		FailureReason: reason,
		RemoteAddress: authCtx.GetRemoteAddr(),
	}

	bundle.Apply(result)

	return result
}

func (a *BaseAuthenticator) NewAuthEventSuccess(authCtx AuthContext, bundle *AuthBundle) *event.AuthenticationEvent {
	result := &event.AuthenticationEvent{
		Namespace:     event.AuthenticationEventNS,
		EventSrcId:    a.env.GetId(),
		Timestamp:     time.Now(),
		EventType:     event.AuthenticationEventTypeSuccess,
		Method:        a.method,
		RemoteAddress: authCtx.GetRemoteAddr(),
	}

	bundle.Apply(result)

	return result
}

func (a *BaseAuthenticator) DispatchEvent(event *event.AuthenticationEvent) {
	a.env.GetEventDispatcher().AcceptAuthenticationEvent(event)
}
