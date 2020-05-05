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

package routes

import (
	"encoding/base64"
	"encoding/pem"
	"github.com/fullsailor/pkcs7"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/enroll"
	"github.com/openziti/edge/rest_server/operations/well_known"
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

	ae.Api.EnrollEnrollHandler = enroll.EnrollHandlerFunc(func(params enroll.EnrollParams) middleware.Responder {
		return ae.IsAllowed(ro.enrollHandler, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.Api.EnrollEnrollCaHandler = enroll.EnrollCaHandlerFunc(func(params enroll.EnrollCaParams) middleware.Responder {
		return ae.IsAllowed(ro.enrollHandler, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.Api.EnrollEnrollOttCaHandler = enroll.EnrollOttCaHandlerFunc(func(params enroll.EnrollOttCaParams) middleware.Responder {
		return ae.IsAllowed(ro.enrollHandler, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.Api.EnrollEnrollOttHandler = enroll.EnrollOttHandlerFunc(func(params enroll.EnrollOttParams) middleware.Responder {
		return ae.IsAllowed(ro.enrollHandler, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.Api.EnrollEnrollErOttHandler = enroll.EnrollErOttHandlerFunc(func(params enroll.EnrollErOttParams) middleware.Responder {
		return ae.IsAllowed(ro.enrollHandler, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.Api.WellKnownListWellKnownCasHandler = well_known.ListWellKnownCasHandlerFunc(func(params well_known.ListWellKnownCasParams) middleware.Responder {
		return ae.IsAllowed(ro.getCaCerts, params.HTTPRequest, "", "", permissions.Always())
	})
}

func (ro *EnrollRouter) getCaCerts(ae *env.AppEnv, rc *response.RequestContext) {
	rc.ResponseWriter.Header().Set("content-type", "application/pkcs7-mime")
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
		rc.RespondWithError(&apierror.ApiError{
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
		rc.RespondWithError(err)
		return
	}

	result, err := ae.Handlers.Enrollment.Enroll(enrollContext)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if result == nil {
		rc.RespondWithApiError(apierror.NewUnauthorized())
		return
	}

	//prefer json producer for non ott methods (backwards compat for ott)
	explicitJsonAccept := false
	if accept := rc.Request.Header.Values("accept"); len(accept) == 0 {
		explicitJsonAccept = false //no headers specified
	} else {
		for _, val := range accept {
			if strings.Split(val, ";")[0] == "application/json" {
				explicitJsonAccept = true
			}
		}
	}

	// for non ott enrollment, always return JSON
	//prefer JSON if explicitly acceptable
	if enrollContext.Method != persistence.MethodEnrollOtt || explicitJsonAccept {
		rc.SetProducer(runtime.JSONProducer())
	}

	if producer, ok := rc.GetProducer().(*env.TextProducer); ok {
		response.Respond(rc.ResponseWriter, rc.Id, producer, result.TextContent, http.StatusOK)
		return
	}

	rc.RespondWithOk(result.Content, &rest_model.Meta{})
}
