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

	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	restidentity "github.com/openziti/edge-api/rest_management_api_client/identity"
	restservice "github.com/openziti/edge-api/rest_management_api_client/service"
	edgeTerminator "github.com/openziti/edge-api/rest_management_api_client/terminator"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	fabricRouter "github.com/openziti/ziti/v2/controller/rest_client/router"
	fabricTerminator "github.com/openziti/ziti/v2/controller/rest_client/terminator"
	fabricRestModel "github.com/openziti/ziti/v2/controller/rest_model"
	"github.com/openziti/ziti/v2/controller/xt"
)

// Test_PutKeepsFieldsItsBodyCannotCarry checks that a full update (PUT) replaces only what its request
// body carries. Stored fields that the controller, routers or SDKs set through their own paths, such as
// an identity's disable or a terminator's hostId, must survive an admin's PUT.
func Test_PutKeepsFieldsItsBodyCannotCarry(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	edgeApi := ctx.RestClients.Edge
	fabricApi := ctx.RestClients.Fabric
	managers := ctx.EdgeController.AppEnv.Managers

	t.Run("identity PUT keeps the identity disabled", func(t *testing.T) {
		ctx.NextTest(t)

		createResp, err := edgeApi.Identity.CreateIdentity(&restidentity.CreateIdentityParams{
			Identity: &rest_model.IdentityCreate{
				Name:    util.Ptr(eid.New()),
				IsAdmin: util.Ptr(false),
				Type:    util.Ptr(rest_model.IdentityTypeDefault),
			},
		}, nil)
		ctx.Req.NoError(err)
		identityId := createResp.Payload.Data.ID

		_, err = edgeApi.Identity.DisableIdentity(&restidentity.DisableIdentityParams{
			ID:      identityId,
			Disable: &rest_model.DisableParams{DurationMinutes: util.Ptr(int64(0))},
		}, nil)
		ctx.Req.NoError(err)

		newName := eid.New()
		_, err = edgeApi.Identity.UpdateIdentity(&restidentity.UpdateIdentityParams{
			ID: identityId,
			Identity: &rest_model.IdentityUpdate{
				Name:    &newName,
				IsAdmin: util.Ptr(false),
				Type:    util.Ptr(rest_model.IdentityTypeDefault),
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := edgeApi.Identity.DetailIdentity(&restidentity.DetailIdentityParams{ID: identityId}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(newName, *detailResp.Payload.Data.Name, "the PUT should have renamed the identity")
		ctx.Req.True(*detailResp.Payload.Data.Disabled, "a PUT can't carry the disable, so the identity should stay disabled")
		ctx.Req.NotNil(detailResp.Payload.Data.DisabledAt, "a PUT can't carry the disable, so disabledAt should be kept")
	})

	t.Run("identity PUT keeps an ER/T identity's reported interfaces", func(t *testing.T) {
		ctx.NextTest(t)

		routerName := eid.New()
		createResp, err := edgeApi.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name:              &routerName,
				IsTunnelerEnabled: true,
			},
		}, nil)
		ctx.Req.NoError(err)
		routerId := createResp.Payload.Data.ID

		// an ER/T's reported interfaces are written to its router identity as well
		err = managers.Router.UpdateRouterInterfaces(routerId, []*model.Interface{
			{Name: "eth0", Addresses: []string{"10.0.0.1/24"}},
		}, change.New())
		ctx.Req.NoError(err)

		_, err = edgeApi.Identity.UpdateIdentity(&restidentity.UpdateIdentityParams{
			ID: routerId,
			Identity: &rest_model.IdentityUpdate{
				Name:    &routerName,
				IsAdmin: util.Ptr(false),
				Type:    util.Ptr(rest_model.IdentityTypeRouter),
				Tags:    &rest_model.Tags{SubTags: map[string]interface{}{"updated": true}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := edgeApi.Identity.DetailIdentity(&restidentity.DetailIdentityParams{ID: routerId}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(true, detailResp.Payload.Data.Tags.SubTags["updated"], "the PUT should have replaced the tags")
		ctx.Req.Len(detailResp.Payload.Data.Interfaces, 1, "a PUT can't carry reported interfaces, so they should be kept")
	})

	t.Run("fabric router PUT keeps the router's reported interfaces", func(t *testing.T) {
		ctx.NextTest(t)

		routerId := eid.New()
		fingerprint := eid.New()
		_, err := fabricApi.Router.CreateRouter(&fabricRouter.CreateRouterParams{
			Router: &fabricRestModel.RouterCreate{
				ID:          &routerId,
				Name:        util.Ptr(eid.New()),
				Fingerprint: &fingerprint,
				Cost:        util.Ptr(int64(0)),
				NoTraversal: util.Ptr(false),
			},
		}, nil)
		ctx.Req.NoError(err)

		err = managers.Router.UpdateRouterInterfaces(routerId, []*model.Interface{
			{Name: "eth0", Addresses: []string{"10.0.0.1/24"}},
		}, change.New())
		ctx.Req.NoError(err)

		newName := eid.New()
		_, err = fabricApi.Router.UpdateRouter(&fabricRouter.UpdateRouterParams{
			ID: routerId,
			Router: &fabricRestModel.RouterUpdate{
				Name:        &newName,
				Fingerprint: &fingerprint,
				Cost:        util.Ptr(int64(5)),
				NoTraversal: util.Ptr(false),
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := fabricApi.Router.DetailRouter(&fabricRouter.DetailRouterParams{ID: routerId}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(newName, *detailResp.Payload.Data.Name, "the PUT should have renamed the router")
		ctx.Req.Len(detailResp.Payload.Data.Interfaces, 1, "a PUT can't carry reported interfaces, so they should be kept")
	})

	t.Run("terminator PUT", func(t *testing.T) {
		ctx.NextTest(t)

		serviceResp, err := edgeApi.Service.CreateService(&restservice.CreateServiceParams{
			Service: &rest_model.ServiceCreate{
				Name:               util.Ptr(eid.New()),
				EncryptionRequired: util.Ptr(true),
			},
		}, nil)
		ctx.Req.NoError(err)
		serviceId := serviceResp.Payload.Data.ID

		routerId := eid.New()
		_, err = fabricApi.Router.CreateRouter(&fabricRouter.CreateRouterParams{
			Router: &fabricRestModel.RouterCreate{
				ID:          &routerId,
				Name:        util.Ptr(eid.New()),
				Fingerprint: util.Ptr(eid.New()),
				Cost:        util.Ptr(int64(0)),
				NoTraversal: util.Ptr(false),
			},
		}, nil)
		ctx.Req.NoError(err)

		t.Run("through the edge management API keeps hostId, peerData, sourceCtrl and savedPrecedence", func(t *testing.T) {
			ctx.NextTest(t)

			// a hosted terminator on a quiesced router, carrying state no update API can set
			terminator := &model.Terminator{
				Service:         serviceId,
				Router:          routerId,
				Binding:         "edge",
				Address:         "hosted:" + eid.New(),
				Precedence:      xt.Precedences.Failed,
				SavedPrecedence: xt.Precedences.Required,
				HostId:          eid.New(),
				PeerData:        map[uint32][]byte{1: []byte("peer-data")},
				SourceCtrl:      eid.New(),
			}
			terminator.Id = eid.New()
			ctx.Req.NoError(managers.Terminator.Create(terminator, change.New()))

			_, err := edgeApi.Terminator.UpdateTerminator(&edgeTerminator.UpdateTerminatorParams{
				ID: terminator.Id,
				Terminator: &rest_model.TerminatorUpdate{
					Service:    &serviceId,
					Router:     &routerId,
					Binding:    &terminator.Binding,
					Address:    &terminator.Address,
					Cost:       util.Ptr(rest_model.TerminatorCost(10)),
					Precedence: rest_model.TerminatorPrecedenceFailed,
				},
			}, nil)
			ctx.Req.NoError(err)

			stored, err := managers.Terminator.Read(terminator.Id)
			ctx.Req.NoError(err)
			ctx.Req.Equal(uint16(10), stored.Cost, "the PUT should have set the cost")
			ctx.Req.Equal(terminator.HostId, stored.HostId, "the edge management API can't carry hostId, so it should be kept")
			ctx.Req.Equal(terminator.PeerData, stored.PeerData, "a PUT can't carry peerData, so it should be kept")
			ctx.Req.Equal(terminator.SourceCtrl, stored.SourceCtrl, "a PUT can't carry sourceCtrl, so it should be kept")
			ctx.Req.Equal(xt.Precedences.Required, stored.SavedPrecedence, "a PUT can't carry savedPrecedence, so it should be kept")
		})

		t.Run("through the fabric API replaces hostId and keeps peerData, sourceCtrl and savedPrecedence", func(t *testing.T) {
			ctx.NextTest(t)

			terminator := &model.Terminator{
				Service:         serviceId,
				Router:          routerId,
				Binding:         "edge",
				Address:         "hosted:" + eid.New(),
				Precedence:      xt.Precedences.Failed,
				SavedPrecedence: xt.Precedences.Required,
				HostId:          eid.New(),
				PeerData:        map[uint32][]byte{1: []byte("peer-data")},
				SourceCtrl:      eid.New(),
			}
			terminator.Id = eid.New()
			ctx.Req.NoError(managers.Terminator.Create(terminator, change.New()))

			newHostId := eid.New()
			_, err := fabricApi.Terminator.UpdateTerminator(&fabricTerminator.UpdateTerminatorParams{
				ID: terminator.Id,
				Terminator: &fabricRestModel.TerminatorUpdate{
					Service:    &serviceId,
					Router:     &routerId,
					Binding:    &terminator.Binding,
					Address:    &terminator.Address,
					Cost:       util.Ptr(fabricRestModel.TerminatorCost(10)),
					Precedence: fabricRestModel.TerminatorPrecedenceFailed,
					HostID:     newHostId,
				},
			}, nil)
			ctx.Req.NoError(err)

			stored, err := managers.Terminator.Read(terminator.Id)
			ctx.Req.NoError(err)
			ctx.Req.Equal(uint16(10), stored.Cost, "the PUT should have set the cost")
			ctx.Req.Equal(newHostId, stored.HostId, "the fabric API carries hostId, so the PUT should have replaced it")
			ctx.Req.Equal(terminator.PeerData, stored.PeerData, "a PUT can't carry peerData, so it should be kept")
			ctx.Req.Equal(terminator.SourceCtrl, stored.SourceCtrl, "a PUT can't carry sourceCtrl, so it should be kept")
			ctx.Req.Equal(xt.Precedences.Required, stored.SavedPrecedence, "a PUT can't carry savedPrecedence, so it should be kept")
		})
	})
}
