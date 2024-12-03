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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"sync"
)

type IdentityConfig struct {
	Config     *Config
	ConfigType *ConfigType
}

type IdentityService struct {
	Service     *Service
	Checks      map[string]struct{}
	Configs     map[string]*IdentityConfig
	DialAllowed bool
	BindAllowed bool
}

func (self *IdentityService) Equals(other *IdentityService) bool {
	if self.Service.index != other.Service.index {
		return false
	}

	if len(self.Checks) != len(other.Checks) {
		return false
	}

	if len(self.Configs) != len(other.Configs) {
		return false
	}

	if self.DialAllowed != other.DialAllowed {
		return false
	}

	if self.BindAllowed != other.BindAllowed {
		return false
	}

	for id := range self.Checks {
		if _, ok := other.Checks[id]; !ok {
			return false
		}
	}

	for id, config := range self.Configs {
		otherConfig, ok := other.Configs[id]
		if !ok {
			return false
		}
		if config.Config.index != otherConfig.Config.index {
			return false
		}
		if config.ConfigType.index != otherConfig.ConfigType.index {
			return false
		}
	}

	return true
}

type IdentitySubscription struct {
	IdentityId string
	Identity   *Identity
	Services   map[string]*IdentityService
	Listeners  concurrenz.CopyOnWriteSlice[IdentityEventSubscriber]
	Checks     map[string]*PostureCheck

	sync.Mutex
}

func (self *IdentitySubscription) getState() *IdentityState {
	return &IdentityState{
		Identity:      self.Identity,
		PostureChecks: self.Checks,
		Services:      self.Services,
	}
}

func (self *IdentitySubscription) identityUpdated(rdm *RouterDataModel, identity *Identity) {
	notify := false
	present := false
	var state *IdentityState
	self.Lock()
	if self.Identity != nil {
		if identity.identityIndex > self.Identity.identityIndex {
			self.Identity = identity
			notify = true
		}
		present = true
		state = self.getState()
	}
	self.Unlock()

	if !present {
		for _, subscriber := range self.Listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventFullState)
		}
	} else if notify {
		for _, subscriber := range self.Listeners.Value() {
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
		for _, subscriber := range self.Listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventIdentityDeleted)
		}
	}
}

func (self *IdentitySubscription) initialize(rdm *RouterDataModel, identity *Identity) *IdentityState {
	self.Lock()
	defer self.Unlock()
	if self.Identity == nil {
		self.Identity = identity
		if self.Services == nil {
			self.Services, self.Checks = rdm.buildServiceList(self)
		}
	}
	return self.getState()
}

func (self *IdentitySubscription) checkForChanges(rdm *RouterDataModel) {
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

	if notifyRemoved {
		state := &IdentityState{
			Identity:      oldIdentity,
			PostureChecks: oldChecks,
			Services:      oldServices,
		}
		for _, subscriber := range self.Listeners.Value() {
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
		for _, subscriber := range self.Listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventFullState)
		}
		return
	}

	if oldIdentity.identityIndex < newIdentity.identityIndex {
		for _, subscriber := range self.Listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventIdentityUpdated)
		}
	}

	for svcId, service := range oldServices {
		newService, ok := newServices[svcId]
		if !ok {
			for _, subscriber := range self.Listeners.Value() {
				subscriber.NotifyServiceChange(state, service, EventAccessRemoved)
			}
		} else if !service.Equals(newService) {
			for _, subscriber := range self.Listeners.Value() {
				subscriber.NotifyServiceChange(state, newService, EventUpdated)
			}
		}
	}

	for svcId, service := range newServices {
		if _, ok := oldServices[svcId]; !ok {
			for _, subscriber := range self.Listeners.Value() {
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
		for _, subscriber := range self.Listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventPostureChecksUpdated)
		}
	}
}

type IdentityEventType byte

type ServiceEventType byte

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
		state := sub.initialize(rdm, self.identity)
		for _, subscriber := range sub.Listeners.Value() {
			subscriber.NotifyIdentityEvent(state, EventFullState)
		}
	}
}

type identityUpdatedEvent struct {
	identity *Identity
}

func (self identityUpdatedEvent) process(rdm *RouterDataModel) {
	if sub, found := rdm.subscriptions.Get(self.identity.Id); found {
		sub.identityUpdated(rdm, self.identity)
	}
}

type syncAllSubscribersEvent struct{}

func (self syncAllSubscribersEvent) process(rdm *RouterDataModel) {
	pfxlog.Logger().WithField("subs", rdm.subscriptions.Count()).Info("sync all subscribers")
	rdm.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
		v.checkForChanges(rdm)
	})
}
