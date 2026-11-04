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

package handler_edge_ctrl

import (
	"testing"

	"github.com/google/uuid"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/stretchr/testify/require"
)

type terminatorUpdateTestHandler struct {
	network *network.Network
}

func (self *terminatorUpdateTestHandler) getAppEnv() *env.AppEnv       { return nil }
func (self *terminatorUpdateTestHandler) getNetwork() *network.Network { return self.network }
func (self *terminatorUpdateTestHandler) getChannel() channel.Channel  { return nil }
func (self *terminatorUpdateTestHandler) Label() string                { return "terminator-update-test" }

// followerDispatcher reports this controller as not being a caught-up leader, so its local view of
// the model may trail the cluster's.
type followerDispatcher struct {
	command.Dispatcher
}

func (followerDispatcher) IsLeaderAndCaughtUp() bool { return false }

// terminatorUpdateTest stores terminators and provides request handlers that see this controller as a
// caught-up leader or as a follower.
type terminatorUpdateTest struct {
	t        *testing.T
	managers *model.Managers
	service  *model.EdgeService
	router   *model.EdgeRouter
	leader   requestHandler
	follower requestHandler
}

func newTerminatorUpdateTest(t *testing.T) *terminatorUpdateTest {
	testCtx := model.NewTestContext(t)
	t.Cleanup(testCtx.Cleanup)
	testCtx.Init()

	managers := testCtx.GetManagers()

	service := &model.EdgeService{Name: uuid.NewString()}
	require.NoError(t, managers.EdgeService.Create(service, change.New()))

	edgeRouter := &model.EdgeRouter{Name: uuid.NewString()}
	require.NoError(t, managers.EdgeRouter.Create(edgeRouter, change.New()))

	followerManagers := *managers
	followerManagers.Dispatcher = followerDispatcher{Dispatcher: managers.Dispatcher}

	return &terminatorUpdateTest{
		t:        t,
		managers: managers,
		service:  service,
		router:   edgeRouter,
		leader:   &terminatorUpdateTestHandler{network: &network.Network{Managers: managers}},
		follower: &terminatorUpdateTestHandler{network: &network.Network{Managers: &followerManagers}},
	}
}

// createTerminator stores a terminator with cost 10, default precedence and peerData, returning its id.
func (self *terminatorUpdateTest) createTerminator(peerData map[uint32][]byte) string {
	id := uuid.NewString()
	require.NoError(self.t, self.managers.Terminator.Create(&model.Terminator{
		BaseEntity: models.BaseEntity{Id: id},
		Service:    self.service.Id,
		Router:     self.router.Id,
		Binding:    common.EdgeBinding,
		Address:    id,
		Cost:       10,
		Precedence: xt.Precedences.Default,
		PeerData:   peerData,
	}, change.New()))
	return id
}

func (self *terminatorUpdateTest) read(id string) *model.Terminator {
	terminator, err := self.managers.Terminator.Read(id)
	require.NoError(self.t, err)
	return terminator
}

