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
	"maps"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const (
	IdentityAdded   byte = 1
	IdentityUpdated byte = 2
	IdentityDeleted byte = 4
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

// serviceAccess tracks the number of dial and bind policies that grant an identity access to a service.
// It maintains counters for both policy types and tracks the latest index for each to support incremental updates.
type serviceAccess struct {
	DialPoliciesCount int32
	BindPoliciesCount int32

	dialPoliciesIndex uint64
	bindPoliciesIndex uint64
}

func (self *serviceAccess) updateServicePolicies(servicePolicy *ServicePolicy, index uint64, add bool) {
	if servicePolicy.IsDial() {
		if add {
			self.DialPoliciesCount++
		} else {
			self.DialPoliciesCount--
		}
		if index > self.dialPoliciesIndex {
			self.dialPoliciesIndex = index
		}
	} else if servicePolicy.IsBind() {
		if add {
			self.BindPoliciesCount++
		} else {
			self.BindPoliciesCount--
		}

		if index > self.bindPoliciesIndex {
			self.bindPoliciesIndex = index
		}
	}
}

func (self *serviceAccess) updatePostureChecksIndex(servicePolicy *ServicePolicy, index uint64) {
	if servicePolicy.IsDial() {
		if index > self.dialPoliciesIndex {
			self.dialPoliciesIndex = index
		}
	} else if servicePolicy.IsBind() {
		if index > self.bindPoliciesIndex {
			self.bindPoliciesIndex = index
		}
	}
}

func (self *serviceAccess) IsDialAllowed() bool {
	return self.DialPoliciesCount > 0
}

func (self *serviceAccess) IsBindAllowed() bool {
	return self.BindPoliciesCount > 0
}

func (self *serviceAccess) HasAccess() bool {
	return self.IsDialAllowed() || self.IsBindAllowed()
}

func (self *serviceAccess) ToMap() map[string]interface{} {
	result := map[string]interface{}{}

	result["dialPoliciesCount"] = self.DialPoliciesCount
	result["bindPoliciesCount"] = self.BindPoliciesCount
	result["dialPoliciesIndex"] = self.dialPoliciesIndex
	result["bindPoliciesIndex"] = self.bindPoliciesIndex

	return result
}

type DataStateIdentity = edge_ctrl_pb.DataState_Identity

// Identity represents an identity in the router data model. An identity is an entity that can access services
// in the network. It contains hosting configuration for services, application-specific data, service configurations,
// and tracking information for service policies, service access, and posture checks. The identity tracks which
// service policies apply to it and maintains denormalized service access information when tracking is enabled.
type Identity struct {
	Id                        string
	Name                      string
	DefaultHostingPrecedence  edge_ctrl_pb.TerminatorPrecedence
	DefaultHostingCost        uint32
	ServiceHostingPrecedences map[string]edge_ctrl_pb.TerminatorPrecedence
	ServiceHostingCosts       map[string]uint32
	AppDataJson               []byte
	ServiceConfigs            map[string]*edge_ctrl_pb.DataState_ServiceConfigs
	Disabled                  bool

	lock            sync.Mutex
	ServicePolicies map[string]struct{}
	ServiceAccess   map[string]*serviceAccess
	PostureChecks   map[string]int32
	identityIndex   uint64

	serviceAccessTrackingEnabled atomic.Bool
}

func (self *Identity) GetId() string {
	return self.Id
}

func (self *Identity) ToProtobuf() *edge_ctrl_pb.DataState_Identity {
	return &edge_ctrl_pb.DataState_Identity{
		Id:                        self.Id,
		Name:                      self.Name,
		DefaultHostingPrecedence:  self.DefaultHostingPrecedence,
		DefaultHostingCost:        self.DefaultHostingCost,
		ServiceHostingPrecedences: self.ServiceHostingPrecedences,
		ServiceHostingCosts:       self.ServiceHostingCosts,
		AppDataJson:               self.AppDataJson,
		ServiceConfigs:            self.ServiceConfigs,
		Disabled:                  self.Disabled,
	}
}

func (self *Identity) getReferencedConfigs() map[string]struct{} {
	if len(self.ServiceConfigs) == 0 {
		return nil
	}

	configs := map[string]struct{}{}

	for _, svcConfigs := range self.ServiceConfigs {
		for _, configId := range svcConfigs.Configs {
			configs[configId] = struct{}{}
		}
	}

	return configs
}

func (x *Identity) GetServiceConfigsAsMap() map[string]map[string]string {
	if x.ServiceConfigs == nil {
		return nil
	}

	result := map[string]map[string]string{}
	for serviceId, configs := range x.ServiceConfigs {
		m := map[string]string{}
		for configType, configId := range configs.Configs {
			m[configType] = configId
		}
		result[serviceId] = m
	}

	return result
}

func (self *Identity) IterateServicePolicies(f func(servicePolicyId string)) {
	self.lock.Lock()
	defer self.lock.Unlock()
	for servicePolicyId := range self.ServicePolicies {
		f(servicePolicyId)
	}
}

func (self *Identity) WithLock(f func()) {
	self.lock.Lock()
	defer self.lock.Unlock()
	f()
}

func (self *Identity) NotifyServiceChange(rdm *RouterDataModel) {
	rdm.queueSyncCheck(self.Id)
}

func (self *Identity) updateServiceCount(policy *ServicePolicy, serviceId string, index uint64, add bool) {
	current, present := self.ServiceAccess[serviceId]
	if current == nil {
		current = &serviceAccess{}
	}
	current.updateServicePolicies(policy, index, add)
	if current.HasAccess() {
		if !present {
			self.ServiceAccess[serviceId] = current
		}
	} else if present {
		delete(self.ServiceAccess, serviceId)
	}
}

func (self *Identity) decrementUseCount(m map[string]int32, id string) {
	currentValue, ok := m[id]
	if ok {
		currentValue--
		if currentValue < 1 {
			delete(m, id)
		} else {
			m[id] = currentValue
		}
	}
}

func (self *Identity) addedToPolicy(policy *ServicePolicy, index uint64) {
	if _, ok := self.ServicePolicies[policy.Id]; !ok {
		self.ServicePolicies[policy.Id] = struct{}{}
		if self.serviceAccessTrackingEnabled.Load() {
			policy.Services.IterCb(func(serviceId string, _ struct{}) {
				self.updateServiceCount(policy, serviceId, index, true)
			})
		}

		if !policy.Services.IsEmpty() {
			policy.PostureChecks.IterCb(func(key string, _ struct{}) {
				self.PostureChecks[key]++
			})
		}
	}
}

func (self *Identity) removedFromPolicy(policy *ServicePolicy, index uint64) {
	if _, ok := self.ServicePolicies[policy.Id]; ok {
		delete(self.ServicePolicies, policy.Id)

		if self.serviceAccessTrackingEnabled.Load() {
			policy.Services.IterCb(func(serviceId string, _ struct{}) {
				self.updateServiceCount(policy, serviceId, index, false)
			})
		}

		if !policy.Services.IsEmpty() {
			policy.PostureChecks.IterCb(func(postureCheckId string, _ struct{}) {
				self.decrementUseCount(self.PostureChecks, postureCheckId)
			})
		}
	}
}

func (self *Identity) servicesAddedToPolicy(policy *ServicePolicy, serviceIds []string, index uint64) {
	if self.serviceAccessTrackingEnabled.Load() {
		if _, ok := self.ServicePolicies[policy.Id]; ok {
			for _, serviceId := range serviceIds {
				self.updateServiceCount(policy, serviceId, index, true)
			}
		}
	}
}

func (self *Identity) servicesRemovedFromPolicy(policy *ServicePolicy, serviceIds []string, index uint64) {
	if self.serviceAccessTrackingEnabled.Load() {
		if _, ok := self.ServicePolicies[policy.Id]; ok {
			for _, serviceId := range serviceIds {
				self.updateServiceCount(policy, serviceId, index, false)
			}
		}
	}
}

func (self *Identity) postureChecksAddedToPolicy(policy *ServicePolicy, postureCheckIds []string, index uint64) {
	if _, ok := self.ServicePolicies[policy.Id]; ok {
		for _, postureCheckId := range postureCheckIds {
			self.PostureChecks[postureCheckId]++
		}
	}

	if self.serviceAccessTrackingEnabled.Load() {
		policy.Services.IterCb(func(serviceId string, _ struct{}) {
			access := self.ServiceAccess[serviceId]
			access.updatePostureChecksIndex(policy, index)
		})
	}
}

func (self *Identity) postureChecksRemovedFromPolicy(policy *ServicePolicy, postureCheckIds []string, index uint64) {
	if _, ok := self.ServicePolicies[policy.Id]; ok {
		for _, postureCheckId := range postureCheckIds {
			self.decrementUseCount(self.PostureChecks, postureCheckId)
		}
	}

	if self.serviceAccessTrackingEnabled.Load() {
		policy.Services.IterCb(func(serviceId string, _ struct{}) {
			access := self.ServiceAccess[serviceId]
			access.updatePostureChecksIndex(policy, index)
		})
	}
}

func (self *Identity) Equals(other *Identity) bool {
	log := pfxlog.Logger().WithField("identity", self.identityIndex)
	if self.Disabled != other.Disabled {
		log.Debug("identity updated, disabled flag changed")
		return false
	}

	if self.Name != other.Name {
		log.Debug("identity updated, name changed")
		return false
	}

	if string(self.AppDataJson) != string(other.AppDataJson) {
		log.Debug("identity updated, appDataJson changed")
		return false
	}

	if self.DefaultHostingPrecedence != other.DefaultHostingPrecedence {
		log.Debug("identity updated, default hosting precedence changed")
		return false
	}

	if self.DefaultHostingCost != other.DefaultHostingCost {
		log.Debug("identity updated, default hosting host changed")
		return false
	}

	if len(self.ServiceHostingPrecedences) != len(other.ServiceHostingPrecedences) {
		log.Debug("identity updated, number of service hosting precedences changed")
		return false
	}

	if len(self.ServiceHostingCosts) != len(other.ServiceHostingCosts) {
		log.Debug("identity updated, number of service hosting costs changed")
		return false
	}

	if len(self.ServiceConfigs) != len(other.ServiceConfigs) {
		log.Debug("identity updated, number of service configs changed")
		return false
	}

	for k, v := range self.ServiceHostingPrecedences {
		v2, ok := other.ServiceHostingPrecedences[k]
		if !ok || v != v2 {
			log.Debug("identity updated, a service hosting precedence changed")
			return false
		}
	}

	for k, v := range self.ServiceHostingCosts {
		v2, ok := other.ServiceHostingCosts[k]
		if !ok || v != v2 {
			log.Debug("identity updated, a service hosting cost changed")
			return false
		}
	}

	for k, v := range self.ServiceConfigs {
		v2, ok := other.ServiceConfigs[k]
		if !ok || !maps.Equal(v.Configs, v2.Configs) {
			log.Debug("identity updated, a service config changed")
			return false
		}
	}

	return true
}

type DataStateConfigType = edge_ctrl_pb.DataState_ConfigType

// ConfigType represents a configuration type that defines the schema or category for configuration data.
// Config instances reference a ConfigType to indicate what kind of configuration they contain.
type ConfigType struct {
	Id    string
	Name  string
	index uint64
}

func (self *ConfigType) GetId() string {
	return self.Id
}

func (self *ConfigType) ToProtobuf() *edge_ctrl_pb.DataState_ConfigType {
	return &edge_ctrl_pb.DataState_ConfigType{
		Id:   self.Id,
		Name: self.Name,
	}
}

func (self *ConfigType) Equals(other *ConfigType) bool {
	return self.Name == other.Name
}

type DataStateConfig = edge_ctrl_pb.DataState_Config

// Config represents a configuration instance that contains JSON data of a specific ConfigType.
// Configs can be associated with services and identities to provide application-specific settings.
// The struct tracks which services and identities reference this configuration for efficient updates.
type Config struct {
	Id         string
	TypeId     string
	Name       string
	DataJson   string
	services   cmap.ConcurrentMap[string, struct{}]
	identities cmap.ConcurrentMap[string, struct{}]
	index      uint64
}

func (self *Config) GetId() string {
	return self.Id
}

func (self *Config) ToProtobuf() *edge_ctrl_pb.DataState_Config {
	return &edge_ctrl_pb.DataState_Config{
		Id:       self.Id,
		TypeId:   self.TypeId,
		Name:     self.Name,
		DataJson: self.DataJson,
	}
}

func (self *Config) Equals(other *Config) bool {
	if other == nil {
		return self == nil
	}

	if self.Id != other.Id {
		return false
	}

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

// Service represents a network service that identities can dial or bind to. A service defines
// the network resource being accessed and can require encryption, have associated configurations,
// and be governed by service policies that control which identities have access.
type Service struct {
	Id                 string
	Name               string
	EncryptionRequired bool
	Configs            []string
	servicePolicies    cmap.ConcurrentMap[string, struct{}]
	index              uint64
}

func (self *Service) GetId() string {
	return self.Id
}

func (self *Service) ToProtobuf() *edge_ctrl_pb.DataState_Service {
	return &edge_ctrl_pb.DataState_Service{
		Id:                 self.Id,
		Name:               self.Name,
		EncryptionRequired: self.EncryptionRequired,
		Configs:            self.Configs,
	}
}

func (self *Service) GetIndex() uint64 {
	return self.index
}

type DataStatePostureCheck = edge_ctrl_pb.DataState_PostureCheck

// PostureCheck represents a security posture requirement that must be satisfied before an identity
// can access a service. Posture checks can verify things like OS version, running processes, domain
// membership, MFA status, and other security attributes. They are associated with service policies
// to enforce security requirements.
type PostureCheck struct {
	*DataStatePostureCheck
	servicePolicies cmap.ConcurrentMap[string, struct{}]
	index           uint64
}

type DataStateServicePolicy = edge_ctrl_pb.DataState_ServicePolicy

// ServicePolicy defines the authorization rules that control which identities can access which services.
// A policy can be either a dial policy (allowing identities to connect to services) or a bind policy
// (allowing identities to host services). Policies can include posture checks that must be satisfied
// for access to be granted. The struct tracks the sets of identities, services, and posture checks
// associated with the policy, along with indices for tracking incremental updates.
type ServicePolicy struct {
	Id                 string
	Name               string
	PolicyType         edge_ctrl_pb.PolicyType
	Services           cmap.ConcurrentMap[string, struct{}]
	PostureChecks      cmap.ConcurrentMap[string, struct{}]
	Identities         cmap.ConcurrentMap[string, struct{}]
	servicesIndex      uint64
	postureChecksIndex uint64
}

func (self *ServicePolicy) GetId() string {
	return self.Id
}

func (self *ServicePolicy) ToProtobuf() *edge_ctrl_pb.DataState_ServicePolicy {
	return &edge_ctrl_pb.DataState_ServicePolicy{
		Id:         self.Id,
		Name:       self.Name,
		PolicyType: self.PolicyType,
	}
}

func (self *ServicePolicy) IsDial() bool {
	return self.PolicyType == edge_ctrl_pb.PolicyType_DialPolicy
}

func (self *ServicePolicy) IsBind() bool {
	return self.PolicyType == edge_ctrl_pb.PolicyType_BindPolicy
}

// RouterDataModel represents a sub-set of a controller's data model. Enough to validate an identities access to dial/bind
// a service through policies and posture checks. RouterDataModel can operate in two modes: sender (controller) and
// receiver (router). Sender mode allows a controller support an event cache that supports replays for routers connecting
// for the first time/after disconnects. Receive mode does not maintain an event cache and does not support replays.
// It instead is used as a reference data structure for authorization computations.
type RouterDataModel struct {
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

	lastSaveIndex *uint64

	subscriptions            cmap.ConcurrentMap[string, *IdentitySubscription]
	updatedIdentities        cmap.ConcurrentMap[string, byte]
	subscriptionUpdateNotify chan struct{}

	events         chan subscriberEvent
	scanSubsNotify chan struct{}

	closeNotify <-chan struct{}
	stopNotify  chan struct{}
	stopped     atomic.Bool

	// timelineId identifies the database that events are flowing from. This will be reset whenever we change the
	// underlying datastore
	timelineId string

	lock  sync.Mutex
	index uint64
}

// NewBareRouterDataModel creates a new RouterDataModel that is expected to have no buffers, listeners or subscriptions
func NewBareRouterDataModel() *RouterDataModel {
	return &RouterDataModel{
		ConfigTypes:       cmap.New[*ConfigType](),
		Configs:           cmap.New[*Config](),
		Identities:        cmap.New[*Identity](),
		Services:          cmap.New[*Service](),
		ServicePolicies:   cmap.New[*ServicePolicy](),
		PostureChecks:     cmap.New[*PostureCheck](),
		PublicKeys:        cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:       cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		terminatorIdCache: cmap.New[string](),
		subscriptions:     cmap.New[*IdentitySubscription](),
	}
}

// NewReceiverRouterDataModel creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModel(closeNotify <-chan struct{}) *RouterDataModel {
	result := &RouterDataModel{
		ConfigTypes:              cmap.New[*ConfigType](),
		Configs:                  cmap.New[*Config](),
		Identities:               cmap.New[*Identity](),
		Services:                 cmap.New[*Service](),
		ServicePolicies:          cmap.New[*ServicePolicy](),
		PostureChecks:            cmap.New[*PostureCheck](),
		PublicKeys:               cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:              cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		subscriptions:            cmap.New[*IdentitySubscription](),
		updatedIdentities:        cmap.New[byte](),
		subscriptionUpdateNotify: make(chan struct{}, 1),
		events:                   make(chan subscriberEvent, 250),
		scanSubsNotify:           make(chan struct{}, 1),
		closeNotify:              closeNotify,
		stopNotify:               make(chan struct{}),
		terminatorIdCache:        cmap.New[string](),
	}
	go result.processSubscriberEvents()
	return result
}

// NewReceiverRouterDataModelFromDataState creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromDataState(dataState *edge_ctrl_pb.DataState, closeNotify <-chan struct{}) *RouterDataModel {
	result := &RouterDataModel{
		ConfigTypes:              cmap.New[*ConfigType](),
		Configs:                  cmap.New[*Config](),
		Identities:               cmap.New[*Identity](),
		Services:                 cmap.New[*Service](),
		ServicePolicies:          cmap.New[*ServicePolicy](),
		PostureChecks:            cmap.New[*PostureCheck](),
		PublicKeys:               cmap.New[*edge_ctrl_pb.DataState_PublicKey](),
		Revocations:              cmap.New[*edge_ctrl_pb.DataState_Revocation](),
		subscriptions:            cmap.New[*IdentitySubscription](),
		updatedIdentities:        cmap.New[byte](),
		subscriptionUpdateNotify: make(chan struct{}, 1),
		events:                   make(chan subscriberEvent, 250),
		scanSubsNotify:           make(chan struct{}, 1),
		closeNotify:              closeNotify,
		stopNotify:               make(chan struct{}),
		timelineId:               dataState.TimelineId,
		terminatorIdCache:        cmap.New[string](),
	}

	if tIdCache, ok := dataState.Caches[edge_ctrl_pb.CacheType_TerminatorIds.String()]; ok && tIdCache != nil && tIdCache.Data != nil {
		for k, v := range tIdCache.Data {
			result.terminatorIdCache.Set(k, string(v))
		}
	}

	go result.processSubscriberEvents()

	result.WhileLocked(func(u uint64) {
		for _, event := range dataState.Events {
			result.Handle(dataState.EndIndex, event)
		}
		result.SetCurrentIndex(dataState.EndIndex)
	})

	return result
}

