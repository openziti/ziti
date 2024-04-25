package common

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"io"
	"os"
)

// AccessPolicies represents the Identity's access to a Service through many Policies. The PostureChecks provided
// are referenced by the granting Policies. The PostureChecks for each of the Policies may be evaluated to determine
// a valid policy and posture access path.
type AccessPolicies struct {
	Identity      *Identity
	Service       *edge_ctrl_pb.DataState_Service
	Policies      []*ServicePolicy
	PostureChecks map[string]*edge_ctrl_pb.DataState_PostureCheck
}

type DataStateIdentity = edge_ctrl_pb.DataState_Identity

type Identity struct {
	*DataStateIdentity
	ServicePolicies map[string]struct{} `json:"servicePolicies"`
}

type DataStateServicePolicy = edge_ctrl_pb.DataState_ServicePolicy

type ServicePolicy struct {
	*DataStateServicePolicy
	Services      map[string]struct{} `json:"services"`
	PostureChecks map[string]struct{} `json:"postureChecks"`
}

// RouterDataModel represents a sub-set of a controller's data model. Enough to validate an identities access to dial/bind
// a service through policies and posture checks. RouterDataModel can operate in two modes: sender (controller) and
// receiver (router). Sender mode allows a controller support an event cache that supports replays for routers connecting
// for the first time/after disconnects. Receive mode does not maintain an event cache and does not support replays.
// It instead is used as a reference data structure for authorization computations.
type RouterDataModel struct {
	EventCache
	listeners map[chan *edge_ctrl_pb.DataState_ChangeSet]struct{}

	Identities      cmap.ConcurrentMap[string, *Identity]                            `json:"identities"`
	Services        cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Service]      `json:"services"`
	ServicePolicies cmap.ConcurrentMap[string, *ServicePolicy]                       `json:"servicePolicies"`
	PostureChecks   cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PostureCheck] `json:"postureChecks"`
	PublicKeys      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PublicKey]    `json:"publicKeys"`
	Revocations     cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Revocation]   `json:"revocations"`

	listenerBufferSize uint
	lastSaveIndex      *uint64
}

// NewSenderRouterDataModel creates a new RouterDataModel that will store events in a circular buffer of
// logSize. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewSenderRouterDataModel(logSize uint64, listenerBufferSize uint) *RouterDataModel {
	return &RouterDataModel{
		EventCache:         NewLoggingEventCache(logSize),
		Identities:         cmap.New[*Identity](),
		Services:           cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies:    cmap.New[*ServicePolicy](),
		PostureChecks:      cmap.New[*edge_ctrl_pb.DataState_PostureCheck](),
		PublicKeys:         cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:        cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		listenerBufferSize: listenerBufferSize,
	}
}

// NewReceiverRouterDataModel creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModel(listenerBufferSize uint) *RouterDataModel {
	return &RouterDataModel{
		EventCache:         NewForgetfulEventCache(),
		Identities:         cmap.New[*Identity](),
		Services:           cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies:    cmap.New[*ServicePolicy](),
		PostureChecks:      cmap.New[*edge_ctrl_pb.DataState_PostureCheck](),
		PublicKeys:         cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:        cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		listenerBufferSize: listenerBufferSize,
	}
}

// NewReceiverRouterDataModelFromFile creates a new RouterDataModel that does not store events and is initialized from
// a file backup. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromFile(path string, listenerBufferSize uint) (*RouterDataModel, error) {
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

	rdmContents := &rdmDb{
		RouterDataModel: NewReceiverRouterDataModel(listenerBufferSize),
	}

	err = json.Unmarshal(data, rdmContents)
	if err != nil {
		return nil, err
	}

	rdmContents.RouterDataModel.lastSaveIndex = &rdmContents.Index

	return rdmContents.RouterDataModel, nil
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

// Apply applies the given even to the router data model.
func (rdm *RouterDataModel) ApplyChangeSet(change *edge_ctrl_pb.DataState_ChangeSet) {
	changeAccepted := false
	err := rdm.EventCache.Store(change, func(index uint64, change *edge_ctrl_pb.DataState_ChangeSet) {
		for _, event := range change.Changes {
			rdm.Handle(event)
		}
		changeAccepted = true
	})

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("index", change.Index).
			Error("could not store identity event")
		return
	}

	if changeAccepted {
		rdm.sendEvent(change)
	}
}

func (rdm *RouterDataModel) Handle(event *edge_ctrl_pb.DataState_Event) {
	switch typedModel := event.Model.(type) {
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
func (rdm *RouterDataModel) HandleIdentityEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Identities.Remove(model.Identity.Id)
	} else {
		rdm.Identities.Upsert(model.Identity.Id, nil, func(exist bool, valueInMap *Identity, newValue *Identity) *Identity {
			if valueInMap == nil {
				return &Identity{
					DataStateIdentity: model.Identity,
					ServicePolicies:   map[string]struct{}{},
				}
			}
			valueInMap.DataStateIdentity = model.Identity
			return valueInMap
		})
	}
}

// HandleServiceEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServiceEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServiceEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Service) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Services.Remove(model.Service.Id)
	} else {
		rdm.Services.Set(model.Service.Id, model.Service)
	}
}

