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
	"net/http"
	"strings"

	"github.com/go-openapi/runtime"
	"github.com/openziti/ziti/controller/change"
)

type EnrollmentResult struct {
	Identity      *Identity
	Authenticator *Authenticator
	Content       interface{}
	TextContent   []byte
	Producer      runtime.Producer
	Status        int
}

type EnrollmentProcessor interface {
	CanHandle(method string) bool
	Process(context EnrollmentContext) (*EnrollmentResult, error)
}

type EnrollmentRegistry interface {
	Add(method EnrollmentProcessor)
	GetByMethod(method string) EnrollmentProcessor
}

type EnrollmentRegistryImpl struct {
	processors []EnrollmentProcessor
}

func (registry *EnrollmentRegistryImpl) Add(processor EnrollmentProcessor) {
	registry.processors = append(registry.processors, processor)
}

func (registry *EnrollmentRegistryImpl) GetByMethod(method string) EnrollmentProcessor {
	for _, processor := range registry.processors {
		if processor.CanHandle(method) {
			return processor
		}
	}

	return nil
}

type Headers map[string]interface{}

func (h *Headers) Set(key string, value interface{}) {
	key = strings.ToLower(key)
	(*h)[key] = value
}

func (h *Headers) Get(key string) interface{} {
	key = strings.ToLower(key)
	return (*h)[key]
}

func (h *Headers) Remove(key string) {
	key = strings.ToLower(key)
	delete(*h, key)
}

func (h *Headers) GetStrings(key string) []string {
	key = strings.ToLower(key)
	val := h.Get(key)

	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	}

	return nil
}

type EnrollmentContext interface {
	GetParameters() map[string]interface{}
	GetToken() string
	GetData() *EnrollmentData
	GetCerts() []*x509.Certificate
	GetHeaders() Headers
	GetMethod() string
	GetChangeContext() *change.Context
}

type EnrollmentData struct {
	RequestedName string
	ServerCsrPem  []byte
	ClientCsrPem  []byte
	Username      string
	Password      string
}

type EnrollmentContextHttp struct {
	Headers       Headers
	Parameters    map[string]interface{}
	Data          *EnrollmentData
	Certs         []*x509.Certificate
	Token         string
	Method        string
	ChangeContext *change.Context
}

func (context *EnrollmentContextHttp) GetToken() string {
	return context.Token
}

func (context *EnrollmentContextHttp) GetParameters() map[string]interface{} {
	return context.Parameters
}

func (context *EnrollmentContextHttp) GetData() *EnrollmentData {
	return context.Data
}

func (context *EnrollmentContextHttp) GetMethod() string {
	return context.Method
}

func (context *EnrollmentContextHttp) GetCerts() []*x509.Certificate {
	return context.Certs
}

func (context *EnrollmentContextHttp) GetHeaders() Headers {
	return context.Headers
}

func (context *EnrollmentContextHttp) GetChangeContext() *change.Context {
	return context.ChangeContext
}

func (context *EnrollmentContextHttp) FillFromHttpRequest(request *http.Request, changeCtx *change.Context) error {
	context.SetParametersFromRequest(request)
	context.SetHeadersFromRequest(request)

	context.Certs = request.TLS.PeerCertificates
	context.ChangeContext = changeCtx.SetChangeAuthorType("enrollment")

	return nil
}

func (context *EnrollmentContextHttp) SetHeadersFromRequest(request *http.Request) {
	headers := Headers{}
	for h, v := range request.Header {
		headers[strings.ToLower(h)] = v
	}

	context.Headers = headers
}

func (context *EnrollmentContextHttp) SetParametersFromRequest(request *http.Request) {
	queryValues := request.URL.Query()
	parameters := map[string]interface{}{}

	for key, value := range queryValues {
		parameters[key] = value

		if key == "token" && len(value) >= 1 {
			context.Token = value[0]
		} else if key == "method" {
			context.Method = value[0]
		}
	}

	context.Parameters = parameters
}
