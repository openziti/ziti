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

// ctrlListControllers holds a controller set the way the router's does: replacing the details drops
// every connected controller the new details don't name.
type ctrlListControllers struct {
	env.NetworkControllers
	lock      sync.Mutex
	details   map[string]*ctrl_pb.CtrlDetail
	connected map[string]env.NetworkController
	leaderId  string
	// readHook, if set, runs at the start of each GetControllerDetails.
	readHook func()
	// leaderHook, if set, runs at the end of each UpdateLeader.
	leaderHook func()
	// leaderCtrl is what GetLeader returns.
	leaderCtrl env.NetworkController
}

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

func (c *ctrlListControllers) GetControllerDetails() map[string]*ctrl_pb.CtrlDetail {
	if c.readHook != nil {
		c.readHook()
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	return maps.Clone(c.details)
}

func (c *ctrlListControllers) UpdateControllerDetails(controllers []*ctrl_pb.CtrlDetail) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
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
	c.leaderId = leaderId
	c.lock.Unlock()
	if c.leaderHook != nil {
		c.leaderHook()
	}
}

func (c *ctrlListControllers) GetLeader() env.NetworkController {
	return c.leaderCtrl
}

// recordingCtrl is a connected leader whose channel reports each content type sent on it.
type recordingCtrl struct {
	env.NetworkController
	ch *recordingChannel
}

func (c *recordingCtrl) Channel() channel.Channel {
	return c.ch
}

func (c *recordingCtrl) IsConnected() bool {
	return true
}

type recordingChannel struct {
	channel.Channel
	id   string
	sent chan int32
}

func (c *recordingChannel) Id() string {
	return c.id
}

func newRecordingCtrl(id string) *recordingCtrl {
	return &recordingCtrl{ch: &recordingChannel{id: id, sent: make(chan int32, 1)}}
}

// requireRequested waits for ctrl to be asked for its controller list.
func requireRequested(t *testing.T, ctrl *recordingCtrl) {
	select {
	case contentType := <-ctrl.ch.sent:
		require.Equal(t, int32(ctrl_pb.ContentType_RequestClusterMembers), contentType)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the controller list was not requested from "+ctrl.ch.id)
	}
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

func newStuckCtrl(t *testing.T, id string) *stuckCtrl {
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
	})
	return &stuckCtrl{ch: &stuckChannel{id: id, done: done}}
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

func (c *ctrlListControllers) leader() string {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.leaderId
}

type ctrlListEnv struct {
	env.RouterEnv
	ctrls       *ctrlListControllers
	closeNotify chan struct{}
	closeOnce   sync.Once
}

// stop closes the router, as far as the handler can tell.
func (e *ctrlListEnv) stop() {
	e.closeOnce.Do(func() {
		close(e.closeNotify)
	})
}

func (e *ctrlListEnv) GetNetworkControllers() env.NetworkControllers {
	return e.ctrls
}

func (e *ctrlListEnv) UpdateCtrlEndpointDetails(controllers []*ctrl_pb.CtrlDetail) {
	e.ctrls.UpdateControllerDetails(controllers)
}

func (e *ctrlListEnv) UpdateLeader(leaderId string) {
	e.ctrls.UpdateLeader(leaderId)
}

func (e *ctrlListEnv) GetCloseNotify() <-chan struct{} {
	return e.closeNotify
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

func ctrlDetail(id string) *ctrl_pb.CtrlDetail {
	return &ctrl_pb.CtrlDetail{Id: id, Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: "tls:" + id + ":6262"}}}
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

// flush waits until every update handler has received has been applied or superseded.
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
		detail := ctrlDetail(id)
		upd.Controllers = append(upd.Controllers, detail)
		upd.Addresses = append(upd.Addresses, detail.Endpoints[0].Address)
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

// sendClusterLeader delivers a leader announcement for term from controller from, and waits for it to be
// applied. Term 0 stands for a controller that doesn't send one.
func sendClusterLeader(t *testing.T, handler *updateCtrlAddressesHandler, from string, term uint64) {
	handler.clusterLeaderReceiver().HandleReceive(clusterLeaderMsg(t, term), &ctrlListChannel{id: from})
	flush(t, handler)
}

