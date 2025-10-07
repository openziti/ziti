package state

import (
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
	currentSubscription := self.state.GetCurrentDataModelSubscription()

	logger := pfxlog.Logger().
		WithField("ctrlId", ch.Id()).
		WithField("subscribedCtrlId", currentSubscription.CtrlId).
		WithField("subscriptionId", currentSubscription.SubscriptionId)

	// ignore state from controllers we are not currently subscribed to
	if !currentSubscription.IsCurrentController(ch.Id()) {
		logger.Info("data state received from ctrl other than the one currently subscribed to")
		return
	}

	subscriptionId, ok := msg.GetStringHeader(int32(edge_ctrl_pb.Header_RouterDataModelSubscriptionId))
	if ok && subscriptionId != currentSubscription.SubscriptionId {
		logger.WithField("eventSubscriptionId", subscriptionId).
			Info("data state received from inactive or invalid subscription")
	}

	err := self.state.GetRouterDataModelPool().Queue(func() {
		newState := &edge_ctrl_pb.DataState{}
		if err := proto.Unmarshal(msg.Body, newState); err != nil {
			logger.WithError(err).Errorf("could not marshal data state event message")
			return
		}

		logger.WithField("index", newState.EndIndex).Info("received full router data model state")

		model := common.NewReceiverRouterDataModelFromDataState(newState, self.state.GetEnv().GetCloseNotify())
		self.state.SetRouterDataModel(model, false)

		logger.WithField("index", newState.EndIndex).Info("finished processing full router data model state")
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not queue processing of full router data model state")
	}
}
