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

	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

// Test_RouterConnected_adoptsSubscribeCreatedSender covers a subscribe that beats
// RouterConnected and creates the sender for the connecting channel first. The connect path must
// take that sender over and still do its work: skipping it as a duplicate leaves the router with no
// server hello, so it never replies with a client hello and never synchronizes, while still being
// reported online with no protocols and no sync status.
func Test_RouterConnected_adoptsSubscribeCreatedSender(t *testing.T) {
	req := require.New(t)

	strategy := &InstantStrategy{
		rtxMap:               newRouterTxMap(),
		routerConnectedQueue: make(chan *RouterSender, 1),
	}

	ch := &stubCtrlChannel{id: "current"}
	router := &model.Router{BaseEntity: models.BaseEntity{Id: "r1"}, Control: ch}

	// as receiveSubscribeRequest leaves it: online and subscribed, but never queued for a hello
	existing := newStubSender("r1", ch, false)
	existing.SupportsRouterModel = true
	existing.SetIsOnline(true)
	seedSender(strategy.rtxMap, existing)

	strategy.RouterConnected(router)

	req.Same(existing, strategy.rtxMap.Get("r1"), "the sender for the connecting channel must be adopted, not replaced")
	req.False(isStopped(existing), "an adopted sender must not be stopped")
	req.True(existing.isEdge(), "the connect path is authoritative for edge classification")
	req.Equal(env.RouterSyncQueued, existing.SyncStatus())
	req.True(existing.GetState().IsOnline)

	select {
	case queued := <-strategy.routerConnectedQueue:
		req.Same(existing, queued, "the adopted sender must be queued so the hello gets sent")
	case <-time.After(2 * time.Second):
		t.Fatal("adopted sender was never queued on routerConnectedQueue")
	}
}

func Test_RouterConnected_ignoresClosedChannel(t *testing.T) {
	req := require.New(t)

	strategy := &InstantStrategy{
		rtxMap:               newRouterTxMap(),
		routerConnectedQueue: make(chan *RouterSender, 1),
	}

	ch := &stubCtrlChannel{id: "closed", closed: true}
	strategy.RouterConnected(&model.Router{BaseEntity: models.BaseEntity{Id: "r1"}, Control: ch})

	req.Nil(strategy.rtxMap.Get("r1"), "no sender should be created for a closed channel")
	req.Len(strategy.routerConnectedQueue, 0, "nothing should be queued for a closed channel")
}
