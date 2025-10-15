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
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge/posture"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/xt_smartrouting"
)

func Test_PostureCheck_SDK_OS_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()
	postureCheckRoleAttr := eid.New()

	adminManagementApi := ctx.NewEdgeManagementApi(nil)
	adminManagementApiSession, err := adminManagementApi.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminManagementApiSession)

	targetOs := &rest_model.OperatingSystem{
		Type:     ToPtr(rest_model.OsTypeWindows),
		Versions: []string{"1.0.0"},
	}

	validOsInfo := posture.OsInfo{
		Type:    string(*targetOs.Type),
		Version: targetOs.Versions[0],
	}

	invalidOsInfo := posture.OsInfo{
		Type:    string(*targetOs.Type),
		Version: "2.0.0",
	}

	postureCheckOs, err := adminManagementApi.CreatePostureCheckOs([]*rest_model.OperatingSystem{targetOs}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)
	ctx.Req.NotNil(postureCheckOs)

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
			name, eof := conn.ReadString(1024, 1*time.Minute)
			if eof {
				return conn.server.close()
			}

			if name == "quit" {
				conn.WriteString("ok", time.Second)
				return conn.server.close()
			}

			result := "hello, " + name
			conn.WriteString(result, time.Second)
		}
	})
	testServer.start()

	t.Run("a client sdk", func(t *testing.T) {
		ctx.testContextChanged(t)

		var clientContext *ziti.ContextImpl

		_, ztx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)

		ztx.(*ziti.ContextImpl).CtrlClt.SetAllowOidcDynamicallyEnabled(true)
		var ok bool
		clientContext, ok = ztx.(*ziti.ContextImpl)
		ctx.Req.True(ok)

		defer clientContext.Close()

		t.Run("can authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)
			err := clientContext.Authenticate()
			ctx.Req.NoError(err)
		})

		t.Run("can provide valid posture data and dial the service", func(t *testing.T) {
			ctx.testContextChanged(t)
			postureCache := clientContext.CtrlClt.PostureCache
			ctx.Req.NoError(err)

			currentReportingOsInfo := validOsInfo
			postureCache.SetOsProviderFunc(func() posture.OsInfo {
				return currentReportingOsInfo
			})

			clientConn := ctx.WrapConn(clientContext.Dial(service.Name))
			defer func() { _ = clientConn.Close() }()

			t.Run("the dialed service can be sent data", func(t *testing.T) {
				ctx.testContextChanged(t)
				name := eid.New()
				clientConn.WriteString(name, time.Second)
				clientConn.ReadExpected("hello, "+name, time.Second)
			})

			t.Run("sending invalid posture data", func(t *testing.T) {
				ctx.testContextChanged(t)

				currentReportingOsInfo = invalidOsInfo
				postureCache.Evaluate()

				lastReadCount := 0
				var lastReadErr error
				count := 0
				for !clientConn.IsClosed() && count <= 20 {
					var buff []byte

					//read till end of client buffer
					lastReadCount, lastReadErr = clientConn.Read(buff)

					if lastReadErr != nil {
						break
					}

					time.Sleep(100 * time.Millisecond)
					count = count + 1
				}

				t.Run("closes the connection", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.Error(lastReadErr)
					ctx.Req.Equal(0, lastReadCount)
					ctx.Req.True(clientConn.IsClosed())
				})

				t.Run("cannot be written to", func(t *testing.T) {
					ctx.testContextChanged(t)
					nWritten, err := clientConn.Write([]byte("hi"))
					ctx.Req.Equal(0, nWritten)
					ctx.Req.Error(err)
				})

				t.Run("dialing again with invalid posture data should fail", func(t *testing.T) {
					ctx.testContextChanged(t)
					clientConn, err := clientContext.Dial(service.Name)

					ctx.Req.Nil(clientConn)
					ctx.Req.Error(err)
				})
			})
		})
	})
}
