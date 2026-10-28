package handler_ctrl

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// CtrlAddressUpdater changes the router's controller endpoints and its record of the cluster leader.
type CtrlAddressUpdater interface {
	// UpdateCtrlEndpoints replaces the router's controller endpoints, closing connections to endpoints
	// no longer listed.
	UpdateCtrlEndpoints(endpoints []string)
	UpdateLeader(leaderId string)
	// GetCtrlEndpoints returns the router's current controller endpoints.
	GetCtrlEndpoints() []string
}

// updateCtrlAddressesHandler keeps the router's controller endpoints and its record of the leader
// current. One goroutine applies everything, one update at a time, in the order it arrived:
//
//   - controller lists, from the leader and from followers
//   - leader announcements
//   - a controller's new address, from its NewCtrlAddress setting
//   - index resets, which clear the recorded versions
//
// Controller lists are judged against the recorded version, the index of the last leader's list applied:
//
//   - A leader's list at a newer index replaces the endpoints and becomes the recorded version.
//   - A leader's list at the recorded version, or a follower's list at or above it, only adds endpoints
//     the router doesn't have. A follower that hasn't caught up can omit current members.
//   - Any list below the recorded version is ignored.
//
// So only a leader's list removes controllers. A leader sends its list when a router connects and when
// the cluster's membership changes. A list from a controller that doesn't mark its list as the leader's
// never removes anything.
//
// A leader's list at or above the recorded version, and a leader announcement above the last one
// applied, record the sender as leader.
//
// A new address replaces the router's endpoints with that one address.
//
// Receiving never waits for an update to be applied. Past maxPendingCtrlUpdates waiting updates, the
// waiting follower lists are dropped, and of the rest only each controller's newest leader list, newest
// leader announcement and latest new address are kept. Follower lists only add controllers, and the
// leader sends its own list when membership changes, so a dropped follower list delays an addition until
// the leader's list is applied.
type updateCtrlAddressesHandler struct {
	updater     CtrlAddressUpdater
	closeNotify <-chan struct{}

	// lock guards pending, received and resets. Receive goroutines and index resets only record work
	// under it, and the run goroutine applies the work in the order it was received, so checking an
	// update's index, applying it and recording it is one step, and nothing that records work waits on
	// an update being applied.
	lock     sync.Mutex
	pending  []*pendingCtrlUpdate
	received uint64
	resets   uint64
	wake     chan struct{}

	// currentVersion, leaderVersion and appliedResets belong to the run goroutine. currentVersion is the
	// index of the last leader's list applied, leaderVersion that of the last leader announcement.
	currentVersion uint64
	leaderVersion  uint64
	appliedResets  uint64

	// appliedThrough is the receive sequence through which every update has been applied or dropped.
	appliedThrough atomic.Uint64
}

// maxPendingCtrlUpdates bounds the updates waiting to be applied. See updateCtrlAddressesHandler for
// what is kept past it.
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
	handler.pending = append(handler.pending, p)
	if len(handler.pending) > maxPendingCtrlUpdates {
		handler.pending = boundPendingCtrlUpdates(handler.pending)
	}
	handler.lock.Unlock()
	handler.signal()
}

// queueNewCtrlAddress queues ctrlId's move to address, so it applies in order with the controller lists,
// which also change the endpoints.
func (handler *updateCtrlAddressesHandler) queueNewCtrlAddress(ctrlId string, label string, address string) {
	handler.queue(&pendingCtrlUpdate{newAddress: address, ctrlId: ctrlId, label: label})
}

// boundPendingCtrlUpdates drops the follower lists in pending and keeps, from each controller, its
// newest leader list and leader announcement, newest by index within the latest index reset, and its
// latest new address, in the order they arrived.
func boundPendingCtrlUpdates(pending []*pendingCtrlUpdate) []*pendingCtrlUpdate {
	type sourceKey struct {
		ctrlId string
		kind   int
	}
	const (
		leaderListKind = iota
		clusterLeaderKind
		newAddressKind
	)

	newer := func(a, b *pendingCtrlUpdate, aIndex, bIndex uint64) bool {
		if a.resets != b.resets {
			return a.resets > b.resets
		}
		return aIndex >= bIndex
	}

	kept := map[sourceKey]int{}
	for i, p := range pending {
		switch {
		case p.clusterLeader != nil:
			key := sourceKey{p.ctrlId, clusterLeaderKind}
			if j, ok := kept[key]; !ok || newer(p, pending[j], p.clusterLeader.Index, pending[j].clusterLeader.Index) {
				kept[key] = i
			}
		case p.upd == nil:
			kept[sourceKey{p.ctrlId, newAddressKind}] = i
		case p.upd.IsLeader:
			key := sourceKey{p.ctrlId, leaderListKind}
			if j, ok := kept[key]; !ok || newer(p, pending[j], p.upd.Index, pending[j].upd.Index) {
				kept[key] = i
			}
		}
	}

	var indexes []int
	for _, i := range kept {
		indexes = append(indexes, i)
	}
	slices.Sort(indexes)

	result := make([]*pendingCtrlUpdate, 0, len(indexes))
	for _, i := range indexes {
		result = append(result, pending[i])
	}
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
		case <-handler.closeNotify:
			return
		}
	}
}

