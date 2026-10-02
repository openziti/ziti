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

package api_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openziti/xweb/v3/middleware"
	"github.com/openziti/ziti/v2/controller/api"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/response"
	"github.com/stretchr/testify/require"
)

func TestTimeoutHandlerRespondsWhenWriteTimeoutIsShort(t *testing.T) {
	const timeout = 300 * time.Millisecond

	tests := []struct {
		name         string
		writeTimeout time.Duration
		handlerDelay time.Duration
		wantStatus   int
	}{
		{"timeout response, write timeout equal to timeout", timeout, 2 * timeout, http.StatusServiceUnavailable},
		{"timeout response, write timeout shorter than timeout", timeout / 3, 2 * timeout, http.StatusServiceUnavailable},
		{"handler response after write timeout", timeout / 3, timeout / 2, http.StatusOK},
	}

	encodings := []middleware.HttpEncoding{
		middleware.HttpEncodingIdentity,
		middleware.HttpEncodingGzip,
		middleware.HttpEncodingBr,
		middleware.HttpEncodingDeflate,
	}

	for _, tt := range tests {
		for _, h2 := range []bool{false, true} {
			for _, encoding := range encodings {
				proto := "http1"
				if h2 {
					proto = "h2"
				}
				t.Run(tt.name+"/"+proto+"/"+string(encoding), func(t *testing.T) {
					t.Parallel()
					req := require.New(t)

					next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						time.Sleep(tt.handlerDelay)
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"data":{}}`))
					})
					handler := api.TimeoutHandler(next, timeout, tt.writeTimeout, apierror.NewTimeoutError(), response.EdgeResponseMapper{})

					// xweb wraps API handlers in its compression middleware, so both the deadline and the
					// status have to make it through its response writer for every encoding.
					srv := httptest.NewUnstartedServer(middleware.NewCompressionHandler(handler))
					srv.Config.WriteTimeout = tt.writeTimeout
					srv.EnableHTTP2 = h2
					srv.StartTLS()
					defer srv.Close()

					r, err := http.NewRequest(http.MethodPost, srv.URL, nil)
					req.NoError(err)
					r.Header.Set(middleware.HttpHeaderAcceptEncoding, string(encoding))

					client := &http.Client{Transport: &http.Transport{
						TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
						ForceAttemptHTTP2:  h2,
						DisableCompression: true,
					}}
					resp, err := client.Do(r)
					req.NoError(err)
					defer func() { _ = resp.Body.Close() }()

					body, err := io.ReadAll(resp.Body)
					req.NoError(err)
					req.Equal(h2, resp.ProtoMajor == 2)
					req.Equal(tt.wantStatus, resp.StatusCode)

					if encoding == middleware.HttpEncodingIdentity {
						req.Empty(resp.Header.Get(middleware.HttpHeaderContentEncoding))
						if tt.wantStatus == http.StatusServiceUnavailable {
							req.Contains(string(body), apierror.TimeoutCode)
						}
					} else {
						req.Equal(string(encoding), resp.Header.Get(middleware.HttpHeaderContentEncoding))
					}
				})
			}
		}
	}
}
