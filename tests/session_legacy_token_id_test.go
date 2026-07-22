//go:build apitests

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

package tests

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/v2/common/eid"
)

// jtiFromServiceToken returns the "jti" (session id) claim from a service-access JWT without
// verifying its signature.
func jtiFromServiceToken(t *testing.T, token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a 3-segment JWT, got %d segments", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to base64-decode JWT payload: %v", err)
	}

	claims := map[string]any{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("failed to parse JWT payload: %v", err)
	}

	jti, _ := claims["jti"].(string)
	return jti
}

// Test_Legacy_Session_Token_Matches_Persisted_Session guards the legacy create-session flow
// against a regression where the JWT was minted with a freshly generated session id before the
// durable Create ran. When a session already existed for the (api-session, type, service), Create
// deduped the entity to the existing id, but the token had already committed to the new id, so the
// client held a token whose id backed no stored session. Every subsequent create-circuit /
// create-terminator then failed to load the session. The token's id must always match the durable
// session's id, including when a create dedups to an existing session.
func Test_Legacy_Session_Token_Matches_Persisted_Session(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	identityRole := eid.New()
	serviceRole := eid.New()

	_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
	clientSession, err := identityAuth.AuthenticateClientApi(ctx)
	ctx.Req.NoError(err)

	service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), nil)
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

	// first create persists a new durable session; its id and token must agree
	resp, err := clientSession.createNewSession(service.Id)
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	first, err := gabs.ParseJSON(resp.Body())
	ctx.Req.NoError(err)
	firstId, ok := first.Path("data.id").Data().(string)
	ctx.Req.True(ok, "first response is missing data.id")
	firstToken, ok := first.Path("data.token").Data().(string)
	ctx.Req.True(ok, "first response is missing data.token")
	ctx.Req.Equal(firstId, jtiFromServiceToken(t, firstToken), "token id must match session id on first create")

	// a second create for the same (api-session, type, service) dedups to the existing session; the
	// returned token must still reference that persisted session id, not a freshly generated one
	resp, err = clientSession.createNewSession(service.Id)
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	second, err := gabs.ParseJSON(resp.Body())
	ctx.Req.NoError(err)
	secondId, ok := second.Path("data.id").Data().(string)
	ctx.Req.True(ok, "second response is missing data.id")
	secondToken, ok := second.Path("data.token").Data().(string)
	ctx.Req.True(ok, "second response is missing data.token")

	ctx.Req.Equal(firstId, secondId, "dedup must return the existing session id")
	ctx.Req.Equal(secondId, jtiFromServiceToken(t, secondToken), "token id must match the persisted (deduped) session id")
}
