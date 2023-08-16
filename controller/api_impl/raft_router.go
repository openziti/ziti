package api_impl

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/network"
	nfraft "github.com/openziti/fabric/controller/raft"
	"github.com/openziti/fabric/controller/rest_model"
	"github.com/openziti/fabric/controller/rest_server/operations"
	"github.com/openziti/fabric/controller/rest_server/operations/raft"
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
		return wrapper.WrapRequest(r.ListMembers, params.HTTPRequest, "", "")
	})
}

func (r *RaftRouter) ListMembers(n *network.Network, rc api.RequestContext) {
	vals := make([]*rest_model.RaftMemberListValue, 0)

	if n.Dispatcher != nil {
		rctrl := n.Dispatcher.(*nfraft.Controller)
		members, err := rctrl.ListMembers()
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
	}

	rc.Respond(rest_model.RaftMemberListResponse{
		Values: vals,
	}, http.StatusOK)
}
