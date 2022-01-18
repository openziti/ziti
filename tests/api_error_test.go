//go:build apitests
// +build apitests

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
	"github.com/google/uuid"
	"net/http"
	"testing"
)

func Test_Api_Errors(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("json API error expected for no accept header", func(t *testing.T) {
		ctx.testContextChanged(t)
		madeUpToken := uuid.New().String()

		resp, err := ctx.newAnonymousClientApiRequest().
			//no accept header set
			Post("enroll?token=" + madeUpToken)

		ctx.Req.NoError(err)

		contentTypeHeaders := resp.Header().Values("content-type")

		ctx.Req.NotEmpty(contentTypeHeaders)
		ctx.Req.Equal("application/json", contentTypeHeaders[0])

		standardErrorJsonResponseTests(resp, "INVALID_ENROLLMENT_TOKEN", http.StatusBadRequest, t)
	})

	t.Run("json API error expected for accept of */*", func(t *testing.T) {
		ctx.testContextChanged(t)
		madeUpToken := uuid.New().String()

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", "*/*").
			Post("enroll?token=" + madeUpToken)

		ctx.Req.NoError(err)

		contentTypeHeaders := resp.Header().Values("content-type")

		ctx.Req.NotEmpty(contentTypeHeaders)
		ctx.Req.Equal("application/json", contentTypeHeaders[0])

		standardErrorJsonResponseTests(resp, "INVALID_ENROLLMENT_TOKEN", http.StatusBadRequest, t)
	})

	t.Run("json API error expected for accept of application/json", func(t *testing.T) {
		ctx.testContextChanged(t)
		madeUpToken := uuid.New().String()

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", "application/json").
			Post("enroll?token=" + madeUpToken)

		ctx.Req.NoError(err)

		contentTypeHeaders := resp.Header().Values("content-type")

		ctx.Req.NotEmpty(contentTypeHeaders)
		ctx.Req.Equal("application/json", contentTypeHeaders[0])

		standardErrorJsonResponseTests(resp, "INVALID_ENROLLMENT_TOKEN", http.StatusBadRequest, t)
	})

	t.Run("json API error expected for accept of application/x-pem-file and application/json", func(t *testing.T) {
		ctx.testContextChanged(t)
		madeUpToken := uuid.New().String()

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", "appliaction/x-pem-file, application/json").
			Post("enroll?token=" + madeUpToken)

		ctx.Req.NoError(err)

		contentTypeHeaders := resp.Header().Values("content-type")

		ctx.Req.NotEmpty(contentTypeHeaders)
		ctx.Req.Equal("application/json", contentTypeHeaders[0])

		standardErrorJsonResponseTests(resp, "INVALID_ENROLLMENT_TOKEN", http.StatusBadRequest, t)
	})

	t.Run("empty body expected on error for an accept of application/x-pem-file", func(t *testing.T) {
		ctx.testContextChanged(t)
		madeUpToken := uuid.New().String()

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", "application/x-pem-file").
			Post("enroll?token=" + madeUpToken)

		ctx.Req.NoError(err)

		contentTypeHeaders := resp.Header().Values("content-type")

		ctx.Req.NotEmpty(contentTypeHeaders)
		ctx.Req.Equal("application/x-pem-file", contentTypeHeaders[0])

		ctx.Req.Empty(resp.Body())
	})
}