func clusterLeaderMsg(t *testing.T, term uint64) *channel.Message {
	body, err := proto.Marshal(&ctrl_pb.UpdateClusterLeader{Term: term})
	require.NoError(t, err)
	return channel.NewMessage(int32(ctrl_pb.ContentType_UpdateClusterLeaderRequestType), body)
}

// sendLeaderList delivers a leader's controller list naming ids at index, claiming term, and waits for
// it to be applied.
func sendLeaderList(t *testing.T, handler *updateCtrlAddressesHandler, from string, index, term uint64, ids ...string) {
	upd := &ctrl_pb.UpdateCtrlAddresses{IsLeader: true, Index: index, Term: term}
	for _, id := range ids {
		upd.Controllers = append(upd.Controllers, ctrlDetail(id))
	}
	body, err := proto.Marshal(upd)
	require.NoError(t, err)
	handler.HandleReceive(channel.NewMessage(int32(ctrl_pb.ContentType_UpdateCtrlAddressesType), body), &ctrlListChannel{id: from})
	flush(t, handler)
}

// awaitLeaderList marks handler as waiting for the leader's list, without asking anyone.
func awaitLeaderList(handler *updateCtrlAddressesHandler) {
	handler.leaderListRequest.Store(&leaderListRequest{done: make(chan struct{})})
}

func awaitingLeaderList(handler *updateCtrlAddressesHandler) bool {
	return handler.leaderListRequest.Load() != nil
}

// Test_UpdateCtrlAddresses_FollowerListOnlyAdds: a follower's list adds the controllers it names that the
// router doesn't have, and removes none, since a follower that hasn't caught up can omit current members.
func Test_UpdateCtrlAddresses_FollowerListOnlyAdds(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	sendCtrlList(t, handler, "ctrl2", 5, false, "ctrl1")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "a follower's list must not remove controllers")

	sendCtrlList(t, handler, "ctrl2", 6, false, "ctrl1", "ctrl4")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl4"}, ctrls.ids(), "a follower's list must add controllers it names")
}

// Test_UpdateCtrlAddresses_StaleFollowerListsAroundLeaderList: restarted followers send lists naming only
// the leader, before and after the leader's full list. The router keeps every controller and records the
// leader's version.
func Test_UpdateCtrlAddresses_StaleFollowerListsAroundLeaderList(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	sendCtrlList(t, handler, "ctrl2", 1927, false, "ctrl1")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "a stale follower list must not remove controllers")

	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2", "ctrl3")
	sendCtrlList(t, handler, "ctrl3", 1932, false, "ctrl1")

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids())
	req.Equal(uint64(3432), handler.currentVersion)
	req.Equal("ctrl1", ctrls.leader())
}

// Test_UpdateCtrlAddresses_NewerLeaderListReplaces: a leader's list at a newer version is the controller
// set, removals included.
func Test_UpdateCtrlAddresses_NewerLeaderListReplaces(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2")
	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal(uint64(10), handler.currentVersion)

	sendCtrlList(t, handler, "ctrl1", 9, true, "ctrl1", "ctrl2", "ctrl3")
	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "an older leader list must be ignored")
}

// Test_UpdateCtrlAddresses_LeaderListAtRecordedVersionRestoresMissing: a leader's list at the recorded
// version restores controllers the router lacks, without removing any.
func Test_UpdateCtrlAddresses_LeaderListAtRecordedVersionRestoresMissing(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1")
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1")

	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2", "ctrl3")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the leader's list must restore missing controllers")

	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2")
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "only a newer leader list may remove controllers")
}

