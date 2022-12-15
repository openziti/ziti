// Code generated by go-swagger; DO NOT EDIT.

//
// Copyright NetFoundry Inc.
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

package inspect

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"

	"github.com/go-openapi/runtime"

	"github.com/openziti/fabric/rest_model"
)

// InspectOKCode is the HTTP code returned for type InspectOK
const InspectOKCode int = 200

/*
InspectOK A response to an inspect request

swagger:response inspectOK
*/
type InspectOK struct {

	/*
	  In: Body
	*/
	Payload *rest_model.InspectResponse `json:"body,omitempty"`
}

// NewInspectOK creates InspectOK with default headers values
func NewInspectOK() *InspectOK {

	return &InspectOK{}
}

// WithPayload adds the payload to the inspect o k response
func (o *InspectOK) WithPayload(payload *rest_model.InspectResponse) *InspectOK {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the inspect o k response
func (o *InspectOK) SetPayload(payload *rest_model.InspectResponse) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *InspectOK) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(200)
	if o.Payload != nil {
		payload := o.Payload
		if err := producer.Produce(rw, payload); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}

// InspectUnauthorizedCode is the HTTP code returned for type InspectUnauthorized
const InspectUnauthorizedCode int = 401

/*
InspectUnauthorized The currently supplied session does not have the correct access rights to request this resource

swagger:response inspectUnauthorized
*/
type InspectUnauthorized struct {

	/*
	  In: Body
	*/
	Payload *rest_model.APIErrorEnvelope `json:"body,omitempty"`
}

// NewInspectUnauthorized creates InspectUnauthorized with default headers values
func NewInspectUnauthorized() *InspectUnauthorized {

	return &InspectUnauthorized{}
}

// WithPayload adds the payload to the inspect unauthorized response
func (o *InspectUnauthorized) WithPayload(payload *rest_model.APIErrorEnvelope) *InspectUnauthorized {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the inspect unauthorized response
func (o *InspectUnauthorized) SetPayload(payload *rest_model.APIErrorEnvelope) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *InspectUnauthorized) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(401)
	if o.Payload != nil {
		payload := o.Payload
		if err := producer.Produce(rw, payload); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}
