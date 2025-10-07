package common

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type subscriberTest struct {
	rdm *RouterDataModel

	minIdentities      int
	maxIdentities      int
	identityRandStream *randomStream[*Identity]

	minServices       int
	maxServices       int
	serviceRandStream *randomStream[*Service]

	minServicePolicies      int
	maxServicePolicies      int
	servicePolicyRandStream *randomStream[*ServicePolicy]

	identitiesPerServicePolicy int
	servicesPerServicePolicy   int

	servicePolicyToIdentitiesMaps map[string]map[string]struct{}
	maxSubscribers                int

	index uint64

	notifications atomic.Int64
}

func (self *subscriberTest) handleEvent(event *edge_ctrl_pb.DataState_Event) {
	self.index++
	self.rdm.Handle(self.index, event)
}

func (self *subscriberTest) handleChangeSet(changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	for _, event := range changeSet.Changes {
		self.rdm.Handle(changeSet.Index, event)
	}
}

func (self *subscriberTest) AddIdentity() {
	if self.rdm.Identities.Count() >= self.maxIdentities {
		return
	}

	identityId := eid.New()
	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: &edge_ctrl_pb.DataState_Identity{
				Id:   identityId,
				Name: eid.New(),
			},
		},
	}

	self.handleEvent(event)
	if self.rdm.subscriptions.Count() < self.maxSubscribers {
		if err := self.rdm.SubscribeToIdentityChanges(identityId, self, false); err != nil {
			panic(err)
		}
	}
}

func (self *subscriberTest) RemoveIdentity() {
	if self.rdm.Identities.Count() <= self.minIdentities {
		return
	}

	identity := self.identityRandStream.Next()

	self.index++
	changeSet := &edge_ctrl_pb.DataState_ChangeSet{
		Index:       self.index,
		IsSynthetic: false,
		Changes:     nil,
		TimestampId: "",
	}

	self.rdm.withLockedIdentity(true, identity.Id, func(identity *Identity) {
		for servicePolicyId := range identity.ServicePolicies {
			changeSet.Changes = append(changeSet.Changes,
				&edge_ctrl_pb.DataState_Event{
					Action: edge_ctrl_pb.DataState_Create,
					Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
						ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
							PolicyId:          servicePolicyId,
							RelatedEntityIds:  []string{identity.Id},
							RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
							Add:               false,
						},
					},
				})
		}
	})

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: identity.DataStateIdentity,
		},
	}

	changeSet.Changes = append(changeSet.Changes, event)

	self.handleChangeSet(changeSet)

	for self.rdm.subscriptions.Count() < self.maxSubscribers {
		id := self.identityRandStream.Next()
		if !self.rdm.subscriptions.Has(id.Id) {
			if err := self.rdm.SubscribeToIdentityChanges(id.Id, self, false); err != nil {
				panic(err)
			}
		}
	}
}

func (self *subscriberTest) AddService() {
	if self.rdm.Services.Count() >= self.maxServices {
		return
	}

	serviceId := eid.New()
	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: &edge_ctrl_pb.DataState_Service{
				Id:   serviceId,
				Name: eid.New(),
			},
		},
	}

	self.handleEvent(event)
}

func (self *subscriberTest) RemoveService() {
	if self.rdm.Services.Count() <= self.minServices {
		return
	}

	service := self.serviceRandStream.Next()

	changeSet := &edge_ctrl_pb.DataState_ChangeSet{
		Index:       self.index,
		IsSynthetic: false,
		Changes:     nil,
		TimestampId: "",
	}

	service.ServicePolicies.IterCb(func(key string, v struct{}) {
		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          key,
						RelatedEntityIds:  []string{service.Id},
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
						Add:               false,
					},
				},
			})
	})

	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: service.DataStateService,
		},
	}

	changeSet.Changes = append(changeSet.Changes, event)

	self.handleChangeSet(changeSet)
}

func (self *subscriberTest) AddServicePolicy() string {
	if self.rdm.ServicePolicies.Count() >= self.maxServicePolicies {
		return ""
	}

	policyId := eid.New()
	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
			ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
				Id:   policyId,
				Name: eid.New(),
				PolicyType: func() edge_ctrl_pb.PolicyType {
					if rand.Int()%2 == 0 {
						return edge_ctrl_pb.PolicyType_DialPolicy
					}
					return edge_ctrl_pb.PolicyType_BindPolicy
				}(),
			},
		},
	}

	self.handleEvent(event)
	return policyId
}

