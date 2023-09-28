package common

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"io"
	"os"
	"sync"
)

const (
	DataRouterModel      = "routerDataModel"
	DataRouterModelIndex = "routerModelIndex"
)

type RouterDataModel struct {
	lock sync.RWMutex

	*EventCache
	eventChannel chan *edge_ctrl_pb.DataState_Event

	Identities      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Identity]      `json:"identities"`
	Services        cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Service]       `json:"services"`
	ServicePolicies cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_ServicePolicy] `json:"servicePolicies"`
	PostureChecks   cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PostureCheck]  `json:"postureChecks"`
	PublicKeys      cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PublicKey]     `json:"publicKeys"`
	Revocations     cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Revocation]    `json:"revocations"`

	bufferSize    uint
	lastSaveIndex *uint64
}

func NewRouterDataModel(logSize uint64, bufferSize uint) *RouterDataModel {
	return &RouterDataModel{
		EventCache:      NewEventCache(logSize),
		Identities:      cmap.New[*edge_ctrl_pb.DataState_Identity](),
		Services:        cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies: cmap.New[*edge_ctrl_pb.DataState_ServicePolicy](),
		PostureChecks:   cmap.New[*edge_ctrl_pb.DataState_PostureCheck](),
		PublicKeys:      cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:     cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		bufferSize:      bufferSize,
	}
}

func NewReceiverRouterDataModel() *RouterDataModel {
	return &RouterDataModel{
		Identities:      cmap.New[*edge_ctrl_pb.DataState_Identity](),
		Services:        cmap.New[*edge_ctrl_pb.DataState_Service](),
		ServicePolicies: cmap.New[*edge_ctrl_pb.DataState_ServicePolicy](),
		PostureChecks:   cmap.New[*edge_ctrl_pb.DataState_PostureCheck](),
		PublicKeys:      cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:     cmap.New[*edge_ctrl_pb.DataState_Revocation](),
	}
}

func NewReceiverRouterDataModelFromFile(path string) (*RouterDataModel, error) {
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
		RouterDataModel: NewReceiverRouterDataModel(),
	}

	err = json.Unmarshal(data, rdmContents)
	if err != nil {
		return nil, err
	}

	rdmContents.RouterDataModel.lastSaveIndex = &rdmContents.Index

	return rdmContents.RouterDataModel, nil
}

type RouterDataModelCache interface {
	PeerAdded(peers []*event.ClusterPeer)

	Initialize(int64, int) error

	Apply(event *edge_ctrl_pb.DataState_Event)

	GetEventChannel() <-chan *edge_ctrl_pb.DataState_Event
}

func (rdm *RouterDataModel) GetEventChannel() <-chan *edge_ctrl_pb.DataState_Event {
	if rdm.eventChannel == nil {
		rdm.eventChannel = make(chan *edge_ctrl_pb.DataState_Event, rdm.bufferSize)
	}

	return rdm.eventChannel
}

func (rdm *RouterDataModel) broadcastEvent(event *edge_ctrl_pb.DataState_Event) {
	if rdm.eventChannel != nil {
		rdm.eventChannel <- event
	}
}

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

func (rdm *RouterDataModel) ApplyIdentityEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

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

	rdm.store(event)
	rdm.broadcastEvent(event)
}

func (rdm *RouterDataModel) ApplyServiceEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Service) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Services.Remove(model.Service.Id)
	} else {
		rdm.Services.Set(model.Service.Id, model.Service)
	}

	rdm.store(event)
	rdm.broadcastEvent(event)
}

func (rdm *RouterDataModel) ApplyCreateServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

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

func (rdm *RouterDataModel) ApplyUpdateServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	oldPolicy, ok := rdm.ServicePolicies.Get(model.ServicePolicy.Id)

	if !ok {
		rdm.ApplyCreateServicePolicyEvent(event, model)
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

func (rdm *RouterDataModel) ApplyDeleteServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	for _, identityId := range model.ServicePolicy.IdentityIds {
		if identity, ok := rdm.Identities.Get(identityId); ok {
			identity.ServicePolicyIds = stringz.Remove(identity.ServicePolicyIds, model.ServicePolicy.Id)
		}
	}

	rdm.ServicePolicies.Remove(model.ServicePolicy.Id)
}

func (rdm *RouterDataModel) ApplyServicePolicyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	switch event.Action {
	case edge_ctrl_pb.DataState_Create:
		rdm.ApplyCreateServicePolicyEvent(event, model)
	case edge_ctrl_pb.DataState_Update:
		rdm.ApplyUpdateServicePolicyEvent(event, model)
	case edge_ctrl_pb.DataState_Delete:
		rdm.ApplyDeleteServicePolicyEvent(event, model)
	}

	rdm.store(event)
	rdm.broadcastEvent(event)
}

