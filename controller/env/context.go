package env

import (
	"bytes"
	"io"
	"net/http"

	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/response"
)

func NewRequestContext(rw http.ResponseWriter, r *http.Request) *response.RequestContext {
	rid := eid.New()

	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))

	requestContext := &response.RequestContext{
		Id:                rid,
		ResponseWriter:    rw,
		Request:           r,
		Body:              body,
		Identity:          nil,
		ApiSession:        nil,
		ActivePermissions: map[string]struct{}{},
	}

	requestContext.Responder = response.NewResponder(requestContext)

	return requestContext
}
