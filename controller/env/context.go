package env

import (
	"bytes"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/response"
	"io"
	"net/http"
)

// NewRequestContext creates a bare request context for responses rendered outside the normal
// request pipeline. The body read is capped at api.MaxRequestBodySize; a body that exceeds the
// cap is truncated rather than rejected, since this context only renders error responses.
func NewRequestContext(rw http.ResponseWriter, r *http.Request) *response.RequestContext {
	rid := eid.New()

	r.Body = http.MaxBytesReader(rw, r.Body, api.MaxRequestBodySize)
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))

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
