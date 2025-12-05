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
	"encoding/json"
	"sync"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// IdentityConfig represents a configuration assigned to an identity for a specific service.
// It contains the configuration type information and the actual JSON configuration data.
type IdentityConfig struct {
	TypeId   string
	TypeName string
	DataJson string
}

func (self *IdentityConfig) Equals(other *IdentityConfig) bool {
	return self.TypeId == other.TypeId &&
		self.TypeName == other.TypeName &&
		self.DataJson == other.DataJson
}

// IdentityService represents a service from the perspective of a specific identity, including
// the identity's access permissions (dial/bind), associated configurations, and policy indices.
// This is used in subscriptions to track what services an identity can access and how.
type IdentityService struct {
	Service           *Service
	Configs           map[string]*IdentityConfig
	DialAllowed       bool
	BindAllowed       bool
	dialPoliciesIndex uint64
	bindPoliciesIndex uint64
}

func (self *IdentityService) IsDialAllowed() bool {
	return self.DialAllowed
}

func (self *IdentityService) IsBindAllowed() bool {
	return self.BindAllowed
}

func (self *IdentityService) GetId() string {
	return self.Service.Id
}

func (self *IdentityService) GetName() string {
	return self.Service.Name
}

func (self *IdentityService) IsEncryptionRequired() bool {
	return self.Service.EncryptionRequired
}

