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

// MaxRequestBodySize is the maximum number of HTTP request body bytes the controller's
// web APIs accept per request. Request bodies are buffered into memory before
// authentication, so this cap bounds the memory an unauthenticated client can consume.
// Requests with larger bodies are rejected with HTTP 413 Request Entity Too Large.
const MaxRequestBodySize = 1024 * 1024
