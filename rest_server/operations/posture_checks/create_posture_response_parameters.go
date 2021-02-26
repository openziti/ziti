// Code generated by go-swagger; DO NOT EDIT.

//
// Copyright NetFoundry, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// __          __              _
// \ \        / /             (_)
//  \ \  /\  / /_ _ _ __ _ __  _ _ __   __ _
//   \ \/  \/ / _` | '__| '_ \| | '_ \ / _` |
//    \  /\  / (_| | |  | | | | | | | | (_| | : This file is generated, do not edit it.
//     \/  \/ \__,_|_|  |_| |_|_|_| |_|\__, |
//                                      __/ |
//                                     |___/

package posture_checks

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"io"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/openziti/edge/rest_model"
)

// NewCreatePostureResponseParams creates a new CreatePostureResponseParams object
// no default values defined in spec.
func NewCreatePostureResponseParams() CreatePostureResponseParams {

	return CreatePostureResponseParams{}
}

// CreatePostureResponseParams contains all the bound params for the create posture response operation
// typically these are obtained from a http.Request
//
// swagger:parameters createPostureResponse
type CreatePostureResponseParams struct {

	// HTTP Request Object
	HTTPRequest *http.Request `json:"-"`

	/*A Posture Response
	  Required: true
	  In: body
	*/
	PostureResponse rest_model.PostureResponseCreate
}

// BindRequest both binds and validates a request, it assumes that complex things implement a Validatable(strfmt.Registry) error interface
// for simple values it will use straight method calls.
//
// To ensure default values, the struct must have been initialized with NewCreatePostureResponseParams() beforehand.
func (o *CreatePostureResponseParams) BindRequest(r *http.Request, route *middleware.MatchedRoute) error {
	var res []error

	o.HTTPRequest = r

	if runtime.HasBody(r) {
		defer r.Body.Close()
		body, err := rest_model.UnmarshalPostureResponseCreate(r.Body, route.Consumer)
		if err != nil {
			if err == io.EOF {
				err = errors.Required("postureResponse", "body", "")
			}
			res = append(res, err)
		} else {
			// validate body object
			if err := body.Validate(route.Formats); err != nil {
				res = append(res, err)
			}

			if len(res) == 0 {
				o.PostureResponse = body
			}
		}
	} else {
		res = append(res, errors.Required("postureResponse", "body", ""))
	}
	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
