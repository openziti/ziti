package tests

import (
	mangementAuthenticator "github.com/openziti/edge-api/rest_management_api_client/authenticator"
	"github.com/openziti/edge-api/rest_model"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"testing"
)

func Test_Cert_Request_Extend_Legacy_Auth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminManagementClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)

	adminApiSession, err := adminManagementClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminApiSession)

	t.Run("an admin can request key extension for a cert authenticator", func(t *testing.T) {
		ctx.testContextChanged(t)

		idDetial, idCertCreds, err := adminManagementClient.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(idDetial)
		ctx.Req.NotNil(idCertCreds)

		idAuthenticators, err := adminManagementClient.GetIdentityAuthenticators(*idDetial.ID)
		ctx.Req.NoError(err)
		ctx.Req.Len(idAuthenticators, 1)

		idCertAuthenticator := idAuthenticators[0]

		t.Run("newly created enrollment has no extend flags set", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.False(idCertAuthenticator.IsExtendRequested)
			ctx.Req.False(idCertAuthenticator.IsKeyRollRequested)
		})

		t.Run("extend flag can be set", func(t *testing.T) {
			ctx.testContextChanged(t)
			extendParams := mangementAuthenticator.NewRequestExtendAuthenticatorParams()
			extendParams.ID = *idCertAuthenticator.ID
			extendParams.RequestExtendAuthenticator = &rest_model.RequestExtendAuthenticator{
				RollKeys: false,
			}

			resp, err := adminManagementClient.API.Authenticator.RequestExtendAuthenticator(extendParams, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("extended flags are set", func(t *testing.T) {
				updatedAuthenticators, err := adminManagementClient.GetIdentityAuthenticators(*idDetial.ID)
				ctx.Req.NoError(err)
				ctx.Req.Len(updatedAuthenticators, 1)

				updatedAuthenticator := updatedAuthenticators[0]
				ctx.Req.True(updatedAuthenticator.IsExtendRequested)
				ctx.Req.False(updatedAuthenticator.IsKeyRollRequested)

				t.Run("on auth the identity is notified via current-api-session", func(t *testing.T) {
					ctx.testContextChanged(t)

					idClient := ctx.NewEdgeClientApi(nil)
					idApiSession, err := idClient.Authenticate(idCertCreds, nil)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(idApiSession)

					currentSession, err := idClient.GetCurrentApiSessionDetail()
					ctx.Req.NoError(err)
					ctx.Req.NotNil(currentSession)
					ctx.Req.True(currentSession.IsCertExtendRequested)
					ctx.Req.False(currentSession.IsCertKeyRollRequested)

					t.Run("extend flag can be set", func(t *testing.T) {
						ctx.testContextChanged(t)

						newIdCertCreds, err := idClient.ExtendCertsWithAuthenticatorId(*updatedAuthenticator.ID)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(newIdCertCreds)

						t.Run("on extend and verify the flags are cleared", func(t *testing.T) {
							ctx.testContextChanged(t)

							postExtendAuthenticators, err := adminManagementClient.GetIdentityAuthenticators(*idDetial.ID)
							ctx.Req.NoError(err)
							ctx.Req.Len(postExtendAuthenticators, 1)

							postExtendAuthenticator := postExtendAuthenticators[0]
							ctx.Req.False(postExtendAuthenticator.IsExtendRequested)
							ctx.Req.False(postExtendAuthenticator.IsKeyRollRequested)
						})
					})
				})
			})
		})
	})

	t.Run("an admin can request key extension and rolling for a cert authenticator", func(t *testing.T) {
		ctx.testContextChanged(t)

		idDetial, idCertCreds, err := adminManagementClient.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(idDetial)
		ctx.Req.NotNil(idCertCreds)

		idAuthenticators, err := adminManagementClient.GetIdentityAuthenticators(*idDetial.ID)
		ctx.Req.NoError(err)
		ctx.Req.Len(idAuthenticators, 1)

		idCertAuthenticator := idAuthenticators[0]

		t.Run("newly created enrollment has no extend flags set", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.False(idCertAuthenticator.IsExtendRequested)
			ctx.Req.False(idCertAuthenticator.IsKeyRollRequested)
		})

		t.Run("extend flag can be set", func(t *testing.T) {
			ctx.testContextChanged(t)
			extendParams := mangementAuthenticator.NewRequestExtendAuthenticatorParams()
			extendParams.ID = *idCertAuthenticator.ID
			extendParams.RequestExtendAuthenticator = &rest_model.RequestExtendAuthenticator{
				RollKeys: true,
			}

			resp, err := adminManagementClient.API.Authenticator.RequestExtendAuthenticator(extendParams, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("extended flags are set", func(t *testing.T) {
				updatedAuthenticators, err := adminManagementClient.GetIdentityAuthenticators(*idDetial.ID)
				ctx.Req.NoError(err)
				ctx.Req.Len(updatedAuthenticators, 1)

				updatedAuthenticator := updatedAuthenticators[0]
				ctx.Req.True(updatedAuthenticator.IsExtendRequested)
				ctx.Req.True(updatedAuthenticator.IsKeyRollRequested)

				t.Run("on auth the identity is notified via current-api-session", func(t *testing.T) {
					ctx.testContextChanged(t)

					idClient := ctx.NewEdgeClientApi(nil)
					idApiSession, err := idClient.Authenticate(idCertCreds, nil)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(idApiSession)

					currentSession, err := idClient.GetCurrentApiSessionDetail()
					ctx.Req.NoError(err)
					ctx.Req.NotNil(currentSession)
					ctx.Req.True(currentSession.IsCertExtendRequested)
					ctx.Req.True(currentSession.IsCertKeyRollRequested)

					t.Run("extend flag can be set", func(t *testing.T) {
						ctx.testContextChanged(t)

						newIdCertCreds, err := idClient.ExtendCertsWithAuthenticatorId(*updatedAuthenticator.ID)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(newIdCertCreds)

						t.Run("on extend and verify the flags are cleared", func(t *testing.T) {
							ctx.testContextChanged(t)

							postExtendAuthenticators, err := adminManagementClient.GetIdentityAuthenticators(*idDetial.ID)
							ctx.Req.NoError(err)
							ctx.Req.Len(postExtendAuthenticators, 1)

							postExtendAuthenticator := postExtendAuthenticators[0]
							ctx.Req.False(postExtendAuthenticator.IsExtendRequested)
							ctx.Req.False(postExtendAuthenticator.IsKeyRollRequested)
						})
					})
				})
			})
		})
	})
}
