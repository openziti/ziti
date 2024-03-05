package oidc_auth

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"net/http"
	"strings"
)

// negotiateResponseContentType returns the response content type that should be
// used based on the results of parseAcceptHeader. If the accept header
// cannot be satisfied an error is returned.
func negotiateResponseContentType(r *http.Request) (string, *errorz.ApiError) {
	acceptHeader := r.Header.Get(AcceptHeader)
	contentTypes := parseAcceptHeader(acceptHeader)

	if len(contentTypes) == 0 || acceptHeader == "" {
		return JsonContentType, nil
	}

	for _, contentType := range contentTypes {
		if contentType == JsonContentType {
			return contentType, nil
		} else if contentType == HtmlContentType {
			return HtmlContentType, nil
		}
	}

	return "", newNotAcceptableError(acceptHeader)
}

func negotiateBodyContentType(r *http.Request) (string, *errorz.ApiError) {
	contentType := r.Header.Get(ContentTypeHeader)

	switch contentType {
	case FormContentType:
		return FormContentType, nil
	case JsonContentType:
		return JsonContentType, nil
	}

	return "", &errorz.ApiError{

		Code:        "UNSUPPORTED_MEDIA_TYPE",
		Message:     fmt.Sprintf("the content type: %s, is not supported (supported: %s, %s)", contentType, FormContentType, JsonContentType),
		Status:      0,
		Cause:       nil,
		AppendCause: false,
	}
}

// parseAcceptHeader parses HTTP accept headers and returns an array of supported
// content types sorted by quality factor (0=most desired response type). The return
// strings are the content type only (e.g. "application/json")
func parseAcceptHeader(acceptHeader string) []string {
	parts := strings.Split(acceptHeader, ",")
	contentTypes := make([]string, len(parts))

	for i, part := range parts {
		typeAndFactor := strings.Split(strings.TrimSpace(part), ";")
		contentTypes[i] = typeAndFactor[0]
	}

	return contentTypes
}
