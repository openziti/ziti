package tests

import (
	"testing"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/common/eid"
)

func Test_SDK_API_Session_Token_Update(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	sdkRole := eid.New()
	_, hostContextIface := ctx.AdminManagementSession.RequireCreateSdkContext(sdkRole)
	defer hostContextIface.Close()

	sdkContext, ok := hostContextIface.(*ziti.ContextImpl)
	ctx.Req.True(ok, "sdkContext should be of type *ziti.ContextImpl")
	ctx.Req.NotNil(sdkContext)

	err := sdkContext.Authenticate()
	ctx.Req.NoError(err)

	err = sdkContext.ConnectAllAvailableErs()
	ctx.Req.NoError(err)

	functionalErr, erErr := sdkContext.RefreshApiSession()
	ctx.Req.NoError(functionalErr)
	ctx.Req.NoError(erErr)
}
