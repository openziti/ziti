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
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"sync"
)

type IdentityConfig struct {
	Config     *Config
	ConfigType *ConfigType
}

func (self *IdentityConfig) Equals(other *IdentityConfig) bool {
	return self.Config.Equals(other.Config) && self.ConfigType.Equals(other.ConfigType)
}

type IdentityService struct {
	Service     *Service
	Checks      map[string]struct{}
	Configs     map[string]*IdentityConfig
	DialAllowed bool
	BindAllowed bool
}

func (self *IdentityService) GetId() string {
	return self.Service.GetId()
}

func (self *IdentityService) GetName() string {
	return self.Service.GetName()
}

func (self *IdentityService) IsEncryptionRequired() bool {
	return self.Service.EncryptionRequired
}

func (self *IdentityService) Equals(other *IdentityService) bool {
	log := pfxlog.Logger().WithField("serviceId", other.Service.Id).WithField("serviceName", other.Service.Name)
	if self.Service.Name != other.Service.Name {
		log.WithField("field", "name").Info("service updated")
		return false
	}

	if self.Service.EncryptionRequired != other.Service.EncryptionRequired {
		log.WithField("field", "encryptionRequired").Info("service updated")
		return false
	}

	if len(self.Service.Configs) != len(other.Service.Configs) {
		log.WithField("field", "configs.len").Info("service updated")
		return false
	}

	if len(self.Checks) != len(other.Checks) {
		log.WithField("field", "checks.len").Info("service updated")
		return false
	}

	if len(self.Configs) != len(other.Configs) {
		log.WithField("field", "identity.configs.len").Info("service updated")
		return false
	}

	if self.DialAllowed != other.DialAllowed {
		log.WithField("field", "dialAllowed").Info("service updated")
		return false
	}

	if self.BindAllowed != other.BindAllowed {
		log.WithField("field", "bindAllowed").Info("service updated")
		return false
	}

	for id := range self.Checks {
		if _, ok := other.Checks[id]; !ok {
			log.WithField("field", "checks").Info("service updated")
			return false
		}
	}

	for id, config := range self.Configs {
		otherConfig, ok := other.Configs[id]
		if !ok {
			log.WithField("field", "identity.configs").Info("service updated")
			return false
		}
		if !config.Equals(otherConfig) {
			log.WithField("field", "identity.configs").Info("service updated")
			return false
		}
	}

	for idx, v := range self.Service.Configs {
		if other.Service.Configs[idx] != v {
			log.WithField("field", "configs").Info("service updated")
			return false
		}
	}

	return true
}

type IdentitySubscription struct {
	IdentityId string
	Identity   *Identity
	Services   map[string]*IdentityService
	Checks     map[string]*PostureCheck

	listeners concurrenz.CopyOnWriteSlice[IdentityEventSubscriber]

	sync.Mutex
}

func (self *IdentitySubscription) Diff(rdm *RouterDataModel, sink DiffSink) {
	currentState := &IdentitySubscription{IdentityId: self.IdentityId}
	identity, found := rdm.Identities.Get(currentState.IdentityId)
	if found {
		currentState.initialize(rdm, identity)
	}

	diffReporter := &compareReporter{
		key: self.IdentityId,
		f: func(key string, detail string) {
			sink("subscriber", key, DiffTypeMod, detail)
		},
	}

	adapter := cmp.Reporter(diffReporter)
	syncSetT := cmp.Transformer("syncSetToMap", func(s cmap.ConcurrentMap[string, struct{}]) map[string]struct{} {
		return CMapToMap(s)
	})
	cmp.Diff(currentState, self, syncSetT, cmpopts.IgnoreUnexported(
		sync.Mutex{}, IdentitySubscription{}, IdentityService{},
		Config{}, ConfigType{},
		DataStateConfig{}, DataStateConfigType{},
		Identity{}, DataStateIdentity{},
		Service{}, DataStateService{},
		ServicePolicy{}, DataStateServicePolicy{},
		PostureCheck{}, DataStatePostureCheck{}, edge_ctrl_pb.DataState_PostureCheck_Domains_{}, edge_ctrl_pb.DataState_PostureCheck_Domains{},
		edge_ctrl_pb.DataState_PostureCheck_Mac_{}, edge_ctrl_pb.DataState_PostureCheck_Mac{},
		edge_ctrl_pb.DataState_PostureCheck_Mfa_{}, edge_ctrl_pb.DataState_PostureCheck_Mfa{},
		edge_ctrl_pb.DataState_PostureCheck_OsList_{}, edge_ctrl_pb.DataState_PostureCheck_OsList{}, edge_ctrl_pb.DataState_PostureCheck_Os{},
		edge_ctrl_pb.DataState_PostureCheck_Process_{}, edge_ctrl_pb.DataState_PostureCheck_Process{},
		edge_ctrl_pb.DataState_PostureCheck_ProcessMulti_{}, edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{},
	), adapter)
}

