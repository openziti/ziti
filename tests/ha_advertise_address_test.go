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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hashiraft "github.com/hashicorp/raft"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/openziti/ziti/v2/controller"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/raft"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

// Test_AdvertiseAddressReconcile_SingleNode restarts a single-node cluster with a different
// advertise address and verifies the raft configuration and a connected router's endpoints file
// pick up the new address.
func Test_AdvertiseAddressReconcile_SingleNode(t *testing.T) {
	const originalAddr = "tls:127.0.0.1:6262"
	const updatedAddr = "tls:localhost:6262"

	ctx := NewTestContextWithConfigSet(t, SingleRaft)
	defer ctx.Teardown()

	ctx.StartServerRaft()
	ctx.RequireAdminManagementApiLogin()
	ctx.requireMemberAddresses(ctx.fabricController, map[string]string{"ctrl1": originalAddr})

	endpointsFile := ctx.startRouterWithEndpointsFile()
	ctx.requireEndpointsFileAddresses(endpointsFile, originalAddr)

	ctx.restartPrimaryController("testdata/single-raft-unused.db", advertiseAddressModifier(ctx, "", updatedAddr))

	ctx.requireMemberAddresses(ctx.fabricController, map[string]string{"ctrl1": updatedAddr})
	ctx.requireEndpointsFileAddresses(endpointsFile, updatedAddr)
}

var ha3Addresses = map[string]string{
	"ctrl1": "tls:127.0.0.1:6262",
	"ctrl2": "tls:127.0.0.1:6363",
	"ctrl3": "tls:127.0.0.1:6464",
}

// Test_AdvertiseAddressReconcile_Follower restarts a follower of a three-node cluster with a new
// advertise address and verifies every member stores the new address, holds a single mesh
// connection to the follower, and replicates to it, and that the router picks up the new address.
func Test_AdvertiseAddressReconcile_Follower(t *testing.T) {
	t.Run("old address still reachable", func(t *testing.T) {
		testFollowerAdvertiseAddressChange(t, "", "tls:localhost:6464", false)
	})

	t.Run("old address unreachable", func(t *testing.T) {
		testFollowerAdvertiseAddressChange(t, "tls:127.0.0.1:6565", "tls:127.0.0.1:6565", false)
	})

	t.Run("non-voter", func(t *testing.T) {
		testFollowerAdvertiseAddressChange(t, "", "tls:localhost:6464", true)
	})
}

func testFollowerAdvertiseAddressChange(t *testing.T, listener, updatedAddr string, nonVoter bool) {
	ctx := NewTestContextWithConfigSet(t, Ha3)
	defer ctx.Teardown()

	ctx.StartHaCluster(Ha3DataDir)
	ctx.RequireAdminManagementApiLogin()
	ctx.requireLeader("ctrl1")

	raftCtrl := ctx.requireRaftController(ctx.fabricController)
	if nonVoter {
		ctx.Req.NoError(raftCtrl.GetRaft().DemoteVoter("ctrl3", 0, 0).Error())
		ctx.requireSuffrage(ctx.fabricController, "ctrl3", hashiraft.Nonvoter)
	}

	original := ha3Addresses
	endpointsFile := ctx.startRouterWithEndpointsFile()
	ctx.requireEndpointsFileAddresses(endpointsFile, mapValues(original)...)

	ctx.restartPeerController(1, advertiseAddressModifier(ctx, listener, updatedAddr), nil)

	updated := map[string]string{
		"ctrl1": original["ctrl1"],
		"ctrl2": original["ctrl2"],
		"ctrl3": updatedAddr,
	}

	for _, ctrl := range ctx.allControllers() {
		ctx.requireMemberAddresses(ctrl, updated)
	}

	for _, ctrl := range []*controller.Controller{ctx.fabricController, ctx.peerControllers[0].fabricController} {
		ctx.requireSingleMeshPeer(ctrl, "ctrl3")
	}

	if nonVoter {
		ctx.requireSuffrage(ctx.fabricController, "ctrl3", hashiraft.Nonvoter)
	}

	identity := ctx.AdminManagementSession.requireNewIdentity(false)
	ctx.Req.NoError(ctx.waitForIdentityOnPeer(1, identity.Id, 30*time.Second))

	ctx.requireEndpointsFileAddresses(endpointsFile, mapValues(updated)...)
}