// NewReceiverRouterDataModelFromExisting creates a new RouterDataModel that does not store events. listenerBufferSize affects the
// buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromExisting(existing *RouterDataModel, closeNotify <-chan struct{}) *RouterDataModel {
	result := &RouterDataModel{
		ConfigTypes:              existing.ConfigTypes,
		Configs:                  existing.Configs,
		Identities:               existing.Identities,
		Services:                 existing.Services,
		ServicePolicies:          existing.ServicePolicies,
		PostureChecks:            existing.PostureChecks,
		PublicKeys:               existing.PublicKeys,
		cachedPublicKeys:         existing.cachedPublicKeys,
		Revocations:              existing.Revocations,
		subscriptions:            cmap.New[*IdentitySubscription](),
		updatedIdentities:        cmap.New[byte](),
		subscriptionUpdateNotify: make(chan struct{}, 1),
		events:                   make(chan subscriberEvent, 250),
		scanSubsNotify:           make(chan struct{}, 1),
		closeNotify:              closeNotify,
		stopNotify:               make(chan struct{}),
		timelineId:               existing.timelineId,
		terminatorIdCache:        existing.terminatorIdCache,
	}
	currentIndex := existing.CurrentIndex()
	result.SetCurrentIndex(currentIndex)
	go result.processSubscriberEvents()
	return result
}

