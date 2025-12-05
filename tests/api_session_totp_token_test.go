package tests

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common"
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

			totpToken, err := adminManagementClient.GetTotpToken(code)
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

				delta := issuedAt.Time.UTC().Sub(accessClaims.IssuedAt.AsTime().UTC()).Abs()
				ctx.True(delta < 2*time.Millisecond)

				ctx.Equal(accessClaims.ApiSessionId, totpClaims.ApiSessionId)

			})
		})
	})

}