// Test_AdvertiseAddressReconcile_Leader restarts the leader of a three-node cluster with a new
// advertise address, hands leadership back to it, and verifies the cluster runs under it at the
// new address.
func Test_AdvertiseAddressReconcile_Leader(t *testing.T) {
	const updatedAddr = "tls:localhost:6262"

	ctx := NewTestContextWithConfigSet(t, Ha3)
	defer ctx.Teardown()

	ctx.StartHaCluster(Ha3DataDir)
	ctx.RequireAdminManagementApiLogin()
	ctx.requireLeader("ctrl1")

	endpointsFile := ctx.startRouterWithEndpointsFile()
	ctx.requireEndpointsFileAddresses(endpointsFile, mapValues(ha3Addresses)...)

	ctx.restartPrimaryController("testdata/ha-unused.db", advertiseAddressModifier(ctx, "", updatedAddr))

	updated := map[string]string{
		"ctrl1": updatedAddr,
		"ctrl2": ha3Addresses["ctrl2"],
		"ctrl3": ha3Addresses["ctrl3"],
	}

	for _, ctrl := range ctx.allControllers() {
		ctx.requireMemberAddresses(ctrl, updated)
	}

	ctx.requireLeader("ctrl1")

	for _, peer := range ctx.peerControllers {
		ctx.requireSingleMeshPeer(peer.fabricController, "ctrl1")
	}

	ctx.RequireAdminManagementApiLogin()
	identity := ctx.AdminManagementSession.requireNewIdentity(false)
	for i := range ctx.peerControllers {
		ctx.Req.NoError(ctx.waitForIdentityOnPeer(i, identity.Id, 30*time.Second))
	}

	ctx.requireEndpointsFileAddresses(endpointsFile, mapValues(updated)...)
}

// Test_AdvertiseAddressReconcile_Refused restarts a follower with advertise addresses that must not
// be stored, an address nothing listens on and an address belonging to another member, and verifies
// the configuration is unchanged.
func Test_AdvertiseAddressReconcile_Refused(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, Ha3)
	defer ctx.Teardown()

	ctx.StartHaCluster(Ha3DataDir)
	ctx.requireLeader("ctrl1")
	raftCtrl := ctx.requireRaftController(ctx.fabricController)

	requireUnchanged := func(msg string) {
		ctx.Req.Never(func() bool {
			cfgFuture := raftCtrl.GetRaft().GetConfiguration()
			if cfgFuture.Error() != nil {
				return false
			}
			for _, srv := range cfgFuture.Configuration().Servers {
				if ha3Addresses[string(srv.ID)] != string(srv.Address) {
					return true
				}
			}
			return false
		}, 10*time.Second, 250*time.Millisecond, msg)
	}

	ctx.restartPeerController(1, advertiseAddressModifier(ctx, "", "tls:127.0.0.1:6666"), nil)
	requireUnchanged("unreachable advertise address was stored")

	ctx.restartPeerController(1, advertiseAddressModifier(ctx, "", ha3Addresses["ctrl2"]), nil)
	requireUnchanged("advertise address belonging to another member was stored")
}

