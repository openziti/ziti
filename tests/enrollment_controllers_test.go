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
	"fmt"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/webapis"
)

// Test_EnrollmentControllerList verifies that a successful client identity enrollment response carries
// the cluster's controllers, exposing only client and OIDC API addresses. Non-HA test controllers do
// not register themselves, so the controller list is seeded directly via the manager.
func Test_EnrollmentControllerList(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminManClient := ctx.NewEdgeManagementApi(nil)
	adminManApiSession, err := adminManClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminManApiSession)

	expected := []expectedController{
		{id: "enroll-ctrl-a", clientUrl: "https://ctrl-a.client.controllers.test", oidcUrl: "https://ctrl-a.oidc.controllers.test"},
		{id: "enroll-ctrl-b", clientUrl: "https://ctrl-b.client.controllers.test", oidcUrl: "https://ctrl-b.oidc.controllers.test"},
	}

	for _, ec := range expected {
		// the controller's cert is parsed and distributed via the RDM on create, so a real cert is required
		controllerCa := newTestCa()
		controller := &model.Controller{
			BaseEntity:   models.BaseEntity{Id: ec.id},
			Name:         ec.id,
			CtrlAddress:  "tls:" + ec.id + ".controllers.test:6262",
			CertPem:      controllerCa.certPem,
			Fingerprint:  ec.id + "-fingerprint",
			IsOnline:     true,
			LastJoinedAt: time.Now(),
			ApiAddresses: map[string][]model.ApiAddress{
				webapis.ClientApiBinding:     {{Url: ec.clientUrl, Version: "v1"}},
				webapis.OidcApiBinding:       {{Url: ec.oidcUrl, Version: "v1"}},
				webapis.ManagementApiBinding: {{Url: "https://" + ec.id + ".mgmt.controllers.test", Version: "v1"}},
			},
		}
		ctx.Req.NoError(ctx.Managers.CreateController(controller))
	}

	t.Run("ott enrollment response includes the controller list", func(t *testing.T) {
		ctx.testContextChanged(t)

		newIdentityLoc, err := adminManClient.CreateIdentity(eid.New(), false)
		ctx.Req.NoError(err)

		expiresAt := time.Now().Add(10 * time.Minute).UTC()
		enrollmentLoc, err := adminManClient.CreateEnrollmentOtt(ToPtr(newIdentityLoc.ID), &expiresAt)
		ctx.Req.NoError(err)

		enrollment, err := adminManClient.GetEnrollment(enrollmentLoc.ID)
		ctx.Req.NoError(err)

		clientApi := ctx.NewEdgeClientApi(nil)
		_, controllers, err := clientApi.CompleteOttEnrollmentWithControllers(*enrollment.Token)
		ctx.Req.NoError(err)
		ctx.Req.NoError(assertEnrollmentControllers(expected, controllers))
	})

	t.Run("ottca enrollment response includes the controller list", func(t *testing.T) {
		ctx.testContextChanged(t)

		newIdentityLoc, err := adminManClient.CreateIdentity(eid.New(), false)
		ctx.Req.NoError(err)

		testCa := newTestCa()
		newCaLoc, err := adminManClient.CreateCa(testCa)
		ctx.Req.NoError(err)

		newCa, err := adminManClient.GetCa(newCaLoc.ID)
		ctx.Req.NoError(err)
		err = adminManClient.VerifyCa(*newCa.ID, newCa.VerificationToken.String(), testCa.publicCert, testCa.privateKey)
		ctx.Req.NoError(err)

		expiresAt := time.Now().Add(10 * time.Minute).UTC()
		enrollmentLoc, err := adminManClient.CreateEnrollmentOttCa(ToPtr(newIdentityLoc.ID), newCa.ID, &expiresAt)
		ctx.Req.NoError(err)

		enrollment, err := adminManClient.GetEnrollment(enrollmentLoc.ID)
		ctx.Req.NoError(err)

		clientApi := ctx.NewEdgeClientApi(nil)
		certAuth := testCa.CreateSignedCert(eid.New())
		_, controllers, err := clientApi.CompleteOttCaEnrollmentWithControllers(*enrollment.Token, certAuth.certs, certAuth.key)
		ctx.Req.NoError(err)
		ctx.Req.NoError(assertEnrollmentControllers(expected, controllers))
	})

	t.Run("updb enrollment response includes the controller list and established username", func(t *testing.T) {
		ctx.testContextChanged(t)

		newIdentityLoc, err := adminManClient.CreateIdentity(eid.New(), false)
		ctx.Req.NoError(err)

		username := eid.New()
		expiresAt := time.Now().Add(10 * time.Minute).UTC()
		enrollmentLoc, err := adminManClient.CreateEnrollmentUpdb(ToPtr(newIdentityLoc.ID), &username, &expiresAt)
		ctx.Req.NoError(err)

		enrollment, err := adminManClient.GetEnrollment(enrollmentLoc.ID)
		ctx.Req.NoError(err)

		clientApi := ctx.NewEdgeClientApi(nil)
		_, response, err := clientApi.CompleteUpdbEnrollmentWithResponse(*enrollment.Token, username, eid.New())
		ctx.Req.NoError(err)
		ctx.Req.NotNil(response)
		ctx.Req.Equal(username, response.Username)
		ctx.Req.NoError(assertEnrollmentControllers(expected, response.Controllers))
	})

	t.Run("token enrollment response includes the controller list", func(t *testing.T) {
		ctx.testContextChanged(t)

		authPolicy := createAuthPolicyComponents("controllers-test-ext-jwt")
		authPolicy.Create.Primary.ExtJWT.Allowed = ToPtr(true)
		authPolicy.Detail, err = adminManClient.CreateAuthPolicy(authPolicy.Create)
		ctx.Req.NoError(err)

		extJwt := createExtJwtComponents("controllers-test-enroll-to-token")
		extJwt.Create.EnrollToTokenEnabled = true
		extJwt.Create.EnrollAuthPolicyID = *authPolicy.Detail.ID
		extJwt.Create.EnrollAttributeClaimsSelector = ""
		extJwt.Create.ClaimsProperty = nil
		extJwt.Create.EnrollNameClaimsSelector = ""
		extJwt.Detail, err = adminManClient.CreateExtJwtSigner(extJwt.Create)
		ctx.Req.NoError(err)

		enrollmentJwt, err := newJwtForExtJwtSigner(extJwt, &claimsWithAttributes{})
		ctx.Req.NoError(err)

		clientApi := ctx.NewEdgeClientApi(nil)
		controllers, err := clientApi.CompleteJwtTokenEnrollmentToTokenAuthWithControllers(enrollmentJwt)
		ctx.Req.NoError(err)
		ctx.Req.NoError(assertEnrollmentControllers(expected, controllers))
	})
}

