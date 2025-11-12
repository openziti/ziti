package tests

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	restidentity "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	edgeapis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/controller/webapis"
)

func Test_Identity_HasErConnection(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	sdkIdentity, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)

	isRunning := true

	defer func() {
		isRunning = false
		ctx.Req.NoError(listener.Close())
	}()
	managementStr := "https://" + ctx.ApiHost + webapis.ManagementRestApiBaseUrlV1
	managementUrl, err := url.Parse(managementStr)
	ctx.Req.NoError(err)

	creds := edgeapis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)

	caPool, err := ziti.GetControllerWellKnownCaPool("https://" + ctx.ApiHost)
	ctx.Req.NoError(err)

	managementClient := edgeapis.NewManagementApiClient([]*url.URL{managementUrl}, caPool, func(strings chan string) {
		strings <- "123"
	})

	curSession, err := managementClient.Authenticate(creds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(curSession)

	result := make(chan *rest_model.IdentityDetail)
	detailIdentityParams := restidentity.NewDetailIdentityParams()
	detailIdentityParams.ID = sdkIdentity.Id

	//HasEdgeRouterConnection can take up to the minimum heartbeat interval (default 60s, configured in tests for 10s)
	//Check every 1s for an update
	go func() {
		for isRunning {
			resp, err := managementClient.API.Identity.DetailIdentity(detailIdentityParams, nil)

			ctx.Req.NoError(err)
			ctx.NotNil(resp)

			if *resp.Payload.Data.HasEdgeRouterConnection {
				result <- resp.Payload.Data
				return
			}

			time.Sleep(1 * time.Second)
		}
	}()

	//Should receive a valid result no later than ~10s later based on the heartbeat interval.
	select {
	case id := <-result:
		ctx.Req.NotNil(id.EdgeRouterConnectionStatus)
		ctx.Req.Equal(*id.EdgeRouterConnectionStatus, "online")
		ctx.Req.NotNil(*id.HasEdgeRouterConnection)
		ctx.Req.True(*id.HasEdgeRouterConnection)
	case <-time.After(15 * time.Second):
		fmt.Printf("CONNECTION EVENTS: timed out (%s)\n", time.Now().String())
		ctx.Fail("timed out")
	}

}
