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
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"sort"
	"sync/atomic"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// RouterDataModelConfig contains the configuration values for a RouterDataModel
type RouterDataModelConfig struct {
	Enabled            bool
	LogSize            uint64
	ListenerBufferSize uint
}

// AccessPolicies represents the Identity's access to a Service through many Policies. The PostureChecks provided
// are referenced by the granting Policies. The PostureChecks for each of the Policies may be evaluated to determine
// a valid policy and posture access path.
type AccessPolicies struct {
	Identity      *Identity
	Service       *Service
	Policies      []*ServicePolicy
	PostureChecks map[string]*edge_ctrl_pb.DataState_PostureCheck
}

type DataStateIdentity = edge_ctrl_pb.DataState_Identity

type Identity struct {
	*DataStateIdentity
	ServicePolicies cmap.ConcurrentMap[string, struct{}] `json:"servicePolicies"`
	identityIndex   uint64
	serviceSetIndex uint64
}

func (self *Identity) Equals(other *Identity) bool {
	log := pfxlog.Logger().WithField("identity", self.identityIndex)
	if self.Disabled != other.Disabled {
		log.Info("identity updated, disabled flag changed")
		return false
	}

	if self.Name != other.Name {
		log.Info("identity updated, name changed")
		return false
	}

	if string(self.AppDataJson) != string(other.AppDataJson) {
		log.Info("identity updated, appDataJson changed")
		return false
	}

	if self.DefaultHostingPrecedence != other.DefaultHostingPrecedence {
		log.Info("identity updated, default hosting precedence changed")
		return false
	}

	if self.DefaultHostingCost != other.DefaultHostingCost {
		log.Info("identity updated, default hosting host changed")
		return false
	}

	if len(self.ServiceHostingPrecedences) != len(other.ServiceHostingPrecedences) {
		log.Info("identity updated, number of service hosting precedences changed")
		return false
	}

	if len(self.ServiceHostingCosts) != len(other.ServiceHostingCosts) {
		log.Info("identity updated, number of service hosting costs changed")
		return false
	}

	for k, v := range self.ServiceHostingPrecedences {
		v2, ok := other.ServiceHostingPrecedences[k]
		if !ok || v != v2 {
			log.Info("identity updated, a service hosting precedence changed")
			return false
		}
	}

	for k, v := range self.ServiceHostingCosts {
		v2, ok := other.ServiceHostingCosts[k]
		if !ok || v != v2 {
			log.Info("identity updated, a service hosting cost changed")
			return false
		}
	}

	return true
}

type DataStateConfigType = edge_ctrl_pb.DataState_ConfigType

type ConfigType struct {
	*DataStateConfigType
	index uint64
}

func (self *ConfigType) Equals(other *ConfigType) bool {
	return self.Name == other.Name
}

type DataStateConfig = edge_ctrl_pb.DataState_Config

type Config struct {
	*DataStateConfig
	index uint64
}

func (self *Config) Equals(other *Config) bool {
	if self.Name != other.Name {
		return false
	}

	if self.TypeId != other.TypeId {
		return false
	}

	if self.DataJson != other.DataJson {
		return false
	}

	return true
}

type DataStateService = edge_ctrl_pb.DataState_Service

type Service struct {
	*DataStateService
	index uint64
}

type DataStatePostureCheck = edge_ctrl_pb.DataState_PostureCheck

type PostureCheck struct {
	*DataStatePostureCheck
	index uint64
}

type DataStateServicePolicy = edge_ctrl_pb.DataState_ServicePolicy

type ServicePolicy struct {
	*DataStateServicePolicy
	Services      cmap.ConcurrentMap[string, struct{}] `json:"services"`
	PostureChecks cmap.ConcurrentMap[string, struct{}] `json:"postureChecks"`
}

// RouterDataModel represents a sub-set of a controller's data model. Enough to validate an identities access to dial/bind
// a service through policies and posture checks. RouterDataModel can operate in two modes: sender (controller) and
// receiver (router). Sender mode allows a controller support an event cache that supports replays for routers connecting
// for the first time/after disconnects. Receive mode does not maintain an event cache and does not support replays.
// It instead is used as a reference data structure for authorization computations.
type RouterDataModel struct {
	EventCache
	listeners map[chan *edge_ctrl_pb.DataState_ChangeSet]struct{}

	ConfigTypes      cmap.ConcurrentMap[string, *ConfigType]                        `json:"configTypes"`
	Configs          cmap.ConcurrentMap[string, *Config]                            `json:"configs"`
	Identities       cmap.ConcurrentMap[string, *Identity]                          `json:"identities"`
	Services         cmap.ConcurrentMap[string, *Service]                           `json:"services"`
	ServicePolicies  cmap.ConcurrentMap[string, *ServicePolicy]                     `json:"servicePolicies"`
	PostureChecks    cmap.ConcurrentMap[string, *PostureCheck]                      `json:"postureChecks"`
	PublicKeys       cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PublicKey]  `json:"publicKeys"`
	Revocations      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Revocation] `json:"revocations"`
	cachedPublicKeys concurrenz.AtomicValue[map[string]crypto.PublicKey]

	terminatorIdCache cmap.ConcurrentMap[string, string]

	listenerBufferSize uint
	lastSaveIndex      *uint64

	subscriptions cmap.ConcurrentMap[string, *IdentitySubscription]
	events        chan subscriberEvent
	closeNotify   <-chan struct{}
	stopNotify    chan struct{}
	stopped       atomic.Bool

	// timelineId identifies the database that events are flowing from. This will be reset whenever we change the
	// underlying datastore
	timelineId string
}

