/*
	Copyright NetFoundry, Inc.

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
	"github.com/go-openapi/runtime"
	"github.com/openziti/edge/controller/apierror"
	fabricApiError "github.com/openziti/fabric/controller/apierror"
	"io/ioutil"
	"net/http"
	"strings"
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

type EnrollmentContext interface {
	GetParameters() map[string]interface{}
	GetToken() string
	GetData() interface{}
	GetDataAsMap() map[string]interface{}
	GetDataAsByteArray() []byte
	GetCerts() []*x509.Certificate
	GetHeaders() map[string]interface{}
	GetMethod() string
}

type EnrollmentContextHttp struct {
	Headers    map[string]interface{}
	Parameters map[string]interface{}
	Data       interface{}
	Certs      []*x509.Certificate
	Token      string
	Method     string
}

func (context *EnrollmentContextHttp) GetToken() string {
	return context.Token
}

func (context *EnrollmentContextHttp) GetParameters() map[string]interface{} {
	return context.Parameters
}

func (context *EnrollmentContextHttp) GetData() interface{} {
	return context.Data
}

func (context *EnrollmentContextHttp) GetDataAsMap() map[string]interface{} {
	data, ok := context.Data.(map[string]interface{})

	if !ok {
		return nil
	}

	return data
}

func (context *EnrollmentContextHttp) GetMethod() string {
	return context.Method
}

func (context *EnrollmentContextHttp) GetDataAsByteArray() []byte {
	data, ok := context.Data.([]byte)

	if !ok {
		return nil
	}

	return data
}

func (context *EnrollmentContextHttp) GetCerts() []*x509.Certificate {
	return context.Certs
}

func (context *EnrollmentContextHttp) GetHeaders() map[string]interface{} {
	return context.Headers
}

func (context *EnrollmentContextHttp) FillFromHttpRequest(request *http.Request) error {
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

	var enrollData interface{}
	body, _ := ioutil.ReadAll(request.Body)

	contentType := strings.Split(request.Header.Get("content-type"), ";")

	switch contentType[0] {
	case "application/json":
		data := map[string]interface{}{}
		if len(body) > 0 {
			err := json.Unmarshal(body, &data)

			if err != nil {
				err = fabricApiError.GetJsonParseError(err, body)
				apiErr := apierror.NewCouldNotParseBody(err)
				apiErr.AppendCause = true
				return apiErr
			}
		}
		enrollData = data
	case "text/plain":
		enrollData = body
	case "application/pkcs7-mime":
		enrollData = body
	case "application/x-pem-file":
		enrollData = body
	default:
		return apierror.NewInvalidContentType(contentType[0])
	}

	headers := map[string]interface{}{}
	for h, v := range request.Header {
		headers[h] = v
	}

	context.Parameters = parameters
	context.Data = enrollData
	context.Certs = request.TLS.PeerCertificates
	context.Headers = headers

	return nil
}
