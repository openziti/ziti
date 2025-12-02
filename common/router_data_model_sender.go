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
	"crypto"
	"crypto/x509"
	"fmt"
	"sort"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// RouterDataModelSenderConfig contains the configuration values for a RouterDataModelSender
type RouterDataModelSenderConfig struct {
	Enabled            bool
	LogSize            uint64
	ListenerBufferSize uint
}

type SenderIdentity struct {
	*DataStateIdentity
	ServicePolicies cmap.ConcurrentMap[string, struct{}] `json:"servicePolicies"`
}

type SenderServicePolicy struct {
	*DataStateServicePolicy
	Services      cmap.ConcurrentMap[string, struct{}] `json:"services"`
	PostureChecks cmap.ConcurrentMap[string, struct{}] `json:"postureChecks"`
}

// RouterDataModelSender represents a sub-set of a controller's data model. Enough to validate an identities access to dial/bind
// a service through policies and posture checks. RouterDataModelSender can operate in two modes: sender (controller) and
// receiver (router). Sender mode allows a controller support an event cache that supports replays for routers connecting
// for the first time/after disconnects. Receive mode does not maintain an event cache and does not support replays.
// It instead is used as a reference data structure for authorization computations.
type RouterDataModelSender struct {
	EventCache
	listeners map[chan *edge_ctrl_pb.DataState_ChangeSet]struct{}

	ConfigTypes      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_ConfigType]   `json:"configTypes"`
	Configs          cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Config]       `json:"configs"`
	Identities       cmap.ConcurrentMap[string, *SenderIdentity]                      `json:"identities"`
	Services         cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Service]      `json:"services"`
	ServicePolicies  cmap.ConcurrentMap[string, *SenderServicePolicy]                 `json:"servicePolicies"`
	PostureChecks    cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PostureCheck] `json:"postureChecks"`
	PublicKeys       cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PublicKey]    `json:"publicKeys"`
	Revocations      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Revocation]   `json:"revocations"`
	cachedPublicKeys concurrenz.AtomicValue[map[string]crypto.PublicKey]

	listenerBufferSize uint

	// timelineId identifies the database that events are flowing from. This will be reset whenever we change the
	// underlying datastore
	timelineId string
}

// NewRouterDataModelSender creates a new RouterDataModelSender that will store events in a circular buffer of
// logSize. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewRouterDataModelSender(timelineId string, logSize uint64, listenerBufferSize uint) *RouterDataModelSender {
	return &RouterDataModelSender{
		EventCache:         NewLoggingEventCache(logSize),
		ConfigTypes:        cmap.New[*edge_ctrl_pb.DataState_ConfigType](),
		Configs:            cmap.New[*edge_ctrl_pb.DataState_Config](),
		Identities:         cmap.New[*SenderIdentity](),
		Services:           cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies:    cmap.New[*SenderServicePolicy](),
		PostureChecks:      cmap.New[*edge_ctrl_pb.DataState_PostureCheck](),
		PublicKeys:         cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:        cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		listenerBufferSize: listenerBufferSize,
		timelineId:         timelineId,
	}
}

// NewListener returns a channel that will receive the events applied to this data model.
func (rdm *RouterDataModelSender) NewListener() <-chan *edge_ctrl_pb.DataState_ChangeSet {
	if rdm.listeners == nil {
		rdm.listeners = map[chan *edge_ctrl_pb.DataState_ChangeSet]struct{}{}
	}

	newCh := make(chan *edge_ctrl_pb.DataState_ChangeSet, rdm.listenerBufferSize)
	rdm.listeners[newCh] = struct{}{}

	return newCh
}

func (rdm *RouterDataModelSender) sendEvent(event *edge_ctrl_pb.DataState_ChangeSet) {
	for listener := range rdm.listeners {
		listener <- event
	}
}

func (rdm *RouterDataModelSender) GetTimelineId() string {
	return rdm.timelineId
}

