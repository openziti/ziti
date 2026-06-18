package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Test_OidcListener_BindFailure_DoesNotPanic verifies that when a configured web server's
// listener cannot bind (here, an edge-oidc server whose bind point interface is unbindable),
// the controller logs the error and the remaining web servers keep serving, rather than
// panicking on a nil listener during startup.
func Test_OidcListener_BindFailure_DoesNotPanic(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, OidcListenerBindFailure)
	defer ctx.Teardown()

	// StartServer waits for the REST API port to come up; if the failing server panicked
	// during startup, the controller process would abort before this returns.
	ctx.StartServer()

	t.Run("primary server stays up and serves OIDC discovery", func(t *testing.T) {
		ctx.testContextChanged(t)

		discoveryUrl := "https://" + ctx.ApiHost + "/oidc/.well-known/openid-configuration"
		resp, err := ctx.newAnonymousClientApiRequest().Get(discoveryUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		var discovery map[string]interface{}
		ctx.Req.NoError(json.Unmarshal(resp.Body(), &discovery))
		ctx.Req.Contains(discovery, "issuer")
	})
}
