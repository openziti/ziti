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

package handler_ctrl

import (
	"errors"
	"maps"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test_updateCtrlAddressesHandler_ControllerLists locks in how controller lists change the router's
// controller set. Only a leader's list at a newer index removes controllers, since a follower that hasn't
// caught up can send a list omitting current members.
func Test_updateCtrlAddressesHandler_ControllerLists(t *testing.T) {
	t.Run("a follower's list adds the controllers it names and removes none", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl2", followerList(6, "ctrl1", "ctrl4"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl4"}, ctrls.ids(),
			"a follower's list must add the controllers it names and keep the ones it omits")
	})

	t.Run("stale follower lists around the leader's list remove nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)

		// act: restarted followers send lists naming only the leader, before and after the leader's list
		receive(t, handler, "ctrl2", followerList(1927, "ctrl1"))
		flush(t, handler)
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		receive(t, handler, "ctrl3", followerList(1932, "ctrl1"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "a stale follower list must not remove controllers")
		req.Equal(uint64(3432), handler.currentVersion, "the leader's list must be the recorded version")
		req.Equal("ctrl1", ctrls.leader())
	})

	t.Run("a newer leader's list replaces the controller set", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "a newer leader's list must remove the controllers it omits")
		req.Equal(uint64(10), handler.currentVersion)

		t.Run("an older leader's list is ignored", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl1", leaderList(9, 0, "ctrl1", "ctrl2", "ctrl3"))
			flush(t, handler)

			// assert
			req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "a leader's list below the recorded version must be ignored")
		})
	})

	t.Run("a leader's list at the recorded version restores missing controllers", func(t *testing.T) {
		req := require.New(t)

		// arrange: the router has lost ctrl2 and ctrl3
		ctrls := newCtrlListControllers("ctrl1")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1"))
		flush(t, handler)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the leader's list must restore missing controllers")

		t.Run("a leader's list at the recorded version removes none", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2"))
			flush(t, handler)

			// assert
			req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "only a newer leader's list may remove controllers")
		})
	})

	t.Run("after an index reset a leader's list at a lower index replaces the set", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// act
		handler.NotifyIndexReset()
		receive(t, handler, "ctrl1", leaderList(5, 0, "ctrl1", "ctrl2"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
		req.Equal(uint64(5), handler.currentVersion)
	})

	t.Run("a follower's list keeps connected controllers the router has no details for", func(t *testing.T) {
		req := require.New(t)

		// arrange: a router started from bare endpoints, with one controller it dialed and one that dialed in
		ctrls := newCtrlListControllers()
		ctrls.connected["ctrl2"] = &connectedCtrl{address: "tls:ctrl2:6262"}
		ctrls.connected["ctrl3"] = &connectedCtrl{address: "10.0.0.3:51234", ctrlCh: ctrlchan.NewListenerCtrlChannel()}
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl2", followerList(5, "ctrl1"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids())
		req.Contains(ctrls.GetAll(), "ctrl2", "the dialed controller must not be dropped")
		req.Contains(ctrls.GetAll(), "ctrl3", "the inbound controller must not be dropped")
		req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "a dialed controller must keep the address it was dialed at")
		req.Empty(ctrls.detail("ctrl3").Endpoints, "an inbound controller's socket address must not become an endpoint")
	})

	t.Run("a connected controller without a detail takes the incoming list's detail", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers()
		ctrls.connected["ctrl2"] = &connectedCtrl{address: "10.0.0.2:51234", ctrlCh: ctrlchan.NewListenerCtrlChannel()}
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl3", followerList(5, "ctrl1", "ctrl2"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
		req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "the list's detail must win over the connection's address")
	})

	t.Run("a controller detail without endpoints takes the incoming list's endpoints", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl3")
		ctrls.details["ctrl2"] = &ctrl_pb.CtrlDetail{Id: "ctrl2"}
		ctrls.details["ctrl3"] = &ctrl_pb.CtrlDetail{Id: "ctrl3", Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: "tls:ctrl3-old:6262"}}}
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl3", followerList(5, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "the empty detail must be filled in")
		req.Equal("tls:ctrl3-old:6262", ctrls.detail("ctrl3").Endpoints[0].Address, "a follower's list must not replace endpoints")
	})
}

