package handler_ctrl

import (
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// updateCtrlAddressesHandler keeps the router's controller set and its record of the leader current. One
// goroutine applies everything, one update at a time, in the order it arrived:
//
//   - controller lists, from the leader and from followers
//   - leader announcements
//   - a controller's new address, from its NewCtrlAddress setting
//   - index resets, which clear the recorded version and term
//
// Controller lists are judged against the recorded version, the index of the last leader's list applied:
//
//   - A leader's list at a newer index replaces the controller set and becomes the recorded version. It
//     is the only update applied as authoritative, so a controller it omits is refused when it connects.
//   - A leader's list at the recorded version, or a follower's list at or above it, only adds controllers
//     the router doesn't have. A follower that hasn't caught up can omit current members, and the router
//     may have lost controllers the leader still names.
//   - Any list below the recorded version is ignored.
//
// Leadership, claimed by announcements and by leaders' lists, is ordered by raft term:
//
//   - A claim above the highest recorded term is applied, and its term recorded.
//   - A claim at the recorded term is applied only from the controller that claimed it, since raft has
//     one leader per term.
//   - A claim without a term comes from a controller that doesn't send one. It is applied as it arrives,
//     from a leader's list only when the list is at or above the recorded version, and leaves the recorded
//     term alone. While such controllers remain, a former leader's delayed claim can still be applied.
//
// A new address replaces that controller's endpoints and keeps every other controller.
//
// Receiving never waits for an update to be applied. Past maxPendingCtrlUpdates waiting updates, the
// waiting lists and announcements are dropped and the leader's list is requested in their place, as it
// also is when a list is empty or a follower's list omits controllers.
type updateCtrlAddressesHandler struct {
	env env.RouterEnv

	// lock guards pending, resync, received and resets. Receive goroutines and index resets only record
	// work under it, and the run goroutine applies the work in the order it was received, so checking an
	// update's index, applying it and recording it is one step, and nothing that records work waits on an
	// update being applied.
	lock     sync.Mutex
	pending  []*pendingCtrlUpdate
	resync   bool
	received uint64
	resets   uint64
	wake     chan struct{}

	// currentVersion, leaderTerm, leaderTermId and appliedResets belong to the run goroutine.
	// leaderTerm is the highest raft term claimed, and leaderTermId the controller that claimed it.
	currentVersion uint64
	leaderTerm     uint64
	leaderTermId   string
	appliedResets  uint64

	// appliedThrough is the receive sequence through which every update has been applied or dropped.
	appliedThrough atomic.Uint64
	// leaderListRequest is the request for the leader's list under way, if any.
	leaderListRequest atomic.Pointer[leaderListRequest]
	askInterval       time.Duration
}

// leaderListRequest is one run of asking every controller for its list. Ending it closes done, so its
// goroutine stops at once, and a later run never shares its state.
type leaderListRequest struct {
	done     chan struct{}
	interval time.Duration
}

// maxPendingCtrlUpdates bounds the updates waiting to be applied. Past it, the waiting lists and
// announcements are dropped and the leader's list is requested in their place. Address settings are
// kept, the latest for each controller, since nothing can request them again.
const maxPendingCtrlUpdates = 1024

// pendingCtrlUpdate is a controller list, or if clusterLeader is set a leader announcement, or if
// neither is set the address a controller has moved to.
type pendingCtrlUpdate struct {
	upd           *ctrl_pb.UpdateCtrlAddresses
	clusterLeader *ctrl_pb.UpdateClusterLeader
	newAddress    string
	ctrlId        string
	label         string
	resets        uint64
}

func (handler *updateCtrlAddressesHandler) NotifyIndexReset() {
	handler.lock.Lock()
	handler.resets++
	handler.lock.Unlock()
	handler.signal()
}

func (handler *updateCtrlAddressesHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateCtrlAddressesType)
}

func (handler *updateCtrlAddressesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	upd := &ctrl_pb.UpdateCtrlAddresses{}
	if err := proto.Unmarshal(msg.Body, upd); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("error unmarshalling")
		return
	}
	handler.queue(&pendingCtrlUpdate{upd: upd, ctrlId: ch.Id(), label: ch.Label()})
}

// queue adds p to what waits for the run goroutine, without waiting itself.
func (handler *updateCtrlAddressesHandler) queue(p *pendingCtrlUpdate) {
	handler.lock.Lock()
	handler.received++
	p.resets = handler.resets
	if len(handler.pending) < maxPendingCtrlUpdates {
		handler.pending = append(handler.pending, p)
	} else {
		handler.pending = latestNewCtrlAddresses(append(handler.pending, p))
		handler.resync = true
	}
	handler.lock.Unlock()
	handler.signal()
}

