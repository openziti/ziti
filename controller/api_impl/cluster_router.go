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

package api_impl

import (
	"fmt"
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/raft"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/controller/rest_server/operations"
	"github.com/openziti/ziti/controller/rest_server/operations/cluster"
	"net/http"
)

func init() {
	r := NewClusterRouter()
	AddRouter(r)
}

type ClusterRouter struct {
}

func NewClusterRouter() *ClusterRouter {
	return &ClusterRouter{}
}

func (r *ClusterRouter) Register(fabricApi *operations.ZitiFabricAPI, wrapper RequestWrapper) {
	fabricApi.ClusterClusterListMembersHandler = cluster.ClusterListMembersHandlerFunc(func(params cluster.ClusterListMembersParams) middleware.Responder {
		return wrapper.WrapRequest(r.listMembers, params.HTTPRequest, "", "")
	})

	fabricApi.ClusterClusterMemberAddHandler = cluster.ClusterMemberAddHandlerFunc(func(params cluster.ClusterMemberAddParams) middleware.Responder {
		return wrapper.WrapRequest(func(network *network.Network, rc api.RequestContext) {
			r.addMember(network, rc, params)
		}, params.HTTPRequest, "", "")
	})

	fabricApi.ClusterClusterMemberRemoveHandler = cluster.ClusterMemberRemoveHandlerFunc(func(params cluster.ClusterMemberRemoveParams) middleware.Responder {
		return wrapper.WrapRequest(func(network *network.Network, rc api.RequestContext) {
			r.removeMember(network, rc, params)
		}, params.HTTPRequest, "", "")
	})

	fabricApi.ClusterClusterTransferLeadershipHandler = cluster.ClusterTransferLeadershipHandlerFunc(func(params cluster.ClusterTransferLeadershipParams) middleware.Responder {
		return wrapper.WrapRequest(func(network *network.Network, rc api.RequestContext) {
			r.transferLeadership(network, rc, params)
		}, params.HTTPRequest, "", "")
	})
}

func (r *ClusterRouter) getClusterController(n *network.Network) *raft.Controller {
	if n.Dispatcher == nil {
		return nil
	}

	if ClusterController, ok := n.Dispatcher.(*raft.Controller); ok {
		return ClusterController
	}

	return nil
}

func (r *ClusterRouter) listMembers(n *network.Network, rc api.RequestContext) {
	ClusterController := r.getClusterController(n)
	if ClusterController != nil {
		vals := make([]*rest_model.ClusterMemberListValue, 0)
		members, err := ClusterController.ListMembers()
		if err != nil {
			rc.Respond(rest_model.ClusterMemberListResponse{}, http.StatusInternalServerError)
		}
		readOnly := ClusterController.Mesh.IsReadOnly()
		for _, member := range members {
			vals = append(vals, &rest_model.ClusterMemberListValue{
				Address:   &member.Addr,
				Connected: &member.Connected,
				ID:        &member.Id,
				Leader:    &member.Leader,
				Version:   &member.Version,
				Voter:     &member.Voter,
				ReadOnly:  &readOnly,
			})
		}

		rc.Respond(rest_model.ClusterMemberListResponse{
			Data: vals,
		}, http.StatusOK)

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}

func (r *ClusterRouter) addMember(n *network.Network, rc api.RequestContext, params cluster.ClusterMemberAddParams) {
	ClusterController := r.getClusterController(n)
	if ClusterController != nil {
		addr := *params.Member.Address

		req := &cmd_pb.AddPeerRequest{
			Addr:    addr,
			IsVoter: *params.Member.IsVoter,
		}

		if err := ClusterController.Join(req); err != nil {
			msg := fmt.Sprintf("unable to add cluster member for supplied address: [%s]", err.Error())
			rc.RespondWithApiError(models.ToApiErrorWithDefault(err, func(err error) *errorz.ApiError {
				return apierror.NewBadRequestFieldError(*errorz.NewFieldError(msg, "address", addr))
			}))
			return
		}

		rc.RespondWithEmptyOk()

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}

func (r *ClusterRouter) removeMember(n *network.Network, rc api.RequestContext, params cluster.ClusterMemberRemoveParams) {
	ClusterController := r.getClusterController(n)
	if ClusterController != nil {
		req := &cmd_pb.RemovePeerRequest{
			Id: *params.Member.ID,
		}

		if err := ClusterController.HandleRemovePeer(req); err != nil {
			msg := fmt.Sprintf("unable to remove cluster member node for supplied node id: [%s]", err.Error())
			rc.RespondWithApiError(models.ToApiErrorWithDefault(err, func(err error) *errorz.ApiError {
				return apierror.NewBadRequestFieldError(*errorz.NewFieldError(msg, "id", *params.Member.ID))
			}))
			return
		}

		rc.RespondWithEmptyOk()

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}

func (r *ClusterRouter) transferLeadership(n *network.Network, rc api.RequestContext, params cluster.ClusterTransferLeadershipParams) {
	ClusterController := r.getClusterController(n)
	if ClusterController != nil {
		req := &cmd_pb.TransferLeadershipRequest{
			Id: params.Member.NewLeaderID,
		}

		if err := ClusterController.HandleTransferLeadership(req); err != nil {
			apiErr := models.ToApiErrorWithDefault(err, apierror.NewTransferLeadershipError)
			rc.RespondWithApiError(apiErr)
			return
		}

		rc.RespondWithEmptyOk()

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}
