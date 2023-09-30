package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/router/fabric"
	"google.golang.org/protobuf/proto"
)

type signingCertAddedHandler struct {
	state fabric.StateManager
}

func NewSigningCertAddedHandler(state fabric.StateManager) *signingCertAddedHandler {
	return &signingCertAddedHandler{
		state: state,
	}
}

func (h *signingCertAddedHandler) ContentType() int32 {
	return env.SigningCertAdded
}

func (h *signingCertAddedHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	signingCerts := &edge_ctrl_pb.SignerCerts{}
	if err := proto.Unmarshal(msg.Body, signingCerts); err == nil {
		h.state.AddSignerPublicCert(signingCerts.Keys)
	} else {
		pfxlog.Logger().WithError(err).Errorf("could not marshal signing certs message")
	}
}
