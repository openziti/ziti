//go:build perftests
// +build perftests

package tests

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/models"
	idloader "github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/sdk-golang/ziti/edge/api"
	"github.com/openziti/sdk-golang/ziti/sdkinfo"
	"github.com/rcrowley/go-metrics"
	"io"
	"net/url"
	"os"
	"testing"
	"time"
)

type modelPerf struct {
	*TestContext
}

func Test_SpecManyService(t *testing.T) {
	spec := &perfScenarioSpec{
		name:                         "many-services",
		serviceCount:                 10_000,
		identityCount:                10_000,
		edgeRouterCount:              100,
		servicePolicyCount:           2500,
		edgeRouterPolicyCount:        250,
		serviceEdgeRouterPolicyCount: 100,
	}

	p := &modelPerf{TestContext: NewTestContext(t)}
	p.runScenario(spec)
}

func Test_SpecLarge(t *testing.T) {
	spec := &perfScenarioSpec{
		name:                         "large",
		serviceCount:                 2000,
		identityCount:                100_000,
		edgeRouterCount:              500,
		servicePolicyCount:           250,
		edgeRouterPolicyCount:        250,
		serviceEdgeRouterPolicyCount: 100,
	}

	p := &modelPerf{TestContext: NewTestContext(t)}
	p.runScenario(spec)
}

func Test_SpecMedium(t *testing.T) {
	spec := &perfScenarioSpec{
		name:                         "medium",
		serviceCount:                 100,
		identityCount:                5000,
		edgeRouterCount:              100,
		servicePolicyCount:           50,
		edgeRouterPolicyCount:        50,
		serviceEdgeRouterPolicyCount: 25,
	}

	p := &modelPerf{TestContext: NewTestContext(t)}
	p.runScenario(spec)
}

func Test_SpecCurrent(t *testing.T) {
	spec := &perfScenarioSpec{
		name:                         "medium",
		serviceCount:                 2,
		identityCount:                10000,
		edgeRouterCount:              2,
		servicePolicyCount:           1,
		edgeRouterPolicyCount:        1,
		serviceEdgeRouterPolicyCount: 1,
	}

	p := &modelPerf{TestContext: NewTestContext(t)}
	p.runScenario(spec)
}

func Test_SpecSmall(t *testing.T) {
	spec := &perfScenarioSpec{
		name:                         "small",
		serviceCount:                 20,
		identityCount:                100,
		edgeRouterCount:              10,
		servicePolicyCount:           10,
		edgeRouterPolicyCount:        10,
		serviceEdgeRouterPolicyCount: 10,
	}

	p := &modelPerf{TestContext: NewTestContext(t)}
	p.runScenario(spec)
}

func Test_SpecBaseline(t *testing.T) {
	spec := &perfScenarioSpec{
		name:                         "baseline",
		serviceCount:                 1,
		identityCount:                1,
		edgeRouterCount:              1,
		servicePolicyCount:           1,
		edgeRouterPolicyCount:        1,
		serviceEdgeRouterPolicyCount: 1,
	}

	p := &modelPerf{TestContext: NewTestContext(t)}
	p.runScenario(spec)
}

type perfScenarioSpec struct {
	name string

	serviceCount    int
	identityCount   int
	edgeRouterCount int

	servicePolicyCount           int
	edgeRouterPolicyCount        int
	serviceEdgeRouterPolicyCount int

	serviceAttrs      [][]string
	serviceAttrSet    []string
	identityAttrs     [][]string
	identityAttrSet   []string
	edgeRouterAttrs   [][]string
	edgeRouterAttrSet []string

	services    []*model.Service
	identities  []*model.Identity
	edgeRouters []*model.EdgeRouter

	config *config.Config
}

func (spec *perfScenarioSpec) generateAllRoleAttributes() {
	spec.serviceAttrs, spec.serviceAttrSet = spec.generateRoleAttributes(spec.serviceCount)
	spec.identityAttrs, spec.identityAttrSet = spec.generateRoleAttributes(spec.identityCount)
	spec.edgeRouterAttrs, spec.edgeRouterAttrSet = spec.generateRoleAttributes(spec.edgeRouterCount)
}

