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

package response

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/internal/version"
	"net/http"
)

type ErrorResponse struct {
	*apierror.ApiError
	Type string
	Args map[string]interface{}
}

func (er *ErrorResponse) Respond(rc *RequestContext) error {
	if er.Args == nil {
		er.Args = map[string]interface{}{}
	}

	jsonObj := gabs.New()

	if _, err := jsonObj.SetP(er.Code, "error.code"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := jsonObj.SetP(er.Message, "error.message"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := jsonObj.SetP(er.Args, "error.args"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := jsonObj.SetP(er.Cause, "error.cause"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	cm := ""
	if er.Cause != nil {
		cm = er.Cause.Error()
	}

	if _, err := jsonObj.SetP(cm, "error.causeMessage"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := jsonObj.SetP(rc.Id.String(), "error.requestId"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := jsonObj.SetP(version.GetApiVersion(), "meta.apiVersion"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := jsonObj.SetP(version.GetApiEnrollmentVersion(), "meta.apiEnrolmentVersion"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if er.Status == 0 {
		log := pfxlog.Logger()
		er.Status = http.StatusInternalServerError
		log.Warn("invalid HTTP status code detected for error response, defaulting to 500")
	}

	AddVersionHeader(rc.ResponseWriter)
	rc.ResponseWriter.WriteHeader(er.Status)
	_, err := rc.ResponseWriter.Write(jsonObj.Bytes())

	if err != nil {
		return fmt.Errorf("could not write to body: %v", err)
	}

	return nil
}

func NewErrorResponse(apiErr *apierror.ApiError, args map[string]interface{}) (*ErrorResponse, error) {
	r := &ErrorResponse{
		ApiError: apiErr,
		Args:     args,
	}
	return r, nil
}
