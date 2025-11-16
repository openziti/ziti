//go:build perftests

/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package common

import (
	"math/rand"

	"github.com/google/uuid"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
)

func (self *subscriberTest) AddConfigType() {
	if self.rdm.ConfigTypes.Count() >= self.maxConfigTypes {
		return
	}

	configType := &edge_ctrl_pb.DataState_ConfigType{
		Id:   eid.New(),
		Name: eid.New(),
	}

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_ConfigType{
			ConfigType: configType,
		},
	}
	self.handleEvent(event)
}

func (self *subscriberTest) RemoveConfigType() {
	if self.rdm.ConfigTypes.Count() <= self.minConfigTypes {
		return
	}

	configType := self.getRandomConfigType()

	for entry := range self.rdm.Configs.IterBuffered() {
		if entry.Val.TypeId == configType.Id {
			self.RemoveSelectedConfig(entry.Val)
		}
	}

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_ConfigType{
			ConfigType: configType,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) ChangeConfigType() {
	configType := self.getRandomConfigType()
	configType.Name = uuid.NewString()

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Update,
		Model: &edge_ctrl_pb.DataState_Event_ConfigType{
			ConfigType: configType,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) getRandomConfigType() *edge_ctrl_pb.DataState_ConfigType {
	selectedIdx := rand.Intn(self.rdm.ConfigTypes.Count())
	idx := 0

	var configType *ConfigType
	self.rdm.ConfigTypes.IterCb(func(key string, v *ConfigType) {
		if idx == selectedIdx {
			configType = v
		}
		idx++
	})

	return configType.ToProtobuf()
}

func (self *subscriberTest) AddConfig() {
	if self.rdm.Configs.Count() >= self.maxConfigs {
		return
	}

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Config{
			Config: &edge_ctrl_pb.DataState_Config{
				Id:       eid.New(),
				Name:     eid.New(),
				TypeId:   self.getRandomConfigType().Id,
				DataJson: uuid.NewString(),
			},
		},
	}
	self.handleEvent(event)
}

func (self *subscriberTest) ChangeConfig() {
	config := self.configRandStream.Next()
	config.Name = eid.New()
	config.DataJson = uuid.NewString()

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Update,
		Model: &edge_ctrl_pb.DataState_Event_Config{
			Config: config.ToProtobuf(),
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) RemoveConfig() {
	if self.rdm.Configs.Count() <= self.minConfigs {
		return
	}

	config := self.configRandStream.Next()
	self.RemoveSelectedConfig(config)
}

func (self *subscriberTest) RemoveSelectedConfig(config *Config) {
	changeSet := &edge_ctrl_pb.DataState_ChangeSet{}

	config.services.IterCb(func(serviceId string, _ struct{}) {
		service, _ := self.rdm.Services.Get(serviceId)
		updatedService := &edge_ctrl_pb.DataState_Service{
			Id:                 serviceId,
			Name:               service.Name,
			EncryptionRequired: service.EncryptionRequired,
		}

		for _, configId := range service.Configs {
			if configId != config.Id {
				updatedService.Configs = append(updatedService.Configs, configId)
			}
		}

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Update,
				Model: &edge_ctrl_pb.DataState_Event_Service{
					Service: updatedService,
				},
			})
	})

	config.identities.IterCb(func(key string, _ struct{}) {
		identity, _ := self.rdm.Identities.Get(key)

		updatedIdentity := &DataStateIdentity{
			Id:                        identity.Id,
			Name:                      identity.Name,
			DefaultHostingPrecedence:  identity.DefaultHostingPrecedence,
			DefaultHostingCost:        identity.DefaultHostingCost,
			ServiceHostingPrecedences: identity.ServiceHostingPrecedences,
			ServiceHostingCosts:       identity.ServiceHostingCosts,
			AppDataJson:               identity.AppDataJson,
			ServiceConfigs:            map[string]*edge_ctrl_pb.DataState_ServiceConfigs{},
			Disabled:                  identity.Disabled,
		}

		for k, v := range identity.ServiceConfigs {
			configs := map[string]string{}

			for configTypeId, configId := range v.Configs {
				if configId != config.Id {
					configs[configTypeId] = configId
				}
			}

			if len(configs) > 0 {
				updatedIdentity.ServiceConfigs[k] = &edge_ctrl_pb.DataState_ServiceConfigs{
					Configs: configs,
				}
			}
		}

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Update,
				Model: &edge_ctrl_pb.DataState_Event_Identity{
					Identity: updatedIdentity,
				},
			})
	})

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_Config{
			Config: config.ToProtobuf(),
		},
	}

	changeSet.Changes = append(changeSet.Changes, event)
	self.handleChangeSet(changeSet)
}
