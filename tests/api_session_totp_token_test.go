package tests

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
)

func Test_API_Session_TOTP_Tokens(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementApi := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()

	apiSession, err := managementApi.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(apiSession)

	t.Run("non admins in the client api", func(t *testing.T) {

		ctx.testContextChanged(t)

		nonAdminIdentity, nonAdminCreds, err := managementApi.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(nonAdminIdentity)
		ctx.Req.NotNil(nonAdminCreds)

		nonAdminClientClient := ctx.NewEdgeClientApi(nil)

		nonAdminApiSession, err := nonAdminClientClient.Authenticate(nonAdminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(nonAdminApiSession)

		nonAdminTotpProvider, nonAdminTotpDetail, err := nonAdminClientClient.EnrollTotpMfa()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(nonAdminTotpDetail)
		ctx.Req.NotNil(nonAdminTotpProvider)

		t.Run("cannot create totp tokens using legacy authentication", func(t *testing.T) {
			ctx.testContextChanged(t)

			nonAdminClientClient = ctx.NewEdgeClientApi(nonAdminTotpProvider.FuncProvider())
			nonAdminClientClient.SetUseOidc(false)
			nonAdminClientClient.SetAllowOidcDynamicallyEnabled(false)

			nonAdminApiSession, err = nonAdminClientClient.Authenticate(nonAdminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(nonAdminApiSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeLegacy, nonAdminApiSession.GetType())

			code := nonAdminTotpProvider.Code()

			token, err := nonAdminClientClient.GetTotpToken(code)
			ctx.Req.Error(err)
			ctx.Req.Empty(token)
		})

		t.Run("can create totp tokens using oidc authentication", func(t *testing.T) {
			ctx.testContextChanged(t)

			nonAdminClientClient = ctx.NewEdgeClientApi(nonAdminTotpProvider.FuncProvider())
			nonAdminClientClient.SetUseOidc(true)
			nonAdminClientClient.SetAllowOidcDynamicallyEnabled(true)

			nonAdminApiSession, err = nonAdminClientClient.Authenticate(nonAdminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(nonAdminApiSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeOidc, nonAdminApiSession.GetType())

			code := nonAdminTotpProvider.Code()

			totpToken, err := nonAdminClientClient.GetTotpToken(code)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(totpToken)
			ctx.Req.NotNil(totpToken.Token)

			t.Run("the token is a JWT and has the correct values", func(t *testing.T) {
				ctx.testContextChanged(t)

				totpClaims := &common.TotpClaims{}
				totpJwtToken, err := jwt.ParseWithClaims(*totpToken.Token, totpClaims, ctx.EdgeController.AppEnv.JwtSignerKeyFunc)
				ctx.NoError(err)
				ctx.NotNil(totpJwtToken)

				accessClaims := &common.AccessClaims{}
				apiSessionJwtToken, err := jwt.ParseWithClaims(string(nonAdminApiSession.GetToken()), accessClaims, ctx.EdgeController.AppEnv.JwtSignerKeyFunc)
				ctx.NoError(err)
				ctx.NotNil(apiSessionJwtToken)

				ctx.Req.NoError(err)
				ctx.NotNil(totpJwtToken)

				ctx.Equal(common.TokenTypeTotp, totpClaims.Type)

				expTime, err := totpJwtToken.Claims.GetExpirationTime()
				ctx.NoError(err)
				ctx.Nil(expTime)

				ctx.Equal(accessClaims.ApiSessionId, totpClaims.ApiSessionId)

			})
		})
	})

	t.Run("admins in the management api", func(t *testing.T) {

		ctx.testContextChanged(t)

		adminIdentity, adminCreds, err := managementApi.CreateAndEnrollOttIdentity(true)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminIdentity)
		ctx.Req.NotNil(adminCreds)

		adminManagementClient := ctx.NewEdgeManagementApi(nil)

		adminApiSession, err := adminManagementClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminApiSession)

		adminTotpProvider, adminTotpDetail, err := adminManagementClient.EnrollTotpMfa()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminTotpDetail)
		ctx.Req.NotNil(adminTotpProvider)

		t.Run("cannot create totp tokens using legacy authentication", func(t *testing.T) {
			ctx.testContextChanged(t)

			adminManagementClient = ctx.NewEdgeManagementApi(adminTotpProvider.FuncProvider())
			adminManagementClient.SetUseOidc(false)
			adminManagementClient.SetAllowOidcDynamicallyEnabled(false)

			adminApiSession, err = adminManagementClient.Authenticate(adminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(adminApiSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeLegacy, adminApiSession.GetType())

			code := adminTotpProvider.Code()

			token, err := adminManagementClient.GetTotpToken(code)
			ctx.Req.Error(err)
			ctx.Req.Empty(token)
		})

		t.Run("can create totp tokens using oidc authentication", func(t *testing.T) {
			ctx.testContextChanged(t)

			adminManagementClient = ctx.NewEdgeManagementApi(adminTotpProvider.FuncProvider())
			adminManagementClient.SetUseOidc(true)
			adminManagementClient.SetAllowOidcDynamicallyEnabled(true)

			adminApiSession, err = adminManagementClient.Authenticate(adminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(adminApiSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeOidc, adminApiSession.GetType())

			code := adminTotpProvider.Code()

			beforeTotpCreate := time.Now()
			totpToken, err := adminManagementClient.GetTotpToken(code)
			afterTotpCreate := time.Now()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(totpToken)
			ctx.Req.NotNil(totpToken.Token)

			t.Run("the token is a JWT and has the correct values", func(t *testing.T) {
				ctx.testContextChanged(t)

				totpClaims := &common.TotpClaims{}
				totpJwtToken, err := jwt.ParseWithClaims(*totpToken.Token, totpClaims, ctx.EdgeController.AppEnv.JwtSignerKeyFunc)
				ctx.NoError(err)
				ctx.NotNil(totpJwtToken)

				accessClaims := &common.AccessClaims{}
				apiSessionJwtToken, err := jwt.ParseWithClaims(string(adminApiSession.GetToken()), accessClaims, ctx.EdgeController.AppEnv.JwtSignerKeyFunc)
				ctx.NoError(err)
				ctx.NotNil(apiSessionJwtToken)

				ctx.Req.NoError(err)
				ctx.NotNil(totpJwtToken)

				ctx.Equal(common.TokenTypeTotp, totpClaims.Type)

				expTime, err := totpJwtToken.Claims.GetExpirationTime()
				ctx.NoError(err)
				ctx.Nil(expTime)

				issuedAt, err := totpJwtToken.Claims.GetIssuedAt()
				ctx.NoError(err)
				ctx.NotNil(issuedAt)

				// The serialized iat truncates to whole seconds, so the lower bound must too.
				ctx.Req.WithinRange(issuedAt.Time, beforeTotpCreate.Truncate(time.Second), afterTotpCreate)

				ctx.Equal(accessClaims.ApiSessionId, totpClaims.ApiSessionId)

			})
		})
	})

}

// Test_API_Session_TOTP_Tokens_MfaPosture locks in GH-4558 for OIDC api sessions whose token does not say amr=totp
// (they logged in before the identity had MFA): a TOTP code the controller verifies for the session passes MFA
// posture checks for that session, as /authenticate/mfa does for legacy api sessions. The code can come from
// completing the MFA enrollment or from exchanging it for a TOTP token.
func Test_API_Session_TOTP_Tokens_MfaPosture(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementApi := ctx.NewEdgeManagementApi(nil)
	_, err := managementApi.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	identity, creds, err := managementApi.CreateAndEnrollOttIdentity(false)
	ctx.Req.NoError(err)

	postureCheckRole := eid.New()
	postureCheck, err := managementApi.CreatePostureCheckMfa(-1, false, false, []string{postureCheckRole})
	ctx.Req.NoError(err)
	checkId := *postureCheck.ID()

	service, err := managementApi.CreateService(eid.New(), false, nil)
	ctx.Req.NoError(err)

	_, err = managementApi.CreateServicePolicy(rest_model.DialBindDial, []string{"@" + service.ID}, []string{"@" + *identity.ID}, []string{"#" + postureCheckRole})
	ctx.Req.NoError(err)

	// OIDC api sessions of the identity, all from before it has MFA: no token has amr=totp. Once the identity has
	// MFA, a new login has to answer a TOTP prompt (and would pass the check from its amr), so they are opened now.
	newOidcSession := func() *ClientHelperClient {
		client := ctx.NewEdgeClientApi(nil)
		client.SetUseOidc(true)
		client.SetAllowOidcDynamicallyEnabled(true)
		apiSession, err := client.Authenticate(creds, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(edge_apis.ApiSessionTypeOidc, apiSession.GetType())
		return client
	}
	enrolling := newOidcSession()
	other := newOidcSession()
	third := newOidcSession()

	// mfaCheckPassing is the isPassing the controller lists for the MFA posture query of the service, for the
	// api session of client
	mfaCheckPassing := func(client *ClientHelperClient) bool {
		detail, err := client.GetServiceDetail(service.ID)
		ctx.Req.NoError(err)

		for _, querySet := range detail.PostureQueries {
			for _, query := range querySet.PostureQueries {
				if query.ID != nil && *query.ID == checkId {
					ctx.Req.NotNil(query.IsPassing)
					return *query.IsPassing
				}
			}
		}
		ctx.Req.Failf("mfa posture query not found", "service %s has no posture query for check %s", service.ID, checkId)
		return false
	}

	t.Run("without mfa the check is failing", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.Req.False(mfaCheckPassing(enrolling))
		ctx.Req.False(mfaCheckPassing(other))
		ctx.Req.False(mfaCheckPassing(third))
	})

	totpProvider, _, err := enrolling.EnrollTotpMfa()
	ctx.Req.NoError(err)

	t.Run("the code that completes the enrollment passes the check for that api session only", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.Req.True(mfaCheckPassing(enrolling))
		ctx.Req.False(mfaCheckPassing(other), "the enrollment is not an MFA pass for the other api sessions")
		ctx.Req.False(mfaCheckPassing(third), "the enrollment is not an MFA pass for the other api sessions")
	})

	t.Run("a code exchanged for a totp token passes the check", func(t *testing.T) {
		ctx.testContextChanged(t)

		totpToken, err := other.GetTotpToken(totpProvider.Code())
		ctx.Req.NoError(err)
		ctx.Req.NotNil(totpToken)

		ctx.Req.True(mfaCheckPassing(other))
	})

	t.Run("an invalid code does not pass the check", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, err := third.GetTotpToken("000000")
		ctx.Req.Error(err)
		ctx.Req.False(mfaCheckPassing(third))
	})
}
