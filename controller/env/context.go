package env

import (
	"bytes"
	"io"
	"net/http"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/response"
)

func NewRequestContext(rw http.ResponseWriter, r *http.Request) *response.RequestContext {
	rid := eid.New()

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