// Test_updateCtrlAddressesHandler_Authoritative locks in that only a leader's list at a newer index is
// applied as the complete controller set. Every other update only adds, so a controller it omits must
// still be accepted when it connects.
func Test_updateCtrlAddressesHandler_Authoritative(t *testing.T) {
	t.Run("a follower's list is not authoritative", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl2", followerList(5, "ctrl1", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]bool{false}, ctrls.updates(), "a follower's list is not the complete set")
	})

	t.Run("a newer leader's list is authoritative", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2"))
		flush(t, handler)

		// assert
		req.Equal([]bool{true}, ctrls.updates(), "a newer leader's list is the complete set")

		t.Run("a leader's list at the recorded version is not authoritative", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2", "ctrl4"))
			flush(t, handler)

			// assert
			req.Equal([]bool{true, false}, ctrls.updates(), "a leader's list at the recorded version only adds")
		})
	})

	t.Run("a follower's list above the recorded version is not authoritative", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2"))
		flush(t, handler)

		// act
		receive(t, handler, "ctrl2", followerList(11, "ctrl1", "ctrl5"))
		flush(t, handler)

		// assert
		req.Equal([]bool{true, false}, ctrls.updates(), "a follower's list after the leader's only adds")
	})

	t.Run("a new address is not authoritative", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting("tls:ctrl1-new:6262"))
		flush(t, handler)

		// assert
		req.Equal([]bool{false}, ctrls.updates(), "a new address is not the complete set")
	})
}

