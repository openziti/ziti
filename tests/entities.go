// +build apitests

/*
	Copyright 2019 Netfoundry, Inc.

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
	"sort"

	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
)

type testEntity interface {
	getId() string
	setId(string)
	getEntityType() string
	toJson(create bool, ctx *TestContext, fields ...string) string
	validate(ctx *TestContext, c *gabs.Container)
}

type testService struct {
	id              string
	name            string
	dnsHostname     string
	dnsPort         int
	egressRouter    string
	endpointAddress string
	edgeRouterRoles []string
	roleAttributes  []string
	permissions     []string
	tags            map[string]interface{}
}

func (entity *testService) getId() string {
	return entity.id
}

func (entity *testService) setId(id string) {
	entity.id = id
}

func (entity *testService) getEntityType() string {
	return "services"
}

func (entity *testService) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.egressRouter, "egressRouter")
	ctx.setJsonValue(entityData, entity.endpointAddress, "endpointAddress")
	ctx.setJsonValue(entityData, entity.dnsHostname, "dns", "hostname")
	ctx.setJsonValue(entityData, entity.dnsPort, "dns", "port")
	ctx.setJsonValue(entityData, entity.edgeRouterRoles, "edgeRouterRoles")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}

	return entityData.String()
}

func (entity *testService) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.egressRouter, path("egressRouter"))
	ctx.pathEquals(c, entity.endpointAddress, path("endpointAddress"))
	ctx.pathEquals(c, entity.dnsHostname, path("dns.hostname"))
	ctx.pathEquals(c, float64(entity.dnsPort), path("dns.port"))
	ctx.pathEquals(c, entity.tags, path("tags"))

	sort.Strings(entity.edgeRouterRoles)
	ctx.pathEqualsStringSlice(c, entity.edgeRouterRoles, path("edgeRouterRoles"))

	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))

	sort.Strings(entity.permissions)
	ctx.pathEqualsStringSlice(c, entity.permissions, path("permissions"))
}

func newTestIdentity(isAdmin bool, roleAttributes ...string) *testIdentity {
	return &testIdentity{
		name:           uuid.New().String(),
		identityType:   "User",
		isAdmin:        isAdmin,
		roleAttributes: roleAttributes,
	}
}

type testIdentity struct {
	id             string
	name           string
	identityType   string
	isAdmin        bool
	roleAttributes []string
	tags           map[string]interface{}
}

func (entity *testIdentity) getId() string {
	return entity.id
}

func (entity *testIdentity) setId(id string) {
	entity.id = id
}

func (entity *testIdentity) getEntityType() string {
	return "identities"
}

func (entity *testIdentity) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.identityType, "type")
	ctx.setJsonValue(entityData, entity.isAdmin, "isAdmin")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")

	enrollments := map[string]interface{}{
		"updb": entity.name,
	}
	ctx.setJsonValue(entityData, enrollments, "enrollment")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *testIdentity) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newTestEdgeRouter(roleAttributes ...string) *testEdgeRouter {
	return &testEdgeRouter{
		name:           uuid.New().String(),
		roleAttributes: roleAttributes,
	}
}

type testEdgeRouter struct {
	id             string
	name           string
	roleAttributes []string
	tags           map[string]interface{}
}

func (entity *testEdgeRouter) getId() string {
	return entity.id
}

func (entity *testEdgeRouter) setId(id string) {
	entity.id = id
}

func (entity *testEdgeRouter) getEntityType() string {
	return "edge-routers"
}

func (entity *testEdgeRouter) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *testEdgeRouter) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newTestEdgeRouterPolicy(edgeRouterRoles, identityRoles []string) *testEdgeRouterPolicy {
	return &testEdgeRouterPolicy{
		name:            uuid.New().String(),
		edgeRouterRoles: edgeRouterRoles,
		identityRoles:   identityRoles,
	}
}

type testEdgeRouterPolicy struct {
	id              string
	name            string
	edgeRouterRoles []string
	identityRoles   []string
	tags            map[string]interface{}
}

func (entity *testEdgeRouterPolicy) getId() string {
	return entity.id
}

func (entity *testEdgeRouterPolicy) setId(id string) {
	entity.id = id
}

func (entity *testEdgeRouterPolicy) getEntityType() string {
	return "edge-router-policies"
}

func (entity *testEdgeRouterPolicy) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.edgeRouterRoles, "edgeRouterRoles")
	ctx.setJsonValue(entityData, entity.identityRoles, "identityRoles")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *testEdgeRouterPolicy) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	sort.Strings(entity.edgeRouterRoles)
	ctx.pathEqualsStringSlice(c, entity.edgeRouterRoles, path("edgeRouterRoles"))
	sort.Strings(entity.identityRoles)
	ctx.pathEqualsStringSlice(c, entity.identityRoles, path("identityRoles"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newTestServicePolicy(policyType string, serviceRoles, identityRoles []string) *testServicePolicy {
	return &testServicePolicy{
		name:          uuid.New().String(),
		policyType:    policyType,
		serviceRoles:  serviceRoles,
		identityRoles: identityRoles,
	}
}

type testServicePolicy struct {
	id            string
	name          string
	policyType    string
	identityRoles []string
	serviceRoles  []string
	tags          map[string]interface{}
}

func (entity *testServicePolicy) getId() string {
	return entity.id
}

func (entity *testServicePolicy) setId(id string) {
	entity.id = id
}

func (entity *testServicePolicy) getEntityType() string {
	return "service-policies"
}

func (entity *testServicePolicy) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.policyType, "type")
	ctx.setJsonValue(entityData, entity.identityRoles, "identityRoles")
	ctx.setJsonValue(entityData, entity.serviceRoles, "serviceRoles")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *testServicePolicy) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.policyType, path("type"))
	sort.Strings(entity.identityRoles)
	ctx.pathEqualsStringSlice(c, entity.identityRoles, path("identityRoles"))
	sort.Strings(entity.serviceRoles)
	ctx.pathEqualsStringSlice(c, entity.serviceRoles, path("serviceRoles"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type testConfig struct {
	id   string
	name string
	data map[string]interface{}
	tags map[string]interface{}
}

func (entity *testConfig) getId() string {
	return entity.id
}

func (entity *testConfig) setId(id string) {
	entity.id = id
}

func (entity *testConfig) getEntityType() string {
	return "configs"
}

func (entity *testConfig) toJson(_ bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.name, fields, "name")
	ctx.setValue(entityData, entity.data, fields, "data")
	ctx.setValue(entityData, entity.tags, fields, "tags")
	return entityData.String()
}

func (entity *testConfig) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.data, path("data"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}
