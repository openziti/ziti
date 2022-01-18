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
	"encoding/json"
	"github.com/Jeffail/gabs"
	"gopkg.in/resty.v1"
	"gopkg.in/yaml.v2"
	"net/http"
	"testing"
)

func Test_Specs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	var resp *resty.Response

	t.Run("specs can be listed", func(t *testing.T) {
		ctx.testContextChanged(t)
		var err error
		resp, err = ctx.newAnonymousClientApiRequest().Get("specs")
		ctx.Req.NoError(err)

		standardJsonResponseTests(resp, http.StatusOK, t)

		t.Run("contains swagger spec", func(t *testing.T) {
			ctx.testContextChanged(t)
			parsed, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			children, err := parsed.Path("data").Children()
			ctx.Req.NoError(err)

			ctx.Req.Len(children, 1)

			id := children[0].Path("id").Data().(string)

			ctx.Req.Equal("edge-client", id)
		})
	})

	t.Run("swagger spec can be detailed", func(t *testing.T) {
		ctx.testContextChanged(t)

		resp, err := ctx.newAnonymousClientApiRequest().Get("specs/edge-client")
		ctx.Req.NoError(err)

		standardJsonResponseTests(resp, http.StatusOK, t)

		t.Run("contains the swagger spec", func(t *testing.T) {
			ctx.testContextChanged(t)

			parsed, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			data := parsed.Path("data")
			id := data.Path("id").Data().(string)
			ctx.Req.Equal("edge-client", id)
		})
	})

	t.Run("swagger spec body can be retrieved as text/yaml", func(t *testing.T) {
		ctx.testContextChanged(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", "text/yaml").
			Get("specs/edge-client/spec")
		ctx.Req.NoError(err)

		t.Run("has a 200 ok status", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		})

		t.Run("has text/yaml content type", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.Equal("text/yaml", resp.Header().Get("content-type"))
		})

		t.Run("has a yaml parsable body", func(t *testing.T) {
			ctx.testContextChanged(t)
			out := map[string]interface{}{}
			err := yaml.Unmarshal(resp.Body(), &out)
			ctx.Req.NoError(err)

			t.Run("parsed yaml body has non zero length", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotEmpty(out)
			})

		})
	})

	t.Run("swagger spec body can be retrieved as application/json", func(t *testing.T) {
		ctx.testContextChanged(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", "application/json").
			Get("specs/edge-client/spec")
		ctx.Req.NoError(err)

		t.Run("has a 200 ok status", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		})

		t.Run("has application/json content type", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.Equal("application/json", resp.Header().Get("content-type"))
		})

		t.Run("has a json parsable body", func(t *testing.T) {
			ctx.testContextChanged(t)
			out := map[string]interface{}{}
			err := json.Unmarshal(resp.Body(), &out)
			ctx.Req.NoError(err)

			t.Run("parsed json body has non zero length", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotEmpty(out)
			})

		})
	})
}
