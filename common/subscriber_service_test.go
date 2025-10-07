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
	"maps"
	"math/rand"
	"slices"

	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
)

func (self *subscriberTest) AddService() {
	if self.rdm.Services.Count() >= self.maxServices {
		return
	}

	service := &edge_ctrl_pb.DataState_Service{
		Id: eid.New(),
	}

	self.RandomizeService(service)

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: service,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) RandomizeService(service *edge_ctrl_pb.DataState_Service) {
	service.Name = eid.New()
	service.EncryptionRequired = rand.Int()%2 == 0

	configs := map[string]string{}

	for len(configs) < 2 {
		config := self.configRandStream.Next()
		configs[config.TypeId] = config.Id
	}

	service.Configs = slices.Collect(maps.Values(configs))
}

func (self *subscriberTest) ChangeService() {
	serviceToChange := self.serviceRandStream.Next()

	service := &edge_ctrl_pb.DataState_Service{
		Id: serviceToChange.Id,
	}

	self.RandomizeService(service)

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Update,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: service,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) RemoveService() {
	if self.rdm.Services.Count() <= self.minServices {
		return
	}

	service := self.serviceRandStream.Next()

	changeSet := &edge_ctrl_pb.DataState_ChangeSet{}

	service.servicePolicies.IterCb(func(key string, v struct{}) {
		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          key,
						RelatedEntityIds:  []string{service.Id},
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
						Add:               false,
					},
				},
			})
	})

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: service.ToProtobuf(),
		},
	}

	changeSet.Changes = append(changeSet.Changes, event)

	self.handleChangeSet(changeSet)
}