// NewBareRouterDataModel creates a new RouterDataModel that is expected to have no buffers, listeners or subscriptions
func NewBareRouterDataModel() *RouterDataModel {
	return &RouterDataModel{
		EventCache:        NewForgetfulEventCache(),
		ConfigTypes:       cmap.New[*ConfigType](),
		Configs:           cmap.New[*Config](),
		Identities:        cmap.New[*Identity](),
		Services:          cmap.New[*Service](),
		ServicePolicies:   cmap.New[*ServicePolicy](),
		PostureChecks:     cmap.New[*PostureCheck](),
		PublicKeys:        cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:       cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		terminatorIdCache: cmap.New[string](),
	}
}

// NewSenderRouterDataModel creates a new RouterDataModel that will store events in a circular buffer of
// logSize. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewSenderRouterDataModel(timelineId string, logSize uint64, listenerBufferSize uint) *RouterDataModel {
	return &RouterDataModel{
		EventCache:         NewLoggingEventCache(logSize),
		ConfigTypes:        cmap.New[*ConfigType](),
		Configs:            cmap.New[*Config](),
		Identities:         cmap.New[*Identity](),
		Services:           cmap.New[*Service](),
		ServicePolicies:    cmap.New[*ServicePolicy](),
		PostureChecks:      cmap.New[*PostureCheck](),
		PublicKeys:         cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:        cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		listenerBufferSize: listenerBufferSize,
		timelineId:         timelineId,
		terminatorIdCache:  cmap.New[string](),
	}
}

// NewReceiverRouterDataModel creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModel(listenerBufferSize uint, closeNotify <-chan struct{}) *RouterDataModel {
	result := &RouterDataModel{
		EventCache:         NewForgetfulEventCache(),
		ConfigTypes:        cmap.New[*ConfigType](),
		Configs:            cmap.New[*Config](),
		Identities:         cmap.New[*Identity](),
		Services:           cmap.New[*Service](),
		ServicePolicies:    cmap.New[*ServicePolicy](),
		PostureChecks:      cmap.New[*PostureCheck](),
		PublicKeys:         cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:        cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		listenerBufferSize: listenerBufferSize,
		subscriptions:      cmap.New[*IdentitySubscription](),
		events:             make(chan subscriberEvent),
		closeNotify:        closeNotify,
		stopNotify:         make(chan struct{}),
		terminatorIdCache:  cmap.New[string](),
	}
	go result.processSubscriberEvents()
	return result
}

// NewReceiverRouterDataModelFromDataState creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromDataState(dataState *edge_ctrl_pb.DataState, listenerBufferSize uint, closeNotify <-chan struct{}) *RouterDataModel {
	result := &RouterDataModel{
		EventCache:         NewForgetfulEventCache(),
		ConfigTypes:        cmap.New[*ConfigType](),
		Configs:            cmap.New[*Config](),
		Identities:         cmap.New[*Identity](),
		Services:           cmap.New[*Service](),
		ServicePolicies:    cmap.New[*ServicePolicy](),
		PostureChecks:      cmap.New[*PostureCheck](),
		PublicKeys:         cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:        cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		listenerBufferSize: listenerBufferSize,
		subscriptions:      cmap.New[*IdentitySubscription](),
		events:             make(chan subscriberEvent),
		closeNotify:        closeNotify,
		stopNotify:         make(chan struct{}),
		timelineId:         dataState.TimelineId,
		terminatorIdCache:  cmap.New[string](),
	}

	if tIdCache, ok := dataState.Caches[edge_ctrl_pb.CacheType_TerminatorIds.String()]; ok && tIdCache != nil && tIdCache.Data != nil {
		for k, v := range tIdCache.Data {
			result.terminatorIdCache.Set(k, string(v))
		}
	}

	go result.processSubscriberEvents()

	result.WhileLocked(func(u uint64, b bool) {
		for _, event := range dataState.Events {
			result.Handle(dataState.EndIndex, event)
		}
		result.SetCurrentIndex(dataState.EndIndex)
	})

	return result
}

// NewReceiverRouterDataModelFromExisting creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromExisting(existing *RouterDataModel, listenerBufferSize uint, closeNotify <-chan struct{}) *RouterDataModel {
	result := &RouterDataModel{
		EventCache:         NewForgetfulEventCache(),
		ConfigTypes:        existing.ConfigTypes,
		Configs:            existing.Configs,
		Identities:         existing.Identities,
		Services:           existing.Services,
		ServicePolicies:    existing.ServicePolicies,
		PostureChecks:      existing.PostureChecks,
		PublicKeys:         existing.PublicKeys,
		cachedPublicKeys:   existing.cachedPublicKeys,
		Revocations:        existing.Revocations,
		listenerBufferSize: listenerBufferSize,
		subscriptions:      cmap.New[*IdentitySubscription](),
		events:             make(chan subscriberEvent),
		closeNotify:        closeNotify,
		stopNotify:         make(chan struct{}),
		timelineId:         existing.timelineId,
		terminatorIdCache:  existing.terminatorIdCache,
	}
	currentIndex, _ := existing.CurrentIndex()
	result.SetCurrentIndex(currentIndex)
	go result.processSubscriberEvents()
	return result
}

