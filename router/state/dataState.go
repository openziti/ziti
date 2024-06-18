package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	controllerEnv "github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
)

type DataStateHandler struct {
	state Manager
}

func NewDataStateHandler(state Manager) *DataStateHandler {
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

	model := common.NewReceiverRouterDataModel(RouterDataModelListerBufferSize, dsh.state.GetEnv().GetCloseNotify())

	pfxlog.Logger().WithField("endIndex", newState.EndIndex).Debug("received full router data model state")
	for _, event := range newState.Events {
		model.Handle(newState.EndIndex, event)
	}

	model.SetCurrentIndex(newState.EndIndex)
	dsh.state.SetRouterDataModel(model)
}
