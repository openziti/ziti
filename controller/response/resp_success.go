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
	"github.com/netfoundry/ziti-edge/controller/entities"
	"net/http"
)

type Responder interface {
	Respond(rc *RequestContext) error
}

type ApiResponseBody struct {
	Meta *Meta       `json:"meta"`
	Data interface{} `json:"data"`
}

type Link struct {
	Href    string `json:"href"`
	Method  string `json:"method,omitempty"`
	Comment string `json:"comment,omitempty"`
}

func NewLink(path string) *Link {
	return &Link{Href: path}
}

type Links map[string]*Link

type Meta map[string]interface{}

func NewApiResponseBody(data interface{}, meta *Meta) *ApiResponseBody {
	if data == nil {
		data = new(entities.Empty)
	}

	if meta == nil {
		meta = &Meta{}
	}

	body := ApiResponseBody{
		Data: data,
		Meta: meta,
	}

	return &body
}

func NewApiEntityResponder(data interface{}, meta *Meta, httpStatusCode int) (*Response, error) {
	rsp, err := NewEntityResponse(NewApiResponseBody(data, meta), httpStatusCode)

	if err != nil {
		return nil, fmt.Errorf("could not create responder: %v", err)
	}

	return rsp, nil
}

func RespondWithOk(data interface{}, meta *Meta, rc *RequestContext) {
	rsp, err := NewApiEntityResponder(data, meta, http.StatusOK)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not create responder")
		return
	}

	err = rsp.Respond(rc.ResponseWriter)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not respond")
		return
	}
}

func RespondWithSimpleCreated(id string, link *Link, rc *RequestContext) {
	json := gabs.New()

	if _, err := json.SetP(id, "id"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := json.SetP(map[string]*Link{"self": link}, "_links"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	RespondWithCreated(json.Data(), nil, link, rc)
}

func RespondWithCreated(data interface{}, meta *Meta, link *Link, rc *RequestContext) {
	rc.ResponseWriter.Header().Set("location", link.Href)

	rsp, err := NewApiEntityResponder(data, meta, http.StatusCreated)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not create responder")
		return
	}

	err = rsp.Respond(rc.ResponseWriter)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not respond")
		return
	}
}
