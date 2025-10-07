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

func (self *subscriberTest) AddPostureCheck() {
	if self.rdm.PostureChecks.Count() >= self.maxPostureChecks {
		return
	}

	macCheck := &edge_ctrl_pb.DataState_PostureCheck_Mac{}

	for range rand.Intn(5) + 1 {
		macCheck.MacAddresses = append(macCheck.MacAddresses, uuid.NewString())
	}

	postureCheck := &edge_ctrl_pb.DataState_PostureCheck{
		Id:     eid.New(),
		TypeId: "MAC",
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
			Mac: macCheck,
		},
	}

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
			PostureCheck: postureCheck,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) ChangePostureCheck() {
	postureCheckToChange := self.postureCheckRandStream.Next()

	macCheck := &edge_ctrl_pb.DataState_PostureCheck_Mac{}

	for range rand.Intn(5) + 1 {
		macCheck.MacAddresses = append(macCheck.MacAddresses, uuid.NewString())
	}

	postureCheck := &edge_ctrl_pb.DataState_PostureCheck{
		Id:     postureCheckToChange.Id,
		TypeId: postureCheckToChange.TypeId,
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
			Mac: macCheck,
		},
	}

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Update,
		Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
			PostureCheck: postureCheck,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) RemovePostureCheck() {
	if self.rdm.PostureChecks.Count() <= self.minPostureChecks {
		return
	}

	postureCheck := self.postureCheckRandStream.Next()

	changeSet := &edge_ctrl_pb.DataState_ChangeSet{}

	postureCheck.servicePolicies.IterCb(func(key string, v struct{}) {
		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          key,
						RelatedEntityIds:  []string{postureCheck.Id},
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
						Add:               false,
					},
				},
			})
	})

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
			PostureCheck: postureCheck.DataStatePostureCheck,
		},
	}

	changeSet.Changes = append(changeSet.Changes, event)

	self.handleChangeSet(changeSet)
}