// ApplyChangeSet applies the given even to the router data model.
func (rdm *RouterDataModelSender) ApplyChangeSet(change *edge_ctrl_pb.DataState_ChangeSet) {
	changeAccepted := false
	logger := pfxlog.Logger().
		WithField("index", change.Index).
		WithField("synthetic", change.IsSynthetic).
		WithField("entries", len(change.Changes))

	err := rdm.EventCache.Store(change, func(index uint64, change *edge_ctrl_pb.DataState_ChangeSet) {
		for idx, event := range change.Changes {
			logger.
				WithField("entry", idx).
				WithField("action", event.Action.String()).
				WithField("type", fmt.Sprintf("%T", event.Model)).
				WithField("summary", event.Summarize()).
				Info("handling change set entry")
			rdm.Handle(event)
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

func (rdm *RouterDataModelSender) Handle(event *edge_ctrl_pb.DataState_Event) {
	switch typedModel := event.Model.(type) {
	case *edge_ctrl_pb.DataState_Event_ConfigType:
		rdm.HandleConfigTypeEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Config:
		rdm.HandleConfigEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Identity:
		rdm.HandleIdentityEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Service:
		rdm.HandleServiceEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_ServicePolicy:
		rdm.HandleServicePolicyEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PostureCheck:
		rdm.HandlePostureCheckEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PublicKey:
		rdm.HandlePublicKeyEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Revocation:
		rdm.HandleRevocationEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_ServicePolicyChange:
		rdm.HandleServicePolicyChange(typedModel.ServicePolicyChange)
	}
}

// HandleIdentityEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyIdentityEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModelSender) HandleIdentityEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Identities.Remove(model.Identity.Id)
	} else {
		var identity *SenderIdentity
		rdm.Identities.Upsert(model.Identity.Id, nil, func(exist bool, valueInMap *SenderIdentity, newValue *SenderIdentity) *SenderIdentity {
			if valueInMap == nil {
				identity = &SenderIdentity{
					DataStateIdentity: model.Identity,
					ServicePolicies:   cmap.New[struct{}](),
				}
			} else {
				identity = &SenderIdentity{
					DataStateIdentity: model.Identity,
					ServicePolicies:   valueInMap.ServicePolicies,
				}
			}
			return identity
		})
	}
}

// HandleServiceEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServiceEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModelSender) HandleServiceEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Service) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Services.Remove(model.Service.Id)
		rdm.ServicePolicies.IterCb(func(key string, v *SenderServicePolicy) {
			v.Services.Remove(model.Service.Id)
		})
	} else {
		rdm.Services.Set(model.Service.Id, model.Service)
	}
}

// HandleConfigTypeEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyConfigTypeEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModelSender) HandleConfigTypeEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ConfigType) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.ConfigTypes.Remove(model.ConfigType.Id)
	} else {
		rdm.ConfigTypes.Set(model.ConfigType.Id, model.ConfigType)
	}
}

// HandleConfigEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyConfigEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModelSender) HandleConfigEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Config) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Configs.Remove(model.Config.Id)
	} else {
		rdm.Configs.Set(model.Config.Id, model.Config)
	}
}

func (rdm *RouterDataModelSender) applyUpdateServicePolicyEvent(model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	result := &SenderServicePolicy{DataStateServicePolicy: model.ServicePolicy}
	rdm.ServicePolicies.Upsert(model.ServicePolicy.Id, nil, func(exist bool, valueInMap *SenderServicePolicy, newValue *SenderServicePolicy) *SenderServicePolicy {
		if valueInMap != nil {
			result.Services = valueInMap.Services
			result.PostureChecks = valueInMap.PostureChecks
		} else {
			result.Services = cmap.New[struct{}]()
			result.PostureChecks = cmap.New[struct{}]()
		}
		return result
	})
}

func (rdm *RouterDataModelSender) applyDeleteServicePolicyEvent(model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	rdm.ServicePolicies.Remove(model.ServicePolicy.Id)
}

// HandleServicePolicyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServicePolicyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModelSender) HandleServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	pfxlog.Logger().
		WithField("policyId", model.ServicePolicy.Id).
		WithField("action", event.Action).
		Debug("applying service policy event")
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
func (rdm *RouterDataModelSender) HandlePostureCheckEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PostureCheck) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.PostureChecks.Remove(model.PostureCheck.Id)
	} else {
		rdm.PostureChecks.Set(model.PostureCheck.Id, model.PostureCheck)
	}
}

// HandlePublicKeyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyPublicKeyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModelSender) HandlePublicKeyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PublicKey) {
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
func (rdm *RouterDataModelSender) HandleRevocationEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Revocation) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Revocations.Remove(model.Revocation.Id)
	} else {
		rdm.Revocations.Set(model.Revocation.Id, model.Revocation)
	}
}

