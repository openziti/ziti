package handler_edge_ctrl

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/storage/ast"
	"github.com/sirupsen/logrus"
	"time"
)

type listTunnelServicesHandler struct {
	baseRequestHandler
	*TunnelState
}

func NewListTunnelServicesHandler(appEnv *env.AppEnv, ch channel2.Channel, tunnelState *TunnelState) channel2.ReceiveHandler {
	return &listTunnelServicesHandler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
		TunnelState:        tunnelState,
	}
}

func (self *listTunnelServicesHandler) getTunnelState() *TunnelState {
	return self.TunnelState
}

func (self *listTunnelServicesHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_ListServicesRequestType)
}

func (self *listTunnelServicesHandler) Label() string {
	return "tunnel.list.services"
}

func (self *listTunnelServicesHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	logger := logrus.WithField("router", self.ch.Id().Token)

	ctx := &listTunnelServicesRequestContext{
		baseTunnelRequestContext: baseTunnelRequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		},
	}

	if len(msg.Body) > 0 {
		if len(msg.Body) > 50 {
			logger.Errorf("list service request too long, exiting channel. len=%v", len(msg.Body))
			_ = self.ch.Close()
			return
		}

		if err := ctx.lastUpdate.UnmarshalBinary(msg.Body); err != nil {
			logger.WithError(err).Errorf("failed to parse last update time: %v", string(msg.Body))
		}
	}

	go self.listServices(ctx)
}

func (self *listTunnelServicesHandler) listServices(ctx *listTunnelServicesRequestContext) {
	if !ctx.loadRouter() {
		return
	}

	ctx.loadIdentity()
	ctx.ensureApiSession(nil)

	logger := logrus.WithField("router", ctx.sourceRouter.Name)

	if ctx.err != nil {
		logger.WithError(ctx.err).Error("could not load identity")
		return
	}

	var lastUpdate time.Time
	if val, found := self.appEnv.IdentityRefreshMap.Get(ctx.identity.Id); found {
		lastUpdate = val.(time.Time)
	} else {
		lastUpdate = self.appEnv.StartupTime
	}

	if !ctx.lastUpdate.Before(lastUpdate) {
		logger.Debug("service list requested, but no update available")
		return
	}

	query, err := ast.Parse(self.appEnv.BoltStores.EdgeService, "limit none")
	if err != nil {
		logger.WithError(err).Error("could not create service list query")
		return
	}

	result, err := self.appEnv.Handlers.EdgeService.PublicQueryForIdentity(ctx.identity, ctx.apiSession.ConfigTypes, query)
	if err != nil {
		logger.WithError(err).Error("could not create service list query")
		return
	}

	serviceList := &edge_ctrl_pb.ServicesList{}
	for _, modelService := range result.Services {
		var configData []byte
		if configData, err = json.Marshal(modelService.Config); err != nil {
			logger.WithError(err).WithField("service", modelService.Id).Error("failed to parse config data for service")
			return
		}

		var tagData []byte
		if tagData, err = json.Marshal(modelService.Tags); err != nil {
			logger.WithError(err).WithField("service", modelService.Id).Error("failed to parse tag data for service")
			return
		}
		service := &edge_ctrl_pb.TunnelService{
			Id:          modelService.Id,
			Name:        modelService.Name,
			Permissions: modelService.Permissions,
			Encryption:  modelService.EncryptionRequired,
			Config:      configData,
			Tags:        tagData,
		}
		serviceList.Services = append(serviceList.Services, service)
	}

	t, err := lastUpdate.MarshalBinary()
	if err != nil {
		logger.WithError(err).Error("failed to marshal last update time")
	}
	serviceList.LastUpdate = t

	body, err := proto.Marshal(serviceList)
	if err != nil {
		logger.Error("failed to marshal services list")
		return
	}

	serviceListMsg := channel2.NewMessage(serviceList.GetContentType(), body)
	if err = self.ch.Send(serviceListMsg); err != nil {
		logger.Error("failed to send services list")
	}
}

type listTunnelServicesRequestContext struct {
	baseTunnelRequestContext
	lastUpdate time.Time
}
