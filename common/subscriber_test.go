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
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/agent"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/version"
)

type testState struct {
	identity      *Identity
	services      map[string]*IdentityService
	postureChecks map[string]*PostureCheck
}

type subscriberTest struct {
	rdm *RouterDataModel

	minIdentities      int
	maxIdentities      int
	identityRandStream *randomStream[*Identity]

	minServices       int
	maxServices       int
	serviceRandStream *randomStream[*Service]

	minPostureChecks       int
	maxPostureChecks       int
	postureCheckRandStream *randomStream[*PostureCheck]

	minServicePolicies      int
	maxServicePolicies      int
	servicePolicyRandStream *randomStream[*ServicePolicy]

	minConfigTypes int
	maxConfigTypes int

	minConfigs       int
	maxConfigs       int
	configRandStream *randomStream[*Config]

	identitiesPerServicePolicy       int
	servicesPerServicePolicy         int
	maxPostureChecksPerServicePolicy int

	maxSubscribers           int
	maxSubscribersToValidate int

	index uint64

	notifications atomic.Int64

	lock          sync.Mutex
	identityState map[string]*testState

	removedIdentities map[string]struct{}
}

func (self *subscriberTest) handleEvent(event *edge_ctrl_pb.DataState_Event) {
	self.index++
	self.rdm.Handle(self.index, event)
}

func (self *subscriberTest) handleChangeSet(changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	self.index++
	changeSet.Index = self.index
	for _, event := range changeSet.Changes {
		self.rdm.Handle(changeSet.Index, event)
	}
}

func (self *subscriberTest) NotifyIdentityEvent(state *IdentityState, eventType IdentityEventType) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.notifications.Add(1)

	if eventType == IdentityDeletedEvent {
		delete(self.identityState, state.Identity.Id)
		return
	}

	currentState := self.identityState[state.Identity.Id]
	if currentState == nil {
		currentState = &testState{}
		self.identityState[state.Identity.Id] = currentState
	}

	if eventType == IdentityUpdatedEvent || eventType == IdentityFullStateState {
		currentState.identity = state.Identity
	}

	if eventType == IdentityFullStateState {
		currentState.services = state.Services
	}

	if eventType == IdentityPostureChecksUpdatedEvent || eventType == IdentityFullStateState {
		currentState.postureChecks = state.PostureChecks
	}
}

func (self *subscriberTest) NotifyServiceChange(state *IdentityState, service *IdentityService, eventType ServiceEventType) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.notifications.Add(1)

	currentState := self.identityState[state.Identity.Id]
	if currentState == nil {
		currentState = &testState{
			identity:      state.Identity,
			services:      map[string]*IdentityService{},
			postureChecks: state.PostureChecks,
		}
		self.identityState[state.Identity.Id] = currentState
	}

	switch eventType {
	case ServiceAccessGainedEvent:
		fallthrough
	case ServiceUpdatedEvent:
		currentState.services[service.GetId()] = service
	case ServiceAccessLostEvent:
		delete(currentState.services, service.GetId())
	}
}

func (self *subscriberTest) initializeRouterDataModel() {
	fmt.Print("adding config types... ")
	for i := 0; i < self.minConfigTypes; i++ {
		self.AddConfigType()
	}
	fmt.Printf("%d config types added\n", self.rdm.ConfigTypes.Count())

	fmt.Print("adding configs... ")
	for i := 0; i < self.minConfigs; i++ {
		self.AddConfig()
	}
	fmt.Printf("%d configs added\n", self.rdm.Configs.Count())

	go self.configRandStream.run()

	fmt.Print("adding services... ")
	for i := 0; i < self.minServices; i++ {
		self.AddService()
	}
	fmt.Printf("%d services added\n", self.rdm.Services.Count())

	go self.serviceRandStream.run()

	fmt.Print("adding posture checks... ")
	for i := 0; i < self.minPostureChecks; i++ {
		self.AddPostureCheck()
	}
	fmt.Printf("%d posture checks added\n", self.rdm.PostureChecks.Count())

	go self.postureCheckRandStream.run()

	fmt.Print("adding identities... ")
	for i := 0; i < self.minIdentities; i++ {
		self.AddIdentity()
	}
	fmt.Printf("%d identities added\n", self.rdm.Identities.Count())

	go self.identityRandStream.run()

	fmt.Print("adding service policies... ")
	for i := 0; i < self.minServicePolicies; i++ {
		self.AddServicePolicy()
	}
	fmt.Printf("%d Services policies added\n", self.rdm.ServicePolicies.Count())

	go self.servicePolicyRandStream.run()

	for tuple := range self.rdm.ServicePolicies.IterBuffered() {
		self.ChangeServicePolicies(tuple.Val)
	}

	for tuple := range self.rdm.ServicePolicies.IterBuffered() {
		policy := tuple.Val
		if policy.Services.Count() != self.servicesPerServicePolicy {
			fmt.Printf("policy %s doesn't have enough Services: %d\n", policy.Id, policy.Services.Count())
		}
		if policy.Identities.Count() != self.identitiesPerServicePolicy {
			fmt.Printf("policy %s doesn't have enough identities: %d\n", policy.Id, policy.Identities.Count())
		}
	}

	start := time.Now()
	self.rdm.waitForQueueEmpty()
	fmt.Printf("sync subscribers time: %v, with %d notifications\n", time.Since(start), self.notifications.Load())
	self.notifications.Store(0)
}

