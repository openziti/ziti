package controller

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel"
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