// Test_updateCtrlAddressesHandler_WaitingUpdates locks in that receiving an update only queues it, and
// that updates waiting while another is applied are applied in the order they arrived, with the result
// applying each as it arrived would have.
func Test_updateCtrlAddressesHandler_WaitingUpdates(t *testing.T) {
	t.Run("receiving a list doesn't wait for an update being applied", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		var deliveries []delivery
		for i := range 100 {
			msg, err := ctrlMsg(handler, followerList(uint64(6+i), "ctrl1"))
			req.NoError(err)
			deliveries = append(deliveries, delivery{from: []string{"ctrl2", "ctrl3"}[i%2], msg: msg})
		}
		msg, err := ctrlMsg(handler, leaderList(200, 0, "ctrl1", "ctrl2", "ctrl4"))
		req.NoError(err)
		deliveries = append(deliveries, delivery{from: "ctrl1", msg: msg})
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		received := make(chan struct{})
		go receiveAll(handler, deliveries, received)

		// assert
		requireClosed(t, received, "receiving a list waited for another controller's update")
		held.release()
		flush(t, handler)
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl4"}, ctrls.ids(), "the leader's list must be applied")
		req.Equal(uint64(200), handler.currentVersion)
	})

	t.Run("an index reset applies between the updates received around it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2", "ctrl3"))
		handler.NotifyIndexReset()
		receive(t, handler, "ctrl2", leaderList(5, 0, "ctrl1", "ctrl2"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the list after the reset must be applied")
		req.Equal(uint64(5), handler.currentVersion)
	})

	t.Run("one controller's leader lists apply on their own side of an index reset", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2", "ctrl3"))
		handler.NotifyIndexReset()
		receive(t, handler, "ctrl1", leaderList(5, 0, "ctrl1", "ctrl2"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the list after the reset must be applied")
		req.Equal(uint64(5), handler.currentVersion)
	})

	t.Run("a controller's follower list doesn't replace its waiting leader list", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(20, 0, "ctrl1", "ctrl2"))
		receive(t, handler, "ctrl1", followerList(15, "ctrl1"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the leader's list must be applied")
		req.Equal(uint64(20), handler.currentVersion)
	})

	t.Run("a controller's follower list after its waiting leader list still adds", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act: ctrl1 sends its leader list, then, no longer leader, a newer follower list naming ctrl4
		receive(t, handler, "ctrl1", leaderList(20, 0, "ctrl1", "ctrl2"))
		receive(t, handler, "ctrl1", followerList(25, "ctrl1", "ctrl2", "ctrl4"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl4"}, ctrls.ids(), "the leader's removal and the follower's addition must both apply")
		req.Equal(uint64(20), handler.currentVersion)
	})

	t.Run("a later stale follower list keeps an earlier list's addition", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(5, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl2", followerList(6, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
		receive(t, handler, "ctrl2", followerList(3, "ctrl1"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl4"}, ctrls.ids(), "the earlier list's addition must be kept")
	})

	t.Run("each waiting follower list is checked against the recorded version", func(t *testing.T) {
		req := require.New(t)

		// arrange: the leader's list at 20 has removed ctrl3
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(20, 0, "ctrl1", "ctrl2"))
		flush(t, handler)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act: only the stale list names ctrl3, though a later list from the same follower is current
		receive(t, handler, "ctrl2", followerList(15, "ctrl1", "ctrl2", "ctrl3"))
		receive(t, handler, "ctrl2", followerList(25, "ctrl1", "ctrl2"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "a controller the leader removed must not come back from a stale list")
	})

	t.Run("of a controller's waiting leader lists the one at the higher index is applied", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(20, 0, "ctrl1", "ctrl2"))
		receive(t, handler, "ctrl1", leaderList(18, 0, "ctrl1", "ctrl2", "ctrl3"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the lower leader's list must be ignored though it arrived last")
		req.Equal(uint64(20), handler.currentVersion)
	})

	t.Run("a follower's waiting addition doesn't outlive a later leader's removal", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl2", followerList(25, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
		receive(t, handler, "ctrl1", leaderList(20, 0, "ctrl1", "ctrl2", "ctrl3"))
		receive(t, handler, "ctrl2", followerList(26, "ctrl1", "ctrl2", "ctrl3"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the leader's later list must remove the earlier addition")
		req.Equal(uint64(20), handler.currentVersion)
	})

	t.Run("a leader's list received while a follower's list is applied removes what it omits", func(t *testing.T) {
		req := require.New(t)

		// arrange: a follower's list naming ctrl3 has read the controller details and is held there
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(20, 0, "ctrl1", "ctrl2"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the follower's list must not re-add a controller the leader removed")
		req.Equal(uint64(20), handler.currentVersion)
	})

	t.Run("a leader announcement applies in order with lists", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, 0, "ctrl1", "ctrl2", "ctrl3"))
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(0))
		held.release()
		flush(t, handler)

		// assert
		req.Equal("ctrl2", ctrls.leader(), "the announcement received after the leader's list must decide the leader")
	})
}

// Test_updateCtrlAddressesHandler_TooManyWaitingUpdates locks in that past maxPendingCtrlUpdates waiting
// updates, receiving still doesn't wait, the waiting lists and announcements are dropped, and the leader's
// list is requested in their place.
func Test_updateCtrlAddressesHandler_TooManyWaitingUpdates(t *testing.T) {
	t.Run("the waiting lists are dropped and the leader's list requested", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		leader := newRecordingCtrl("ctrl1")
		ctrls.leaderCtrl = leader
		ctrls.connected["ctrl1"] = leader
		handler := newCtrlListHandler(t, ctrls)
		var deliveries []delivery
		for i := range maxPendingCtrlUpdates + 1 {
			msg, err := ctrlMsg(handler, followerList(uint64(i+1), "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
			req.NoError(err)
			deliveries = append(deliveries, delivery{from: "ctrl2", msg: msg})
		}
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act
		received := make(chan struct{})
		go receiveAll(handler, deliveries, received)
		requireClosed(t, received, "receiving a list blocked past the bound on waiting lists")
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the waiting lists must be dropped")
		requireRequested(t, leader)
	})

	t.Run("without a known leader every connected controller is asked", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		ctrl1, ctrl2 := newRecordingCtrl("ctrl1"), newRecordingCtrl("ctrl2")
		ctrls.connected["ctrl1"] = ctrl1
		ctrls.connected["ctrl2"] = ctrl2
		handler := newCtrlListHandler(t, ctrls)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act: the leader's list is among the lists dropped
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2"))
		for i := range maxPendingCtrlUpdates {
			receive(t, handler, "ctrl2", followerList(uint64(i+1), "ctrl1", "ctrl2"))
		}
		held.release()
		flush(t, handler)

		// assert
		requireRequested(t, ctrl1)
		requireRequested(t, ctrl2)
	})

	t.Run("a dropped leader announcement still lets the request reach the new leader", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		formerLeader, newLeader := newRecordingCtrl("ctrl1"), newRecordingCtrl("ctrl2")
		ctrls.connected["ctrl1"] = formerLeader
		ctrls.connected["ctrl2"] = newLeader
		ctrls.leaderCtrl = formerLeader
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(10, 0, "ctrl1", "ctrl2"))
		flush(t, handler)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act: ctrl2's announcement is among the updates dropped
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(0))
		for i := range maxPendingCtrlUpdates {
			receive(t, handler, "ctrl1", followerList(uint64(11+i), "ctrl1", "ctrl2"))
		}
		held.release()
		flush(t, handler)

		// assert
		requireRequested(t, newLeader)
	})
}