func (spec *perfScenarioSpec) generateRoleAttributes(count int) ([][]string, []string) {
	result := make([][]string, count)
	if count < 2 {
		return result, nil
	}
	var attrs []string

	set := result[1:]

	// assign 8 role attributes
	offset := 2
	index := 0

	counts := []int{count / 2, count / 2, count / 3, count / 4, count / 5, count / 8, count / 10, count / 10}
	attr := eid.New()
	attrs = append(attrs, "#"+attr)
	for len(counts) > 0 {
		set[index] = append(set[index], attr)
		counts[0]--
		for len(counts) > 0 && counts[0] <= 0 {
			counts = counts[1:]
			if len(counts) > 0 && counts[0] > 0 {
				if len(counts) < 4 {
					offset++
				}
				attr = eid.New()
				attrs = append(attrs, "#"+attr)
			}
		}
		index = (index + offset) % len(set)
	}

	return result, attrs
}

func (ctx *modelPerf) runScenario(spec *perfScenarioSpec) {
	shutdown := false
	defer func() {
		if !shutdown {
			ctx.Teardown()
		}
	}()

	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	ctx.createScenario(spec)

	stats := newPerfStats(ctx.TestContext, spec.config, spec.name, spec.services[0].Id, edge.SessionDial)
	stats.collectStats(100)

	stats = newPerfStats(ctx.TestContext, spec.config, spec.name, spec.services[0].Id, edge.SessionDial)
	stats.collectStats(100)

	ctx.Teardown()
	shutdown = true
	time.Sleep(time.Second)

	stats.dumpStatsToStdOut()
}

// For policies, want the following, using service policies as an example
// a. 1 service policy which has 1 id and all services
// b. 1 service policy which has all ids and 1 service
// c. 1 identity with all policies (which must be the same as the one in line a)
// d. 1 service with all policies (which must the same as the one in line b)
//
// randomly assign role attributes  to 1/2, 1/4, 1/8, then do permutations of those with both anyof and allof
// When more groups are needed, create more sets of 1/2, 1/4, 1/8 and 1/16
func (ctx *modelPerf) createScenario(spec *perfScenarioSpec) {
	spec.generateAllRoleAttributes()
	ctx.createServices(spec)
	ctx.createIdentities(spec)
	ctx.createEdgeRouters(spec)
	ctx.createPolicies(spec)
}

func (ctx *modelPerf) createServices(spec *perfScenarioSpec) {
	serviceHandler := ctx.EdgeController.AppEnv.Handlers.EdgeService
	for i := 0; i < spec.serviceCount; i++ {
		id := eid.New()
		if i == 0 {
			id = "zzzzzzzzzzzzzz"
		}
		service := &model.Service{
			BaseEntity: models.BaseEntity{
				Id: id,
			},
			Name:           id,
			RoleAttributes: spec.serviceAttrs[i],
		}
		_, err := serviceHandler.Create(service)
		ctx.Req.NoError(err)
		service.Id = id
		spec.services = append(spec.services, service)
		if (i+1)%100 == 0 {
			pfxlog.Logger().Tracef("created %v services\n", i)
		}
	}
	pfxlog.Logger().Tracef("finished creating %v services\n", spec.serviceCount)
}