// Test_AdvertiseAddressReconcile_MembershipChangedWhileOffline changes a follower's membership
// while it is offline, restarts it with a new advertise address, and verifies the address update
// does not undo the membership change.
func Test_AdvertiseAddressReconcile_MembershipChangedWhileOffline(t *testing.T) {
	const updatedAddr = "tls:localhost:6464"

	t.Run("removed", func(t *testing.T) {
		ctx := NewTestContextWithConfigSet(t, Ha3)
		defer ctx.Teardown()

		ctx.StartHaCluster(Ha3DataDir)
		ctx.requireLeader("ctrl1")
		raftCtrl := ctx.requireRaftController(ctx.fabricController)

		ctx.restartPeerController(1, advertiseAddressModifier(ctx, "", updatedAddr), func() {
			ctx.Req.NoError(raftCtrl.RemoveServer("ctrl3"))
		})

		ctx.Req.Never(func() bool {
			_, found := raftCtrl.GetMemberId(updatedAddr)
			return found || raftCtrl.IsPeerMember("ctrl3")
		}, 10*time.Second, 250*time.Millisecond, "removed member was added back")
	})

	t.Run("demoted", func(t *testing.T) {
		ctx := NewTestContextWithConfigSet(t, Ha3)
		defer ctx.Teardown()

		ctx.StartHaCluster(Ha3DataDir)
		ctx.requireLeader("ctrl1")
		raftCtrl := ctx.requireRaftController(ctx.fabricController)

		ctx.restartPeerController(1, advertiseAddressModifier(ctx, "", updatedAddr), func() {
			ctx.Req.NoError(raftCtrl.GetRaft().DemoteVoter("ctrl3", 0, 0).Error())
		})

		ctx.requireMemberAddresses(ctx.fabricController, map[string]string{
			"ctrl1": ha3Addresses["ctrl1"],
			"ctrl2": ha3Addresses["ctrl2"],
			"ctrl3": updatedAddr,
		})
		ctx.requireSuffrage(ctx.fabricController, "ctrl3", hashiraft.Nonvoter)
	})
}

// advertiseAddressModifier returns a config modifier that sets the advertise address and, if
// listener is non-empty, the ctrl listener.
func advertiseAddressModifier(ctx *TestContext, listener, advertiseAddr string) func(*config.Config) {
	return func(cfg *config.Config) {
		if listener != "" {
			addr, err := transport.ParseAddress(listener)
			ctx.Req.NoError(err)
			cfg.Ctrl.Listener = addr
		}
		addr, err := transport.ParseAddress(advertiseAddr)
		ctx.Req.NoError(err)
		cfg.Ctrl.Options.AdvertiseAddress = &addr
		cfg.Raft.AdvertiseAddress = addr
	}
}

func (ctx *TestContext) startRouterWithEndpointsFile() string {
	endpointsFile := filepath.Join(ctx.T().TempDir(), "endpoints.yml")
	ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.Ctrl.EndpointsFile = endpointsFile
	})
	return endpointsFile
}

func (ctx *TestContext) allControllers() []*controller.Controller {
	result := []*controller.Controller{ctx.fabricController}
	for _, peer := range ctx.peerControllers {
		result = append(result, peer.fabricController)
	}
	return result
}

func (ctx *TestContext) requireRaftController(ctrl *controller.Controller) *raft.Controller {
	raftCtrl, ok := ctrl.GetCommandDispatcher().(*raft.Controller)
	ctx.Req.True(ok, "controller is not running in cluster mode")
	return raftCtrl
}

// requireLeader waits until the member with the given id leads the cluster, transferring leadership
// to it if needed.
func (ctx *TestContext) requireLeader(id string) {
	raftCtrl := ctx.requireRaftController(ctx.fabricController)
	ctx.Req.EventuallyWithT(func(c *assert.CollectT) {
		_, leaderId := raftCtrl.GetRaft().LeaderWithID()
		if !assert.NotEmpty(c, leaderId) {
			return
		}
		if string(leaderId) != id {
			assert.NoError(c, raftCtrl.HandleTransferLeadership(&cmd_pb.TransferLeadershipRequest{Id: id}))
			assert.Fail(c, "leader is "+string(leaderId))
		}
	}, 30*time.Second, 250*time.Millisecond)
}

