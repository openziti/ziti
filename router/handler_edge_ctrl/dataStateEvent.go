package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	controllerEnv "github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/router/state"
	"google.golang.org/protobuf/proto"
)

type dataStateEventHandler struct {
	state state.Manager
}

func NewDataStateEventHandler(state state.Manager) channel.TypedReceiveHandler {
	return &dataStateEventHandler{
		state: state,
	}
}

func (eventHandler *dataStateEventHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	newEvent := &edge_ctrl_pb.DataState_Event{}
	if err := proto.Unmarshal(msg.Body, newEvent); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal data state event message")
		return
	}

	model := eventHandler.state.RouterDataModel()
	model.Apply(newEvent)
}

func (*dataStateEventHandler) ContentType() int32 {
	return controllerEnv.DataStateEventType
}
