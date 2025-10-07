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
	"fmt"
	"math"
	"math/rand"

	"github.com/google/uuid"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
)

func (self *subscriberTest) AddIdentity() {
	if self.rdm.Identities.Count() >= self.maxIdentities {
		return
	}

	identityId := eid.New()

	identity := &edge_ctrl_pb.DataState_Identity{
		Id: identityId,
	}

	self.RandomizeIdentity(identity)

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: identity,
		},
	}

	self.handleEvent(event)
	if self.rdm.subscriptions.Count() < self.maxSubscribers {
		fmt.Printf("subscriber count < %d, adding subscriber %s\n", self.rdm.subscriptions.Count(), identityId)
		if err := self.rdm.SubscribeToIdentityChanges(identityId, self, false); err != nil {
			panic(err)
		}
	}
}

func (self *subscriberTest) ChangeIdentity() {
	identityToChange := self.identityRandStream.Next()

	identity := &edge_ctrl_pb.DataState_Identity{
		Id: identityToChange.Id,
	}

	self.RandomizeIdentity(identity)

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Update,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: identity,
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) RandomizeIdentity(identity *edge_ctrl_pb.DataState_Identity) {
	identity.Name = eid.New()
	identity.DefaultHostingPrecedence = self.randomTerminatorPrecedence()
	identity.DefaultHostingCost = uint32(rand.Intn(math.MaxUint16))
	identity.ServiceHostingPrecedences = map[string]edge_ctrl_pb.TerminatorPrecedence{}
	identity.ServiceHostingCosts = map[string]uint32{}
	identity.AppDataJson = []byte(uuid.NewString())
	identity.Disabled = rand.Int()%2 == 0

	if current, _ := self.rdm.Identities.Get(identity.Id); current != nil {
		counter := 0
		for k := range current.ServiceAccess {
			identity.ServiceHostingPrecedences[k] = self.randomTerminatorPrecedence()
			identity.ServiceHostingCosts[k] = uint32(rand.Intn(math.MaxUint16))

			identity.ServiceConfigs = map[string]*edge_ctrl_pb.DataState_ServiceConfigs{}

			configs := &edge_ctrl_pb.DataState_ServiceConfigs{
				Configs: map[string]string{},
			}

			identity.ServiceConfigs[k] = configs

			attempts := 1
			for len(configs.Configs) < 2 {
				if attempts%5 == 0 {
					self.AddConfig()
				}

				if attempts&10 == 0 {
					self.AddConfigType()
				}

				config := self.configRandStream.Next()
				configs.Configs[config.TypeId] = config.Id

				attempts++
			}

			counter++
			if counter >= 2 {
				break
			}
		}
	}
}

func (self *subscriberTest) randomTerminatorPrecedence() edge_ctrl_pb.TerminatorPrecedence {
	return edge_ctrl_pb.TerminatorPrecedence(rand.Int31n(int32(len(edge_ctrl_pb.TerminatorPrecedence_value))))
}

func (self *subscriberTest) RemoveIdentity() {
	if self.rdm.Identities.Count() <= self.minIdentities {
		return
	}

	identity := self.identityRandStream.Next()
	self.removedIdentities[identity.Id] = struct{}{}
	self.RemoveSelectedIdentity(identity)
}

func (self *subscriberTest) RemoveSelectedIdentity(identity *Identity) {
	changeSet := &edge_ctrl_pb.DataState_ChangeSet{}

	fmt.Printf("removing identity: %s\n", identity.Id)

	self.rdm.withLockedIdentity(identity.Id, func(identity *Identity) {
		for servicePolicyId := range identity.ServicePolicies {
			changeSet.Changes = append(changeSet.Changes,
				&edge_ctrl_pb.DataState_Event{
					Action: edge_ctrl_pb.DataState_Create,
					Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
						ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
							PolicyId:          servicePolicyId,
							RelatedEntityIds:  []string{identity.Id},
							RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
							Add:               false,
						},
					},
				})
		}
	})

	changeSet.Changes = append(changeSet.Changes, &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: identity.ToProtobuf(),
		},
	})

	self.handleChangeSet(changeSet)

	for self.rdm.subscriptions.Count() < self.maxSubscribers {
		id := self.identityRandStream.Next()
		_, identityRemoved := self.removedIdentities[id.Id]
		if !self.rdm.subscriptions.Has(id.Id) && !identityRemoved {
			fmt.Printf("subscriber count < %d, adding subscriber %s\n", self.rdm.subscriptions.Count(), id.Id)
			if err := self.rdm.SubscribeToIdentityChanges(id.Id, self, false); err != nil {
				panic(err)
			}
		}
	}
}
