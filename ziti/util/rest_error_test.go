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

package util

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControllerResponseErrorFromParts(t *testing.T) {
	const unauthorizedBody = `{
		"error": {"code": "UNAUTHORIZED", "message": "The request could not be completed. The session is not authorized or the credentials are invalid", "requestId": "aszEJJLVr"},
		"meta": {"apiEnrollmentVersion": "0.0.1", "apiVersion": "0.0.1"}
	}`

	t.Run("401 returns a clean login hint and no server body", func(t *testing.T) {
		err := controllerResponseErrorFromParts("listing identities", http.StatusUnauthorized, "401 Unauthorized", []byte(unauthorizedBody))
		require.EqualError(t, err, "not authorized: your session is invalid or has expired, please run 'ziti edge login' again (401 Unauthorized)")
		require.NotContains(t, err.Error(), "requestId")
		require.NotContains(t, err.Error(), "apiVersion")
	})

	t.Run("403 is treated like 401", func(t *testing.T) {
		err := controllerResponseErrorFromParts("deleting identity", http.StatusForbidden, "403 Forbidden", nil)
		require.EqualError(t, err, "not authorized: your session is invalid or has expired, please run 'ziti edge login' again (403 Forbidden)")
	})

	t.Run("other errors surface the api error code and message, not the raw body", func(t *testing.T) {
		body := `{"error": {"code": "NOT_FOUND", "message": "resource not found"}}`
		err := controllerResponseErrorFromParts("getting identity", http.StatusNotFound, "404 Not Found", []byte(body))
		require.EqualError(t, err, "error getting identity. Status code: 404 Not Found, NOT_FOUND - resource not found")
	})

	t.Run("unparseable body falls back to status only", func(t *testing.T) {
		err := controllerResponseErrorFromParts("creating identity", http.StatusInternalServerError, "500 Internal Server Error", []byte("<html>boom</html>"))
		require.EqualError(t, err, "error creating identity. Status code: 500 Internal Server Error")
	})
}

func TestParseApiErrorMessage(t *testing.T) {
	t.Run("valid envelope", func(t *testing.T) {
		require.Equal(t, "NOT_FOUND - missing", parseApiErrorMessage([]byte(`{"error":{"code":"NOT_FOUND","message":"missing"}}`)))
	})
	t.Run("no error field", func(t *testing.T) {
		require.Equal(t, "", parseApiErrorMessage([]byte(`{"data":{}}`)))
	})
	t.Run("not json", func(t *testing.T) {
		require.Equal(t, "", parseApiErrorMessage([]byte("nope")))
	})
}
