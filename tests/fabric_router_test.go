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
	"context"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
	restClientRouter "github.com/openziti/ziti/v2/controller/rest_client/router"
	fabricRestModel "github.com/openziti/ziti/v2/controller/rest_model"
)

func Test_FabricRouters(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("ctrlChanListeners can be created with an empty list", func(t *testing.T) {
		ctx.testContextChanged(t)

		timeoutContext, cancelF := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelF()

		id := eid.New()
		name := eid.New()

		createParams := &restClientRouter.CreateRouterParams{
			Router: &fabricRestModel.RouterCreate{
				Cost:        util.Ptr(int64(0)),
				ID:          &id,
				Name:        &name,
				NoTraversal: util.Ptr(false),
			},
			Context: timeoutContext,
		}
		createResp, err := ctx.RestClients.Fabric.Router.CreateRouter(createParams)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createResp)

		detailParams := &restClientRouter.DetailRouterParams{
			ID:      id,
			Context: timeoutContext,
		}
		detailResp, err := ctx.RestClients.Fabric.Router.DetailRouter(detailParams)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be set on create and retrieved", func(t *testing.T) {
		ctx.testContextChanged(t)

		timeoutContext, cancelF := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelF()

		id := eid.New()
		name := eid.New()
		listeners := []string{"tls:1.2.3.4:6262", "tls:5.6.7.8:6262"}

		createParams := &restClientRouter.CreateRouterParams{
			Router: &fabricRestModel.RouterCreate{
				Cost:              util.Ptr(int64(0)),
				ID:                &id,
				Name:              &name,
				NoTraversal:       util.Ptr(false),
				CtrlChanListeners: listeners,
			},
			Context: timeoutContext,
		}
		createResp, err := ctx.RestClients.Fabric.Router.CreateRouter(createParams)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createResp)

		detailParams := &restClientRouter.DetailRouterParams{
			ID:      id,
			Context: timeoutContext,
		}
		detailResp, err := ctx.RestClients.Fabric.Router.DetailRouter(detailParams)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp)
		ctx.Req.Equal(listeners, detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be updated via patch", func(t *testing.T) {
		ctx.testContextChanged(t)

		timeoutContext, cancelF := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelF()

		id := eid.New()
		name := eid.New()

		createParams := &restClientRouter.CreateRouterParams{
			Router: &fabricRestModel.RouterCreate{
				Cost:              util.Ptr(int64(0)),
				ID:                &id,
				Name:              &name,
				NoTraversal:       util.Ptr(false),
				CtrlChanListeners: []string{"tls:1.2.3.4:6262"},
			},
			Context: timeoutContext,
		}
		_, err := ctx.RestClients.Fabric.Router.CreateRouter(createParams)
		ctx.Req.NoError(err)

		detailParams := &restClientRouter.DetailRouterParams{
			ID:      id,
			Context: timeoutContext,
		}

		// update to more addresses
		patchParams := &restClientRouter.PatchRouterParams{
			ID: id,
			Router: &fabricRestModel.RouterPatch{
				CtrlChanListeners: []string{"tls:10.0.0.1:6262", "tls:10.0.0.2:6262", "tls:10.0.0.3:6262"},
			},
			Context: timeoutContext,
		}
		_, err = ctx.RestClients.Fabric.Router.PatchRouter(patchParams)
		ctx.Req.NoError(err)

		detailResp, err := ctx.RestClients.Fabric.Router.DetailRouter(detailParams)
		ctx.Req.NoError(err)
		ctx.Req.Equal([]string{"tls:10.0.0.1:6262", "tls:10.0.0.2:6262", "tls:10.0.0.3:6262"}, detailResp.Payload.Data.CtrlChanListeners)

		// update to fewer addresses
		patchParams = &restClientRouter.PatchRouterParams{
			ID: id,
			Router: &fabricRestModel.RouterPatch{
				CtrlChanListeners: []string{"tls:10.0.0.1:6262"},
			},
			Context: timeoutContext,
		}
		_, err = ctx.RestClients.Fabric.Router.PatchRouter(patchParams)
		ctx.Req.NoError(err)

		detailResp, err = ctx.RestClients.Fabric.Router.DetailRouter(detailParams)
		ctx.Req.NoError(err)
		ctx.Req.Equal([]string{"tls:10.0.0.1:6262"}, detailResp.Payload.Data.CtrlChanListeners)

		// update to zero addresses
		patchParams = &restClientRouter.PatchRouterParams{
			ID: id,
			Router: &fabricRestModel.RouterPatch{
				CtrlChanListeners: []string{},
			},
			Context: timeoutContext,
		}
		_, err = ctx.RestClients.Fabric.Router.PatchRouter(patchParams)
		ctx.Req.NoError(err)

		detailResp, err = ctx.RestClients.Fabric.Router.DetailRouter(detailParams)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})
}