func TestSubscriberCorrectness(t *testing.T) {
	closeNotify := make(chan struct{})
	rdm := NewReceiverRouterDataModel(closeNotify)

	options := agent.Options{
		AppId:      "subscriber-test",
		AppType:    "test",
		AppVersion: version.GetVersion(),
	}

	if err := agent.Listen(options); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
	}

	test := &subscriberTest{
		rdm:                rdm,
		minIdentities:      1_000,
		maxIdentities:      1_200,
		identityRandStream: newRandomStream(rdm.Identities),

		minServices:       10_000,
		maxServices:       12_000,
		serviceRandStream: newRandomStream(rdm.Services),

		minPostureChecks:       25,
		maxPostureChecks:       100,
		postureCheckRandStream: newRandomStream(rdm.PostureChecks),

		minServicePolicies:      1_000,
		maxServicePolicies:      2_000,
		servicePolicyRandStream: newRandomStream(rdm.ServicePolicies),

		minConfigTypes: 10,
		maxConfigTypes: 15,

		minConfigs:       500,
		maxConfigs:       1000,
		configRandStream: newRandomStream(rdm.Configs),

		identitiesPerServicePolicy:       100,
		servicesPerServicePolicy:         250,
		maxPostureChecksPerServicePolicy: 5,

		maxSubscribers:           5,
		maxSubscribersToValidate: 5,

		identityState:     map[string]*testState{},
		removedIdentities: map[string]struct{}{},
	}

	test.initializeRouterDataModel()
	test.validateSubscriptions()

	identityId := test.rdm.subscriptions.Keys()[0]
	identity, _ := test.rdm.Identities.Get(identityId)

	test.RemoveSelectedIdentity(identity)
	test.rdm.waitForQueueEmpty()
	test.validateSubscriptions()

	for j := 0; j < 1000; j++ {
		for i := 0; i < 10; i++ {
			start := time.Now()
			n := rand.Intn(20)
			iters := rand.Intn(10) + 1
			test.updateTestModel(n, iters)

			fmt.Printf("make rdm changes time: %v\n", time.Since(start))

			test.rdm.waitForQueueEmpty()
			fmt.Printf("sync subscribers time: %v, with %d notifications\n", time.Since(start), test.notifications.Load())

			fmt.Printf("%d: total elapsed: %s\n\n", i, time.Since(start))
			test.notifications.Store(0)

			test.validateSubscriptions()
		}

		for i := 0; i < scenarioChangeServicePolicy; i++ {
			start := time.Now()
			test.updateTestModel(i, 2)

			fmt.Printf("make rdm changes time: %v\n", time.Since(start))

			test.rdm.waitForQueueEmpty()
			fmt.Printf("sync subscribers time: %v, with %d notifications\n", time.Since(start), test.notifications.Load())

			fmt.Printf("%d: total elapsed: %s\n\n", i, time.Since(start))
			test.notifications.Store(0)

			test.validateSubscriptions()
		}
	}
}

func TestSubscriberScale(t *testing.T) {
	closeNotify := make(chan struct{})
	rdm := NewReceiverRouterDataModel(closeNotify)

	options := agent.Options{
		AppId:      "subscriber-test",
		AppType:    "test",
		AppVersion: version.GetVersion(),
	}

	if err := agent.Listen(options); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
	}

	test := &subscriberTest{
		rdm:                rdm,
		minIdentities:      100_000,
		maxIdentities:      102_000,
		identityRandStream: newRandomStream(rdm.Identities),

		minServices:       10_000,
		maxServices:       12_000,
		serviceRandStream: newRandomStream(rdm.Services),

		minPostureChecks:       25,
		maxPostureChecks:       100,
		postureCheckRandStream: newRandomStream(rdm.PostureChecks),

		minServicePolicies:      5_000,
		maxServicePolicies:      10_000,
		servicePolicyRandStream: newRandomStream(rdm.ServicePolicies),

		minConfigTypes: 10,
		maxConfigTypes: 15,

		minConfigs:       500,
		maxConfigs:       1000,
		configRandStream: newRandomStream(rdm.Configs),

		identitiesPerServicePolicy:       100,
		servicesPerServicePolicy:         250,
		maxPostureChecksPerServicePolicy: 5,

		maxSubscribers:           1000,
		maxSubscribersToValidate: 10,

		identityState:     map[string]*testState{},
		removedIdentities: map[string]struct{}{},
	}

	test.initializeRouterDataModel()

	for j := 0; j < 10; j++ {
		for i := 0; i < 10; i++ {
			start := time.Now()
			n := rand.Intn(20)
			iters := rand.Intn(10) + 1
			test.updateTestModel(n, iters)

			fmt.Printf("make rdm changes time: %v\n", time.Since(start))

			//test.rdm.waitForQueueEmpty()
			//fmt.Printf("sync subscribers time: %v, with %d notifications\n", time.Since(start), test.notifications.Load())

			fmt.Printf("%d: total elapsed: %s\n\n", i, time.Since(start))
			test.notifications.Store(0)
		}

		for i := 0; i < scenarioChangeServicePolicy; i++ {
			start := time.Now()
			test.updateTestModel(i, 2)

			fmt.Printf("make rdm changes time: %v\n", time.Since(start))

			//test.rdm.waitForQueueEmpty()
			//fmt.Printf("sync subscribers time: %v, with %d notifications\n", time.Since(start), test.notifications.Load())

			fmt.Printf("%d: total elapsed: %s\n\n", i, time.Since(start))
			test.notifications.Store(0)
		}
	}
}

