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
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

func Test_ClusterStore(t *testing.T) {
	ctx := &TestContext{ReferenceTime: time.Now()}
	defer ctx.Cleanup()
	ctx.Init()
	req := require.New(t)
	req.NoError(ctx.err)

	t.Run("test create clusters", ctx.testCreateCluster)
	t.Run("test update clusters", ctx.testUpdateCluster)
	t.Run("test delete clusters", ctx.testDeleteCluster)
	t.Run("test query clusters", ctx.testQueryCluster)
}

func (ctx *TestContext) testCreateCluster(_ *testing.T) {
	ctx.cleanupAll()

	cluster := NewCluster(uuid.New().String())
	ctx.requireCreate(cluster)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		load := &Cluster{}
		ctx.validateBaseline(cluster, load)
		ctx.Equal(0, len(ctx.stores.Cluster.GetRelatedEntitiesIdList(tx, cluster.Id, FieldClusterEdgeRouters)))

		testCluster, err := ctx.stores.Cluster.LoadOneByName(tx, cluster.Name)
		ctx.NoError(err)
		ctx.NotNil(testCluster)
		ctx.Equal(cluster.Name, testCluster.Name)

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateCluster(t *testing.T) {
	ctx.cleanupAll()

	clusterId := uuid.New().String()
	clusterName1 := uuid.New().String()
	clusterName2 := uuid.New().String()
	cluster := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: clusterId},
		Name:               clusterName1,
	}

	req := require.New(t)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		return ctx.stores.Cluster.Create(boltz.NewMutateContext(tx), cluster)
	})
	req.NoError(err)

	var createTime time.Time

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testCluster, err := ctx.stores.Cluster.LoadOneById(tx, clusterId)
		req.NoError(err)
		req.NotNil(testCluster)
		req.Equal(clusterName1, testCluster.Name)
		req.True(testCluster.CreatedAt.After(ctx.ReferenceTime) || testCluster.CreatedAt.Equal(ctx.ReferenceTime))
		req.NotNil(testCluster.UpdatedAt)
		req.Equal(testCluster.CreatedAt, testCluster.UpdatedAt)
		createTime = testCluster.CreatedAt

		testCluster, err = ctx.stores.Cluster.LoadOneByName(tx, clusterName1)
		req.NoError(err)
		req.NotNil(testCluster)
		req.Equal(clusterName1, testCluster.Name)

		return nil
	})
	req.NoError(err)

	time.Sleep(time.Millisecond * 5) // ensure that update is after create

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		testCluster, err := ctx.stores.Cluster.LoadOneById(tx, clusterId)
		req.NoError(err)
		testCluster.Name = clusterName2
		return ctx.stores.Cluster.Update(boltz.NewMutateContext(tx), testCluster, nil)
	})
	req.NoError(err)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testCluster, err := ctx.stores.Cluster.LoadOneById(tx, clusterId)
		req.NoError(err)
		req.NotNil(testCluster)
		req.Equal(clusterName2, testCluster.Name)
		req.Equal(createTime, testCluster.CreatedAt)
		req.NotEqual(testCluster.CreatedAt, testCluster.UpdatedAt)
		req.True(testCluster.UpdatedAt.After(testCluster.CreatedAt))

		testCluster, err = ctx.stores.Cluster.LoadOneByName(tx, clusterName2)
		req.NoError(err)
		req.NotNil(testCluster)
		req.Equal(clusterName2, testCluster.Name)

		testCluster, err = ctx.stores.Cluster.LoadOneByName(tx, clusterName1)
		req.NoError(err)
		req.Nil(testCluster)

		return nil
	})

	req.NoError(err)
}

func (ctx *TestContext) testDeleteCluster(t *testing.T) {
	ctx.cleanupAll()

	clusterId := uuid.New().String()
	clusterName := uuid.New().String()
	cluster := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: clusterId},
		Name:               clusterName,
	}

	req := require.New(t)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		return ctx.stores.Cluster.Create(boltz.NewMutateContext(tx), cluster)
	})
	req.NoError(err)

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		return ctx.stores.Cluster.DeleteById(boltz.NewMutateContext(tx), clusterId)
	})
	req.NoError(err)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testCluster, err := ctx.stores.Cluster.LoadOneById(tx, clusterId)
		req.NoError(err)
		req.Nil(testCluster)

		testCluster, err = ctx.stores.Cluster.LoadOneByName(tx, clusterName)
		req.NoError(err)
		req.Nil(testCluster)

		return nil
	})
	req.NoError(err)
}

func (ctx *TestContext) testQueryCluster(t *testing.T) {
	req := require.New(t)
	ctx.cleanupAll()

	cluster1 := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{
			Id: uuid.New().String(),
			EdgeEntityFields: EdgeEntityFields{
				Tags: map[string]interface{}{
					"location": "Chicago",
					"enabled":  true,
					"capacity": 100,
				},
			},
		},
		Name: "alpha",
	}

	cluster2 := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{
			Id: uuid.New().String(),
			EdgeEntityFields: EdgeEntityFields{
				Tags: map[string]interface{}{
					"location": "Chicago",
					"enabled":  false,
					"capacity": 200,
				},
			},
		},
		Name: "beta",
	}

	cluster3 := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{
			Id: uuid.New().String(),
			EdgeEntityFields: EdgeEntityFields{
				Tags: map[string]interface{}{
					"location": "Springville",
					"enabled":  true,
					"capacity": 300,
				},
			},
		},
		Name: "gamma",
	}

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		if err := ctx.stores.Cluster.Create(mutateContext, cluster1); err != nil {
			return err
		}
		if err := ctx.stores.Cluster.Create(mutateContext, cluster2); err != nil {
			return err
		}
		return ctx.stores.Cluster.Create(mutateContext, cluster3)
	})
	req.NoError(err)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ids, count, err := ctx.stores.Cluster.QueryIds(tx, `name != "gamma"`)
		req.NoError(err)
		req.Equal(int(2), int(count))
		req.Equal(int(2), len(ids))
		strIds := stringz.ToStringSlice(ids)
		req.True(stringz.Contains(strIds, cluster1.Id))
		req.True(stringz.Contains(strIds, cluster2.Id))
		return nil
	})
	req.NoError(err)
}