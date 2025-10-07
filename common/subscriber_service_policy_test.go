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

func (self *subscriberTest) AddServicePolicy() string {
	if self.rdm.ServicePolicies.Count() >= self.maxServicePolicies {
		return ""
	}

	policyId := eid.New()
	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
			ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
				Id:   policyId,
				Name: eid.New(),
				PolicyType: func() edge_ctrl_pb.PolicyType {
					if rand.Int()%2 == 0 {
						return edge_ctrl_pb.PolicyType_DialPolicy
					}
					return edge_ctrl_pb.PolicyType_BindPolicy
				}(),
			},
		},
	}

	self.handleEvent(event)
	return policyId
}

func (self *subscriberTest) RemoveServicePolicy() {
	if self.rdm.ServicePolicies.Count() <= self.minServicePolicies {
		return
	}

	servicePolicy := self.servicePolicyRandStream.Next()
	changeSet := &edge_ctrl_pb.DataState_ChangeSet{}

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          servicePolicy.Id,
					RelatedEntityIds:  servicePolicy.Identities.Keys(),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
					Add:               false,
				},
			},
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          servicePolicy.Id,
					RelatedEntityIds:  servicePolicy.Services.Keys(),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
					Add:               false,
				},
			},
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          servicePolicy.Id,
					RelatedEntityIds:  servicePolicy.Identities.Keys(),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
					Add:               false,
				},
			},
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          servicePolicy.Id,
					RelatedEntityIds:  servicePolicy.PostureChecks.Keys(),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
					Add:               false,
				},
			},
		})

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
			ServicePolicy: servicePolicy.ToProtobuf(),
		},
	}

	changeSet.Changes = append(changeSet.Changes, event)

	self.handleChangeSet(changeSet)
}

func (self *subscriberTest) RemoveIdentitiesFromPolicy(policy *ServicePolicy, changeSet *edge_ctrl_pb.DataState_ChangeSet) map[string]struct{} {
	idsToRemoveCount := min(policy.Identities.Count(), rand.Intn(15))
	idsToRemove := map[string]struct{}{}
	if idsToRemoveCount > 0 {
		policy.Identities.IterCb(func(key string, _ struct{}) {
			if len(idsToRemove) < idsToRemoveCount {
				idsToRemove[key] = struct{}{}
			}
		})

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          policy.Id,
						RelatedEntityIds:  slices.Collect(maps.Keys(idsToRemove)),
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
						Add:               false,
					},
				},
			})

		return idsToRemove
	}

	return idsToRemove
}

func (self *subscriberTest) AddIdentitiesToPolicy(n int, policy *ServicePolicy, removed map[string]struct{}, changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	if n == 0 {
		return
	}

	identityIds := self.identityRandStream.GetFilteredSet(n,
		func(identity *Identity) bool {
			_, hasKey := policy.Identities.Get(identity.Id)
			_, keyDeleted := removed[identity.Id]
			return !hasKey && !keyDeleted
		},
		func(identity *Identity) string {
			return identity.Id
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          policy.Id,
					RelatedEntityIds:  slices.Collect(maps.Keys(identityIds)),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
					Add:               true,
				},
			},
		})
}

func (self *subscriberTest) RemoveServicesFromPolicy(policy *ServicePolicy, changeSet *edge_ctrl_pb.DataState_ChangeSet) map[string]struct{} {
	servicesToRemoveCount := min(policy.Services.Count(), rand.Intn(15))
	servicesToRemove := map[string]struct{}{}
	if servicesToRemoveCount > 0 {
		policy.Services.IterCb(func(key string, v struct{}) {
			if len(servicesToRemove) < servicesToRemoveCount {
				servicesToRemove[key] = struct{}{}
			}
		})

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          policy.Id,
						RelatedEntityIds:  slices.Collect(maps.Keys(servicesToRemove)),
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
						Add:               false,
					},
				},
			})

	}

	return servicesToRemove
}

func (self *subscriberTest) AddServicesToPolicy(n int, policy *ServicePolicy, removed map[string]struct{}, changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	if n == 0 {
		return
	}

	serviceIds := self.serviceRandStream.GetFilteredSet(n,
		func(service *Service) bool {
			_, keyDeleted := removed[service.Id]
			return !policy.Services.Has(service.Id) && !keyDeleted
		},
		func(service *Service) string {
			return service.Id
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          policy.Id,
					RelatedEntityIds:  slices.Collect(maps.Keys(serviceIds)),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
					Add:               true,
				},
			},
		})
}

func (self *subscriberTest) RemovePostureChecksFromPolicy(policy *ServicePolicy, changeSet *edge_ctrl_pb.DataState_ChangeSet) map[string]struct{} {
	postureChecksToRemoveCount := min(policy.PostureChecks.Count(), rand.Intn(2))
	postureChecksToRemove := map[string]struct{}{}
	if postureChecksToRemoveCount > 0 {
		policy.PostureChecks.IterCb(func(key string, v struct{}) {
			if len(postureChecksToRemove) < postureChecksToRemoveCount {
				postureChecksToRemove[key] = struct{}{}
			}
		})

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          policy.Id,
						RelatedEntityIds:  slices.Collect(maps.Keys(postureChecksToRemove)),
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
						Add:               false,
					},
				},
			})

	}

	return postureChecksToRemove
}

func (self *subscriberTest) AddPostureChecksToPolicy(n int, policy *ServicePolicy, removed map[string]struct{}, changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	if n == 0 {
		return
	}

	postureCheckIds := self.postureCheckRandStream.GetFilteredSet(n,
		func(postureCheck *PostureCheck) bool {
			_, keyDeleted := removed[postureCheck.Id]
			return !policy.Services.Has(postureCheck.Id) && !keyDeleted
		},
		func(postureCheck *PostureCheck) string {
			return postureCheck.Id
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          policy.Id,
					RelatedEntityIds:  slices.Collect(maps.Keys(postureCheckIds)),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
					Add:               true,
				},
			},
		})
}

func (self *subscriberTest) ChangeServicePolicies(policy *ServicePolicy) {
	changeSet := &edge_ctrl_pb.DataState_ChangeSet{}

	idsToRemove := self.RemoveIdentitiesFromPolicy(policy, changeSet)
	missingIds := self.identitiesPerServicePolicy - (policy.Identities.Count() - len(idsToRemove))
	self.AddIdentitiesToPolicy(missingIds, policy, idsToRemove, changeSet)

	servicesToRemove := self.RemoveServicesFromPolicy(policy, changeSet)
	missingServices := self.servicesPerServicePolicy - (policy.Services.Count() - len(servicesToRemove))
	self.AddServicesToPolicy(missingServices, policy, servicesToRemove, changeSet)

	postureChecksToRemove := self.RemovePostureChecksFromPolicy(policy, changeSet)
	missingPostureChecks := rand.Intn(self.maxPostureChecksPerServicePolicy+1) - (policy.PostureChecks.Count() - len(postureChecksToRemove))
	if missingPostureChecks > 0 {
		self.AddPostureChecksToPolicy(missingPostureChecks, policy, postureChecksToRemove, changeSet)
	}

	self.handleChangeSet(changeSet)
}