// Test_CreateTerminatorV2RequestContext_applyToExisting covers a create for a terminator id the
// controller already has, which is how a router re-establishes a terminator. Every value the create
// carries must replace the stored one, or a change made while the terminator existed never lands.
func Test_CreateTerminatorV2RequestContext_applyToExisting(t *testing.T) {
	test := newTerminatorUpdateTest(t)

	apply := func(handler requestHandler, id string, cost uint32, precedence edge_ctrl_pb.TerminatorPrecedence, peerData map[uint32][]byte) *model.Terminator {
		requestCtx := &CreateTerminatorV2RequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: handler},
			req:                       &edge_ctrl_pb.CreateTerminatorV2Request{Cost: cost, Precedence: precedence, PeerData: peerData},
		}
		require.NoError(t, requestCtx.applyToExisting(test.read(id), change.New()))
		return test.read(id)
	}

	t.Run("a caught-up leader does not write unchanged values", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(map[uint32][]byte{1: []byte("key1")})
		before := test.read(id)

		after := apply(test.leader, id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{1: []byte("key1")})

		req.Equal(before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("nil and empty peer data are the same", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(nil)
		before := test.read(id)

		after := apply(test.leader, id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{})

		req.Equal(before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("any other controller writes unchanged values", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(map[uint32][]byte{1: []byte("key1")})
		before := test.read(id)

		after := apply(test.follower, id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{1: []byte("key1")})

		req.NotEqual(before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("new peer data replaces the stored peer data", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(map[uint32][]byte{1: []byte("key1")})

		after := apply(test.leader, id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{1: []byte("key2")})

		req.Equal("key2", string(after.PeerData[1]))
	})

	t.Run("absent peer data clears the stored peer data", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(map[uint32][]byte{1: []byte("key1")})

		after := apply(test.leader, id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, nil)

		req.Empty(after.PeerData)
	})

	t.Run("cost and precedence are applied", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(map[uint32][]byte{1: []byte("key1")})

		after := apply(test.leader, id, 20, edge_ctrl_pb.TerminatorPrecedence_Failed, map[uint32][]byte{1: []byte("key1")})

		req.Equal(uint16(20), after.Cost)
		req.Equal(xt.Precedences.Failed, after.Precedence)
		req.Equal("key1", string(after.PeerData[1]))
	})
}

// Test_updateTerminator_unchangedValues covers an sdk re-sending values the terminator already has,
// which it does each time a bind is confirmed. A caught-up leader skips the write, since it would cost
// a raft entry. Any other controller's copy may be stale, so it writes them regardless.
func Test_updateTerminator_unchangedValues(t *testing.T) {
	test := newTerminatorUpdateTest(t)

	update := func(handler requestHandler, id string, request *edge_ctrl_pb.UpdateTerminatorRequest) *model.Terminator {
		requestCtx := &baseSessionRequestContext{handler: handler}
		requestCtx.updateTerminator(test.read(id), request, change.New())
		require.NoError(t, requestCtx.err)
		return test.read(id)
	}

	unchanged := &edge_ctrl_pb.UpdateTerminatorRequest{
		UpdateCost:       true,
		Cost:             10,
		UpdatePrecedence: true,
		Precedence:       edge_ctrl_pb.TerminatorPrecedence_Default,
	}

	t.Run("a caught-up leader does not write unchanged values", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(nil)
		before := test.read(id)

		after := update(test.leader, id, unchanged)

		req.Equal(before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("any other controller writes unchanged values", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(nil)
		before := test.read(id)

		after := update(test.follower, id, unchanged)

		req.NotEqual(before.UpdatedAt, after.UpdatedAt)
		req.Equal(uint16(10), after.Cost)
		req.Equal(xt.Precedences.Default, after.Precedence)
	})

	t.Run("a caught-up leader writes a changed value", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(nil)

		after := update(test.leader, id, &edge_ctrl_pb.UpdateTerminatorRequest{
			UpdateCost:       true,
			Cost:             10,
			UpdatePrecedence: true,
			Precedence:       edge_ctrl_pb.TerminatorPrecedence_Failed,
		})

		req.Equal(uint16(10), after.Cost)
		req.Equal(xt.Precedences.Failed, after.Precedence)
	})
}

// Test_createTunnelTerminatorV2RequestContext_readExistingTerminator covers a tunnel create for a
// terminator the controller may already have. Only a caught-up leader's copy can show that it exists;
// anywhere else the create is dispatched so the leader decides.
func Test_createTunnelTerminatorV2RequestContext_readExistingTerminator(t *testing.T) {
	test := newTerminatorUpdateTest(t)

	readExisting := func(handler requestHandler, id string) *model.Terminator {
		requestCtx := &createTunnelTerminatorV2RequestContext{
			baseTunnelRequestContext: baseTunnelRequestContext{
				baseSessionRequestContext: baseSessionRequestContext{handler: handler},
			},
			req: &edge_ctrl_pb.CreateTunnelTerminatorRequestV2{Address: id},
		}
		return requestCtx.readExistingTerminator()
	}

	t.Run("a caught-up leader uses the terminator it holds", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(nil)

		terminator := readExisting(test.leader, id)

		req.NotNil(terminator)
		req.Equal(id, terminator.Id)
	})

	t.Run("any other controller leaves existence to the leader", func(t *testing.T) {
		req := require.New(t)
		id := test.createTerminator(nil)

		req.Nil(readExisting(test.follower, id))
	})

	t.Run("a terminator the leader doesn't hold is created", func(t *testing.T) {
		req := require.New(t)

		req.Nil(readExisting(test.leader, uuid.NewString()))
	})
}