func (self *IdentityService) GetConfig(configTypeName string, v any) (bool, error) {
	if config, ok := self.Configs[configTypeName]; ok {
		if err := json.Unmarshal([]byte(config.DataJson), &v); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (self *IdentityService) Equals(other *IdentityService) bool {
	log := pfxlog.Logger().WithField("serviceId", other.Service.Id).WithField("serviceName", other.Service.Name)

	if self.Service.GetIndex() != other.Service.GetIndex() {
		if self.Service.Name != other.Service.Name {
			log.WithField("field", "name").Debug("service updated")
			return false
		}

		if self.Service.EncryptionRequired != other.Service.EncryptionRequired {
			log.WithField("field", "encryptionRequired").Debug("service updated")
			return false
		}

		if len(self.Service.Configs) != len(other.Service.Configs) {
			log.WithField("field", "configs.len").Debug("service updated")
			return false
		}

		for idx, v := range self.Service.Configs {
			if other.Service.Configs[idx] != v {
				log.WithField("field", "configs").Debug("service updated")
				return false
			}
		}
	}

	if self.IsDialAllowed() != other.IsDialAllowed() {
		log.WithField("field", "dialAllowed").Debug("service updated")
		return false
	}

	if self.IsBindAllowed() != other.IsBindAllowed() {
		log.WithField("field", "bindAllowed").Debug("service updated")
		return false
	}

	if len(self.Configs) != len(other.Configs) {
		log.WithField("field", "identity.configs.len").Debug("service updated")
		return false
	}

	for id, config := range self.Configs {
		otherConfig, ok := other.Configs[id]
		if !ok {
			log.WithField("field", "identity.configs").Debug("service updated")
			return false
		}
		if !config.Equals(otherConfig) {
			log.WithField("field", "identity.configs").Debug("service updated")
			return false
		}
	}

	return true
}

// IdentitySubscription tracks changes to an identity's state and notifies subscribers when the identity,
// its services, or posture checks are modified. It maintains a snapshot of the identity's current state
// including accessible services and applicable posture checks. The subscription supports multiple listeners
// and can handle special cases like router identities that may be temporarily deleted and recreated.
type IdentitySubscription struct {
	IdentityId string
	Identity   *Identity
	// Some identities, like router identities, may be deleted and recreated, as the tunneler flag is toggled
	// If IsRouterIdentity is true then the subscription should remain active when the identity is deleted
	IsRouterIdentity bool
	Services         map[string]*IdentityService
	Checks           map[string]*PostureCheck

	listeners concurrenz.CopyOnWriteSlice[IdentityEventSubscriber]

	sync.Mutex
}

func (self *IdentitySubscription) Diff(rdm *RouterDataModel, useDenormData bool, sink DiffSink) {
	currentState := &IdentitySubscription{
		IdentityId:       self.IdentityId,
		IsRouterIdentity: self.IsRouterIdentity,
	}
	identity, found := rdm.Identities.Get(currentState.IdentityId)
	if found {
		if useDenormData {
			currentState.initializeWithDenorm(rdm, identity)
		} else {
			currentState.initialize(rdm, identity)
		}
	}

	self.DiffWith(currentState, sink)
}

func (self *IdentitySubscription) DiffWith(other *IdentitySubscription, sink DiffSink) {
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

	cmp.Diff(other, self, syncSetT, cmpopts.IgnoreUnexported(
		sync.Mutex{}, IdentitySubscription{}, IdentityService{},
		Config{}, ConfigType{}, serviceAccess{},
		DataStateConfig{}, DataStateConfigType{}, edge_ctrl_pb.DataState_ServiceConfigs{},
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

func (self *IdentitySubscription) identityRemoved() {
	notify := false
	self.Lock()
	var state *IdentityState

	if self.Identity != nil {
		state = self.getState()

		// we only want the old identity, not the services and posture checks
		state.Services = map[string]*IdentityService{}
		state.PostureChecks = map[string]*PostureCheck{}

		self.Identity = nil
		self.Checks = nil
		self.Services = nil
		notify = true
	}
	self.Unlock()

	if notify {
		for _, subscriber := range self.listeners.Value() {
			subscriber.NotifyIdentityEvent(state, IdentityDeletedEvent)
		}
	}
}

func (self *IdentitySubscription) initialize(rdm *RouterDataModel, identity *Identity) (*IdentityState, bool) {
	self.Lock()
	defer self.Unlock()

	if !identity.serviceAccessTrackingEnabled.Load() {
		rdm.EnableServiceAccessTracking(identity.Id)
	}

	wasInitialized := false
	if self.Identity == nil {
		self.Identity = identity
		if self.Services == nil {
			self.Services, self.Checks = rdm.buildServiceList(self.Identity)
		}
	} else {
		wasInitialized = true
	}
	return self.getState(), wasInitialized
}

func (self *IdentitySubscription) notifyIdentityEvent(state *IdentityState, eventType IdentityEventType) {
	for _, subscriber := range self.listeners.Value() {
		subscriber.NotifyIdentityEvent(state, eventType)
	}
}

func (self *IdentitySubscription) notifyServiceChange(state *IdentityState, service *IdentityService, eventType ServiceEventType) {
	for _, subscriber := range self.listeners.Value() {
		subscriber.NotifyServiceChange(state, service, eventType)
	}
}

func (self *IdentitySubscription) initializeWithDenorm(rdm *RouterDataModel, identity *Identity) (*IdentityState, bool) {
	self.Lock()
	defer self.Unlock()
	wasInitialized := false
	if self.Identity == nil {
		self.Identity = identity
		if self.Services == nil {
			self.Services, self.Checks = rdm.buildServiceListUsingDenormalizedData(self)
		}
	} else {
		wasInitialized = true
	}
	return self.getState(), wasInitialized
}

func (self *IdentitySubscription) checkForChanges(rdm *RouterDataModel) {
	idx := rdm.CurrentIndex()
	log := pfxlog.Logger().
		WithField("index", idx).
		WithField("identity", self.IdentityId)

	self.Lock()
	newIdentity, identityExists := rdm.Identities.Get(self.IdentityId)
	notifyRemoved := newIdentity == nil && self.Identity != nil
	oldIdentity := self.Identity
	oldServices := self.Services
	oldChecks := self.Checks
	self.Identity = newIdentity

	if oldIdentity == nil && newIdentity != nil {
		rdm.EnableServiceAccessTracking(self.IdentityId)
	}

	if identityExists {
		self.Services, self.Checks = rdm.buildServiceListUsingDenormalizedData(self)
	}
	newServices := self.Services
	newChecks := self.Checks
	self.Unlock()
	log.Debugf("identity subscriber updated. identities old: %p new: %p, rdm: %p", oldIdentity, newIdentity, rdm)

	if newIdentity == nil {
		if notifyRemoved {
			state := &IdentityState{
				Identity:      oldIdentity,
				PostureChecks: map[string]*PostureCheck{},
				Services:      map[string]*IdentityService{},
			}
			self.Services = nil
			self.Checks = nil

			self.notifyIdentityEvent(state, IdentityDeletedEvent)
		}
		return
	}

	state := &IdentityState{
		Identity:      newIdentity,
		PostureChecks: newChecks,
		Services:      newServices,
	}

	if oldIdentity == nil {
		self.notifyIdentityEvent(state, IdentityFullStateState)
		return
	}

	if oldIdentity.identityIndex < newIdentity.identityIndex {
		if !oldIdentity.Equals(newIdentity) {
			self.notifyIdentityEvent(state, IdentityUpdatedEvent)
		}
	}

	for svcId, service := range oldServices {
		newService, ok := newServices[svcId]
		if !ok {
			self.notifyServiceChange(state, service, ServiceAccessLostEvent)
		} else if !service.Equals(newService) {
			self.notifyServiceChange(state, newService, ServiceUpdatedEvent)
		}
	}

	for svcId, service := range newServices {
		if _, ok := oldServices[svcId]; !ok {
			self.notifyServiceChange(state, service, ServiceAccessGainedEvent)
		}
	}

	lockNew := oldIdentity != newIdentity
	oldIdentity.lock.Lock()
	if lockNew {
		newIdentity.lock.Lock()
	}

	for svcId, newService := range newServices {
		if oldService := oldServices[svcId]; oldService != nil {
			if newService.DialAllowed && oldService.dialPoliciesIndex != newService.dialPoliciesIndex {
				self.notifyServiceChange(state, newService, ServiceDialPoliciesChanged)
			}
			if newService.BindAllowed && oldService.bindPoliciesIndex != newService.bindPoliciesIndex {
				self.notifyServiceChange(state, newService, ServiceBindPoliciesChanged)
			}
		}
	}

	if lockNew {
		newIdentity.lock.Unlock()
	}
	oldIdentity.lock.Unlock()

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
		self.notifyIdentityEvent(state, IdentityPostureChecksUpdatedEvent)
	}
}

// IdentityEventType represents the type of change that occurred to an identity. It is used
// to classify notifications sent to identity event subscribers.
type IdentityEventType byte

func (self IdentityEventType) String() string {
	switch self {
	case IdentityFullStateState:
		return "identity.full-state"
	case IdentityUpdatedEvent:
		return "identity.updated"
	case IdentityPostureChecksUpdatedEvent:
		return "identity.posture-checks-updated"
	case IdentityDeletedEvent:
		return "identity.deleted"
	default:
		return "unknown"
	}
}

// ServiceEventType represents the type of change that occurred to a service's accessibility
// or configuration for an identity. It is used to classify service change notifications.
type ServiceEventType byte

func (self ServiceEventType) String() string {
	switch self {
	case ServiceAccessGainedEvent:
		return "access.gained"
	case ServiceUpdatedEvent:
		return "updated"
	case ServiceAccessLostEvent:
		return "access.removed"
	case ServiceDialPoliciesChanged:
		return "dial.policies-changed"
	case ServiceBindPoliciesChanged:
		return "Bind.policies-changed"
	default:
		return "unknown"
	}
}

const (
	ServiceAccessGainedEvent ServiceEventType = 1
	ServiceUpdatedEvent      ServiceEventType = 2
	ServiceAccessLostEvent   ServiceEventType = 3

	ServiceDialPoliciesChanged ServiceEventType = 4
	ServiceBindPoliciesChanged ServiceEventType = 5

	IdentityFullStateState            IdentityEventType = 6
	IdentityUpdatedEvent              IdentityEventType = 7
	IdentityPostureChecksUpdatedEvent IdentityEventType = 8
	IdentityDeletedEvent              IdentityEventType = 9
)

// IdentityState represents a snapshot of an identity's current state including the identity itself,
// the posture checks that apply to it, and the services it has access to. This is passed to
// subscribers when notifying them of identity changes.
type IdentityState struct {
	Identity      *Identity
	PostureChecks map[string]*PostureCheck
	Services      map[string]*IdentityService
}

// IdentityEventSubscriber is the interface that must be implemented to receive notifications
// about changes to an identity's state or service access. Subscribers are notified when
// the identity is created, updated, or deleted, and when services are added, removed, or modified.
type IdentityEventSubscriber interface {
	NotifyIdentityEvent(state *IdentityState, eventType IdentityEventType)
	NotifyServiceChange(state *IdentityState, service *IdentityService, eventType ServiceEventType)
}

// subscriberEvent is an internal interface for events that need to be processed to update
// identity subscriptions. These events are queued and processed asynchronously.
type subscriberEvent interface {
	process(rdm *RouterDataModel)
}

// identityDeletedEvent is queued when an identity is deleted to notify subscribers.
type identityDeletedEvent struct {
	identityId string
}

func (self identityDeletedEvent) process(rdm *RouterDataModel) {
	rdm.checkSubsForDeletedIdentity(self.identityId, true)
}

// identityCreatedEvent is queued when a new identity is created to check for relevant subscriptions.
type identityCreatedEvent struct {
	identityId string
}

func (self identityCreatedEvent) process(rdm *RouterDataModel) {
	rdm.checkSubsForNewIdentity(self.identityId, true)
}

// checkForIdentityChangesEvent is queued when an identity is updated to sync with subscribers.
type checkForIdentityChangesEvent struct {
	identityId string
}

func (self checkForIdentityChangesEvent) process(rdm *RouterDataModel) {
	rdm.syncSubscriptionIfRequired(self.identityId, true)
}

// syncAllSubscribersEvent is queued to trigger a full sync of all active subscriptions.
type syncAllSubscribersEvent struct{}

func (self syncAllSubscribersEvent) process(rdm *RouterDataModel) {
	pfxlog.Logger().WithField("subs", rdm.subscriptions.Count()).
		WithField("updatedIdentities", rdm.updatedIdentities.Count()).
		Debug("sync all subscribers: start")
	rdm.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
		rdm.markIdentityCheckComplete(key, IdentityUpdated)
		v.checkForChanges(rdm)
	})
	pfxlog.Logger().WithField("subs", rdm.subscriptions.Count()).Debug("sync all subscribers: done")
}