// Test_updateCtrlAddressesHandler_RaftTerm locks in that leadership claims are ordered by raft term, so a
// former leader's delayed claim can't take leadership back, while claims from controllers that send no
// raft term still apply as they arrive.
func Test_updateCtrlAddressesHandler_RaftTerm(t *testing.T) {
	t.Run("an announcement from an earlier raft term is ignored", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(6))
		flush(t, handler)

		// act
		receive(t, handler.clusterLeaderReceiver(), "ctrl1", leaderAnnouncement(5))
		flush(t, handler)

		// assert
		req.Equal("ctrl2", ctrls.leader(), "a former leader's delayed announcement must not take leadership back")
	})

	t.Run("a leader's list from an earlier raft term doesn't undo an announcement", func(t *testing.T) {
		req := require.New(t)

		// arrange: ctrl1 led raft term 5, and ctrl2 has since announced it leads raft term 6
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(100, 5, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(6))
		flush(t, handler)

		// act: ctrl1's delayed list, at the same membership index
		receive(t, handler, "ctrl1", leaderList(100, 5, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal("ctrl2", ctrls.leader(), "a former leader's delayed list must not take leadership back")
	})

	t.Run("another controller's claim to the recorded raft term is ignored", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler.clusterLeaderReceiver(), "ctrl1", leaderAnnouncement(5))
		flush(t, handler)

		// act
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(5))
		flush(t, handler)

		// assert
		req.Equal("ctrl1", ctrls.leader(), "raft elects at most one leader per term, so a second claim to it is stale")

		t.Run("the controller that claimed the raft term may claim it again", func(t *testing.T) {
			req := require.New(t)

			// arrange
			awaitLeaderList(handler)

			// act
			receive(t, handler, "ctrl1", leaderList(100, 5, "ctrl1", "ctrl2", "ctrl3"))
			flush(t, handler)

			// assert
			req.Equal("ctrl1", ctrls.leader())
			req.False(awaitingLeaderList(handler), "the leader's repeated claim must be accepted, ending the request for its list")
		})
	})

	t.Run("a claim without a raft term applies after one with a raft term", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(6))
		flush(t, handler)

		// act
		receive(t, handler.clusterLeaderReceiver(), "ctrl3", leaderAnnouncement(0))
		flush(t, handler)

		// assert
		req.Equal("ctrl3", ctrls.leader(), "a claim without a raft term must apply as it arrives")

		t.Run("it leaves the recorded raft term to order the claims that carry one", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler.clusterLeaderReceiver(), "ctrl1", leaderAnnouncement(5))
			flush(t, handler)

			// assert
			req.Equal("ctrl3", ctrls.leader(), "a claim without a raft term must not let an earlier raft term back in")

			t.Run("a claim above the recorded raft term applies", func(t *testing.T) {
				req := require.New(t)

				// act
				receive(t, handler.clusterLeaderReceiver(), "ctrl1", leaderAnnouncement(7))
				flush(t, handler)

				// assert
				req.Equal("ctrl1", ctrls.leader())
			})
		})
	})

	t.Run("after an index reset an announcement at a lower raft term is applied", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(3500))
		flush(t, handler)

		// act
		handler.NotifyIndexReset()
		receive(t, handler.clusterLeaderReceiver(), "ctrl3", leaderAnnouncement(5))
		flush(t, handler)

		// assert
		req.Equal("ctrl3", ctrls.leader())
	})

	t.Run("an older controller that takes over from a newer one leads", func(t *testing.T) {
		req := require.New(t)

		// arrange: in a mixed-version cluster, newer ctrl2 leads raft term 6 and older ctrl1 sends no raft term
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl2", leaderList(100, 6, "ctrl1", "ctrl2"))
		flush(t, handler)
		req.Equal("ctrl2", ctrls.leader(), "precondition: ctrl2's claim to raft term 6 must be recorded")

		// act
		receive(t, handler.clusterLeaderReceiver(), "ctrl1", leaderAnnouncement(0))
		flush(t, handler)

		// assert
		req.Equal("ctrl1", ctrls.leader(), "the older controller's announcement must apply after the newer one's claim")

		t.Run("its list ends the request for the leader's list", func(t *testing.T) {
			req := require.New(t)

			// arrange
			awaitLeaderList(handler)

			// act
			receive(t, handler, "ctrl1", leaderList(100, 0, "ctrl1", "ctrl2"))
			receive(t, handler, "ctrl2", followerList(100, "ctrl1", "ctrl2"))
			flush(t, handler)

			// assert
			req.Equal("ctrl1", ctrls.leader())
			req.False(awaitingLeaderList(handler), "the older leader's list must end the request")
		})
	})
}