// NewReceiverRouterDataModelFromFile creates a new RouterDataModel that does not store events and is initialized from
// a file backup. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromFile(path string, listenerBufferSize uint, closeNotify <-chan struct{}) (*RouterDataModel, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()

	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}

	state := &edge_ctrl_pb.DataState{}
	if err = proto.Unmarshal(data, state); err != nil {
		return nil, err
	}

	rdm := NewReceiverRouterDataModelFromDataState(state, listenerBufferSize, closeNotify)
	rdm.lastSaveIndex = &state.EndIndex

	return rdm, nil
}

func (rdm *RouterDataModel) processSubscriberEvents() {
	for !rdm.stopped.Load() {
		select {
		case <-rdm.closeNotify:
			return
		case <-rdm.stopNotify:
			return
		case evt := <-rdm.events:
			evt.process(rdm)
		}
	}
}

func (rdm *RouterDataModel) Stop() {
	if rdm.stopped.CompareAndSwap(false, true) {
		close(rdm.stopNotify)
	}
}

// NewListener returns a channel that will receive the events applied to this data model.
func (rdm *RouterDataModel) NewListener() <-chan *edge_ctrl_pb.DataState_ChangeSet {
	if rdm.listeners == nil {
		rdm.listeners = map[chan *edge_ctrl_pb.DataState_ChangeSet]struct{}{}
	}

	newCh := make(chan *edge_ctrl_pb.DataState_ChangeSet, rdm.listenerBufferSize)
	rdm.listeners[newCh] = struct{}{}

	return newCh
}

func (rdm *RouterDataModel) sendEvent(event *edge_ctrl_pb.DataState_ChangeSet) {
	for listener := range rdm.listeners {
		listener <- event
	}
}

func (rdm *RouterDataModel) GetTimelineId() string {
	return rdm.timelineId
}

// ApplyChangeSet applies the given even to the router data model.
func (rdm *RouterDataModel) ApplyChangeSet(change *edge_ctrl_pb.DataState_ChangeSet) {
	changeAccepted := false
	logger := pfxlog.Logger().
		WithField("index", change.Index).
		WithField("synthetic", change.IsSynthetic).
		WithField("entries", len(change.Changes))

	err := rdm.EventCache.Store(change, func(index uint64, change *edge_ctrl_pb.DataState_ChangeSet) {
		syncSubscribers := false
		for idx, event := range change.Changes {
			logger.
				WithField("entry", idx).
				WithField("action", event.Action.String()).
				WithField("type", fmt.Sprintf("%T", event.Model)).
				WithField("summary", event.Summarize()).
				Debug("handling change set entry")
			if rdm.Handle(index, event) {
				syncSubscribers = true
			}
		}
		if syncSubscribers {
			rdm.SyncAllSubscribers()
		}
		changeAccepted = true
	})

	if err != nil {
		if len(change.Changes) > 0 {
			logger = logger.WithField("action", change.Changes[0].Action.String()).
				WithField("type", fmt.Sprintf("%T", change.Changes[0].Model))
		}

		logger.WithError(err).Error("could not apply change set")
		return
	}

	if changeAccepted {
		rdm.sendEvent(change)
	}
}

func (rdm *RouterDataModel) Handle(index uint64, event *edge_ctrl_pb.DataState_Event) bool {
	switch typedModel := event.Model.(type) {
	case *edge_ctrl_pb.DataState_Event_ConfigType:
		rdm.HandleConfigTypeEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Config:
		rdm.HandleConfigEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Identity:
		rdm.HandleIdentityEvent(index, event, typedModel)
		return false // identity events are handled individually, don't require a full subscriber sync
	case *edge_ctrl_pb.DataState_Event_Service:
		rdm.HandleServiceEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_ServicePolicy:
		rdm.HandleServicePolicyEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PostureCheck:
		rdm.HandlePostureCheckEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PublicKey:
		rdm.HandlePublicKeyEvent(event, typedModel)
		return false // don't affect identity subscribers, so don't require a sync
	case *edge_ctrl_pb.DataState_Event_Revocation:
		rdm.HandleRevocationEvent(event, typedModel)
		return false // don't affect identity subscribers, so don't require a sync
	case *edge_ctrl_pb.DataState_Event_ServicePolicyChange:
		rdm.HandleServicePolicyChange(index, typedModel.ServicePolicyChange)
	}
	return true
}

func (rdm *RouterDataModel) queueEvent(event subscriberEvent) {
	if rdm.events != nil {
		rdm.events <- event
	}
}

func (rdm *RouterDataModel) SyncAllSubscribers() {
	if rdm.events != nil {
		rdm.events <- syncAllSubscribersEvent{}
	}
}

// HandleIdentityEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyIdentityEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleIdentityEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Identities.Remove(model.Identity.Id)
		rdm.queueEvent(identityRemoveEvent{identityId: model.Identity.Id})
	} else {
		var identity *Identity
		rdm.Identities.Upsert(model.Identity.Id, nil, func(exist bool, valueInMap *Identity, newValue *Identity) *Identity {
			if valueInMap == nil {
				identity = &Identity{
					DataStateIdentity: model.Identity,
					ServicePolicies:   cmap.New[struct{}](),
					identityIndex:     index,
				}
			} else {
				identity = &Identity{
					DataStateIdentity: model.Identity,
					ServicePolicies:   valueInMap.ServicePolicies,
					identityIndex:     index,
					serviceSetIndex:   valueInMap.serviceSetIndex,
				}
			}
			return identity
		})

		if event.Action == edge_ctrl_pb.DataState_Create {
			rdm.queueEvent(identityCreatedEvent{identity: identity})
		} else if event.Action == edge_ctrl_pb.DataState_Update {
			rdm.queueEvent(identityUpdatedEvent{identity: identity})
		}
	}
}

// HandleServiceEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServiceEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServiceEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Service) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Services.Remove(model.Service.Id)
		rdm.ServicePolicies.IterCb(func(key string, v *ServicePolicy) {
			v.Services.Remove(model.Service.Id)
		})
	} else {
		rdm.Services.Set(model.Service.Id, &Service{
			DataStateService: model.Service,
			index:            index,
		})
	}
}

// HandleConfigTypeEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyConfigTypeEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleConfigTypeEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ConfigType) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.ConfigTypes.Remove(model.ConfigType.Id)
	} else {
		rdm.ConfigTypes.Set(model.ConfigType.Id, &ConfigType{
			DataStateConfigType: model.ConfigType,
			index:               index,
		})
	}
}

// HandleConfigEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyConfigEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleConfigEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Config) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Configs.Remove(model.Config.Id)
	} else {
		rdm.Configs.Set(model.Config.Id, &Config{
			DataStateConfig: model.Config,
			index:           index,
		})
	}
}

func (rdm *RouterDataModel) applyUpdateServicePolicyEvent(model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	servicePolicy := model.ServicePolicy
	rdm.ServicePolicies.Upsert(servicePolicy.Id, nil, func(exist bool, valueInMap *ServicePolicy, newValue *ServicePolicy) *ServicePolicy {
		if valueInMap == nil {
			return &ServicePolicy{
				DataStateServicePolicy: servicePolicy,
				Services:               cmap.New[struct{}](),
				PostureChecks:          cmap.New[struct{}](),
			}
		} else {
			return &ServicePolicy{
				DataStateServicePolicy: servicePolicy,
				Services:               valueInMap.Services,
				PostureChecks:          valueInMap.PostureChecks,
			}
		}
	})
}

func (rdm *RouterDataModel) applyDeleteServicePolicyEvent(model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	rdm.ServicePolicies.Remove(model.ServicePolicy.Id)
}

// HandleServicePolicyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServicePolicyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	pfxlog.Logger().WithField("policyId", model.ServicePolicy.Id).WithField("action", event.Action).Debug("applying service policy event")
	switch event.Action {
	case edge_ctrl_pb.DataState_Create:
		rdm.applyUpdateServicePolicyEvent(model)
	case edge_ctrl_pb.DataState_Update:
		rdm.applyUpdateServicePolicyEvent(model)
	case edge_ctrl_pb.DataState_Delete:
		rdm.applyDeleteServicePolicyEvent(model)
	}
}

// HandlePostureCheckEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyPostureCheckEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandlePostureCheckEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PostureCheck) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.PostureChecks.Remove(model.PostureCheck.Id)
	} else {
		rdm.PostureChecks.Set(model.PostureCheck.Id, &PostureCheck{
			DataStatePostureCheck: model.PostureCheck,
			index:                 index,
		})
	}
}

// HandlePublicKeyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyPublicKeyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandlePublicKeyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PublicKey) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.PublicKeys.Remove(model.PublicKey.Kid)
	} else {
		rdm.PublicKeys.Set(model.PublicKey.Kid, model.PublicKey)
	}
	rdm.recalculateCachedPublicKeys()
}

// HandleRevocationEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyRevocationEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleRevocationEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Revocation) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Revocations.Remove(model.Revocation.Id)
	} else {
		rdm.Revocations.Set(model.Revocation.Id, model.Revocation)
	}
}

func (rdm *RouterDataModel) HandleServicePolicyChange(index uint64, model *edge_ctrl_pb.DataState_ServicePolicyChange) {
	pfxlog.Logger().
		WithField("policyId", model.PolicyId).
		WithField("isAdd", model.Add).
		WithField("relatedEntityType", model.RelatedEntityType).
		WithField("relatedEntityIds", model.RelatedEntityIds).
		Debug("applying service policy change event")

	if model.RelatedEntityType == edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity {
		for _, identityId := range model.RelatedEntityIds {
			rdm.Identities.Upsert(identityId, nil, func(exist bool, valueInMap *Identity, newValue *Identity) *Identity {
				if valueInMap != nil {
					if model.Add {
						valueInMap.ServicePolicies.Set(model.PolicyId, struct{}{})
					} else {
						valueInMap.ServicePolicies.Remove(model.PolicyId)
					}
					valueInMap.serviceSetIndex = index
				}
				return valueInMap
			})
		}
		return
	}

	if !rdm.ServicePolicies.Has(model.PolicyId) {
		return
	}

	rdm.ServicePolicies.Upsert(model.PolicyId, nil, func(exist bool, valueInMap *ServicePolicy, newValue *ServicePolicy) *ServicePolicy {
		if valueInMap == nil {
			return nil
		}

		switch model.RelatedEntityType {
		case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService:
			if model.Add {
				for _, serviceId := range model.RelatedEntityIds {
					valueInMap.Services.Set(serviceId, struct{}{})
				}
			} else {
				for _, serviceId := range model.RelatedEntityIds {
					valueInMap.Services.Remove(serviceId)
				}
			}
		case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck:
			if model.Add {
				for _, postureCheckId := range model.RelatedEntityIds {
					valueInMap.PostureChecks.Set(postureCheckId, struct{}{})
				}
			} else {
				for _, postureCheckId := range model.RelatedEntityIds {
					valueInMap.PostureChecks.Remove(postureCheckId)
				}
			}
		}

		return valueInMap
	})
}