// Test_UpdateCtrlAddresses_IndexResetAcceptsLowerLeaderList: after an index reset, a leader's list at a
// lower index replaces the set.
func Test_UpdateCtrlAddresses_IndexResetAcceptsLowerLeaderList(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 3432, true, "ctrl1", "ctrl2", "ctrl3")

	handler.NotifyIndexReset()
	sendCtrlList(t, handler, "ctrl1", 5, true, "ctrl1", "ctrl2")

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal(uint64(5), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_KeepsConnectedControllersMissingFromDetails: a router started from bare
// endpoints has connected controllers but no details for them, and a follower's list must not drop them.
// A controller the router dialed keeps the address it dialed. One that dialed in is kept without
// endpoints, since its address is the peer's socket.
func Test_UpdateCtrlAddresses_KeepsConnectedControllersMissingFromDetails(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers()
	ctrls.connected["ctrl2"] = &connectedCtrl{address: "tls:ctrl2:6262"}
	ctrls.connected["ctrl3"] = &connectedCtrl{address: "10.0.0.3:51234", ctrlCh: ctrlchan.NewListenerCtrlChannel()}
	handler := newCtrlListHandler(t, ctrls)

	sendCtrlList(t, handler, "ctrl2", 5, false, "ctrl1")

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids())
	req.Contains(ctrls.GetAll(), "ctrl2", "the dialed controller must not be dropped")
	req.Contains(ctrls.GetAll(), "ctrl3", "the inbound controller must not be dropped")
	req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address)
	req.Empty(ctrls.detail("ctrl3").Endpoints, "an inbound controller's socket address must not become an endpoint")
}

// Test_UpdateCtrlAddresses_ConnectedControllerTakesIncomingDetail: a connected controller with no detail
// that the incoming list names gets the list's detail, not the address of its connection.
func Test_UpdateCtrlAddresses_ConnectedControllerTakesIncomingDetail(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers()
	ctrls.connected["ctrl2"] = &connectedCtrl{address: "10.0.0.2:51234", ctrlCh: ctrlchan.NewListenerCtrlChannel()}
	handler := newCtrlListHandler(t, ctrls)

	sendCtrlList(t, handler, "ctrl3", 5, false, "ctrl1", "ctrl2")

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address)
}