// Test_EnrollmentControllerList_SingleController verifies that with no controllers registered
// (single-controller non-HA mode, where nothing self-registers without raft) the enrollment
// response still carries the running controller, synthesized with its real id and client/OIDC API
// addresses and with the management binding and ctrlAddress filtered out.
func Test_EnrollmentControllerList_SingleController(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminManClient := ctx.NewEdgeManagementApi(nil)
	adminManApiSession, err := adminManClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminManApiSession)

	newIdentityLoc, err := adminManClient.CreateIdentity(eid.New(), false)
	ctx.Req.NoError(err)

	expiresAt := time.Now().Add(10 * time.Minute).UTC()
	enrollmentLoc, err := adminManClient.CreateEnrollmentOtt(ToPtr(newIdentityLoc.ID), &expiresAt)
	ctx.Req.NoError(err)

	enrollment, err := adminManClient.GetEnrollment(enrollmentLoc.ID)
	ctx.Req.NoError(err)

	clientApi := ctx.NewEdgeClientApi(nil)
	_, controllers, err := clientApi.CompleteOttEnrollmentWithControllers(*enrollment.Token)
	ctx.Req.NoError(err)

	ctx.Req.Len(controllers, 1)
	self := controllers[0]
	ctx.Req.NotNil(self.ID)
	ctx.Req.Equal(ctx.EdgeController.AppEnv.GetId(), *self.ID)
	ctx.Req.Nil(self.CtrlAddress)

	_, hasMgmt := self.APIAddresses[webapis.ManagementApiBinding]
	ctx.Req.False(hasMgmt)

	clientAddrs := self.APIAddresses[webapis.ClientApiBinding]
	ctx.Req.NotEmpty(clientAddrs)
	ctx.Req.NotEmpty(clientAddrs[0].URL)

	oidcAddrs := self.APIAddresses[webapis.OidcApiBinding]
	ctx.Req.NotEmpty(oidcAddrs)
	ctx.Req.NotEmpty(oidcAddrs[0].URL)
}