func (rdm *RouterDataModel) GetPublicKeys() map[string]crypto.PublicKey {
	return rdm.cachedPublicKeys.Load()
}

func (rdm *RouterDataModel) getPublicKeysAsCmap() cmap.ConcurrentMap[string, crypto.PublicKey] {
	m := cmap.New[crypto.PublicKey]()
	for k, v := range rdm.cachedPublicKeys.Load() {
		m.Set(k, v)
	}
	return m
}

func (rdm *RouterDataModel) recalculateCachedPublicKeys() {
	publicKeys := map[string]crypto.PublicKey{}
	rdm.PublicKeys.IterCb(func(kid string, pubKey *edge_ctrl_pb.DataState_PublicKey) {
		log := pfxlog.Logger().WithField("format", pubKey.Format).WithField("kid", kid)

		switch pubKey.Format {
		case edge_ctrl_pb.DataState_PublicKey_X509CertDer:
			if cert, err := x509.ParseCertificate(pubKey.GetData()); err != nil {
				log.WithError(err).Error("error parsing x509 certificate DER")
			} else {
				publicKeys[kid] = cert.PublicKey
			}
		case edge_ctrl_pb.DataState_PublicKey_PKIXPublicKey:
			if pub, err := x509.ParsePKIXPublicKey(pubKey.GetData()); err != nil {
				log.WithError(err).Error("error parsing PKIX public key DER")
			} else {
				publicKeys[kid] = pub
			}
		default:
			log.Error("unknown public key format")
		}
	})
	rdm.cachedPublicKeys.Store(publicKeys)
}

func (rdm *RouterDataModel) GetDataState() *edge_ctrl_pb.DataState {
	var result *edge_ctrl_pb.DataState
	rdm.EventCache.WhileLocked(func(currentIndex uint64, _ bool) {
		result = rdm.getDataStateAlreadyLocked(currentIndex)
	})
	return result
}

func (rdm *RouterDataModel) getDataStateAlreadyLocked(index uint64) *edge_ctrl_pb.DataState {
	var events []*edge_ctrl_pb.DataState_Event

	rdm.ConfigTypes.IterCb(func(key string, v *ConfigType) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ConfigType{
				ConfigType: v.DataStateConfigType,
			},
		}
		events = append(events, newEvent)
	})

	rdm.Configs.IterCb(func(key string, v *Config) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Config{
				Config: v.DataStateConfig,
			},
		}
		events = append(events, newEvent)
	})

	servicePolicyIdentities := map[string]*edge_ctrl_pb.DataState_ServicePolicyChange{}

	rdm.Identities.IterCb(func(key string, v *Identity) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Identity{
				Identity: v.DataStateIdentity,
			},
		}
		events = append(events, newEvent)

		v.ServicePolicies.IterCb(func(policyId string, _ struct{}) {
			change := servicePolicyIdentities[policyId]
			if change == nil {
				change = &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          policyId,
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
					Add:               true,
				}
				servicePolicyIdentities[policyId] = change
			}
			change.RelatedEntityIds = append(change.RelatedEntityIds, v.Id)
		})
	})

	rdm.Services.IterCb(func(key string, v *Service) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Service{
				Service: v.DataStateService,
			},
		}
		events = append(events, newEvent)
	})

	rdm.PostureChecks.IterCb(func(key string, v *PostureCheck) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
				PostureCheck: v.DataStatePostureCheck,
			},
		}
		events = append(events, newEvent)
	})

	rdm.ServicePolicies.IterCb(func(key string, v *ServicePolicy) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
				ServicePolicy: v.DataStateServicePolicy,
			},
		}
		events = append(events, newEvent)

		addServicesChange := &edge_ctrl_pb.DataState_ServicePolicyChange{
			PolicyId:          v.Id,
			RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
			Add:               true,
		}
		v.Services.IterCb(func(serviceId string, _ struct{}) {
			addServicesChange.RelatedEntityIds = append(addServicesChange.RelatedEntityIds, serviceId)
		})
		events = append(events, &edge_ctrl_pb.DataState_Event{
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: addServicesChange,
			},
		})

		addPostureCheckChanges := &edge_ctrl_pb.DataState_ServicePolicyChange{
			PolicyId:          v.Id,
			RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
			Add:               true,
		}
		v.PostureChecks.IterCb(func(postureCheckId string, _ struct{}) {
			addPostureCheckChanges.RelatedEntityIds = append(addPostureCheckChanges.RelatedEntityIds, postureCheckId)
		})
		events = append(events, &edge_ctrl_pb.DataState_Event{
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: addPostureCheckChanges,
			},
		})

		if addIdentityChanges, found := servicePolicyIdentities[v.Id]; found {
			events = append(events, &edge_ctrl_pb.DataState_Event{
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: addIdentityChanges,
				},
			})
		}
	})

	rdm.PublicKeys.IterCb(func(key string, v *edge_ctrl_pb.DataState_PublicKey) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_PublicKey{
				PublicKey: v,
			},
			IsSynthetic: true,
		}
		events = append(events, newEvent)
	})

	var caches map[string]*edge_ctrl_pb.Cache

	if !rdm.terminatorIdCache.IsEmpty() {
		caches = map[string]*edge_ctrl_pb.Cache{}

		cache := &edge_ctrl_pb.Cache{
			Data: map[string][]byte{},
		}

		rdm.terminatorIdCache.IterCb(func(key string, v string) {
			if rdm.Services.Has(key) {
				cache.Data[key] = []byte(v)
			}
		})

		caches[edge_ctrl_pb.CacheType_TerminatorIds.String()] = cache
	}

	return &edge_ctrl_pb.DataState{
		Events:     events,
		EndIndex:   index,
		TimelineId: rdm.timelineId,
		Caches:     caches,
	}
}

