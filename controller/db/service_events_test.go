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

package db

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
)

// Test_ServiceUpdatedEventsCoalesce verifies that a service joining a policy with several posture
// checks raises one ServiceUpdated event per identity, not one per posture check, alongside the
// single access event for the pair.
func Test_ServiceUpdatedEventsCoalesce(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()
	ctx.CleanupAll()

	identity := newIdentity(eid.New(), ctx.getIdentityTypeId(), "a")
	boltztest.RequireCreate(ctx, identity)
	service := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, service)

	var postureCheckRoles []string
	for i := 0; i < 3; i++ {
		postureCheck := newTestPostureCheck()
		boltztest.RequireCreate(ctx, postureCheck)
		postureCheckRoles = append(postureCheckRoles, entityRef(postureCheck.Id))
	}

	policy := ctx.newServicePolicy(PolicyTypeDial, SemanticAllOf, []string{roleRef("a")}, nil, postureCheckRoles)
	boltztest.RequireCreate(ctx, policy)

	// handlers cannot be removed from the registry, so this one switches itself off instead
	var lock sync.Mutex
	var received []*ServiceEvent
	var active atomic.Bool
	active.Store(true)
	defer active.Store(false)
	ServiceEvents.AddServiceEventHandler(func(event *ServiceEvent) {
		if !active.Load() {
			return
		}
		lock.Lock()
		defer lock.Unlock()
		received = append(received, event)
	})

	policy.ServiceRoles = []string{entityRef(service.Id)}
	boltztest.RequireUpdate(ctx, policy)

	// events for one write are dispatched in order by one goroutine, so once the last expected
	// event has arrived any duplicates would already be queued behind it
	deadline := time.Now().Add(5 * time.Second)
	for {
		lock.Lock()
		count := len(received)
		lock.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	lock.Lock()
	defer lock.Unlock()
	counts := map[ServiceEventType]int{}
	for _, event := range received {
		ctx.Equal(identity.Id, event.IdentityId)
		ctx.Equal(service.Id, event.ServiceId)
		counts[event.Type]++
	}
	ctx.Equal(1, counts[ServiceDialAccessGained], "events: %v", received)
	ctx.Equal(1, counts[ServiceUpdated], "events: %v", received)
	ctx.Len(received, 2)
}
