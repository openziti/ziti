package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
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

func (self *DataStateHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	err := self.state.GetRouterDataModelPool().Queue(func() {
		newState := &edge_ctrl_pb.DataState{}
		if err := proto.Unmarshal(msg.Body, newState); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not marshal data state event message")
			return
		}

		model := common.NewReceiverRouterDataModel(RouterDataModelListerBufferSize, self.state.GetEnv().GetCloseNotify())

		pfxlog.Logger().WithField("index", newState.EndIndex).Info("received full router data model state")
		for _, event := range newState.Events {
			model.Handle(newState.EndIndex, event)
		}

		model.SetCurrentIndex(newState.EndIndex)
		self.state.SetRouterDataModel(model)
		pfxlog.Logger().WithField("index", newState.EndIndex).Info("finished processing full router data model state")
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not queue processing of full router data model state")
	}
}
