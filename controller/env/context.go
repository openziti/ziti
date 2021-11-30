package env

import (
	"bytes"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/eid"
	"io/ioutil"
	"net/http"
)

func NewRequestContext(rw http.ResponseWriter, r *http.Request) *response.RequestContext {
	rid := eid.New()

	body, _ := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewReader(body))

	requestContext := &response.RequestContext{
		Id:                rid,
		ResponseWriter:    rw,
		Request:           r,
		Body:              body,
		Identity:          nil,
		ApiSession:        nil,
		ActivePermissions: []string{},
	}

	requestContext.Responder = response.NewResponder(requestContext)

	return requestContext
}
