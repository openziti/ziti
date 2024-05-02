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
	"encoding/json"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/posture_checks"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"strings"
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
	ctx                *TestContext
	identityEvents     chan *identityEvent
	serviceEvents      chan *serviceEvent
	currentState       *common.IdentityState
	mutex              sync.Mutex
	savedServiceEvents []*serviceEvent
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

func (self *testSubscriber) getSavedEvent(eventType common.ServiceEventType) *serviceEvent {
	var newList []*serviceEvent
	var result *serviceEvent
	for _, savedEvent := range self.savedServiceEvents {
		if savedEvent.eventType == eventType {
			result = savedEvent
		} else {
			newList = append(newList, savedEvent)
		}
	}
	self.savedServiceEvents = newList
	return result
}

func (self *testSubscriber) getNextServiceEvent(eventType common.ServiceEventType) *serviceEvent {
	evt := self.getSavedEvent(eventType)
	if evt != nil {
		return evt
	}

	select {
	case evt = <-self.serviceEvents:
		self.ctx.Equal(eventType, evt.eventType)
		return evt
	case <-time.After(time.Second):
		self.ctx.Fail("timed out waiting for service event")
		return nil
	}
}

func (self *testSubscriber) getNextServiceEventOfType(eventType common.ServiceEventType) *serviceEvent {
	evt := self.getSavedEvent(eventType)
	if evt != nil {
		return evt
	}

	start := time.Now()
	for time.Since(start) < time.Second {
		select {
		case evt = <-self.serviceEvents:
			if evt.eventType == eventType {
				return evt
			} else {
				self.savedServiceEvents = append(self.savedServiceEvents, evt)
			}
		case <-time.After(time.Second):
			self.ctx.Fail("timed out waiting for service event")
			return nil
		}
	}
	self.ctx.Fail("timed out waiting for service event")
	return nil
}

