package handler_mgmt

import (
	"fmt"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/mgmt_pb"
)

type snapshotDbHandler struct {
	network *network.Network
}

func newSnapshotDbHandler(network *network.Network) *snapshotDbHandler {
	return &snapshotDbHandler{
		network: network,
	}
}

func (h *snapshotDbHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_SnapshotDbRequestType)
}

func (h *snapshotDbHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	if err := h.network.SnapshotDatabase(); err == nil {
		handler_common.SendSuccess(msg, ch, "")
	} else {
		handler_common.SendFailure(msg, ch, fmt.Sprintf("error snapshotting db: (%v)", err))
	}
}
