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

package sync_strats

import (
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// TestLoadRevocations verifies that rebuilding the router data model from the
// store loads unexpired revocations and skips expired ones. It guards the
// regression where the rebuild omitted revocations entirely, which dropped
// revocation enforcement on routers after a controller restart or leadership
// change and stranded stale revocations on routers.
func TestLoadRevocations(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	now := time.Unix(1700000000, 0)

	const validId = "revocation-valid"
	const expiredId = "revocation-expired"

	boltztest.RequireCreate(ctx, &db.Revocation{
		BaseExtEntity: *boltz.NewExtEntity(validId, nil),
		Type:          "IDENTITY",
		ExpiresAt:     now.Add(time.Hour),
		IssuedBefore:  now,
	})
	boltztest.RequireCreate(ctx, &db.Revocation{
		BaseExtEntity: *boltz.NewExtEntity(expiredId, nil),
		Type:          "IDENTITY",
		ExpiresAt:     now.Add(-time.Hour),
	})

	rdm := common.NewRouterDataModelSender(stubTimelineSource("test-timeline"), 16, 1)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return loadRevocations(ctx.GetStores().Revocation, tx, rdm, now)
	})
	require.NoError(t, err)

	t.Run("loads an unexpired revocation", func(t *testing.T) {
		rev, found := rdm.Revocations.Get(validId)
		require.True(t, found, "expected the unexpired revocation to be loaded into the data model")
		require.Equal(t, "IDENTITY", rev.Type)
		require.NotNil(t, rev.IssuedBefore)
		require.Equal(t, now.Unix(), rev.IssuedBefore.AsTime().Unix())
	})

	t.Run("skips an expired revocation", func(t *testing.T) {
		_, found := rdm.Revocations.Get(expiredId)
		require.False(t, found, "expected the expired revocation to be skipped")
	})
}
