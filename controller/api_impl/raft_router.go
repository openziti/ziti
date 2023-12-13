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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"net/http"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/network"
	nfraft "github.com/openziti/ziti/controller/raft"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/controller/rest_server/operations"
	"github.com/openziti/ziti/controller/rest_server/operations/raft"
)

func init() {
	r := NewRaftRouter()
	AddRouter(r)
}

type RaftRouter struct {
}

func NewRaftRouter() *RaftRouter {
	return &RaftRouter{}
}

func (r *RaftRouter) Register(fabricApi *operations.ZitiFabricAPI, wrapper RequestWrapper) {
	fabricApi.RaftRaftListMembersHandler = raft.RaftListMembersHandlerFunc(func(params raft.RaftListMembersParams) middleware.Responder {
		return wrapper.WrapRequest(r.listMembers, params.HTTPRequest, "", "")
	})

	fabricApi.RaftRaftMemberAddHandler = raft.RaftMemberAddHandlerFunc(func(params raft.RaftMemberAddParams) middleware.Responder {
		return wrapper.WrapRequest(func(network *network.Network, rc api.RequestContext) {
			r.addMember(network, rc, params)
		}, params.HTTPRequest, "", "")
	})

	fabricApi.RaftRaftMemberRemoveHandler = raft.RaftMemberRemoveHandlerFunc(func(params raft.RaftMemberRemoveParams) middleware.Responder {
		return wrapper.WrapRequest(func(network *network.Network, rc api.RequestContext) {
			r.removeMember(network, rc, params)
		}, params.HTTPRequest, "", "")
	})

	fabricApi.RaftRaftTranferLeadershipHandler = raft.RaftTranferLeadershipHandlerFunc(func(params raft.RaftTranferLeadershipParams) middleware.Responder {
		return wrapper.WrapRequest(func(network *network.Network, rc api.RequestContext) {
			r.transferLeadership(network, rc, params)
		}, params.HTTPRequest, "", "")
	})
}

func (r *RaftRouter) getRaftController(n *network.Network) *nfraft.Controller {
	if n.Dispatcher == nil {
		return nil
	}

	if raftController, ok := n.Dispatcher.(*nfraft.Controller); ok {
		return raftController
	}

	return nil
}

func (r *RaftRouter) listMembers(n *network.Network, rc api.RequestContext) {
	raftController := r.getRaftController(n)
	if raftController != nil {
		vals := make([]*rest_model.RaftMemberListValue, 0)
		members, err := raftController.ListMembers()
		if err != nil {
			rc.Respond(rest_model.RaftMemberListResponse{}, http.StatusInternalServerError)
		}
		for _, member := range members {
			vals = append(vals, &rest_model.RaftMemberListValue{
				Address:   &member.Addr,
				Connected: &member.Connected,
				ID:        &member.Id,
				Leader:    &member.Leader,
				Version:   &member.Version,
				Voter:     &member.Voter,
			})
		}

		rc.Respond(rest_model.RaftMemberListResponse{
			Values: vals,
		}, http.StatusOK)

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}

func (r *RaftRouter) addMember(n *network.Network, rc api.RequestContext, params raft.RaftMemberAddParams) {
	raftController := r.getRaftController(n)
	if raftController != nil {
		addr := *params.Member.Address
		peerId, peerAddr, err := raftController.Mesh.GetPeerInfo(addr, 15*time.Second)
		if err != nil {
			msg := fmt.Sprintf("unable to retrieve cluster member id [%s] for supplied address", err.Error())
			rc.RespondWithApiError(apierror.NewBadRequestFieldError(*errorz.NewFieldError(msg, "address", addr)))
			return
		}

		id := string(peerId)
		addr = string(peerAddr)

		req := &cmd_pb.AddPeerRequest{
			Addr:    addr,
			Id:      id,
			IsVoter: *params.Member.IsVoter,
		}

		if err = raftController.Join(req); err != nil {
			msg := fmt.Sprintf("unable to add cluster member for supplied address: [%s]", err.Error())
			rc.RespondWithApiError(apierror.NewBadRequestFieldError(*errorz.NewFieldError(msg, "address", addr)))
			return
		}

		rc.RespondWithEmptyOk()

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}

func (r *RaftRouter) removeMember(n *network.Network, rc api.RequestContext, params raft.RaftMemberRemoveParams) {
	raftController := r.getRaftController(n)
	if raftController != nil {
		req := &cmd_pb.RemovePeerRequest{
			Id: *params.Member.ID,
		}

		if err := raftController.HandleRemovePeer(req); err != nil {
			msg := fmt.Sprintf("unable to remove cluster member node for supplied node id: [%s]", err.Error())
			rc.RespondWithApiError(apierror.NewBadRequestFieldError(*errorz.NewFieldError(msg, "id", *params.Member.ID)))
			return
		}

		rc.RespondWithEmptyOk()

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}

func (r *RaftRouter) transferLeadership(n *network.Network, rc api.RequestContext, params raft.RaftTranferLeadershipParams) {
	raftController := r.getRaftController(n)
	if raftController != nil {
		req := &cmd_pb.TransferLeadershipRequest{
			Id: params.Member.NewLeaderID,
		}

		if err := raftController.HandleTransferLeadership(req); err != nil {
			rc.RespondWithApiError(&errorz.ApiError{
				Code:        apierror.TransferLeadershipErrorCode,
				Message:     apierror.TransferLeadershipErrorMessage,
				Status:      apierror.TransferLeadershipErrorStatus,
				Cause:       err,
				AppendCause: true,
			})
			return
		}

		rc.RespondWithEmptyOk()

	} else {
		rc.RespondWithApiError(apierror.NewNotRunningInHAModeError())
	}
}
