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

package persistence

import (
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"math/rand"
	"testing"
	"time"
)

func Test_EdgeServiceStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()
	req := require.New(t)
	req.NoError(ctx.err)

	t.Run("test service parent child relationship", ctx.testServiceParentChild)
	t.Run("test create invalid api services", ctx.testCreateInvalidServices)
	t.Run("test create service", ctx.testCreateServices)
	t.Run("test load/query services", ctx.testLoadQueryServices)
	t.Run("test update services", ctx.testUpdateServices)
	t.Run("test delete services", ctx.testDeleteServices)
}

func (ctx *TestContext) testServiceParentChild(_ *testing.T) {
	fabricService := &network.Service{
		Id:              uuid.New().String(),
		Binding:         uuid.New().String(),
		EndpointAddress: uuid.New().String(),
		Egress:          uuid.New().String(),
	}

	ctx.requireCreate(fabricService)
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.False(ctx.stores.EdgeService.IsEntityPresent(tx, fabricService.GetId()))

		_, count, err := ctx.stores.EdgeService.QueryIds(tx, "true")
		ctx.NoError(err)
		ctx.Equal(int64(0), count)
		return nil
	})
	ctx.NoError(err)

	edgeService := &EdgeService{
		Service:     *fabricService,
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     0,
	}

	ctx.requireCreate(edgeService)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		query := fmt.Sprintf(`binding = "%v" and name = "%v"`, fabricService.Binding, edgeService.Name)
		ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
		if err != nil {
			return err
		}
		ctx.Equal(1, len(ids))
		ctx.Equal(fabricService.Id, string(ids[0]))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testCreateInvalidServices(_ *testing.T) {
	defer ctx.cleanupAll()

	identity := ctx.requireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	ctx.requireCreate(apiSession)

	cluster := ctx.requireNewCluster(uuid.New().String())

	edgeService := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         uuid.New().String(),
			EndpointAddress: uuid.New().String(),
			Egress:          uuid.New().String(),
		},
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     uint16(rand.Uint32()),
	}

	edgeService.HostIds = []string{"invalid-id"}
	err := ctx.create(edgeService)
	ctx.EqualError(err, "can't link to unknown identities with id invalid-id")

	edgeService.HostIds = []string{identity.Id}
	edgeService.Clusters = []string{"invalid-id"}
	err = ctx.create(edgeService)
	ctx.EqualError(err, "can't link to unknown clusters with id invalid-id")

	edgeService.Clusters = []string{cluster.Id}
	ctx.requireCreate(edgeService)
	err = ctx.create(edgeService)
	ctx.EqualError(err, fmt.Sprintf("an entity of type services already exists with id %v", edgeService.Id))
}

func (ctx *TestContext) testCreateServices(_ *testing.T) {
	defer ctx.cleanupAll()

	edgeService := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         uuid.New().String(),
			EndpointAddress: uuid.New().String(),
			Egress:          uuid.New().String(),
		},
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     uint16(rand.Uint32()),
	}
	ctx.requireCreate(edgeService)
	ctx.validateBaseline(edgeService, &EdgeService{})
}

type serviceTestEntities struct {
	cluster1    *Cluster
	identity1   *Identity
	apiSession1 *ApiSession
	service1    *EdgeService
	service2    *EdgeService
	session1    *Session
	session2    *Session
}

func (ctx *TestContext) createServiceTestEntities() *serviceTestEntities {
	cluster1 := ctx.requireNewCluster(uuid.New().String())
	identity1 := ctx.requireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)
	ctx.requireCreate(apiSession1)

	service1 := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         uuid.New().String(),
			EndpointAddress: uuid.New().String(),
			Egress:          uuid.New().String(),
		},
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     uint16(rand.Uint32()),
		Clusters:    []string{cluster1.Id},
		HostIds:     []string{identity1.Id},
	}

	ctx.requireCreate(service1)
	service2 := ctx.requireNewService(uuid.New().String())

	session1 := NewSession(apiSession1.Id, service1.Id)
	ctx.requireCreate(session1)

	session2 := NewSession(apiSession1.Id, service2.Id)
	ctx.requireCreate(session2)

	return &serviceTestEntities{
		cluster1:    cluster1,
		identity1:   identity1,
		apiSession1: apiSession1,
		service1:    service1,
		service2:    service2,
		session1:    session1,
		session2:    session2,
	}
}

func (ctx *TestContext) testLoadQueryServices(_ *testing.T) {
	ctx.cleanupAll()

	entities := ctx.createServiceTestEntities()

	err := ctx.db.View(func(tx *bbolt.Tx) error {
		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		service, err = ctx.stores.EdgeService.LoadOneByName(tx, entities.service1.Name)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(sessions) = "%v"`, entities.session1.Id)
		ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, string(ids[0]))

		query = fmt.Sprintf(`anyOf(clusters) = "%v"`, entities.cluster1.Id)
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, string(ids[0]))

		query = fmt.Sprintf(`anyOf(hostingIdentities) = "%v"`, entities.identity1.Id)
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, string(ids[0]))

		query = fmt.Sprintf(`anyOf(sessions.apiSession) = "%v"`, entities.apiSession1.Id)
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		strIds := stringz.ToStringSlice(ids)
		ctx.True(stringz.Contains(strIds, entities.service1.Id))
		ctx.True(stringz.Contains(strIds, entities.service2.Id))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateServices(_ *testing.T) {
	ctx.cleanupAll()
	entities := ctx.createServiceTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.db.Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)

		tags := ctx.createTags()
		now := time.Now()
		service.Name = uuid.New().String()
		service.Binding = uuid.New().String()
		service.EndpointAddress = uuid.New().String()
		service.Egress = uuid.New().String()
		service.UpdatedAt = earlier
		service.CreatedAt = now
		service.Tags = tags

		err = ctx.stores.EdgeService.Update(boltz.NewMutateContext(tx), service, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		service.CreatedAt = loaded.CreatedAt
		service.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(service, loaded), cmp.Diff(service, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteServices(_ *testing.T) {
	ctx.cleanupAll()
	entities := ctx.createServiceTestEntities()
	ctx.requireDelete(entities.service1)
	ctx.requireDelete(entities.service2)
}
