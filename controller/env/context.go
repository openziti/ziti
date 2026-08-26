package env

import (
	"bytes"
	"io"
	"net/http"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/api"
	"github.com/openziti/ziti/v2/controller/response"
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
		Id:             rid,
		ResponseWriter: rw,
		Request:        r,
		Body:           body,
	}

	requestContext.Responder = response.NewResponder(requestContext)

	return requestContext
}