const (
	scenarioAddConfigType = iota
	scenarioRemoveConfigType
	scenarioChangeConfigType
	scenarioAddConfig
	scenarioRemoveConfig
	scenarioChangeConfig
	scenarioAddService
	scenarioRemoveService
	scenarioChangeService
	scenarioAddIdentity
	scenarioRemoveIdentity
	scenarioChangeIdentity
	scenarioAddServicePolicy
	scenarioRemoveServicePolicy
	scenarioChangeServicePolicy
)

func (self *subscriberTest) validateSubscriptions() {
	start := time.Now()

	diffSink := func(entityType string, id string, diffType DiffType, detail string) {
		err := fmt.Errorf("%s (direct) id: %s diffType: %s, detail: %s", entityType, id, diffType, detail)
		panic(err)
	}

	count := 0

	self.rdm.subscriptions.IterCb(func(identityId string, v *IdentitySubscription) {
		count++
		if count > self.maxSubscribersToValidate {
			return
		}

		v.Diff(self.rdm, false, diffSink)

		self.lock.Lock()
		defer self.lock.Unlock()

		currentState := self.identityState[identityId]
		if currentState == nil {
			panic(fmt.Errorf("no identity state for %s", identityId))
		}

		sub := &IdentitySubscription{
			IdentityId: identityId,
			Identity:   currentState.identity,
			Services:   currentState.services,
			Checks:     currentState.postureChecks,
		}

		if len(v.Services) == 0 && v.Services != nil && sub.Services == nil {
			sub.Services = map[string]*IdentityService{}
		}

		if len(v.Checks) == 0 && v.Checks != nil && sub.Checks == nil {
			sub.Checks = map[string]*PostureCheck{}
		}

		sub.DiffWith(v, diffSink)
	})

	fmt.Printf("validated %d subscribers time: %v\n\n", self.rdm.subscriptions.Count(), time.Since(start))
}

func (self *subscriberTest) updateTestModel(n, iters int) {
	switch n {
	// config types
	case scenarioAddConfigType:
		fmt.Printf("adding a config type\n")
		self.AddConfigType()
	case scenarioRemoveConfigType:
		fmt.Printf("removing a config type\n")
		self.RemoveConfigType()
	case scenarioChangeConfigType:
		fmt.Printf("changing a config type\n")
		self.ChangeConfigType()

	// configs
	case scenarioAddConfig:
		fmt.Printf("adding %d configs\n", iters)
		for range iters {
			self.AddConfig()
		}
	case scenarioRemoveConfig:
		fmt.Printf("removing %d configs\n", iters)
		for range iters {
			self.RemoveConfig()
		}
	case scenarioChangeConfig:
		fmt.Printf("changing %d configs\n", iters)
		for range iters {
			self.ChangeConfigType()
		}

	// identities
	case scenarioAddIdentity:
		fmt.Printf("adding %d identities\n", iters)
		for range iters {
			self.AddIdentity()
		}
	case scenarioRemoveIdentity:
		fmt.Printf("removing %d identities\n", iters)
		for range iters {
			self.RemoveIdentity()
		}
		clear(self.removedIdentities)
	case scenarioChangeIdentity:
		fmt.Printf("changing %d identities\n", iters)
		for range iters {
			self.ChangeIdentity()
		}

	// services
	case scenarioAddService:
		fmt.Printf("adding %d services\n", iters)
		for range iters {
			self.AddService()
		}
	case scenarioRemoveService:
		fmt.Printf("removing %d services\n", iters)
		for range iters {
			self.RemoveService()
		}
	case scenarioChangeService:
		fmt.Printf("changing %d services\n", iters)
		for range iters {
			self.ChangeService()
		}

	// service polices
	case scenarioAddServicePolicy:
		fmt.Printf("adding %d policies\n", iters)
		for range iters {
			policyId := self.AddServicePolicy()
			if policyId != "" {
				p, _ := self.rdm.ServicePolicies.Get(policyId)
				self.ChangeServicePolicies(p)
			}
		}
	case scenarioRemoveServicePolicy:
		fmt.Printf("remove %d service policies\n", iters)
		for range iters {
			self.RemoveServicePolicy()
		}

	case scenarioChangeServicePolicy:
		fallthrough

	default:
		fmt.Printf("changing %d service policies\n", iters)
		for range iters {
			self.ChangeServicePolicies(self.servicePolicyRandStream.Next())
		}
	}

}
