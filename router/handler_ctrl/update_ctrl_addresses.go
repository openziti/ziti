package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
)

var updateCtrlAddressesHandlerInstance *updateCtrlAddressesHandler

type CtrlAddressUpdater interface {
	UpdateCtrlEndpoints(endpoints []string)
	UpdateLeader(leaderId string)
}

type updateCtrlAddressesHandler struct {
	callback       CtrlAddressUpdater
	currentVersion atomic.Uint64
}

func (handler *updateCtrlAddressesHandler) NotifyIndexReset() {
	handler.currentVersion.Store(0)
}

func (handler *updateCtrlAddressesHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateCtrlAddressesType)
}

func (handler *updateCtrlAddressesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry
	upd := &ctrl_pb.UpdateCtrlAddresses{}
	if err := proto.Unmarshal(msg.Body, upd); err != nil {
		log.WithError(err).Error("error unmarshalling")
		return
	}

	log = log.WithFields(logrus.Fields{
		"endpoints":     upd.Addresses,
		"localVersion":  handler.currentVersion.Load(),
		"remoteVersion": upd.Index,
		"isLeader":      upd.IsLeader,
		"ctrlId":        ch.Id(),
	})

	log.Info("update ctrl endpoints message received")

	if handler.currentVersion.Load() == 0 || handler.currentVersion.Load() < upd.Index {
		if len(upd.Addresses) > 0 {
			log.Info("updating to newer controller endpoints")
			handler.callback.UpdateCtrlEndpoints(upd.Addresses)
			handler.currentVersion.Store(upd.Index)

			if upd.IsLeader {
				handler.callback.UpdateLeader(ch.Id())
			}
		} else {
			log.Info("ignoring empty controller endpoint list")
		}
	} else {
		log.Info("ignoring outdated controller endpoint list")
	}
}

func newUpdateCtrlAddressesHandler(env env.RouterEnv, callback CtrlAddressUpdater) channel.TypedReceiveHandler {
	if updateCtrlAddressesHandlerInstance == nil {
		updateCtrlAddressesHandlerInstance = &updateCtrlAddressesHandler{
			callback: callback,
		}
		env.GetIndexWatchers().AddIndexResetWatcher(updateCtrlAddressesHandlerInstance.NotifyIndexReset)
	}
	return updateCtrlAddressesHandlerInstance
}