func (rdm *RouterDataModel) Save(path string) {
	rdm.EventCache.WhileLocked(func(index uint64, indexInitialized bool) {
		if !indexInitialized {
			pfxlog.Logger().Debug("could not save router data model, no index")
			return
		}

		// nothing to save
		if rdm.lastSaveIndex != nil && *rdm.lastSaveIndex == index {
			pfxlog.Logger().Debug("no changes to router model, nothing to save")
			return
		}

		state := rdm.getDataStateAlreadyLocked(index)
		stateBytes, err := proto.Marshal(state)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not marshal router data model")
			return
		}

		// Create a new gzip file
		file, err := os.Create(path)
		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not marshal router data model, could not create file")
			return
		}
		defer func() { _ = file.Close() }()

		// Create a gzip writer
		gz := gzip.NewWriter(file)
		defer func() { _ = gz.Close() }()

		// Write the gzipped protobuf data to the file
		_, err = gz.Write(stateBytes)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not marshal router data model, could not compress and write")
			return
		}

		rdm.lastSaveIndex = &index
	})
}

// GetServiceAccessPolicies returns an AccessPolicies instance for an identity attempting to access a service.
func (rdm *RouterDataModel) GetServiceAccessPolicies(identityId string, serviceId string, policyType edge_ctrl_pb.PolicyType) (*AccessPolicies, error) {
	identity, ok := rdm.Identities.Get(identityId)

	if !ok {
		return nil, fmt.Errorf("identity not foud by id")
	}

	service, ok := rdm.Services.Get(serviceId)

	if !ok {
		return nil, fmt.Errorf("service not found by id")
	}

	var policies []*ServicePolicy

	postureChecks := map[string]*edge_ctrl_pb.DataState_PostureCheck{}

	identity.ServicePolicies.IterCb(func(servicePolicyId string, _ struct{}) {
		servicePolicy, ok := rdm.ServicePolicies.Get(servicePolicyId)

		if ok && servicePolicy.PolicyType == policyType {
			policies = append(policies, servicePolicy)

			servicePolicy.PostureChecks.IterCb(func(postureCheckId string, _ struct{}) {
				if _, ok := postureChecks[postureCheckId]; !ok {
					//ignore ok, if !ok postureCheck == nil which will trigger
					//failure during evaluation
					postureCheck, _ := rdm.PostureChecks.Get(postureCheckId)
					postureChecks[postureCheckId] = postureCheck.DataStatePostureCheck
				}
			})
		}
	})

	return &AccessPolicies{
		Identity:      identity,
		Service:       service,
		Policies:      policies,
		PostureChecks: postureChecks,
	}, nil
}

func CloneMap[V any](m cmap.ConcurrentMap[string, V]) cmap.ConcurrentMap[string, V] {
	result := cmap.New[V]()
	m.IterCb(func(key string, v V) {
		result.Set(key, v)
	})
	return result
}

func (rdm *RouterDataModel) SubscribeToIdentityChanges(identityId string, subscriber IdentityEventSubscriber, isRouterIdentity bool) error {
	pfxlog.Logger().WithField("identityId", identityId).Debug("subscribing to changes for identity")
	identity, ok := rdm.Identities.Get(identityId)
	if !ok && !isRouterIdentity {
		return fmt.Errorf("identity %s not found", identityId)
	}

	subscription := rdm.subscriptions.Upsert(identityId, nil, func(exist bool, valueInMap *IdentitySubscription, newValue *IdentitySubscription) *IdentitySubscription {
		if exist {
			valueInMap.listeners.Append(subscriber)
			return valueInMap
		}
		result := &IdentitySubscription{
			IdentityId: identityId,
		}
		result.listeners.Append(subscriber)
		return result
	})

	if identity != nil {
		state, _ := subscription.initialize(rdm, identity)
		subscriber.NotifyIdentityEvent(state, EventFullState)
	}

	return nil
}

func (rdm *RouterDataModel) InheritLocalData(other *RouterDataModel) {
	other.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
		rdm.subscriptions.Set(key, v)
	})

	other.terminatorIdCache.IterCb(func(key string, v string) {
		rdm.terminatorIdCache.Set(key, v)
	})
}

