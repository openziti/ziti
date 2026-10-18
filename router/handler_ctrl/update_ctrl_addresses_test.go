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
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// ctrlListUpdater holds controller endpoints the way the router does: an update replaces them.
type ctrlListUpdater struct {
	lock      sync.Mutex
	endpoints []string
	leaderId  string
	// readHook, if set, runs at the start of each GetCtrlEndpoints.
	readHook func()
	// leaderHook, if set, runs at the end of each UpdateLeader.
	leaderHook func()
}

func newCtrlListUpdater(ids ...string) *ctrlListUpdater {
	result := &ctrlListUpdater{}
	for _, id := range ids {
		result.endpoints = append(result.endpoints, ctrlAddr(id))
	}
	return result
}

func (u *ctrlListUpdater) UpdateCtrlEndpoints(endpoints []string) {
	u.lock.Lock()
	defer u.lock.Unlock()
	u.endpoints = slices.Clone(endpoints)
}

func (u *ctrlListUpdater) UpdateLeader(leaderId string) {
	u.lock.Lock()
	u.leaderId = leaderId
	u.lock.Unlock()
	if u.leaderHook != nil {
		u.leaderHook()
	}
}

func (u *ctrlListUpdater) GetCtrlEndpoints() []string {
	if u.readHook != nil {
		u.readHook()
	}
	u.lock.Lock()
	defer u.lock.Unlock()
	return slices.Clone(u.endpoints)
}

// ids returns the controllers the router has endpoints for, sorted.
func (u *ctrlListUpdater) ids() []string {
	u.lock.Lock()
	defer u.lock.Unlock()
	var result []string
	for _, endpoint := range u.endpoints {
		result = append(result, endpoint[len("tls:"):len(endpoint)-len(":6262")])
	}
	slices.Sort(result)
	return result
}

func (u *ctrlListUpdater) leader() string {
	u.lock.Lock()
	defer u.lock.Unlock()
	return u.leaderId
}

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

func ctrlAddr(id string) string {
	return "tls:" + id + ":6262"
}

// newCtrlListHandler returns a running handler over updater, stopped when the test ends.
func newCtrlListHandler(t *testing.T, updater *ctrlListUpdater) *updateCtrlAddressesHandler {
	closeNotify := make(chan struct{})
	t.Cleanup(func() {
		close(closeNotify)
	})
	return newUpdateCtrlAddressesHandler(updater, closeNotify)
}

// flush waits until every update handler has received has been applied or dropped.
func flush(t *testing.T, handler *updateCtrlAddressesHandler) {
	handler.lock.Lock()
	target := handler.received
	handler.lock.Unlock()
	require.Eventually(t, func() bool {
		return handler.appliedThrough.Load() >= target
	}, 5*time.Second, time.Millisecond, "received controller list updates were not applied")
}

// ctrlListMsg builds a controller list message naming ids.
func ctrlListMsg(t *testing.T, index uint64, isLeader bool, ids ...string) *channel.Message {
	upd := &ctrl_pb.UpdateCtrlAddresses{IsLeader: isLeader, Index: index}
	for _, id := range ids {
		upd.Addresses = append(upd.Addresses, ctrlAddr(id))
	}
	body, err := proto.Marshal(upd)
	require.NoError(t, err)
	return channel.NewMessage(int32(ctrl_pb.ContentType_UpdateCtrlAddressesType), body)
}

// sendCtrlList delivers a controller list naming ids, as controller from would send it, and waits for it
// to be applied.
func sendCtrlList(t *testing.T, handler *updateCtrlAddressesHandler, from string, index uint64, isLeader bool, ids ...string) {
	handler.HandleReceive(ctrlListMsg(t, index, isLeader, ids...), &ctrlListChannel{id: from})
	flush(t, handler)
}

func clusterLeaderMsg(t *testing.T, index uint64) *channel.Message {
	body, err := proto.Marshal(&ctrl_pb.UpdateClusterLeader{Index: index})
	require.NoError(t, err)
	return channel.NewMessage(int32(ctrl_pb.ContentType_UpdateClusterLeaderRequestType), body)
}

// sendClusterLeader delivers a leader announcement at index from controller from, and waits for it to be
// applied.
func sendClusterLeader(t *testing.T, handler *updateCtrlAddressesHandler, from string, index uint64) {
	handler.clusterLeaderReceiver().HandleReceive(clusterLeaderMsg(t, index), &ctrlListChannel{id: from})
	flush(t, handler)
}