// Test_updateCtrlAddressesHandler_LeaderListRequest locks in when the router's request for the leader's
// list ends, and that asking stops once it has.
func Test_updateCtrlAddressesHandler_LeaderListRequest(t *testing.T) {
	t.Run("a former leader's list doesn't end the request", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(6))
		flush(t, handler)
		awaitLeaderList(handler)

		// act
		receive(t, handler, "ctrl1", leaderList(100, 5, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.True(awaitingLeaderList(handler), "a list whose leadership claim is rejected must not end the request")

		t.Run("the current leader's list ends it", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl2", leaderList(100, 6, "ctrl1", "ctrl2", "ctrl3"))
			flush(t, handler)

			// assert
			req.False(awaitingLeaderList(handler), "the leader's list must end the request")
		})
	})

	t.Run("a list without a raft term below the recorded index doesn't end the request", func(t *testing.T) {
		req := require.New(t)

		// arrange: no controller has sent a raft term
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", leaderList(100, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		awaitLeaderList(handler)

		// act
		receive(t, handler, "ctrl3", leaderList(50, 0, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.True(awaitingLeaderList(handler), "a list without a raft term below the recorded index must not end the request")

		t.Run("one at the recorded index ends it", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl3", leaderList(100, 0, "ctrl1", "ctrl2", "ctrl3"))
			flush(t, handler)

			// assert
			req.False(awaitingLeaderList(handler), "a list without a raft term at the recorded index must end the request")
		})
	})

	t.Run("a stuck controller doesn't hold up requests to the others", func(t *testing.T) {
		// arrange: ctrl1's send queue is full
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		ctrl2 := newRecordingCtrl("ctrl2")
		ctrls.connected["ctrl1"] = newStuckCtrl(t, "ctrl1")
		ctrls.connected["ctrl2"] = ctrl2
		handler := newCtrlListHandler(t, ctrls)

		// act: a follower's list omitting a controller starts the request
		receive(t, handler, "ctrl1", followerList(5, "ctrl1"))
		flush(t, handler)

		// assert
		requireRequested(t, ctrl2)
	})

	t.Run("the request repeats while it waits for the leader's list", func(t *testing.T) {
		// arrange
		ctrls := newCtrlListControllers("ctrl1")
		ctrl1 := newRecordingCtrl("ctrl1")
		ctrls.connected["ctrl1"] = ctrl1
		handler := newCtrlListHandler(t, ctrls)
		handler.askInterval = 10 * time.Millisecond

		// act
		handler.requestLeaderList()

		// assert
		requireRequested(t, ctrl1)
		requireRequested(t, ctrl1)

		t.Run("asking stops when the router closes", func(t *testing.T) {
			req := require.New(t)

			// act
			handler.env.(*ctrlListEnv).stop()

			// assert: a request already under way may still arrive, then a quiet spell of many rounds follows
			deadline := time.After(5 * time.Second)
			for quiet := false; !quiet; {
				select {
				case <-ctrl1.ch.sent:
				case <-time.After(300 * time.Millisecond):
					quiet = true
				case <-deadline:
					req.FailNow("the request kept repeating after the router closed")
				}
			}
		})
	})

	t.Run("an ended request doesn't keep asking alongside a later one", func(t *testing.T) {
		req := require.New(t)

		// arrange: a request asking every 100ms, ended by the leader's list
		ctrls := newCtrlListControllers("ctrl1")
		ctrl1 := newRecordingCtrl("ctrl1")
		ctrls.connected["ctrl1"] = ctrl1
		handler := newCtrlListHandler(t, ctrls)
		handler.askInterval = 100 * time.Millisecond
		handler.requestLeaderList()
		requireRequested(t, ctrl1)
		receive(t, handler, "ctrl1", leaderList(10, 5, "ctrl1"))
		flush(t, handler)

		// act: a later request asks once, then not again for an hour
		handler.askInterval = time.Hour
		handler.requestLeaderList()

		// assert: past the later request's first round, a quiet spell of several of the ended one's rounds follows
		deadline := time.After(5 * time.Second)
		for quiet := false; !quiet; {
			select {
			case <-ctrl1.ch.sent:
			case <-time.After(300 * time.Millisecond):
				quiet = true
			case <-deadline:
				req.FailNow("the ended request kept asking alongside the later one")
			}
		}
	})
}

