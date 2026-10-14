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
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/xt"
	"github.com/stretchr/testify/require"
)

type terminatorUpdateTestHandler struct {
	network *network.Network
}

func (self *terminatorUpdateTestHandler) getAppEnv() *env.AppEnv       { return nil }
func (self *terminatorUpdateTestHandler) getNetwork() *network.Network { return self.network }
func (self *terminatorUpdateTestHandler) getChannel() channel.Channel  { return nil }
func (self *terminatorUpdateTestHandler) Label() string                { return "terminator-update-test" }

// Test_CreateTerminatorV2RequestContext_applyToExisting covers a create for a terminator id the
// controller already has, which is how a router re-establishes a terminator. Every value the create
// carries must replace the stored one, or a change made while the terminator existed never lands.
func Test_CreateTerminatorV2RequestContext_applyToExisting(t *testing.T) {
	testCtx := model.NewTestContext(t)
	defer testCtx.Cleanup()
	testCtx.Init()

	managers := testCtx.GetManagers()

	service := &model.EdgeService{Name: uuid.NewString()}
	require.NoError(t, managers.EdgeService.Create(service, change.New()))

	edgeRouter := &model.EdgeRouter{Name: uuid.NewString()}
	require.NoError(t, managers.EdgeRouter.Create(edgeRouter, change.New()))

	handler := &terminatorUpdateTestHandler{network: &network.Network{Managers: managers}}

	createTerminator := func(peerData map[uint32][]byte) string {
		id := uuid.NewString()
		require.NoError(t, managers.Terminator.Create(&model.Terminator{
			BaseEntity: models.BaseEntity{Id: id},
			Service:    service.Id,
			Router:     edgeRouter.Id,
			Binding:    common.EdgeBinding,
			Address:    id,
			Cost:       10,
			Precedence: xt.Precedences.Default,
			PeerData:   peerData,
		}, change.New()))
		return id
	}

	read := func(id string) *model.Terminator {
		terminator, err := managers.Terminator.Read(id)
		require.NoError(t, err)
		return terminator
	}

	apply := func(id string, cost uint32, precedence edge_ctrl_pb.TerminatorPrecedence, peerData map[uint32][]byte) *model.Terminator {
		requestCtx := &CreateTerminatorV2RequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: handler},
			req:                       &edge_ctrl_pb.CreateTerminatorV2Request{Cost: cost, Precedence: precedence, PeerData: peerData},
		}
		require.NoError(t, requestCtx.applyToExisting(read(id), change.New()))
		return read(id)
	}

	t.Run("unchanged values are not written", func(t *testing.T) {
		req := require.New(t)
		id := createTerminator(map[uint32][]byte{1: []byte("key1")})
		before := read(id)

		after := apply(id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{1: []byte("key1")})

		req.Equal(before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("nil and empty peer data are the same", func(t *testing.T) {
		req := require.New(t)
		id := createTerminator(nil)
		before := read(id)

		after := apply(id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{})

		req.Equal(before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("new peer data replaces the stored peer data", func(t *testing.T) {
		req := require.New(t)
		id := createTerminator(map[uint32][]byte{1: []byte("key1")})

		after := apply(id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, map[uint32][]byte{1: []byte("key2")})

		req.Equal("key2", string(after.PeerData[1]))
	})

	t.Run("absent peer data clears the stored peer data", func(t *testing.T) {
		req := require.New(t)
		id := createTerminator(map[uint32][]byte{1: []byte("key1")})

		after := apply(id, 10, edge_ctrl_pb.TerminatorPrecedence_Default, nil)

		req.Empty(after.PeerData)
	})

	t.Run("cost and precedence are applied", func(t *testing.T) {
		req := require.New(t)
		id := createTerminator(map[uint32][]byte{1: []byte("key1")})

		after := apply(id, 20, edge_ctrl_pb.TerminatorPrecedence_Failed, map[uint32][]byte{1: []byte("key1")})

		req.Equal(uint16(20), after.Cost)
		req.Equal(xt.Precedences.Failed, after.Precedence)
		req.Equal("key1", string(after.PeerData[1]))
	})
}
