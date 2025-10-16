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
	"fmt"
	"net/http"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/event"
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
	GetHeaders() Headers
	GetChangeContext() *change.Context
	GetRemoteAddr() string

	GetEnvInfo() *EnvInfo
	GetSdkInfo() *SdkInfo

	// GetPrimaryIdentity returns the current in context identity, which should be nil for primary and filled for secondary
	GetPrimaryIdentity() *Identity

	// SetPrimaryIdentity sets the identity already verified by a primary authentication method, used during secondary methods
	SetPrimaryIdentity(*Identity)
}

type AuthContextHttp struct {
	Method          string
	Data            map[string]interface{}
	Certs           []*x509.Certificate
	Headers         Headers
	ChangeContext   *change.Context
	PrimaryIdentity *Identity
	RemoteAddr      string
	SdkInfo         *SdkInfo
	EnvInfo         *EnvInfo
}

func (context *AuthContextHttp) GetEnvInfo() *EnvInfo {
	return context.EnvInfo
}

func (context *AuthContextHttp) GetSdkInfo() *SdkInfo {
	return context.SdkInfo
}

func NewAuthContextHttp(request *http.Request, method string, data interface{}, ctx *change.Context) AuthContext {
	//TODO: this is a giant hack to not deal w/ removing the AuthContext layer
	sigh, _ := json.Marshal(data)
	mapData := map[string]interface{}{}
	_ = json.Unmarshal(sigh, &mapData)

	headers := Headers{}
	for h, v := range request.Header {
		headers.Set(h, v)
	}

	sdkInfo, envInfo, err := parseSdkEnvInfo(mapData)

	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to parse sdk and env info, continuing with authentication processing but sdk and env info will not be updated")
	}

	return &AuthContextHttp{
		Method:        method,
		Data:          mapData,
		Certs:         request.TLS.PeerCertificates,
		Headers:       headers,
		ChangeContext: ctx,
		RemoteAddr:    request.RemoteAddr,
		SdkInfo:       sdkInfo,
		EnvInfo:       envInfo,
	}
}

func parseSdkEnvInfo(data map[string]any) (*SdkInfo, *EnvInfo, error) {

	var sdkInfo *SdkInfo
	var envInfo *EnvInfo

	if envInfoInterface := data["envInfo"]; envInfoInterface != nil {
		if envInfoMap := envInfoInterface.(map[string]interface{}); envInfoMap != nil {
			if err := mapstructure.Decode(envInfoMap, &envInfo); err != nil {
				return nil, nil, fmt.Errorf("could not decode key [envInfo] of type %T as %T", envInfoMap, envInfo)
			}
		}
	}

	if sdkInfoInterface := data["sdkInfo"]; sdkInfoInterface != nil {
		if sdkInfoMap := sdkInfoInterface.(map[string]interface{}); sdkInfoMap != nil {
			if err := mapstructure.Decode(sdkInfoMap, &sdkInfo); err != nil {
				return nil, nil, fmt.Errorf("could not decode key [sdkInfo] of type %T as %T", sdkInfoMap, sdkInfo)
			}
		}
	}

	return sdkInfo, envInfo, nil
}

func (context *AuthContextHttp) GetMethod() string {
	return context.Method
}

func (context *AuthContextHttp) GetData() map[string]interface{} {
	return context.Data
}

func (context *AuthContextHttp) GetHeaders() Headers {
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
	TokenIssuer             TokenIssuer
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

	if a.TokenIssuer != nil {
		event.ExternalJwtSignerId = a.TokenIssuer.Id()
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
