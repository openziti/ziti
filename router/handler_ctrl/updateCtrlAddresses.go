package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

var updateCtrlAddressesHandlerInstance *updateCtrlAddressesHandler

type CtrlAddressUpdater interface {
	UpdateCtrlEndpoints(endpoints []string)
}

type updateCtrlAddressesHandler struct {
	callback       CtrlAddressUpdater
	currentVersion uint64
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
		"endpoints": upd.Addresses,
		"version":   handler.currentVersion,
	})

	if upd.IsLeader || handler.currentVersion == 0 || handler.currentVersion < upd.Index {
		log.Info("updating to controller endpoints to version")
		handler.callback.UpdateCtrlEndpoints(upd.Addresses)
		handler.currentVersion = upd.Index
	}
}

func newUpdateCtrlAddressesHandler(callback CtrlAddressUpdater) channel.TypedReceiveHandler {
	if updateCtrlAddressesHandlerInstance == nil {
		updateCtrlAddressesHandlerInstance = &updateCtrlAddressesHandler{
			callback: callback,
		}
	}
	return updateCtrlAddressesHandlerInstance
}
