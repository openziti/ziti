//go:build perftests

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

package tests

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/rcrowley/go-metrics"
)

// Test_SpecIdPolicyWritesMedium drives the management API write path against a store of 10,000
// identities and services where every service has a dial policy, a bind policy and a service edge
// router policy naming their targets by id.
func Test_SpecIdPolicyWritesMedium(t *testing.T) {
	p := &writePerf{TestContext: NewTestContext(t)}
	p.run(&writePerfSpec{name: "id-policy-writes-medium", seedCount: 10_000, clients: 4, iterationsPerClient: 50})
}

// Test_SpecIdPolicyWritesLarge is Test_SpecIdPolicyWritesMedium against 50,000 of each.
func Test_SpecIdPolicyWritesLarge(t *testing.T) {
	p := &writePerf{TestContext: NewTestContext(t)}
	p.run(&writePerfSpec{name: "id-policy-writes-large", seedCount: 50_000, clients: 4, iterationsPerClient: 50})
}

type writePerfSpec struct {
	name                string
	seedCount           int
	clients             int
	iterationsPerClient int

	identityIds []string
}

// writePerf runs concurrent management API clients through the create and delete sequence that
// automation performs per resource (a service, two id policies and a service edge router policy)
// and records the latency and status of every request.
type writePerf struct {
	*TestContext
	spec *writePerfSpec

	lock     sync.Mutex
	timers   map[string]metrics.Histogram
	statuses map[string]map[int]int
	failures int
}

func (p *writePerf) run(spec *writePerfSpec) {
	defer p.Teardown()

	p.spec = spec
	p.timers = map[string]metrics.Histogram{}
	p.statuses = map[string]map[int]int{}

	p.StartServer()
	p.RequireAdminManagementApiLogin()

	start := time.Now()
	p.seed()
	seedTime := time.Since(start)

	sessions := make([]*session, spec.clients)
	for i := range sessions {
		var err error
		sessions[i], err = p.AdminAuthenticator.AuthenticateManagementApi(p.TestContext)
		p.Req.NoError(err)
	}

	start = time.Now()
	var wg sync.WaitGroup
	for i, sess := range sessions {
		wg.Add(1)
		go func(client int, sess *session) {
			defer wg.Done()
			p.runClient(client, sess)
		}(i, sess)
	}
	wg.Wait()
	elapsed := time.Since(start)

	p.report(seedTime, elapsed)
	p.Req.Zero(p.failures, "responses other than success")
}

// seed writes the store directly, in batched transactions, since the time to build the fixture is
// not what is being measured.
func (p *writePerf) seed() {
	stores := p.EdgeController.AppEnv.GetStores()
	const batchSize = 500
	for start := 0; start < p.spec.seedCount; start += batchSize {
		end := min(start+batchSize, p.spec.seedCount)
		err := p.EdgeController.AppEnv.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
			for i := start; i < end; i++ {
				identity := &db.Identity{
					BaseExtEntity:  boltz.BaseExtEntity{Id: eid.New()},
					Name:           eid.New(),
					IdentityTypeId: db.DefaultIdentityType,
				}
				if err := stores.Identity.Create(mctx, identity); err != nil {
					return err
				}
				p.spec.identityIds = append(p.spec.identityIds, identity.Id)

				service := &db.Service{
					BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
					Name:          eid.New(),
				}
				if err := stores.Service.Create(mctx, service); err != nil {
					return err
				}

				for _, policyType := range []db.PolicyType{db.PolicyTypeDial, db.PolicyTypeBind} {
					policy := &db.ServicePolicy{
						BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
						Name:          eid.New(),
						PolicyType:    policyType,
						Semantic:      db.SemanticAllOf,
						IdentityRoles: s("@" + identity.Id),
						ServiceRoles:  s("@" + service.Id),
					}
					if err := stores.ServicePolicy.Create(mctx, policy); err != nil {
						return err
					}
				}

				serp := &db.ServiceEdgeRouterPolicy{
					BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
					Name:            eid.New(),
					Semantic:        db.SemanticAllOf,
					ServiceRoles:    s("@" + service.Id),
					EdgeRouterRoles: s("#all"),
				}
				if err := stores.ServiceEdgeRouterPolicy.Create(mctx, serp); err != nil {
					return err
				}
			}
			return nil
		})
		p.Req.NoError(err)
	}
}

