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
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_SDK_MfaBaselineSeededFromApiSession verifies a session that authenticated with TOTP passes
// MFA posture checks without a posture TOTP token, including after a router restart.
func Test_SDK_MfaBaselineSeededFromApiSession(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()
	postureCheckRoleAttr := eid.New()

	adminManagementApi := ctx.NewEdgeManagementApi(nil)
	_, err := adminManagementApi.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	_, err = adminManagementApi.CreatePostureCheckMfa(-1, false, false, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	service.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(service)

	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), s("#"+postureCheckRoleAttr))
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), nil)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	echo := func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return conn.server.close()
			}
			conn.WriteString("hello, "+name, time.Second)
		}
	}

	testServer := newTestServer(listener, echo)
	testServer.start()

	identity, enrollCtxIface := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)

	enrollCtx, ok := enrollCtxIface.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	enrollCtx.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	ctx.Req.NoError(enrollCtx.Authenticate())

	er := &EdgeRouterHelper{Router: ctx.routers[0]}
	ctx.Req.True(er.WaitForIdentityWithServices(identity.Id, 10*time.Second),
		"identity service policies should reach the router RDM")

	t.Run("without totp at auth the mfa check blocks the dial", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, err := enrollCtx.Dial(service.Name)
		ctx.Req.Error(err, "MFA never passed and no seed applies: the posture gate must block the dial")
	})

	mfaDetail, err := enrollCtxIface.EnrollZitiMfa()
	ctx.Req.NoError(err)
	ctx.Req.NotNil(mfaDetail)

	totpProvider := &TotpProvider{}
	ctx.Req.NoError(totpProvider.ApplyProvisioningUrl(mfaDetail.ProvisioningURL))
	ctx.Req.NoError(enrollCtxIface.VerifyZitiMfa(totpProvider.Code()))
	enrollCtxIface.Close()

	ztx, err := ziti.NewContext(identity.config)
	ctx.Req.NoError(err)
	defer ztx.Close()

	clientCtx, ok := ztx.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	clientCtx.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	ztx.Events().AddMfaTotpCodeListener(func(_ ziti.Context, _ *rest_model.AuthQueryDetail, response ziti.MfaCodeResponse) {
		_ = response(totpProvider.Code())
	})

	totpTokenRequested := atomic.Bool{}
	clientCtx.CtrlClt.PostureCache.SetTotpProviderFunc(func() <-chan edge_apis.TotpTokenResult {
		totpTokenRequested.Store(true)
		return nil
	})

	ctx.Req.NoError(clientCtx.Authenticate())

	t.Run("totp at auth seeds the router mfa baseline", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientConn := ctx.WrapConn(clientCtx.Dial(service.Name))
		defer func() { _ = clientConn.Close() }()

		clientConn.WriteString("mfa-seeded", time.Second)
		clientConn.ReadExpected("hello, mfa-seeded", time.Second)

		ctx.Req.False(totpTokenRequested.Load(), "no posture TOTP token may be requested: the session seed is the only MFA source")
	})

	t.Run("the mfa baseline is seeded again after a router restart", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.shutdownRouters()
		er = ctx.startEdgeRouter(nil)
		ctx.Req.True(er.WaitForIdentityWithServices(identity.Id, 10*time.Second),
			"identity service policies should reach the restarted router RDM")

		// in-process shutdown leaves SDK connections open on the old router; a real restart drops them
		hostCtx, ok := hostContext.(*ziti.ContextImpl)
		ctx.Req.True(ok)
		hostCtx.CloseAllEdgeRouterConns()
		clientCtx.CloseAllEdgeRouterConns()

		restartListener, err := hostContext.Listen(service.Name)
		ctx.Req.NoError(err)
		defer func() { _ = restartListener.Close() }()

		newTestServer(restartListener, echo).start()

		var clientConn *TestConn
		ctx.Req.Eventually(func() bool {
			conn, dialErr := clientCtx.Dial(service.Name)
			if dialErr != nil {
				return false
			}
			clientConn = ctx.WrapConn(conn, nil)
			return true
		}, 30*time.Second, 250*time.Millisecond, "the dial should succeed off the api session seed once the SDK reconnects")
		defer func() { _ = clientConn.Close() }()

		clientConn.WriteString("mfa-reseeded", time.Second)
		clientConn.ReadExpected("hello, mfa-reseeded", time.Second)

		ctx.Req.False(totpTokenRequested.Load(), "no posture TOTP token may be requested: the session seed is the only MFA source")
	})
}
