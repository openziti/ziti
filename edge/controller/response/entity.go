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
	"net/http"
	"net/url"
)

const ZitiControllerVersionHeader = "ziti-version"

type Identifier interface {
	GetId() string
	GetSelfUrl() url.URL
}

type Response struct {
	Contents       interface{}
	HttpStatusCode int
}

func NewEntityResponse(contents interface{}, httpStatusCode int) (*Response, error) {
	r := &Response{
		Contents:       contents,
		HttpStatusCode: httpStatusCode,
	}
	return r, nil
}

func (r *Response) Respond(w http.ResponseWriter) error {
	jsonObj := gabs.New()

	_, err := jsonObj.Set(r.Contents)

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Error("could not set contents of response")
	}
	AddVersionHeader(w)

	w.WriteHeader(r.HttpStatusCode)
	_, err = w.Write(jsonObj.Bytes())

	if err != nil {
		return fmt.Errorf("could not write to body: %v", err)
	}

	return nil
}
