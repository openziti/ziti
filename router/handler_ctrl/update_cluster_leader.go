package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
)

var updateClusterLeaderHandlerInstance *updateClusterLeaderHandler

type updateClusterLeaderHandler struct {
	callback       CtrlAddressUpdater
	currentVersion atomic.Uint64
}

func (handler *updateClusterLeaderHandler) NotifyIndexReset() {
	handler.currentVersion.Store(0)
}

func (handler *updateClusterLeaderHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateClusterLeaderRequestType)
}

func (handler *updateClusterLeaderHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry
	upd := &ctrl_pb.UpdateClusterLeader{}
	if err := proto.Unmarshal(msg.Body, upd); err != nil {
		log.WithError(err).Error("error unmarshalling")
		return
	}

	log = log.WithFields(logrus.Fields{
		"localVersion":  handler.currentVersion.Load(),
		"remoteVersion": upd.Index,
		"ctrlId":        ch.Id(),
	})

	if handler.currentVersion.Load() == 0 || handler.currentVersion.Load() < upd.Index {
		log.Info("handling update of cluster leader")
		handler.callback.UpdateLeader(ch.Id())
		handler.currentVersion.Store(upd.Index)
	} else {
		log.Info("ignoring outdated update cluster leader message")
	}
}

func newUpdateClusterLeaderHandler(env env.RouterEnv, callback CtrlAddressUpdater) channel.TypedReceiveHandler {
	if updateClusterLeaderHandlerInstance == nil {
		updateClusterLeaderHandlerInstance = &updateClusterLeaderHandler{
			callback: callback,
		}
		env.GetIndexWatchers().AddIndexResetWatcher(updateClusterLeaderHandlerInstance.NotifyIndexReset)
	}
	return updateClusterLeaderHandlerInstance
}