// Test_EnrollmentControllerList_SingleControllerRaft verifies the raft path: a single controller
// running in cluster mode registers ITSELF in the Controller store on leadership (no synthesized
// fallback), and the enrollment response reflects that stored controller with its real id and
// client/OIDC API addresses. The store being non-empty is what proves the raft self-registration
// path (ReadAllForClient takes the BaseList branch, not the synthesis branch).
func Test_EnrollmentControllerList_SingleControllerRaft(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, SingleRaft)
	defer ctx.Teardown()
	ctx.StartServerRaft()

	adminManClient := ctx.NewEdgeManagementApi(nil)
	adminManApiSession, err := adminManClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminManApiSession)

	// raft self-registration (UpdateControllerState) fires asynchronously on the leadership event,
	// so wait for the controller to appear in the store before enrolling
	var storedCount int
	for start := time.Now(); time.Since(start) < 30*time.Second; {
		result, err := ctx.EdgeController.AppEnv.Managers.Controller.BaseList("true limit none")
		ctx.Req.NoError(err)
		storedCount = len(result.Entities)
		if storedCount > 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	ctx.Req.Equal(1, storedCount, "expected the single raft controller to register itself in the store")

	newIdentityLoc, err := adminManClient.CreateIdentity(eid.New(), false)
	ctx.Req.NoError(err)

	expiresAt := time.Now().Add(10 * time.Minute).UTC()
	enrollmentLoc, err := adminManClient.CreateEnrollmentOtt(ToPtr(newIdentityLoc.ID), &expiresAt)
	ctx.Req.NoError(err)

	enrollment, err := adminManClient.GetEnrollment(enrollmentLoc.ID)
	ctx.Req.NoError(err)

	clientApi := ctx.NewEdgeClientApi(nil)
	_, controllers, err := clientApi.CompleteOttEnrollmentWithControllers(*enrollment.Token)
	ctx.Req.NoError(err)

	ctx.Req.Len(controllers, 1)
	self := controllers[0]
	ctx.Req.NotNil(self.ID)
	ctx.Req.Equal(ctx.EdgeController.AppEnv.GetId(), *self.ID)
	ctx.Req.Nil(self.CtrlAddress)

	_, hasMgmt := self.APIAddresses[webapis.ManagementApiBinding]
	ctx.Req.False(hasMgmt)

	clientAddrs := self.APIAddresses[webapis.ClientApiBinding]
	ctx.Req.NotEmpty(clientAddrs)
	ctx.Req.NotEmpty(clientAddrs[0].URL)

	oidcAddrs := self.APIAddresses[webapis.OidcApiBinding]
	ctx.Req.NotEmpty(oidcAddrs)
	ctx.Req.NotEmpty(oidcAddrs[0].URL)
}

type expectedController struct {
	id        string
	clientUrl string
	oidcUrl   string
}

// assertEnrollmentControllers returns an error unless the returned controller list matches the seeded
// controllers: every expected id present, ctrlAddress omitted, client and OIDC addresses correct, and
// the management binding filtered out of the client-facing response.
func assertEnrollmentControllers(expected []expectedController, actual rest_model.ControllersList) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("expected %d controllers, got %d", len(expected), len(actual))
	}

	byId := map[string]*rest_model.ControllerDetail{}
	for _, detail := range actual {
		if detail.ID == nil {
			return fmt.Errorf("controller detail missing id")
		}
		byId[*detail.ID] = detail
	}

	for _, ec := range expected {
		detail, ok := byId[ec.id]
		if !ok {
			return fmt.Errorf("expected controller %s not present in response", ec.id)
		}

		if detail.CtrlAddress != nil {
			return fmt.Errorf("controller %s ctrlAddress should be omitted from client responses, got %s", ec.id, *detail.CtrlAddress)
		}

		if _, ok := detail.APIAddresses[webapis.ManagementApiBinding]; ok {
			return fmt.Errorf("controller %s should not expose the management API binding", ec.id)
		}

		clientAddrs := detail.APIAddresses[webapis.ClientApiBinding]
		if len(clientAddrs) != 1 || clientAddrs[0].URL != ec.clientUrl {
			return fmt.Errorf("controller %s expected client API address %s, got %v", ec.id, ec.clientUrl, clientAddrs)
		}

		oidcAddrs := detail.APIAddresses[webapis.OidcApiBinding]
		if len(oidcAddrs) != 1 || oidcAddrs[0].URL != ec.oidcUrl {
			return fmt.Errorf("controller %s expected OIDC API address %s, got %v", ec.id, ec.oidcUrl, oidcAddrs)
		}
	}

	return nil
}
