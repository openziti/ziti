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

package xweb

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// DemuxFactory generates a http.Handler that interrogates a http.Request and routes them to WebHandlers. The selected
// WebHandler is added to the context with a key of WebHandlerContextKey. Each DemuxFactory implementation must define
// its own behaviors for an unmatched http.Request.
type DemuxFactory interface {
	Build(handlers []WebHandler) (http.Handler, error)
}

// PathPrefixDemuxFactory is a DemuxFactory that routes http.Request requests to a specific WebHandler from a set of
// WebHandler's by URL path prefixes. A http.Handler for NoHandlerFound can be provided to specify behavior to perform
// when a WebHandler is not selected. By default an empty response with a http.StatusNotFound (404) will be sent.
type PathPrefixDemuxFactory struct {
	NoHandlerFound http.Handler
}

// Build performs WebHandler selection based on URL path prefixes
func (factory *PathPrefixDemuxFactory) Build(handlers []WebHandler) (http.Handler, error) {

	handlerMap := map[string]WebHandler{}

	for _, handler := range handlers {
		if existing, ok := handlerMap[handler.RootPath()]; ok {
			return nil, fmt.Errorf("duplicate root path [%s] detected for both bindings [%s] and [%s]", handler.RootPath(), handler.Binding(), existing.Binding())
		}
		handlerMap[handler.RootPath()] = handler
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for _, handler := range handlers {
			if strings.HasPrefix(request.URL.Path, handler.RootPath()) {

				//store this WebHandler on the request context, useful for logging by downstream http handlers
				ctx := context.WithValue(request.Context(), WebHandlerContextKey, handler)
				newRequest := request.WithContext(ctx)
				handler.ServeHTTP(writer, newRequest)
				return
			}
		}
		factory.noHandlerFound(writer, request)
	}), nil
}

// noHandlerFound provides an empty 404 for unmatched paths if NoHandlerFound is nil, otherwise call and defer to NoHandlerFound.
func (factory *PathPrefixDemuxFactory) noHandlerFound(writer http.ResponseWriter, request *http.Request) {
	if factory.NoHandlerFound != nil {
		//defer to externally specified http.Handler func
		factory.NoHandlerFound.ServeHTTP(writer, request)
		return
	}

	writer.WriteHeader(http.StatusNotFound)
	_, _ = writer.Write([]byte{})
}

// IsHandledDemuxFactory is a DemuxFactory that routes http.Request requests to a specific WebHandler by delegating
// to the WebHandler's IsHandled function.
type IsHandledDemuxFactory struct {
	NoHandlerFound http.Handler
}

// Build performs WebHandler selection based on IsHandled()
func (factory *IsHandledDemuxFactory) Build(handlers []WebHandler) (http.Handler, error) {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for _, handler := range handlers {
			if handler.IsHandler(request) {
				ctx := context.WithValue(request.Context(), WebHandlerContextKey, handler)
				newRequest := request.WithContext(ctx)
				handler.ServeHTTP(writer, newRequest)
				return
			}
		}
		factory.noHandlerFound(writer, request)
	}), nil
}

// noHandlerFound provides an empty 404 for unmatched paths if NoHandlerFound is nil, otherwise call and defer to NoHandlerFound.
func (factory *IsHandledDemuxFactory) noHandlerFound(writer http.ResponseWriter, request *http.Request) {
	if factory.NoHandlerFound != nil {
		//defer to externally specified http.Handler func
		factory.NoHandlerFound.ServeHTTP(writer, request)
		return
	}

	writer.WriteHeader(http.StatusNotFound)
	_, _ = writer.Write([]byte{})
}
