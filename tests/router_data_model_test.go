//go:build dataflow
// +build dataflow

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

package tests

import (
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/eid"
	"sync"
	"testing"
	"time"
)

type identityEvent struct {
	state     *common.IdentityState
	eventType common.IdentityEventType
}

type serviceEvent struct {
	state     *common.IdentityState
	service   *common.IdentityService
	eventType common.ServiceEventType
}

type testSubscriber struct {
	ctx            *TestContext
	identityEvents chan *identityEvent
	serviceEvents  chan *serviceEvent
	currentState   *common.IdentityState
	mutex          sync.Mutex
}

func newTestSubscriber(ctx *TestContext) *testSubscriber {
	return &testSubscriber{
		ctx:            ctx,
		identityEvents: make(chan *identityEvent, 100),
		serviceEvents:  make(chan *serviceEvent, 100),
	}
}

func (self *testSubscriber) NotifyIdentityEvent(state *common.IdentityState, eventType common.IdentityEventType) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.identityEvents <- &identityEvent{
		state:     state,
		eventType: eventType,
	}
	self.currentState = state
}

func (self *testSubscriber) NotifyServiceChange(state *common.IdentityState, service *common.IdentityService, eventType common.ServiceEventType) {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	self.serviceEvents <- &serviceEvent{
		state:     state,
		service:   service,
		eventType: eventType,
	}
	self.currentState = state
}

func (self *testSubscriber) getNextIdentityEvent(eventType common.IdentityEventType) *identityEvent {
	select {
	case evt := <-self.identityEvents:
		self.ctx.Equal(eventType, evt.eventType)
		return evt
	case <-time.After(time.Second):
		self.ctx.Fail("timed out waiting for identity event")
		return nil
	}
}

func (self *testSubscriber) getNextServiceEvent(eventType common.ServiceEventType) *serviceEvent {
	select {
	case evt := <-self.serviceEvents:
		self.ctx.Equal(eventType, evt.eventType)
		return evt
	case <-time.After(time.Second):
		self.ctx.Fail("timed out waiting for service event")
		return nil
	}
}

func Test_RouterDataModel(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	sub := newTestSubscriber(ctx)

	identityRole1 := eid.New()
	identityRole2 := eid.New()

	testIdentity, _ := ctx.AdminManagementSession.requireCreateIdentityWithUpdbEnrollment(eid.New(), eid.New(), false, identityRole1, identityRole2)
	ctx.Req.NoError(ctx.router.GetRouterDataModel().SubscribeToIdentityChanges(testIdentity.Id, sub, false))

	// test that initial event shows up
	idEvent := sub.getNextIdentityEvent(common.EventFullState)
	ctx.Equal(0, len(idEvent.state.Services))
	ctx.Equal(0, len(idEvent.state.PostureChecks))

	serviceRole1 := eid.New()
	serviceRole2 := eid.New()
	service1 := ctx.AdminManagementSession.requireNewService(s(serviceRole1), nil)

	policy1 := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AnyOf", s("#"+serviceRole1, "#"+serviceRole2), s("#"+identityRole1), s())

	svcEvent := sub.getNextServiceEvent(common.EventAccessGained)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(false, svcEvent.service.BindAllowed)

	// add and remove a policy to ensure no extraneous events are created
	policy2 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole1), s("#"+identityRole1), s())
	ctx.AdminManagementSession.requireDeleteEntity(policy2)

	// add a policy for later
	_ = ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole2), s("#"+identityRole1), s())

	// add a bind policy to ensure and make sure the service change shows up
	policy2 = ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AnyOf", s("#"+serviceRole1, "#"+serviceRole2), s("#"+identityRole1), s())

	svcEvent = sub.getNextServiceEvent(common.EventUpdated)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(true, svcEvent.service.BindAllowed)

	// remove the initial policy, bind should now be disabled
	ctx.AdminManagementSession.requireDeleteEntity(policy1)

	svcEvent = sub.getNextServiceEvent(common.EventUpdated)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Equal(false, svcEvent.service.DialAllowed)
	ctx.Equal(true, svcEvent.service.BindAllowed)

	service2 := ctx.AdminManagementSession.requireNewService(s(serviceRole2), nil)
	svcEvent = sub.getNextServiceEvent(common.EventAccessGained)
	ctx.Equal(service2.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(false, svcEvent.service.BindAllowed)

	// testing losing access via loss of policy
	ctx.AdminManagementSession.requireDeleteEntity(policy2)

	svcEvent = sub.getNextServiceEvent(common.EventAccessRemoved)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)

	// testing losing access via service being removed
	ctx.AdminManagementSession.requireDeleteEntity(service2)
	svcEvent = sub.getNextServiceEvent(common.EventAccessRemoved)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)

	//service1 = ctx.AdminManagementSession.requireNewService(s(serviceRole1), nil)

}