// requireSuffrage waits until ctrl's raft configuration gives the member id the expected suffrage.
func (ctx *TestContext) requireSuffrage(ctrl *controller.Controller, id string, expected hashiraft.ServerSuffrage) {
	raftCtrl := ctx.requireRaftController(ctrl)
	ctx.Req.EventuallyWithT(func(c *assert.CollectT) {
		cfgFuture := raftCtrl.GetRaft().GetConfiguration()
		if !assert.NoError(c, cfgFuture.Error()) {
			return
		}
		for _, srv := range cfgFuture.Configuration().Servers {
			if string(srv.ID) == id {
				assert.Equal(c, expected, srv.Suffrage)
				return
			}
		}
		assert.Fail(c, "member not found: "+id)
	}, 30*time.Second, 100*time.Millisecond)
}

// restartPrimaryController shuts down the primary controller and starts it again from its config
// file and existing data, applying modifier to the loaded config.
func (ctx *TestContext) restartPrimaryController(testDb string, modifier func(*config.Config)) {
	ctrlListener := ctx.ControllerConfig.Ctrl.Listener.String()

	// Controller.Shutdown leaves accepted router channels open, which a real process exit would
	// not, so close them to have routers reconnect.
	for _, r := range ctx.fabricController.GetNetwork().AllConnectedRouters() {
		ctx.Req.NoError(r.Control.Close())
	}
	ctx.EdgeController.Shutdown()
	ctx.fabricController.Shutdown()
	ctx.Req.NoError(ctx.waitForPortClose(ctx.ApiHost, 30*time.Second))
	ctx.Req.NoError(ctx.waitForPortClose(strings.TrimPrefix(ctrlListener, "tls:"), 30*time.Second))

	ctx.startServerWith(testDb, false, modifier, true)
}

// requireMemberAddresses waits until ctrl's raft configuration maps exactly the given member ids to
// addresses.
func (ctx *TestContext) requireMemberAddresses(ctrl *controller.Controller, expected map[string]string) {
	raftCtrl := ctx.requireRaftController(ctrl)
	ctx.Req.EventuallyWithT(func(c *assert.CollectT) {
		cfgFuture := raftCtrl.GetRaft().GetConfiguration()
		if !assert.NoError(c, cfgFuture.Error()) {
			return
		}
		stored := map[string]string{}
		for _, srv := range cfgFuture.Configuration().Servers {
			stored[string(srv.ID)] = string(srv.Address)
		}
		assert.Equal(c, expected, stored)
	}, 30*time.Second, 100*time.Millisecond)
}

// requireSingleMeshPeer waits until ctrl's mesh holds exactly one connection to the member id.
func (ctx *TestContext) requireSingleMeshPeer(ctrl *controller.Controller, id string) {
	raftCtrl := ctx.requireRaftController(ctrl)
	ctx.Req.EventuallyWithT(func(c *assert.CollectT) {
		var addrs []string
		for addr, peer := range raftCtrl.GetMesh().GetPeers() {
			if string(peer.Id) == id {
				addrs = append(addrs, addr)
			}
		}
		assert.Len(c, addrs, 1, "mesh peers for %s: %v", id, addrs)
	}, 30*time.Second, 100*time.Millisecond)
}

// requireEndpointsFileAddresses waits until the router endpoints file at path lists exactly addrs,
// in any order.
func (ctx *TestContext) requireEndpointsFileAddresses(path string, addrs ...string) {
	ctx.Req.EventuallyWithT(func(c *assert.CollectT) {
		contents, err := os.ReadFile(path)
		if !assert.NoError(c, err) {
			return
		}
		endpoints := struct {
			Endpoints []string `yaml:"endpoints"`
		}{}
		if !assert.NoError(c, yaml.Unmarshal(contents, &endpoints)) {
			return
		}
		assert.ElementsMatch(c, addrs, endpoints.Endpoints)
	}, 30*time.Second, 100*time.Millisecond)
}

func mapValues(m map[string]string) []string {
	var result []string
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
