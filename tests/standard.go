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

package tests

import (
	"encoding/json"
	"github.com/Jeffail/gabs"
	"github.com/stretchr/testify/require"
	"gopkg.in/resty.v1"
	"strings"
	"testing"
)

func standardJsonResponseTests(response *resty.Response, expectedStatusCode int, t *testing.T) {
	t.Run("has standard json response ("+response.Request.URL+")", func(t *testing.T) {
		t.Run("response has content type application/json", func(t *testing.T) {
			parts := strings.Split(response.Header().Get("content-type"), ";")
			require.New(t).Equal("application/json", parts[0])
		})

		t.Run("response is parsable", func(t *testing.T) {
			body := response.Body()

			out := map[string]interface{}{}
			err := json.Unmarshal(body, &out)

			require.New(t).NoError(err)
		})

		t.Run("response body is a valid envelope", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			if len(body) == 0 {
				return
			}

			data, err := gabs.ParseJSON(body)
			r.NoError(err)

			hasData := data.ExistsP("data")
			hasError := data.ExistsP("error")

			r.False(hasError, "response has 'error' property, not expected, value: %s", data.StringIndent("", "  "))

			r.True(hasData, "response is missing 'data' property")

			_, err = data.Object("data")
			r.NoError(err, "response envelope does not have an object value for 'data', got body: %s", string(body))

			r.True(data.ExistsP("meta"), "missing a meta property", string(body))

			_, err = data.Object("meta")
			r.NoError(err, "response envelope does not have an object value for 'meta', got body: %s", string(body))
		})

		t.Run("has a valid meta section", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			bodyData, err := gabs.ParseJSON(body)
			r.NoError(err)

			_, err = bodyData.Object("meta")
			r.NoError(err, "property 'meta' was not an object")
		})

		t.Run("has a valid data section", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			bodyData, err := gabs.ParseJSON(body)
			r.NoError(err)

			_, err = bodyData.Object("data", "property 'data' was not an object")

			if err != nil {
				_, err = bodyData.Array("data")
			}

			r.NoError(err, "expected property 'data' to be an object or array")
		})

		t.Run("has the expected HTTP status code", func(t *testing.T) {
			require.New(t).Equal(expectedStatusCode, response.StatusCode())
		})
	})
}

func standardErrorJsonResponseTests(response *resty.Response, expectedErrorCode string, expectedStatusCode int, t *testing.T) {
	t.Run("has standard json error response ("+response.Request.URL+")", func(t *testing.T) {
		t.Run("response has content type application/json", func(t *testing.T) {
			parts := strings.Split(response.Header().Get("content-type"), ";")
			require.New(t).Equal("application/json", parts[0])
		})

		t.Run("response is parsable", func(t *testing.T) {
			body := response.Body()

			out := map[string]interface{}{}
			err := json.Unmarshal(body, &out)

			require.New(t).NoError(err, `could not parse JSON: "%s""`, body)
		})

		t.Run("response body is a valid envelope", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			data, err := gabs.ParseJSON(body)
			r.NoError(err, "could not parse JSON: %s", body)

			hasData := data.ExistsP("data")
			hasError := data.ExistsP("error")

			r.False(hasData, "response has 'data' property, not expected', value: %s", data.Path("data").String())

			r.True(hasError, "response is missing 'error' property")

			_, err = data.Object("error")
			r.NoError(err, "response envelope does not have an object value for 'error', body: %s", string(body))

			r.True(data.ExistsP("meta"), "missing a meta property, body: %s", string(body))

			_, err = data.Object("meta")
			r.NoError(err, "response envelope does not have an object value for 'meta', body: %s", string(body))
		})

		t.Run("has a valid meta section", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			data, err := gabs.ParseJSON(body)
			r.NoError(err)

			r.True(data.ExistsP("meta.apiVersion"), "missing 'meta.apiVersion' property for error response")
			r.True(data.ExistsP("meta.apiEnrollmentVersion"), "missing 'meta.apiEnrollmentVersion' property for error response")
		})

		t.Run("has a valid error section", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			data, err := gabs.ParseJSON(body)
			r.NoError(err)

			r.True(data.ExistsP("error.code"), "missing 'error.code' property for error response")
			r.True(data.ExistsP("error.message"), "missing 'error.message' property for error response")
			r.True(data.ExistsP("error.requestId"), "missing 'error.requestId' property for error response")
		})

		t.Run("has the expected error code", func(t *testing.T) {
			body := response.Body()
			r := require.New(t)

			data, err := gabs.ParseJSON(body)
			r.NoError(err)

			errorCode := data.Path("error.code").Data().(string)

			switch errorCode {
			case "COULD_NOT_VALIDATE":
				r.Equal(expectedErrorCode, errorCode, `response cause: "%s"`, data.Path("error.cause").String())
			default:
				r.Equal(expectedErrorCode, errorCode)
			}

		})

		t.Run("has the expected HTTP status code", func(t *testing.T) {
			require.New(t).Equal(expectedStatusCode, response.StatusCode())
		})
	})
}
