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

package routes

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"github.com/fullsailor/pkcs7"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"net/http"
	"strings"
)

func init() {
	r := NewEnrollRouter()
	env.AddRouter(r)
}

type EnrollRouter struct {
}

func NewEnrollRouter() *EnrollRouter {
	return &EnrollRouter{}
}

func (ro *EnrollRouter) Register(ae *env.AppEnv) {
	enrollHandler := ae.WrapHandler(ro.enrollHandler, permissions.Always())
	ae.RootRouter.HandleFunc("/enroll", enrollHandler).Methods("POST")
	ae.RootRouter.HandleFunc("/enroll/", enrollHandler).Methods("POST")

	caCertsHandler := ae.WrapHandler(ro.getCaCerts, permissions.Always())
	ae.RootRouter.HandleFunc("/.well-known/est/cacerts", caCertsHandler).Methods("GET")
}

func (ro *EnrollRouter) getCaCerts(ae *env.AppEnv, rc *response.RequestContext) {
	rc.ResponseWriter.Header().Set("Content-Type", "application/pkcs7-mime")
	rc.ResponseWriter.Header().Set("Content-Transfer-Encoding", "base64")
	response.AddVersionHeader(rc.ResponseWriter)
	rc.ResponseWriter.WriteHeader(http.StatusOK)

	// Decode each PEM block in the input and append the ASN.1
	// DER bytes for each certificate therein to the data slice.

	input := ae.Config.CaPems()
	var data []byte

	for len(input) > 0 {
		var block *pem.Block
		block, input = pem.Decode(input)
		data = append(data, block.Bytes...)
	}

	// Build a PKCS#7 degenerate "certs only" structure from that ASN.1 certificates data.
	var err error
	data, err = pkcs7.DegenerateCertificate(data)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected issue creating pkcs7 degenerate: %s", err)
		rc.RequestResponder.RespondWithError(&apierror.ApiError{
			Code:    apierror.UnhandledCode,
			Message: apierror.UnhandledMessage,
			Status:  http.StatusInternalServerError,
		})
		return
	}

	//encode as b64 and write to a string so the string can be written out in 64 byte lines
	//there has to be a standard library for this - it feels strange to have to reinvent this
	//write the bytes out in 64 byte lines...
	bytes := []byte(base64.StdEncoding.EncodeToString(data))
	step := 64
	tot := len(bytes)
	for i := 0; i < tot; i += step {
		if i+step < tot {
			_, _ = rc.ResponseWriter.Write(bytes[i : i+step])
			_, _ = rc.ResponseWriter.Write([]byte("\n"))
		} else {
			_, _ = rc.ResponseWriter.Write(bytes[i:tot])
		}
	}
}

func (ro *EnrollRouter) enrollHandler(ae *env.AppEnv, rc *response.RequestContext) {

	enrollContext := &model.EnrollmentContextHttp{}
	err := enrollContext.FillFromHttpRequest(rc.Request)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	result, err := ae.Handlers.Enrollment.HandleEnroll(enrollContext)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if result == nil {
		rc.RequestResponder.RespondWithUnauthorizedError(rc)
		return
	}

	err = ro.Respond(rc, result)

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error attempting to respond with enrollment response")
	}
}

func (ro *EnrollRouter) Respond(rc *response.RequestContext, enrollmentResult *model.EnrollmentResult) error {
	rc.ResponseWriter.Header().Add("content-type", enrollmentResult.ContentType)

	contentType := strings.Split(enrollmentResult.ContentType, ";")

	if contentType[0] == "application/json" {
		data := map[string]interface{}{}
		err := json.Unmarshal(enrollmentResult.Content, &data)
		if err != nil {
			return err
		}

		apiResponse := response.NewApiResponseBody(data, nil)
		enrollmentResult.Content, err = json.Marshal(apiResponse)

		if err != nil {
			return err
		}
	}

	rc.ResponseWriter.WriteHeader(enrollmentResult.Status)
	_, err := rc.ResponseWriter.Write(enrollmentResult.Content)

	return err
}
