package handler_edge_ctrl

import (
	"encoding/json"
	"github.com/openziti/channel/v2"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type ServiceListHandler struct {
	handler func(ch channel.Channel, lastUpdateToken []byte, list []*rest_model.ServiceDetail)
}

func NewServiceListHandler(handler func(ch channel.Channel, lastUpdateToken []byte, list []*rest_model.ServiceDetail)) *ServiceListHandler {
	return &ServiceListHandler{
		handler: handler,
	}
}

func (self *ServiceListHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_ServiceListType)
}

func (self *ServiceListHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	serviceList := &edge_ctrl_pb.ServicesList{}
	if err := proto.Unmarshal(msg.Body, serviceList); err == nil {
		logrus.Debugf("received services list with %v entries", len(serviceList.Services))
		go self.handleServicesList(ch, serviceList)
	} else {
		logrus.WithError(err).Error("could not unmarshal services list")
	}
}

func (self *ServiceListHandler) handleServicesList(ch channel.Channel, list *edge_ctrl_pb.ServicesList) {
	var serviceList []*rest_model.ServiceDetail

	for _, entry := range list.Services {

		var permissions rest_model.DialBindArray

		for _, perm := range entry.Permissions {
			permissions = append(permissions, rest_model.DialBind(perm))
		}

		service := &rest_model.ServiceDetail{
			BaseEntity: rest_model.BaseEntity{
				ID:   &entry.Id,
				Tags: &rest_model.Tags{},
			},
			Name:               &entry.Name,
			Permissions:        permissions,
			EncryptionRequired: &entry.Encryption,
			Config:             map[string]map[string]interface{}{},
		}

		err := json.Unmarshal(entry.Config, &service.Config)
		if err != nil {
			logrus.
				WithError(err).
				WithField("json", string(entry.Config)).
				WithField("service", *service.ID).
				Error("unable to unmarshal config json")
			return
		}

		err = json.Unmarshal(entry.Tags, &service.Tags)
		if err != nil {
			logrus.
				WithError(err).
				WithField("json", string(entry.Tags)).
				WithField("service", *service.ID).
				Error("unable to unmarshal tag json")
			return
		}

		serviceList = append(serviceList, service)
	}
	self.handler(ch, list.LastUpdate, serviceList)
}
