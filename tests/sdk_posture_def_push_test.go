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
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// servicePostureQueryIds collects the posture query ids across all of a service detail's posture
// query sets.
func servicePostureQueryIds(detail *rest_model.ServiceDetail) map[string]struct{} {
	result := map[string]struct{}{}
	if detail == nil {
		return result
	}
	for _, set := range detail.PostureQueries {
		for _, q := range set.PostureQueries {
			if q.ID != nil {
				result[*q.ID] = struct{}{}
			}
		}
	}
	return result
}

// Test_SDK_PostureCheckDefinitionPush verifies the issue's third deliverable — posture-check
// changes pushed to subscribed SDKs — for changes made after subscribe, with polling paused:
//
//  1. A pure DEFINITION edit (the check's MAC list changes; no service, policy, or membership
//     change) reaches the SDK: the router re-evaluates and pushes the pass/fail flip even though
//     no posture data changed on the SDK side.
//  2. Membership ADD (a new check joins the granting policy via its role attribute) reaches the
//     SDK through the service-updated path: the service's posture queries gain the new check and
//     its pushed state arrives.
//  3. Membership REMOVE (the check's role attribute no longer matches the policy) removes the
//     check from the service's posture queries.
func Test_SDK_PostureCheckDefinitionPush(t *testing.T) {
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

	validMac := "00:1B:21:AE:43:97"
	otherMac := "5E:9D:04:C2:7F:38"

	postureCheck, err := adminManagementApi.CreatePostureCheckMac([]string{validMac}, []string{postureCheckRoleAttr})
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

	_, ztx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	defer ztx.Close()

	clientCtx, ok := ztx.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	clientCtx.CtrlClt.SetAllowOidcDynamicallyEnabled(true) // posture push is the OIDC (router-evaluated) path
	ctx.Req.NoError(clientCtx.Authenticate())

	postureCache := clientCtx.CtrlClt.PostureCache
	postureCache.SetMacProviderFunc(func() []string {
		return []string{validMac}
	})

	// Dial with a valid MAC so posture data is on the router, then subscribe.
	clientConn := ctx.WrapConn(clientCtx.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	ctx.Req.NoError(clientCtx.SubscribeToServiceUpdatesFromRouter())

	ctx.Req.Eventually(func() bool {
		passing, found := clientCtx.GetPostureCheckPassing(checkId)
		return found && passing
	}, 15*time.Second, 250*time.Millisecond, "MAC posture check should be pushed as passing with valid posture data")

	// Phase 1 — pure definition edit. The check now allows only a different MAC; the SDK's
	// posture data is untouched and no service or policy changed, so only the definition-change
	// push can tell the SDK, via the router's re-evaluation.
	seqBefore := clientCtx.GetLastPostureSeq()
	ctx.Req.NoError(adminManagementApi.PatchPostureCheck(checkId, &rest_model.PostureCheckMacAddressPatch{
		MacAddresses: []string{otherMac},
	}))

	ctx.Req.Eventually(func() bool {
		passing, found := clientCtx.GetPostureCheckPassing(checkId)
		return found && !passing && clientCtx.GetLastPostureSeq() > seqBefore
	}, 15*time.Second, 250*time.Millisecond,
		"a posture-check definition edit should be pushed: the check flips to failing with a higher seq, with no SDK-side posture data change")

	// Phase 2 — membership add. A second check joins the granting policy via the shared role
	// attribute; the SDK's view of the service should gain its posture query (delivered through
	// the service-updated path with reference closure) and its pushed state should arrive.
	check2, err := adminManagementApi.CreatePostureCheckMac([]string{validMac}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)
	check2Id := *check2.ID()

	ctx.Req.Eventually(func() bool {
		detail, found := clientCtx.GetService(service.Name)
		if !found {
			return false
		}
		_, present := servicePostureQueryIds(detail)[check2Id]
		if !present {
			return false
		}
		passing, stateFound := clientCtx.GetPostureCheckPassing(check2Id)
		return stateFound && passing
	}, 15*time.Second, 250*time.Millisecond,
		"a check added to the granting policy should appear in the pushed service's posture queries with pushed state")

	// Phase 3 — membership remove. Clearing the check's role attribute detaches it from the
	// policy; the service's posture queries should drop it.
	emptyAttrs := rest_model.Attributes{}
	check2Patch := &rest_model.PostureCheckMacAddressPatch{}
	check2Patch.SetRoleAttributes(&emptyAttrs)
	ctx.Req.NoError(adminManagementApi.PatchPostureCheck(check2Id, check2Patch))

	ctx.Req.Eventually(func() bool {
		detail, found := clientCtx.GetService(service.Name)
		if !found {
			return false
		}
		_, present := servicePostureQueryIds(detail)[check2Id]
		return !present
	}, 15*time.Second, 250*time.Millisecond,
		"a check removed from the granting policy should leave the pushed service's posture queries")
}