func (ctx *modelPerf) createIdentities(spec *perfScenarioSpec) {
	identityHandler := ctx.EdgeController.AppEnv.Handlers.Identity
	for i := 0; i < spec.identityCount; i++ {
		id := eid.New()
		if i == 0 {
			id = "zzzzzzzzzzzzzz"
		}
		identity := &model.Identity{
			BaseEntity: models.BaseEntity{
				Id: id,
			},
			Name:           id,
			IdentityTypeId: "User",
			IsDefaultAdmin: false,
			IsAdmin:        false,
			RoleAttributes: spec.identityAttrs[i],
		}

		enrollments := []*model.Enrollment{
			{
				BaseEntity: models.BaseEntity{},
				Method:     persistence.MethodEnrollOtt,
				Token:      uuid.New().String(),
			},
		}

		_, _, err := identityHandler.CreateWithEnrollments(identity, enrollments)
		ctx.Req.NoError(err)
		spec.identities = append(spec.identities, identity)

		if i == 0 {
			spec.config = ctx.EnrollIdentity(identity.Id)
		}
		if (i+1)%100 == 0 {
			pfxlog.Logger().Tracef("created %v identities\n", i+1)
		}
	}
	pfxlog.Logger().Tracef("finished creating %v identities\n", spec.identityCount)
}

func (ctx *modelPerf) createEdgeRouters(spec *perfScenarioSpec) {
	edgeRouterHandler := ctx.EdgeController.AppEnv.Handlers.EdgeRouter
	for i := 0; i < spec.edgeRouterCount; i++ {
		id := eid.New()
		if i == 0 {
			id = "zzzzzzzzzzzzzz"
		}
		edgeRouter := &model.EdgeRouter{
			BaseEntity:     models.BaseEntity{Id: id},
			Name:           id,
			RoleAttributes: spec.edgeRouterAttrs[i],
			IsVerified:     false,
		}
		_, err := edgeRouterHandler.Create(edgeRouter)
		ctx.Req.NoError(err)
		spec.edgeRouters = append(spec.edgeRouters, edgeRouter)
		if (i+1)%100 == 0 {
			pfxlog.Logger().Tracef("created %v edge routers\n", i+1)
		}
	}
	fmt.Printf("finished creating %v edge routers\n", spec.edgeRouterCount)
}

func (ctx *modelPerf) createPolicies(spec *perfScenarioSpec) {
	ctx.createServicePolicy("Dial", s("@"+spec.services[0].Id), s("#all"))
	ctx.createServicePolicy("Dial", s("#all"), s("@"+spec.identities[0].Id))

	serviceRoles := ctx.firstNPermuations(spec.servicePolicyCount, spec.serviceAttrSet)
	identityRoles := ctx.firstNPermuations(spec.servicePolicyCount, spec.identityAttrSet)
	for i := 2; i < spec.servicePolicyCount; i++ {
		serviceRoles[i] = append(serviceRoles[i], "@"+spec.services[0].Id)
		identityRoles[i] = append(identityRoles[i], "@"+spec.identities[0].Id)
		ctx.createServicePolicy("Dial", serviceRoles[i], identityRoles[i])
	}

	ctx.createEdgeRouterPolicy(s("@"+spec.edgeRouters[0].Id), s("#all"))
	ctx.createEdgeRouterPolicy(s("#all"), s("@"+spec.identities[0].Id))

	edgeRouterRoles := ctx.firstNPermuations(spec.edgeRouterPolicyCount, spec.edgeRouterAttrSet)
	identityRoles = ctx.firstNPermuations(spec.edgeRouterPolicyCount, spec.identityAttrSet)
	for i := 2; i < spec.edgeRouterPolicyCount; i++ {
		edgeRouterRoles[i] = append(edgeRouterRoles[i], "@"+spec.edgeRouters[0].Id)
		identityRoles[i] = append(identityRoles[i], "@"+spec.identities[0].Id)
		ctx.createEdgeRouterPolicy(edgeRouterRoles[i], identityRoles[i])
	}

	ctx.createServiceEdgeRouterPolicy(s("@"+spec.edgeRouters[0].Id), s("#all"))
	ctx.createServiceEdgeRouterPolicy(s("#all"), s("@"+spec.services[0].Id))

	serviceRoles = ctx.firstNPermuations(spec.serviceEdgeRouterPolicyCount, spec.serviceAttrSet)
	edgeRouterRoles = ctx.firstNPermuations(spec.serviceEdgeRouterPolicyCount, spec.edgeRouterAttrSet)
	for i := 2; i < spec.serviceEdgeRouterPolicyCount; i++ {
		serviceRoles[i] = append(serviceRoles[i], "@"+spec.services[0].Id)
		edgeRouterRoles[i] = append(edgeRouterRoles[i], "@"+spec.edgeRouters[0].Id)
		ctx.createServiceEdgeRouterPolicy(edgeRouterRoles[i], serviceRoles[i])
	}
}

