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

package router

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/openziti/fabric/rest_model"
)

// DetailRouterReader is a Reader for the DetailRouter structure.
type DetailRouterReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DetailRouterReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewDetailRouterOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 401:
		result := NewDetailRouterUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 404:
		result := NewDetailRouterNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	default:
		return nil, runtime.NewAPIError("response status code does not match any response statuses defined for this endpoint in the swagger spec", response, response.Code())
	}
}

// NewDetailRouterOK creates a DetailRouterOK with default headers values
func NewDetailRouterOK() *DetailRouterOK {
	return &DetailRouterOK{}
}

/*
DetailRouterOK describes a response with status code 200, with default header values.

A single router
*/
type DetailRouterOK struct {
	Payload *rest_model.DetailRouterEnvelope
}

// IsSuccess returns true when this detail router o k response has a 2xx status code
func (o *DetailRouterOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this detail router o k response has a 3xx status code
func (o *DetailRouterOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this detail router o k response has a 4xx status code
func (o *DetailRouterOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this detail router o k response has a 5xx status code
func (o *DetailRouterOK) IsServerError() bool {
	return false
}

// IsCode returns true when this detail router o k response a status code equal to that given
func (o *DetailRouterOK) IsCode(code int) bool {
	return code == 200
}

func (o *DetailRouterOK) Error() string {
	return fmt.Sprintf("[GET /routers/{id}][%d] detailRouterOK  %+v", 200, o.Payload)
}

func (o *DetailRouterOK) String() string {
	return fmt.Sprintf("[GET /routers/{id}][%d] detailRouterOK  %+v", 200, o.Payload)
}

func (o *DetailRouterOK) GetPayload() *rest_model.DetailRouterEnvelope {
	return o.Payload
}

func (o *DetailRouterOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(rest_model.DetailRouterEnvelope)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewDetailRouterUnauthorized creates a DetailRouterUnauthorized with default headers values
func NewDetailRouterUnauthorized() *DetailRouterUnauthorized {
	return &DetailRouterUnauthorized{}
}

/*
DetailRouterUnauthorized describes a response with status code 401, with default header values.

The currently supplied session does not have the correct access rights to request this resource
*/
type DetailRouterUnauthorized struct {
	Payload *rest_model.APIErrorEnvelope
}

// IsSuccess returns true when this detail router unauthorized response has a 2xx status code
func (o *DetailRouterUnauthorized) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this detail router unauthorized response has a 3xx status code
func (o *DetailRouterUnauthorized) IsRedirect() bool {
	return false
}

// IsClientError returns true when this detail router unauthorized response has a 4xx status code
func (o *DetailRouterUnauthorized) IsClientError() bool {
	return true
}

// IsServerError returns true when this detail router unauthorized response has a 5xx status code
func (o *DetailRouterUnauthorized) IsServerError() bool {
	return false
}

// IsCode returns true when this detail router unauthorized response a status code equal to that given
func (o *DetailRouterUnauthorized) IsCode(code int) bool {
	return code == 401
}

func (o *DetailRouterUnauthorized) Error() string {
	return fmt.Sprintf("[GET /routers/{id}][%d] detailRouterUnauthorized  %+v", 401, o.Payload)
}

func (o *DetailRouterUnauthorized) String() string {
	return fmt.Sprintf("[GET /routers/{id}][%d] detailRouterUnauthorized  %+v", 401, o.Payload)
}

func (o *DetailRouterUnauthorized) GetPayload() *rest_model.APIErrorEnvelope {
	return o.Payload
}

func (o *DetailRouterUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(rest_model.APIErrorEnvelope)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewDetailRouterNotFound creates a DetailRouterNotFound with default headers values
func NewDetailRouterNotFound() *DetailRouterNotFound {
	return &DetailRouterNotFound{}
}

/*
DetailRouterNotFound describes a response with status code 404, with default header values.

The requested resource does not exist
*/
type DetailRouterNotFound struct {
	Payload *rest_model.APIErrorEnvelope
}

// IsSuccess returns true when this detail router not found response has a 2xx status code
func (o *DetailRouterNotFound) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this detail router not found response has a 3xx status code
func (o *DetailRouterNotFound) IsRedirect() bool {
	return false
}

// IsClientError returns true when this detail router not found response has a 4xx status code
func (o *DetailRouterNotFound) IsClientError() bool {
	return true
}

// IsServerError returns true when this detail router not found response has a 5xx status code
func (o *DetailRouterNotFound) IsServerError() bool {
	return false
}

// IsCode returns true when this detail router not found response a status code equal to that given
func (o *DetailRouterNotFound) IsCode(code int) bool {
	return code == 404
}

func (o *DetailRouterNotFound) Error() string {
	return fmt.Sprintf("[GET /routers/{id}][%d] detailRouterNotFound  %+v", 404, o.Payload)
}

func (o *DetailRouterNotFound) String() string {
	return fmt.Sprintf("[GET /routers/{id}][%d] detailRouterNotFound  %+v", 404, o.Payload)
}

func (o *DetailRouterNotFound) GetPayload() *rest_model.APIErrorEnvelope {
	return o.Payload
}

func (o *DetailRouterNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(rest_model.APIErrorEnvelope)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
