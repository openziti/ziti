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

// Test_updateCtrlAddressesHandler_ControllerLists locks in how controller lists change the router's
// controller endpoints. Only a leader's list at a newer index removes controllers, since a follower that
// hasn't caught up can send a list omitting current members.
func Test_updateCtrlAddressesHandler_ControllerLists(t *testing.T) {
	t.Run("a follower's list adds the controllers it names and removes none", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)

		// act
		receive(t, handler, "ctrl2", followerList(6, "ctrl1", "ctrl4"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl4"}, updater.ids(),
			"a follower's list must add the controllers it names and keep the ones it omits")
	})

	t.Run("stale follower lists around the leader's list remove nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange: the router has recorded no version yet
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)

		// act: followers that haven't caught up send lists naming only themselves, before and after the
		// leader's list
		receive(t, handler, "ctrl2", followerList(3109, "ctrl2"))
		flush(t, handler)
		receive(t, handler, "ctrl1", leaderList(4011, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		receive(t, handler, "ctrl3", followerList(3109, "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "a stale follower list must not remove controllers")
		req.Equal(uint64(4011), handler.currentVersion, "the leader's list must be the recorded version")
		req.Equal("ctrl1", updater.leader())
	})

	t.Run("a newer leader's list replaces the controller set", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)

		// act
		receive(t, handler, "ctrl1", leaderList(10, "ctrl1", "ctrl2"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "a newer leader's list must remove the controllers it omits")
		req.Equal(uint64(10), handler.currentVersion)
		req.Equal("ctrl1", updater.leader())

		t.Run("an older leader's list is ignored", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl3", leaderList(9, "ctrl1", "ctrl2", "ctrl3"))
			flush(t, handler)

			// assert
			req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "a leader's list below the recorded version must be ignored")
			req.Equal("ctrl1", updater.leader(), "a leader's list below the recorded version must not claim leadership")
		})
	})

	t.Run("a leader's list at the recorded version restores missing controllers", func(t *testing.T) {
		req := require.New(t)

		// arrange: the router has lost ctrl2 and ctrl3
		updater := newCtrlListUpdater("ctrl1")
		handler := newCtrlListHandler(t, updater)
		receive(t, handler, "ctrl1", leaderList(3432, "ctrl1"))
		flush(t, handler)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "the leader's list must restore missing controllers")

		t.Run("a leader's list at the recorded version removes none", func(t *testing.T) {
			req := require.New(t)

			// act
			receive(t, handler, "ctrl1", leaderList(3432, "ctrl1", "ctrl2"))
			flush(t, handler)

			// assert
			req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "only a newer leader's list may remove controllers")
		})
	})

	t.Run("a follower's list below the recorded version is ignored", func(t *testing.T) {
		req := require.New(t)

		// arrange: the leader's list at 20 has removed ctrl3
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		receive(t, handler, "ctrl1", leaderList(20, "ctrl1", "ctrl2"))
		flush(t, handler)

		// act
		receive(t, handler, "ctrl2", followerList(15, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "a stale list must not bring back a controller the leader removed")
	})

	t.Run("empty lists change and record nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, updater)

		// act
		receive(t, handler, "ctrl1", leaderList(10))
		receive(t, handler, "ctrl2", followerList(11))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
		req.Equal(uint64(0), handler.currentVersion, "an empty leader's list must not be recorded")
		req.Equal("", updater.leader(), "an empty leader's list must not claim leadership")
	})

	t.Run("after an index reset a leader's list at a lower index replaces the set", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		receive(t, handler, "ctrl1", leaderList(3432, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)

		// act
		handler.NotifyIndexReset()
		receive(t, handler, "ctrl1", leaderList(5, "ctrl1", "ctrl2"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
		req.Equal(uint64(5), handler.currentVersion)
	})
}

// Test_updateCtrlAddressesHandler_WaitingUpdates locks in that receiving an update only queues it, and
// that updates waiting while another is applied are applied in the order they arrived, with the result
// applying each as it arrived would have.
func Test_updateCtrlAddressesHandler_WaitingUpdates(t *testing.T) {
	t.Run("receiving a list doesn't wait for an update being applied", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		var deliveries []delivery
		for i := range 100 {
			msg, err := ctrlMsg(handler, followerList(uint64(6+i), "ctrl1"))
			req.NoError(err)
			deliveries = append(deliveries, delivery{from: []string{"ctrl2", "ctrl3"}[i%2], msg: msg})
		}
		msg, err := ctrlMsg(handler, leaderList(200, "ctrl1", "ctrl2", "ctrl4"))
		req.NoError(err)
		deliveries = append(deliveries, delivery{from: "ctrl1", msg: msg})
		held, err := holdApplying(t, updater, handler)
		req.NoError(err)

		// act
		received := make(chan struct{})
		go receiveAll(handler, deliveries, received)

		// assert
		requireClosed(t, received, "receiving a list waited for another controller's update")
		held.release()
		flush(t, handler)
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl4"}, updater.ids(), "the leader's list must be applied")
		req.Equal(uint64(200), handler.currentVersion)
	})

	t.Run("a leader's list received while a follower's list is applied removes what it omits", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		receive(t, handler, "ctrl1", leaderList(10, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		held := updater.holdReads()
		t.Cleanup(held.release)

		// act: a follower's list naming ctrl4 reads the endpoints, then the leader's list removing ctrl3 and
		// ctrl4 arrives
		receive(t, handler, "ctrl2", followerList(15, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
		requireClosed(t, held.entered, "the follower's list never read the controller endpoints")
		receive(t, handler, "ctrl1", leaderList(20, "ctrl1", "ctrl2"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "the follower's list must not re-add a controller the leader removed")
		req.Equal(uint64(20), handler.currentVersion)
	})

	t.Run("an index reset applies between the updates received around it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		held, err := holdApplying(t, updater, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, "ctrl1", "ctrl2", "ctrl3"))
		handler.NotifyIndexReset()
		receive(t, handler, "ctrl1", leaderList(5, "ctrl1", "ctrl2"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids(), "the list after the reset must be applied")
		req.Equal(uint64(5), handler.currentVersion)
	})

	t.Run("a follower's waiting addition doesn't outlive a later leader's removal", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		receive(t, handler, "ctrl1", leaderList(10, "ctrl1", "ctrl2", "ctrl3"))
		flush(t, handler)
		held, err := holdApplying(t, updater, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl2", followerList(25, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
		receive(t, handler, "ctrl1", leaderList(20, "ctrl1", "ctrl2", "ctrl3"))
		receive(t, handler, "ctrl2", followerList(26, "ctrl1", "ctrl2", "ctrl3"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, updater.ids(), "the leader's later list must remove the earlier addition")
		req.Equal(uint64(20), handler.currentVersion)
	})

	// Applying each update on arrival orders these the same way, so this guards the queue's ordering
	// rather than showing the fix.
	t.Run("a leader announcement applies in order with lists", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		held, err := holdApplying(t, updater, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(3432, "ctrl1", "ctrl2", "ctrl3"))
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(4000))
		held.release()
		flush(t, handler)

		// assert
		req.Equal("ctrl2", updater.leader(), "the announcement received after the leader's list must decide the leader")
	})
}

// Test_updateCtrlAddressesHandler_LeaderAnnouncements locks in that a leader announcement is judged
// against the last one applied.
func Test_updateCtrlAddressesHandler_LeaderAnnouncements(t *testing.T) {
	t.Run("an announcement below the last one applied is ignored", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		receive(t, handler.clusterLeaderReceiver(), "ctrl2", leaderAnnouncement(3500))
		flush(t, handler)

		// act
		receive(t, handler.clusterLeaderReceiver(), "ctrl1", leaderAnnouncement(3400))
		flush(t, handler)

		// assert
		req.Equal("ctrl2", updater.leader(), "a delayed announcement must not take leadership back")

		t.Run("after an index reset a lower announcement is applied", func(t *testing.T) {
			req := require.New(t)

			// act
			handler.NotifyIndexReset()
			receive(t, handler.clusterLeaderReceiver(), "ctrl3", leaderAnnouncement(5))
			flush(t, handler)

			// assert
			req.Equal("ctrl3", updater.leader())
		})
	})
}

// Test_updateCtrlAddressesHandler_TooManyWaitingUpdates locks in that past maxPendingCtrlUpdates waiting
// updates, receiving still doesn't wait, the waiting follower lists are dropped, and each controller's
// newest leader list is still applied.
func Test_updateCtrlAddressesHandler_TooManyWaitingUpdates(t *testing.T) {
	t.Run("the follower lists are dropped and the newest leader list applied", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, updater)
		newest, err := ctrlMsg(handler, leaderList(50, "ctrl1", "ctrl2", "ctrl3", "ctrl5"))
		req.NoError(err)
		deliveries := []delivery{{from: "ctrl1", msg: newest}}
		for i := range maxPendingCtrlUpdates {
			msg, err := ctrlMsg(handler, followerList(uint64(i+1), "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
			req.NoError(err)
			deliveries = append(deliveries, delivery{from: "ctrl2", msg: msg})
		}
		older, err := ctrlMsg(handler, leaderList(40, "ctrl1", "ctrl2"))
		req.NoError(err)
		deliveries = append(deliveries, delivery{from: "ctrl1", msg: older})
		held, err := holdApplying(t, updater, handler)
		req.NoError(err)

		// act
		received := make(chan struct{})
		go receiveAll(handler, deliveries, received)
		requireClosed(t, received, "receiving a list blocked past the bound on waiting updates")
		handler.lock.Lock()
		waiting := len(handler.pending)
		handler.lock.Unlock()
		held.release()
		flush(t, handler)

		// assert
		req.LessOrEqual(waiting, maxPendingCtrlUpdates, "the waiting updates must be bounded")
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl5"}, updater.ids(), "the newest leader list must be applied and the follower lists dropped")
		req.Equal(uint64(50), handler.currentVersion)
	})
}

// Test_boundPendingCtrlUpdates locks in what survives the bound on waiting updates.
func Test_boundPendingCtrlUpdates(t *testing.T) {
	t.Run("each controller's newest leader list and announcement and latest address are kept in arrival order", func(t *testing.T) {
		req := require.New(t)

		// arrange: newest is by index within the latest index reset
		pending := []*pendingCtrlUpdate{
			{ctrlId: "ctrl1", upd: &ctrl_pb.UpdateCtrlAddresses{Index: 20, IsLeader: true}},           // 0: superseded by 3, which comes after a reset
			{ctrlId: "ctrl1", newAddress: "a"},                                                        // 1: superseded by 6
			{ctrlId: "ctrl2", clusterLeader: &ctrl_pb.UpdateClusterLeader{Index: 9}},                  // 2: kept, newer than 5
			{ctrlId: "ctrl1", upd: &ctrl_pb.UpdateCtrlAddresses{Index: 5, IsLeader: true}, resets: 1}, // 3: kept
			{ctrlId: "ctrl2", upd: &ctrl_pb.UpdateCtrlAddresses{Index: 30}},                           // 4: follower list, dropped
			{ctrlId: "ctrl2", clusterLeader: &ctrl_pb.UpdateClusterLeader{Index: 8}},                  // 5: older than 2
			{ctrlId: "ctrl1", newAddress: "b"},                                                        // 6: kept
			{ctrlId: "ctrl2", upd: &ctrl_pb.UpdateCtrlAddresses{Index: 12, IsLeader: true}},           // 7: kept
		}

		// act
		result := boundPendingCtrlUpdates(pending)

		// assert
		req.Equal([]*pendingCtrlUpdate{pending[2], pending[3], pending[6], pending[7]}, result)
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
		upd.Addresses = append(upd.Addresses, ctrlAddr(id))
	}
	return upd
}

// leaderList returns a leader's controller list at index, naming ids.
func leaderList(index uint64, ids ...string) *ctrl_pb.UpdateCtrlAddresses {
	upd := &ctrl_pb.UpdateCtrlAddresses{IsLeader: true, Index: index}
	for _, id := range ids {
		upd.Addresses = append(upd.Addresses, ctrlAddr(id))
	}
	return upd
}

// leaderAnnouncement returns a leader announcement at index.
func leaderAnnouncement(index uint64) *ctrl_pb.UpdateClusterLeader {
	return &ctrl_pb.UpdateClusterLeader{Index: index}
}

func ctrlAddr(id string) string {
	return "tls:" + id + ":6262"
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

// newCtrlListHandler returns a running handler over updater, stopped when the test ends.
func newCtrlListHandler(t *testing.T, updater *ctrlListUpdater) *updateCtrlAddressesHandler {
	closeNotify := make(chan struct{})
	t.Cleanup(func() {
		close(closeNotify)
	})
	return newUpdateCtrlAddressesHandler(updater, closeNotify)
}

// holdApplying holds handler's run goroutine in applying a follower's list until the returned gate is
// released, so the updates received meanwhile wait. The held list names every controller updater has, so
// applying it changes nothing. The gate is released when the test ends.
func holdApplying(t *testing.T, updater *ctrlListUpdater, handler *updateCtrlAddressesHandler) (*gate, error) {
	ids := updater.ids()
	held := updater.holdReads()
	t.Cleanup(held.release)

	// Above any recorded version, so the list reaches the merge, which reads the endpoints.
	msg, err := ctrlMsg(handler, followerList(math.MaxUint64, ids...))
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

// ctrlListUpdater holds controller endpoints the way the router does: an update replaces them.
type ctrlListUpdater struct {
	lock      sync.Mutex
	endpoints []string
	leaderId  string
	readGate  *gate
}

// newCtrlListUpdater returns an updater with an endpoint for each of ids.
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
	defer u.lock.Unlock()
	u.leaderId = leaderId
}

// GetCtrlEndpoints waits on the gate from holdReads, if any, before reading.
func (u *ctrlListUpdater) GetCtrlEndpoints() []string {
	u.lock.Lock()
	readGate := u.readGate
	u.lock.Unlock()
	if readGate != nil {
		readGate.wait()
	}

	u.lock.Lock()
	defer u.lock.Unlock()
	return slices.Clone(u.endpoints)
}

// holdReads makes every later GetCtrlEndpoints wait until the returned gate is released.
func (u *ctrlListUpdater) holdReads() *gate {
	u.lock.Lock()
	defer u.lock.Unlock()
	u.readGate = newGate()
	return u.readGate
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