func (self *subscriberTest) RemoveServicePolicy() {
	if self.rdm.ServicePolicies.Count() <= self.minServicePolicies {
		return
	}

	servicePolicy := self.servicePolicyRandStream.Next()
	event := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Delete,
		Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
			ServicePolicy: servicePolicy.DataStateServicePolicy,
		},
	}

	delete(self.servicePolicyToIdentitiesMaps, servicePolicy.GetId())

	self.handleEvent(event)
}

func (self *subscriberTest) getServicePolicyToIdentityMap(policyId string) map[string]struct{} {
	m, ok := self.servicePolicyToIdentitiesMaps[policyId]
	if !ok {
		m = make(map[string]struct{})
		self.servicePolicyToIdentitiesMaps[policyId] = m
	}
	return m
}

func (self *subscriberTest) RemoveIdentitiesFromPolicy(policy *ServicePolicy, changeSet *edge_ctrl_pb.DataState_ChangeSet) (map[string]struct{}, func()) {
	policyToIdMap := self.getServicePolicyToIdentityMap(policy.Id)
	idsToRemoveCount := min(len(policyToIdMap), rand.Intn(15))
	idsToRemove := map[string]struct{}{}
	if idsToRemoveCount > 0 {
		for id := range policyToIdMap {
			idsToRemove[id] = struct{}{}
			if len(idsToRemove) == idsToRemoveCount {
				break
			}
		}

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          policy.Id,
						RelatedEntityIds:  slices.Collect(maps.Keys(idsToRemove)),
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
						Add:               false,
					},
				},
			})

		return idsToRemove, func() {
			for id := range idsToRemove {
				delete(policyToIdMap, id)
			}
		}
	}

	return idsToRemove, func() {}
}

func (self *subscriberTest) AddIdentitiesToPolicy(n int, policy *ServicePolicy, removed map[string]struct{}, changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	policyToIdMap := self.getServicePolicyToIdentityMap(policy.Id)

	if n == 0 {
		return
	}

	identityIds := self.identityRandStream.GetFilteredSet(n,
		func(identity *Identity) bool {
			_, hasKey := policyToIdMap[identity.Id]
			_, keyDeleted := removed[identity.Id]
			return !hasKey && !keyDeleted
		},
		func(identity *Identity) string {
			return identity.Id
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          policy.Id,
					RelatedEntityIds:  slices.Collect(maps.Keys(identityIds)),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
					Add:               true,
				},
			},
		})

	for id := range identityIds {
		policyToIdMap[id] = struct{}{}
	}
}

func (self *subscriberTest) RemoveServicesFromPolicy(policy *ServicePolicy, changeSet *edge_ctrl_pb.DataState_ChangeSet) map[string]struct{} {
	servicesToRemoveCount := min(policy.Services.Count(), rand.Intn(15))
	servicesToRemove := map[string]struct{}{}
	if servicesToRemoveCount > 0 {
		policy.Services.IterCb(func(key string, v struct{}) {
			if len(servicesToRemove) < servicesToRemoveCount {
				servicesToRemove[key] = struct{}{}
			}
		})

		changeSet.Changes = append(changeSet.Changes,
			&edge_ctrl_pb.DataState_Event{
				Action: edge_ctrl_pb.DataState_Create,
				Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
					ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
						PolicyId:          policy.Id,
						RelatedEntityIds:  slices.Collect(maps.Keys(servicesToRemove)),
						RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
						Add:               false,
					},
				},
			})

	}

	return servicesToRemove
}

func (self *subscriberTest) AddServicesToPolicy(n int, policy *ServicePolicy, removed map[string]struct{}, changeSet *edge_ctrl_pb.DataState_ChangeSet) {
	if n == 0 {
		return
	}

	serviceIds := self.serviceRandStream.GetFilteredSet(n,
		func(service *Service) bool {
			_, keyDeleted := removed[service.Id]
			return !policy.Services.Has(service.Id) && !keyDeleted
		},
		func(service *Service) string {
			return service.Id
		})

	changeSet.Changes = append(changeSet.Changes,
		&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
				ServicePolicyChange: &edge_ctrl_pb.DataState_ServicePolicyChange{
					PolicyId:          policy.Id,
					RelatedEntityIds:  slices.Collect(maps.Keys(serviceIds)),
					RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
					Add:               true,
				},
			},
		})
}