func (p *writePerf) runClient(client int, sess *session) {
	for i := 0; i < p.spec.iterationsPerClient; i++ {
		identityId := p.spec.identityIds[(client*p.spec.iterationsPerClient+i)%len(p.spec.identityIds)]

		service := p.newService(nil, nil)
		serviceId, ok := p.timedCreate(sess, "service create", service)
		if !ok {
			continue
		}

		bindId, _ := p.timedCreate(sess, "policy create", newServicePolicy("Bind", db.SemanticAllOf, s("@"+serviceId), s("@"+identityId), nil))
		dialId, _ := p.timedCreate(sess, "policy create", newServicePolicy("Dial", db.SemanticAllOf, s("@"+serviceId), s("@"+identityId), nil))
		serpId, _ := p.timedCreate(sess, "service edge router policy create", newServiceEdgeRouterPolicy(db.SemanticAllOf, s("#all"), s("@"+serviceId)))

		if serpId != "" {
			p.timedDelete(sess, "service edge router policy delete", "service-edge-router-policies", serpId)
		}
		for _, policyId := range []string{dialId, bindId} {
			if policyId != "" {
				p.timedDelete(sess, "policy delete", "service-policies", policyId)
			}
		}
		p.timedDelete(sess, "service delete", "services", serviceId)
	}
}

func (p *writePerf) timedCreate(sess *session, op string, entity entity) (string, bool) {
	start := time.Now()
	resp := sess.createEntity(entity)
	p.record(op, resp.StatusCode(), time.Since(start))
	if resp.StatusCode() != http.StatusCreated {
		return "", false
	}
	parsed, err := gabs.ParseJSON(resp.Body())
	if err != nil {
		return "", false
	}
	id, _ := parsed.Path("data.id").Data().(string)
	return id, id != ""
}

func (p *writePerf) timedDelete(sess *session, op string, entityType string, id string) {
	start := time.Now()
	resp := sess.deleteEntityOfType(entityType, id)
	p.record(op, resp.StatusCode(), time.Since(start))
}

func (p *writePerf) record(op string, status int, elapsed time.Duration) {
	p.lock.Lock()
	defer p.lock.Unlock()

	timer, ok := p.timers[op]
	if !ok {
		timer = metrics.NewHistogram(metrics.NewUniformSample(100_000))
		p.timers[op] = timer
		p.statuses[op] = map[int]int{}
	}
	timer.Update(elapsed.Microseconds())
	p.statuses[op][status]++
	if status < 200 || status > 299 {
		p.failures++
	}
}

func (p *writePerf) report(seedTime, elapsed time.Duration) {
	ops := make([]string, 0, len(p.timers))
	var total int64
	for op, timer := range p.timers {
		ops = append(ops, op)
		total += timer.Count()
	}
	sort.Strings(ops)

	fmt.Printf("%s: %d identities and services with 3 id policies each seeded in %v; %d clients x %d iterations, %d requests in %v (%.0f/s)\n",
		p.spec.name, p.spec.seedCount, seedTime.Round(time.Millisecond), p.spec.clients, p.spec.iterationsPerClient,
		total, elapsed.Round(time.Millisecond), float64(total)/elapsed.Seconds())
	fmt.Printf("%-36s %6s %9s %9s %9s %9s  %s\n", "op", "count", "min", "mean", "p95", "max", "statuses")
	for _, op := range ops {
		timer := p.timers[op]
		fmt.Printf("%-36s %6d %9s %9s %9s %9s  %v\n", op, timer.Count(),
			ms(timer.Min()), ms(int64(timer.Mean())), ms(int64(timer.Percentile(0.95))), ms(timer.Max()), p.statuses[op])
	}
}

func ms(micros int64) string {
	return fmt.Sprintf("%.2fms", float64(micros)/1000)
}