// blockFirstRead makes the first GetCtrlEndpoints, which a merge calls, wait until the test ends or
// release is called, and returns a channel closed once that read starts.
func blockFirstRead(t *testing.T, updater *ctrlListUpdater) (reading <-chan struct{}, release func()) {
	readingC := make(chan struct{})
	releaseC := make(chan struct{})
	var readOnce, releaseOnce sync.Once
	updater.readHook = func() {
		readOnce.Do(func() {
			close(readingC)
			<-releaseC
		})
	}
	release = func() {
		releaseOnce.Do(func() {
			close(releaseC)
		})
	}
	t.Cleanup(release)
	return readingC, release
}

func waitFor(t *testing.T, c <-chan struct{}, what string) {
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		require.FailNow(t, what)
	}
}

// whileApplying holds the run goroutine in a follower list's merge while the test delivers updates, then
// lets it apply them. The held list names every controller the router has, so it changes nothing.
func whileApplying(t *testing.T, updater *ctrlListUpdater, handler *updateCtrlAddressesHandler, deliver func()) {
	ids := updater.ids()
	reading, release := blockFirstRead(t, updater)
	// Above any recorded version, so the list reaches the merge.
	first := ctrlListMsg(t, math.MaxUint64, false, ids...)
	go handler.HandleReceive(first, &ctrlListChannel{id: "ctrl9"})
	waitFor(t, reading, "the first update was not applied")
	deliver()
	release()
	flush(t, handler)
}

// Test_UpdateCtrlAddresses_FollowerListOnlyAdds: a follower's list adds the controllers it names that the
// router doesn't have, and removes none, since a follower that hasn't caught up can omit current members.
func Test_UpdateCtrlAddresses_FollowerListOnlyAdds(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	sendCtrlList(t, handler, "ctrl2", 5, false, "ctrl1")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "a follower's list must not remove controllers")

	sendCtrlList(t, handler, "ctrl2", 6, false, "ctrl1", "ctrl4")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl4"}, updater.ids(), "a follower's list must add controllers it names")
}

// Test_UpdateCtrlAddresses_StaleFollowerListsAroundLeaderList: followers that haven't caught up send lists
// naming only themselves, before and after the leader's full list, to a router that has recorded no
// version yet. The router keeps every controller and records the leader's version.
func Test_UpdateCtrlAddresses_StaleFollowerListsAroundLeaderList(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	sendCtrlList(t, handler, "ctrl2", 3109, false, "ctrl2")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "a stale follower list must not remove controllers")

	sendCtrlList(t, handler, "ctrl1", 4011, true, "ctrl1", "ctrl2", "ctrl3")
	sendCtrlList(t, handler, "ctrl3", 3109, false, "ctrl3")

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids())
	req.Equal(uint64(4011), handler.currentVersion)
	req.Equal("ctrl1", updater.leader())
}

// Test_UpdateCtrlAddresses_NewerLeaderListReplaces: a leader's list at a newer version is the controller
// set, removals included.
func Test_UpdateCtrlAddresses_NewerLeaderListReplaces(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2")
	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
	req.Equal(uint64(10), handler.currentVersion)
	req.Equal("ctrl1", updater.leader())

	sendCtrlList(t, handler, "ctrl3", 9, true, "ctrl1", "ctrl2", "ctrl3")
	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "an older leader list must be ignored")
	req.Equal("ctrl1", updater.leader(), "an older leader list must not claim leadership")
}

// Test_UpdateCtrlAddresses_LeaderListAtRecordedVersionOnlyAdds: a leader's list at the recorded version
// restores controllers the router lacks, without removing any.
func Test_UpdateCtrlAddresses_LeaderListAtRecordedVersionOnlyAdds(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1")
	handler := newCtrlListHandler(t, updater)
	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1")

	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2", "ctrl3")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "the leader's list must restore missing controllers")

	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "only a newer leader list may remove controllers")
}

// Test_UpdateCtrlAddresses_FollowerListBelowRecordedVersionIsIgnored: a follower's list below the
// recorded version adds nothing, so it can't bring back a controller the leader removed.
func Test_UpdateCtrlAddresses_FollowerListBelowRecordedVersionIsIgnored(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)
	sendCtrlList(t, handler, "ctrl1", 20, true, "ctrl1", "ctrl2")

	sendCtrlList(t, handler, "ctrl2", 15, false, "ctrl1", "ctrl2", "ctrl3")
	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
}