func (self *IdentitySubscription) getState() *IdentityState {
	return &IdentityState{
		Identity:      self.Identity,
		PostureChecks: self.Checks,
		Services:      self.Services,
	}
}

func (self *IdentitySubscription) identityUpdated(identity *Identity) {
	notify := false
	present := false
	var state *IdentityState
	self.Lock()
	if self.Identity != nil {
		if identity.identityIndex > self.Identity.identityIndex {
			if !identity.Equals(self.Identity) {
				notify = true
			}
			self.Identity = identity
		}
		present = true
		state = self.getState()
	}
	self.Unlock()

	if !present {
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventFullState)
		}
	} else if notify {
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventIdentityUpdated)
		}
	}
}

func (self *IdentitySubscription) identityRemoved() {
	notify := false
	self.Lock()
	var state *IdentityState
	if self.Identity != nil {
		state = self.getState()
		self.Identity = nil
		self.Checks = nil
		self.Services = nil
		notify = true
	}
	self.Unlock()

	if notify {
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventIdentityDeleted)
		}
	}
}

func (self *IdentitySubscription) initialize(rdm *RouterDataModel, identity *Identity) (*IdentityState, bool) {
	self.Lock()
	defer self.Unlock()
	wasInitialized := false
	if self.Identity == nil {
		self.Identity = identity
		if self.Services == nil {
			self.Services, self.Checks = rdm.buildServiceList(self)
		}
	} else {
		wasInitialized = true
	}
	return self.getState(), wasInitialized
}

func (self *IdentitySubscription) checkForChanges(rdm *RouterDataModel) {
	idx, _ := rdm.CurrentIndex()
	log := pfxlog.Logger().
		WithField("index", idx).
		WithField("identity", self.IdentityId)

	self.Lock()
	newIdentity, ok := rdm.Identities.Get(self.IdentityId)
	notifyRemoved := !ok && self.Identity != nil
	oldIdentity := self.Identity
	oldServices := self.Services
	oldChecks := self.Checks
	self.Identity = newIdentity
	if ok {
		self.Services, self.Checks = rdm.buildServiceList(self)
	}
	newServices := self.Services
	newChecks := self.Checks
	self.Unlock()
	log.Debugf("identity subscriber updated. identities old: %p new: %p, rdm: %p", oldIdentity, newIdentity, rdm)

	if notifyRemoved {
		state := &IdentityState{
			Identity:      oldIdentity,
			PostureChecks: oldChecks,
			Services:      oldServices,
		}
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventIdentityDeleted)
		}
		return
	}

	if !ok {
		return
	}

	state := &IdentityState{
		Identity:      newIdentity,
		PostureChecks: newChecks,
		Services:      newServices,
	}

	if oldIdentity == nil {
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventFullState)
		}
		return
	}

	if oldIdentity.identityIndex < newIdentity.identityIndex {
		if !oldIdentity.Equals(newIdentity) {
			for _, subscriber := range self.listeners.Value() {
				subscriber.NotifyIdentityEvent(state, EventIdentityUpdated)
			}
		}
	}

	for svcId, service := range oldServices {
		newService, ok := newServices[svcId]
		if !ok {
			for _, subscriber := range self.listeners.Value() {
				subscriber.NotifyServiceChange(state, service, EventAccessRemoved)
			}
		} else if !service.Equals(newService) {
			for _, subscriber := range self.listeners.Value() {
				subscriber.NotifyServiceChange(state, newService, EventUpdated)
			}
		}
	}

	for svcId, service := range newServices {
		if _, ok := oldServices[svcId]; !ok {
			for _, subscriber := range self.listeners.Value() {
				subscriber.NotifyServiceChange(state, service, EventAccessGained)
			}
		}
	}

	checksChanged := false
	if len(oldChecks) != len(newChecks) {
		checksChanged = true
	} else {
		for checkId, check := range oldChecks {
			newCheck, ok := newChecks[checkId]
			if !ok {
				checksChanged = true
				break
			}
			if check.index != newCheck.index {
				checksChanged = true
				break
			}
		}
	}

	if checksChanged {
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventPostureChecksUpdated)
		}
	}
}

