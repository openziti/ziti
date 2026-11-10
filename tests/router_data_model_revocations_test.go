//go:build apitests

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

package tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/revocation"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/sync_strats"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A controller builds its router data model from the store at startup, so every stored
// revocation has to come through that build unchanged. If one doesn't, routers syncing from a
// restarted controller stop enforcing it, and the controller's own data model validation should
// say so.
func Test_RouterDataModelRevocations(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	// keeps the revocation enforcer from deleting the expired fixture mid-test
	ctx.StartServerWithConfigModifier(func(cfg *config.Config) {
		cfg.Edge.Oidc.RevocationEnforcerFrequency = time.Hour
	})

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	identity, err := mgmtClient.CreateIdentity("rdm-revocation-"+eid.New(), false)
	ctx.Req.NoError(err)

	_, err = mgmtClient.API.Revocation.CreateRevocation(&revocation.CreateRevocationParams{
		Revocation: &rest_model.RevocationCreate{
			ID:   util.Ptr(identity.ID),
			Type: util.Ptr(rest_model.RevocationTypeEnumIDENTITY),
		},
	}, nil)
	ctx.Req.NoError(err)

	// The management API always sets a future expiry and an issued-before cutoff, so the expired
	// revocation and the one without a cutoff go in through the manager.
	appEnv := ctx.EdgeController.AppEnv
	expiredId := uuid.NewString()
	err = appEnv.Managers.Revocation.Create(&model.Revocation{
		BaseEntity: models.BaseEntity{Id: expiredId},
		ExpiresAt:  time.Now().Add(-time.Hour),
		Type:       common.RevocationTypeToken,
	}, change.New())
	ctx.Req.NoError(err)

	noCutoffIdentity, err := mgmtClient.CreateIdentity("rdm-revocation-no-cutoff-"+eid.New(), false)
	ctx.Req.NoError(err)

	err = appEnv.Managers.Revocation.Create(&model.Revocation{
		BaseEntity: models.BaseEntity{Id: noCutoffIdentity.ID},
		ExpiresAt:  time.Now().Add(time.Hour),
		Type:       common.RevocationTypeIdentity,
	}, change.New())
	ctx.Req.NoError(err)

	strategy, ok := appEnv.Broker.GetRouterSyncStrategy().(*sync_strats.InstantStrategy)
	ctx.Req.True(ok, "the controller should run the instant sync strategy")

	t.Run("a model built from the store holds the stored revocation", func(t *testing.T) {
		ctx.NextTest(t)
		stored, err := appEnv.Managers.Revocation.Read(identity.ID)
		ctx.Req.NoError(err)
		built := common.NewRouterDataModelSender(appEnv, 16, 1)

		err = strategy.BuildAll(built)
		ctx.Req.NoError(err)

		rev, found := built.Revocations.Get(identity.ID)
		ctx.Req.True(found, "a data model built from the store should hold the stored revocation")
		ctx.Req.Equal(stored.Type, rev.Type)
		ctx.Req.WithinDuration(stored.ExpiresAt, rev.ExpiresAt.AsTime(), 0)
		ctx.Req.NotNil(rev.IssuedBefore, "an identity revocation should carry its issued-before cutoff")
		ctx.Req.WithinDuration(stored.IssuedBefore, rev.IssuedBefore.AsTime(), 0)
	})

	t.Run("a model built from the store holds an expired revocation", func(t *testing.T) {
		ctx.NextTest(t)
		built := common.NewRouterDataModelSender(appEnv, 16, 1)

		err := strategy.BuildAll(built)
		ctx.Req.NoError(err)

		_, found := built.Revocations.Get(expiredId)
		ctx.Req.True(found, "an expired revocation should stay in the model until the enforcer deletes it from the store")
	})

	t.Run("a model built from the store keeps a missing issued-before cutoff as nil", func(t *testing.T) {
		ctx.NextTest(t)
		built := common.NewRouterDataModelSender(appEnv, 16, 1)

		err := strategy.BuildAll(built)
		ctx.Req.NoError(err)

		rev, found := built.Revocations.Get(noCutoffIdentity.ID)
		ctx.Req.True(found, "a data model built from the store should hold the stored revocation")
		ctx.Req.Nil(rev.IssuedBefore, "routers treat a nil cutoff as revoking every session for the identity")
	})

	t.Run("validation reports a revocation missing from the model", func(t *testing.T) {
		ctx.NextTest(t)
		built := common.NewRouterDataModelSender(appEnv, 16, 1)
		err := strategy.BuildAll(built)
		ctx.Req.NoError(err)
		ctx.Req.Empty(strategy.ValidateAll(built), "a model freshly built from the store should validate clean")
		built.Revocations.Remove(identity.ID)

		errs := strategy.ValidateAll(built)

		ctx.Req.Len(errs, 1, "the removed revocation should be the only difference from the store")
		ctx.Req.ErrorContains(errs[0], identity.ID)
	})

	t.Run("validation reports a revocation whose expiry differs from the store", func(t *testing.T) {
		ctx.NextTest(t)
		built := common.NewRouterDataModelSender(appEnv, 16, 1)
		err := strategy.BuildAll(built)
		ctx.Req.NoError(err)
		ctx.Req.Empty(strategy.ValidateAll(built), "a model freshly built from the store should validate clean")
		rev, found := built.Revocations.Get(identity.ID)
		ctx.Req.True(found)
		built.Revocations.Set(identity.ID, &edge_ctrl_pb.DataState_Revocation{
			Id:           rev.Id,
			Type:         rev.Type,
			ExpiresAt:    timestamppb.New(rev.ExpiresAt.AsTime().Add(time.Second)),
			IssuedBefore: rev.IssuedBefore,
		})

		errs := strategy.ValidateAll(built)

		ctx.Req.Len(errs, 1, "the shifted expiry should be the only difference from the store")
		ctx.Req.ErrorContains(errs[0], "expires at")
	})
}