func (self *subscriberTest) ChangeServicePolicies(policy *ServicePolicy) {
	self.index++
	changeSet := &edge_ctrl_pb.DataState_ChangeSet{
		Index:       self.index,
		IsSynthetic: false,
		Changes:     nil,
		TimestampId: "",
	}

	policyToIdMap := self.getServicePolicyToIdentityMap(policy.Id)
	idsToRemove, cleanup := self.RemoveIdentitiesFromPolicy(policy, changeSet)
	defer cleanup()

	missingIds := self.identitiesPerServicePolicy - (len(policyToIdMap) - len(idsToRemove))
	self.AddIdentitiesToPolicy(missingIds, policy, idsToRemove, changeSet)

	servicesToRemove := self.RemoveServicesFromPolicy(policy, changeSet)
	missingServices := self.servicesPerServicePolicy - (policy.Services.Count() - len(servicesToRemove))
	self.AddServicesToPolicy(missingServices, policy, servicesToRemove, changeSet)

	self.handleChangeSet(changeSet)
}

func (self *subscriberTest) NotifyIdentityEvent(state *IdentityState, eventType IdentityEventType) {
	self.notifications.Add(1)
}

func (self *subscriberTest) NotifyServiceChange(state *IdentityState, service *IdentityService, eventType ServiceEventType) {
	self.notifications.Add(1)
}

func newRandomStream[T HasId](m cmap.ConcurrentMap[string, T]) *randomStream[T] {
	result := &randomStream[T]{
		m:       m,
		c:       make(chan T, 10),
		closeCh: make(chan struct{}),
	}
	return result
}

type HasId interface {
	GetId() string
	comparable
}

type randomStream[T HasId] struct {
	m       cmap.ConcurrentMap[string, T]
	c       chan T
	stopped atomic.Bool
	closeCh chan struct{}
	in      <-chan cmap.Tuple[string, T]
}

func (self *randomStream[T]) run() {
	for !self.stopped.Load() {
		skip := rand.Intn(10)
		for i := 0; i < skip; i++ {
			self.getNextSeq()
		}
		val, ok := self.getNextSeq()
		if !ok {
			break
		}
		if reflect.ValueOf(val).IsNil() {
			panic("pushing nil value onto random stream")
		}
		select {
		case self.c <- val:
		case <-self.closeCh:
			return
		}
	}
}

func (self *randomStream[T]) getNextSeq() (T, bool) {
	var defaultT T

	if self.in == nil {
		if self.m.Count() == 0 {
			self.stop()
			return defaultT, false
		}
		self.in = self.m.IterBuffered()
	}

	select {
	case <-self.closeCh:
		return defaultT, false
	case t, ok := <-self.in:
		if !ok {
			self.in = nil
			return self.getNextSeq()
		}
		if reflect.ValueOf(t.Val).IsNil() {
			fmt.Println(reflect.TypeOf(self.m).String())
			panic("get nil value from map iterator")
		}
		return t.Val, true
	}
}

func (self *randomStream[T]) stop() {
	debugz.DumpLocalStack()
	if self.stopped.CompareAndSwap(false, true) {
		close(self.closeCh)
	}
}

func (self *randomStream[T]) Next() T {
	for {
		select {
		case <-self.closeCh:
			var defaultT T
			return defaultT
		case v := <-self.c:
			if self.m.Has(v.GetId()) {
				return v
			}
		}
	}
}

func (self *randomStream[T]) GetFilteredSet(n int, filterF func(T) bool, mapF func(T) string) map[string]struct{} {
	result := map[string]struct{}{}
	for len(result) < n {
		next := self.Next()
		if filterF(next) {
			result[mapF(next)] = struct{}{}
		}
	}
	return result
}