// Test_updateCtrlAddressesHandler_LeaderListOmittingLeader covers a leader whose member list hasn't caught
// up, so the list it sends omits the leader itself.
func Test_updateCtrlAddressesHandler_LeaderListOmittingLeader(t *testing.T) {
	t.Run("a leader's list that omits the leader removes nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange: a restarted router, at no recorded version, that knows all three controllers
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		receive(t, handler, "ctrl1", followerList(3405, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// act: ctrl2 leads while replaying its configuration entries after a snapshot install
		receive(t, handler, "ctrl2", leaderList(1796, 4, "ctrl1"))
		receive(t, handler, "ctrl2", leaderList(2412, 4, "ctrl1", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "a leader's list omitting the leader must not remove controllers")
		req.Empty(ctrls.updates(), "a leader's list omitting the leader must not be applied as the complete set")
		req.Zero(handler.currentVersion, "a leader's list omitting the leader must not be the recorded version")
		req.Equal("ctrl2", ctrls.leader(), "the leadership claim must still apply")
		req.True(awaitingLeaderList(handler), "the router must keep asking for the leader's list")

		t.Run("the leader's list naming the leader replaces the set and ends the request", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl2", leaderList(2414, 4, "ctrl1", "ctrl2"))
			flush(t, handler)

			// assert
			req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "a newer leader's list naming the leader must remove the controllers it omits")
			req.Equal(uint64(2414), handler.currentVersion)
			req.False(awaitingLeaderList(handler), "the leader's list must end the request")
		})
	})

	t.Run("a leader's list that omits the leader adds the controllers it names", func(t *testing.T) {
		req := require.New(t)

		// arrange: the list omits nothing the router has, so only the missing leader marks it stale
		ctrls := newCtrlListControllers("ctrl1")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, handler, "ctrl2", leaderList(5, 1, "ctrl1", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl3"}, ctrls.ids())
		req.Equal([]bool{false}, ctrls.updates(), "the additions must not be applied as the complete set")
		req.True(awaitingLeaderList(handler), "the router must ask for the leader's list")
	})
}

// receive delivers m to receiver as controller from sends it. It doesn't wait for m to be applied.
func receive(t *testing.T, receiver channel.TypedReceiveHandler, from string, m proto.Message) {
	t.Helper()
	msg, err := ctrlMsg(receiver, m)
	require.NoError(t, err)
	receiver.HandleReceive(msg, &ctrlListChannel{id: from})
}

// flush waits until every update handler has received has been applied or dropped.
func flush(t *testing.T, handler *updateCtrlAddressesHandler) {
	t.Helper()
	handler.lock.Lock()
	target := handler.received
	handler.lock.Unlock()
	require.Eventually(t, func() bool {
		return handler.appliedThrough.Load() >= target
	}, 5*time.Second, time.Millisecond, "received controller list updates were not applied")
}

// requireClosed fails the test unless ch is closed within five seconds.
func requireClosed(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		require.FailNow(t, msg)
	}
}

// requireRequested fails the test unless ctrl is asked for its controller list within five seconds.
func requireRequested(t *testing.T, ctrl *recordingCtrl) {
	t.Helper()
	select {
	case contentType := <-ctrl.ch.sent:
		require.Equal(t, int32(ctrl_pb.ContentType_RequestClusterMembers), contentType,
			"the message sent to %s must be a request for its controller list", ctrl.ch.id)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the controller list was not requested from "+ctrl.ch.id)
	}
}

// ctrlMsg returns m as the message receiver handles.
func ctrlMsg(receiver channel.TypedReceiveHandler, m proto.Message) (*channel.Message, error) {
	body, err := proto.Marshal(m)
	if err != nil {
		return nil, err
	}
	return channel.NewMessage(receiver.ContentType(), body), nil
}

// followerList returns a follower's controller list at index, naming ids.
func followerList(index uint64, ids ...string) *ctrl_pb.UpdateCtrlAddresses {
	upd := &ctrl_pb.UpdateCtrlAddresses{Index: index}
	for _, id := range ids {
		upd.Controllers = append(upd.Controllers, ctrlDetail(id))
	}
	return upd
}

// leaderList returns a leader's controller list at index, naming ids, from a controller leading raftTerm.
// A raftTerm of 0 stands for a controller that doesn't send one.
func leaderList(index, raftTerm uint64, ids ...string) *ctrl_pb.UpdateCtrlAddresses {
	upd := &ctrl_pb.UpdateCtrlAddresses{IsLeader: true, Index: index, RaftTerm: raftTerm}
	for _, id := range ids {
		upd.Controllers = append(upd.Controllers, ctrlDetail(id))
	}
	return upd
}

// leaderAnnouncement returns a leader announcement from a controller leading raftTerm. A raftTerm of 0
// stands for a controller that doesn't send one.
func leaderAnnouncement(raftTerm uint64) *ctrl_pb.UpdateClusterLeader {
	return &ctrl_pb.UpdateClusterLeader{RaftTerm: raftTerm}
}

func ctrlDetail(id string) *ctrl_pb.CtrlDetail {
	return &ctrl_pb.CtrlDetail{Id: id, Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: "tls:" + id + ":6262"}}}
}

// delivery is a message as controller from sends it.
type delivery struct {
	from string
	msg  *channel.Message
}

