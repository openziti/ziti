package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
)

var updateCtrlAddressesHandlerInstance *updateCtrlAddressesHandler

type CtrlAddressUpdater interface {
	UpdateCtrlEndpoints(endpoints []string) error
}

type updateCtrlAddressesHandler struct {
	callback CtrlAddressUpdater

	currentVersion uint64
}

func (handler *updateCtrlAddressesHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateCtrlAddressesType)
}

func (handler *updateCtrlAddressesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	upd := &ctrl_pb.UpdateCtrlAddresses{}
	if err := proto.Unmarshal(msg.Body, upd); err != nil {
		log.WithError(err).Error("error unmarshalling")
		return
	}

	if upd.IsLeader || handler.currentVersion == 0 || handler.currentVersion < upd.Index {
		if err := handler.callback.UpdateCtrlEndpoints(upd.Addresses); err != nil {
			log.WithError(err).Error("Unable to update ctrl addresses")
		}
		handler.currentVersion = upd.Index
		log.Infof("updated to version %v", handler.currentVersion)
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
