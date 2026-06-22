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
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_SDK_MfaBaselineSeededFromApiSession verifies that a session that authenticated with TOTP
// passes router-enforced MFA posture checks without ever submitting a posture-response TOTP
// token: the router seeds the MFA-passed baseline from the api session token tied to the
// connection (amr totp + auth_time). An identity that has not passed TOTP is the control — its
// MFA check is pushed as failing and its dial is blocked.
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

	// No timeout and no prompts: the posture cache never enters the TOTP refresh window, so the
	// SDK has no reason to submit a posture TOTP token — the session seed is the only MFA source.
	postureCheck, err := adminManagementApi.CreatePostureCheckMfa(-1, false, false, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)
	checkId := *postureCheck.ID()

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

	testServer := newTestServer(listener, func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return conn.server.close()
			}
			conn.WriteString("hello, "+name, time.Second)
		}
	})
	testServer.start()

	identity, enrollCtxIface := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)

	enrollCtx, ok := enrollCtxIface.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	enrollCtx.CtrlClt.SetAllowOidcDynamicallyEnabled(true) // router-evaluated posture is the OIDC path
	ctx.Req.NoError(enrollCtx.Authenticate())

	er := &EdgeRouterHelper{Router: ctx.routers[0]}
	ctx.Req.True(er.WaitForIdentityWithServices(identity.Id, 10*time.Second),
		"identity service policies should reach the router RDM")

	t.Run("without totp at auth the mfa check blocks the dial", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, err := enrollCtx.Dial(service.Name)
		ctx.Req.Error(err, "MFA never passed and no seed applies: the posture gate must block the dial")
	})

	// Enroll TOTP on the first context, then discard it. The second context authenticates fresh,
	// answering the TOTP auth query, so its api session attests totp from the start and its
	// router connections are established under that session.
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

	// Prove no posture TOTP token is ever requested: the test fails if the posture cache asks
	// the app for a TOTP code to exchange for a token.
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

		ctx.Req.NoError(clientCtx.SubscribeToServiceUpdatesFromRouter())

		// The pushed posture state must show the MFA check passing off the session seed alone.
		ctx.Req.Eventually(func() bool {
			passing, found := clientCtx.GetPostureCheckPassing(checkId)
			return found && passing
		}, 15*time.Second, 250*time.Millisecond, "the MFA check should be pushed as passing from the api session seed")

		ctx.Req.False(totpTokenRequested.Load(), "no posture TOTP token may be requested: the session seed is the only MFA source")
	})
}
