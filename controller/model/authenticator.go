/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"io/ioutil"
	"net/http"
	"strings"
)

type AuthProcessor interface {
	CanHandle(method string) bool
	Process(context AuthContext) (string, error)
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
	GetParameters() map[string]interface{}
	GetData() interface{}
	GetDataAsMap() map[string]interface{}
	GetCerts() []*x509.Certificate
	GetHeaders() map[string]interface{}
}

type AuthContextHttp struct {
	Method     string
	Headers    map[string]interface{}
	Parameters map[string]interface{}
	Data       interface{}
	Certs      []*x509.Certificate
}

func (context *AuthContextHttp) GetMethod() string {
	return context.Method
}

func (context *AuthContextHttp) GetParameters() map[string]interface{} {
	return context.Parameters
}

func (context *AuthContextHttp) GetData() interface{} {
	return context.Data
}

func (context *AuthContextHttp) GetDataAsMap() map[string]interface{} {
	data, ok := context.Data.(map[string]interface{})

	if !ok {
		return nil
	}

	return data
}

func (context *AuthContextHttp) GetCerts() []*x509.Certificate {
	return context.Certs
}

func (context *AuthContextHttp) GetHeaders() map[string]interface{} {
	return context.Headers
}

func (context *AuthContextHttp) FillFromHttpRequest(request *http.Request) error {
	method := request.URL.Query().Get("method")
	queryValues := request.URL.Query()
	parameters := map[string]interface{}{}

	for key, value := range queryValues {
		parameters[key] = value
	}

	var authData interface{}
	body, _ := ioutil.ReadAll(request.Body)

	contentType := strings.Split(request.Header.Get("content-type"), ";")

	switch contentType[0] {
	case "application/json":
		data := map[string]interface{}{}

		err := json.Unmarshal(body, &data)

		if err != nil {
			err = apierror.GetJsonParseError(err, body)
			apiErr := apierror.NewCouldNotParseBody()
			apiErr.Cause = err
			apiErr.AppendCause = true
			return apiErr
		}
		authData = data
	default:
		return apierror.NewInvalidContentType(contentType[0])
	}

	headers := map[string]interface{}{}
	for h, v := range request.Header {
		headers[h] = v
	}

	context.Method = method
	context.Parameters = parameters
	context.Data = authData
	context.Certs = request.TLS.PeerCertificates
	context.Headers = headers

	return nil
}
