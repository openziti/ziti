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

	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/link"
	"github.com/openziti/ziti/v2/router/managedconfig"
)

// Test_ManagedConfig_LocalWinsOverController exercises the strict-local-wins
// semantics end-to-end. The router has a local router.link.v1 entry; the
// controller assigns a controller-source router.link.v1 config to the
// router via the management API; after RDM propagation, the router's
// registry must still show the local entry as the applied one.
//
// Seeds local config by calling registry.ApplyLocal directly rather than
// relying on YAML translation. The YAML translation path has its own
// unit-test coverage (Test_ConfigFromLocalYaml_Roundtrip); using
// ApplyLocal here avoids needing a free TCP port for an actual listener.
func Test_ManagedConfig_LocalWinsOverController(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		// Allow the controller to manage router.link configs.
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})
	registry := r.Router.GetRouterConfigRegistry()

	// Seed local. Empty router.link.v1 — no listeners/dialers, Apply is a
	// no-op that still marks local-source applied.
	ctx.Req.NoError(registry.ApplyLocal(link.ConfigTypeV1, `{}`))
	registry.WaitForIdle()

	src, ver, found := registry.Applied(link.ConfigTypeV1)
	ctx.Req.True(found, "local apply should land")
	ctx.Req.Equal(managedconfig.SourceLocal, src)
	ctx.Req.Equal(1, ver)

	// Use the management API to create a controller-source Config of the
	// built-in router.link.v1 type and assign it to the running edge
	// router via PATCH.
	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	configTypeName := "router.link.v1"
	createCfgResp, err := mgmtClient.API.Config.CreateConfig(&config.CreateConfigParams{
		Config: &rest_model.ConfigCreate{
			Name:         util.Ptr(eid.New()),
			ConfigTypeID: &configTypeName,
			Data:         map[string]interface{}{},
		},
	}, nil)
	ctx.Req.NoError(err)
	configId := createCfgResp.Payload.Data.ID

	_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
		ID: ctx.edgeRouterEntity.id,
		EdgeRouter: &rest_model.EdgeRouterPatch{
			Configs: []string{configId},
		},
	}, nil)
	ctx.Req.NoError(err)

	// Sanity: verify the PATCH actually persisted the Configs field.
	detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
		ID: ctx.edgeRouterEntity.id,
	}, nil)
	ctx.Req.NoError(err)
	ctx.Req.Equal([]string{configId}, detailResp.Payload.Data.Configs, "PATCH should set Configs on the edge router")

	// Wait for RDM propagation: poll until the registry has stashed the
	// controller-source data. Until that happens, we haven't actually
	// exercised the local-wins decision.
	ctx.Req.Eventually(func() bool {
		snap := registry.Inspect()
		for _, h := range snap.Handlers {
			if h.BaseType != "router.link" {
				continue
			}
			for _, v := range h.ControllerConfigs {
				if v.Version == 1 {
					return true
				}
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond, "controller-source config should propagate to router")

	// Strict local-wins: Applied state remains Local.
	src, _, found = registry.Applied(link.ConfigTypeV1)
	ctx.Req.True(found)
	ctx.Req.Equal(managedconfig.SourceLocal, src, "local should still own the applied state after controller push")

	// Sanity: both sources are observable via Inspect.
	snap := registry.Inspect()
	for _, h := range snap.Handlers {
		if h.BaseType == "router.link" {
			ctx.Req.NotNil(h.LocalConfig, "local config should remain present")
			ctx.Req.NotEmpty(h.ControllerConfigs, "controller-source data should be stashed even though it didn't win")
		}
	}
}
