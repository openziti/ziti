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

import "context"

// WebHandlerFromRequestContext us a utility function to retrieve a WebHandler reference, that the demux http.Handler
// deferred to, during downstream  http.Handler processing from the http.Request context.
func WebHandlerFromRequestContext(ctx context.Context) *WebHandler {
	if val := ctx.Value(WebHandlerContextKey); val != nil {
		if handler, ok := val.(*WebHandler); ok {
			return handler
		}
	}
	return nil
}

// WebContextFromRequestContext is a utility function to retrieve a *XWebContext reference from the http.Request
// that provides access to XWeb configuration like BindPoint, WebListener, and Config values.
func WebContextFromRequestContext(ctx context.Context) *XWebContext {
	if val := ctx.Value(WebContextKey); val != nil {
		if webContext, ok := val.(*XWebContext); ok {
			return webContext
		}
	}
	return nil
}