func TestSubscriberScale(t *testing.T) {
	closeNotify := make(chan struct{})
	rdm := NewReceiverRouterDataModel(0, closeNotify)

	defer func() {
		rdmJson := rdm.ToJson()
		jsonBytes, err := json.Marshal(rdmJson)
		if err != nil {
			fmt.Printf("unable to marshal json: %v\n", err)
			return
		}
		if err = os.WriteFile("/home/plorenz/tmp/test.json", jsonBytes, 0644); err != nil {
			fmt.Printf("unable to write json: %v\n", err)
		}
	}()

	test := &subscriberTest{
		rdm:                rdm,
		minIdentities:      1_000,
		maxIdentities:      1_200,
		identityRandStream: newRandomStream(rdm.Identities),

		minServices:       10_000,
		maxServices:       12_000,
		serviceRandStream: newRandomStream(rdm.Services),

		minServicePolicies:      1_000,
		maxServicePolicies:      2_000,
		servicePolicyRandStream: newRandomStream(rdm.ServicePolicies),

		servicePolicyToIdentitiesMaps: map[string]map[string]struct{}{},
		identitiesPerServicePolicy:    100,
		servicesPerServicePolicy:      250,

		maxSubscribers: 100,
	}

	//test = &subscriberTest{
	//	rdm:                rdm,
	//	minIdentities:      1_00,
	//	maxIdentities:      1_20,
	//	identityRandStream: newRandomStream(rdm.Identities),
	//
	//	minServices:       10_00,
	//	maxServices:       12_00,
	//	serviceRandStream: newRandomStream(rdm.Services),
	//
	//	minServicePolicies:      1_00,
	//	maxServicePolicies:      2_00,
	//	servicePolicyRandStream: newRandomStream(rdm.ServicePolicies),
	//
	//	servicePolicyToIdentitiesMaps: map[string]map[string]struct{}{},
	//	identitiesPerServicePolicy:    10,
	//	servicesPerServicePolicy:      25,
	//
	//	maxSubscribers: 10,
	//}

	fmt.Print("adding identities... ")
	for i := 0; i < test.minIdentities; i++ {
		test.AddIdentity()
	}
	fmt.Printf("%d identities added\n", test.rdm.Identities.Count())

	fmt.Print("adding Services... ")
	for i := 0; i < test.minServices; i++ {
		test.AddService()
	}
	fmt.Printf("%d Services added\n", test.rdm.Services.Count())

	fmt.Print("adding service policies... ")
	for i := 0; i < test.minServicePolicies; i++ {
		test.AddServicePolicy()
	}
	fmt.Printf("%d Services policies added\n", test.rdm.ServicePolicies.Count())

	go test.identityRandStream.run()
	go test.serviceRandStream.run()
	go test.servicePolicyRandStream.run()

	for tuple := range test.rdm.ServicePolicies.IterBuffered() {
		test.ChangeServicePolicies(tuple.Val)
	}

	for tuple := range test.rdm.ServicePolicies.IterBuffered() {
		policy := tuple.Val
		if policy.Services.Count() != test.servicesPerServicePolicy {
			fmt.Printf("policy %s doesn't have enough Services: %d\n", policy.Id, test.rdm.Services.Count())
		}
		identities := test.getServicePolicyToIdentityMap(policy.Id)
		if len(identities) != test.identitiesPerServicePolicy {
			fmt.Printf("policy %s doesn't have enough identities: %d\n", policy.Id, len(identities))
		}
	}

	start := time.Now()
	rdm.SyncUpdatedSubscribers()
	rdm.waitForQueueEmpty()
	fmt.Printf("sync all subscribers time: %v, with %d notificaitons\n", time.Since(start), test.notifications.Load())
	test.notifications.Store(0)

	for i := 0; i < 10; i++ {
		start := time.Now()
		n := rand.Intn(17)
		iters := rand.Intn(10) + 1
		switch n {
		case 0:
			fmt.Printf("adding %d identities\n", iters)
			for range iters {
				test.AddIdentity()
			}
		case 1:
			fmt.Printf("removing %d identities\n", iters)
			for range iters {
				test.RemoveIdentity()
			}
		case 2:
			fmt.Printf("adding %d Services\n", iters)
			for range iters {
				test.AddService()
			}
		case 3:
			fmt.Printf("removing %d Services\n", iters)
			for range iters {
				test.RemoveService()
			}
		case 4:
			fmt.Printf("adding %d policies\n", iters)
			for range iters {
				policyId := test.AddServicePolicy()
				if policyId != "" {
					p, _ := test.rdm.ServicePolicies.Get(policyId)
					test.ChangeServicePolicies(p)
				}
			}
		case 5:
			fmt.Printf("remove %d service policies\n", iters)
			for range iters {
				test.RemoveServicePolicy()
			}
		default:
			fmt.Printf("changing %d service policies\n", iters)
			for range iters {
				test.ChangeServicePolicies(test.servicePolicyRandStream.Next())
			}
		}

		fmt.Printf("make rdm changes time: %v\n", time.Since(start))

		test.rdm.SyncUpdatedSubscribers()
		test.rdm.waitForQueueEmpty()
		fmt.Printf("sync all subscribers time: %v, with %d notificaitons\n", time.Since(start), test.notifications.Load())

		fmt.Printf("%d: total elapsed: %s\n\n", i, time.Since(start))
		test.notifications.Store(0)

		start = time.Now()
		rdm.subscriptions.IterCb(func(key string, v *IdentitySubscription) {
			v.Diff(rdm, false, func(entityType string, id string, diffType DiffType, detail string) {
				err := fmt.Errorf("%s (direct) id: %s diffType: %s, detail: %s", entityType, id, diffType, detail)
				panic(err)
			})

		})
		fmt.Printf("%d: validated %d subscribers time: %v\n\n", i, test.rdm.subscriptions.Count(), time.Since(start))
	}
}