// NewReceiverRouterDataModelFromFile creates a new RouterDataModel that does not store events and is initialized from
// a file backup. listenerBufferSize affects the buffer size of channels returned to listeners of the data model.
func NewReceiverRouterDataModelFromFile(path string, closeNotify <-chan struct{}) (*RouterDataModel, error) {
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

	rdm := NewReceiverRouterDataModelFromDataState(state, closeNotify)
	rdm.lastSaveIndex = &state.EndIndex

	return rdm, nil
}

func (rdm *RouterDataModel) WhileLocked(callback func(uint64)) {
	rdm.lock.Lock()
	defer rdm.lock.Unlock()

	callback(rdm.CurrentIndex())
}

func (rdm *RouterDataModel) SetCurrentIndex(index uint64) {
	atomic.StoreUint64(&rdm.index, index)
}

func (rdm *RouterDataModel) CurrentIndex() uint64 {
	return atomic.LoadUint64(&rdm.index)
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
		case <-rdm.scanSubsNotify:
			start := time.Now()
			count := rdm.updatedIdentities.Count()
			for entry := range rdm.updatedIdentities.IterBuffered() {
				rdm.updatedIdentities.Remove(entry.Key) // do all checks, so we can just remove the flag entirely

				if entry.Val&IdentityDeleted == IdentityDeleted {
					rdm.checkSubsForDeletedIdentity(entry.Key, false)
					continue
				}

				if entry.Val&IdentityAdded == IdentityAdded {
					rdm.checkSubsForNewIdentity(entry.Key, false)
				}

				if entry.Val&IdentityUpdated == IdentityUpdated {
					rdm.syncSubscriptionIfRequired(entry.Key, false)
				}
			}
			pfxlog.Logger().
				WithField("processed", count).
				WithField("subs", rdm.subscriptions.Count()).
				Debugf("sync all pending subscribers in %v", time.Since(start))
		}
	}
}

func (rdm *RouterDataModel) Stop() {
	if rdm.stopped.CompareAndSwap(false, true) {
		close(rdm.stopNotify)
	}
}

func (rdm *RouterDataModel) GetTimelineId() string {
	return rdm.timelineId
}

// ApplyChangeSet applies the given even to the router data model.
func (rdm *RouterDataModel) ApplyChangeSet(change *edge_ctrl_pb.DataState_ChangeSet) {
	logger := pfxlog.Logger().
		WithField("index", change.Index).
		WithField("synthetic", change.IsSynthetic).
		WithField("entries", len(change.Changes))

	err := rdm.Store(change, func(index uint64, change *edge_ctrl_pb.DataState_ChangeSet) {
		for idx, event := range change.Changes {
			logger.
				WithField("entry", idx).
				WithField("action", event.Action.String()).
				WithField("type", fmt.Sprintf("%T", event.Model)).
				WithField("summary", event.Summarize()).
				Debug("handling change set entry")
			rdm.Handle(index, event)
		}
	})

	if err != nil {
		if len(change.Changes) > 0 {
			logger = logger.
				WithField("action", change.Changes[0].Action.String()).
				WithField("type", fmt.Sprintf("%T", change.Changes[0].Model))
		}

		logger.WithError(err).Error("could not apply change set")
		return
	}
}

func (rdm *RouterDataModel) Store(event *edge_ctrl_pb.DataState_ChangeSet, onSuccess OnStoreSuccess) error {
	rdm.lock.Lock()
	defer rdm.lock.Unlock()

	// Synthetic events are not backed by any kind of data store that provides and index. They are not stored and
	// trigger the on success callback immediately.
	if event.IsSynthetic {
		onSuccess(event.Index, event)
		return nil
	}

	if rdm.index > 0 && rdm.index >= event.Index {
		return fmt.Errorf("out of order event detected, currentIndex: %d, receivedIndex: %d, type :%T", rdm.index, event.Index, rdm)
	}

	rdm.index = event.Index

	if onSuccess != nil {
		onSuccess(rdm.index, event)
	}

	return nil
}

func (rdm *RouterDataModel) Handle(index uint64, event *edge_ctrl_pb.DataState_Event) {
	switch typedModel := event.Model.(type) {
	case *edge_ctrl_pb.DataState_Event_ConfigType:
		rdm.HandleConfigTypeEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Config:
		rdm.HandleConfigEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Identity:
		rdm.HandleIdentityEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Service:
		rdm.HandleServiceEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_ServicePolicy:
		rdm.HandleServicePolicyEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PostureCheck:
		rdm.HandlePostureCheckEvent(index, event, typedModel)
	case *edge_ctrl_pb.DataState_Event_PublicKey:
		rdm.HandlePublicKeyEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_Revocation:
		rdm.HandleRevocationEvent(event, typedModel)
	case *edge_ctrl_pb.DataState_Event_ServicePolicyChange:
		rdm.HandleServicePolicyChange(index, typedModel.ServicePolicyChange)
	}
}

func (rdm *RouterDataModel) queueEvent(event subscriberEvent) {
	if rdm.events != nil {
		rdm.events <- event
	}
}

func (rdm *RouterDataModel) SyncAllSubscribers() {
	rdm.queueEvent(syncAllSubscribersEvent{})
}

func (rdm *RouterDataModel) markIdentityForCheck(identityId string, checkType byte) {
	rdm.updatedIdentities.Upsert(identityId, 0, func(exist bool, valueInMap byte, newValue byte) byte {
		return valueInMap | checkType
	})
}

func (rdm *RouterDataModel) markIdentityCheckComplete(identityId string, checkType byte) bool {
	flagWasSet := false
	rdm.updatedIdentities.Upsert(identityId, 0, func(exist bool, valueInMap byte, newValue byte) byte {
		flagWasSet = valueInMap&checkType != 0
		return valueInMap &^ checkType
	})

	rdm.updatedIdentities.RemoveCb(identityId, func(key string, v byte, exists bool) bool {
		return exists && v == 0
	})

	return flagWasSet
}

// HandleIdentityEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyIdentityEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleIdentityEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Identity) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		var cleanupActions []*edge_ctrl_pb.DataState_Event

		rdm.withLockedIdentity(model.Identity.Id, func(identity *Identity) {
			rdm.removeConfigReferences(identity)

			for servicePolicyId := range identity.ServicePolicies {
				// If a service is deleted, the service policy changes should come in first, so this should always be a no-op
				if servicePolicy, _ := rdm.ServicePolicies.Get(servicePolicyId); servicePolicy != nil {
					if servicePolicy.Identities.Has(model.Identity.Id) {
						cleanupActions = append(cleanupActions, &edge_ctrl_pb.DataState_Event{
							Action:      edge_ctrl_pb.DataState_Create,
							IsSynthetic: false,
							Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
								ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
									PolicyId:          servicePolicyId,
									RelatedEntityIds:  []string{model.Identity.Id},
									RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
									Add:               false,
								},
							},
						})
					}
				}
			}
		})

		for _, cleanupAction := range cleanupActions {
			rdm.Handle(index, cleanupAction)
		}

		rdm.Identities.Remove(model.Identity.Id)
		rdm.queueIdentityDeletedSubCheck(model.Identity.Id)

	} else {
		identity := &Identity{
			Id:                        model.Identity.Id,
			Name:                      model.Identity.Name,
			DefaultHostingPrecedence:  model.Identity.DefaultHostingPrecedence,
			DefaultHostingCost:        model.Identity.DefaultHostingCost,
			ServiceHostingPrecedences: model.Identity.ServiceHostingPrecedences,
			ServiceHostingCosts:       model.Identity.ServiceHostingCosts,
			AppDataJson:               model.Identity.AppDataJson,
			ServiceConfigs:            model.Identity.ServiceConfigs,
			Disabled:                  model.Identity.Disabled,
			identityIndex:             index,
		}

		rdm.Identities.Upsert(model.Identity.Id, nil, func(exist bool, valueInMap *Identity, newValue *Identity) *Identity {
			if valueInMap == nil {
				identity.ServicePolicies = map[string]struct{}{}
				identity.ServiceAccess = map[string]*serviceAccess{}
				identity.PostureChecks = map[string]int32{}
			} else {
				rdm.removeConfigReferences(valueInMap)

				identity.ServicePolicies = valueInMap.ServicePolicies
				identity.ServiceAccess = valueInMap.ServiceAccess
				identity.PostureChecks = valueInMap.PostureChecks
				identity.serviceAccessTrackingEnabled.Store(valueInMap.serviceAccessTrackingEnabled.Load())
			}

			rdm.addConfigReferences(identity)

			return identity
		})

		if event.Action == edge_ctrl_pb.DataState_Create {
			rdm.queueNewIdentitySubCheck(identity.Id)
		} else if event.Action == edge_ctrl_pb.DataState_Update {
			rdm.queueSyncCheck(identity.Id)
		}
	}
}

func (rdm *RouterDataModel) removeConfigReferences(identity *Identity) {
	configs := identity.getReferencedConfigs()

	for configId := range configs {
		if config, _ := rdm.Configs.Get(configId); config != nil {
			config.identities.Remove(identity.Id)
		}
	}
}

func (rdm *RouterDataModel) addConfigReferences(identity *Identity) {
	configs := identity.getReferencedConfigs()

	for configId := range configs {
		if config, _ := rdm.Configs.Get(configId); config != nil {
			config.identities.Set(identity.Id, struct{}{})
		} else {
			pfxlog.Logger().
				WithField("configId", configId).
				WithField("identityId", identity.Id).
				Error("config not found when adding config references to identity")
		}
	}
}

// HandleServiceEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServiceEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServiceEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Service) {
	removeFromConfigs := func(configIds []string) {
		for _, configId := range configIds {
			if config, _ := rdm.Configs.Get(configId); config != nil {
				config.services.Remove(model.Service.Id)
			}
		}
	}

	addToConfigs := func(configIds []string) {
		for _, configId := range configIds {
			if config, _ := rdm.Configs.Get(configId); config != nil {
				config.services.Set(model.Service.Id, struct{}{})
			} else {
				pfxlog.Logger().
					WithField("configId", configId).
					WithField("serviceId", model.Service.Id).
					Error("config not found when adding config references to service")
			}
		}
	}

	if event.Action == edge_ctrl_pb.DataState_Delete {
		var cleanupActions []*edge_ctrl_pb.DataState_Event
		rdm.Services.RemoveCb(model.Service.Id, func(key string, v *Service, exists bool) bool {
			if exists {
				removeFromConfigs(v.Configs)

				v.servicePolicies.IterCb(func(servicePolicyId string, _ struct{}) {
					// If a service is deleted, the service policy changes should come in first, so this should always be a no-op
					if servicePolicy, _ := rdm.ServicePolicies.Get(servicePolicyId); servicePolicy != nil {
						if servicePolicy.Services.Has(model.Service.Id) {
							cleanupActions = append(cleanupActions, &edge_ctrl_pb.DataState_Event{
								Action:      edge_ctrl_pb.DataState_Create,
								IsSynthetic: false,
								Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
									ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
										PolicyId:          servicePolicyId,
										RelatedEntityIds:  []string{model.Service.Id},
										RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
										Add:               false,
									},
								},
							})
						}
					}
				})
			}
			return exists
		})

		for _, cleanupAction := range cleanupActions {
			rdm.Handle(index, cleanupAction)
		}
	} else {
		updatedService := &Service{
			Id:                 model.Service.Id,
			Name:               model.Service.Name,
			EncryptionRequired: model.Service.EncryptionRequired,
			Configs:            model.Service.Configs,
			index:              index,
		}

		rdm.Services.Upsert(model.Service.Id, updatedService, func(exist bool, valueInMap *Service, newValue *Service) *Service {
			var configsToRemove []string
			var configsToAdd []string

			if !exist {
				configsToAdd = newValue.Configs
			} else {
				configsToRemove, configsToAdd = diffStringSlices(valueInMap.Configs, newValue.Configs)
			}

			removeFromConfigs(configsToRemove)
			addToConfigs(configsToAdd)

			if valueInMap != nil {
				updatedService.servicePolicies = valueInMap.servicePolicies
			} else {
				updatedService.servicePolicies = cmap.New[struct{}]()
			}

			return newValue
		})

		updatedService.servicePolicies.IterCb(func(servicePolicyId string, _ struct{}) {
			rdm.NotifyServicePolicyServiceChange(servicePolicyId, index)
		})
	}
}

func diffStringSlices(slice1, slice2 []string) (onlyInFirst, onlyInSecond []string) {
	// Sort both slices
	sort.Strings(slice1)
	sort.Strings(slice2)

	i, j := 0, 0

	for i < len(slice1) && j < len(slice2) {
		if slice1[i] < slice2[j] {
			// Element only in first slice
			onlyInFirst = append(onlyInFirst, slice1[i])
			i++
		} else if slice1[i] > slice2[j] {
			// Element only in second slice
			onlyInSecond = append(onlyInSecond, slice2[j])
			j++
		} else {
			// Element in both slices, skip
			i++
			j++
		}
	}

	// Add remaining elements from first slice
	for i < len(slice1) {
		onlyInFirst = append(onlyInFirst, slice1[i])
		i++
	}

	// Add remaining elements from second slice
	for j < len(slice2) {
		onlyInSecond = append(onlyInSecond, slice2[j])
		j++
	}

	return onlyInFirst, onlyInSecond
}

// HandleConfigTypeEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyConfigTypeEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleConfigTypeEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ConfigType) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.ConfigTypes.Remove(model.ConfigType.Id)
	} else {
		rdm.ConfigTypes.Set(model.ConfigType.Id, &ConfigType{
			Id:    model.ConfigType.Id,
			Name:  model.ConfigType.Name,
			index: index,
		})

		rdm.Configs.IterCb(func(configId string, config *Config) {
			if config.TypeId == model.ConfigType.Id {
				config.services.IterCb(func(serviceId string, _ struct{}) {
					rdm.NotifyServiceOfConfigChange(serviceId, index)
				})
				config.identities.IterCb(func(identityId string, _ struct{}) {
					rdm.NotifyIdentityChange(identityId, index)
				})
			}
		})
	}
}

// HandleConfigEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyConfigEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleConfigEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_Config) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		rdm.Configs.RemoveCb(model.Config.Id, func(key string, v *Config, exists bool) bool {
			if v != nil {
				v.services.IterCb(func(serviceId string, _ struct{}) {
					rdm.NotifyServiceOfConfigChange(serviceId, index)
				})
				v.identities.IterCb(func(identityId string, _ struct{}) {
					rdm.withLockedIdentity(identityId, func(identity *Identity) {
						for serviceId, configData := range identity.ServiceConfigs {
							for configTypeId, configId := range configData.Configs {
								if configId == v.Id {
									delete(configData.Configs, configTypeId)
								}
							}
							if len(configData.Configs) == 0 {
								delete(identity.ServiceConfigs, serviceId)
							}
						}
					})
					rdm.NotifyIdentityChange(identityId, index)
				})
			}
			return exists
		})
	} else {
		rdm.Configs.Upsert(model.Config.Id, nil, func(exist bool, valueInMap *Config, newValue *Config) *Config {
			result := &Config{
				Id:         model.Config.Id,
				TypeId:     model.Config.TypeId,
				Name:       model.Config.Name,
				DataJson:   model.Config.DataJson,
				services:   cmap.New[struct{}](),
				identities: cmap.New[struct{}](),
				index:      index,
			}

			if valueInMap != nil {
				result.services = valueInMap.services
				result.identities = valueInMap.identities
			}

			if !result.Equals(valueInMap) {
				result.services.IterCb(func(serviceId string, _ struct{}) {
					rdm.NotifyServiceOfConfigChange(serviceId, index)
				})

				result.identities.IterCb(func(key string, _ struct{}) {
					if identity, _ := rdm.Identities.Get(key); identity != nil {
						identity.NotifyServiceChange(rdm)
					}
				})
			}

			return result
		})
	}
}

func (rdm *RouterDataModel) applyUpdateServicePolicyEvent(index uint64, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	servicePolicy := model.ServicePolicy
	rdm.ServicePolicies.Upsert(servicePolicy.Id, nil, func(exist bool, valueInMap *ServicePolicy, newValue *ServicePolicy) *ServicePolicy {
		if valueInMap == nil {
			return &ServicePolicy{
				Id:                 servicePolicy.Id,
				Name:               servicePolicy.Name,
				PolicyType:         servicePolicy.PolicyType,
				Services:           cmap.New[struct{}](),
				PostureChecks:      cmap.New[struct{}](),
				Identities:         cmap.New[struct{}](),
				servicesIndex:      index,
				postureChecksIndex: 0,
			}
		} else {
			updatedValue := &ServicePolicy{
				Id:            servicePolicy.Id,
				Name:          servicePolicy.Name,
				PolicyType:    servicePolicy.PolicyType,
				Services:      valueInMap.Services,
				PostureChecks: valueInMap.PostureChecks,
				Identities:    valueInMap.Identities,
				servicesIndex: index,
			}

			if valueInMap.PolicyType != servicePolicy.PolicyType {
				valueInMap.Identities.IterCb(func(key string, _ struct{}) {
					rdm.withLockedIdentity(key, func(identity *Identity) {
						identity.removedFromPolicy(valueInMap, index)
						identity.addedToPolicy(updatedValue, index)
					})
				})
			}

			return updatedValue
		}
	})
}

func (rdm *RouterDataModel) applyDeleteServicePolicyEvent(index uint64, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	rdm.ServicePolicies.RemoveCb(model.ServicePolicy.Id, func(key string, v *ServicePolicy, exists bool) bool {
		if v != nil {
			v.Identities.IterCb(func(identityId string, _ struct{}) {
				rdm.withLockedIdentity(identityId, func(identity *Identity) {
					identity.removedFromPolicy(v, index)
				})
				rdm.queueSyncCheck(identityId)
			})
		}
		return exists
	})
}

// HandleServicePolicyEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyServicePolicyEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandleServicePolicyEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_ServicePolicy) {
	pfxlog.Logger().WithField("policyId", model.ServicePolicy.Id).WithField("action", event.Action).Debug("applying service policy event")
	switch event.Action {
	case edge_ctrl_pb.DataState_Create:
		rdm.applyUpdateServicePolicyEvent(index, model)
	case edge_ctrl_pb.DataState_Update:
		rdm.applyUpdateServicePolicyEvent(index, model)
	case edge_ctrl_pb.DataState_Delete:
		rdm.applyDeleteServicePolicyEvent(index, model)
	}
}