// queueNewCtrlAddress queues ctrlId's move to address, so it applies in order with the controller lists,
// which also change the controller set.
func (handler *updateCtrlAddressesHandler) queueNewCtrlAddress(ctrlId string, label string, address string) {
	handler.queue(&pendingCtrlUpdate{newAddress: address, ctrlId: ctrlId, label: label})
}

// latestNewCtrlAddresses returns the address settings in pending, the latest for each controller, in the
// order they arrived.
func latestNewCtrlAddresses(pending []*pendingCtrlUpdate) []*pendingCtrlUpdate {
	var result []*pendingCtrlUpdate
	seen := map[string]struct{}{}
	for i := len(pending) - 1; i >= 0; i-- {
		p := pending[i]
		if p.upd != nil || p.clusterLeader != nil {
			continue
		}
		if _, ok := seen[p.ctrlId]; !ok {
			seen[p.ctrlId] = struct{}{}
			result = append(result, p)
		}
	}
	slices.Reverse(result)
	return result
}

// clusterLeaderReceiver queues leader announcements for the handler's run goroutine, so they apply in
// order with the controller lists, which also set the leader.
type clusterLeaderReceiver struct {
	handler *updateCtrlAddressesHandler
}

func (self *clusterLeaderReceiver) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateClusterLeaderRequestType)
}

func (self *clusterLeaderReceiver) HandleReceive(msg *channel.Message, ch channel.Channel) {
	upd := &ctrl_pb.UpdateClusterLeader{}
	if err := proto.Unmarshal(msg.Body, upd); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("error unmarshalling")
		return
	}
	self.handler.queue(&pendingCtrlUpdate{clusterLeader: upd, ctrlId: ch.Id(), label: ch.Label()})
}

func (handler *updateCtrlAddressesHandler) signal() {
	select {
	case handler.wake <- struct{}{}:
	default:
	}
}

func (handler *updateCtrlAddressesHandler) run() {
	for {
		select {
		case <-handler.wake:
			handler.applyPending()
		case <-handler.env.GetCloseNotify():
			return
		}
	}
}

// applyPending applies the pending updates in the order they were received, applying each index reset
// after the updates received before it.
func (handler *updateCtrlAddressesHandler) applyPending() {
	handler.lock.Lock()
	updates := handler.pending
	resync := handler.resync
	resets := handler.resets
	received := handler.received
	handler.pending = nil
	handler.resync = false
	handler.lock.Unlock()

	for _, p := range updates {
		handler.applyResets(p.resets)
		switch {
		case p.clusterLeader != nil:
			handler.applyClusterLeader(p.clusterLeader, p.ctrlId, p.label)
		case p.upd == nil:
			handler.applyNewCtrlAddress(p.ctrlId, p.newAddress, p.label)
		case p.upd.IsLeader:
			handler.applyLeaderList(p.upd, p.ctrlId, p.label)
		default:
			handler.applyFollowerList(p.upd, p.ctrlId, p.label)
		}
	}
	handler.applyResets(resets)

	if resync {
		pfxlog.Logger().Warn("too many controller endpoint lists waiting, dropped them and requesting latest set from leader")
		handler.requestLeaderList()
	}
	handler.appliedThrough.Store(received)
}

func (handler *updateCtrlAddressesHandler) applyResets(resets uint64) {
	if handler.appliedResets < resets {
		handler.currentVersion = 0
		handler.leaderTerm, handler.leaderTermId = 0, ""
		handler.appliedResets = resets
	}
}

// acceptLeaderClaim reports whether ctrlId's claim to lead in term is current, recording the term if it
// is the highest yet. A claim in term 0 is current when legacy is, the test that controllers which don't
// send a term have always been held to, and leaves the recorded term as it was.
func (handler *updateCtrlAddressesHandler) acceptLeaderClaim(ctrlId string, term uint64, legacy bool) bool {
	switch {
	case term == 0:
		return legacy
	case term > handler.leaderTerm:
		handler.leaderTerm, handler.leaderTermId = term, ctrlId
		return true
	default:
		return term == handler.leaderTerm && ctrlId == handler.leaderTermId
	}
}

func (handler *updateCtrlAddressesHandler) applyClusterLeader(upd *ctrl_pb.UpdateClusterLeader, ctrlId string, label string) {
	log := pfxlog.ContextLogger(label).WithFields(logrus.Fields{
		"localTerm":  handler.leaderTerm,
		"remoteTerm": upd.Term,
		"ctrlId":     ctrlId,
	})

	if handler.acceptLeaderClaim(ctrlId, upd.Term, true) {
		log.Info("handling update of cluster leader")
		handler.env.UpdateLeader(ctrlId)
	} else {
		log.Info("ignoring outdated update cluster leader message")
	}
}

