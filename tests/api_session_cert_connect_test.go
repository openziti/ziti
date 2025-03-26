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
	"github.com/openziti/sdk-golang/ziti"
	"testing"
	"time"
)

func Test_ApiSessionCertConnection(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	testService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("weighted")

	ctx.CreateEnrollAndStartEdgeRouter()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithUpdb(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientConfig.EnableHa = true
	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	connectChan := make(chan struct{}, 1)
	clientContext.Events().AddRouterConnectedListener(func(ztx ziti.Context, name string, addr string) {
		connectChan <- struct{}{}
	})

	disconnectedChan := make(chan struct{}, 1)
	clientContext.Events().AddRouterDisconnectedListener(func(ztx ziti.Context, name string, addr string) {
		disconnectedChan <- struct{}{}
	})

	//dial is expected to fail, but will trigger a connection to the ER
	conn, _ := clientContext.Dial(testService.Name)

	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	select {
	case <-connectChan:
		break
	case <-time.After(time.Second * 5):
		ctx.Fail("router connection did not occur after 5 seconds")
	}

	select {
	case <-disconnectedChan:
		ctx.Fail("router disconnected")
	case <-time.After(time.Second * 1):
		return //success
	}
}
