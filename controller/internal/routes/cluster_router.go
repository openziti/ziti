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

package routes

import (
	"fmt"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/permissions"
	"github.com/openziti/ziti/controller/raft"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/controller/rest_server/operations/cluster"
)

func init() {
	r := NewClusterRouter()
	env.AddRouter(r)
}

type ClusterRouter struct {
}

func NewClusterRouter() *ClusterRouter {
	return &ClusterRouter{}
}

func (r *ClusterRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.ClusterClusterListMembersHandler = cluster.ClusterListMembersHandlerFunc(func(params cluster.ClusterListMembersParams) middleware.Responder {
		return ae.IsAllowed(r.listMembers, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.ClusterClusterMemberAddHandler = cluster.ClusterMemberAddHandlerFunc(func(params cluster.ClusterMemberAddParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.addMember(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.ClusterClusterMemberRemoveHandler = cluster.ClusterMemberRemoveHandlerFunc(func(params cluster.ClusterMemberRemoveParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.removeMember(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.ClusterClusterTransferLeadershipHandler = cluster.ClusterTransferLeadershipHandlerFunc(func(params cluster.ClusterTransferLeadershipParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.transferLeadership(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (r *ClusterRouter) getClusterController(ae *env.AppEnv) *raft.Controller {
	dispatcher := ae.Managers.Dispatcher
	if dispatcher == nil {
		return nil
	}

	if ClusterController, ok := dispatcher.(*raft.Controller); ok {
		return ClusterController
	}

	return nil
}

func (r *ClusterRouter) listMembers(ae *env.AppEnv, rc *response.RequestContext) {
	ClusterController := r.getClusterController(ae)
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

func (r *ClusterRouter) addMember(ae *env.AppEnv, rc *response.RequestContext, params cluster.ClusterMemberAddParams) {
	ClusterController := r.getClusterController(ae)
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

func (r *ClusterRouter) removeMember(ae *env.AppEnv, rc *response.RequestContext, params cluster.ClusterMemberRemoveParams) {
	ClusterController := r.getClusterController(ae)
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

func (r *ClusterRouter) transferLeadership(ae *env.AppEnv, rc *response.RequestContext, params cluster.ClusterTransferLeadershipParams) {
	ClusterController := r.getClusterController(ae)
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
