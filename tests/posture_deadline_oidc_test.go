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

const (
	// mfaDeadlineTimeout is the MFA posture check timeout the deadline tests run with: long
	// enough that the first dial lands while the check still passes, short enough that the test
	// does not spend long waiting for it to expire.
	mfaDeadlineTimeout = 10 * time.Second

	// mfaDeadlineEnforcementBudget bounds how long enforcement may take once the MFA timeout has
	// elapsed. The router sweeps posture deadlines on the same 5s tick it verifies connections
	// on, so the wait covers the timeout, a full sweep interval, and the circuit teardown.
	mfaDeadlineEnforcementBudget = 25 * time.Second

	// postureDeadlineSweepInterval mirrors the router's connection verification tick, which the
	// posture deadline sweep shares. Tests wait past a multiple of it to prove the sweep ran.
	postureDeadlineSweepInterval = 5 * time.Second
)

// requireTotpAuthenticatedContext creates an identity in the given role, enrolls TOTP on it, and
// returns a context authenticated with TOTP so its api session attests totp and seeds the
// router's MFA-passed baseline from auth_time. The returned context supplies no posture-response
// TOTP token, since its provider yields nothing. That models a user who never answers the
// re-prompt, so the seeded baseline is the only thing keeping an MFA posture check passing. The
// returned TotpProvider holds the enrolled secret, so a caller that wants the opposite behavior
// can install a provider that mints posture TOTP tokens.
//
// The router is given time to learn the identity's policies before the context authenticates:
// the MFA timeout runs from auth_time, so anything waited on afterwards eats into the window the
// first dial has to land in.
func requireTotpAuthenticatedContext(ctx *TestContext, identityRole string) (*ziti.ContextImpl, *TotpProvider) {
	enrollIdentity, enrollZtx := ctx.AdminManagementSession.RequireCreateSdkContext(identityRole)
	enrollContext, ok := enrollZtx.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	enrollContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	ctx.Req.NoError(enrollContext.Authenticate())

	mfaDetail, err := enrollZtx.EnrollZitiMfa()
	ctx.Req.NoError(err)
	ctx.Req.NotNil(mfaDetail)

	totpProvider := &TotpProvider{}
	ctx.Req.NoError(totpProvider.ApplyProvisioningUrl(mfaDetail.ProvisioningURL))
	ctx.Req.NoError(enrollZtx.VerifyZitiMfa(totpProvider.Code()))
	enrollZtx.Close()

	er := &EdgeRouterHelper{Router: ctx.routers[0]}
	ctx.Req.True(er.WaitForIdentityWithServices(enrollIdentity.Id, 10*time.Second),
		"identity service policies should reach the router RDM")

	ztx, err := ziti.NewContext(enrollIdentity.config)
	ctx.Req.NoError(err)

	context, ok := ztx.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	context.CtrlClt.SetAllowOidcDynamicallyEnabled(true) // router-evaluated posture is the OIDC path
	ztx.Events().AddMfaTotpCodeListener(func(_ ziti.Context, _ *rest_model.AuthQueryDetail, response ziti.MfaCodeResponse) {
		_ = response(totpProvider.Code())
	})
	context.CtrlClt.PostureCache.SetTotpProviderFunc(func() <-chan edge_apis.TotpTokenResult {
		return nil
	})

	ctx.Req.NoError(context.Authenticate())
	return context, totpProvider
}

// requireMfaDeadlineService builds the service and policies the deadline tests share: one
// service, an MFA posture check with a short timeout on the policy named by mfaPolicyType, and
// nothing on the other policy. Returns the service name, which is what hosting and dialing need.
func requireMfaDeadlineService(ctx *TestContext, mfaPolicyType, dialIdentityRole, hostIdentityRole, serviceRole string) string {
	postureCheckRoleAttr := eid.New()

	adminManagementApi := ctx.NewEdgeManagementApi(nil)
	_, err := adminManagementApi.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	_, err = adminManagementApi.CreatePostureCheckMfa(int64(mfaDeadlineTimeout.Seconds()), false, false, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	svc := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	svc.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(svc)

	dialPostureChecks := s("#" + postureCheckRoleAttr)
	hostPostureChecks := dialPostureChecks
	if mfaPolicyType == "Dial" {
		hostPostureChecks = nil
	} else {
		dialPostureChecks = nil
	}

	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), dialPostureChecks)
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), hostPostureChecks)

	return svc.Name
}

