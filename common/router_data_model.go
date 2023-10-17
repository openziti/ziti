package common

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"io"
	"os"
)

// AccessPolicies represents the Identity's access to a Service through many Policies. The PostureChecks provided
// are referenced by the granting Policies. The PostureChecks for each of the Policies may be evaluated to determine
// a valid policy and posture access path.
type AccessPolicies struct {
	Identity      *edge_ctrl_pb.DataState_Identity
	Service       *edge_ctrl_pb.DataState_Service
	Policies      []*edge_ctrl_pb.DataState_ServicePolicy
	PostureChecks map[string]*edge_ctrl_pb.DataState_PostureCheck
}

// RouterDataModel represents a sub-set of a controller's data model. Enough to validate an identities access to dial/bind
// a service through policies and posture checks. RouterDataModel can operate in two modes: sender (controller) and
// receiver (router). Sender mode allows a controller support an event cache that supports replays for routers connecting
// for the first time/after disconnects. Receive mode does not maintain an event cache and does not support replays.
// It instead is used as a reference data structure for authorization computations.
type RouterDataModel struct {
	EventCache
	listeners map[chan *edge_ctrl_pb.DataState_Event]struct{}

	Identities      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Identity]      `json:"identities"`
	Services        cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Service]       `json:"services"`
	ServicePolicies cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_ServicePolicy] `json:"servicePolicies"`
	PostureChecks   cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PostureCheck]  `json:"postureChecks"`
	PublicKeys      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PublicKey]     `json:"publicKeys"`
	Revocations     cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Revocation]    `json:"revocations"`

	listenerBufferSize uint
	lastSaveIndex      *uint64
}

// NewSenderRouterDataModel creates a new RouterDataModel that will store events in a circular buffer of
// logSize. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewSenderRouterDataModel(logSize uint64, listenerBufferSize uint) *RouterDataModel {
	return &RouterDataModel{
		EventCache:         NewLoggingEventCache(logSize),
		Identities:         cmap.New[*edge_ctrl_pb.DataState_Identity](),
		Services:           cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies:    cmap.New[*edge_ctrl_pb.DataState_ServicePolicy](),
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
		Identities:         cmap.New[*edge_ctrl_pb.DataState_Identity](),
		Services:           cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies:    cmap.New[*edge_ctrl_pb.DataState_ServicePolicy](),
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
func (rdm *RouterDataModel) NewListener() <-chan *edge_ctrl_pb.DataState_Event {
	if rdm.listeners == nil {
		rdm.listeners = map[chan *edge_ctrl_pb.DataState_Event]struct{}{}
	}

	newCh := make(chan *edge_ctrl_pb.DataState_Event, rdm.listenerBufferSize)
	rdm.listeners[newCh] = struct{}{}

	return newCh
}

func (rdm *RouterDataModel) sendEvent(event *edge_ctrl_pb.DataState_Event) {
	for listener := range rdm.listeners {
		listener <- event
	}
}

// Apply applies the given even to the router data model.
func (rdm *RouterDataModel) Apply(event *edge_ctrl_pb.DataState_Event) {
	switch typedModel := event.Model.(type) {
	case *edge_ctrl_pb.DataState_Event_Identity:
		rdm.ApplyIdentityEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Service:
		rdm.ApplyServiceEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_ServicePolicy:
		rdm.ApplyServicePolicyEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PostureCheck:
		rdm.ApplyPostureCheckEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PublicKey:
		rdm.ApplyPublicKeyEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Revocation:
		rdm.ApplyRevocationEvent(event, typedModel)
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
	}
}

// HandleIdentityEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyIdentityEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleIdentityEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Identities.Remove(model.Identity.Id)

		rdm.ServicePolicies.IterCb(func(servicePolicyId string, servicePolicy *edge_ctrl_pb.DataState_ServicePolicy) {
			servicePolicy.IdentityIds = stringz.Remove(servicePolicy.IdentityIds, model.Identity.Id)
		})
	} else {
		rdm.Identities.Upsert(model.Identity.Id, nil, func(exist bool, valueInMap *edge_ctrl_pb.DataState_Identity, newValue *edge_ctrl_pb.DataState_Identity) *edge_ctrl_pb.DataState_Identity {
			if !exist {
				return model.Identity
			}

			valueInMap.Name = model.Identity.Name

			return valueInMap
		})
	}
}

