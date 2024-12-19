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
	logger := pfxlog.Logger().WithField("ctrlId", ch.Id())
	currentCtrlId := eventHandler.state.GetCurrentDataModelSource()

	// ignore state from controllers we are not currently subscribed to
	if currentCtrlId != ch.Id() {
		logger.WithField("dataModelSrcId", currentCtrlId).Info("data state received from ctrl other than the one currently subscribed to")
		return
	}

	err := eventHandler.state.GetRouterDataModelPool().Queue(func() {
		newEvent := &edge_ctrl_pb.DataState_ChangeSet{}
		if err := proto.Unmarshal(msg.Body, newEvent); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not marshal data state change set message")
			return
		}

		model := eventHandler.state.RouterDataModel()
		pfxlog.Logger().WithField("index", newEvent.Index).Info("received data state change set")
		model.ApplyChangeSet(newEvent)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not queue processing data state change set message")
	}
}

func (*dataStateChangeSetHandler) ContentType() int32 {
	return controllerEnv.DataStateChangeSetType
}
