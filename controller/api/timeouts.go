/*
	Copyright NetFoundry Inc.

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

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/go-openapi/runtime"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/foundation/v2/errorz"
	"net/http"
	"path"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
)

// TimeoutHandler will create a http.Handler that wraps the given http.Handler. If the given timeout is reached, the
// supplied errorz.ApiError and ResponseMapper will be used to create an error response. This handler assumes JSON
// output.
//
// This handler functions by creating a proxy ResponseWriter that is passed to downstream http.Handlers.
// This proxy ResponseWriter is ignored on timeout and panics. On panic, a blank response is returned with a
// 500 Internal Error status code. Downstream handlers are encouraged to implement their own panic recovery.
func TimeoutHandler(next http.Handler, timeout time.Duration, apiErr *errorz.ApiError, mapper ResponseMapper) http.Handler {
	return &timeoutHandler{
		next:           next,
		apiError:       apiErr,
		timeout:        timeout,
		producer:       runtime.JSONProducer(),
		responseMapper: mapper,
	}
}

type timeoutHandler struct {
	next           http.Handler
	timeout        time.Duration
	apiError       *errorz.ApiError
	producer       runtime.Producer
	responseMapper ResponseMapper
}

func (h *timeoutHandler) errorBody(w http.ResponseWriter, r *http.Request) error {
	if h.apiError != nil {
		requestId := ""
		rc, _ := GetRequestContextFromHttpContext(r)
		if rc != nil {
			requestId = rc.GetId()
		}
		apiError := h.responseMapper.MapApiError(requestId, h.apiError)
		err := h.producer.Produce(w, apiError)

		return err
	}
	return errors.New("no timout api error specified")
}

func (h *timeoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancelCtx := context.WithTimeout(r.Context(), h.timeout)
	defer cancelCtx()

	r = r.WithContext(ctx)
	done := make(chan struct{})
	tw := &timeoutWriter{
		writer:  w,
		header:  make(http.Header),
		request: r,
	}
	panicChan := make(chan interface{}, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				pfxlog.Logger().Errorf("panic caught by timeout next: %v\n%v", p, debugz.GenerateLocalStack())
				panicChan <- p
			}
		}()
		h.next.ServeHTTP(tw, r)
		close(done)
	}()
	select {
	case <-panicChan:
		tw.mu.Lock()
		defer tw.mu.Unlock()
		dst := w.Header()
		for k, vv := range tw.header {
			dst[k] = vv
		}
		w.WriteHeader(http.StatusInternalServerError)
	case <-done:
		tw.mu.Lock()
		defer tw.mu.Unlock()
		dst := w.Header()
		for k, vv := range tw.header {
			dst[k] = vv
		}
		if !tw.wroteHeader {
			tw.code = http.StatusOK
		}
		w.WriteHeader(tw.code)
		_, _ = w.Write(tw.buffer.Bytes())
	case <-ctx.Done():
		tw.mu.Lock()
		defer tw.mu.Unlock()

		pfxlog.Logger().WithFields(map[string]interface{}{
			"url":    r.URL,
			"method": r.Method,
		}).Errorf("timeout for request hit, returning Service Unavailable 503")

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = h.errorBody(w, r)
		tw.timedOut = true
	}
}

// timeoutWriter is a proxy http.ResponseWriter that is used by TimeoutHandler.
type timeoutWriter struct {
	writer  http.ResponseWriter
	header  http.Header
	buffer  bytes.Buffer
	request *http.Request

	mu          sync.Mutex
	timedOut    bool
	wroteHeader bool
	code        int
}

var _ http.Pusher = (*timeoutWriter)(nil)

func (tw *timeoutWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := tw.writer.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (tw *timeoutWriter) Header() http.Header { return tw.header }

func (tw *timeoutWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return 0, http.ErrHandlerTimeout
	}
	if !tw.wroteHeader {
		tw.writeHeaderLocked(http.StatusOK)
	}
	return tw.buffer.Write(p)
}

func (tw *timeoutWriter) writeHeaderLocked(code int) {
	checkWriteHeaderCode(code)

	switch {
	case tw.timedOut:
		return
	case tw.wroteHeader:
		if tw.request != nil {
			caller := relevantCaller()
			pfxlog.Logger().Errorf("http: superfluous response.WriteHeader call from %s (%s:%d)", caller.Function, path.Base(caller.File), caller.Line)
		}
	default:
		tw.wroteHeader = true
		tw.code = code
	}
}

func (tw *timeoutWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.writeHeaderLocked(code)
}

func checkWriteHeaderCode(code int) {
	if code < 100 || code > 999 {
		panic(fmt.Sprintf("invalid WriteHeader code %v", code))
	}
}

func relevantCaller() goruntime.Frame {
	pc := make([]uintptr, 16)
	n := goruntime.Callers(1, pc)
	frames := goruntime.CallersFrames(pc[:n])
	var frame goruntime.Frame
	for {
		frame, more := frames.Next()
		if !strings.HasPrefix(frame.Function, "net/http.") && !strings.HasPrefix(frame.Function, "github.com/openziti/fabric/controller/api_impl/timeout") {
			return frame
		}
		if !more {
			break
		}
	}
	return frame
}
