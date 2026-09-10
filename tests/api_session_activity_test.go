//go:build apitests

package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/controller/response"
)

// Test_ApiSession_LegacyRestActivity verifies that REST requests carrying a legacy zt-session token
// count as activity on that API session, so the session's idle timeout is reset by API use and not
// only by edge router connections.
func Test_ApiSession_LegacyRestActivity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctrl := ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	collector := ctrl.AppEnv.GetManagers().ApiSession.HeartbeatCollector
	apiSessionId := *ctx.AdminManagementSession.AuthResponse.ID

	lastActivity := func() time.Time {
		at, ok := collector.LastAccessedAt(apiSessionId)
		ctx.Req.True(ok, "expected api session %s to have a last activity time", apiSessionId)
		return *at
	}

	// The collector records wall-clock time, so leave a gap between the reference read and the
	// request under test rather than relying on sub-millisecond resolution.
	settle := func() { time.Sleep(20 * time.Millisecond) }

	t.Run("an anonymous request does not mark activity", func(t *testing.T) {
		ctx.testContextChanged(t)

		before := lastActivity()
		settle()

		resp, err := ctx.newAnonymousManagementApiRequest().Get("/version")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), resp.String())

		ctx.Req.Equal(before, lastActivity())
	})

	t.Run("a request to an authenticated endpoint marks activity", func(t *testing.T) {
		ctx.testContextChanged(t)

		before := lastActivity()
		settle()

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/current-identity")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), resp.String())

		after := lastActivity()
		ctx.Req.True(after.After(before), "expected activity to advance past %s, got %s", before, after)
	})

	t.Run("a request to an anonymous endpoint with a session token marks activity", func(t *testing.T) {
		ctx.testContextChanged(t)

		before := lastActivity()
		settle()

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), resp.String())

		after := lastActivity()
		ctx.Req.True(after.After(before), "expected activity to advance past %s, got %s", before, after)

		t.Run("and the response carries session lifetime headers", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NotEmpty(resp.Header().Get(response.ApiSessionExpirationSecondsHeader))
			ctx.Req.NotEmpty(resp.Header().Get(response.ApiSessionExpiresAtHeader))
		})
	})

	t.Run("current api session reports the latest activity", func(t *testing.T) {
		ctx.testContextChanged(t)

		before := lastActivity()
		settle()

		envelope := &rest_model.CurrentAPISessionDetailEnvelope{}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(envelope).Get("/current-api-session")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), resp.String())
		ctx.Req.NotNil(envelope.Data)

		reported := time.Time(envelope.Data.CachedLastActivityAt)
		ctx.Req.True(reported.After(before), "expected reported activity %s to be after %s", reported, before)
	})
}
