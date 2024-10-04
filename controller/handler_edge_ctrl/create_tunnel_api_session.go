package handler_edge_ctrl

import (
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type createApiSessionHandler struct {
	baseRequestHandler
	*TunnelState
}

func NewCreateApiSessionHandler(appEnv *env.AppEnv, ch channel.Channel, tunnelState *TunnelState) channel.TypedReceiveHandler {
	return &createApiSessionHandler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
		TunnelState:        tunnelState,
	}
}

func (self *createApiSessionHandler) getTunnelState() *TunnelState {
	return self.TunnelState
}

func (self *createApiSessionHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateApiSessionRequestType)
}

func (self *createApiSessionHandler) Label() string {
	return "tunnel.create.api_session"
}

func (self *createApiSessionHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	req := &edge_ctrl_pb.CreateApiSessionRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		logrus.WithField("routerId", self.ch.Id()).WithError(err).Error("could not unmarshal CreateApiSessionRequest")
		return
	}

	logrus.WithField("routerId", self.ch.Id()).Debug("create api session request received")

	ctx := &createApiSessionRequestContext{
		baseTunnelRequestContext: baseTunnelRequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		},
		req: req,
	}

	go self.createApiSession(ctx)
}

func (self *createApiSessionHandler) createApiSession(ctx *createApiSessionRequestContext) {
	if !ctx.loadRouter() {
		return
	}

	ctx.loadIdentity()
	ctx.ensureApiSession(ctx.req.ConfigTypes)
	ctx.updateIdentityInfo(ctx.req.EnvInfo, ctx.req.SdkInfo)

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	result, err := ctx.getCreateApiSessionResponse()
	if err != nil {
		self.returnError(ctx, internalError(err))
		return
	}

	body, err := proto.Marshal(result)
	if err != nil {
		self.returnError(ctx, internalError(err))
		return
	}

	responseMsg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateApiSessionResponseType), body)
	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		logrus.WithField("routerId", self.ch.Id()).WithError(err).Error("failed to send response")
	} else {
		logrus.WithField("routerId", self.ch.Id()).Debug("create api session response sent")
	}
}

type createApiSessionRequestContext struct {
	baseTunnelRequestContext
	req *edge_ctrl_pb.CreateApiSessionRequest
}