// HandlePostureCheckEvent will apply the delta event to the router data model. It is not restricted by index calculations.
// Use ApplyPostureCheckEvent for event logged event handling. This method is generally meant for bulk loading of data
// during startup.
func (rdm *RouterDataModel) HandlePostureCheckEvent(index uint64, event *edge_ctrl_pb.DataState_Event, model *edge_ctrl_pb.DataState_Event_PostureCheck) {
	if event.Action == edge_ctrl_pb.DataState_Delete {
		var cleanupActions []*edge_ctrl_pb.DataState_Event

		rdm.withPostureCheck(model.PostureCheck.Id, func(postureCheck *PostureCheck) {
			postureCheck.servicePolicies.IterCb(func(servicePolicyId string, _ struct{}) {
				rdm.withServicePolicy(servicePolicyId, func(servicePolicy *ServicePolicy) {
					if servicePolicy.PostureChecks.Has(model.PostureCheck.Id) {
						cleanupActions = append(cleanupActions, &edge_ctrl_pb.DataState_Event{
							Action: edge_ctrl_pb.DataState_Create,
							Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
								ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
									PolicyId:          servicePolicyId,
									RelatedEntityIds:  []string{model.PostureCheck.Id},
									RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
									Add:               false,
								},
							},
						})
					}
				})
			})
		})

		for _, cleanupAction := range cleanupActions {
			rdm.Handle(index, cleanupAction)
		}

		rdm.PostureChecks.Remove(model.PostureCheck.Id)

	} else {
		postureCheck := &PostureCheck{
			DataStatePostureCheck: model.PostureCheck,
			index:                 index,
		}

		rdm.PostureChecks.Upsert(model.PostureCheck.Id, nil, func(exist bool, valueInMap *PostureCheck, newValue *PostureCheck) *PostureCheck {
			if valueInMap != nil {
				postureCheck.servicePolicies = valueInMap.servicePolicies
			} else {
				postureCheck.servicePolicies = cmap.New[struct{}]()
			}
			return postureCheck
		})

		postureCheck.servicePolicies.IterCb(func(servicePolicyId string, _ struct{}) {
			rdm.NotifyServicePolicyPostureChecksChange(servicePolicyId, index)
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
	log := pfxlog.Logger().
		WithField("index", index).
		WithField("rdm", fmt.Sprintf("%p", rdm)).
		WithField("policyId", model.PolicyId).
		WithField("isAdd", model.Add).
		WithField("relatedEntityType", model.RelatedEntityType).
		WithField("relatedEntityIds", model.RelatedEntityIds)
	log.Debug("applying service policy change event")

	servicePolicy, _ := rdm.ServicePolicies.Get(model.PolicyId)

	if servicePolicy == nil {
		if model.Add {
			log.Error("service policy not present in router data model")
		}
		return
	}

	servicesIndex := servicePolicy.servicesIndex
	postureChecksIndex := servicePolicy.postureChecksIndex

	var additionalIdentitiesToUpdate []string

	switch model.RelatedEntityType {
	case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity:
		for _, identityId := range model.RelatedEntityIds {
			rdm.withLockedIdentity(identityId, func(identity *Identity) {
				if model.Add {
					servicePolicy.Identities.Set(identityId, struct{}{})
					identity.addedToPolicy(servicePolicy, index)
				} else {
					servicePolicy.Identities.Remove(identityId)
					identity.removedFromPolicy(servicePolicy, index)
				}
			})
		}

		if !model.Add {
			additionalIdentitiesToUpdate = model.RelatedEntityIds
		}

	case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService:
		servicesIndex = index
		servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
			rdm.withLockedIdentity(identityId, func(identity *Identity) {
				if model.Add {
					identity.servicesAddedToPolicy(servicePolicy, model.RelatedEntityIds, index)
				} else {
					identity.servicesRemovedFromPolicy(servicePolicy, model.RelatedEntityIds, index)
				}
			})
		})

		startCount := servicePolicy.Services.Count()

		if model.Add {
			for _, serviceId := range model.RelatedEntityIds {
				servicePolicy.Services.Set(serviceId, struct{}{})
			}
			rdm.withServices(model.RelatedEntityIds, func(service *Service) {
				service.servicePolicies.Set(servicePolicy.Id, struct{}{})
			})

			if startCount == 0 && servicePolicy.Services.Count() > 0 {
				servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
					rdm.withLockedIdentity(identityId, func(identity *Identity) {
						identity.postureChecksAddedToPolicy(servicePolicy, servicePolicy.PostureChecks.Keys(), index)
					})
				})
			}

		} else {
			for _, serviceId := range model.RelatedEntityIds {
				servicePolicy.Services.Remove(serviceId)
			}
			rdm.withServices(model.RelatedEntityIds, func(service *Service) {
				service.servicePolicies.Remove(servicePolicy.Id)
			})

			if startCount > 0 && servicePolicy.Services.Count() == 0 {
				servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
					rdm.withLockedIdentity(identityId, func(identity *Identity) {
						identity.postureChecksRemovedFromPolicy(servicePolicy, servicePolicy.PostureChecks.Keys(), index)
					})
				})
			}
		}
	case edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck:
		postureChecksIndex = index

		if !servicePolicy.Services.IsEmpty() {
			servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
				rdm.withLockedIdentity(identityId, func(identity *Identity) {
					if model.Add {
						identity.postureChecksAddedToPolicy(servicePolicy, model.RelatedEntityIds, index)
					} else {
						identity.postureChecksRemovedFromPolicy(servicePolicy, model.RelatedEntityIds, index)
					}
				})
			})
		}

		if model.Add {
			for _, postureCheckId := range model.RelatedEntityIds {
				servicePolicy.PostureChecks.Set(postureCheckId, struct{}{})
			}
			rdm.withPostureChecks(model.RelatedEntityIds, func(postureCheck *PostureCheck) {
				postureCheck.servicePolicies.Set(servicePolicy.Id, struct{}{})
			})
		} else {
			for _, postureCheckId := range model.RelatedEntityIds {
				servicePolicy.PostureChecks.Remove(postureCheckId)
			}
			rdm.withPostureChecks(model.RelatedEntityIds, func(postureCheck *PostureCheck) {
				postureCheck.servicePolicies.Remove(servicePolicy.Id)
			})
		}
	}

	rdm.ServicePolicies.Set(model.PolicyId, &ServicePolicy{
		Id:                 servicePolicy.Id,
		Name:               servicePolicy.Name,
		PolicyType:         servicePolicy.PolicyType,
		Services:           servicePolicy.Services,
		PostureChecks:      servicePolicy.PostureChecks,
		Identities:         servicePolicy.Identities,
		servicesIndex:      servicesIndex,
		postureChecksIndex: postureChecksIndex,
	})

	servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
		if identity, _ := rdm.Identities.Get(identityId); identity != nil {
			identity.NotifyServiceChange(rdm)
		}
	})

	for _, identityId := range additionalIdentitiesToUpdate {
		if identity, _ := rdm.Identities.Get(identityId); identity != nil {
			identity.NotifyServiceChange(rdm)
		}
	}
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
	rdm.WhileLocked(func(currentIndex uint64) {
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
				ConfigType: v.ToProtobuf(),
			},
		}
		events = append(events, newEvent)
	})

	rdm.Configs.IterCb(func(key string, v *Config) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Config{
				Config: v.ToProtobuf(),
			},
		}
		events = append(events, newEvent)
	})

	servicePolicyIdentities := map[string]*edge_ctrl_pb.DataState_ServicePolicyChange{}

	rdm.Identities.IterCb(func(key string, v *Identity) {
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Identity{
				Identity: v.ToProtobuf(),
			},
		}
		events = append(events, newEvent)

		v.IterateServicePolicies(func(policyId string) {
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
				Service: v.ToProtobuf(),
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
				ServicePolicy: v.ToProtobuf(),
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
	rdm.WhileLocked(func(index uint64) {
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
		return nil, fmt.Errorf("identity not found by id")
	}

	service, ok := rdm.Services.Get(serviceId)

	if !ok {
		return nil, fmt.Errorf("service not found by id")
	}

	var policies []*ServicePolicy

	postureChecks := map[string]*edge_ctrl_pb.DataState_PostureCheck{}

	identity.IterateServicePolicies(func(servicePolicyId string) {
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

func (rdm *RouterDataModel) withIdentity(identityId string, f func(identity *Identity)) {
	if identity, _ := rdm.Identities.Get(identityId); identity != nil {
		f(identity)
	}
}

func (rdm *RouterDataModel) withLockedIdentity(identityId string, f func(identity *Identity)) {
	if identity, _ := rdm.Identities.Get(identityId); identity != nil {
		identity.lock.Lock()
		defer identity.lock.Unlock()
		f(identity)
	}
}

func (rdm *RouterDataModel) withServices(serviceIds []string, f func(service *Service)) {
	for _, serviceId := range serviceIds {
		if service, _ := rdm.Services.Get(serviceId); service != nil {
			f(service)
		}
	}
}

func (rdm *RouterDataModel) withService(serviceId string, f func(service *Service)) {
	if service, _ := rdm.Services.Get(serviceId); service != nil {
		f(service)
	}
}

func (rdm *RouterDataModel) withServicePolicy(servicePolicyId string, f func(servicePolicy *ServicePolicy)) {
	if servicePolicy, _ := rdm.ServicePolicies.Get(servicePolicyId); servicePolicy != nil {
		f(servicePolicy)
	} else {
		pfxlog.Logger().
			WithField("servicePolicyId", servicePolicyId).
			Error("service policy not found when updating model")
	}
}

func (rdm *RouterDataModel) withPostureChecks(postureCheckIds []string, f func(PostureCheck *PostureCheck)) {
	for _, postureCheckId := range postureCheckIds {
		if postureCheck, _ := rdm.PostureChecks.Get(postureCheckId); postureCheck != nil {
			f(postureCheck)
		} else {
			pfxlog.Logger().
				WithField("postureCheckId", postureCheckId).
				Error("posture check not found when updating model")
		}
	}
}

func (rdm *RouterDataModel) withPostureCheck(postureCheckId string, f func(PostureCheck *PostureCheck)) {
	if postureCheck, _ := rdm.PostureChecks.Get(postureCheckId); postureCheck != nil {
		f(postureCheck)
	} else {
		pfxlog.Logger().
			WithField("postureCheckId", postureCheckId).
			Error("posture check not found when updating model")
	}
}

func (rdm *RouterDataModel) SubscribeToIdentityChanges(identityId string, subscriber IdentityEventSubscriber, isRouterIdentity bool) error {
	pfxlog.Logger().WithField("identityId", identityId).Debug("subscribing to changes for identity")
	identity, ok := rdm.Identities.Get(identityId)
	if !ok && !isRouterIdentity {
		return fmt.Errorf("identity %s not found", identityId)
	}

	rdm.EnableServiceAccessTracking(identityId)

	subscription := rdm.subscriptions.Upsert(identityId, nil, func(exist bool, valueInMap *IdentitySubscription, newValue *IdentitySubscription) *IdentitySubscription {
		if exist {
			valueInMap.listeners.Append(subscriber)
			return valueInMap
		}
		result := &IdentitySubscription{
			IdentityId:       identityId,
			IsRouterIdentity: isRouterIdentity,
		}
		result.listeners.Append(subscriber)
		pfxlog.Logger().WithField("identityId", identityId).Debug("added subscription for identity")
		return result
	})

	if identity != nil {
		state, _ := subscription.initialize(rdm, identity)
		subscriber.NotifyIdentityEvent(state, IdentityFullStateState)
	}

	return nil
}

func (rdm *RouterDataModel) UnsubscribeFromIdentityChanges(identityId string, subscriber IdentityEventSubscriber) {
	removed := rdm.subscriptions.RemoveCb(identityId, func(key string, v *IdentitySubscription, exists bool) bool {
		if v != nil {
			v.listeners.Delete(subscriber)
			if len(v.listeners.Value()) == 0 {
				return true
			}
		}
		return false
	})

	if removed {
		rdm.DisableServiceAccessTracking(identityId)
	}
}

func (rdm *RouterDataModel) InheritLocalData(other *RouterDataModel) {
	other.Identities.IterCb(func(identityId string, v *Identity) {
		if v.serviceAccessTrackingEnabled.Load() {
			rdm.EnableServiceAccessTracking(identityId)
		}
	})

	other.subscriptions.IterCb(func(identityId string, v *IdentitySubscription) {
		rdm.subscriptions.Set(identityId, v)
	})

	other.terminatorIdCache.IterCb(func(key string, v string) {
		rdm.terminatorIdCache.Set(key, v)
	})
}

func (rdm *RouterDataModel) buildServiceListUsingDenormalizedData(sub *IdentitySubscription) (map[string]*IdentityService, map[string]*PostureCheck) {
	log := pfxlog.Logger().WithField("identityId", sub.IdentityId)
	services := map[string]*IdentityService{}
	postureChecks := map[string]*PostureCheck{}

	rdm.withLockedIdentity(sub.IdentityId, func(identity *Identity) {
		for serviceId, access := range identity.ServiceAccess {
			if service, _ := rdm.Services.Get(serviceId); service != nil {
				identityService := &IdentityService{
					Service:           service,
					Configs:           map[string]*IdentityConfig{},
					DialAllowed:       access.IsDialAllowed(),
					BindAllowed:       access.IsBindAllowed(),
					dialPoliciesIndex: access.dialPoliciesIndex,
					bindPoliciesIndex: access.bindPoliciesIndex,
				}

				services[serviceId] = identityService
				rdm.loadServiceConfigs(sub.Identity, identityService)
			} else {
				log.WithField("serviceId", serviceId).Error("service not found by id")
			}
		}

		for postureCheckId := range identity.PostureChecks {
			if check, _ := rdm.PostureChecks.Get(postureCheckId); check != nil {
				postureChecks[postureCheckId] = check
			} else {
				log.WithField("postureCheckId", postureCheckId).Error("posture check referenced but not found by id")
			}
		}
	})

	return services, postureChecks
}

func (rdm *RouterDataModel) buildServiceList(identity *Identity) (map[string]*IdentityService, map[string]*PostureCheck) {
	log := pfxlog.Logger().WithField("identityId", identity.Id)
	services := map[string]*IdentityService{}
	postureChecks := map[string]*PostureCheck{}

	identity.IterateServicePolicies(func(policyId string) {
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
				}
				services[serviceId] = identityService
				rdm.loadServiceConfigs(identity, identityService)
			}

			if policy.IsDial() {
				identityService.DialAllowed = true
				if policy.postureChecksIndex > identityService.dialPoliciesIndex {
					identityService.dialPoliciesIndex = policy.postureChecksIndex
				}
			} else if policy.IsBind() {
				identityService.BindAllowed = true
				if policy.postureChecksIndex > identityService.bindPoliciesIndex {
					identityService.bindPoliciesIndex = policy.postureChecksIndex
				}
			}
		})

		rdm.loadServicePostureChecks(identity, policy, postureChecks)
	})

	return services, postureChecks
}

func (rdm *RouterDataModel) buildServiceAccessList(identity *Identity) map[string]*serviceAccess {
	services := map[string]*serviceAccess{}

	for servicePolicyId := range identity.ServicePolicies {
		if policy, _ := rdm.ServicePolicies.Get(servicePolicyId); policy != nil {
			policy.Services.IterCb(func(serviceId string, _ struct{}) {
				access, ok := services[serviceId]
				if !ok {
					access = &serviceAccess{}
					services[serviceId] = access
				}
				access.updateServicePolicies(policy, max(policy.postureChecksIndex, policy.servicesIndex), true)
			})
		}
	}

	return services
}

func (rdm *RouterDataModel) loadServicePostureChecks(identity *Identity, policy *ServicePolicy, checks map[string]*PostureCheck) {
	log := pfxlog.Logger().
		WithField("identityId", identity.Id).
		WithField("policyId", policy.Id)

	if !policy.Services.IsEmpty() {
		policy.PostureChecks.IterCb(func(postureCheckId string, _ struct{}) {
			if _, present := checks[postureCheckId]; !present {
				check, ok := rdm.PostureChecks.Get(postureCheckId)
				if !ok {
					log.WithField("postureCheckId", postureCheckId).Error("could not find posture check")
				} else {
					checks[postureCheckId] = check
				}
			}
		})
	}
}

func (rdm *RouterDataModel) loadServiceConfigs(identity *Identity, svc *IdentityService) {
	log := pfxlog.Logger().
		WithField("identityId", identity.Id).
		WithField("serviceId", svc.Service.Id)

	result := map[string]*IdentityConfig{}

	for _, configId := range svc.Service.Configs {
		if identityConfig := rdm.loadIdentityConfig(configId, log); identityConfig != nil {
			result[identityConfig.TypeName] = identityConfig
		}
	}

	if serviceConfigs, hasOverride := identity.ServiceConfigs[svc.Service.Id]; hasOverride {
		for _, configId := range serviceConfigs.Configs {
			if identityConfig := rdm.loadIdentityConfig(configId, log); identityConfig != nil {
				result[identityConfig.TypeName] = identityConfig
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
		TypeId:   configType.Id,
		TypeName: configType.Name,
		DataJson: config.DataJson,
	}
}

func (rdm *RouterDataModel) GetEntityCounts() map[string]uint32 {
	result := map[string]uint32{
		"configType":         uint32(rdm.ConfigTypes.Count()),
		"configs":            uint32(rdm.Configs.Count()),
		"identities":         uint32(rdm.Identities.Count()),
		"Services":           uint32(rdm.Services.Count()),
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

func (rdm *RouterDataModel) queueSyncCheck(identityId string) {
	if rdm.subscriptions.Has(identityId) {
		rdm.markIdentityForCheck(identityId, IdentityUpdated)
		select {
		case rdm.events <- checkForIdentityChangesEvent{identityId: identityId}:
		default:
			select {
			case rdm.scanSubsNotify <- struct{}{}:
			default:
			}
		}
	}
}

func (rdm *RouterDataModel) queueNewIdentitySubCheck(identityId string) {
	if rdm.subscriptions.Has(identityId) {
		rdm.markIdentityForCheck(identityId, IdentityAdded)
		select {
		case rdm.events <- identityCreatedEvent{identityId: identityId}:
		default:
			select {
			case rdm.scanSubsNotify <- struct{}{}:
			default:
			}
		}
	}
}

func (rdm *RouterDataModel) queueIdentityDeletedSubCheck(identityId string) {
	if rdm.subscriptions.Has(identityId) {
		rdm.markIdentityForCheck(identityId, IdentityDeleted)
		select {
		case rdm.events <- identityDeletedEvent{identityId: identityId}:
		default:
			select {
			case rdm.scanSubsNotify <- struct{}{}:
			default:
			}
		}
	}
}

func (rdm *RouterDataModel) syncSubscriptionIfRequired(identityId string, clearUpdateEntry bool) {
	if subscription, _ := rdm.subscriptions.Get(identityId); subscription != nil {
		requiresSync := true

		if clearUpdateEntry {
			requiresSync = rdm.markIdentityCheckComplete(identityId, IdentityUpdated)
		}

		if requiresSync {
			subscription.checkForChanges(rdm)
		}
	}
}

func (rdm *RouterDataModel) checkSubsForNewIdentity(identityId string, clearUpdateEntry bool) {
	pfxlog.Logger().
		WithField("subs", rdm.subscriptions.Count()).
		WithField("identityId", identityId).
		Debug("handling identity created event")

	requiresSync := true

	if clearUpdateEntry {
		requiresSync = rdm.markIdentityCheckComplete(identityId, IdentityAdded)
	}

	if sub, found := rdm.subscriptions.Get(identityId); found {
		if requiresSync {
			identity, _ := rdm.Identities.Get(identityId)
			if identity == nil {
				pfxlog.Logger().WithField("identityId", identityId).Warn("identity not found while checking for subscription initialization")
				return
			}
			state, wasInitialized := sub.initialize(rdm, identity)
			if wasInitialized {
				sub.checkForChanges(rdm)
			} else {
				for _, subscriber := range sub.listeners.Value() {
					subscriber.NotifyIdentityEvent(state, IdentityFullStateState)
				}
			}
		}
	}
}

func (rdm *RouterDataModel) checkSubsForDeletedIdentity(identityId string, clearUpdateEntry bool) {
	if clearUpdateEntry {
		rdm.markIdentityCheckComplete(identityId, IdentityDeleted)
	}

	if sub, found := rdm.subscriptions.Get(identityId); found {
		sub.identityRemoved()
		if !sub.IsRouterIdentity {
			rdm.subscriptions.Remove(identityId)
			pfxlog.Logger().WithField("identityId", identityId).Debug("removed subscription for identity")
		}
	}
}

func (rdm *RouterDataModel) NotifyServiceOfConfigChange(serviceId string, index uint64) {
	rdm.withService(serviceId, func(service *Service) {
		if service.index < index {
			rdm.Services.Set(serviceId, &Service{
				Id:                 service.Id,
				Name:               service.Name,
				EncryptionRequired: service.EncryptionRequired,
				Configs:            service.Configs,
				servicePolicies:    service.servicePolicies,
				index:              index,
			})
		}

		service.servicePolicies.IterCb(func(servicePolicyId string, _ struct{}) {
			rdm.NotifyServicePolicyServiceChange(servicePolicyId, index)
		})
	})
}

func (rdm *RouterDataModel) NotifyIdentityChange(identityId string, index uint64) {
	rdm.withIdentity(identityId, func(identity *Identity) {
		identity.NotifyServiceChange(rdm)
	})
}

func (rdm *RouterDataModel) NotifyServicePolicyServiceChange(servicePolicyId string, serviceIndex uint64) {
	rdm.withServicePolicy(servicePolicyId, func(servicePolicy *ServicePolicy) {
		if servicePolicy.servicesIndex < serviceIndex {
			rdm.ServicePolicies.Set(servicePolicyId, &ServicePolicy{
				Id:                 servicePolicy.Id,
				Name:               servicePolicy.Name,
				PolicyType:         servicePolicy.PolicyType,
				Services:           servicePolicy.Services,
				PostureChecks:      servicePolicy.PostureChecks,
				Identities:         servicePolicy.Identities,
				servicesIndex:      serviceIndex,
				postureChecksIndex: servicePolicy.postureChecksIndex,
			})
		}

		servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
			if identity, _ := rdm.Identities.Get(identityId); identity != nil {
				identity.NotifyServiceChange(rdm)
			} else {
				pfxlog.Logger().
					WithField("identityId", identityId).
					WithField("servicePolicyId", servicePolicyId).
					Error("identity not found when updating service policy")
			}
		})
	})
}

func (rdm *RouterDataModel) NotifyServicePolicyPostureChecksChange(servicePolicyId string, postureChecksIndex uint64) {
	rdm.withServicePolicy(servicePolicyId, func(servicePolicy *ServicePolicy) {
		if servicePolicy.postureChecksIndex < postureChecksIndex {
			rdm.ServicePolicies.Set(servicePolicyId, &ServicePolicy{
				Id:                 servicePolicy.Id,
				Name:               servicePolicy.Name,
				PolicyType:         servicePolicy.PolicyType,
				Services:           servicePolicy.Services,
				PostureChecks:      servicePolicy.PostureChecks,
				Identities:         servicePolicy.Identities,
				servicesIndex:      servicePolicy.servicesIndex,
				postureChecksIndex: postureChecksIndex,
			})
		}
		servicePolicy.Identities.IterCb(func(identityId string, _ struct{}) {
			if identity, _ := rdm.Identities.Get(identityId); identity != nil {
				identity.NotifyServiceChange(rdm)
			} else {
				pfxlog.Logger().
					WithField("identityId", identityId).
					WithField("servicePolicyId", servicePolicyId).
					Error("identity not found when updating service policy")
			}
		})
	})
}

func (rdm *RouterDataModel) EnableServiceAccessTracking(identityId string) {
	rdm.withLockedIdentity(identityId, func(identity *Identity) {
		if identity.serviceAccessTrackingEnabled.CompareAndSwap(false, true) {
			identity.ServiceAccess = rdm.buildServiceAccessList(identity)
		}
	})
}

func (rdm *RouterDataModel) DisableServiceAccessTracking(identityId string) {
	rdm.withLockedIdentity(identityId, func(identity *Identity) {
		if identity.serviceAccessTrackingEnabled.CompareAndSwap(true, false) {
			identity.ServiceAccess = map[string]*serviceAccess{}
		}
	})
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
		v.Diff(rdm, false, sink)
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
	diffType("identity", rdm.Identities, o.Identities, sink, Identity{}, DataStateIdentity{}, serviceAccess{})
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
	syncSetT := cmp.Transformer("CMapSetToMap", func(s cmap.ConcurrentMap[string, struct{}]) map[string]struct{} {
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

// ToMap returns a map representation of the RouterDataModel with recursively exported data
func (rdm *RouterDataModel) ToMap() map[string]interface{} {
	result := make(map[string]interface{})

	// Export config types
	configTypes := make(map[string]interface{})
	rdm.ConfigTypes.IterCb(func(key string, v *ConfigType) {
		configTypes[key] = map[string]interface{}{
			"id":    v.Id,
			"name":  v.Name,
			"index": v.index,
		}
	})
	result["configTypes"] = configTypes

	// Export configs
	configs := make(map[string]interface{})
	rdm.Configs.IterCb(func(key string, v *Config) {
		services := make([]string, 0)
		v.services.IterCb(func(serviceId string, _ struct{}) {
			services = append(services, serviceId)
		})
		configs[key] = map[string]interface{}{
			"id":       v.Id,
			"name":     v.Name,
			"typeId":   v.TypeId,
			"dataJson": v.DataJson,
			"services": services,
			"index":    v.index,
		}
	})
	result["configs"] = configs

	// Export identities
	identities := make(map[string]interface{})
	rdm.Identities.IterCb(func(key string, v *Identity) {
		servicePolicies := make([]string, 0)
		v.IterateServicePolicies(func(servicePolicyId string) {
			servicePolicies = append(servicePolicies, servicePolicyId)
		})

		services := make(map[string]interface{})
		v.lock.Lock()
		for serviceId, access := range v.ServiceAccess {
			services[serviceId] = map[string]interface{}{
				"dial": access.DialPoliciesCount,
				"bind": access.BindPoliciesCount,
			}
		}

		postureChecks := make(map[string]int32)
		for checkId, count := range v.PostureChecks {
			postureChecks[checkId] = count
		}
		v.lock.Unlock()

		identities[key] = map[string]interface{}{
			"id":                        v.Id,
			"name":                      v.Name,
			"disabled":                  v.Disabled,
			"appDataJson":               string(v.AppDataJson),
			"defaultHostingPrecedence":  v.DefaultHostingPrecedence,
			"defaultHostingCost":        v.DefaultHostingCost,
			"serviceHostingPrecedences": v.ServiceHostingPrecedences,
			"serviceHostingCosts":       v.ServiceHostingCosts,
			"serviceConfigs":            v.ServiceConfigs,
			"servicePolicies":           servicePolicies,
			"services":                  services,
			"postureChecks":             postureChecks,
			"identityIndex":             v.identityIndex,
		}
	})
	result["identities"] = identities

	// Export services
	services := make(map[string]interface{})
	rdm.Services.IterCb(func(key string, v *Service) {
		servicePolicies := make([]string, 0)
		v.servicePolicies.IterCb(func(policyId string, _ struct{}) {
			servicePolicies = append(servicePolicies, policyId)
		})

		services[key] = map[string]interface{}{
			"id":                 v.Id,
			"name":               v.Name,
			"encryptionRequired": v.EncryptionRequired,
			"configs":            v.Configs,
			"servicePolicies":    servicePolicies,
			"index":              v.index,
		}
	})
	result["services"] = services

	// Export service policies
	servicePolicies := make(map[string]interface{})
	rdm.ServicePolicies.IterCb(func(key string, v *ServicePolicy) {
		policyServices := make([]string, 0)
		v.Services.IterCb(func(serviceId string, _ struct{}) {
			policyServices = append(policyServices, serviceId)
		})

		postureChecks := make([]string, 0)
		v.PostureChecks.IterCb(func(checkId string, _ struct{}) {
			postureChecks = append(postureChecks, checkId)
		})

		identitiesList := make([]string, 0)
		v.Identities.IterCb(func(identityId string, _ struct{}) {
			identitiesList = append(identitiesList, identityId)
		})

		servicePolicies[key] = map[string]interface{}{
			"id":            v.Id,
			"name":          v.Name,
			"policyType":    v.PolicyType.String(),
			"services":      policyServices,
			"postureChecks": postureChecks,
			"identities":    identitiesList,
			"index":         v.servicesIndex,
		}
	})
	result["servicePolicies"] = servicePolicies

	// Export posture checks
	postureChecks := make(map[string]interface{})
	rdm.PostureChecks.IterCb(func(key string, v *PostureCheck) {
		postureChecks[key] = map[string]interface{}{
			"id":              v.Id,
			"name":            v.Name,
			"typeId":          v.TypeId,
			"index":           v.index,
			"servicePolicies": v.servicePolicies.Keys(),
		}
	})
	result["postureChecks"] = postureChecks

	// Export subscriptions
	subscriptions := make(map[string]interface{})
	rdm.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
		v.Lock()
		defer v.Unlock()

		// Export services within subscription
		subServices := make(map[string]interface{})
		for serviceId, identityService := range v.Services {
			var dialPoliciesCount int32
			var bindPoliciesCount int32

			v.Identity.WithLock(func() {
				if access := v.Identity.ServiceAccess[serviceId]; access != nil {
					dialPoliciesCount = access.DialPoliciesCount
					bindPoliciesCount = access.BindPoliciesCount
				}
			})

			configsMap := make(map[string]interface{})
			for configTypeName, identityConfig := range identityService.Configs {
				configsMap[configTypeName] = map[string]interface{}{
					"configTypeId":   identityConfig.TypeId,
					"configTypeName": identityConfig.TypeName,
					"dataJson":       identityConfig.DataJson,
				}
			}

			subServices[serviceId] = map[string]interface{}{
				"serviceId":         identityService.Service.Id,
				"serviceName":       identityService.Service.Name,
				"dialAllowed":       identityService.IsDialAllowed(),
				"bindAllowed":       identityService.IsBindAllowed(),
				"dialPoliciesCount": dialPoliciesCount,
				"bindPoliciesCount": bindPoliciesCount,
				"configs":           configsMap,
			}
		}

		// Export posture checks within subscription
		subChecks := make(map[string]interface{})
		for checkId, check := range v.Checks {
			subChecks[checkId] = map[string]interface{}{
				"id":     check.Id,
				"name":   check.Name,
				"typeId": check.TypeId,
			}
		}

		serviceAccessTrackingEnabled := false
		if v.Identity != nil {
			serviceAccessTrackingEnabled = v.Identity.serviceAccessTrackingEnabled.Load()
		}
		subscriptions[key] = map[string]interface{}{
			"identityId":                   v.IdentityId,
			"isRecreatable":                v.IsRouterIdentity,
			"services":                     subServices,
			"postureChecks":                subChecks,
			"serviceAccessTrackingEnabled": serviceAccessTrackingEnabled,
		}
	})
	result["subscriptions"] = subscriptions

	return result
}