func (ctx *modelPerf) createServicePolicy(policyType string, identityRoles, serviceRoles []string) {
	policyHandler := ctx.EdgeController.AppEnv.Handlers.ServicePolicy
	id := eid.New()
	policy := &model.ServicePolicy{
		BaseEntity:    models.BaseEntity{Id: id},
		Name:          id,
		PolicyType:    policyType,
		IdentityRoles: identityRoles,
		ServiceRoles:  serviceRoles,
		Semantic:      persistence.SemanticAnyOf,
	}
	_, err := policyHandler.Create(policy)
	ctx.Req.NoError(err)
}

func (ctx *modelPerf) createEdgeRouterPolicy(identityRoles, edgeRouterRoles []string) {
	policyHandler := ctx.EdgeController.AppEnv.Handlers.EdgeRouterPolicy
	id := eid.New()
	policy := &model.EdgeRouterPolicy{
		BaseEntity:      models.BaseEntity{Id: id},
		Name:            id,
		IdentityRoles:   identityRoles,
		EdgeRouterRoles: edgeRouterRoles,
		Semantic:        persistence.SemanticAnyOf,
	}
	_, err := policyHandler.Create(policy)
	ctx.Req.NoError(err)
}

func (ctx *modelPerf) createServiceEdgeRouterPolicy(edgeRouterRoles, serviceRoles []string) {
	policyHandler := ctx.EdgeController.AppEnv.Handlers.ServiceEdgeRouterPolicy
	id := eid.New()
	policy := &model.ServiceEdgeRouterPolicy{
		BaseEntity:      models.BaseEntity{Id: id},
		Name:            id,
		EdgeRouterRoles: edgeRouterRoles,
		ServiceRoles:    serviceRoles,
		Semantic:        persistence.SemanticAnyOf,
	}
	_, err := policyHandler.Create(policy)
	ctx.Req.NoError(err)
}

func (ctx *modelPerf) firstNPermuations(n int, v []string) [][]string {
	var result [][]string
	ctx.permutations(v, func(strings []string) bool {
		result = append(result, strings)
		return len(result) < n
	})
	// if we don't have enough permutations, just copy
	idx := 0
	for len(result) < n {
		result = append(result, result[idx])
		idx++
	}
	return result
}

func (ctx *modelPerf) permutations(v []string, f func([]string) bool) {
	ctx.permutationWith([]string{}, v, f)
}

func (ctx *modelPerf) permutationWith(base, v []string, f func([]string) bool) bool {
	for i := 0; i < len(v); i++ {
		var result []string
		if len(base) > 0 {
			result = append(result, base...)
		}
		result = append(result, v[i])
		if !f(result) {
			return false
		}
		if !ctx.permutationWith(result, v[i+1:], f) {
			return false
		}
	}
	return true
}

func newHistogram() metrics.Histogram {
	return metrics.NewHistogram(metrics.NewExpDecaySample(128, 0.015))
}

func newPerfStats(ctx *TestContext, config *config.Config, description string, serviceId string, sessionType edge.SessionType) *perfStats {
	zitiUrl, err := url.Parse(config.ZtAPI)
	ctx.Req.NoError(err)

	id, err := idloader.LoadIdentity(config.ID)
	ctx.Req.NoError(err)

	client, err := api.NewClient(zitiUrl, id.ClientTLSConfig(), s("all"))
	ctx.Req.NoError(err)

	return &perfStats{
		TestContext:       ctx,
		client:            client,
		description:       description,
		serviceId:         serviceId,
		sessionType:       sessionType,
		createApiSession:  newHistogram(),
		refreshApiSession: newHistogram(),
		getServices:       newHistogram(),
		createSession:     newHistogram(),
		refreshSession:    newHistogram(),
	}
}

