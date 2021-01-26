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

package database

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
)

// FixDataIntegrityHandlerFunc turns a function with the right signature into a fix data integrity handler
type FixDataIntegrityHandlerFunc func(FixDataIntegrityParams, interface{}) middleware.Responder

// Handle executing the request and returning a response
func (fn FixDataIntegrityHandlerFunc) Handle(params FixDataIntegrityParams, principal interface{}) middleware.Responder {
	return fn(params, principal)
}

// FixDataIntegrityHandler interface for that can handle valid fix data integrity params
type FixDataIntegrityHandler interface {
	Handle(FixDataIntegrityParams, interface{}) middleware.Responder
}

// NewFixDataIntegrity creates a new http.Handler for the fix data integrity operation
func NewFixDataIntegrity(ctx *middleware.Context, handler FixDataIntegrityHandler) *FixDataIntegrity {
	return &FixDataIntegrity{Context: ctx, Handler: handler}
}

/*FixDataIntegrity swagger:route POST /database/fix-data-integrity Database fixDataIntegrity

Runs a data integrity scan on the datastore, attempts to fix any issues it can and returns any found issues

Runs a data integrity scan on the datastore, attempts to fix any issues it can, and returns any found issues. Requires admin access. Only once instance may run at a time, including runs of checkDataIntegrity.

*/
type FixDataIntegrity struct {
	Context *middleware.Context
	Handler FixDataIntegrityHandler
}

func (o *FixDataIntegrity) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		r = rCtx
	}
	var Params = NewFixDataIntegrityParams()

	uprinc, aCtx, err := o.Context.Authorize(r, route)
	if err != nil {
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}
	if aCtx != nil {
		r = aCtx
	}
	var principal interface{}
	if uprinc != nil {
		principal = uprinc
	}

	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params, principal) // actually handle the request

	o.Context.Respond(rw, r, route.Produces, route, res)

}
