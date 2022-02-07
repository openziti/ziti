/*
	Copyright NetFoundry, Inc.

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

package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"github.com/andybalholm/brotli"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type HttpEncoding string

const (
	HttpHeaderContentLength   = "Content-Length"
	HttpHeaderAcceptEncoding  = "Accept-Encoding"
	HttpHeaderContentEncoding = "Content-Encoding"

	HttpEncodingGzip     = HttpEncoding("gzip")
	HttpEncodingBr       = HttpEncoding("br")
	HttpEncodingDeflate  = HttpEncoding("deflate")
	HttpEncodingIdentity = HttpEncoding("identity")
)

var supportedEncodings = map[HttpEncoding]struct{}{
	HttpEncodingGzip:    {},
	HttpEncodingBr:      {},
	HttpEncodingDeflate: {},
}

var gzPool = sync.Pool{
	New: func() interface{} {
		w := gzip.NewWriter(ioutil.Discard)
		return w
	},
}

var brPool = sync.Pool{
	New: func() interface{} {
		w := brotli.NewWriter(ioutil.Discard)
		return w
	},
}

var deflatePool = sync.Pool{
	New: func() interface{} {
		w, _ := flate.NewWriter(ioutil.Discard, 4)
		return w
	},
}

// NewCompressionHandler will return a http.Handler that should be at the top of a response pipeline (i.e. before any
// other http.handlers that write). The returned handler will handle accept-encoding http header interpretation and
// provide a wrapped writer to all downstream http.handlers that will result in all written content to be compressed if
// possible.
//
// The handler will alter the http responses content encoding header (specified algorithm), content body (compressed),
// and content length header (to match compressed body size). Attempting to set any of these values or alter the
// content response body (including writing more data) after the handler exits may cause issues for the receiving
// client.
func NewCompressionHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncodingHeader := getSupportedAcceptEncoding(r)

		switch acceptEncodingHeader {
		case HttpEncodingGzip:
			handleGZip(w, r, next)
			return
		case HttpEncodingBr:
			handleBr(w, r, next)
			return
		case HttpEncodingDeflate:
			handleDeflate(w, r, next)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getSupportedAcceptEncoding returns the highest priority supported encoding supplied by the client.
// HttpEncodingIdentity (no encoding) is returned if no accept header is supplied, invalid headers are supplied, or
// no supported encodings are supplied.
func getSupportedAcceptEncoding(r *http.Request) HttpEncoding {
	rawHeaders := r.Header.Values(HttpHeaderAcceptEncoding)

	highestSupported := HttpEncodingIdentity
	highestQFactor := float32(-1)

	for _, rawHeader := range rawHeaders {
		rawWeightedHeaders := strings.Split(rawHeader, ",")

		for _, rawWeightedHeader := range rawWeightedHeaders {
			rawWeightedHeader = strings.TrimSpace(rawWeightedHeader)
			rawWeightedSplits := strings.Split(rawWeightedHeader, ";")

			encoding := HttpEncoding(strings.TrimSpace(rawWeightedSplits[0]))
			qFactor := float32(1) //if not specified, 1 is default

			_, isSupported := supportedEncodings[encoding]

			if isSupported {
				// if 2+, we have a qFactor
				if len(rawWeightedSplits) > 1 {
					rawWeight := strings.TrimSpace(rawWeightedSplits[1])

					if !strings.HasPrefix(rawWeight, "q=") {
						continue //we can't parse this, it isn't a weight, give up on the value
					}

					rawWeight = strings.TrimPrefix(rawWeight, "q=")

					if parsedQFactor, err := strconv.ParseFloat(rawWeight, 32); err == nil {
						qFactor = float32(parsedQFactor)
					} else {
						continue //value isn't a parsable float, give up
					}
				}

				//qFactors are 0.0-1.0 values only
				if qFactor >= 0 && qFactor <= 1 && qFactor > highestQFactor {
					highestSupported = encoding
					highestQFactor = qFactor
				}
			}
		}
	}
	return highestSupported
}

// wrappedResponseWriter satisfies http.ResponseWriter and allows the compression handler to redirect
// Write() calls to compression encoder instead of the actual http.ResponseWriter.
type wrappedResponseWriter struct {
	status int
	io.Writer
	http.ResponseWriter
}

// WriteHeader delays writing the status header till after compression is complete. This is done
// so that the content length header can be properly set. Prematurely calling WriteHeader()
// will cause all subsequent header changes to not be applied.
func (w *wrappedResponseWriter) WriteHeader(status int) {
	w.status = status
}

// Write proxies the normal Write() to instead run through the compression encoder. Actual writing
// to the http.ResponseWriter is handled via a defer'ed function call.
func (w *wrappedResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// CloseHeaderSection is used by the encoder specific function handler to apply the
// requested HTTP status and close the header section. This is called during the encoders
// defer'ed section to occur after all content is written. Emulates
// net/https default http.StatusOk value.
func (w *wrappedResponseWriter) CloseHeaderSection() {
	if w.status == 0 {
		//emulate default status ok behaviour
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)
}

// handleGZip pulls a gzip encoder from the pool encoders and sets it as the writer
// for the response. The next http.Handler is then invoked and when finished
// a deferred function will then pull the compressed contents out of the encoder
// and set the appropriate http headers.
func handleGZip(w http.ResponseWriter, r *http.Request, next http.Handler) {
	gz := gzPool.Get().(*gzip.Writer)
	defer gzPool.Put(gz)

	var b bytes.Buffer
	gz.Reset(&b)

	wrappedWriter := &wrappedResponseWriter{ResponseWriter: w, Writer: gz}

	defer func() {
		_ = gz.Close()
		length := len(b.Bytes())
		w.Header().Set(HttpHeaderContentEncoding, string(HttpEncodingGzip))
		w.Header().Set(HttpHeaderContentLength, fmt.Sprint(length))
		wrappedWriter.CloseHeaderSection()
		_, _ = w.Write(b.Bytes())
	}()

	next.ServeHTTP(wrappedWriter, r)
}

// handleDeflate pulls a deflate encoder from the pool encoders and sets it as the writer
// for the response. The next http.Handler is then invoked and when finished
// a deferred function will then pull the compressed contents out of the encoder
// and set the appropriate http headers.
func handleDeflate(w http.ResponseWriter, r *http.Request, next http.Handler) {
	deflate := deflatePool.Get().(*flate.Writer)
	defer deflatePool.Put(deflate)

	var b bytes.Buffer
	deflate.Reset(&b)

	wrappedWriter := &wrappedResponseWriter{ResponseWriter: w, Writer: deflate}

	defer func() {
		_ = deflate.Close()
		w.Header().Set(HttpHeaderContentEncoding, string(HttpEncodingDeflate))
		w.Header().Set(HttpHeaderContentLength, fmt.Sprint(len(b.Bytes())))
		wrappedWriter.CloseHeaderSection()
		_, _ = w.Write(b.Bytes())
	}()

	next.ServeHTTP(wrappedWriter, r)
}

// handleBr pulls a brotli encoder from the pool encoders and sets it as the writer
// for the response. The next http.Handler is then invoked and when finished
// a deferred function will then pull the compressed contents out of the encoder
// and set the appropriate http headers.
func handleBr(w http.ResponseWriter, r *http.Request, next http.Handler) {
	w.Header().Set(HttpHeaderContentEncoding, string(HttpEncodingBr))

	br := brPool.Get().(*brotli.Writer)
	defer brPool.Put(br)

	var b bytes.Buffer
	br.Reset(&b)

	wrappedWriter := &wrappedResponseWriter{ResponseWriter: w, Writer: br}

	defer func() {
		_ = br.Close()
		w.Header().Set(HttpHeaderContentLength, fmt.Sprint(len(b.Bytes())))
		_, _ = w.Write(b.Bytes())
	}()

	next.ServeHTTP(wrappedWriter, r)
}