func (rdm *RouterDataModel) ApplyPostureCheckEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PostureCheck) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.PostureChecks.Remove(model.PostureCheck.Id)
	} else {
		rdm.PostureChecks.Set(model.PostureCheck.Id, model.PostureCheck)
	}

	rdm.store(event)
	rdm.broadcastEvent(event)
}

func (rdm *RouterDataModel) ApplyPublicKeyEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PublicKey) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.PublicKeys.Remove(model.PublicKey.Kid)
	} else {
		rdm.PublicKeys.Set(model.PublicKey.Kid, model.PublicKey)
	}

	rdm.store(event)
	rdm.broadcastEvent(event)
}

func (rdm *RouterDataModel) ApplyRevocationEvent(event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Revocation) {
	if index, ok := rdm.EventCache.CurrentIndex(); ok && index >= event.Index {
		// old event
		return
	}

	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Revocations.Remove(model.Revocation.Id)
	} else {
		rdm.Revocations.Set(model.Revocation.Id, model.Revocation)
	}

	rdm.store(event)
	rdm.broadcastEvent(event)
}

func (rdm *RouterDataModel) GetDataState() *edge_ctrl_pb.DataState {
	//the resulting events may be good to cache as the model should stagnate at some point as users stop making config changes
	var events []*edge_ctrl_pb.DataState_Event

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

	return &edge_ctrl_pb.DataState{
		Events: events,
	}
}

type rdmDb struct {
	RouterDataModel *RouterDataModel `json:"model"`
	Index           uint64           `json:"index"`
}

func (rdm *RouterDataModel) Save(path string) {
	rdm.lock.Lock()
	defer rdm.lock.Unlock()

	index, ok := rdm.CurrentIndex()

	if !ok {
		pfxlog.Logger().Debug("could not save router data model, no index")
		return
	}

	//nothing to save
	if rdm.lastSaveIndex != nil && *rdm.lastSaveIndex == index {
		pfxlog.Logger().Debug("no changes to router model, nothing to save")
		return
	}

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
}

type AccessPolicies struct {
	Identity      *edge_ctrl_pb.DataState_Identity
	Service       *edge_ctrl_pb.DataState_Service
	Policies      []*edge_ctrl_pb.DataState_ServicePolicy
	PostureChecks map[string]*edge_ctrl_pb.DataState_PostureCheck
}

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

type EventCache struct {
	lock         sync.Mutex
	HeadLogIndex uint64
	LogSize      uint64
	Log          []uint64
	Events       map[uint64]*edge_ctrl_pb.DataState_Event
}

func NewEventCache(logSize uint64) *EventCache {
	return &EventCache{
		HeadLogIndex: 0,
		LogSize:      logSize,
		Log:          make([]uint64, logSize),
		Events:       map[uint64]*edge_ctrl_pb.DataState_Event{},
	}
}

func (cache *EventCache) store(event *edge_ctrl_pb.DataState_Event) {
	if cache == nil {
		return
	}

	cache.lock.Lock()
	defer cache.lock.Unlock()

	targetLogIndex := uint64(0)

	targetLogIndex = (cache.HeadLogIndex + 1) % cache.LogSize

	// delete old value if we have looped
	prevKey := cache.Log[targetLogIndex]

	if prevKey != 0 {
		delete(cache.Events, prevKey)
	}

	// add new values
	cache.Log[targetLogIndex] = event.Index
	cache.Events[event.Index] = event

	//update head
	cache.HeadLogIndex = targetLogIndex
}

func (cache *EventCache) CurrentIndex() (uint64, bool) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	if len(cache.Log) == 0 {
		return 0, false
	}

	return cache.Log[cache.HeadLogIndex], true
}

func (cache *EventCache) ReplayFrom(startIndex uint64) ([]*edge_ctrl_pb.DataState_Event, bool) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	_, eventFound := cache.Events[startIndex]

	if !eventFound {
		return nil, false
	}

	var startLogIndex *uint64

	for logIndex, eventIndex := range cache.Log {
		if eventIndex == startIndex {
			tmp := uint64(logIndex)
			startLogIndex = &tmp
			break
		}
	}

	if startLogIndex == nil {
		return nil, false
	}

	// no replay
	if *startLogIndex == cache.HeadLogIndex {
		return nil, true
	}

	// ez replay
	if *startLogIndex < cache.HeadLogIndex {
		var result []*edge_ctrl_pb.DataState_Event
		for _, key := range cache.Log[*startLogIndex:cache.HeadLogIndex] {
			result = append(result, cache.Events[key])
		}
		return result, true
	}

	//looping replay
	var result []*edge_ctrl_pb.DataState_Event
	for _, key := range cache.Log[*startLogIndex:] {
		result = append(result, cache.Events[key])
	}

	for _, key := range cache.Log[0:cache.HeadLogIndex] {
		result = append(result, cache.Events[key])
	}

	return result, true
}