func (handler *updateCtrlAddressesHandler) applyLeaderList(upd *ctrl_pb.UpdateCtrlAddresses, ctrlId string, label string) {
	log := handler.listLogger(upd, ctrlId, label)
	log.Info("update ctrl endpoints message received")

	if !hasEndpoints(upd.Controllers) {
		log.Info("ctrl list is empty, ignoring and requesting latest set from leader")
		handler.requestLeaderList()
		return
	}

	switch {
	case handler.currentVersion == 0 || handler.currentVersion < upd.Index:
		log.Info("updating to newer controller endpoints")
		handler.env.UpdateCtrlEndpointDetails(upd.Controllers, true)
		handler.currentVersion = upd.Index

	case handler.currentVersion == upd.Index:
		if merged, changed, _ := mergeControllers(handler.env.GetNetworkControllers(), upd.Controllers); changed {
			log.Info("adding controllers from controller endpoint list")
			handler.env.UpdateCtrlEndpointDetails(merged, false)
		}

	default:
		log.Info("ignoring outdated controller endpoint list")
	}

	legacy := handler.currentVersion == 0 || handler.currentVersion <= upd.Index
	if handler.acceptLeaderClaim(ctrlId, upd.Term, legacy) {
		log.Infof("updating current leader to %s", ctrlId)
		handler.env.UpdateLeader(ctrlId)
		handler.endLeaderListRequest()
	} else {
		log.Info("ignoring outdated leadership claim")
	}
}

func (handler *updateCtrlAddressesHandler) applyFollowerList(upd *ctrl_pb.UpdateCtrlAddresses, ctrlId string, label string) {
	log := handler.listLogger(upd, ctrlId, label)
	log.Info("update ctrl endpoints message received")

	if !hasEndpoints(upd.Controllers) {
		log.Info("ctrl list is empty, ignoring and requesting latest set from leader")
		handler.requestLeaderList()
		return
	}

	if handler.currentVersion != 0 && handler.currentVersion > upd.Index {
		log.Info("ignoring outdated controller endpoint list")
		return
	}

	merged, changed, removed := mergeControllers(handler.env.GetNetworkControllers(), upd.Controllers)
	if removed {
		log.Info("updated ctrl list is not from leader, using only additions and requesting latest set from leader")
		handler.requestLeaderList()
	}
	if changed {
		log.Info("adding controllers from controller endpoint list")
		handler.env.UpdateCtrlEndpointDetails(merged, false)
	}
}

func (handler *updateCtrlAddressesHandler) listLogger(upd *ctrl_pb.UpdateCtrlAddresses, ctrlId string, label string) *logrus.Entry {
	return pfxlog.ContextLogger(label).WithFields(logrus.Fields{
		"endpoints":     upd.Addresses,
		"ctrlDetails":   upd.Controllers,
		"localVersion":  handler.currentVersion,
		"remoteVersion": upd.Index,
		"isLeader":      upd.IsLeader,
		"term":          upd.Term,
		"ctrlId":        ctrlId,
	})
}

func hasEndpoints(controllers []*ctrl_pb.CtrlDetail) bool {
	for _, ctrl := range controllers {
		if len(ctrl.Endpoints) > 0 {
			return true
		}
	}
	return false
}

// mergeControllers returns the router's controller details plus each controller in incoming the details
// don't have, plus each connected controller neither names. A detail with no endpoints takes the
// incoming detail's, since the router has no address for that controller. changed reports whether
// incoming added a controller or endpoints, and removed whether the router has a controller incoming
// doesn't name. A connected controller neither names is listed under the address the router dialed, or
// with no endpoints if it dialed in, since its address is then the peer's socket and nothing to dial.
func mergeControllers(ctrls env.NetworkControllers, incoming []*ctrl_pb.CtrlDetail) (merged []*ctrl_pb.CtrlDetail, changed, removed bool) {
	current := controllerDetails(ctrls)

	incomingIds := map[string]struct{}{}
	for _, ctrl := range incoming {
		incomingIds[ctrl.Id] = struct{}{}
		if existing, ok := current[ctrl.Id]; !ok || (len(existing.Endpoints) == 0 && len(ctrl.Endpoints) > 0) {
			current[ctrl.Id] = ctrl
			changed = true
		}
	}

	addConnectedControllers(ctrls, current)

	for ctrlId := range current {
		if _, ok := incomingIds[ctrlId]; !ok {
			removed = true
		}
	}
	return sortedDetails(current), changed, removed
}

// controllerDetails returns a copy of the router's controller details, empty if it has none.
func controllerDetails(ctrls env.NetworkControllers) map[string]*ctrl_pb.CtrlDetail {
	if details := ctrls.GetControllerDetails(); details != nil {
		return details
	}
	return map[string]*ctrl_pb.CtrlDetail{}
}

