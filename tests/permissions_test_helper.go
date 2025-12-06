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
	"math"
	"net/http"
	"time"

	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/common/eid"
)

// Helper types and functions
type permissionTestHelper struct{}

func (self permissionTestHelper) newIdentityWithPermissions(ctx *TestContext, perms []string) *session {
	username := eid.New()
	password := eid.New()

	identityCreate := &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr("default"),
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Updb: username,
		},
		IsAdmin:     ToPtr(false),
		Name:        ToPtr(eid.New()),
		Permissions: (*rest_model.Permissions)(&perms),
		Type:        ToPtr(rest_model.IdentityTypeUser),
	}

	identityCreateResp := &rest_model.CreateEnvelope{}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
		SetResult(identityCreateResp).
		SetBody(identityCreate).
		Post("/identities")

	ctx.Req.NoError(err)
	ctx.Req.NotNil(resp)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

	// Complete UPDB enrollment
	ctx.completeUpdbEnrollment(identityCreateResp.Data.ID, password)

	// Create authenticator and login
	userAuth := &updbAuthenticator{
		Username: username,
		Password: password,
	}

	userSession, err := userAuth.AuthenticateManagementApi(ctx)
	ctx.Req.NoError(err)

	return userSession
}

func (self permissionTestHelper) createIdentity(ctx *TestContext, session *session, name string, expectedStatus int) string {
	identityType := rest_model.IdentityTypeUser
	identityCreate := &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr("default"),
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Ott: true,
		},
		IsAdmin: ToPtr(false),
		Name:    ToPtr(name),
		Type:    &identityType,
	}

	identityCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identityCreateResp).
		SetBody(identityCreate).
		Post("/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expectedStatus, resp.StatusCode(), string(resp.Body()))

	if identityCreateResp.Data != nil {
		return identityCreateResp.Data.ID
	}
	return ""
}

func (self permissionTestHelper) getIdentity(ctx *TestContext, session *session, id string, expected int) *rest_model.IdentityDetail {
	identityResp := &rest_model.DetailIdentityEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identityResp).
		Get("/identities/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return identityResp.Data
}

func (self permissionTestHelper) listIdentities(ctx *TestContext, session *session, expected int) {
	identitiesResp := &rest_model.ListIdentitiesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identitiesResp).
		Get("/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

func (self permissionTestHelper) updateIdentity(ctx *TestContext, session *session, detail *rest_model.IdentityDetail, expected int) {
	identityUpdate := &rest_model.IdentityUpdate{}
	copyRestModelFields(detail, identityUpdate, ctx)
	identityUpdate.Type = ToPtr(rest_model.IdentityTypeUser)
	identityUpdate.RoleAttributes = ToPtr(rest_model.Attributes{"updated"})

	resp, err := session.newAuthenticatedRequest().
		SetBody(identityUpdate).
		Put("/identities/" + *detail.ID)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

func (self permissionTestHelper) deleteIdentity(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/identities/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

func (self permissionTestHelper) createConfig(ctx *TestContext, session *session, name string) *rest_model.CreateEnvelope {
	// First, we need to get or create a config type
	configTypeId := self.getOrCreateConfigType(ctx, "test-config-type")

	configCreate := &rest_model.ConfigCreate{
		ConfigTypeID: ToPtr(configTypeId),
		Name:         ToPtr(name),
		Data:         map[string]interface{}{"test": "data"},
	}

	configCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configCreateResp).
		SetBody(configCreate).
		Post("/configs")

	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

	return configCreateResp
}

func (self permissionTestHelper) getConfig(ctx *TestContext, session *session, id string) *rest_model.ConfigDetail {
	configResp := &rest_model.DetailConfigEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configResp).
		Get("/configs/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

	if configResp.Data != nil {
		return configResp.Data
	}
	return nil
}

func (self permissionTestHelper) updateConfig(ctx *TestContext, session *session, detail *rest_model.ConfigDetail, expected int) {
	configUpdate := &rest_model.ConfigUpdate{}
	copyRestModelFields(detail, configUpdate, ctx)
	configUpdate.Data = map[string]interface{}{"test": "updated"}

	resp, err := session.newAuthenticatedRequest().
		SetBody(configUpdate).
		Put("/configs/" + *detail.ID)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

func (self permissionTestHelper) deleteConfig(ctx *TestContext, session *session, id string) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/configs/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
}

func (self permissionTestHelper) getOrCreateConfigType(ctx *TestContext, name string) string {
	// Try to get it with admin session first
	configTypeResp := &rest_model.ListConfigTypesEnvelope{}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
		SetResult(configTypeResp).
		Get("/config-types?filter=name=\"" + name + "\"")

	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

	if len(configTypeResp.Data) > 0 {
		return *configTypeResp.Data[0].ID
	}

	// Create it
	configTypeCreate := &rest_model.ConfigTypeCreate{
		Name: ToPtr(name),
	}

	configTypeCreateResp := &rest_model.CreateEnvelope{}
	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().
		SetResult(configTypeCreateResp).
		SetBody(configTypeCreate).
		Post("/config-types")

	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

	return configTypeCreateResp.Data.ID
}

// createAdminIdentity creates an identity with isAdmin set to true
func (self permissionTestHelper) createAdminIdentity(ctx *TestContext, session *session, name string, expectedStatus int) string {
	identityType := rest_model.IdentityTypeUser
	identityCreate := &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr("default"),
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Ott: true,
		},
		IsAdmin: ToPtr(true),
		Name:    ToPtr(name),
		Type:    &identityType,
	}

	identityCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identityCreateResp).
		SetBody(identityCreate).
		Post("/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expectedStatus, resp.StatusCode(), string(resp.Body()))

	if identityCreateResp.Data != nil {
		return identityCreateResp.Data.ID
	}
	return ""
}