func (rdm *RouterDataModel) buildServiceList(sub *IdentitySubscription) (map[string]*IdentityService, map[string]*PostureCheck) {
	log := pfxlog.Logger().WithField("identityId", sub.IdentityId)
	services := map[string]*IdentityService{}
	postureChecks := map[string]*PostureCheck{}

	sub.Identity.ServicePolicies.IterCb(func(policyId string, _ struct{}) {
		policy, ok := rdm.ServicePolicies.Get(policyId)
		if !ok {
			log.WithField("policyId", policyId).Error("could not find service policy")
			return
		}

		policy.Services.IterCb(func(serviceId string, _ struct{}) {
			service, ok := rdm.Services.Get(serviceId)
			if !ok {
				log.WithField("policyId", policyId).
					WithField("serviceId", serviceId).
					Error("could not find service")
				return
			}

			identityService, ok := services[serviceId]
			if !ok {
				identityService = &IdentityService{
					Service: service,
					Configs: map[string]*IdentityConfig{},
					Checks:  map[string]struct{}{},
				}
				services[serviceId] = identityService
				rdm.loadServiceConfigs(sub.Identity, identityService)
			}
			rdm.loadServicePostureChecks(sub.Identity, policy, identityService, postureChecks)

			if policy.PolicyType == edge_ctrl_pb.PolicyType_BindPolicy {
				identityService.BindAllowed = true
			} else if policy.PolicyType == edge_ctrl_pb.PolicyType_DialPolicy {
				identityService.DialAllowed = true
			}
		})
	})

	return services, postureChecks
}

func (rdm *RouterDataModel) loadServicePostureChecks(identity *Identity, policy *ServicePolicy, svc *IdentityService, checks map[string]*PostureCheck) {
	log := pfxlog.Logger().
		WithField("identityId", identity.Id).
		WithField("serviceId", svc.Service.Id).
		WithField("policyId", policy.Id)

	policy.PostureChecks.IterCb(func(postureCheckId string, _ struct{}) {
		check, ok := rdm.PostureChecks.Get(postureCheckId)
		if !ok {
			log.WithField("postureCheckId", postureCheckId).Error("could not find posture check")
		} else {
			svc.Checks[postureCheckId] = struct{}{}
			checks[postureCheckId] = check
		}
	})
}

func (rdm *RouterDataModel) loadServiceConfigs(identity *Identity, svc *IdentityService) {
	log := pfxlog.Logger().
		WithField("identityId", identity.Id).
		WithField("serviceId", svc.Service.Id)

	result := map[string]*IdentityConfig{}

	for _, configId := range svc.Service.Configs {
		identityConfig := rdm.loadIdentityConfig(configId, log)
		if identityConfig != nil {
			result[identityConfig.ConfigType.Name] = identityConfig
		}
	}

	if serviceConfigs, hasOverride := identity.ServiceConfigs[svc.Service.Id]; hasOverride {
		for _, configId := range serviceConfigs.Configs {
			identityConfig := rdm.loadIdentityConfig(configId, log)
			if identityConfig != nil {
				result[identityConfig.ConfigType.Name] = identityConfig
			}
		}
	}

	svc.Configs = result
}

func (rdm *RouterDataModel) loadIdentityConfig(configId string, log *logrus.Entry) *IdentityConfig {
	config, ok := rdm.Configs.Get(configId)
	if !ok {
		log.WithField("configId", configId).Error("could not find config")
		return nil
	}

	configType, ok := rdm.ConfigTypes.Get(config.TypeId)
	if !ok {
		log.WithField("configId", configId).
			WithField("configTypeId", config.TypeId).
			Error("could not find config type")
		return nil
	}

	return &IdentityConfig{
		Config:     config,
		ConfigType: configType,
	}
}

func (rdm *RouterDataModel) GetEntityCounts() map[string]uint32 {
	result := map[string]uint32{
		"configType":         uint32(rdm.ConfigTypes.Count()),
		"configs":            uint32(rdm.Configs.Count()),
		"identities":         uint32(rdm.Identities.Count()),
		"services":           uint32(rdm.Services.Count()),
		"service-policies":   uint32(rdm.ServicePolicies.Count()),
		"posture-checks":     uint32(rdm.PostureChecks.Count()),
		"public-keys":        uint32(rdm.PublicKeys.Count()),
		"revocations":        uint32(rdm.Revocations.Count()),
		"cached-public-keys": uint32(rdm.getPublicKeysAsCmap().Count()),
	}
	return result
}

func (rdm *RouterDataModel) GetTerminatorIdCache() cmap.ConcurrentMap[string, string] {
	return rdm.terminatorIdCache
}

type DiffType string

const (
	DiffTypeAdd = "added"
	DiffTypeMod = "modified"
	DiffTypeSub = "removed"
)

type DiffSink func(entityType string, id string, diffType DiffType, detail string)

func (rdm *RouterDataModel) Validate(correct *RouterDataModel, sink DiffSink) {
	correct.Diff(rdm, sink)
	rdm.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
		v.Diff(rdm, sink)
	})
}