func (rdm *RouterDataModel) applyUpdateServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	servicePolicy := model.ServicePolicy
	rdm.ServicePolicies.Upsert(servicePolicy.Id, nil, func(exist bool, valueInMap *ServicePolicy, newValue *ServicePolicy) *ServicePolicy {
		if valueInMap == nil {
			return &ServicePolicy{
				DataStateServicePolicy: servicePolicy,
				Services:               map[string]struct{}{},
				PostureChecks:          map[string]struct{}{},
			}
		}
		valueInMap.DataStateServicePolicy = servicePolicy
		return valueInMap
	})
}

func (rdm *RouterDataModel) applyDeleteServicePolicyEvent(_ *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	rdm.ServicePolicies.Remove(model.ServicePolicy.Id)
}

// HandleServicePolicyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServicePolicyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	switch event.Action {
	case edge_ctrl_pb.DataState_Create:
		rdm.applyUpdateServicePolicyEvent(event, model)
	case edge_ctrl_pb.DataState_Update:
		rdm.applyUpdateServicePolicyEvent(event, model)
	case edge_ctrl_pb.DataState_Delete:
		rdm.applyDeleteServicePolicyEvent(event, model)
	}
}

// HandlePostureCheckEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyPostureCheckEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandlePostureCheckEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PostureCheck) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.PostureChecks.Remove(model.PostureCheck.Id)
	} else {
		rdm.PostureChecks.Set(model.PostureCheck.Id, model.PostureCheck)
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

func (rdm *RouterDataModel) HandleServicePolicyChange(model *edge_ctrl_pb.DataState_ServicePolicyChange) {
	if model.RelatedEntityType == edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity {
		for _, identityId := range model.RelatedEntityIds {
			rdm.Identities.Upsert(identityId, nil, func(exist bool, valueInMap *Identity, newValue *Identity) *Identity {
				if valueInMap != nil {
					if model.Add {
						valueInMap.ServicePolicies[model.PolicyId] = struct{}{}
					} else {
						delete(valueInMap.ServicePolicies, model.PolicyId)
					}
				}
				return valueInMap
			})
		}
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
					valueInMap.Services[serviceId] = struct{}{}
				}
			} else {
				for _, serviceId := range model.RelatedEntityIds {
					delete(valueInMap.Services, serviceId)
				}
			}
		case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck:
			if model.Add {
				for _, postureCheckId := range model.RelatedEntityIds {
					valueInMap.PostureChecks[postureCheckId] = struct{}{}
				}
			} else {
				for _, postureCheckId := range model.RelatedEntityIds {
					delete(valueInMap.PostureChecks, postureCheckId)
				}
			}
		}

		return valueInMap
	})

}

func (rdm *RouterDataModel) GetPublicKeys() map[string]*edge_ctrl_pb.DataState_PublicKey {
	return rdm.PublicKeys.Items()
}

func (rdm *RouterDataModel) GetDataState() *edge_ctrl_pb.DataState {
	var events []*edge_ctrl_pb.DataState_Event

	rdm.EventCache.WhileLocked(func(_ uint64, _ bool) {
		servicePolicyIdentities := map[string]*edge_ctrl_pb.DataState_ServicePolicyChange{}

		rdm.Identities.IterCb(func(key string, v *Identity) {
			newEvent := &edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_Identity{
					Identity: v.DataStateIdentity,
				},
			}
			events = append(events, newEvent)

			for policyId := range v.ServicePolicies {
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
			}
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
			for serviceId := range v.Services {
				addServicesChange.RelatedEntityIds = append(addServicesChange.RelatedEntityIds, serviceId)
			}
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
			for postureCheckId := range v.PostureChecks {
				addPostureCheckChanges.RelatedEntityIds = append(addPostureCheckChanges.RelatedEntityIds, postureCheckId)
			}
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
			}
			events = append(events, newEvent)
		})
	})

	return &edge_ctrl_pb.DataState{
		Events: events,
	}
}

// rdmDb is a helper structure of serializing router data models to JSON gzipped files.
type rdmDb struct {
	RouterDataModel *RouterDataModel `json:"model"`
	Index           uint64           `json:"index"`
}

func (rdm *RouterDataModel) Save(path string) {
	rdm.EventCache.WhileLocked(func(index uint64, indexInitialized bool) {
		if !indexInitialized {
			pfxlog.Logger().Debug("could not save router data model, no index")
			return
		}

		//nothing to save
		if rdm.lastSaveIndex != nil && *rdm.lastSaveIndex == index {
			pfxlog.Logger().Debug("no changes to router model, nothing to save")
			return
		}

		rdm.lastSaveIndex = &index

		rdmFile := rdmDb{
			RouterDataModel: rdm,
			Index:           index,
		}

		jsonBytes, err := json.Marshal(rdmFile)

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

		// Write the gzipped JSON data to the file
		_, err = gz.Write(jsonBytes)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not marshal router data model, could not compress and write")
			return
		}
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

	for servicePolicyId := range identity.ServicePolicies {
		servicePolicy, ok := rdm.ServicePolicies.Get(servicePolicyId)

		if !ok {
			continue
		}

		if servicePolicy.PolicyType != policyType {
			continue
		}

		policies = append(policies, servicePolicy)

		for postureCheckId := range servicePolicy.PostureChecks {
			if _, ok := postureChecks[postureCheckId]; !ok {
				//ignore ok, if !ok postureCheck == nil which will trigger
				//failure during evaluation
				postureCheck, _ := rdm.PostureChecks.Get(postureCheckId)
				postureChecks[postureCheckId] = postureCheck
			}
		}
	}

	return &AccessPolicies{
		Identity:      identity,
		Service:       service,
		Policies:      policies,
		PostureChecks: postureChecks,
	}, nil
}