// Test_UpdateCtrlAddresses_EmptyListsAreIgnored: an empty list, from the leader or a follower, changes
// nothing and records nothing.
func Test_UpdateCtrlAddresses_EmptyListsAreIgnored(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2")
	handler := newCtrlListHandler(t, updater)

	sendCtrlList(t, handler, "ctrl1", 10, true)
	sendCtrlList(t, handler, "ctrl2", 11, false)

	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
	req.Equal(uint64(0), handler.currentVersion)
	req.Equal("", updater.leader())
}

// Test_UpdateCtrlAddresses_IndexResetAcceptsLowerLeaderList: after an index reset, a leader's list at a
// lower index replaces the set.
func Test_UpdateCtrlAddresses_IndexResetAcceptsLowerLeaderList(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)
	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2", "ctrl3")

	handler.NotifyIndexReset()
	sendCtrlList(t, handler, "ctrl1", 5, true, "ctrl1", "ctrl2")

	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
	req.Equal(uint64(5), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_ReceiveDoesNotWaitForAnUpdateInProgress: receiving a list only records it, so
// a channel's receive goroutine keeps running however many lists arrive while an update is applied.
func Test_UpdateCtrlAddresses_ReceiveDoesNotWaitForAnUpdateInProgress(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)
	reading, release := blockFirstRead(t, updater)

	first := ctrlListMsg(t, 5, false, "ctrl1")
	go handler.HandleReceive(first, &ctrlListChannel{id: "ctrl2"})
	waitFor(t, reading, "the first update was not applied")

	var msgs []*channel.Message
	for i := range 100 {
		msgs = append(msgs, ctrlListMsg(t, uint64(6+i), false, "ctrl1"))
	}
	leaderMsg := ctrlListMsg(t, 200, true, "ctrl1", "ctrl2", "ctrl4")

	received := make(chan struct{})
	go func() {
		defer close(received)
		for i, msg := range msgs {
			handler.HandleReceive(msg, &ctrlListChannel{id: []string{"ctrl2", "ctrl3"}[i%2]})
		}
		handler.HandleReceive(leaderMsg, &ctrlListChannel{id: "ctrl1"})
	}()
	select {
	case <-received:
	case <-time.After(time.Second):
		req.FailNow("receiving a list waited for another controller's update")
	}

	release()
	flush(t, handler)
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl4"}, updater.ids(), "the leader's list must be applied")
	req.Equal(uint64(200), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_FollowerListCannotUndoConcurrentLeaderRemoval: a follower's list received
// before a newer leader list must not re-add a controller the leader removed.
func Test_UpdateCtrlAddresses_FollowerListCannotUndoConcurrentLeaderRemoval(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)
	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2", "ctrl3")

	followerReading := make(chan struct{})
	leaderApplied := make(chan struct{})
	var readOnce, leaderOnce sync.Once
	updater.leaderHook = func() {
		leaderOnce.Do(func() {
			close(leaderApplied)
		})
	}
	updater.readHook = func() {
		readOnce.Do(func() {
			close(followerReading)
			// Wait for the leader's list to be applied, unless applying it is waiting on this update.
			select {
			case <-leaderApplied:
			case <-time.After(200 * time.Millisecond):
			}
		})
	}

	go handler.HandleReceive(ctrlListMsg(t, 15, false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"), &ctrlListChannel{id: "ctrl2"})
	waitFor(t, followerReading, "the follower's list never read the controller endpoints")
	handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
	flush(t, handler)

	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "a follower list must not re-add a controller the leader removed")
	req.Equal(uint64(20), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_IndexResetAppliesBetweenTheUpdatesAroundIt: an index reset received while
// updates are waiting applies after the updates received before it and before those received after it.
func Test_UpdateCtrlAddresses_IndexResetAppliesBetweenTheUpdatesAroundIt(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	whileApplying(t, updater, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 3432, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		handler.NotifyIndexReset()
		handler.HandleReceive(ctrlListMsg(t, 5, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
	})

	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "the list after the reset must be applied")
	req.Equal(uint64(5), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_WaitingFollowerAdditionDoesNotOutliveALaterLeaderRemoval: a follower's
// addition, a leader list without it and a later list from the same follower, all waiting, leave the
// controller removed, as applying them as they arrived does.
func Test_UpdateCtrlAddresses_WaitingFollowerAdditionDoesNotOutliveALaterLeaderRemoval(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)
	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2", "ctrl3")

	whileApplying(t, updater, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 25, false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"), &ctrlListChannel{id: "ctrl2"})
		handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		handler.HandleReceive(ctrlListMsg(t, 26, false, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "the leader's later list must remove the earlier addition")
	req.Equal(uint64(20), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_LeaderAnnouncementAppliesInOrderWithLists: a leader announcement received
// after a leader list, both waiting, decides the leader, since the two apply in the order they arrived.
// Applying each on arrival orders them the same way, so this guards the queue's ordering rather than
// showing the fix.
func Test_UpdateCtrlAddresses_LeaderAnnouncementAppliesInOrderWithLists(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	whileApplying(t, updater, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 3432, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		handler.clusterLeaderReceiver().HandleReceive(clusterLeaderMsg(t, 4000), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal("ctrl2", updater.leader(), "the later announcement must decide the leader")
}

// Test_UpdateCtrlAddresses_OutdatedLeaderAnnouncementIsIgnored: an announcement below the last one applied
// doesn't change the leader, until an index reset.
func Test_UpdateCtrlAddresses_OutdatedLeaderAnnouncementIsIgnored(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	sendClusterLeader(t, handler, "ctrl2", 3500)
	sendClusterLeader(t, handler, "ctrl1", 3400)
	req.Equal("ctrl2", updater.leader())

	handler.NotifyIndexReset()
	sendClusterLeader(t, handler, "ctrl3", 5)
	req.Equal("ctrl3", updater.leader())
}

// Test_UpdateCtrlAddresses_TooManyWaitingUpdatesKeepsLeaderLists: past the bound on waiting updates,
// receiving doesn't block, the waiting follower lists are dropped, and the newest leader list from each
// controller is still applied.
func Test_UpdateCtrlAddresses_TooManyWaitingUpdatesKeepsLeaderLists(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, updater)

	var msgs []*channel.Message
	for i := range maxPendingCtrlUpdates {
		msgs = append(msgs, ctrlListMsg(t, uint64(i+1), false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
	}

	whileApplying(t, updater, handler, func() {
		received := make(chan struct{})
		go func() {
			defer close(received)
			handler.HandleReceive(ctrlListMsg(t, 50, true, "ctrl1", "ctrl2", "ctrl3", "ctrl5"), &ctrlListChannel{id: "ctrl1"})
			for _, msg := range msgs {
				handler.HandleReceive(msg, &ctrlListChannel{id: "ctrl2"})
			}
			handler.HandleReceive(ctrlListMsg(t, 40, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
		}()
		waitFor(t, received, "receiving a list blocked past the bound on waiting updates")

		handler.lock.Lock()
		waiting := len(handler.pending)
		handler.lock.Unlock()
		req.LessOrEqual(waiting, maxPendingCtrlUpdates)
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl5"}, updater.ids(), "the newest leader list must be applied and the follower lists dropped")
	req.Equal(uint64(50), handler.currentVersion)
}

// Test_BoundPendingCtrlUpdates_KeepsNewestPerControllerInArrivalOrder: of each controller's leader
// lists and announcements, the newest by index within the latest reset is kept, and the latest address.
func Test_BoundPendingCtrlUpdates_KeepsNewestPerControllerInArrivalOrder(t *testing.T) {
	req := require.New(t)
	list := func(ctrlId string, index uint64, isLeader bool, resets uint64) *pendingCtrlUpdate {
		return &pendingCtrlUpdate{upd: &ctrl_pb.UpdateCtrlAddresses{Index: index, IsLeader: isLeader}, ctrlId: ctrlId, resets: resets}
	}
	announce := func(ctrlId string, index uint64) *pendingCtrlUpdate {
		return &pendingCtrlUpdate{clusterLeader: &ctrl_pb.UpdateClusterLeader{Index: index}, ctrlId: ctrlId}
	}
	address := func(ctrlId string, addr string) *pendingCtrlUpdate {
		return &pendingCtrlUpdate{newAddress: addr, ctrlId: ctrlId}
	}

	pending := []*pendingCtrlUpdate{
		list("ctrl1", 20, true, 0),  // 0: superseded by 3, which comes after a reset
		address("ctrl1", "a"),       // 1: superseded by 6
		announce("ctrl2", 9),        // 2: kept, newer than 5
		list("ctrl1", 5, true, 1),   // 3: kept
		list("ctrl2", 30, false, 0), // 4: follower list, dropped
		announce("ctrl2", 8),        // 5: older than 2
		address("ctrl1", "b"),       // 6: kept
		list("ctrl2", 12, true, 0),  // 7: kept
	}

	result := boundPendingCtrlUpdates(pending)
	req.Equal([]*pendingCtrlUpdate{pending[2], pending[3], pending[6], pending[7]}, result)
}
