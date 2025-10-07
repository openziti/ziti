package state

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
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
	logger := pfxlog.Logger().WithField("ctrlId", ch.Id())
	currentCtrlId := self.state.GetCurrentDataModelSource()

	// ignore state from controllers we are not currently subscribed to
	if currentCtrlId != ch.Id() {
		logger.WithField("dataModelSrcId", currentCtrlId).Info("data state received from ctrl other than the one currently subscribed to")
		return
	}

	err := self.state.GetRouterDataModelPool().Queue(func() {
		newState := &edge_ctrl_pb.DataState{}
		if err := proto.Unmarshal(msg.Body, newState); err != nil {
			logger.WithError(err).Errorf("could not marshal data state event message")
			return
		}

		logger.WithField("index", newState.EndIndex).Info("received full router data model state")

		model := common.NewReceiverRouterDataModelFromDataState(newState, self.state.GetEnv().GetCloseNotify())

		fileName := fmt.Sprintf("/tmp/model-%s-%d.json", ch.Id(), newState.EndIndex)
		output, _ := json.Marshal(model.ToJson())
		_ = os.WriteFile(fileName, output, 0644)
		self.state.SetRouterDataModel(model, false)

		logger.WithField("index", newState.EndIndex).Info("finished processing full router data model state")
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not queue processing of full router data model state")
	}
}
