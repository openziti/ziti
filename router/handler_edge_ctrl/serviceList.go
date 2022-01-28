package handler_edge_ctrl

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/channel"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
)

type ServiceListHandler struct {
	handler func(lastUpdateToken []byte, list []*edge.Service)
}

func NewServiceListHandler(handler func(lastUpdateToken []byte, list []*edge.Service)) *ServiceListHandler {
	return &ServiceListHandler{
		handler: handler,
	}
}

func (self *ServiceListHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_ServiceListType)
}

func (self *ServiceListHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	serviceList := &edge_ctrl_pb.ServicesList{}
	if err := proto.Unmarshal(msg.Body, serviceList); err == nil {
		logrus.Debugf("received services list with %v entries", len(serviceList.Services))
		go self.handleServicesList(serviceList)
	} else {
		logrus.WithError(err).Error("could not unmarshal services list")
	}
}

func (self *ServiceListHandler) handleServicesList(list *edge_ctrl_pb.ServicesList) {
	var serviceList []*edge.Service
	for _, entry := range list.Services {
		service := &edge.Service{
			Id:          entry.Id,
			Name:        entry.Name,
			Permissions: entry.Permissions,
			Encryption:  entry.Encryption,
			Configs:     map[string]map[string]interface{}{},
			Tags:        map[string]string{},
		}

		err := json.Unmarshal(entry.Config, &service.Configs)
		if err != nil {
			logrus.
				WithError(err).
				WithField("json", string(entry.Config)).
				WithField("service", service.Id).
				Error("unable to unmarshal config json")
			return
		}

		err = json.Unmarshal(entry.Tags, &service.Tags)
		if err != nil {
			logrus.
				WithError(err).
				WithField("json", string(entry.Tags)).
				WithField("service", service.Id).
				Error("unable to unmarshal tag json")
			return
		}

		serviceList = append(serviceList, service)
	}
	self.handler(list.LastUpdate, serviceList)
}
