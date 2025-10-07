package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
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
	currentSubscription := eventHandler.state.GetCurrentDataModelSubscription()

	logger := pfxlog.Logger().
		WithField("ctrlId", ch.Id()).
		WithField("subscribedCtrlId", currentSubscription.CtrlId).
		WithField("subscriptionId", currentSubscription.SubscriptionId)

	newEvent := &edge_ctrl_pb.DataState_ChangeSet{}
	if err := proto.Unmarshal(msg.Body, newEvent); err != nil {
		logger.WithError(err).Errorf("could not unmarshal data state change set message")
		return
	}

	logger = logger.WithField("index", newEvent.Index).WithField("synthetic", newEvent.IsSynthetic)

	// ignore state from controllers we are not currently subscribed to
	if !currentSubscription.IsCurrentController(ch.Id()) {
		logger.Info("data state change received from ctrl other than the one currently subscribed to")
		return
	}

	subscriptionId, ok := msg.GetStringHeader(int32(edge_ctrl_pb.Header_RouterDataModelSubscriptionId))
	if ok && subscriptionId != currentSubscription.SubscriptionId {
		logger.WithField("eventSubscriptionId", subscriptionId).
			Info("data state change received from inactive or invalid subscription")
	}

	err := eventHandler.state.GetRouterDataModelPool().Queue(func() {
		model := eventHandler.state.RouterDataModel()
		logger.Info("received data state change set")
		model.ApplyChangeSet(newEvent)
	})

	if err != nil {
		logger.WithError(err).Errorf("could not queue processing data state change set message, resyncing full router data model")
		// because we were unable to accept events, we're out of sync and will have to completely resync
		eventHandler.state.ResyncRouterDataModel()
	}
}

func (*dataStateChangeSetHandler) ContentType() int32 {
	return controllerEnv.DataStateChangeSetType
}
