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
	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
)

type testEntity interface {
	getId() string
	getEntityType() string
	toJson(create bool, ctx *TestContext) string
	validate(ctx *TestContext, c *gabs.Container)
}

func newTestAppwan() *testAppwan {
	return &testAppwan{
		name:       uuid.New().String(),
		identities: []string{},
		services:   []string{},
	}
}

type testAppwan struct {
	id         string
	name       string
	identities []string
	services   []string
	tags       map[string]interface{}
}

func (entity *testAppwan) getId() string {
	return entity.id
}

func (entity *testAppwan) getEntityType() string {
	return "app-wans"
}

func (entity *testAppwan) toJson(_ bool, ctx *TestContext) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.identities, "identities")
	ctx.setJsonValue(entityData, entity.services, "services")
	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *testAppwan) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.tags, path("tags"))

}

type testService struct {
	id              string
	name            string
	dnsHostname     string
	dnsPort         int
	egressRouter    string
	endpointAddress string
	clusterIds      []string
	hostIds         []string
	tags            map[string]interface{}
}

func (entity *testService) getId() string {
	return entity.id
}

func (entity *testService) getEntityType() string {
	return "services"
}

func (entity *testService) toJson(create bool, ctx *TestContext) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.egressRouter, "egressRouter")
	ctx.setJsonValue(entityData, entity.endpointAddress, "endpointAddress")
	ctx.setJsonValue(entityData, entity.dnsHostname, "dns", "hostname")
	ctx.setJsonValue(entityData, entity.dnsPort, "dns", "port")

	if create {
		if len(entity.clusterIds) > 0 {
			ctx.setJsonValue(entityData, entity.clusterIds, "clusters")
		}
		if len(entity.hostIds) > 0 {
			ctx.setJsonValue(entityData, entity.hostIds, "hostIds")
		}
	}

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

	cluster := c.Search("clusters")
	for _, clusterId := range entity.clusterIds {
		ctx.requireChildWith(cluster, "id", clusterId)
	}
}
