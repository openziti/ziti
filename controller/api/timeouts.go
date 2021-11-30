package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/go-openapi/runtime"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/debugz"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
	"path"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
)

func TimeoutHandler(h http.Handler, dt time.Duration, apiErr *errorz.ApiError, mapper ResponseMapper) http.Handler {
	return &timeoutHandler{
		handler:        h,
		apiError:       apiErr,
		dt:             dt,
		producer:       runtime.JSONProducer(),
		resopnseMapper: mapper,
	}
}

type timeoutHandler struct {
	handler        http.Handler
	dt             time.Duration
	apiError       *errorz.ApiError
	producer       runtime.Producer
	resopnseMapper ResponseMapper
}

func (h *timeoutHandler) errorBody(w http.ResponseWriter, r *http.Request) error {
	if h.apiError != nil {
		requestId := ""
		rc, _ := GetRequestContextFromHttpContext(r)
		if rc != nil {
			requestId = rc.GetId()
		}
		apiError := h.resopnseMapper.MapApiError(requestId, h.apiError)
		err := h.producer.Produce(w, apiError)

		return err
	}
	return errors.New("no timout api error specified")
}

func (h *timeoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancelCtx := context.WithTimeout(r.Context(), h.dt)
	defer cancelCtx()

	r = r.WithContext(ctx)
	done := make(chan struct{})
	tw := &timeoutWriter{
		w:   w,
		h:   make(http.Header),
		req: r,
	}
	panicChan := make(chan interface{}, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				pfxlog.Logger().Errorf("panic caught by timeout handler: %v\n%v", p, debugz.GenerateLocalStack())
				panicChan <- p
			}
		}()
		h.handler.ServeHTTP(tw, r)
		close(done)
	}()
	select {
	case <-panicChan:
	case <-done:
		tw.mu.Lock()
		defer tw.mu.Unlock()
		dst := w.Header()
		for k, vv := range tw.h {
			dst[k] = vv
		}
		if !tw.wroteHeader {
			tw.code = http.StatusOK
		}
		w.WriteHeader(tw.code)
		_, _ = w.Write(tw.wbuf.Bytes())
	case <-ctx.Done():
		tw.mu.Lock()
		defer tw.mu.Unlock()
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = h.errorBody(w, r)
		tw.timedOut = true
	}
}

type timeoutWriter struct {
	w    http.ResponseWriter
	h    http.Header
	wbuf bytes.Buffer
	req  *http.Request

	mu          sync.Mutex
	timedOut    bool
	wroteHeader bool
	code        int
}

var _ http.Pusher = (*timeoutWriter)(nil)

func (tw *timeoutWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := tw.w.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (tw *timeoutWriter) Header() http.Header { return tw.h }

func (tw *timeoutWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return 0, http.ErrHandlerTimeout
	}
	if !tw.wroteHeader {
		tw.writeHeaderLocked(http.StatusOK)
	}
	return tw.wbuf.Write(p)
}

func (tw *timeoutWriter) writeHeaderLocked(code int) {
	checkWriteHeaderCode(code)

	switch {
	case tw.timedOut:
		return
	case tw.wroteHeader:
		if tw.req != nil {
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
