//go:build perftests

package tests

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	service2 "github.com/openziti/edge-api/rest_client_api_client/service"
	apiClientSession "github.com/openziti/edge-api/rest_client_api_client/session"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	idloader "github.com/openziti/identity"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
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
		identityCount:                16_000,
		edgeRouterCount:              100,
		servicePolicyCount:           100,
		edgeRouterPolicyCount:        100,
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

	config *ziti.Config
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

	stats := newPerfStats(ctx.TestContext, spec.config, spec.name, spec.services[0].Id, rest_model.DialBindDial)
	stats.collectStats(10)

	stats = newPerfStats(ctx.TestContext, spec.config, spec.name, spec.services[0].Id, rest_model.DialBindBind)
	stats.collectStats(10)

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
	serviceHandler := ctx.EdgeController.AppEnv.Managers.EdgeService
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
		ctx.Req.NoError(serviceHandler.Create(service))
		spec.services = append(spec.services, service)
		if (i+1)%100 == 0 {
			pfxlog.Logger().Tracef("created %v services\n", i)
		}
	}
	pfxlog.Logger().Tracef("finished creating %v services\n", spec.serviceCount)
}

func (ctx *modelPerf) createIdentities(spec *perfScenarioSpec) {
	identityHandler := ctx.EdgeController.AppEnv.Managers.Identity
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
			IdentityTypeId: string(rest_model.IdentityTypeDefault),
			IsDefaultAdmin: false,
			IsAdmin:        false,
			RoleAttributes: spec.identityAttrs[i],
		}

		enrollments := []*model.Enrollment{
			{
				BaseEntity: models.BaseEntity{},
				Method:     db.MethodEnrollOtt,
				Token:      uuid.New().String(),
			},
		}

		ctx.Req.NoError(identityHandler.CreateWithEnrollments(identity, enrollments))
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
	edgeRouterHandler := ctx.EdgeController.AppEnv.Managers.EdgeRouter
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
		ctx.Req.NoError(edgeRouterHandler.Create(edgeRouter))
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
	policyHandler := ctx.EdgeController.AppEnv.Managers.ServicePolicy
	id := eid.New()
	policy := &model.ServicePolicy{
		BaseEntity:    models.BaseEntity{Id: id},
		Name:          id,
		PolicyType:    policyType,
		IdentityRoles: identityRoles,
		ServiceRoles:  serviceRoles,
		Semantic:      db.SemanticAnyOf,
	}
	ctx.Req.NoError(policyHandler.Create(policy))
}

func (ctx *modelPerf) createEdgeRouterPolicy(identityRoles, edgeRouterRoles []string) {
	policyHandler := ctx.EdgeController.AppEnv.Managers.EdgeRouterPolicy
	id := eid.New()
	policy := &model.EdgeRouterPolicy{
		BaseEntity:      models.BaseEntity{Id: id},
		Name:            id,
		IdentityRoles:   identityRoles,
		EdgeRouterRoles: edgeRouterRoles,
		Semantic:        db.SemanticAnyOf,
	}
	ctx.NoError(policyHandler.Create(policy))
}

func (ctx *modelPerf) createServiceEdgeRouterPolicy(edgeRouterRoles, serviceRoles []string) {
	policyHandler := ctx.EdgeController.AppEnv.Managers.ServiceEdgeRouterPolicy
	id := eid.New()
	policy := &model.ServiceEdgeRouterPolicy{
		BaseEntity:      models.BaseEntity{Id: id},
		Name:            id,
		EdgeRouterRoles: edgeRouterRoles,
		ServiceRoles:    serviceRoles,
		Semantic:        db.SemanticAnyOf,
	}
	ctx.NoError(policyHandler.Create(policy))
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

func newPerfStats(ctx *TestContext, config *ziti.Config, description string, serviceId string, sessionType rest_model.DialBind) *perfStats {
	zitiUrl, err := url.Parse(config.ZtAPI)
	ctx.Req.NoError(err)

	id, err := idloader.LoadIdentity(config.ID)
	ctx.Req.NoError(err)

	creds := edge_apis.NewIdentityCredentials(id)
	creds.ConfigTypes = []string{"all"}

	caPool, err := ziti.GetControllerWellKnownCaPool(config.ZtAPI)

	ctx.Req.NoError(err)

	client := edge_apis.NewClientApiClient(zitiUrl, caPool)

	return &perfStats{
		TestContext:       ctx,
		client:            client,
		description:       description,
		serviceId:         serviceId,
		sessionType:       sessionType,
		credentials:       creds,
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
	client            *edge_apis.ClientApiClient
	sessionType       rest_model.DialBind
	sessionId         string
	createApiSession  metrics.Histogram
	refreshApiSession metrics.Histogram
	getServices       metrics.Histogram
	createSession     metrics.Histogram
	refreshSession    metrics.Histogram
	credentials       *edge_apis.IdentityCredentials
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
		_, err := s.client.Authenticate(s.credentials)
		s.Req.NoError(err)
	})
}

func (s *perfStats) timeRefreshApiSession() {
	s.time(s.createApiSession, func() {
		params := current_api_session.NewGetCurrentAPISessionParams()
		_, err := s.client.API.CurrentAPISession.GetCurrentAPISession(params, nil)
		s.Req.NoError(err)
	})
}

func (s *perfStats) timeGetServices() {
	s.time(s.getServices, func() {
		params := service2.NewListServicesParams()
		params.Limit = I(500)
		_, err := s.client.API.Service.ListServices(params, nil)
		s.Req.NoError(err)
	})
}

func (s *perfStats) timeCreateSession() {
	s.time(s.createSession, func() {
		//session, err := s.client.CreateSession(s.serviceId, s.sessionType)

		params := apiClientSession.NewCreateSessionParams()
		params.Session = &rest_model.SessionCreate{}
		params.Session.Type = s.sessionType
		params.Session.ServiceID = s.serviceId

		sessionDetail, err := s.client.API.Session.CreateSession(params, nil)

		if err != nil {
			s.Req.NoError(err)
		}

		s.sessionId = *sessionDetail.Payload.Data.ID
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