// receiveAll delivers each of deliveries to receiver in order, then closes done.
func receiveAll(receiver channel.TypedReceiveHandler, deliveries []delivery, done chan<- struct{}) {
	defer close(done)
	for _, d := range deliveries {
		receiver.HandleReceive(d.msg, &ctrlListChannel{id: d.from})
	}
}

// newCtrlListHandler returns a running handler over ctrls, stopped when the test ends.
func newCtrlListHandler(t *testing.T, ctrls *ctrlListControllers) *updateCtrlAddressesHandler {
	routerEnv := &ctrlListEnv{ctrls: ctrls, closeNotify: make(chan struct{})}
	t.Cleanup(routerEnv.stop)
	handler := &updateCtrlAddressesHandler{
		env:         routerEnv,
		wake:        make(chan struct{}, 1),
		askInterval: 30 * time.Second,
	}
	go handler.run()
	return handler
}

// holdApplying holds handler's run goroutine in applying a follower's list until the returned gate is
// released, so the updates received meanwhile wait. The held list names every controller ctrls has, so
// applying it changes nothing. The gate is released when the test ends.
func holdApplying(t *testing.T, ctrls *ctrlListControllers, handler *updateCtrlAddressesHandler) (*gate, error) {
	held := ctrls.holdReads()
	t.Cleanup(held.release)

	// Above any recorded version, so the list reaches the merge, which reads the details.
	msg, err := ctrlMsg(handler, followerList(math.MaxUint64, ctrls.ids()...))
	if err != nil {
		return nil, err
	}
	handler.HandleReceive(msg, &ctrlListChannel{id: "ctrl9"})

	select {
	case <-held.entered:
		return held, nil
	case <-time.After(5 * time.Second):
		return nil, errors.New("the run goroutine never started applying the held list")
	}
}

// awaitLeaderList marks handler as waiting for the leader's list, without asking anyone.
func awaitLeaderList(handler *updateCtrlAddressesHandler) {
	handler.leaderListRequest.Store(&leaderListRequest{done: make(chan struct{})})
}

// awaitingLeaderList reports whether handler is waiting for the leader's list.
func awaitingLeaderList(handler *updateCtrlAddressesHandler) bool {
	return handler.leaderListRequest.Load() != nil
}

// gate blocks callers of wait until release is called. entered is closed when the first caller reaches
// wait.
type gate struct {
	entered     chan struct{}
	enteredOnce sync.Once
	released    chan struct{}
	releaseOnce sync.Once
}

func newGate() *gate {
	return &gate{entered: make(chan struct{}), released: make(chan struct{})}
}

func (g *gate) wait() {
	g.enteredOnce.Do(func() { close(g.entered) })
	<-g.released
}

// release unblocks every current and later wait. It is safe to call more than once.
func (g *gate) release() {
	g.releaseOnce.Do(func() { close(g.released) })
}

// ctrlListControllers holds a controller set the way the router's does: replacing the details drops
// every connected controller the new details don't name.
type ctrlListControllers struct {
	env.NetworkControllers
	lock      sync.Mutex
	details   map[string]*ctrl_pb.CtrlDetail
	connected map[string]env.NetworkController
	leaderId  string
	readGate  *gate
	// leaderCtrl is what GetLeader returns.
	leaderCtrl env.NetworkController
	// authoritative records the authoritative flag of each UpdateControllerDetails, in order.
	authoritative []bool
}

// newCtrlListControllers returns a controller set with a detail for each of ids and nothing connected.
func newCtrlListControllers(ids ...string) *ctrlListControllers {
	result := &ctrlListControllers{
		details:   map[string]*ctrl_pb.CtrlDetail{},
		connected: map[string]env.NetworkController{},
	}
	for _, id := range ids {
		result.details[id] = ctrlDetail(id)
	}
	return result
}

// GetControllerDetails waits on the gate from holdReads, if any, before reading.
func (c *ctrlListControllers) GetControllerDetails() map[string]*ctrl_pb.CtrlDetail {
	c.lock.Lock()
	readGate := c.readGate
	c.lock.Unlock()
	if readGate != nil {
		readGate.wait()
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	return maps.Clone(c.details)
}

func (c *ctrlListControllers) UpdateControllerDetails(controllers []*ctrl_pb.CtrlDetail, authoritative bool) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.authoritative = append(c.authoritative, authoritative)
	c.details = map[string]*ctrl_pb.CtrlDetail{}
	for _, ctrl := range controllers {
		c.details[ctrl.Id] = ctrl
	}
	for id := range c.connected {
		if _, ok := c.details[id]; !ok {
			delete(c.connected, id)
		}
	}
	return true
}

