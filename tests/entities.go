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
	"fmt"
	"sort"

	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
)

type entity interface {
	getId() string
	setId(string)
	getEntityType() string
	toJson(create bool, ctx *TestContext, fields ...string) string
	validate(ctx *TestContext, c *gabs.Container)
}

type service struct {
	id              string
	name            string
	dnsHostname     string
	dnsPort         int
	egressRouter    string
	endpointAddress string
	edgeRouterRoles []string
	roleAttributes  []string
	configs         []string
	permissions     []string
	tags            map[string]interface{}
}

func (entity *service) getId() string {
	return entity.id
}

func (entity *service) setId(id string) {
	entity.id = id
}

func (entity *service) getEntityType() string {
	return "services"
}

func (entity *service) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.egressRouter, "egressRouter")
	ctx.setJsonValue(entityData, entity.endpointAddress, "endpointAddress")
	ctx.setJsonValue(entityData, entity.dnsHostname, "dns", "hostname")
	ctx.setJsonValue(entityData, entity.dnsPort, "dns", "port")
	ctx.setJsonValue(entityData, entity.edgeRouterRoles, "edgeRouterRoles")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")
	ctx.setJsonValue(entityData, entity.configs, "configs")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}

	return entityData.String()
}

func (entity *service) validate(ctx *TestContext, c *gabs.Container) {
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

func newTestIdentity(isAdmin bool, roleAttributes ...string) *identity {
	return &identity{
		name:           uuid.New().String(),
		identityType:   "User",
		isAdmin:        isAdmin,
		roleAttributes: roleAttributes,
	}
}

type identity struct {
	id             string
	name           string
	identityType   string
	isAdmin        bool
	roleAttributes []string
	tags           map[string]interface{}
}

func (entity *identity) getId() string {
	return entity.id
}

func (entity *identity) setId(id string) {
	entity.id = id
}

func (entity *identity) getEntityType() string {
	return "identities"
}

func (entity *identity) toJson(_ bool, ctx *TestContext, _ ...string) string {
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

func (entity *identity) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newTestEdgeRouter(roleAttributes ...string) *edgeRouter {
	return &edgeRouter{
		name:           uuid.New().String(),
		roleAttributes: roleAttributes,
	}
}

type edgeRouter struct {
	id             string
	name           string
	roleAttributes []string
	tags           map[string]interface{}
}

func (entity *edgeRouter) getId() string {
	return entity.id
}

func (entity *edgeRouter) setId(id string) {
	entity.id = id
}

func (entity *edgeRouter) getEntityType() string {
	return "edge-routers"
}

func (entity *edgeRouter) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *edgeRouter) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newTestEdgeRouterPolicy(edgeRouterRoles, identityRoles []string) *edgeRouterPolicy {
	return &edgeRouterPolicy{
		name:            uuid.New().String(),
		edgeRouterRoles: edgeRouterRoles,
		identityRoles:   identityRoles,
	}
}

type edgeRouterPolicy struct {
	id              string
	name            string
	edgeRouterRoles []string
	identityRoles   []string
	tags            map[string]interface{}
}

func (entity *edgeRouterPolicy) getId() string {
	return entity.id
}

func (entity *edgeRouterPolicy) setId(id string) {
	entity.id = id
}

func (entity *edgeRouterPolicy) getEntityType() string {
	return "edge-router-policies"
}

func (entity *edgeRouterPolicy) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.edgeRouterRoles, "edgeRouterRoles")
	ctx.setJsonValue(entityData, entity.identityRoles, "identityRoles")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *edgeRouterPolicy) validate(ctx *TestContext, c *gabs.Container) {
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

func newTestServicePolicy(policyType string, serviceRoles, identityRoles []string) *servicePolicy {
	return &servicePolicy{
		name:          uuid.New().String(),
		policyType:    policyType,
		serviceRoles:  serviceRoles,
		identityRoles: identityRoles,
	}
}

type servicePolicy struct {
	id            string
	name          string
	policyType    string
	identityRoles []string
	serviceRoles  []string
	tags          map[string]interface{}
}

func (entity *servicePolicy) getId() string {
	return entity.id
}

func (entity *servicePolicy) setId(id string) {
	entity.id = id
}

func (entity *servicePolicy) getEntityType() string {
	return "service-policies"
}

func (entity *servicePolicy) toJson(_ bool, ctx *TestContext, _ ...string) string {
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

func (entity *servicePolicy) validate(ctx *TestContext, c *gabs.Container) {
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

type config struct {
	id         string
	configType string
	name       string
	data       map[string]interface{}
	tags       map[string]interface{}
	sendType   bool
}

func (entity *config) getId() string {
	return entity.id
}

func (entity *config) setId(id string) {
	entity.id = id
}

func (entity *config) getEntityType() string {
	return "configs"
}

func (entity *config) toJson(isCreate bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.name, fields, "name")
	if isCreate || entity.sendType {
		ctx.setValue(entityData, entity.configType, fields, "type")
	}
	ctx.setValue(entityData, entity.data, fields, "data")
	ctx.setValue(entityData, entity.tags, fields, "tags")
	return entityData.String()
}

func (entity *config) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.configType, path("type"))
	ctx.pathEquals(c, entity.data, path("data"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type configType struct {
	id   string
	name string
	tags map[string]interface{}
}

func (entity *configType) getId() string {
	return entity.id
}

func (entity *configType) setId(id string) {
	entity.id = id
}

func (entity *configType) getEntityType() string {
	return "configTypes"
}

func (entity *configType) toJson(isCreate bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.name, fields, "name")
	ctx.setValue(entityData, entity.tags, fields, "tags")
	return entityData.String()
}

func (entity *configType) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type apiSession struct {
	id          string
	token       string
	identityId  string
	configTypes []string
	tags        map[string]interface{}
}

func (entity *apiSession) getId() string {
	return entity.id
}

func (entity *apiSession) setId(id string) {
	entity.id = id
}

func (entity *apiSession) getEntityType() string {
	return "apiSessions"
}

func (entity *apiSession) toJson(_ bool, ctx *TestContext, fields ...string) string {
	ctx.req.FailNow("should not be called")
	return ""
}

func (entity *apiSession) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.token, path("token"))
	ctx.pathEquals(c, entity.identityId, path("identity", "id"))
	ctx.pathEquals(c, entity.configTypes, path("configTypes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type configValidatingService struct {
	*service
	configs map[string]*config
}

func (entity *configValidatingService) validate(ctx *TestContext, c *gabs.Container) {
	configs := c.Path("config")
	if len(entity.configs) == 0 && configs == nil {
		return
	}

	children, err := configs.Children()
	ctx.req.NoError(err)
	ctx.req.Equal(len(entity.configs), len(children))
	for configType, config := range entity.configs {
		fmt.Printf("checking if config with config type name %v is present.\n", configType)
		ctx.pathEquals(configs, config.data, path(configType))
	}
}
