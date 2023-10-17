package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	controllerEnv "github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/router/state"
	"google.golang.org/protobuf/proto"
)

type DataStateHandler struct {
	state state.Manager
}

func NewDataStateHandler(state state.Manager) *DataStateHandler {
	return &DataStateHandler{
		state: state,
	}
}

func (*DataStateHandler) ContentType() int32 {
	return controllerEnv.DataStateType
}

func (dsh *DataStateHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	newState := &edge_ctrl_pb.DataState{}
	if err := proto.Unmarshal(msg.Body, newState); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal data state event message")
		return
	}

	model := common.NewReceiverRouterDataModel(state.RouterDataModelListerBufferSize)

	pfxlog.Logger().WithField("endIndex", newState.EndIndex).Debug("received full router data model state")
	for _, event := range newState.Events {
		model.Handle(event)
	}

	model.SetCurrentIndex(newState.EndIndex)
	dsh.state.SetRouterDataModel(model)
}