// createIdentityWithPermissions creates an identity with specific permissions
func (self permissionTestHelper) createIdentityWithPermissions(ctx *TestContext, session *session, name string, perms []string, expectedStatus int) string {
	identityType := rest_model.IdentityTypeUser
	identityCreate := &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr("default"),
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Ott: true,
		},
		IsAdmin:     ToPtr(false),
		Name:        ToPtr(name),
		Permissions: (*rest_model.Permissions)(&perms),
		Type:        &identityType,
	}

	identityCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identityCreateResp).
		SetBody(identityCreate).
		Post("/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expectedStatus, resp.StatusCode(), string(resp.Body()))

	if identityCreateResp.Data != nil {
		return identityCreateResp.Data.ID
	}
	return ""
}

// patchIdentity updates an identity using PATCH method
func (self permissionTestHelper) patchIdentity(ctx *TestContext, session *session, id string, patch *rest_model.IdentityPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/identities/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Auth Policy helper methods

// createAuthPolicy creates an auth policy
func (self permissionTestHelper) createAuthPolicy(ctx *TestContext, session *session, name string, expected int) string {
	authPolicyCreate := &rest_model.AuthPolicyCreate{
		Name: ToPtr(name),
		Primary: &rest_model.AuthPolicyPrimary{
			Cert: &rest_model.AuthPolicyPrimaryCert{
				AllowExpiredCerts: ToPtr(true),
				Allowed:           ToPtr(true),
			},
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
				Allowed:        ToPtr(false),
				AllowedSigners: []string{},
			},
			Updb: &rest_model.AuthPolicyPrimaryUpdb{
				Allowed:                ToPtr(true),
				LockoutDurationMinutes: ToPtr(int64(0)),
				MaxAttempts:            ToPtr(int64(5)),
				MinPasswordLength:      ToPtr(int64(5)),
				RequireMixedCase:       ToPtr(false),
				RequireNumberChar:      ToPtr(false),
				RequireSpecialChar:     ToPtr(false),
			},
		},
		Secondary: &rest_model.AuthPolicySecondary{
			RequireExtJWTSigner: nil,
			RequireTotp:         ToPtr(false),
		},
	}

	authPolicyCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(authPolicyCreateResp).
		SetBody(authPolicyCreate).
		Post("/auth-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if authPolicyCreateResp.Data != nil {
		return authPolicyCreateResp.Data.ID
	}
	return ""
}

