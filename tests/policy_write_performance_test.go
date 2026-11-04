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
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_model"
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

	start := time.Now()
	p.seed()
	seedTime := time.Since(start)

	clients := make([]*ManagementHelperClient, spec.clients)
	for i := range clients {
		clients[i] = p.NewEdgeManagementApi(nil)
		_, err := clients[i].Authenticate(p.NewAdminCredentials(), nil)
		p.Req.NoError(err)
	}

	start = time.Now()
	var wg sync.WaitGroup
	for i, mgmt := range clients {
		wg.Add(1)
		go func(client int, mgmt *ManagementHelperClient) {
			defer wg.Done()
			p.runClient(client, mgmt)
		}(i, mgmt)
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

func (p *writePerf) runClient(client int, mgmt *ManagementHelperClient) {
	for i := 0; i < p.spec.iterationsPerClient; i++ {
		identityId := p.spec.identityIds[(client*p.spec.iterationsPerClient+i)%len(p.spec.identityIds)]

		start := time.Now()
		service, err := mgmt.CreateService(eid.New(), false, nil)
		p.record("service create", statusOf(err, http.StatusCreated), time.Since(start))
		if err != nil {
			continue
		}

		var policyIds []string
		for _, policyType := range []rest_model.DialBind{rest_model.DialBindBind, rest_model.DialBindDial} {
			start = time.Now()
			policy, err := mgmt.CreateServicePolicy(policyType, s("@"+service.ID), s("@"+identityId), nil)
			p.record("policy create", statusOf(err, http.StatusCreated), time.Since(start))
			if err == nil {
				policyIds = append(policyIds, policy.ID)
			}
		}

		start = time.Now()
		serp, err := mgmt.CreateServiceEdgeRouterPolicy(s("#all"), s("@"+service.ID))
		p.record("service edge router policy create", statusOf(err, http.StatusCreated), time.Since(start))
		if err == nil {
			start = time.Now()
			err = mgmt.DeleteServiceEdgeRouterPolicy(serp.ID)
			p.record("service edge router policy delete", statusOf(err, http.StatusOK), time.Since(start))
		}

		for _, policyId := range slices.Backward(policyIds) {
			start = time.Now()
			err = mgmt.DeleteServicePolicy(policyId)
			p.record("policy delete", statusOf(err, http.StatusOK), time.Since(start))
		}

		start = time.Now()
		err = mgmt.DeleteService(service.ID)
		p.record("service delete", statusOf(err, http.StatusOK), time.Since(start))
	}
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

// statusOf returns the HTTP status of a management API call: success when err is nil, the status of
// the error response otherwise, or 0 when the request failed without a response.
func statusOf(err error, success int) int {
	if err == nil {
		return success
	}
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	var apiErr *runtime.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return 0
}