// applyPending applies the pending updates in the order they were received, applying each index reset
// after the updates received before it.
func (handler *updateCtrlAddressesHandler) applyPending() {
	handler.lock.Lock()
	updates := handler.pending
	resets := handler.resets
	received := handler.received
	handler.pending = nil
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
	handler.appliedThrough.Store(received)
}

func (handler *updateCtrlAddressesHandler) applyResets(resets uint64) {
	if handler.appliedResets < resets {
		handler.currentVersion = 0
		handler.leaderVersion = 0
		handler.appliedResets = resets
	}
}

func (handler *updateCtrlAddressesHandler) applyClusterLeader(upd *ctrl_pb.UpdateClusterLeader, ctrlId string, label string) {
	log := pfxlog.ContextLogger(label).WithFields(logrus.Fields{
		"localVersion":  handler.leaderVersion,
		"remoteVersion": upd.Index,
		"ctrlId":        ctrlId,
	})

	if handler.leaderVersion == 0 || handler.leaderVersion < upd.Index {
		log.Info("handling update of cluster leader")
		handler.updater.UpdateLeader(ctrlId)
		handler.leaderVersion = upd.Index
	} else {
		log.Info("ignoring outdated update cluster leader message")
	}
}

func (handler *updateCtrlAddressesHandler) applyLeaderList(upd *ctrl_pb.UpdateCtrlAddresses, ctrlId string, label string) {
	log := handler.listLogger(upd, ctrlId, label)
	log.Info("update ctrl endpoints message received")

	if len(upd.Addresses) == 0 {
		log.Info("ignoring empty controller endpoint list")
		return
	}

	switch {
	case handler.currentVersion == 0 || handler.currentVersion < upd.Index:
		log.Info("updating to newer controller endpoints")
		handler.updater.UpdateCtrlEndpoints(upd.Addresses)
		handler.currentVersion = upd.Index

	case handler.currentVersion == upd.Index:
		if merged, added := addEndpoints(handler.updater.GetCtrlEndpoints(), upd.Addresses); added {
			log.Info("adding controllers from controller endpoint list")
			handler.updater.UpdateCtrlEndpoints(merged)
		}

	default:
		log.Info("ignoring outdated controller endpoint list")
		return
	}

	log.Infof("updating current leader to %s", ctrlId)
	handler.updater.UpdateLeader(ctrlId)
}

func (handler *updateCtrlAddressesHandler) applyFollowerList(upd *ctrl_pb.UpdateCtrlAddresses, ctrlId string, label string) {
	log := handler.listLogger(upd, ctrlId, label)
	log.Info("update ctrl endpoints message received")

	if len(upd.Addresses) == 0 {
		log.Info("ignoring empty controller endpoint list")
		return
	}

	if handler.currentVersion != 0 && handler.currentVersion > upd.Index {
		log.Info("ignoring outdated controller endpoint list")
		return
	}

	if merged, added := addEndpoints(handler.updater.GetCtrlEndpoints(), upd.Addresses); added {
		log.Info("updated ctrl list is not from leader, adding controllers it names and removing none")
		handler.updater.UpdateCtrlEndpoints(merged)
	}
}

func (handler *updateCtrlAddressesHandler) listLogger(upd *ctrl_pb.UpdateCtrlAddresses, ctrlId string, label string) *logrus.Entry {
	return pfxlog.ContextLogger(label).WithFields(logrus.Fields{
		"endpoints":     upd.Addresses,
		"localVersion":  handler.currentVersion,
		"remoteVersion": upd.Index,
		"isLeader":      upd.IsLeader,
		"ctrlId":        ctrlId,
	})
}

// addEndpoints returns current plus each endpoint in incoming that current doesn't have, and whether
// incoming added any.
func addEndpoints(current []string, incoming []string) ([]string, bool) {
	merged := slices.Clone(current)
	added := false
	for _, endpoint := range incoming {
		if !slices.Contains(merged, endpoint) {
			merged = append(merged, endpoint)
			added = true
		}
	}
	return merged, added
}

// applyNewCtrlAddress replaces the router's endpoints with address.
func (handler *updateCtrlAddressesHandler) applyNewCtrlAddress(ctrlId string, address string, label string) {
	pfxlog.ContextLogger(label).WithField("ctrlId", ctrlId).WithField("address", address).Info("updating controller address")
	handler.updater.UpdateCtrlEndpoints([]string{address})
}

// newUpdateCtrlAddressesHandler returns a handler applying updates through updater until closeNotify is
// closed. The caller registers NotifyIndexReset with the router's index watchers.
func newUpdateCtrlAddressesHandler(updater CtrlAddressUpdater, closeNotify <-chan struct{}) *updateCtrlAddressesHandler {
	result := &updateCtrlAddressesHandler{
		updater:     updater,
		closeNotify: closeNotify,
		wake:        make(chan struct{}, 1),
	}
	go result.run()
	return result
}

// clusterLeaderReceiver returns the receive handler for leader announcements, which queues them with
// this handler's controller lists.
func (handler *updateCtrlAddressesHandler) clusterLeaderReceiver() channel.TypedReceiveHandler {
	return &clusterLeaderReceiver{handler: handler}
}
