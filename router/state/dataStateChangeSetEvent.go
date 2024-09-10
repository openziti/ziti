package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	controllerEnv "github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
)

type dataStateChangeSetHandler struct {
	state Manager
}

func NewDataStateEventHandler(state Manager) channel.TypedReceiveHandler {
	return &dataStateChangeSetHandler{
		state: state,
	}
}

func (eventHandler *dataStateChangeSetHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	newEvent := &edge_ctrl_pb.DataState_ChangeSet{}
	if err := proto.Unmarshal(msg.Body, newEvent); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal data state change set message")
		return
	}

	model := eventHandler.state.RouterDataModel()
	pfxlog.Logger().WithField("index", newEvent.Index).Info("received data state change set")
	model.ApplyChangeSet(newEvent)
}

func (*dataStateChangeSetHandler) ContentType() int32 {
	return controllerEnv.DataStateChangeSetType
}