func (c *ctrlListControllers) GetAll() map[string]env.NetworkController {
	c.lock.Lock()
	defer c.lock.Unlock()
	return maps.Clone(c.connected)
}

func (c *ctrlListControllers) UpdateLeader(leaderId string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.leaderId = leaderId
}

func (c *ctrlListControllers) GetLeader() env.NetworkController {
	return c.leaderCtrl
}

// holdReads makes every later GetControllerDetails wait until the returned gate is released.
func (c *ctrlListControllers) holdReads() *gate {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.readGate = newGate()
	return c.readGate
}

func (c *ctrlListControllers) ids() []string {
	c.lock.Lock()
	defer c.lock.Unlock()
	return slices.Sorted(maps.Keys(c.details))
}

func (c *ctrlListControllers) detail(id string) *ctrl_pb.CtrlDetail {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.details[id]
}

// updates returns the authoritative flag of each controller set update so far.
func (c *ctrlListControllers) updates() []bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return slices.Clone(c.authoritative)
}

func (c *ctrlListControllers) leader() string {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.leaderId
}

// ctrlListEnv is a router env over a ctrlListControllers.
type ctrlListEnv struct {
	env.RouterEnv
	ctrls       *ctrlListControllers
	closeNotify chan struct{}
	closeOnce   sync.Once
}

// stop closes the router, as far as the handler can tell. It is safe to call more than once.
func (e *ctrlListEnv) stop() {
	e.closeOnce.Do(func() {
		close(e.closeNotify)
	})
}

func (e *ctrlListEnv) GetNetworkControllers() env.NetworkControllers {
	return e.ctrls
}

func (e *ctrlListEnv) UpdateCtrlEndpointDetails(controllers []*ctrl_pb.CtrlDetail, authoritative bool) {
	e.ctrls.UpdateControllerDetails(controllers, authoritative)
}

func (e *ctrlListEnv) UpdateLeader(leaderId string) {
	e.ctrls.UpdateLeader(leaderId)
}

func (e *ctrlListEnv) GetCloseNotify() <-chan struct{} {
	return e.closeNotify
}

// ctrlListChannel is the channel a controller's message arrives on, identified by the controller's id.
type ctrlListChannel struct {
	channel.Channel
	id string
}

func (c *ctrlListChannel) Id() string {
	return c.id
}

func (c *ctrlListChannel) Label() string {
	return c.id
}

// connectedCtrl is a connected controller. A nil ctrlCh stands for one the router dialed.
type connectedCtrl struct {
	env.NetworkController
	address string
	ctrlCh  ctrlchan.CtrlChannel
}

func (c *connectedCtrl) Address() string {
	return c.address
}

func (c *connectedCtrl) CtrlChannel() ctrlchan.CtrlChannel {
	return c.ctrlCh
}

// IsConnected reports false, so nothing tries to send on a connectedCtrl, which has no channel.
func (c *connectedCtrl) IsConnected() bool {
	return false
}

// recordingCtrl is a connected controller whose channel reports each content type sent on it.
type recordingCtrl struct {
	env.NetworkController
	ch *recordingChannel
}

func newRecordingCtrl(id string) *recordingCtrl {
	return &recordingCtrl{ch: &recordingChannel{id: id, sent: make(chan int32, 1)}}
}

func (c *recordingCtrl) Channel() channel.Channel {
	return c.ch
}

func (c *recordingCtrl) IsConnected() bool {
	return true
}

// recordingChannel puts the content type of each message sent on it on sent, dropping it if sent is full.
type recordingChannel struct {
	channel.Channel
	id   string
	sent chan int32
}

func (c *recordingChannel) Id() string {
	return c.id
}

func (c *recordingChannel) TrySend(s channel.Sendable) (bool, error) {
	select {
	case c.sent <- s.Msg().ContentType:
	default:
	}
	return true, nil
}

// stuckCtrl is a connected controller whose send queue is full: Send waits until the test ends, and
// TrySend drops the message.
type stuckCtrl struct {
	env.NetworkController
	ch *stuckChannel
}

func newStuckCtrl(t *testing.T, id string) *stuckCtrl {
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
	})
	return &stuckCtrl{ch: &stuckChannel{id: id, done: done}}
}

func (c *stuckCtrl) Channel() channel.Channel {
	return c.ch
}

func (c *stuckCtrl) IsConnected() bool {
	return true
}

type stuckChannel struct {
	channel.Channel
	id   string
	done chan struct{}
}

func (c *stuckChannel) Id() string {
	return c.id
}

func (c *stuckChannel) Send(channel.Sendable) error {
	<-c.done
	return nil
}

func (c *stuckChannel) TrySend(channel.Sendable) (bool, error) {
	return false, nil
}