func Test_RouterDataModel_ServicePolicies(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	router := ctx.CreateEnrollAndStartEdgeRouter()

	sub := newTestSubscriber(ctx)

	identityRole1 := eid.New()
	identityRole2 := eid.New()

	testIdentity, _ := ctx.AdminManagementSession.requireCreateIdentityWithUpdbEnrollment(eid.New(), eid.New(), false, identityRole1, identityRole2)
	ctx.Req.NoError(router.GetRouterDataModel().SubscribeToIdentityChanges(testIdentity.Id, sub, false))

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

	// remove the initial policy, dial should now be disabled
	ctx.AdminManagementSession.requireDeleteEntity(policy1)

	svcEvent = sub.getNextServiceEvent(common.EventUpdated)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Equal(false, svcEvent.service.DialAllowed)
	ctx.Equal(true, svcEvent.service.BindAllowed)

	service2 := ctx.AdminManagementSession.requireNewService(s(serviceRole2), nil)
	svcEvent = sub.getNextServiceEvent(common.EventAccessGained)
	ctx.Equal(service2.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(true, svcEvent.service.BindAllowed)

	// testing losing access via loss of policy
	ctx.AdminManagementSession.requireDeleteEntity(policy2)

	svcEvent = sub.getNextServiceEventOfType(common.EventAccessRemoved)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)

	svcEvent = sub.getNextServiceEventOfType(common.EventUpdated)
	ctx.Equal(service2.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(false, svcEvent.service.BindAllowed)

	// testing losing access via service being removed
	ctx.AdminManagementSession.requireDeleteEntity(service2)
	svcEvent = sub.getNextServiceEvent(common.EventAccessRemoved)
	ctx.Equal(service2.Id, svcEvent.service.Service.Id)
}

func Test_RouterDataModel_Configs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	router := ctx.CreateEnrollAndStartEdgeRouter()

	sub := newTestSubscriber(ctx)

	identityRole1 := eid.New()
	identityRole2 := eid.New()

	testIdentity, _ := ctx.AdminManagementSession.requireCreateIdentityWithUpdbEnrollment(eid.New(), eid.New(), false, identityRole1, identityRole2)
	ctx.Req.NoError(router.GetRouterDataModel().SubscribeToIdentityChanges(testIdentity.Id, sub, false))

	// test that initial event shows up
	idEvent := sub.getNextIdentityEvent(common.EventFullState)
	ctx.Equal(0, len(idEvent.state.Services))
	ctx.Equal(0, len(idEvent.state.PostureChecks))

	ct := ctx.newConfigType()
	ct.Schema = map[string]interface{}{
		"$id":                  "http://ziti-edge.netfoundry.io/schemas/test.config.json",
		"type":                 "object",
		"additionalProperties": false,
		"required": []interface{}{
			"hostname",
			"port",
		},
		"properties": map[string]interface{}{
			"hostname": map[string]interface{}{
				"type": "string",
			},
			"port": map[string]interface{}{
				"type": "number",
			},
		},
	}
	ctx.AdminManagementSession.requireCreateEntity(ct)

	cfg := ctx.newConfig(ct.Id, map[string]interface{}{
		"port":     float64(22),
		"hostname": "ssh.globotech.bizniz",
	})
	ctx.AdminManagementSession.requireCreateEntity(cfg)

	serviceRole1 := eid.New()
	service1 := ctx.AdminManagementSession.requireNewService(s(serviceRole1), s(cfg.Id))

	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole1), s("#"+identityRole1), s())

	svcEvent := sub.getNextServiceEvent(common.EventAccessGained)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(false, svcEvent.service.BindAllowed)
	ctx.NotNil(svcEvent.service.Configs[ct.Name])
	ctx.Equal(ct.Id, svcEvent.service.Configs[ct.Name].ConfigType.Id)
	ctx.Equal(ct.Name, svcEvent.service.Configs[ct.Name].ConfigType.Name)
	ctx.Equal(cfg.Id, svcEvent.service.Configs[ct.Name].Config.Id)
	ctx.Equal(cfg.Name, svcEvent.service.Configs[ct.Name].Config.Name)

	configData := map[string]interface{}{}
	ctx.NoError(json.Unmarshal([]byte(svcEvent.service.Configs[ct.Name].Config.DataJson), &configData))
	ctx.Equal(float64(22), configData["port"])
	ctx.Equal("ssh.globotech.bizniz", configData["hostname"])

	// create new config type and config, and ensure we don't get any spurious events
	ct2 := ctx.newConfigType()
	ct2.Schema = map[string]interface{}{
		"$id":                  "http://ziti-edge.netfoundry.io/schemas/test.config.json",
		"type":                 "object",
		"additionalProperties": false,
		"required": []interface{}{
			"port",
		},
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type": "number",
			},
		},
	}
	ctx.AdminManagementSession.requireCreateEntity(ct2)

	cfg2 := ctx.newConfig(ct2.Id, map[string]interface{}{
		"port": float64(22),
	})
	ctx.AdminManagementSession.requireCreateEntity(cfg2)

	// change config type name
	oldConfigTypeName := ct.Name
	ct.Name = eid.New()
	ctx.AdminManagementSession.requireUpdateEntity(ct)

	svcEvent = sub.getNextServiceEvent(common.EventUpdated)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Nil(svcEvent.service.Configs[oldConfigTypeName])
	ctx.NotNil(svcEvent.service.Configs[ct.Name])
	ctx.Equal(ct.Id, svcEvent.service.Configs[ct.Name].ConfigType.Id)
	ctx.Equal(ct.Name, svcEvent.service.Configs[ct.Name].ConfigType.Name)
	ctx.Equal(cfg.Id, svcEvent.service.Configs[ct.Name].Config.Id)
	ctx.Equal(cfg.Name, svcEvent.service.Configs[ct.Name].Config.Name)

	cfg.Data = map[string]interface{}{
		"port":     float64(33),
		"hostname": "fizzy.globotech.bizniz",
	}
	ctx.AdminManagementSession.requireUpdateEntity(cfg)

	svcEvent = sub.getNextServiceEvent(common.EventUpdated)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.NotNil(svcEvent.service.Configs[ct.Name])
	ctx.Equal(ct.Id, svcEvent.service.Configs[ct.Name].ConfigType.Id)
	ctx.Equal(ct.Name, svcEvent.service.Configs[ct.Name].ConfigType.Name)
	ctx.Equal(cfg.Id, svcEvent.service.Configs[ct.Name].Config.Id)
	ctx.Equal(cfg.Name, svcEvent.service.Configs[ct.Name].Config.Name)

	configData = map[string]interface{}{}
	ctx.NoError(json.Unmarshal([]byte(svcEvent.service.Configs[ct.Name].Config.DataJson), &configData))
	ctx.Equal(float64(33), configData["port"])
	ctx.Equal("fizzy.globotech.bizniz", configData["hostname"])
}

