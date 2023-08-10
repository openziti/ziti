package controller

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/raft"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
)

// OnConnectSettingsHandler sends a ctrl_pb.ContentType_SettingsType message when routers connect if necessary
// Settings are a map of  int32 -> []byte data. The type should be used to determine how the setting's []byte
// array is consumed.
type OnConnectSettingsHandler struct {
	config   *Config
	settings map[int32][]byte
}

func (o *OnConnectSettingsHandler) RouterDisconnected(r *network.Router) {
	//do nothing, satisfy interface
}

func (o OnConnectSettingsHandler) RouterConnected(r *network.Router) {
	if len(o.settings) > 0 {
		settingsMsg := &ctrl_pb.Settings{
			Data: map[int32][]byte{},
		}

		for k, v := range o.settings {
			settingsMsg.Data[k] = v
		}

		if body, err := proto.Marshal(settingsMsg); err == nil {
			msg := channel.NewMessage(int32(ctrl_pb.ContentType_SettingsType), body)
			if err := r.Control.Send(msg); err == nil {
				pfxlog.Logger().WithError(err).WithFields(map[string]interface{}{
					"routerId": r.Id,
					"channel":  r.Control.LogicalName(),
				}).Error("error sending settings on router connect")
			}
		}

	} else {
		pfxlog.Logger().WithFields(map[string]interface{}{
			"routerId": r.Id,
			"channel":  r.Control.LogicalName(),
		}).Info("no on connect settings to send")
	}
}

type OnConnectCtrlAddressesUpdateHandler struct {
	ctrlAddress string
	raft        *raft.Controller
}

func NewOnConnectCtrlAddressesUpdateHandler(ctrlAddress string, raft *raft.Controller) *OnConnectCtrlAddressesUpdateHandler {
	return &OnConnectCtrlAddressesUpdateHandler{
		ctrlAddress: ctrlAddress,
		raft:        raft,
	}
}

func (o *OnConnectCtrlAddressesUpdateHandler) RouterDisconnected(r *network.Router) {
	//do nothing, satisfy interface
}

func (o OnConnectCtrlAddressesUpdateHandler) RouterConnected(r *network.Router) {
	log := pfxlog.Logger().WithFields(map[string]interface{}{
		"routerId": r.Id,
		"channel":  r.Control.LogicalName(),
	})
	log.Info("Router connected... syncing ctrl addresses")
	index, data := o.raft.CtrlAddresses()
	log.Info(data)

	updMsg := &ctrl_pb.UpdateCtrlAddresses{
		Addresses: data,
		Index:     index,
	}

	if err := protobufs.MarshalTyped(updMsg).Send(r.Control); err != nil {
		log.WithError(err).Error("error sending UpdateCtrlAddresses on router connect")
	}
}