// addConnectedControllers adds to details each connected controller it doesn't have, under the address
// the router dialed, or with no endpoints if the controller dialed in, since its address is then the
// peer's socket and nothing to dial.
func addConnectedControllers(ctrls env.NetworkControllers, details map[string]*ctrl_pb.CtrlDetail) {
	for ctrlId, ctrl := range ctrls.GetAll() {
		if _, ok := details[ctrlId]; !ok {
			detail := &ctrl_pb.CtrlDetail{Id: ctrlId}
			if _, inbound := ctrl.CtrlChannel().(*ctrlchan.ListenerCtrlChannel); !inbound {
				detail.Endpoints = []*ctrl_pb.CtrlEndpoint{{Address: ctrl.Address()}}
			}
			details[ctrlId] = detail
		}
	}
}

func sortedDetails(details map[string]*ctrl_pb.CtrlDetail) []*ctrl_pb.CtrlDetail {
	result := slices.Collect(maps.Values(details))
	slices.SortFunc(result, func(a, b *ctrl_pb.CtrlDetail) int {
		return strings.Compare(a.Id, b.Id)
	})
	return result
}

// applyNewCtrlAddress moves ctrlId to address, keeping every other controller the router has.
func (handler *updateCtrlAddressesHandler) applyNewCtrlAddress(ctrlId string, address string, label string) {
	ctrls := handler.env.GetNetworkControllers()
	details := controllerDetails(ctrls)
	details[ctrlId] = &ctrl_pb.CtrlDetail{
		Id:        ctrlId,
		Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: address}},
	}
	addConnectedControllers(ctrls, details)

	pfxlog.ContextLogger(label).WithField("ctrlId", ctrlId).WithField("address", address).Info("updating controller address")
	handler.env.UpdateCtrlEndpointDetails(sortedDetails(details), false)
}

// requestLeaderList starts asking every connected controller for its controller list, every 30 seconds
// until a current leader's list arrives or the router closes, unless that is already under way. It doesn't
// ask only the recorded leader: the update saying another controller leads may still be waiting, or
// have been dropped, and only the leader's list identifies the leader.
func (handler *updateCtrlAddressesHandler) requestLeaderList() {
	request := &leaderListRequest{done: make(chan struct{}), interval: handler.askInterval}
	if handler.leaderListRequest.CompareAndSwap(nil, request) {
		go handler.askForLeaderList(request)
	}
}

// endLeaderListRequest stops the request for the leader's list under way, if any.
func (handler *updateCtrlAddressesHandler) endLeaderListRequest() {
	if request := handler.leaderListRequest.Swap(nil); request != nil {
		close(request.done)
	}
}

func (handler *updateCtrlAddressesHandler) askForLeaderList(request *leaderListRequest) {
	for {
		pfxlog.Logger().Info("requesting latest cluster members from every connected controller")
		all := handler.env.GetNetworkControllers().GetAll()
		for _, ctrlId := range slices.Sorted(maps.Keys(all)) {
			if ctrl := all[ctrlId]; ctrl.IsConnected() {
				requestCtrlList(ctrl)
			}
		}

		select {
		case <-time.After(request.interval):
		case <-request.done:
			return
		case <-handler.env.GetCloseNotify():
			return
		}
	}
}

// requestCtrlList asks ctrl for its controller list. It doesn't wait on a full send queue, so one stuck
// controller can't hold up requests to the others; the dropped request is repeated with the next round.
func requestCtrlList(ctrl env.NetworkController) {
	log := pfxlog.Logger().WithField("ctrlId", ctrl.Channel().Id())
	msg := channel.NewMessage(int32(ctrl_pb.ContentType_RequestClusterMembers), nil)
	if queued, err := ctrl.Channel().TrySend(msg); err != nil {
		log.WithError(err).Error("error sending request for latest cluster members")
	} else if !queued {
		log.Info("send queue full, request for latest cluster members will be retried")
	}
}

func newUpdateCtrlAddressesHandler(env env.RouterEnv) *updateCtrlAddressesHandler {
	result := &updateCtrlAddressesHandler{
		env:         env,
		wake:        make(chan struct{}, 1),
		askInterval: 30 * time.Second,
	}

	env.GetIndexWatchers().AddIndexResetWatcher(result.NotifyIndexReset)
	go result.run()

	return result
}

// clusterLeaderReceiver returns the receive handler for leader announcements, which queues them with
// this handler's controller lists.
func (handler *updateCtrlAddressesHandler) clusterLeaderReceiver() channel.ContentTypeReceiver {
	return &clusterLeaderReceiver{handler: handler}
}
