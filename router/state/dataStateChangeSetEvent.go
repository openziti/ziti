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
	currentCtrlId := eventHandler.state.GetCurrentDataModelSource()

	logger := pfxlog.Logger().WithField("ctrlId", ch.Id())

	newEvent := &edge_ctrl_pb.DataState_ChangeSet{}
	if err := proto.Unmarshal(msg.Body, newEvent); err != nil {
		logger.WithError(err).Errorf("could not unmarshal data state change set message")
		return
	}

	logger = logger.WithField("index", newEvent.Index).WithField("synthetic", newEvent.IsSynthetic)

	// ignore state from controllers we are not currently subscribed to
	if currentCtrlId != ch.Id() {
		logger.WithField("dataModelSrcId", currentCtrlId).Info("data state received from ctrl other than the one currently subscribed to")
		return
	}

	err := eventHandler.state.GetRouterDataModelPool().Queue(func() {
		model := eventHandler.state.RouterDataModel()
		logger.Info("received data state change set")
		model.ApplyChangeSet(newEvent)
	})

	if err != nil {
		logger.WithError(err).Errorf("could not queue processing data state change set message")
	}
}

func (*dataStateChangeSetHandler) ContentType() int32 {
	return controllerEnv.DataStateChangeSetType
}