func (rdm *RouterDataModel) ApplyIdentityEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	err := rdm.EventCache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_Event) {
		rdm.HandleIdentityEvent(event, model)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithFields(map[string]interface{}{
				"event": event,
			}).
			Error("could not store identity event")
		return
	}

	rdm.sendEvent(event)
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

func (rdm *RouterDataModel) ApplyServiceEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Service) {
	err := rdm.EventCache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_Event) {
		rdm.HandleServiceEvent(event, model)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithFields(map[string]interface{}{
				"event": event,
			}).
			Error("could not store service event")
		return
	}

	rdm.sendEvent(event)
}

func (rdm *RouterDataModel) applyCreateServicePolicyEvent(_ *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	servicePolicy := model.ServicePolicy
	rdm.ServicePolicies.Set(servicePolicy.Id, servicePolicy)

	for _, identityId := range servicePolicy.IdentityIds {
		rdm.Identities.Upsert(identityId, nil, func(exist bool, valueInMap *edge_ctrl_pb.DataState_Identity, newValue *edge_ctrl_pb.DataState_Identity) *edge_ctrl_pb.DataState_Identity {
			if !exist {
				return &edge_ctrl_pb.DataState_Identity{
					Id:               identityId,
					Name:             "UNKNOWN",
					ServicePolicyIds: []string{servicePolicy.Id},
				}
			}

			if !stringz.Contains(valueInMap.ServicePolicyIds, servicePolicy.Id) {
				valueInMap.ServicePolicyIds = append(valueInMap.ServicePolicyIds, servicePolicy.Id)
			}

			return valueInMap
		})
	}
}

func (rdm *RouterDataModel) applyUpdateServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	oldPolicy, ok := rdm.ServicePolicies.Get(model.ServicePolicy.Id)

	if !ok {
		rdm.applyCreateServicePolicyEvent(event, model)
		return
	}

	removeFromIdentities := stringz.Difference(oldPolicy.IdentityIds, model.ServicePolicy.IdentityIds)
	addToIdentities := stringz.Difference(model.ServicePolicy.IdentityIds, oldPolicy.IdentityIds)

	for _, identityId := range removeFromIdentities {
		rdm.Identities.Upsert(identityId, nil, func(exist bool, valueInMap *edge_ctrl_pb.DataState_Identity, newValue *edge_ctrl_pb.DataState_Identity) *edge_ctrl_pb.DataState_Identity {
			if !exist {
				return &edge_ctrl_pb.DataState_Identity{
					Id: identityId,
				}
			}
			valueInMap.ServicePolicyIds = stringz.Remove(valueInMap.ServicePolicyIds, model.ServicePolicy.Id)

			return valueInMap
		})
	}

	for _, identityId := range addToIdentities {
		rdm.Identities.Upsert(identityId, nil, func(exist bool, valueInMap *edge_ctrl_pb.DataState_Identity, newValue *edge_ctrl_pb.DataState_Identity) *edge_ctrl_pb.DataState_Identity {
			if !exist {
				return &edge_ctrl_pb.DataState_Identity{
					Id:               identityId,
					ServicePolicyIds: []string{model.ServicePolicy.Id},
				}
			}

			if !stringz.Contains(valueInMap.ServicePolicyIds, model.ServicePolicy.Id) {
				valueInMap.ServicePolicyIds = append(valueInMap.ServicePolicyIds, model.ServicePolicy.Id)
			}

			return valueInMap
		})
	}
}

func (rdm *RouterDataModel) applyDeleteServicePolicyEvent(_ *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	for _, identityId := range model.ServicePolicy.IdentityIds {
		if identity, ok := rdm.Identities.Get(identityId); ok {
			identity.ServicePolicyIds = stringz.Remove(identity.ServicePolicyIds, model.ServicePolicy.Id)
		}
	}

	rdm.ServicePolicies.Remove(model.ServicePolicy.Id)
}

// HandleServicePolicyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServicePolicyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	switch event.Action {
	case edge_ctrl_pb.DataState_Create:
		rdm.applyCreateServicePolicyEvent(event, model)
	case edge_ctrl_pb.DataState_Update:
		rdm.applyUpdateServicePolicyEvent(event, model)
	case edge_ctrl_pb.DataState_Delete:
		rdm.applyDeleteServicePolicyEvent(event, model)
	}
}

func (rdm *RouterDataModel) ApplyServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	err := rdm.EventCache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_Event) {
		rdm.HandleServicePolicyEvent(event, model)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithFields(map[string]interface{}{
				"event": event,
			}).
			Error("could not store service policy event")
		return
	}

	rdm.sendEvent(event)
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

func (rdm *RouterDataModel) ApplyPostureCheckEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PostureCheck) {
	err := rdm.EventCache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_Event) {
		rdm.HandlePostureCheckEvent(event, model)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithFields(map[string]interface{}{
				"event": event,
			}).
			Error("could not store posture check event")
		return
	}

	rdm.sendEvent(event)
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

func (rdm *RouterDataModel) ApplyPublicKeyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PublicKey) {
	err := rdm.EventCache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_Event) {
		rdm.HandlePublicKeyEvent(event, model)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithFields(map[string]interface{}{
				"event": event,
			}).
			Error("could not store public key event")
		return
	}

	rdm.sendEvent(event)
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

func (rdm *RouterDataModel) ApplyRevocationEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Revocation) {
	err := rdm.EventCache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_Event) {
		rdm.HandleRevocationEvent(event, model)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithFields(map[string]interface{}{
				"event": event,
			}).
			Error("could not store revocation event")
		return
	}

	rdm.sendEvent(event)
}

func (rdm *RouterDataModel) GetDataState() *edge_ctrl_pb.DataState {
	var events []*edge_ctrl_pb.DataState_Event

	rdm.EventCache.WhileLocked(func(_ uint64, _ bool) {
		identityBuffer := rdm.Identities.IterBuffered()
		serviceBuffer := rdm.Services.IterBuffered()
		ServicePoliciesBuffer := rdm.ServicePolicies.IterBuffered()
		postureCheckBuffer := rdm.PostureChecks.IterBuffered()
		publicKeysBuffer := rdm.PublicKeys.IterBuffered()

		for entry := range identityBuffer {
			newEvent := &edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_Identity{
					Identity: entry.Val,
				},
			}
			events = append(events, newEvent)
		}

		for entry := range serviceBuffer {
			newEvent := &edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_Service{
					Service: entry.Val,
				},
			}
			events = append(events, newEvent)
		}

		for entry := range postureCheckBuffer {
			newEvent := &edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
					PostureCheck: entry.Val,
				},
			}
			events = append(events, newEvent)
		}

		for entry := range ServicePoliciesBuffer {
			newEvent := &edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
					ServicePolicy: entry.Val,
				},
			}
			events = append(events, newEvent)
		}

		for entry := range publicKeysBuffer {
			newEvent := &edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_PublicKey{
					PublicKey: entry.Val,
				},
			}
			events = append(events, newEvent)
		}
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
func (rdm *RouterDataModel) GetServiceAccessPolicies(identityId string, serviceId string) (*AccessPolicies, error) {
	identity, ok := rdm.Identities.Get(identityId)

	if !ok {
		return nil, fmt.Errorf("identity not foud by id")
	}

	service, ok := rdm.Services.Get(serviceId)

	if !ok {
		return nil, fmt.Errorf("service not found by id")
	}

	var policies []*edge_ctrl_pb.DataState_ServicePolicy

	postureChecks := map[string]*edge_ctrl_pb.DataState_PostureCheck{}

	for _, servicePolicyId := range identity.ServicePolicyIds {
		servicePolicy, ok := rdm.ServicePolicies.Get(servicePolicyId)

		if !ok {
			continue
		}

		if stringz.Contains(servicePolicy.IdentityIds, identityId) {
			policies = append(policies, servicePolicy)

			for _, postureCheckId := range servicePolicy.PostureCheckIds {
				if _, ok := postureChecks[postureCheckId]; !ok {
					//ignore ok, if !ok postureCheck == nil which will trigger
					//failure during evaluation
					postureCheck, _ := rdm.PostureChecks.Get(postureCheckId)
					postureChecks[postureCheckId] = postureCheck
				}
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