// Test_PostureDeadline_DialCircuit_OnMfaTimeout_OIDC covers the gap where nothing re-evaluated
// posture on the passage of time: an MFA posture check's timeout expires on an idle client whose
// api session token keeps refreshing, and the client's active circuit survived indefinitely
// because no posture response and no policy change arrived to trigger a re-evaluation. The router
// must notice the elapsed deadline on its own and close the circuit.
func Test_PostureDeadline_DialCircuit_OnMfaTimeout_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()

	serviceName := requireMfaDeadlineService(ctx, "Dial", dialIdentityRole, hostIdentityRole, serviceRole)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(serviceName)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	clientContext, _ := requireTotpAuthenticatedContext(ctx, dialIdentityRole)
	defer clientContext.Close()

	clientConn := ctx.WrapConn(clientContext.Dial(serviceName))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("the client's mfa timeout elapses with no re-pass", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("closes the active dial circuit", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnClosedWithin(ctx, clientConn, mfaDeadlineEnforcementBudget)
		})
	})
}

// Test_PostureDeadline_HostTerminator_OnMfaTimeout_OIDC is the hosted-terminator half of the same
// gap: the host's MFA timeout expires while it is hosting, and its terminator must be revoked,
// tearing down the client's active circuit.
func Test_PostureDeadline_HostTerminator_OnMfaTimeout_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()

	serviceName := requireMfaDeadlineService(ctx, "Bind", dialIdentityRole, hostIdentityRole, serviceRole)

	ctx.CreateEnrollAndStartEdgeRouter()

	hostContext, _ := requireTotpAuthenticatedContext(ctx, hostIdentityRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(serviceName)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	_, clientZtx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	clientContext := clientZtx.(*ziti.ContextImpl)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer clientContext.Close()
	ctx.Req.NoError(clientContext.Authenticate())

	clientConn := ctx.WrapConn(clientContext.Dial(serviceName))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("the host's mfa timeout elapses with no re-pass", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("revokes the host terminator, tearing down the active circuit", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnClosedWithin(ctx, clientConn, mfaDeadlineEnforcementBudget)
		})
	})
}

// Test_PostureDeadline_NoRevocation_MfaRepassed_OIDC is the negative control for the deadline
// sweep, and the end-to-end proof that a TOTP token minted after authentication is honored. The
// client re-passes MFA ahead of each deadline, so the router must keep moving the deadline out and
// leave the circuit alone; once the client stops re-passing, the deadline from its last pass
// elapses and the circuit is closed, showing enforcement re-arms rather than latching off after a
// re-pass.
func Test_PostureDeadline_NoRevocation_MfaRepassed_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()

	serviceName := requireMfaDeadlineService(ctx, "Dial", dialIdentityRole, hostIdentityRole, serviceRole)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(serviceName)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	clientContext, totpProvider := requireTotpAuthenticatedContext(ctx, dialIdentityRole)
	defer clientContext.Close()

	// Mint posture TOTP tokens through the context's own controller client, which binds each
	// token to this api session. A token minted from any other session is ignored by the router.
	// The flag is read rather than the provider swapped, since the posture cache reads the
	// provider from its own goroutine.
	withholdTotpTokens := &atomic.Bool{}
	clientContext.CtrlClt.PostureCache.SetTotpProviderFunc(func() <-chan edge_apis.TotpTokenResult {
		if withholdTotpTokens.Load() {
			return nil
		}
		return clientContext.CtrlClt.RequestTotpToken(totpProvider.Code())
	})

	clientConn := ctx.WrapConn(clientContext.Dial(serviceName))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("mfa is re-passed ahead of each deadline", func(t *testing.T) {
		ctx.testContextChanged(t)

		// Each evaluation submits a freshly minted token, advancing the MFA-passed time and with
		// it the deadline. Run past the seeded baseline's own deadline and a full sweep interval,
		// so the sweep has seen a moved deadline at least once.
		repassUntil := time.Now().Add(mfaDeadlineTimeout + postureDeadlineSweepInterval)
		for time.Now().Before(repassUntil) {
			clientContext.CtrlClt.PostureCache.Evaluate()
			time.Sleep(mfaDeadlineTimeout / 4)
		}

		t.Run("leaves the active dial circuit alone", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnUsable(ctx, clientConn)
		})

		t.Run("closes the circuit once the re-passes stop", func(t *testing.T) {
			ctx.testContextChanged(t)

			withholdTotpTokens.Store(true)
			requireConnClosedWithin(ctx, clientConn, mfaDeadlineEnforcementBudget)
		})
	})
}