func Test_RouterDataModel_PostureChecks(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	router := ctx.CreateEnrollAndStartEdgeRouter()

	sub := newTestSubscriber(ctx)

	identityRole1 := eid.New()
	identityRole2 := eid.New()

	testIdentity, _ := ctx.AdminManagementSession.requireCreateIdentityWithUpdbEnrollment(eid.New(), eid.New(), false, identityRole1, identityRole2)
	ctx.Req.NoError(router.GetRouterDataModel().SubscribeToIdentityChanges(testIdentity.Id, sub, false))

	// test that initial event shows up
	idEvent := sub.getNextIdentityEvent(common.EventFullState)
	ctx.Equal(0, len(idEvent.state.Services))
	ctx.Equal(0, len(idEvent.state.PostureChecks))

	postureCheck1 := &rest_model.PostureCheckMacAddressCreate{
		MacAddresses: []string{strings.ReplaceAll(uuid.NewString(), "-", "")},
	}
	postureCheckRole1 := eid.New()
	postureCheck1.SetName(ToPtr("check1"))
	postureCheck1.SetRoleAttributes(ToPtr(rest_model.Attributes(s(postureCheckRole1))))

	resp, err := ctx.RestClients.Edge.PostureChecks.CreatePostureCheck(&posture_checks.CreatePostureCheckParams{
		PostureCheck: postureCheck1,
	}, nil)
	ctx.NoError(err)
	postureCheck1Id := resp.Payload.Data.ID

	serviceRole1 := eid.New()
	service1 := ctx.AdminManagementSession.requireNewService(s(serviceRole1), nil)

	postureCheckRole2 := eid.New()
	sp := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AnyOf", s("#"+serviceRole1), s("#"+identityRole1), s("#"+postureCheckRole1, "#"+postureCheckRole2))

	svcEvent := sub.getNextServiceEvent(common.EventAccessGained)
	ctx.Equal(service1.Id, svcEvent.service.Service.Id)
	ctx.Equal(true, svcEvent.service.DialAllowed)
	ctx.Equal(false, svcEvent.service.BindAllowed)

	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(1, len(idEvent.state.PostureChecks))
	testPostureCheck := idEvent.state.PostureChecks[postureCheck1Id]
	ctx.NotNil(testPostureCheck)
	subType, ok := testPostureCheck.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mac_)
	ctx.True(ok)
	ctx.Equal(1, len(subType.Mac.MacAddresses))
	ctx.Equal(postureCheck1.MacAddresses[0], subType.Mac.MacAddresses[0])

	// update posture check
	postureUpdate1 := &rest_model.PostureCheckMacAddressPatch{
		MacAddresses: []string{strings.ReplaceAll(uuid.NewString(), "-", "")},
	}
	_, err = ctx.RestClients.Edge.PostureChecks.PatchPostureCheck(&posture_checks.PatchPostureCheckParams{
		ID:           postureCheck1Id,
		PostureCheck: postureUpdate1,
	}, nil)
	ctx.NoError(err)

	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(1, len(idEvent.state.PostureChecks))
	testPostureCheck = idEvent.state.PostureChecks[postureCheck1Id]
	ctx.NotNil(testPostureCheck)
	subType, ok = testPostureCheck.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mac_)
	ctx.True(ok)
	ctx.Equal(1, len(subType.Mac.MacAddresses))
	ctx.Equal(postureUpdate1.MacAddresses[0], subType.Mac.MacAddresses[0])

	// Add a second posture check
	postureCheck2 := &rest_model.PostureCheckMacAddressCreate{
		MacAddresses: []string{strings.ReplaceAll(uuid.NewString(), "-", "")},
	}
	postureCheck2.SetName(ToPtr("check2"))
	postureCheck2.SetRoleAttributes(ToPtr(rest_model.Attributes(s(postureCheckRole2))))

	resp, err = ctx.RestClients.Edge.PostureChecks.CreatePostureCheck(&posture_checks.CreatePostureCheckParams{
		PostureCheck: postureCheck2,
	}, nil)
	ctx.NoError(err)
	postureCheck2Id := resp.Payload.Data.ID

	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(2, len(idEvent.state.PostureChecks))
	testPostureCheck = idEvent.state.PostureChecks[postureCheck1Id]
	ctx.NotNil(testPostureCheck)
	subType, ok = testPostureCheck.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mac_)
	ctx.True(ok)
	ctx.Equal(1, len(subType.Mac.MacAddresses))
	ctx.Equal(postureUpdate1.MacAddresses[0], subType.Mac.MacAddresses[0])

	testPostureCheck = idEvent.state.PostureChecks[postureCheck2Id]
	ctx.NotNil(testPostureCheck)
	subType, ok = testPostureCheck.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mac_)
	ctx.True(ok)
	ctx.Equal(1, len(subType.Mac.MacAddresses))
	ctx.Equal(postureCheck2.MacAddresses[0], subType.Mac.MacAddresses[0])

	// remove one of the posture checks from the policy
	sp.postureCheckRoles = s("#" + postureCheckRole2)
	ctx.AdminManagementSession.requireUpdateEntity(sp)

	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(1, len(idEvent.state.PostureChecks))
	testPostureCheck = idEvent.state.PostureChecks[postureCheck2Id]
	ctx.NotNil(testPostureCheck)
	subType, ok = testPostureCheck.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mac_)
	ctx.True(ok)
	ctx.Equal(1, len(subType.Mac.MacAddresses))
	ctx.Equal(postureCheck2.MacAddresses[0], subType.Mac.MacAddresses[0])

	// add it back
	sp.postureCheckRoles = s("#"+postureCheckRole1, "#"+postureCheckRole2)
	ctx.AdminManagementSession.requireUpdateEntity(sp)

	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(2, len(idEvent.state.PostureChecks))

	// delete the service
	ctx.AdminManagementSession.requireDeleteEntity(service1)
	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(0, len(idEvent.state.PostureChecks))

	service1 = ctx.AdminManagementSession.requireNewService(s(serviceRole1), nil)
	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(2, len(idEvent.state.PostureChecks))
	ctx.NotNil(idEvent.state.PostureChecks[postureCheck1Id])
	ctx.NotNil(idEvent.state.PostureChecks[postureCheck2Id])

	// delete a posture check
	_, err = ctx.RestClients.Edge.PostureChecks.DeletePostureCheck(&posture_checks.DeletePostureCheckParams{
		ID: postureCheck1Id,
	}, nil)
	ctx.NoError(err)

	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(1, len(idEvent.state.PostureChecks))
	ctx.NotNil(idEvent.state.PostureChecks[postureCheck2Id])

	// delete the service policy
	ctx.AdminManagementSession.requireDeleteEntity(sp)
	idEvent = sub.getNextIdentityEvent(common.EventPostureChecksUpdated)
	ctx.Equal(0, len(idEvent.state.PostureChecks))
}