// getAuthPolicy retrieves an auth policy by ID
func (self permissionTestHelper) getAuthPolicy(ctx *TestContext, session *session, id string, expected int) *rest_model.AuthPolicyDetail {
	authPolicyResp := &rest_model.DetailAuthPolicyEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(authPolicyResp).
		Get("/auth-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return authPolicyResp.Data
}

// listAuthPolicies lists all auth policies
func (self permissionTestHelper) listAuthPolicies(ctx *TestContext, session *session, expected int) {
	authPoliciesResp := &rest_model.ListAuthPoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(authPoliciesResp).
		Get("/auth-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchAuthPolicy updates an auth policy using PATCH
func (self permissionTestHelper) patchAuthPolicy(ctx *TestContext, session *session, id string, patch *rest_model.AuthPolicyPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/auth-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteAuthPolicy deletes an auth policy
func (self permissionTestHelper) deleteAuthPolicy(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/auth-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Authenticator helper methods

// createAuthenticator creates a UPDB authenticator for the given identity
func (self permissionTestHelper) createAuthenticator(ctx *TestContext, session *session, identityId string, expected int) string {
	authenticatorCreate := &rest_model.AuthenticatorCreate{
		IdentityID: ToPtr(identityId),
		Method:     ToPtr("updb"),
		Username:   eid.New(),
		Password:   eid.New(),
	}

	authenticatorCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(authenticatorCreateResp).
		SetBody(authenticatorCreate).
		Post("/authenticators")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if authenticatorCreateResp.Data != nil {
		return authenticatorCreateResp.Data.ID
	}
	return ""
}

// getAuthenticator retrieves an authenticator by ID
func (self permissionTestHelper) getAuthenticator(ctx *TestContext, session *session, id string, expected int) *rest_model.AuthenticatorDetail {
	authenticatorResp := &rest_model.DetailAuthenticatorEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(authenticatorResp).
		Get("/authenticators/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return authenticatorResp.Data
}

// listAuthenticators lists all authenticators
func (self permissionTestHelper) listAuthenticators(ctx *TestContext, session *session, expected int) {
	authenticatorsResp := &rest_model.ListAuthenticatorsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(authenticatorsResp).
		Get("/authenticators")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchAuthenticator updates an authenticator using PATCH
func (self permissionTestHelper) patchAuthenticator(ctx *TestContext, session *session, id string, patch *rest_model.AuthenticatorPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/authenticators/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteAuthenticator deletes an authenticator
func (self permissionTestHelper) deleteAuthenticator(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/authenticators/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// CA helper methods

// createCa creates a CA
func (self permissionTestHelper) createCa(ctx *TestContext, session *session, name string, expected int) string {
	_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

	caCreate := &rest_model.CaCreate{
		CertPem: ToPtr(caPEM.String()),
		ExternalIDClaim: &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
			MatcherCriteria: ToPtr(""),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		},
		IdentityRoles:             []string{},
		IsAuthEnabled:             ToPtr(true),
		IsAutoCaEnrollmentEnabled: ToPtr(true),
		IsOttCaEnrollmentEnabled:  ToPtr(true),
		Name:                      ToPtr(name),
	}

	caCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(caCreateResp).
		SetBody(caCreate).
		Post("/cas")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if caCreateResp.Data != nil {
		return caCreateResp.Data.ID
	}
	return ""
}

// getCa retrieves a ca by ID
func (self permissionTestHelper) getCa(ctx *TestContext, session *session, id string, expected int) *rest_model.CaDetail {
	caResp := &rest_model.DetailCaEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(caResp).
		Get("/cas/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return caResp.Data
}

// listCas lists all cas
func (self permissionTestHelper) listCas(ctx *TestContext, session *session, expected int) {
	casResp := &rest_model.ListCasEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(casResp).
		Get("/cas")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchCa updates a ca using PATCH
func (self permissionTestHelper) patchCa(ctx *TestContext, session *session, id string, patch *rest_model.CaPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/cas/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteCa deletes a ca
func (self permissionTestHelper) deleteCa(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/cas/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Config helper methods (additional ones for permission tests)

// createConfigExpectStatus creates a config with expected status
func (self permissionTestHelper) createConfigExpectStatus(ctx *TestContext, session *session, name string, expected int) string {
	configTypeId := self.getOrCreateConfigType(ctx, "test-config-type")

	configCreate := &rest_model.ConfigCreate{
		ConfigTypeID: ToPtr(configTypeId),
		Name:         ToPtr(name),
		Data:         map[string]interface{}{"test": "data"},
	}

	configCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configCreateResp).
		SetBody(configCreate).
		Post("/configs")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if configCreateResp.Data != nil {
		return configCreateResp.Data.ID
	}
	return ""
}

// getConfigExpectStatus retrieves a config by ID with expected status
func (self permissionTestHelper) getConfigExpectStatus(ctx *TestContext, session *session, id string, expected int) *rest_model.ConfigDetail {
	configResp := &rest_model.DetailConfigEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configResp).
		Get("/configs/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return configResp.Data
}

// listConfigs lists all configs
func (self permissionTestHelper) listConfigs(ctx *TestContext, session *session, expected int) {
	configsResp := &rest_model.ListConfigsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configsResp).
		Get("/configs")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchConfig updates a config using PATCH
func (self permissionTestHelper) patchConfig(ctx *TestContext, session *session, id string, patch *rest_model.ConfigPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/configs/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteConfigExpectStatus deletes a config with expected status
func (self permissionTestHelper) deleteConfigExpectStatus(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/configs/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listConfigServices lists services for a config
func (self permissionTestHelper) listConfigServices(ctx *TestContext, session *session, id string, expected int) {
	servicesResp := &rest_model.ListServicesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(servicesResp).
		Get("/configs/" + id + "/services")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// ConfigType helper methods

// createConfigType creates a config type
func (self permissionTestHelper) createConfigType(ctx *TestContext, session *session, name string, expected int) string {
	configTypeCreate := &rest_model.ConfigTypeCreate{
		Name: ToPtr(name),
		Schema: map[string]interface{}{
			"$id":                  "http://edge.openziti.org/schemas/test.v1.config.json",
			"type":                 "object",
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type": "string",
				},
				"port": map[string]interface{}{
					"type":    "integer",
					"minimum": float64(0),
					"maximum": float64(math.MaxUint16),
				},
			},
		},
	}

	configTypeCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configTypeCreateResp).
		SetBody(configTypeCreate).
		Post("/config-types")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if configTypeCreateResp.Data != nil {
		return configTypeCreateResp.Data.ID
	}
	return ""
}

// getConfigType retrieves a config type by ID
func (self permissionTestHelper) getConfigType(ctx *TestContext, session *session, id string, expected int) *rest_model.ConfigTypeDetail {
	configTypeResp := &rest_model.DetailConfigTypeEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configTypeResp).
		Get("/config-types/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return configTypeResp.Data
}

// listConfigTypes lists all config types
func (self permissionTestHelper) listConfigTypes(ctx *TestContext, session *session, expected int) {
	configTypesResp := &rest_model.ListConfigTypesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configTypesResp).
		Get("/config-types")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchConfigType updates a config type using PATCH
func (self permissionTestHelper) patchConfigType(ctx *TestContext, session *session, id string, patch *rest_model.ConfigTypePatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/config-types/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteConfigType deletes a config type
func (self permissionTestHelper) deleteConfigType(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/config-types/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listConfigsForConfigType lists configs for a config type
func (self permissionTestHelper) listConfigsForConfigType(ctx *TestContext, session *session, id string, expected int) {
	configsResp := &rest_model.ListConfigsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configsResp).
		Get("/config-types/" + id + "/configs")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Edge Router helper methods

// createEdgeRouter creates an edge router
func (self permissionTestHelper) createEdgeRouter(ctx *TestContext, session *session, name string, expected int) string {
	edgeRouterCreate := &rest_model.EdgeRouterCreate{
		Name:              ToPtr(name),
		IsTunnelerEnabled: false,
	}

	edgeRouterCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(edgeRouterCreateResp).
		SetBody(edgeRouterCreate).
		Post("/edge-routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if edgeRouterCreateResp.Data != nil {
		return edgeRouterCreateResp.Data.ID
	}
	return ""
}

// getEdgeRouter retrieves an edge router by ID
func (self permissionTestHelper) getEdgeRouter(ctx *TestContext, session *session, id string, expected int) *rest_model.EdgeRouterDetail {
	edgeRouterResp := &rest_model.DetailedEdgeRouterEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(edgeRouterResp).
		Get("/edge-routers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return edgeRouterResp.Data
}

// listEdgeRouters lists all edge routers
func (self permissionTestHelper) listEdgeRouters(ctx *TestContext, session *session, expected int) {
	edgeRoutersResp := &rest_model.ListEdgeRoutersEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(edgeRoutersResp).
		Get("/edge-routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchEdgeRouter updates an edge router using PATCH
func (self permissionTestHelper) patchEdgeRouter(ctx *TestContext, session *session, id string, patch *rest_model.EdgeRouterPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/edge-routers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteEdgeRouter deletes an edge router
func (self permissionTestHelper) deleteEdgeRouter(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/edge-routers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listEdgeRouterPoliciesForEdgeRouter lists edge router policies for an edge router
func (self permissionTestHelper) listEdgeRouterPoliciesForEdgeRouter(ctx *TestContext, session *session, id string, expected int) {
	policiesResp := &rest_model.ListEdgeRouterPoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/edge-routers/" + id + "/edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listServiceEdgeRouterPoliciesForEdgeRouter lists service edge router policies for an edge router
func (self permissionTestHelper) listServiceEdgeRouterPoliciesForEdgeRouter(ctx *TestContext, session *session, id string, expected int) {
	policiesResp := &rest_model.ListServiceEdgeRouterPoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/edge-routers/" + id + "/service-edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listIdentitiesForEdgeRouter lists identities for an edge router
func (self permissionTestHelper) listIdentitiesForEdgeRouter(ctx *TestContext, session *session, id string, expected int) {
	identitiesResp := &rest_model.ListIdentitiesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identitiesResp).
		Get("/edge-routers/" + id + "/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listServicesForEdgeRouter lists services for an edge router
func (self permissionTestHelper) listServicesForEdgeRouter(ctx *TestContext, session *session, id string, expected int) {
	servicesResp := &rest_model.ListServicesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(servicesResp).
		Get("/edge-routers/" + id + "/services")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Edge Router Policy helper methods

// createEdgeRouterPolicy creates an edge router policy
func (self permissionTestHelper) createEdgeRouterPolicy(ctx *TestContext, session *session, name string, expected int) string {
	policyCreate := &rest_model.EdgeRouterPolicyCreate{
		Name:            ToPtr(name),
		EdgeRouterRoles: []string{"#all"},
		IdentityRoles:   []string{"#all"},
		Semantic:        ToPtr(rest_model.SemanticAnyOf),
	}

	policyCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policyCreateResp).
		SetBody(policyCreate).
		Post("/edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if policyCreateResp.Data != nil {
		return policyCreateResp.Data.ID
	}
	return ""
}

// getEdgeRouterPolicy retrieves an edge router policy by ID
func (self permissionTestHelper) getEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) *rest_model.EdgeRouterPolicyDetail {
	policyResp := &rest_model.DetailEdgeRouterPolicyEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policyResp).
		Get("/edge-router-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return policyResp.Data
}

// listEdgeRouterPolicies lists all edge router policies
func (self permissionTestHelper) listEdgeRouterPolicies(ctx *TestContext, session *session, expected int) {
	policiesResp := &rest_model.ListEdgeRouterPoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchEdgeRouterPolicy updates an edge router policy using PATCH
func (self permissionTestHelper) patchEdgeRouterPolicy(ctx *TestContext, session *session, id string, patch *rest_model.EdgeRouterPolicyPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/edge-router-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteEdgeRouterPolicy deletes an edge router policy
func (self permissionTestHelper) deleteEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/edge-router-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listEdgeRoutersForEdgeRouterPolicy lists edge routers for an edge router policy
func (self permissionTestHelper) listEdgeRoutersForEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) {
	routersResp := &rest_model.ListEdgeRoutersEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(routersResp).
		Get("/edge-router-policies/" + id + "/edge-routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listIdentitiesForEdgeRouterPolicy lists identities for an edge router policy
func (self permissionTestHelper) listIdentitiesForEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) {
	identitiesResp := &rest_model.ListIdentitiesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identitiesResp).
		Get("/edge-router-policies/" + id + "/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Enrollment helper methods

// createIdentityWithoutEnrollment creates an identity without any enrollment
func (self permissionTestHelper) createIdentityWithoutEnrollment(ctx *TestContext, session *session, name string, expectedStatus int) string {
	identityType := rest_model.IdentityTypeUser
	identityCreate := &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr("default"),
		IsAdmin:      ToPtr(false),
		Name:         ToPtr(name),
		Type:         &identityType,
	}

	identityCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identityCreateResp).
		SetBody(identityCreate).
		Post("/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expectedStatus, resp.StatusCode(), string(resp.Body()))

	if identityCreateResp.Data != nil {
		return identityCreateResp.Data.ID
	}
	return ""
}

// createEnrollment creates an enrollment for an identity
func (self permissionTestHelper) createEnrollment(ctx *TestContext, session *session, identityId string, expected int) string {
	enrollmentCreate := &rest_model.EnrollmentCreate{
		IdentityID: &identityId,
		Method:     ToPtr(rest_model.EnrollmentCreateMethodOtt),
		ExpiresAt:  ST(time.Now().Add(time.Hour)),
	}

	enrollmentCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(enrollmentCreateResp).
		SetBody(enrollmentCreate).
		Post("/enrollments")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if enrollmentCreateResp.Data != nil {
		return enrollmentCreateResp.Data.ID
	}
	return ""
}

// getEnrollment retrieves an enrollment by ID
func (self permissionTestHelper) getEnrollment(ctx *TestContext, session *session, id string, expected int) *rest_model.EnrollmentDetail {
	enrollmentResp := &rest_model.DetailEnrollmentEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(enrollmentResp).
		Get("/enrollments/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return enrollmentResp.Data
}

// listEnrollments lists all enrollments
func (self permissionTestHelper) listEnrollments(ctx *TestContext, session *session, expected int) {
	enrollmentsResp := &rest_model.ListEnrollmentsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(enrollmentsResp).
		Get("/enrollments")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// refreshEnrollment refreshes an enrollment (requires update permission)
func (self permissionTestHelper) refreshEnrollment(ctx *TestContext, session *session, id string, expected int) {
	enrollmentRefresh := &rest_model.EnrollmentRefresh{
		ExpiresAt: ST(time.Now().Add(time.Hour)),
	}

	resp, err := session.newAuthenticatedRequest().
		SetBody(enrollmentRefresh).
		Post("/enrollments/" + id + "/refresh")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteEnrollment deletes an enrollment
func (self permissionTestHelper) deleteEnrollment(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/enrollments/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// External JWT Signer helper methods

// createExternalJwtSigner creates an external JWT signer
func (self permissionTestHelper) createExternalJwtSigner(ctx *TestContext, session *session, name string, expected int) string {
	// Create a test certificate for the JWT signer
	cert, _ := newSelfSignedCert("test-jwt-signer")
	certPem := nfpem.EncodeToString(cert)
	enabled := true

	signerCreate := &rest_model.ExternalJWTSignerCreate{
		CertPem:         &certPem,
		ClaimsProperty:  ToPtr("someClaim"),
		Enabled:         &enabled,
		ExternalAuthURL: ToPtr("https://test-auth-url"),
		Name:            &name,
		UseExternalID:   ToPtr(true),
		Kid:             ToPtr(eid.New()),
		Issuer:          ToPtr(eid.New()),
		Audience:        ToPtr("test-audience"),
		TargetToken:     ToPtr(rest_model.TargetTokenID),
	}

	signerCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(signerCreateResp).
		SetBody(signerCreate).
		Post("/external-jwt-signers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if signerCreateResp.Data != nil {
		return signerCreateResp.Data.ID
	}
	return ""
}

// getExternalJwtSigner retrieves an external JWT signer by ID
func (self permissionTestHelper) getExternalJwtSigner(ctx *TestContext, session *session, id string, expected int) *rest_model.ExternalJWTSignerDetail {
	signerResp := &rest_model.DetailExternalJWTSignerEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(signerResp).
		Get("/external-jwt-signers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return signerResp.Data
}

// listExternalJwtSigners lists all external JWT signers
func (self permissionTestHelper) listExternalJwtSigners(ctx *TestContext, session *session, expected int) {
	signersResp := &rest_model.ListExternalJWTSignersEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(signersResp).
		Get("/external-jwt-signers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchExternalJwtSigner updates an external JWT signer using PATCH
func (self permissionTestHelper) patchExternalJwtSigner(ctx *TestContext, session *session, id string, patch *rest_model.ExternalJWTSignerPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/external-jwt-signers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteExternalJwtSigner deletes an external JWT signer
func (self permissionTestHelper) deleteExternalJwtSigner(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/external-jwt-signers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Posture Check helper methods

// createPostureCheck creates a posture check (MFA type for simplicity)
func (self permissionTestHelper) createPostureCheck(ctx *TestContext, session *session, name string, expected int) string {
	postureCheck := rest_model.PostureCheckMfaCreate{
		PostureCheckMfaProperties: rest_model.PostureCheckMfaProperties{
			PromptOnUnlock: false,
			PromptOnWake:   false,
			TimeoutSeconds: -1,
		},
	}

	postureCheck.SetName(&name)
	postureCheck.SetTypeID(rest_model.PostureCheckTypeMFA)

	postureCheckJson, err := postureCheck.MarshalJSON()
	ctx.Req.NoError(err)

	postureCheckCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(postureCheckCreateResp).
		SetBody(postureCheckJson).
		Post("/posture-checks")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if postureCheckCreateResp.Data != nil {
		return postureCheckCreateResp.Data.ID
	}
	return ""
}

// getPostureCheck retrieves a posture check by ID
func (self permissionTestHelper) getPostureCheck(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Get("/posture-checks/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listPostureChecks lists all posture checks
func (self permissionTestHelper) listPostureChecks(ctx *TestContext, session *session, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Get("/posture-checks")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchPostureCheck updates a posture check using PATCH
func (self permissionTestHelper) patchPostureCheck(ctx *TestContext, session *session, id string, patch *rest_model.PostureCheckMfaPatch, expected int) {
	patchJson, err := patch.MarshalJSON()
	ctx.Req.NoError(err)

	resp, err := session.newAuthenticatedRequest().
		SetBody(patchJson).
		Patch("/posture-checks/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deletePostureCheck deletes a posture check
func (self permissionTestHelper) deletePostureCheck(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/posture-checks/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Transit Router helper methods

// createTransitRouter creates a transit router
func (self permissionTestHelper) createTransitRouter(ctx *TestContext, session *session, name string, expected int) string {
	routerCreate := &rest_model.RouterCreate{
		Name: &name,
	}

	routerCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(routerCreateResp).
		SetBody(routerCreate).
		Post("/routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if routerCreateResp.Data != nil {
		return routerCreateResp.Data.ID
	}
	return ""
}

// getTransitRouter retrieves a transit router by ID
func (self permissionTestHelper) getTransitRouter(ctx *TestContext, session *session, id string, expected int) *rest_model.RouterDetail {
	routerResp := &rest_model.DetailRouterEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(routerResp).
		Get("/routers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return routerResp.Data
}

// listTransitRouters lists all transit routers
func (self permissionTestHelper) listTransitRouters(ctx *TestContext, session *session, expected int) {
	routersResp := &rest_model.ListRoutersEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(routersResp).
		Get("/routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchTransitRouter updates a transit router using PATCH
func (self permissionTestHelper) patchTransitRouter(ctx *TestContext, session *session, id string, patch *rest_model.RouterPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/routers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteTransitRouter deletes a transit router
func (self permissionTestHelper) deleteTransitRouter(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/routers/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Service helper methods

// createService creates a service
func (self permissionTestHelper) createService(ctx *TestContext, session *session, name string, expected int) string {
	serviceCreate := &rest_model.ServiceCreate{
		Name:               &name,
		EncryptionRequired: ToPtr(true),
	}

	serviceCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(serviceCreateResp).
		SetBody(serviceCreate).
		Post("/services")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if serviceCreateResp.Data != nil {
		return serviceCreateResp.Data.ID
	}
	return ""
}

// getService retrieves a service by ID
func (self permissionTestHelper) getService(ctx *TestContext, session *session, id string, expected int) *rest_model.ServiceDetail {
	serviceResp := &rest_model.DetailServiceEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(serviceResp).
		Get("/services/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return serviceResp.Data
}

// listServices lists all services
func (self permissionTestHelper) listServices(ctx *TestContext, session *session, expected int) {
	servicesResp := &rest_model.ListServicesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(servicesResp).
		Get("/services")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

func (self permissionTestHelper) updateService(ctx *TestContext, session *session, detail *rest_model.ServiceDetail, expected int) {
	serviceUpdate := &rest_model.ServiceUpdate{}
	copyRestModelFields(detail, serviceUpdate, ctx)
	serviceUpdate.RoleAttributes = []string{"updated"}

	resp, err := session.newAuthenticatedRequest().
		SetBody(serviceUpdate).
		Put("/services/" + *detail.ID)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchService updates a service using PATCH
func (self permissionTestHelper) patchService(ctx *TestContext, session *session, id string, patch *rest_model.ServicePatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/services/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteService deletes a service
func (self permissionTestHelper) deleteService(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/services/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listConfigsForService lists configs for a service
func (self permissionTestHelper) listConfigsForService(ctx *TestContext, session *session, id string, expected int) {
	configsResp := &rest_model.ListConfigsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(configsResp).
		Get("/services/" + id + "/configs")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listServiceEdgeRouterPoliciesForService lists service edge router policies for a service
func (self permissionTestHelper) listServiceEdgeRouterPoliciesForService(ctx *TestContext, session *session, id string, expected int) {
	policiesResp := &rest_model.ListServiceEdgeRouterPoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/services/" + id + "/service-edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listEdgeRoutersForService lists edge routers for a service
func (self permissionTestHelper) listEdgeRoutersForService(ctx *TestContext, session *session, id string, expected int) {
	routersResp := &rest_model.ListEdgeRoutersEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(routersResp).
		Get("/services/" + id + "/edge-routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listServicePoliciesForService lists service policies for a service
func (self permissionTestHelper) listServicePoliciesForService(ctx *TestContext, session *session, id string, expected int) {
	policiesResp := &rest_model.ListServicePoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/services/" + id + "/service-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listIdentitiesForService lists identities for a service
func (self permissionTestHelper) listIdentitiesForService(ctx *TestContext, session *session, id string, expected int) {
	identitiesResp := &rest_model.ListIdentitiesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identitiesResp).
		Get("/services/" + id + "/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listTerminatorsForService lists terminators for a service
func (self permissionTestHelper) listTerminatorsForService(ctx *TestContext, session *session, id string, expected int) {
	terminatorsResp := &rest_model.ListTerminatorsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(terminatorsResp).
		Get("/services/" + id + "/terminators")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Service Policy helper methods

// createServicePolicy creates a service policy
func (self permissionTestHelper) createServicePolicy(ctx *TestContext, session *session, name string, expected int) string {
	policyCreate := &rest_model.ServicePolicyCreate{
		Name:          &name,
		Type:          ToPtr(rest_model.DialBindDial),
		Semantic:      ToPtr(rest_model.SemanticAllOf),
		ServiceRoles:  []string{"#all"},
		IdentityRoles: []string{"#all"},
	}

	policyCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policyCreateResp).
		SetBody(policyCreate).
		Post("/service-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if policyCreateResp.Data != nil {
		return policyCreateResp.Data.ID
	}
	return ""
}

// getServicePolicy retrieves a service policy by ID
func (self permissionTestHelper) getServicePolicy(ctx *TestContext, session *session, id string, expected int) *rest_model.ServicePolicyDetail {
	policyResp := &rest_model.DetailServicePolicyEnvelop{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policyResp).
		Get("/service-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return policyResp.Data
}

// listServicePolicies lists all service policies
func (self permissionTestHelper) listServicePolicies(ctx *TestContext, session *session, expected int) {
	policiesResp := &rest_model.ListServicePoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/service-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchServicePolicy updates a service policy using PATCH
func (self permissionTestHelper) patchServicePolicy(ctx *TestContext, session *session, id string, patch *rest_model.ServicePolicyPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/service-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteServicePolicy deletes a service policy
func (self permissionTestHelper) deleteServicePolicy(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/service-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listServicesForServicePolicy lists services for a service policy
func (self permissionTestHelper) listServicesForServicePolicy(ctx *TestContext, session *session, id string, expected int) {
	servicesResp := &rest_model.ListServicesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(servicesResp).
		Get("/service-policies/" + id + "/services")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listIdentitiesForServicePolicy lists identities for a service policy
func (self permissionTestHelper) listIdentitiesForServicePolicy(ctx *TestContext, session *session, id string, expected int) {
	identitiesResp := &rest_model.ListIdentitiesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(identitiesResp).
		Get("/service-policies/" + id + "/identities")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listPostureChecksForServicePolicy lists posture checks for a service policy
func (self permissionTestHelper) listPostureChecksForServicePolicy(ctx *TestContext, session *session, id string, expected int) {
	postureChecksResp := &rest_model.ListPostureCheckEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(postureChecksResp).
		Get("/service-policies/" + id + "/posture-checks")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Service Edge Router Policy helper methods

// createServiceEdgeRouterPolicy creates a service edge router policy
func (self permissionTestHelper) createServiceEdgeRouterPolicy(ctx *TestContext, session *session, name string, expected int) string {
	policyCreate := &rest_model.ServiceEdgeRouterPolicyCreate{
		Name:            &name,
		Semantic:        ToPtr(rest_model.SemanticAllOf),
		EdgeRouterRoles: []string{"#all"},
		ServiceRoles:    []string{"#all"},
	}

	policyCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policyCreateResp).
		SetBody(policyCreate).
		Post("/service-edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if policyCreateResp.Data != nil {
		return policyCreateResp.Data.ID
	}
	return ""
}

// getServiceEdgeRouterPolicy retrieves a service edge router policy by ID
func (self permissionTestHelper) getServiceEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) *rest_model.ServiceEdgeRouterPolicyDetail {
	policyResp := &rest_model.DetailServiceEdgePolicyEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policyResp).
		Get("/service-edge-router-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return policyResp.Data
}

// listServiceEdgeRouterPolicies lists all service edge router policies
func (self permissionTestHelper) listServiceEdgeRouterPolicies(ctx *TestContext, session *session, expected int) {
	policiesResp := &rest_model.ListServiceEdgeRouterPoliciesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(policiesResp).
		Get("/service-edge-router-policies")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchServiceEdgeRouterPolicy updates a service edge router policy using PATCH
func (self permissionTestHelper) patchServiceEdgeRouterPolicy(ctx *TestContext, session *session, id string, patch *rest_model.ServiceEdgeRouterPolicyPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/service-edge-router-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteServiceEdgeRouterPolicy deletes a service edge router policy
func (self permissionTestHelper) deleteServiceEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/service-edge-router-policies/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listEdgeRoutersForServiceEdgeRouterPolicy lists edge routers for a service edge router policy
func (self permissionTestHelper) listEdgeRoutersForServiceEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) {
	routersResp := &rest_model.ListEdgeRoutersEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(routersResp).
		Get("/service-edge-router-policies/" + id + "/edge-routers")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// listServicesForServiceEdgeRouterPolicy lists services for a service edge router policy
func (self permissionTestHelper) listServicesForServiceEdgeRouterPolicy(ctx *TestContext, session *session, id string, expected int) {
	servicesResp := &rest_model.ListServicesEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(servicesResp).
		Get("/service-edge-router-policies/" + id + "/services")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// Terminator helper methods

// createTerminator creates a terminator
func (self permissionTestHelper) createTerminator(ctx *TestContext, session *session, serviceId, routerId string, expected int) string {
	terminatorCreate := &rest_model.TerminatorCreate{
		Service: &serviceId,
		Router:  &routerId,
		Binding: ToPtr("transport"),
		Address: ToPtr("tcp:127.0.0.1:1234"),
	}

	terminatorCreateResp := &rest_model.CreateEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(terminatorCreateResp).
		SetBody(terminatorCreate).
		Post("/terminators")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	if terminatorCreateResp.Data != nil {
		return terminatorCreateResp.Data.ID
	}
	return ""
}

// getTerminator retrieves a terminator by ID
func (self permissionTestHelper) getTerminator(ctx *TestContext, session *session, id string, expected int) *rest_model.TerminatorDetail {
	terminatorResp := &rest_model.DetailTerminatorEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(terminatorResp).
		Get("/terminators/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return terminatorResp.Data
}

// listTerminators lists all terminators
func (self permissionTestHelper) listTerminators(ctx *TestContext, session *session, expected int) {
	terminatorsResp := &rest_model.ListTerminatorsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(terminatorsResp).
		Get("/terminators")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// patchTerminator updates a terminator using PATCH
func (self permissionTestHelper) patchTerminator(ctx *TestContext, session *session, id string, patch *rest_model.TerminatorPatch, expected int) {
	resp, err := session.newAuthenticatedRequest().
		SetBody(patch).
		Patch("/terminators/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteTerminator deletes a terminator
func (self permissionTestHelper) deleteTerminator(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/terminators/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// ===== API Session Helper Methods =====

// getApiSession retrieves an API session by ID
func (self permissionTestHelper) getApiSession(ctx *TestContext, session *session, id string, expected int) *rest_model.APISessionDetail {
	apiSessionResp := &rest_model.DetailAPISessionEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(apiSessionResp).
		Get("/api-sessions/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))

	return apiSessionResp.Data
}

// listApiSessions lists all API sessions
func (self permissionTestHelper) listApiSessions(ctx *TestContext, session *session, expected int) {
	apiSessionsResp := &rest_model.ListAPISessionsEnvelope{}
	resp, err := session.newAuthenticatedRequest().
		SetResult(apiSessionsResp).
		Get("/api-sessions")

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}

// deleteApiSession deletes an API session
func (self permissionTestHelper) deleteApiSession(ctx *TestContext, session *session, id string, expected int) {
	resp, err := session.newAuthenticatedRequest().
		Delete("/api-sessions/" + id)

	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, resp.StatusCode(), string(resp.Body()))
}