type perfStats struct {
	*TestContext
	description       string
	serviceId         string
	client            api.RestClient
	sessionType       edge.SessionType
	sessionId         string
	createApiSession  metrics.Histogram
	refreshApiSession metrics.Histogram
	getServices       metrics.Histogram
	createSession     metrics.Histogram
	refreshSession    metrics.Histogram
}

func (s *perfStats) dumpStatsToStdOut() {
	errWriter := &WriterWrapper{
		Writer: os.Stdout,
	}
	s.dumpStats(errWriter)
	s.Req.NoError(errWriter.GetError())
}

func (s *perfStats) dumpStats(w ErrorWriter) {
	w.Println(s.description)
	w.Println("=======================================")
	s.logHistogram(w, "Create API Session", s.createApiSession)
	s.logHistogram(w, "Refresh API Session", s.refreshApiSession)
	s.logHistogram(w, "Get Services", s.getServices)
	s.logHistogram(w, "Create Session", s.createSession)
	s.logHistogram(w, "Refresh Session", s.refreshSession)
}

func (s *perfStats) logHistogram(w ErrorWriter, name string, h metrics.Histogram) {
	w.Printf("%v:\n", name)
	w.Printf("\tMin  : %vms\n", h.Min())
	w.Printf("\tMax  : %vms\n", h.Max())
	w.Printf("\tMean : %vms\n", h.Mean())
	w.Printf("\t95th : %vms\n", h.Percentile(.95))
}

func (s *perfStats) collectStats(iterations int) {
	s.repeat(iterations, s.timeCreateApiSession)
	s.repeat(iterations, s.timeRefreshApiSession)
	s.repeat(iterations, s.timeGetServices)
	s.repeat(iterations, s.timeCreateSession)
	s.repeat(iterations, s.timeRefreshSession)
}

func (s *perfStats) repeat(n int, f func()) {
	for i := 0; i < n; i++ {
		f()
	}
}

func (s *perfStats) time(h metrics.Histogram, f func()) {
	start := time.Now()
	f()
	h.Update(time.Now().Sub(start).Milliseconds())
}

func (s *perfStats) timeCreateApiSession() {
	s.time(s.createApiSession, func() {
		info := sdkinfo.GetSdkInfo()
		_, err := s.client.Login(info)
		s.Req.NoError(err)
	})
}

func (s *perfStats) timeRefreshApiSession() {
	s.time(s.createApiSession, func() {
		_, err := s.client.Refresh()
		s.Req.NoError(err)
	})
}

func (s *perfStats) timeGetServices() {
	s.time(s.getServices, func() {
		_, err := s.client.GetServices()
		s.Req.NoError(err)
	})
}

func (s *perfStats) timeCreateSession() {
	s.time(s.createSession, func() {
		session, err := s.client.CreateSession(s.serviceId, s.sessionType)
		s.Req.NoError(err)
		s.sessionId = session.Id
	})
}

func (s *perfStats) timeRefreshSession() {
	s.time(s.createSession, func() {
		_, err := s.client.RefreshSession(s.sessionId)
		s.Req.NoError(err)
	})
}

type ErrorWriter interface {
	errorz.ErrorHolder
	Write([]byte) int
	Print(string)
	Println(string)
	Printf(s string, args ...interface{})
}

type WriterWrapper struct {
	errorz.ErrorHolderImpl
	io.Writer
}

func (w *WriterWrapper) Print(s string) {
	w.Write([]byte(s))
}

func (w *WriterWrapper) Println(s string) {
	w.Write([]byte(s))
	w.Write([]byte("\n"))
}

func (w *WriterWrapper) Printf(s string, args ...interface{}) {
	w.Write([]byte(fmt.Sprintf(s, args...)))
}

func (w *WriterWrapper) Write(b []byte) int {
	if !w.HasError() {
		n, err := w.Writer.Write(b)
		w.SetError(err)
		return n
	}
	return 0
}
