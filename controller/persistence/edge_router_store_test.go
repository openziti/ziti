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
	"fmt"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"testing"
)

func Test_EdgeRouterStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	clusterStore := ctx.stores.Cluster

	cluster1Id := uuid.New().String()
	cluster1 := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: cluster1Id},
		Name:               "testCluster1",
	}

	cluster2Id := uuid.New().String()
	cluster2 := &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: cluster2Id},
		Name:               "My Cluster",
	}

	edgeRouterId := uuid.New().String()
	edgeRouter := &EdgeRouter{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: edgeRouterId},
		Name:               "edgeRouter1",
		ClusterId:          &cluster1Id,
	}

	req := require.New(t)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		err := clusterStore.Create(mutateContext, cluster1)
		req.NoError(err)
		return ctx.stores.Cluster.Create(mutateContext, cluster2)
	})
	req.NoError(err)

	fmt.Printf("Created cluster1 with id: %v\n", cluster1.Id)
	fmt.Printf("Created cluster2 with id: %v\n", cluster2.Id)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testCluster, err := clusterStore.LoadOneById(tx, cluster1Id)
		req.NoError(err)
		req.NotNil(testCluster)
		erIds := clusterStore.GetRelatedEntitiesIdList(tx, testCluster.Id, EntityTypeEdgeRouters)
		req.Equal(0, len(erIds))
		return nil
	})

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		return ctx.stores.EdgeRouter.Create(boltz.NewMutateContext(tx), edgeRouter)
	})
	req.NoError(err)
	fmt.Printf("Created edge router 1 with id: %v\n", edgeRouterId)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testGw, err := ctx.stores.EdgeRouter.LoadOneById(tx, edgeRouterId)
		req.NoError(err)

		req.NotNil(testGw)
		req.Equal("edgeRouter1", testGw.Name)
		req.NotNil(testGw.CreatedAt)
		req.NotNil(testGw.UpdatedAt)
		req.Equal(cluster1Id, *testGw.ClusterId)

		testCluster, err := ctx.stores.Cluster.LoadOneById(tx, cluster1Id)
		req.NoError(err)
		req.NotNil(testCluster)
		erIds := clusterStore.GetRelatedEntitiesIdList(tx, testCluster.Id, EntityTypeEdgeRouters)
		req.Equal(1, len(erIds))
		req.Equal(edgeRouterId, erIds[0])
		return nil
	})
	req.NoError(err)

	// make sure we can't delete cluster with edge routers assigned
	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		return ctx.stores.Cluster.DeleteById(boltz.NewMutateContext(tx), cluster1Id)
	})
	req.Error(err, fmt.Sprintf("cannot delete cluster %v, which has edge routers assigned to it", cluster1Id))

	// make sure cluster is still there
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testCluster, err := ctx.stores.Cluster.LoadOneById(tx, cluster1Id)
		req.NoError(err)
		req.NotNil(testCluster)
		erIds := clusterStore.GetRelatedEntitiesIdList(tx, testCluster.Id, EntityTypeEdgeRouters)
		req.Equal(1, len(erIds))
		req.Equal(edgeRouterId, erIds[0])
		return nil
	})
	req.NoError(err)
}
