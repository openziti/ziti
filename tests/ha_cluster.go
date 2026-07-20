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
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	hashiraft "github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	edgeApis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/openziti/ziti/v2/controller"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/raft"
	"github.com/openziti/ziti/v2/controller/server"
)

// peerController is a secondary cluster-member controller started by StartHaCluster. The
// primary controller remains on the TestContext fields (EdgeController, ApiHost, ...) so all
// existing helpers keep working against it; peers are addressed via their own ApiHost.
type peerController struct {
	fabricController *controller.Controller
	EdgeController   *server.Controller
	ApiHost          string
	config           *config.Config
}

// PeerControllerApiHosts returns the API host:port of each peer controller, in
// PeerCtrlConfigs order.
func (ctx *TestContext) PeerControllerApiHosts() []string {
	var result []string
	for _, peer := range ctx.peerControllers {
		result = append(result, peer.ApiHost)
	}
	return result
}

// NewEdgeClientApiForHost returns a ClientHelperClient targeting the given API host instead of
// the primary controller's. The CA pool is shared cluster-wide.
func (ctx *TestContext) NewEdgeClientApiForHost(apiHost string, totpProvider func(chan string)) *ClientHelperClient {
	if totpProvider == nil {
		totpProvider = func(chan string) {}
	}
	apiUrl, err := url.Parse("https://" + apiHost + EdgeClientApiPath)
	ctx.Req.NoError(err)

	client := edgeApis.NewClientApiClient([]*url.URL{apiUrl}, ctx.ControllerCaPool(), totpProvider)

	return &ClientHelperClient{
		ClientApiClient: client,
		testCtx:         ctx,
	}
}

// StartHaCluster starts the config set's primary controller plus every PeerCtrlConfigs member,
// joins them into a single raft cluster, and waits until every member is a voter and is
// registered in the Controller store. dataDir is the parent raft data directory shared by the
// set's controllers (e.g. Ha3DataDir); it is removed first so each run bootstraps fresh.
func (ctx *TestContext) StartHaCluster(dataDir string) *ControllerHelper {
	err := os.RemoveAll(dataDir)
	ctx.Req.NoError(err)

	helper := ctx.startServerWith("testdata/ha-unused.db", true, nil, true)

	raftCtrl, ok := ctx.fabricController.GetCommandDispatcher().(*raft.Controller)
	ctx.Req.True(ok, "primary controller is not running in cluster mode")

	for _, peerConfigFile := range ctx.configSet.PeerCtrlConfigs {
		ctx.startPeerController(peerConfigFile)
	}

	for _, peer := range ctx.peerControllers {
		ctx.Req.NotNil(peer.config.Ctrl.Options.AdvertiseAddress, "peer controller config has no ctrl advertise address")
		peerCtrlAddr := (*peer.config.Ctrl.Options.AdvertiseAddress).String()
		ctx.Req.NoError(ctx.waitForPort(strings.TrimPrefix(peerCtrlAddr, "tls:"), time.Minute))

		req := &cmd_pb.AddPeerRequest{
			Addr:    peerCtrlAddr,
			IsVoter: true,
		}
		pfxlog.Logger().WithField("addr", req.Addr).Info("joining peer controller to test cluster")

		// Adding a voter can bounce leadership while replication to the new member settles,
		// so joins are retried until the cluster has a stable leader again.
		var joinErr error
		for attempt := 0; attempt < 30; attempt++ {
			if joinErr = raftCtrl.HandleAddPeer(req); joinErr == nil {
				break
			}
			pfxlog.Logger().WithError(joinErr).WithField("addr", req.Addr).Warn("join attempt failed, retrying")
			time.Sleep(time.Second)
		}
		ctx.Req.NoError(joinErr, "could not join peer controller to test cluster")
	}

	ctx.waitForClusterReady(raftCtrl, 1+len(ctx.peerControllers), time.Minute)

	return helper
}

// startPeerController starts one secondary cluster-member controller. Unlike the primary, no
// default admin is initialized: peers get all replicated state through raft once joined.
func (ctx *TestContext) startPeerController(configFile string) {
	log := pfxlog.Logger().WithField("config", configFile)
	log.Info("starting peer controller")

	cfg, err := config.LoadConfig(configFile)
	ctx.Req.NoError(err)

	fabricController, err := controller.NewController(cfg, NewVersionProviderTest())
	ctx.Req.NoError(err)

	edgeController, err := server.NewController(fabricController)
	ctx.Req.NoError(err)

	edgeController.Initialize()
	edgeController.Run()
	go func() {
		ctx.Req.NoError(fabricController.Run())
	}()

	peer := &peerController{
		fabricController: fabricController,
		EdgeController:   edgeController,
		ApiHost:          cfg.Edge.Api.Address,
		config:           cfg,
	}
	ctx.peerControllers = append(ctx.peerControllers, peer)

	ctx.Req.NoError(ctx.waitForPort(peer.ApiHost, time.Minute))
	log.WithField("apiHost", peer.ApiHost).Info("peer controller started")
}

// waitForClusterReady polls until the raft configuration lists memberCount voters and the
// Controller store holds memberCount records, so tests observe a fully-registered cluster.
func (ctx *TestContext) waitForClusterReady(raftCtrl *raft.Controller, memberCount int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	var lastState string

	for {
		voters := 0
		configFuture := raftCtrl.GetRaft().GetConfiguration()
		if err := configFuture.Error(); err == nil {
			for _, srv := range configFuture.Configuration().Servers {
				if srv.Suffrage == hashiraft.Voter {
					voters++
				}
			}
		}

		registered := 0
		var recordDescs []string
		if result, err := ctx.EdgeController.AppEnv.Managers.Controller.BaseList("true limit none"); err == nil {
			registered = len(result.Entities)
			for _, entity := range result.Entities {
				recordDescs = append(recordDescs, fmt.Sprintf("%s(%s)", entity.Id, entity.Name))
			}
		}

		var peerCounts []int
		for _, peer := range ctx.peerControllers {
			count := -1
			if result, err := peer.EdgeController.AppEnv.Managers.Controller.BaseList("true limit none"); err == nil {
				count = len(result.Entities)
			}
			peerCounts = append(peerCounts, count)
		}

		lastState = fmt.Sprintf("voters: %v/%v, controller store records: %v/%v %v, peer store counts: %v", voters, memberCount, registered, memberCount, recordDescs, peerCounts)
		if voters >= memberCount && registered >= memberCount {
			pfxlog.Logger().Info("test cluster ready: " + lastState)
			return
		}

		if time.Now().After(deadline) {
			ctx.Req.Fail("timed out waiting for cluster to become ready", lastState)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitForIdentityOnPeer polls until the peer controller at peerIndex can read the given
// identity, covering raft replication lag after creating state via another cluster member.
func (ctx *TestContext) waitForIdentityOnPeer(peerIndex int, identityId string, timeout time.Duration) error {
	peer := ctx.peerControllers[peerIndex]
	deadline := time.Now().Add(timeout)
	for {
		_, err := peer.EdgeController.AppEnv.Managers.Identity.Read(identityId)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("identity %s did not replicate to peer controller %d within %s: %w", identityId, peerIndex, timeout, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// shutdownPeerControllers stops every peer controller started by StartHaCluster.
func (ctx *TestContext) shutdownPeerControllers() {
	for _, peer := range ctx.peerControllers {
		peer.EdgeController.Shutdown()
		peer.fabricController.Shutdown()
	}
	ctx.peerControllers = nil
}
