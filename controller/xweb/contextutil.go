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

// WebHandlerFromContext us a utility function to retrieve a WebHandler reference, that the demux http.Handler
// deferred to, during downstream  http.Handler processing from the http.Request context.
func WebHandlerFromContext(ctx context.Context) *WebHandler {
	if val := ctx.Value(WebHandlerContextKey); val != nil {
		if handler, ok := val.(*WebHandler); ok {
			return handler
		}
	}
	return nil
}

// ListenAddressFromContext is a utility function to retrieve a BindPoint reference from the http.Request context
// that indicates what interface and port combo the incoming http.Request was handled on.
func ListenAddressFromContext(ctx context.Context) *BindPoint {
	if val := ctx.Value(BindPointContextKey); val != nil {
		if handler, ok := val.(*BindPoint); ok {
			return handler
		}
	}
	return nil
}