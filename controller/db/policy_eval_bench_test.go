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
	"fmt"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"go.etcd.io/bbolt"
)

// BenchmarkPolicyWrites times the writes that re-evaluate policy links against a store holding n
// identities, n services and, per service, a dial policy and a service edge router policy that name
// their targets by id. fsync is disabled so the figures reflect evaluation rather than the disk.
// Store tests log an error per policy link change for want of an app env, so discard stderr to
// keep the result lines intact:
//
//	go test ./controller/db/ -run '^$' -bench BenchmarkPolicyWrites -benchtime 50x 2>/dev/null
func BenchmarkPolicyWrites(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			ctx := NewTestContext(b)
			defer ctx.Cleanup()
			ctx.Init()
			ctx.NoError(ctx.GetDb().View(func(tx *bbolt.Tx) error {
				tx.DB().NoSync = true
				return nil
			}))

			identityTypeId := ctx.getIdentityTypeId()
			identityIds, serviceIds := ctx.seedIdPolicies(n, identityTypeId)

			newPolicy := func(i int) *ServicePolicy {
				return &ServicePolicy{
					BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
					Name:          eid.New(),
					PolicyType:    PolicyTypeDial,
					Semantic:      SemanticAllOf,
					IdentityRoles: []string{entityRef(identityIds[i%n])},
					ServiceRoles:  []string{entityRef(serviceIds[i%n])},
				}
			}

			b.Run("service create", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					boltztest.RequireCreate(ctx, newEdgeService(eid.New()))
				}
			})

			b.Run("identity create", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					boltztest.RequireCreate(ctx, newIdentity(eid.New(), identityTypeId))
				}
			})

			b.Run("policy create", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					boltztest.RequireCreate(ctx, newPolicy(i))
				}
			})

			b.Run("policy delete", func(b *testing.B) {
				b.StopTimer()
				var policies []*ServicePolicy
				for i := 0; i < b.N; i++ {
					policy := newPolicy(i)
					boltztest.RequireCreate(ctx, policy)
					policies = append(policies, policy)
				}
				b.StartTimer()
				for _, policy := range policies {
					ctx.NoError(boltztest.Delete(ctx, policy))
				}
			})

			b.Run("service delete", func(b *testing.B) {
				b.StopTimer()
				var services []*Service
				for i := 0; i < b.N; i++ {
					service := newEdgeService(eid.New())
					boltztest.RequireCreate(ctx, service)
					services = append(services, service)
				}
				b.StartTimer()
				for _, service := range services {
					ctx.NoError(boltztest.Delete(ctx, service))
				}
			})
		})
	}
}

// seedIdPolicies creates n identities and n services, and per service a dial policy naming it and
// one identity by id plus a service edge router policy naming it by id and every router. Entities
// are created in batches of one transaction each. It returns the identity and service ids.
func (ctx *TestContext) seedIdPolicies(n int, identityTypeId string) (identityIds, serviceIds []string) {
	const batchSize = 500
	for start := 0; start < n; start += batchSize {
		end := min(start+batchSize, n)
		err := ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
			for i := start; i < end; i++ {
				identity := newIdentity(eid.New(), identityTypeId)
				if err := ctx.stores.Identity.Create(mctx, identity); err != nil {
					return err
				}
				service := newEdgeService(eid.New())
				if err := ctx.stores.Service.Create(mctx, service); err != nil {
					return err
				}
				identityIds = append(identityIds, identity.Id)
				serviceIds = append(serviceIds, service.Id)

				policy := &ServicePolicy{
					BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
					Name:          eid.New(),
					PolicyType:    PolicyTypeDial,
					Semantic:      SemanticAllOf,
					IdentityRoles: []string{entityRef(identity.Id)},
					ServiceRoles:  []string{entityRef(service.Id)},
				}
				if err := ctx.stores.ServicePolicy.Create(mctx, policy); err != nil {
					return err
				}
				serp := &ServiceEdgeRouterPolicy{
					BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
					Name:            eid.New(),
					Semantic:        SemanticAllOf,
					ServiceRoles:    []string{entityRef(service.Id)},
					EdgeRouterRoles: []string{AllRole},
				}
				if err := ctx.stores.ServiceEdgeRouterPolicy.Create(mctx, serp); err != nil {
					return err
				}
			}
			return nil
		})
		ctx.NoError(err)
	}
	return identityIds, serviceIds
}
