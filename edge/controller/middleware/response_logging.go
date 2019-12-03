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

package middleware

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"net/http"
)

type StatusWriter struct {
	http.ResponseWriter
	status    int
	length    int
	RequestId uuid.UUID
}

func (w *StatusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 500
	}
	n, err := w.ResponseWriter.Write(b)
	w.length += n
	return n, err
}

func UseStatusWriter(next http.Handler) http.Handler {
	log := pfxlog.Logger()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := uuid.New()
		log.WithField("id", rid.String()).
			WithField("method", r.Method).
			WithField("url", r.URL.String()).
			Debugf("handling request[%s]: method [%s] URL [%s]", rid.String(), r.Method, r.URL.String())

		sw := &StatusWriter{
			ResponseWriter: w,
			RequestId:      rid,
		}
		next.ServeHTTP(sw, r)

		log.WithField("id", rid.String()).
			WithField("method", r.Method).
			WithField("url", r.URL.String()).
			WithField("status", sw.status).
			Debugf("responding for request [%s] with status [%d]: method [%s] URL [%s]", sw.RequestId.String(), sw.status, r.Method, r.URL.String())
	})
}