func (rdm *RouterDataModelSender) HandleServicePolicyChange(model *edge_ctrl_pb.DataState_ServicePolicyChange) {
	log := pfxlog.Logger().
		WithField("policyId", model.PolicyId).
		WithField("isAdd", model.Add).
		WithField("relatedEntityType", model.RelatedEntityType).
		WithField("relatedEntityIds", model.RelatedEntityIds)
	log.Debug("applying service policy change event")

	if model.RelatedEntityType == edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity {
		for _, identityId := range model.RelatedEntityIds {
			rdm.Identities.Upsert(identityId, nil, func(exist bool, valueInMap *SenderIdentity, newValue *SenderIdentity) *SenderIdentity {
				if valueInMap != nil {
					if model.Add {
						valueInMap.ServicePolicies.Set(model.PolicyId, struct{}{})
					} else {
						valueInMap.ServicePolicies.Remove(model.PolicyId)
					}
				}
				return valueInMap
			})
		}
		return
	}

	servicePolicy, _ := rdm.ServicePolicies.Get(model.PolicyId)

	if servicePolicy == nil {
		if model.Add {
			log.Error("service policy not present in router data model")
		}
		return
	}

	switch model.RelatedEntityType {
	case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService:
		if model.Add {
			for _, serviceId := range model.RelatedEntityIds {
				servicePolicy.Services.Set(serviceId, struct{}{})
			}
		} else {
			for _, serviceId := range model.RelatedEntityIds {
				servicePolicy.Services.Remove(serviceId)
			}
		}
	case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck:
		if model.Add {
			for _, postureCheckId := range model.RelatedEntityIds {
				servicePolicy.PostureChecks.Set(postureCheckId, struct{}{})
			}
		} else {
			for _, postureCheckId := range model.RelatedEntityIds {
				servicePolicy.PostureChecks.Remove(postureCheckId)
			}
		}
	}
}

func (rdm *RouterDataModelSender) GetPublicKeys() map[string]crypto.PublicKey {
	return rdm.cachedPublicKeys.Load()
}

func (rdm *RouterDataModelSender) getPublicKeysAsCmap() cmap.ConcurrentMap[string, crypto.PublicKey] {
	m := cmap.New[crypto.PublicKey]()
	for k, v := range rdm.cachedPublicKeys.Load() {
		m.Set(k, v)
	}
	return m
}

func (rdm *RouterDataModelSender) recalculateCachedPublicKeys() {
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

func (rdm *RouterDataModelSender) GetDataState() *edge_ctrl_pb.DataState {
	var result *edge_ctrl_pb.DataState
	rdm.EventCache.WhileLocked(func(currentIndex uint64) {
		result = rdm.getDataStateAlreadyLocked(currentIndex)
	})
	return result
}

func (rdm *RouterDataModelSender) getDataStateAlreadyLocked(index uint64) *edge_ctrl_pb.DataState {
	var events []*edge_ctrl_pb.DataState_Event

	rdm.ConfigTypes.IterCb(func(key string, configType *edge_ctrl_pb.DataState_ConfigType) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ConfigType{
				ConfigType: configType,
			},
		}
		events = append(events, newEvent)
	})

	rdm.Configs.IterCb(func(key string, v *edge_ctrl_pb.DataState_Config) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Config{
				Config: v,
			},
		}
		events = append(events, newEvent)
	})

	servicePolicyIdentities := map[string]*edge_ctrl_pb.DataState_ServicePolicyChange{}

	rdm.Identities.IterCb(func(key string, v *SenderIdentity) {
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

	rdm.Services.IterCb(func(key string, v *edge_ctrl_pb.DataState_Service) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Service{
				Service: v,
			},
		}
		events = append(events, newEvent)
	})

	rdm.PostureChecks.IterCb(func(key string, v *edge_ctrl_pb.DataState_PostureCheck) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
				PostureCheck: v,
			},
		}
		events = append(events, newEvent)
	})

	rdm.ServicePolicies.IterCb(func(key string, v *SenderServicePolicy) {
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

	return &edge_ctrl_pb.DataState{
		Events:     events,
		EndIndex:   index,
		TimelineId: rdm.timelineId,
	}
}

func (rdm *RouterDataModelSender) GetEntityCounts() map[string]uint32 {
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

func (rdm *RouterDataModelSender) Validate(correct *RouterDataModelSender, sink DiffSink) {
	correct.Diff(rdm, sink)
}

func (rdm *RouterDataModelSender) Diff(o *RouterDataModelSender, sink DiffSink) {
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
