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

package oidc_auth

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// negotiateResponseContentType returns the response content type that should be
// used based on the results of parseAcceptHeader. If the accept header
// cannot be satisfied an error is returned.
func negotiateResponseContentType(r *http.Request) (string, *errorz.ApiError) {
	acceptHeader := r.Header.Get(AcceptHeader)

	if acceptHeader == "" {
		return JsonContentType, nil
	}

	contentTypes := parseAcceptHeader(acceptHeader)

	if len(contentTypes) == 0 {
		return JsonContentType, nil
	}

	for _, contentType := range contentTypes {
		if contentType.Match(JsonContentType) {
			return JsonContentType, nil
		} else if contentType.Match(HtmlContentType) {
			return HtmlContentType, nil
		}
	}

	return "", newNotAcceptableError(acceptHeader)
}

// parseContentType extracts the media type from a Content-Type header value
func parseContentTypeMediaType(contentType string) string {
	contentType = strings.TrimSpace(contentType)

	if contentType == "" {
		return ""
	}
	parts := strings.Split(contentType, ";")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func negotiateBodyContentType(r *http.Request) (string, *errorz.ApiError) {
	contentType := r.Header.Get(ContentTypeHeader)

	contentType = parseContentTypeMediaType(contentType)

	switch contentType {
	case FormContentType:
		return FormContentType, nil
	case JsonContentType:
		return JsonContentType, nil
	}

	return "", &errorz.ApiError{

		Code:        "UNSUPPORTED_MEDIA_TYPE",
		Message:     fmt.Sprintf("the content type: %s, is not supported (supported: %s, %s)", contentType, FormContentType, JsonContentType),
		Status:      http.StatusUnsupportedMediaType,
		Cause:       nil,
		AppendCause: false,
	}
}

// AcceptEntry represents a parsed Accept header entry
type AcceptEntry struct {
	FullHeader string  // Original full header
	MediaType  string  // Parsed media type
	QValue     float64 // Quality factor (default 1.0)
}

// Match checks if a given media type matches this AcceptEntry, supporting wildcards.
func (a AcceptEntry) Match(mediaType string) bool {
	// Split main and subtype (e.g., "image/png" -> "image", "png")
	expectedParts := strings.SplitN(a.MediaType, "/", 2)
	incomingParts := strings.SplitN(mediaType, "/", 2)

	if len(expectedParts) != 2 || len(incomingParts) != 2 {
		return false // Invalid format
	}

	if a.MediaType == "*/*" {
		return true // Matches anything
	}

	// main type and sup type match exactly or *
	return (expectedParts[0] == incomingParts[0] || expectedParts[0] == "*") &&
		(expectedParts[1] == incomingParts[1] || expectedParts[1] == "*")
}

// parseAcceptHeader parses an Accept header string and returns a sorted slice of AcceptEntry based on q value (high to low)
func parseAcceptHeader(header string) []AcceptEntry {
	if header == "" {
		return nil
	}

	entries := strings.Split(header, ",")
	var acceptHeaders []AcceptEntry

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		parts := strings.Split(entry, ";")

		mediaType := strings.TrimSpace(parts[0])
		qValue := 1.0 // Default q-value

		// Check if there's a q= parameter
		if len(parts) > 1 {
			for _, param := range parts[1:] {
				param = strings.TrimSpace(param)
				if strings.HasPrefix(param, "q=") {
					if val, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
						qValue = val
					}
				}
			}
		}

		acceptHeaders = append(acceptHeaders, AcceptEntry{
			FullHeader: entry,
			MediaType:  mediaType,
			QValue:     qValue,
		})
	}

	// Sort by q-value (descending)
	sort.Slice(acceptHeaders, func(i, j int) bool {
		return acceptHeaders[i].QValue > acceptHeaders[j].QValue
	})

	return acceptHeaders
}