type IdentityEventType byte

func (self IdentityEventType) String() string {
	switch self {
	case EventFullState:
		return "identity.full-state"
	case EventIdentityUpdated:
		return "identity.updated"
	case EventPostureChecksUpdated:
		return "identity.posture-checks-updated"
	case EventIdentityDeleted:
		return "identity.deleted"
	default:
		return "unknown"
	}
}

type ServiceEventType byte

func (self ServiceEventType) String() string {
	switch self {
	case EventAccessGained:
		return "access.gained"
	case EventUpdated:
		return "updated"
	case EventAccessRemoved:
		return "access.removed"
	default:
		return "unknown"
	}
}

const (
	EventAccessGained  ServiceEventType = 1
	EventUpdated       ServiceEventType = 2
	EventAccessRemoved ServiceEventType = 3

	EventFullState            IdentityEventType = 4
	EventIdentityUpdated      IdentityEventType = 5
	EventPostureChecksUpdated IdentityEventType = 6
	EventIdentityDeleted      IdentityEventType = 7
)

type IdentityState struct {
	Identity      *Identity
	PostureChecks map[string]*PostureCheck
	Services      map[string]*IdentityService
}

type IdentityEventSubscriber interface {
	NotifyIdentityEvent(state *IdentityState, eventType IdentityEventType)
	NotifyServiceChange(state *IdentityState, service *IdentityService, eventType ServiceEventType)
}

type subscriberEvent interface {
	process(rdm *RouterDataModel)
}

type identityRemoveEvent struct {
	identityId string
}

func (self identityRemoveEvent) process(rdm *RouterDataModel) {
	if sub, found := rdm.subscriptions.Get(self.identityId); found {
		sub.identityRemoved()
	}
}

type identityCreatedEvent struct {
	identity *Identity
}

func (self identityCreatedEvent) process(rdm *RouterDataModel) {
	pfxlog.Logger().
		WithField("subs", rdm.subscriptions.Count()).
		WithField("identityId", self.identity.Id).
		Debug("handling identity created event")

	if sub, found := rdm.subscriptions.Get(self.identity.Id); found {
		state, wasInitialized := sub.initialize(rdm, self.identity)
		if wasInitialized {
			sub.checkForChanges(rdm)
		} else {
			for _, subscriber := range sub.listeners.Value() {
				subscriber.NotifyIdentityEvent(state, EventFullState)
			}
		}
	}
}

type identityUpdatedEvent struct {
	identity *Identity
}

func (self identityUpdatedEvent) process(rdm *RouterDataModel) {
	if sub, found := rdm.subscriptions.Get(self.identity.Id); found {
		sub.identityUpdated(self.identity)
	}
}

type syncAllSubscribersEvent struct{}

func (self syncAllSubscribersEvent) process(rdm *RouterDataModel) {
	pfxlog.Logger().WithField("subs", rdm.subscriptions.Count()).Info("sync all subscribers")
	rdm.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
		v.checkForChanges(rdm)
	})
}
