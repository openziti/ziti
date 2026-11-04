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

package db

import (
	"sort"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"go.etcd.io/bbolt"
)

// Test_RebuildPostureCheckServiceLinks damages the posture check to service tables the way the
// previous layout left them, with a wrong count, a missing entry and a posture check id in a
// service's identity table, then runs the rebuild and checks the tables are consistent with the
// policies and the identity tables hold only identities. Running the rebuild again changes nothing.
func Test_RebuildPostureCheckServiceLinks(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()
	ctx.CleanupAll()

	identity := ctx.RequireNewIdentity(eid.New(), false)
	s1 := ctx.RequireNewService(eid.New())
	s2 := ctx.RequireNewService(eid.New())
	p1 := newTestPostureCheck()
	boltztest.RequireCreate(ctx, p1)
	p2 := newTestPostureCheck()
	boltztest.RequireCreate(ctx, p2)

	dial := ctx.newServicePolicy(PolicyTypeDial, SemanticAllOf,
		[]string{entityRef(identity.Id)}, []string{entityRef(s1.Id), entityRef(s2.Id)}, []string{entityRef(p1.Id)})
	boltztest.RequireCreate(ctx, dial)
	bind := ctx.newServicePolicy(PolicyTypeBind, SemanticAllOf,
		[]string{AllRole}, []string{entityRef(s2.Id)}, []string{entityRef(p1.Id), entityRef(p2.Id)})
	boltztest.RequireCreate(ctx, bind)

	ctx.validateServicePolicyDenormalization()

	stores := ctx.stores.internal
	err := ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		tx := mctx.Tx()
		// a wrong count on both sides of one pair
		if _, _, err := stores.postureCheck.dialServicesCollection.SetLinkCount(tx, []byte(p1.Id), []byte(s1.Id), 3); err != nil {
			return err
		}
		// a pair missing from both sides
		if _, _, err := stores.postureCheck.bindServicesCollection.SetLinkCount(tx, []byte(p2.Id), []byte(s2.Id), 0); err != nil {
			return err
		}
		// a posture check id in a service's identity table, where the previous layout wrote it
		serviceBucket := stores.service.GetEntityBucket(tx, []byte(s1.Id))
		_, err := serviceBucket.GetOrCreatePath(FieldEdgeServiceDialIdentities).SetLinkCount(boltz.TypeString, []byte(p1.Id), 1)
		return err
	})
	ctx.NoError(err)

	damaged := []string{identity.Id, p1.Id}
	sort.Strings(damaged)
	ctx.Equal(damaged, ctx.getRelatedIds(s1, FieldEdgeServiceDialIdentities))

	rebuild := func() {
		migrations := &Migrations{stores: ctx.stores}
		err := ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
			step := &boltz.MigrationStep{Component: "edge", Ctx: mctx, CurrentVersion: CurrentDbVersion - 1}
			migrations.rebuildPostureCheckServiceLinks(step)
			return step.GetError()
		})
		ctx.NoError(err)
	}

	verify := func() {
		ctx.validateServicePolicyDenormalization()
		ctx.Equal([]string{identity.Id}, ctx.getRelatedIds(s1, FieldEdgeServiceDialIdentities))
		err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
			dialCounts := stores.postureCheck.dialServicesCollection
			bindCounts := stores.postureCheck.bindServicesCollection
			ctx.Equal(int32(1), *dialCounts.GetLinkCount(tx, []byte(p1.Id), []byte(s1.Id)))
			ctx.Equal(int32(1), *dialCounts.GetLinkCount(tx, []byte(p1.Id), []byte(s2.Id)))
			ctx.Nil(dialCounts.GetLinkCount(tx, []byte(p2.Id), []byte(s1.Id)))
			ctx.Equal(int32(1), *bindCounts.GetLinkCount(tx, []byte(p1.Id), []byte(s2.Id)))
			ctx.Equal(int32(1), *bindCounts.GetLinkCount(tx, []byte(p2.Id), []byte(s2.Id)))
			ctx.Nil(bindCounts.GetLinkCount(tx, []byte(p1.Id), []byte(s1.Id)))
			return nil
		})
		ctx.NoError(err)
	}

	rebuild()
	verify()
	rebuild()
	verify()
}