// blockFirstRead makes the first GetControllerDetails, which a merge calls, wait until the test ends or
// release is closed, and returns a channel closed once that read starts.
func blockFirstRead(t *testing.T, ctrls *ctrlListControllers) (reading <-chan struct{}, release func()) {
	readingC := make(chan struct{})
	releaseC := make(chan struct{})
	var readOnce, releaseOnce sync.Once
	ctrls.readHook = func() {
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

// Test_UpdateCtrlAddresses_ReceiveDoesNotWaitForAnUpdateInProgress: receiving a list only records it, so
// a channel's receive goroutine keeps running however many lists arrive while an update is applied.
func Test_UpdateCtrlAddresses_ReceiveDoesNotWaitForAnUpdateInProgress(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	reading, release := blockFirstRead(t, ctrls)

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
	req.Equal([]string{"ctrl1", "ctrl2", "ctrl4"}, ctrls.ids(), "the leader's list must be applied")
	req.Equal(uint64(200), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_IndexResetAppliesBetweenTheUpdatesAroundIt: an index reset received while
// updates are waiting applies after the updates received before it and before those received after it.
func Test_UpdateCtrlAddresses_IndexResetAppliesBetweenTheUpdatesAroundIt(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	reading, release := blockFirstRead(t, ctrls)

	first := ctrlListMsg(t, 1, false, "ctrl1")
	go handler.HandleReceive(first, &ctrlListChannel{id: "ctrl3"})
	waitFor(t, reading, "the first update was not applied")

	handler.HandleReceive(ctrlListMsg(t, 3432, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
	handler.NotifyIndexReset()
	handler.HandleReceive(ctrlListMsg(t, 5, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl2"})

	release()
	flush(t, handler)
	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the list after the reset must be applied")
	req.Equal(uint64(5), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_FollowerListDoesNotReplaceAWaitingLeaderList: a controller's leader list
// waiting to be applied is still applied when the same controller sends a follower list after it.
func Test_UpdateCtrlAddresses_FollowerListDoesNotReplaceAWaitingLeaderList(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	reading, release := blockFirstRead(t, ctrls)

	first := ctrlListMsg(t, 1, false, "ctrl1")
	go handler.HandleReceive(first, &ctrlListChannel{id: "ctrl3"})
	waitFor(t, reading, "the first update was not applied")

	handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
	handler.HandleReceive(ctrlListMsg(t, 15, false, "ctrl1"), &ctrlListChannel{id: "ctrl1"})

	release()
	flush(t, handler)
	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the leader's list must be applied")
	req.Equal(uint64(20), handler.currentVersion)
}

// whileApplying holds the worker in a follower list's merge while the test delivers lists, then lets it
// apply them. The held list names every controller the router has, so it changes nothing.
func whileApplying(t *testing.T, ctrls *ctrlListControllers, handler *updateCtrlAddressesHandler, deliver func()) {
	reading, release := blockFirstRead(t, ctrls)
	// Above any recorded version, so the list reaches the merge.
	first := ctrlListMsg(t, math.MaxUint64, false, ctrls.ids()...)
	go handler.HandleReceive(first, &ctrlListChannel{id: "ctrl9"})
	waitFor(t, reading, "the first update was not applied")
	deliver()
	release()
	flush(t, handler)
}

// Test_UpdateCtrlAddresses_FollowerListAfterAWaitingLeaderListStillAdds: a controller that sends its
// leader list and then, no longer leader, a newer follower list naming a new controller has both applied.
func Test_UpdateCtrlAddresses_FollowerListAfterAWaitingLeaderListStillAdds(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
		handler.HandleReceive(ctrlListMsg(t, 25, false, "ctrl1", "ctrl2", "ctrl4"), &ctrlListChannel{id: "ctrl1"})
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl4"}, ctrls.ids(), "the leader's removal and the follower's addition must both apply")
	req.Equal(uint64(20), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_LaterFollowerListKeepsEarlierAddition: a follower's list naming a new
// controller, then a stale list from the same follower omitting it, both waiting, still add it.
func Test_UpdateCtrlAddresses_LaterFollowerListKeepsEarlierAddition(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 5, true, "ctrl1", "ctrl2", "ctrl3")

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 6, false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"), &ctrlListChannel{id: "ctrl2"})
		handler.HandleReceive(ctrlListMsg(t, 3, false, "ctrl1"), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3", "ctrl4"}, ctrls.ids(), "the earlier list's addition must be kept")
}

// Test_UpdateCtrlAddresses_WaitingFollowerListsAreGatedIndividually: of a follower's waiting lists, a
// controller named only by a list below the recorded version is not added, though a later list is above it.
func Test_UpdateCtrlAddresses_WaitingFollowerListsAreGatedIndividually(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 20, true, "ctrl1", "ctrl2")

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 15, false, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl2"})
		handler.HandleReceive(ctrlListMsg(t, 25, false, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "a controller the leader removed must not come back from a stale list")
}

// Test_UpdateCtrlAddresses_WaitingLeaderListWithHigherIndexIsKept: of one controller's waiting leader
// lists, the one with the higher index is applied even if a lower one arrives after it.
func Test_UpdateCtrlAddresses_WaitingLeaderListWithHigherIndexIsKept(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
		handler.HandleReceive(ctrlListMsg(t, 18, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
	})

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal(uint64(20), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_WaitingListsApplyOnTheirSideOfAnIndexReset: one controller's leader lists from
// before and after an index reset are applied on either side of it.
func Test_UpdateCtrlAddresses_WaitingListsApplyOnTheirSideOfAnIndexReset(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 3432, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		handler.NotifyIndexReset()
		handler.HandleReceive(ctrlListMsg(t, 5, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
	})

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "the list after the reset must be applied")
	req.Equal(uint64(5), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_WaitingFollowerAdditionDoesNotOutliveALaterLeaderRemoval: a follower's
// addition, a leader list without it and a later list from the same follower, all waiting, leave the
// controller removed, as applying them as they arrived does.
func Test_UpdateCtrlAddresses_WaitingFollowerAdditionDoesNotOutliveALaterLeaderRemoval(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2", "ctrl3")

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 25, false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"), &ctrlListChannel{id: "ctrl2"})
		handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		handler.HandleReceive(ctrlListMsg(t, 26, false, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the leader's later list must remove the earlier addition")
	req.Equal(uint64(20), handler.currentVersion)
}

// Test_UpdateCtrlAddresses_TooManyWaitingListsRequestsTheLeadersList: past the bound on waiting lists,
// receiving still doesn't block, the waiting lists are dropped, and the leader's list is requested.
func Test_UpdateCtrlAddresses_TooManyWaitingListsRequestsTheLeadersList(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	leader := newRecordingCtrl("ctrl1")
	ctrls.leaderCtrl = leader
	ctrls.connected["ctrl1"] = leader
	handler := newCtrlListHandler(t, ctrls)

	var msgs []*channel.Message
	for i := range maxPendingCtrlUpdates + 1 {
		msgs = append(msgs, ctrlListMsg(t, uint64(i+1), false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
	}

	whileApplying(t, ctrls, handler, func() {
		received := make(chan struct{})
		go func() {
			defer close(received)
			for _, msg := range msgs {
				handler.HandleReceive(msg, &ctrlListChannel{id: "ctrl2"})
			}
		}()
		waitFor(t, received, "receiving a list blocked past the bound on waiting lists")
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the waiting lists must be dropped")
	requireRequested(t, leader)
}

// Test_UpdateCtrlAddresses_TooManyWaitingListsWithoutAKnownLeaderAsksEveryController: a router that hasn't
// learned the leader, because its list was among those dropped, asks every connected controller.
func Test_UpdateCtrlAddresses_TooManyWaitingListsWithoutAKnownLeaderAsksEveryController(t *testing.T) {
	ctrls := newCtrlListControllers("ctrl1", "ctrl2")
	ctrl1, ctrl2 := newRecordingCtrl("ctrl1"), newRecordingCtrl("ctrl2")
	ctrls.connected["ctrl1"] = ctrl1
	ctrls.connected["ctrl2"] = ctrl2
	handler := newCtrlListHandler(t, ctrls)

	msgs := []*channel.Message{ctrlListMsg(t, 10, true, "ctrl1", "ctrl2")}
	for i := range maxPendingCtrlUpdates {
		msgs = append(msgs, ctrlListMsg(t, uint64(i+1), false, "ctrl1", "ctrl2"))
	}

	whileApplying(t, ctrls, handler, func() {
		for i, msg := range msgs {
			handler.HandleReceive(msg, &ctrlListChannel{id: []string{"ctrl1", "ctrl2"}[min(i, 1)]})
		}
	})

	requireRequested(t, ctrl1)
	requireRequested(t, ctrl2)
}

// Test_UpdateCtrlAddresses_DroppedLeaderAnnouncementStillReachesTheNewLeader: when the lists dropped past
// the bound include another controller's leader announcement, the request for the leader's list reaches
// that controller, not only the former leader the router still has recorded.
func Test_UpdateCtrlAddresses_DroppedLeaderAnnouncementStillReachesTheNewLeader(t *testing.T) {
	ctrls := newCtrlListControllers("ctrl1", "ctrl2")
	formerLeader, newLeader := newRecordingCtrl("ctrl1"), newRecordingCtrl("ctrl2")
	ctrls.connected["ctrl1"] = formerLeader
	ctrls.connected["ctrl2"] = newLeader
	ctrls.leaderCtrl = formerLeader
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2")

	var msgs []*channel.Message
	for i := range maxPendingCtrlUpdates {
		msgs = append(msgs, ctrlListMsg(t, uint64(11+i), false, "ctrl1", "ctrl2"))
	}

	whileApplying(t, ctrls, handler, func() {
		handler.clusterLeaderReceiver().HandleReceive(clusterLeaderMsg(t, 0), &ctrlListChannel{id: "ctrl2"})
		for _, msg := range msgs {
			handler.HandleReceive(msg, &ctrlListChannel{id: "ctrl1"})
		}
	})

	requireRequested(t, newLeader)
}

// Test_UpdateCtrlAddresses_StuckControllerDoesNotHoldUpListRequests: asking every controller for its list
// while the leader is unknown reaches each one, though a controller before it has a full send queue.
func Test_UpdateCtrlAddresses_StuckControllerDoesNotHoldUpListRequests(t *testing.T) {
	ctrls := newCtrlListControllers("ctrl1", "ctrl2")
	ctrl2 := newRecordingCtrl("ctrl2")
	ctrls.connected["ctrl1"] = newStuckCtrl(t, "ctrl1")
	ctrls.connected["ctrl2"] = ctrl2
	handler := newCtrlListHandler(t, ctrls)

	// A follower's list omitting a controller asks for the leader's.
	sendCtrlList(t, handler, "ctrl1", 5, false, "ctrl1")

	requireRequested(t, ctrl2)
}

// Test_UpdateCtrlAddresses_LeaderAnnouncementAppliesInOrderWithLists: a leader announcement received
// after a leader list, both waiting, decides the leader, since the two apply in the order they arrived.
func Test_UpdateCtrlAddresses_LeaderAnnouncementAppliesInOrderWithLists(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	whileApplying(t, ctrls, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 3432, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		handler.clusterLeaderReceiver().HandleReceive(clusterLeaderMsg(t, 0), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal("ctrl2", ctrls.leader(), "the later announcement must decide the leader")
}

// Test_UpdateCtrlAddresses_AnnouncementFromAnEarlierTermIsIgnored: a delayed announcement from a former
// leader doesn't take the leadership back.
func Test_UpdateCtrlAddresses_AnnouncementFromAnEarlierTermIsIgnored(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	sendClusterLeader(t, handler, "ctrl2", 6)
	sendClusterLeader(t, handler, "ctrl1", 5)

	req.Equal("ctrl2", ctrls.leader())
}

// Test_UpdateCtrlAddresses_LeaderListFromAnEarlierTermDoesNotUndoAnAnnouncement: a delayed leader's list
// from a former leader, at the same membership index, doesn't take the leadership back.
func Test_UpdateCtrlAddresses_LeaderListFromAnEarlierTermDoesNotUndoAnAnnouncement(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendLeaderList(t, handler, "ctrl1", 100, 5, "ctrl1", "ctrl2", "ctrl3")

	sendClusterLeader(t, handler, "ctrl2", 6)
	sendLeaderList(t, handler, "ctrl1", 100, 5, "ctrl1", "ctrl2", "ctrl3")

	req.Equal("ctrl2", ctrls.leader())
}

// Test_UpdateCtrlAddresses_SameTermClaimFromAnotherControllerIsIgnored: raft has one leader per term, so
// a second controller claiming a recorded term is stale, while the controller that claimed it may repeat.
func Test_UpdateCtrlAddresses_SameTermClaimFromAnotherControllerIsIgnored(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	sendClusterLeader(t, handler, "ctrl1", 5)
	sendClusterLeader(t, handler, "ctrl2", 5)
	req.Equal("ctrl1", ctrls.leader())

	sendLeaderList(t, handler, "ctrl1", 100, 5, "ctrl1", "ctrl2", "ctrl3")
	req.Equal("ctrl1", ctrls.leader())
}

// Test_UpdateCtrlAddresses_ClaimsWithoutATermApplyAsTheyArrive: claims from controllers that don't send a
// term apply in arrival order, and leave the recorded term to order the claims that carry one.
func Test_UpdateCtrlAddresses_ClaimsWithoutATermApplyAsTheyArrive(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	sendClusterLeader(t, handler, "ctrl2", 6)
	sendClusterLeader(t, handler, "ctrl3", 0)
	req.Equal("ctrl3", ctrls.leader(), "a claim without a term must apply as it arrives")

	sendClusterLeader(t, handler, "ctrl1", 5)
	req.Equal("ctrl3", ctrls.leader(), "a claim without a term must not let an earlier term back in")

	sendClusterLeader(t, handler, "ctrl1", 7)
	req.Equal("ctrl1", ctrls.leader())
}

// Test_UpdateCtrlAddresses_OnlyACurrentLeadersListEndsTheRequestForIt: a list whose leadership claim is
// rejected doesn't stop the router asking for the leader's list; one whose claim is accepted does.
func Test_UpdateCtrlAddresses_OnlyACurrentLeadersListEndsTheRequestForIt(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendClusterLeader(t, handler, "ctrl2", 6)

	awaitLeaderList(handler)
	sendLeaderList(t, handler, "ctrl1", 100, 5, "ctrl1", "ctrl2", "ctrl3")
	req.True(awaitingLeaderList(handler), "a former leader's list must not end the request")

	sendLeaderList(t, handler, "ctrl2", 100, 6, "ctrl1", "ctrl2", "ctrl3")
	req.False(awaitingLeaderList(handler), "the leader's list must end the request")
}

// Test_UpdateCtrlAddresses_ListWithoutATermEndsTheRequestByIndex: where no controller has sent a term, a
// list without one ends the request when its membership index is current, as it always has.
func Test_UpdateCtrlAddresses_ListWithoutATermEndsTheRequestByIndex(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendLeaderList(t, handler, "ctrl1", 100, 0, "ctrl1", "ctrl2", "ctrl3")

	awaitLeaderList(handler)
	sendLeaderList(t, handler, "ctrl3", 50, 0, "ctrl1", "ctrl2", "ctrl3")
	req.True(awaitingLeaderList(handler), "a list without a term below the recorded index must not end the request")

	sendLeaderList(t, handler, "ctrl3", 100, 0, "ctrl1", "ctrl2", "ctrl3")
	req.False(awaitingLeaderList(handler), "a list without a term at the recorded index must end the request")
}

// mixedVersionCluster sets up ctrl1 as an older controller that sends no term and ctrl2 as a newer one
// recorded as leader at term 6, both connected and recording what is sent to them.
func mixedVersionCluster(t *testing.T) (*ctrlListControllers, *updateCtrlAddressesHandler, *recordingCtrl, *recordingCtrl) {
	ctrls := newCtrlListControllers("ctrl1", "ctrl2")
	older, newer := newRecordingCtrl("ctrl1"), newRecordingCtrl("ctrl2")
	ctrls.connected["ctrl1"] = older
	ctrls.connected["ctrl2"] = newer
	handler := newCtrlListHandler(t, ctrls)
	sendLeaderList(t, handler, "ctrl2", 100, 6, "ctrl1", "ctrl2")
	require.Equal(t, "ctrl2", ctrls.leader())
	return ctrls, handler, older, newer
}

// Test_UpdateCtrlAddresses_OlderControllerThatTakesOverLeads: in a mixed-version cluster, an older
// controller's claim without a term applies after a newer controller's claim with one, and its list
// ends the request for the leader's list.
func Test_UpdateCtrlAddresses_OlderControllerThatTakesOverLeads(t *testing.T) {
	req := require.New(t)
	ctrls, handler, _, _ := mixedVersionCluster(t)

	sendClusterLeader(t, handler, "ctrl1", 0)
	req.Equal("ctrl1", ctrls.leader())

	awaitLeaderList(handler)
	sendLeaderList(t, handler, "ctrl1", 100, 0, "ctrl1", "ctrl2")
	sendCtrlList(t, handler, "ctrl2", 100, false, "ctrl1", "ctrl2")

	req.Equal("ctrl1", ctrls.leader())
	req.False(awaitingLeaderList(handler))
}

// Test_UpdateCtrlAddresses_RequestForLeadersListStopsWhenTheRouterCloses: the request repeats while it
// waits for the leader's list, and stops once the router closes.
func Test_UpdateCtrlAddresses_RequestForLeadersListStopsWhenTheRouterCloses(t *testing.T) {
	ctrls := newCtrlListControllers("ctrl1")
	ctrl1 := newRecordingCtrl("ctrl1")
	ctrls.connected["ctrl1"] = ctrl1
	handler := newCtrlListHandler(t, ctrls)
	handler.askInterval = 10 * time.Millisecond

	handler.requestLeaderList()
	requireRequested(t, ctrl1)
	requireRequested(t, ctrl1)

	handler.env.(*ctrlListEnv).stop()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-ctrl1.ch.sent:
	default:
	}

	select {
	case <-ctrl1.ch.sent:
		require.FailNow(t, "the request kept repeating after the router closed")
	case <-time.After(100 * time.Millisecond):
	}
}

// Test_UpdateCtrlAddresses_IndexResetAcceptsLowerLeaderAnnouncement: after an index reset, an
// announcement at a lower term changes the leader.
func Test_UpdateCtrlAddresses_IndexResetAcceptsLowerLeaderAnnouncement(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendClusterLeader(t, handler, "ctrl2", 3500)

	handler.NotifyIndexReset()
	sendClusterLeader(t, handler, "ctrl3", 5)

	req.Equal("ctrl3", ctrls.leader())
}

// Test_UpdateCtrlAddresses_EndedRequestDoesNotJoinALaterOne: a request for the leader's list that has
// ended stops asking, even when another starts before its next round would have come.
func Test_UpdateCtrlAddresses_EndedRequestDoesNotJoinALaterOne(t *testing.T) {
	ctrls := newCtrlListControllers("ctrl1")
	ctrl1 := newRecordingCtrl("ctrl1")
	ctrls.connected["ctrl1"] = ctrl1
	handler := newCtrlListHandler(t, ctrls)

	handler.askInterval = 100 * time.Millisecond
	handler.requestLeaderList()
	requireRequested(t, ctrl1)

	// The leader's list ends the first request, and another starts that won't ask again for an hour.
	sendLeaderList(t, handler, "ctrl1", 10, 5, "ctrl1")
	handler.askInterval = time.Hour
	handler.requestLeaderList()

	time.Sleep(20 * time.Millisecond)
	select {
	case <-ctrl1.ch.sent:
	default:
	}

	select {
	case <-ctrl1.ch.sent:
		require.FailNow(t, "the ended request kept asking alongside the later one")
	case <-time.After(200 * time.Millisecond):
	}
}

// Test_UpdateCtrlAddresses_ControllerDetailWithoutEndpointsIsFilledIn: a controller kept with no endpoints
// takes the endpoints a later list gives it. A controller that has endpoints keeps them.
func Test_UpdateCtrlAddresses_ControllerDetailWithoutEndpointsIsFilledIn(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl3")
	ctrls.details["ctrl2"] = &ctrl_pb.CtrlDetail{Id: "ctrl2"}
	ctrls.details["ctrl3"] = &ctrl_pb.CtrlDetail{Id: "ctrl3", Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: "tls:ctrl3-old:6262"}}}
	handler := newCtrlListHandler(t, ctrls)

	sendCtrlList(t, handler, "ctrl3", 5, false, "ctrl1", "ctrl2", "ctrl3")

	req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "the empty detail must be filled in")
	req.Equal("tls:ctrl3-old:6262", ctrls.detail("ctrl3").Endpoints[0].Address, "a follower's list must not replace endpoints")
}

// Test_UpdateCtrlAddresses_FollowerListCannotUndoConcurrentLeaderRemoval: a follower's list received
// before a newer leader list must not re-add a controller the leader removed.
func Test_UpdateCtrlAddresses_FollowerListCannotUndoConcurrentLeaderRemoval(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)
	sendCtrlList(t, handler, "ctrl1", 10, true, "ctrl1", "ctrl2", "ctrl3")

	followerReading := make(chan struct{})
	leaderApplied := make(chan struct{})
	var readOnce, leaderOnce sync.Once
	ctrls.leaderHook = func() {
		leaderOnce.Do(func() {
			close(leaderApplied)
		})
	}
	ctrls.readHook = func() {
		readOnce.Do(func() {
			close(followerReading)
			// Wait for the leader's list to be applied, unless applying it is waiting on this update.
			select {
			case <-leaderApplied:
			case <-time.After(200 * time.Millisecond):
			}
		})
	}

	handler.HandleReceive(ctrlListMsg(t, 15, false, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl2"})
	select {
	case <-followerReading:
	case <-time.After(5 * time.Second):
		req.FailNow("the follower's list never read the controller details")
	}
	handler.HandleReceive(ctrlListMsg(t, 20, true, "ctrl1", "ctrl2"), &ctrlListChannel{id: "ctrl1"})
	flush(t, handler)

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids(), "a stale follower list must not re-add a removed controller")
	req.Equal(uint64(20), handler.currentVersion)
}