func (rdm *RouterDataModel) Diff(o *RouterDataModel, sink DiffSink) {
	if o == nil {
		sink("router-data-model", "root", DiffTypeSub, "router data model not present")
		return
	}

	rdm.PublicKeys.IterCb(func(key string, v *edge_ctrl_pb.DataState_PublicKey) {
		sort.Slice(v.Usages, func(i, j int) bool {
			return v.Usages[i] < v.Usages[j]
		})
	})

	o.PublicKeys.IterCb(func(key string, v *edge_ctrl_pb.DataState_PublicKey) {
		sort.Slice(v.Usages, func(i, j int) bool {
			return v.Usages[i] < v.Usages[j]
		})
	})

	diffType("configType", rdm.ConfigTypes, o.ConfigTypes, sink, ConfigType{}, DataStateConfigType{})
	diffType("config", rdm.Configs, o.Configs, sink, Config{}, DataStateConfig{})
	diffType("identity", rdm.Identities, o.Identities, sink, Identity{}, DataStateIdentity{})
	diffType("service", rdm.Services, o.Services, sink, Service{}, DataStateService{})
	diffType("service-policy", rdm.ServicePolicies, o.ServicePolicies, sink, ServicePolicy{}, DataStateServicePolicy{})
	diffType("posture-check", rdm.PostureChecks, o.PostureChecks, sink,
		PostureCheck{}, DataStatePostureCheck{},
		edge_ctrl_pb.DataState_PostureCheck_Domains_{}, edge_ctrl_pb.DataState_PostureCheck_Domains{},
		edge_ctrl_pb.DataState_PostureCheck_Mac_{}, edge_ctrl_pb.DataState_PostureCheck_Mac{},
		edge_ctrl_pb.DataState_PostureCheck_Mfa_{}, edge_ctrl_pb.DataState_PostureCheck_Mfa{},
		edge_ctrl_pb.DataState_PostureCheck_OsList_{}, edge_ctrl_pb.DataState_PostureCheck_OsList{}, edge_ctrl_pb.DataState_PostureCheck_Os{},
		edge_ctrl_pb.DataState_PostureCheck_Process_{}, edge_ctrl_pb.DataState_PostureCheck_Process{},
		edge_ctrl_pb.DataState_PostureCheck_ProcessMulti_{}, edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{})
	diffType("public-keys", rdm.PublicKeys, o.PublicKeys, sink, edge_ctrl_pb.DataState_PublicKey{})
	diffType("revocations", rdm.Revocations, o.Revocations, sink, edge_ctrl_pb.DataState_Revocation{})
	diffMaps("cached-public-keys", rdm.getPublicKeysAsCmap(), o.getPublicKeysAsCmap(), sink, func(a, b crypto.PublicKey) []string {
		if a == nil || b == nil {
			return []string{fmt.Sprintf("cached public key is nil: orig: %v, dest: %v", a, b)}
		}
		return nil
	})
}

type diffF[T any] func(a, b T) []string

func diffMaps[T any](entityType string, m1, m2 cmap.ConcurrentMap[string, T], sink DiffSink, differ diffF[T]) {
	hasMissing := false
	m1.IterCb(func(key string, v T) {
		v2, exists := m2.Get(key)
		if !exists {
			sink(entityType, key, DiffTypeSub, "entity missing")
			hasMissing = true
		} else {
			for _, diff := range differ(v, v2) {
				sink(entityType, key, DiffTypeMod, diff)
			}
		}
	})

	if m1.Count() != m2.Count() || hasMissing {
		m2.IterCb(func(key string, v2 T) {
			if _, exists := m1.Get(key); !exists {
				sink(entityType, key, DiffTypeAdd, "entity unexpected")
			}
		})
	}
}

func diffType[P any, T *P](entityType string, m1 cmap.ConcurrentMap[string, T], m2 cmap.ConcurrentMap[string, T], sink DiffSink, ignoreTypes ...any) {
	diffReporter := &compareReporter{
		f: func(key string, detail string) {
			sink(entityType, key, DiffTypeMod, detail)
		},
	}

	hasMissing := false
	adapter := cmp.Reporter(diffReporter)
	syncSetT := cmp.Transformer("syncSetToMap", func(s cmap.ConcurrentMap[string, struct{}]) map[string]struct{} {
		return CMapToMap(s)
	})
	m1.IterCb(func(key string, v T) {
		v2, exists := m2.Get(key)
		if !exists {
			sink(entityType, key, DiffTypeSub, "entity missing")
			hasMissing = true
		} else {
			diffReporter.key = key
			cmp.Diff(v, v2, syncSetT, cmpopts.IgnoreUnexported(ignoreTypes...), adapter)
		}
	})

	if m1.Count() != m2.Count() || hasMissing {
		m2.IterCb(func(key string, v2 T) {
			if _, exists := m1.Get(key); !exists {
				sink(entityType, key, DiffTypeAdd, "entity unexpected")
			}
		})
	}
}

func CMapToMap[T any](m cmap.ConcurrentMap[string, T]) map[string]T {
	result := map[string]T{}
	m.IterCb(func(key string, val T) {
		result[key] = val
	})
	return result
}

type compareReporter struct {
	steps []cmp.PathStep
	key   string
	f     func(key string, detail string)
}

func (self *compareReporter) PushStep(step cmp.PathStep) {
	self.steps = append(self.steps, step)
}

func (self *compareReporter) Report(result cmp.Result) {
	if !result.Equal() {
		var step cmp.PathStep
		path := &bytes.Buffer{}
		for _, v := range self.steps {
			path.Write([]byte(v.String()))
			step = v
		}
		if step != nil {
			vx, vy := step.Values()
			var x any
			var y any

			if vx.IsValid() {
				x = vx.Interface()
			}
			if vy.IsValid() {
				y = vy.Interface()
			}
			err := fmt.Sprintf("%s mismatch. orig: %v, copy: %v", path.String(), x, y)
			self.f(self.key, err)
		} else {
			self.f(self.key, "programming error, empty path stack")
		}
	}
}

func (self *compareReporter) PopStep() {
	self.steps = self.steps[:len(self.steps)-1]
}
