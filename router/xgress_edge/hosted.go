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

package xgress_edge

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/common/handler_common"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/idgen"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func newHostedServicesRegistry(env routerEnv.RouterEnv, stateManager state.Manager) *hostedServiceRegistry {
	result := &hostedServiceRegistry{
		terminators:          cmap.New[*edgeTerminator](),
		events:               make(chan terminatorEvent),
		env:                  env,
		stateManager:         stateManager,
		triggerEvalC:         make(chan struct{}, 1),
		establishSet:         map[string]*edgeTerminator{},
		deleteSet:            map[string]*edgeTerminator{},
		notifyCloseSet:       map[string]*pendingSdkCloseNotification{},
		postCreateInspectSet: map[string]*pendingPostCreateInspect{},
	}
	go result.run()
	return result
}

type hostedServiceRegistry struct {
	terminators          cmap.ConcurrentMap[string, *edgeTerminator]
	events               chan terminatorEvent
	env                  routerEnv.RouterEnv
	stateManager         state.Manager
	establishSet         map[string]*edgeTerminator
	deleteSet            map[string]*edgeTerminator
	notifyCloseSet       map[string]*pendingSdkCloseNotification
	postCreateInspectSet map[string]*pendingPostCreateInspect
	triggerEvalC         chan struct{}
}

type terminatorEvent interface {
	handle(registry *hostedServiceRegistry)
}

func (self *hostedServiceRegistry) run() {
	longQueueCheckTicker := time.NewTicker(time.Minute)
	defer longQueueCheckTicker.Stop()

	quickTick := time.NewTicker(10 * time.Millisecond)
	defer quickTick.Stop()

	for {
		var rateLimitedTick <-chan time.Time
		if self.env.GetCtrlRateLimiter().IsRateLimited() {
			rateLimitedTick = quickTick.C
		}
		select {
		case <-self.env.GetCloseNotify():
			return
		case event := <-self.events:
			event.handle(self)
		case <-longQueueCheckTicker.C:
			self.scanForRetries()
		case <-self.triggerEvalC:
		case <-rateLimitedTick:
		}

		// events should be quick to handle, so make sure we do all them before we
		// try the establish/delete queues
		allEventsHandled := false
		for !allEventsHandled {
			select {
			case event := <-self.events:
				event.handle(self)
			default:
				allEventsHandled = true
			}
		}

		if !self.env.GetCtrlRateLimiter().IsRateLimited() {
			self.evaluateEstablishQueue()
		}

		if !self.env.GetCtrlRateLimiter().IsRateLimited() {
			self.evaluateDeleteQueue()
		}

		self.evaluateNotifyCloseQueue()
		self.evaluatePostCreateInspects()
	}
}

func (self *hostedServiceRegistry) triggerEvaluates() {
	select {
	case self.triggerEvalC <- struct{}{}:
	default:
	}
}

func (self *hostedServiceRegistry) evaluateEstablishQueue() {
	for id, terminator := range self.establishSet {
		dequeue := func() {
			delete(self.establishSet, id)
		}

		log := logrus.
			WithField("terminatorId", terminator.terminatorId).
			WithField("state", terminator.state.Load()).
			WithField("serviceSessionTokenId", terminator.serviceSessionToken.TokenId())

		if terminator.edgeClientConn.ch.GetChannel().IsClosed() {
			self.queueRemoveTerminatorUnchecked(terminator, "sdk connection is closed")
			log.Infof("terminator sdk channel closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.terminatorId == "" {
			log.Info("terminator has been closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.state.Load() != xgress_common.TerminatorStateEstablishing {
			dequeue()
			continue
		}

		label := fmt.Sprintf("establish terminator %s", terminator.terminatorId)
		rateLimitCtrl, err := self.env.GetCtrlRateLimiter().RunRateLimited(label)
		if err != nil {
			log.Info("rate limiter hit, waiting for a slot to open")
			return
		}

		if !terminator.operationActive.CompareAndSwap(false, true) && time.Since(terminator.lastAttempt) < 30*time.Second {
			rateLimitCtrl.Failed()
			continue
		}

		log.Info("queuing terminator to send create")

		dequeue()
		terminator.SetRateLimitCallback(rateLimitCtrl)
		terminator.lastAttempt = time.Now()

		if err = self.establishTerminator(terminator); err != nil {
			log.WithError(err).Error("error establishing terminator")
			self.queueEstablishTerminatorSync(terminator)
			rateLimitCtrl.Failed()
			return // if we had an error it's because the channel was busy or a controller wasn't available
		}
	}
}

func (self *hostedServiceRegistry) evaluateDeleteQueue() {
	var deleteList []*edgeTerminator

	for terminatorId, terminator := range self.deleteSet {
		log := logrus.
			WithField("terminatorId", terminator.terminatorId).
			WithField("state", terminator.state.Load()).
			WithField("serviceSessionTokenId", terminator.serviceSessionToken.TokenId())

		delete(self.deleteSet, terminatorId)

		if terminator.state.Load() != xgress_common.TerminatorStateDeleting {
			continue
		}

		if current, exists := self.terminators.Get(terminatorId); exists && current != terminator {
			continue
		}

		if terminator.operationActive.Load() {
			if time.Since(terminator.lastAttempt) > 30*time.Second {
				terminator.operationActive.Store(false)
			} else {
				continue
			}
		}

		log.Info("added terminator to batch delete")
		deleteList = append(deleteList, terminator)
		if len(deleteList) >= 50 {
			if !self.RemoveTerminatorsRateLimited(deleteList) {
				return
			}
			deleteList = nil
		}
	}

	if len(deleteList) != 0 {
		self.RemoveTerminatorsRateLimited(deleteList)
	}
}

func (self *hostedServiceRegistry) RemoveTerminatorsRateLimited(terminators []*edgeTerminator) bool {
	if self.env.GetCtrlRateLimiter().IsRateLimited() {
		self.requeueForDeleteSync(terminators)
		return false
	}

	for _, terminator := range terminators {
		terminator.operationActive.Store(true)
	}

	err := self.env.GetRateLimiterPool().QueueOrError(func() {
		rateLimitCtrl, err := self.env.GetCtrlRateLimiter().RunRateLimited("remove terminator batch")
		if err != nil {
			pfxlog.Logger().Debug("rate limiter hit, waiting for a slot to open before doing sdk terminator deletes")

			for _, terminator := range terminators {
				pfxlog.Logger().WithError(err).WithField("terminatorId", terminator.terminatorId).
					Error("remove terminator failed")
				self.requeueRemoveTerminatorAsync(terminator)
			}
			return
		}

		var terminatorIds []string
		for _, terminator := range terminators {
			terminatorIds = append(terminatorIds, terminator.terminatorId)
		}

		if err := self.RemoveTerminators(terminatorIds); err != nil {
			if command.WasRateLimited(err) {
				rateLimitCtrl.Backoff()
			} else {
				rateLimitCtrl.Failed()
			}

			for _, terminator := range terminators {
				pfxlog.Logger().WithError(err).WithField("terminatorId", terminator.terminatorId).
					Error("remove terminator failed")
				self.requeueRemoveTerminatorAsync(terminator)
			}
		} else {
			rateLimitCtrl.Success()
			for _, terminator := range terminators {
				pfxlog.Logger().WithField("terminatorId", terminator.terminatorId).
					Info("remove terminator succeeded")
				terminator.operationActive.Store(false)
				if !self.Remove(terminator, "controller delete success") {
					// A new terminator with this ID was created while the old delete was in flight.
					// The controller just deleted the ID, so we need to re-establish the replacement.
					if current, exists := self.terminators.Get(terminator.terminatorId); exists {
						pfxlog.Logger().WithField("terminatorId", terminator.terminatorId).
							Info("terminator was replaced during delete, re-establishing replacement")
						current.updateState(xgress_common.TerminatorStateEstablished, xgress_common.TerminatorStateEstablishing, "re-establishing after delete/create race")
						self.queueEstablishTerminatorAsync(current)
					}
				}
			}
		}
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to queue remove terminators operation")
		self.requeueForDeleteSync(terminators)
		return false
	}

	return true
}

func (self *hostedServiceRegistry) requeueForDeleteSync(terminators []*edgeTerminator) {
	for _, terminator := range terminators {
		existing, _ := self.terminators.Get(terminator.terminatorId)
		if existing == nil || existing == terminator { // make sure we're still the current terminator
			terminator.setState(xgress_common.TerminatorStateDeleting, "deleting")
			terminator.operationActive.Store(false)
			self.deleteSet[terminator.terminatorId] = terminator
		}
	}
}

func (self *hostedServiceRegistry) RemoveTerminators(terminatorIds []string) error {
	log := pfxlog.Logger()
	request := &ctrl_pb.RemoveTerminatorsRequest{
		TerminatorIds: terminatorIds,
	}

	ctrls := self.env.GetNetworkControllers()
	ctrlCh := ctrls.AnyValidCtrlChannel()
	if ctrlCh == nil {
		return errors.New("no controller available")
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(ctrls.DefaultRequestTimeout()).SendForReply(ctrlCh)
	if err != nil {
		return fmt.Errorf("failed to send RemoveTerminatorsRequest message (%w)", err)
	}

	if responseMsg.ContentType != channel.ContentTypeResultType {
		return fmt.Errorf("failure deleting terminators (unexpected response content type: %d)", responseMsg.ContentType)
	}

	result := channel.UnmarshalResult(responseMsg)
	if result.Success {
		return nil
	}

	if handler_common.WasRateLimited(responseMsg) {
		log.Errorf("failure removing terminators (%v)", result.Message)
		return apierror.NewTooManyUpdatesError()
	}

	return fmt.Errorf("failure deleting terminators (%s)", result.Message)
}

type queueEstablishTerminator struct {
	terminator *edgeTerminator
}

func (self *queueEstablishTerminator) handle(registry *hostedServiceRegistry) {
	registry.queueEstablishTerminatorSync(self.terminator)
}

type requeueRemoveTerminator struct {
	terminator *edgeTerminator
}

func (self *requeueRemoveTerminator) handle(registry *hostedServiceRegistry) {
	registry.requeueRemoveTerminatorSync(self.terminator)
}

type queueRemoveTerminator struct {
	terminator *edgeTerminator
	reason     string
}

func (self *queueRemoveTerminator) handle(registry *hostedServiceRegistry) {
	registry.queueRemoveTerminatorSync(self.terminator, self.reason)
}

func (self *hostedServiceRegistry) EstablishTerminator(terminator *edgeTerminator) {
	self.queueEstablishTerminatorAsync(terminator)
}

func (self *hostedServiceRegistry) queue(event terminatorEvent) {
	select {
	case self.events <- event:
	case <-self.env.GetCloseNotify():
		pfxlog.Logger().Error("unable to queue terminator event, hosted service registry has been shutdown")
	}
}

func (self *hostedServiceRegistry) queueWithTimeout(event terminatorEvent, timeout time.Duration) error {
	select {
	case self.events <- event:
		return nil
	case <-time.After(timeout):
		return errors.New("timed out")
	case <-self.env.GetCloseNotify():
		return errors.New("closed")
	}
}

func (self *hostedServiceRegistry) queueEstablishTerminatorSync(terminator *edgeTerminator) {
	if terminator.IsEstablishing() {
		terminator.operationActive.Store(false)
		self.establishSet[terminator.terminatorId] = terminator
	}
}

func (self *hostedServiceRegistry) queueEstablishTerminatorAsync(terminator *edgeTerminator) {
	self.queue(&queueEstablishTerminator{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) queueRemoveTerminatorAsync(terminator *edgeTerminator, reason string) {
	self.queue(&queueRemoveTerminator{
		terminator: terminator,
		reason:     reason,
	})
}

func (self *hostedServiceRegistry) requeueRemoveTerminatorAsync(terminator *edgeTerminator) {
	self.queue(&requeueRemoveTerminator{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) requeueRemoveTerminatorSync(terminator *edgeTerminator) {
	existing, _ := self.terminators.Get(terminator.terminatorId)
	if existing == nil || existing == terminator && terminator.state.Load() == xgress_common.TerminatorStateDeleting { // make sure we're still the current terminator
		terminator.operationActive.Store(false)
		self.deleteSet[terminator.terminatorId] = terminator
	}
}

func (self *hostedServiceRegistry) queueRemoveTerminatorSync(terminator *edgeTerminator, reason string) {
	existing, _ := self.terminators.Get(terminator.terminatorId)
	if existing == nil || existing == terminator { // make sure we're still the current terminator
		self.queueRemoveTerminatorUnchecked(terminator, reason)
	}
}

func (self *hostedServiceRegistry) queueRemoveTerminatorUnchecked(terminator *edgeTerminator, reason string) {
	terminator.setState(xgress_common.TerminatorStateDeleting, reason)
	terminator.operationActive.Store(false)
	self.deleteSet[terminator.terminatorId] = terminator
}

type pendingSdkCloseNotification struct {
	terminator *edgeTerminator
	reason     string
	edgeErr    *EdgeError
}

type pendingPostCreateInspect struct {
	terminator  *edgeTerminator
	requestSeqs map[int32]struct{}
	lastSent    time.Time
	firstSent   time.Time
	sendCount   int
}

type addSdkCloseNotificationEvent struct {
	notification *pendingSdkCloseNotification
}

func (self *addSdkCloseNotificationEvent) handle(registry *hostedServiceRegistry) {
	if !self.notification.terminator.GetChannel().IsClosed() {
		registry.notifyCloseSet[idgen.MustNewUUIDString()] = self.notification
	}
}

func (self *hostedServiceRegistry) addPendingSdkCloseNotification(terminator *edgeTerminator, reason string, edgeErr *EdgeError) {
	// We can't necessarily key by terminator as terminators can be replaced and the id can be reused.
	// We can't use the virtual conn id, because those aren't unique across routers.
	// If we do happen to send multiple, it should be harmless, since the second close will be ignored.
	// We could probably use a combination of terminator id and virtual conn id.
	// Let's try that next time we run a longer test cycle
	self.notifyCloseSet[idgen.MustNewUUIDString()] = &pendingSdkCloseNotification{
		terminator: terminator,
		reason:     reason,
		edgeErr:    edgeErr,
	}
}

func (self *hostedServiceRegistry) ensureSdkCloseSent(terminator *edgeTerminator, reason string, edgeErr *EdgeError) {
	self.ensureSdkCloseSentInner(terminator, reason, edgeErr, false)
}

func (self *hostedServiceRegistry) ensureSdkCloseSentInner(terminator *edgeTerminator, reason string, edgeErr *EdgeError, inEventLoop bool) {
	if terminator.GetChannel().IsClosed() {
		return
	}
	closeMsg := edge.NewStateClosedMsg(terminator.Id(), reason)
	if edgeErr != nil {
		edgeErr.ApplyToMsg(closeMsg)
	}
	queued, _ := terminator.GetDefaultSender().TrySend(closeMsg)
	if !queued {
		if inEventLoop {
			self.addPendingSdkCloseNotification(terminator, reason, edgeErr)
		} else {
			self.queueSdkCloseNotification(terminator, reason, edgeErr)
		}
	}
}

func (self *hostedServiceRegistry) queueSdkCloseNotification(terminator *edgeTerminator, reason string, edgeErr *EdgeError) {
	self.queue(&addSdkCloseNotificationEvent{
		notification: &pendingSdkCloseNotification{
			terminator: terminator,
			reason:     reason,
			edgeErr:    edgeErr,
		},
	})
}

func (self *hostedServiceRegistry) evaluateNotifyCloseQueue() {
	for id, notification := range self.notifyCloseSet {
		if notification.terminator.GetChannel().IsClosed() {
			delete(self.notifyCloseSet, id)
			continue
		}

		closeMsg := edge.NewStateClosedMsg(notification.terminator.Id(), notification.reason)
		if notification.edgeErr != nil {
			notification.edgeErr.ApplyToMsg(closeMsg)
		}
		queued, _ := notification.terminator.GetDefaultSender().TrySend(closeMsg)
		if queued {
			delete(self.notifyCloseSet, id)
		}
		// if !queued: channel busy, leave in set for next iteration
	}
}

// queuePostCreateInspect queues an event to add a terminator to the post-create inspect set.
// This is safe to call from any goroutine (e.g., from the edge channel handler goroutine).
func (self *hostedServiceRegistry) queuePostCreateInspect(terminator *edgeTerminator) {
	self.queue(&addPostCreateInspectEvent{terminator: terminator})
}

type addPostCreateInspectEvent struct {
	terminator *edgeTerminator
}

func (self *addPostCreateInspectEvent) handle(registry *hostedServiceRegistry) {
	// Only add if this terminator is still the current one in the registry.
	// Multiple takeovers of the same terminatorId can result in multiple
	// addPostCreateInspectEvents for different terminator objects but the
	// same terminatorId. Without this check, a stale event could overwrite
	// the current one's entry, causing the current inspect to be lost when
	// evaluatePostCreateInspects sees the stale entry as "no longer current".
	current, exists := registry.terminators.Get(self.terminator.terminatorId)
	if !exists || current != self.terminator {
		return
	}
	registry.postCreateInspectSet[self.terminator.terminatorId] = &pendingPostCreateInspect{
		terminator:  self.terminator,
		requestSeqs: map[int32]struct{}{},
	}
}

// inspectResponseEvent is queued by the edge channel handler when an inspect response is received
// from the SDK. We correlate with pending inspects using the request sequence number because
// go-sdk versions older than 1.5 don't return the connId in inspect responses.
type inspectResponseEvent struct {
	conn     *edgeClientConn
	replyFor int32
	result   *edge.InspectResult
}

func (self *inspectResponseEvent) handle(registry *hostedServiceRegistry) {
	for id, pending := range registry.postCreateInspectSet {
		if pending.terminator.edgeClientConn != self.conn {
			continue
		}
		if _, found := pending.requestSeqs[self.replyFor]; !found {
			continue
		}

		log := pfxlog.Logger().
			WithField("terminatorId", pending.terminator.terminatorId).
			WithField("connId", pending.terminator.MsgChannel.Id()).
			WithField("connType", self.result.Type)

		delete(registry.postCreateInspectSet, id)

		if self.result.Type != edge.ConnTypeBind {
			log.Info("post-create inspect: sdk reported terminator invalid, closing")
			// don't need to notify the sdk because the sdk just told us it doesn't know about the terminator.
			// can't call close() here because we're on the event loop and close would deadlock
			// trying to queue back to the event channel
			registry.queueRemoveTerminatorSync(pending.terminator, "post-create inspect: terminator sdk state invalid")
			if pending.terminator.onClose != nil {
				pending.terminator.onClose()
			}
		} else {
			log.Info("post-create inspect: sdk confirmed terminator valid")
		}
		return
	}
}

func (self *hostedServiceRegistry) evaluatePostCreateInspects() {
	for id, pending := range self.postCreateInspectSet {
		log := pfxlog.Logger().
			WithField("terminatorId", pending.terminator.terminatorId).
			WithField("connId", pending.terminator.MsgChannel.Id())

		// Check if the terminator is still current in the registry
		current, exists := self.terminators.Get(pending.terminator.terminatorId)
		if !exists || current != pending.terminator {
			log.Info("post-create inspect: terminator no longer current, removing from inspect set")
			delete(self.postCreateInspectSet, id)
			continue
		}

		if pending.terminator.edgeClientConn.ch.GetChannel().IsClosed() {
			log.Info("post-create inspect: channel closed, closing terminator")
			delete(self.postCreateInspectSet, id)
			self.queueRemoveTerminatorSync(pending.terminator, "post-create inspect: channel closed")
			if pending.terminator.onClose != nil {
				pending.terminator.onClose()
			}
			continue
		}

		// If we've been trying long enough with enough sends, consider it dead.
		// Can't call close() here because we're on the event loop and close would deadlock
		// trying to queue back to the event channel.
		if !pending.firstSent.IsZero() && pending.sendCount >= 3 && time.Since(pending.firstSent) > 10*time.Minute {
			log.Warn("post-create inspect: timed out waiting for response, closing terminator")
			delete(self.postCreateInspectSet, id)
			self.queueRemoveTerminatorSync(pending.terminator, "post-create inspect timed out")
			if pending.terminator.onClose != nil {
				pending.terminator.onClose()
			}
			continue
		}

		// Send/resend inspect request if interval has elapsed
		if pending.lastSent.IsZero() || time.Since(pending.lastSent) > 10*time.Second {
			msg := channel.NewMessage(edge.ContentTypeConnInspectRequest, nil)
			msg.PutUint32Header(edge.ConnIdHeader, pending.terminator.Id())
			queued, err := pending.terminator.GetControlSender().TrySend(msg)
			if err != nil {
				log.WithError(err).Warn("post-create inspect: error sending inspect request, removing")
				delete(self.postCreateInspectSet, id)
				continue
			}
			if queued {
				pending.requestSeqs[msg.Sequence()] = struct{}{}
				pending.lastSent = time.Now()
				pending.sendCount++
				if pending.firstSent.IsZero() {
					pending.firstSent = time.Now()
				}
				log.WithField("sendCount", pending.sendCount).Info("post-create inspect: sent inspect request")
			}
		}
	}
}

func (self *hostedServiceRegistry) markEstablished(terminator *edgeTerminator, reason string) {
	self.queue(&markEstablishedEvent{
		terminator: terminator,
		reason:     reason,
	})
}

func (self *hostedServiceRegistry) scanForRetries() {
	var retryList []*edgeTerminator

	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		currentState := terminator.state.Load()
		if currentState.IsWorkRequired() && time.Since(terminator.lastAttempt) > 2*time.Minute {
			retryList = append(retryList, terminator)
		}
	})

	for _, terminator := range retryList {
		currentState := terminator.state.Load()
		if currentState == xgress_common.TerminatorStateEstablishing {
			self.queueEstablishTerminatorSync(terminator)
		} else if currentState == xgress_common.TerminatorStateDeleting {
			self.requeueRemoveTerminatorSync(terminator)
		}
	}
}

func (self *hostedServiceRegistry) PutV1(serviceSessionToken *state.ServiceSessionToken, terminator *edgeTerminator) {
	self.terminators.Set(serviceSessionToken.TokenId(), terminator)
}

func (self *hostedServiceRegistry) Put(terminator *edgeTerminator) {
	self.terminators.Set(terminator.terminatorId, terminator)
}

func (self *hostedServiceRegistry) Get(terminatorId string) (*edgeTerminator, bool) {
	return self.terminators.Get(terminatorId)
}

func (self *hostedServiceRegistry) Delete(terminatorId string) {
	self.terminators.Remove(terminatorId)
}

func (self *hostedServiceRegistry) Remove(terminator *edgeTerminator, reason string) bool {
	removed := self.terminators.RemoveCb(terminator.terminatorId, func(key string, v *edgeTerminator, exists bool) bool {
		return v == terminator
	})
	if removed {
		pfxlog.Logger().WithField("terminatorId", terminator.terminatorId).
			WithField("reason", reason).
			Info("terminator removed from router set")
	}
	return removed
}

func (self *hostedServiceRegistry) cleanupServices(ch channel.Channel) {
	self.queue(&channelClosedEvent{
		ch: ch,
	})
}

func (self *hostedServiceRegistry) cleanupDuplicates(newest *edgeTerminator) {
	var toClose []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator != newest && newest.serviceSessionToken.TokenId() == terminator.serviceSessionToken.TokenId() && newest.instance == terminator.instance {
			toClose = append(toClose, terminator)
		}
	})

	for _, terminator := range toClose {
		terminator.close(self, true, "duplicate terminator") // don't notify sdk, channel is already closed, we can't send messages
		pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
			WithField("serviceSessionTokenId", terminator.serviceSessionToken.TokenId()).
			WithField("instance", terminator.instance).
			WithField("terminatorId", terminator.terminatorId).
			WithField("duplicateOf", newest.terminatorId).
			Info("duplicate removed")
	}
}

func (self *hostedServiceRegistry) unbindSession(connId uint32, serviceSessionToken *state.ServiceSessionToken, proxy *edgeClientConn) bool {
	var toClose []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.MsgChannel.Id() == connId && terminator.serviceSessionToken.TokenId() == serviceSessionToken.TokenId() && terminator.edgeClientConn == proxy {
			toClose = append(toClose, terminator)
		}
	})

	atLeastOneRemoved := false
	for _, terminator := range toClose {
		terminator.close(self, true, "unbind successful") // don't notify sdk, sdk asked us to unbind
		pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
			WithField("serviceSessionTokenId", serviceSessionToken.TokenId()).
			WithField("connId", connId).
			WithField("terminatorId", terminator.terminatorId).
			Info("terminator removed")
		atLeastOneRemoved = true
	}
	return atLeastOneRemoved
}

func (self *hostedServiceRegistry) getRelatedTerminators(connId uint32, serviceSessionToken *state.ServiceSessionToken, proxy *edgeClientConn) []*edgeTerminator {
	var related []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.MsgChannel.Id() == connId && terminator.serviceSessionToken.TokenId() == serviceSessionToken.TokenId() && terminator.edgeClientConn == proxy {
			related = append(related, terminator)
		}
	})

	return related
}

func (self *hostedServiceRegistry) establishTerminator(terminator *edgeTerminator) error {
	factory := terminator.edgeClientConn.listener.factory

	log := pfxlog.Logger().
		WithField("routerId", factory.env.GetRouterId().Token).
		WithField("terminatorId", terminator.terminatorId)
	log = terminator.serviceSessionToken.AddLoggingFields(log)

	request := &edge_ctrl_pb.CreateTerminatorV2Request{
		Address:         terminator.terminatorId,
		SessionToken:    terminator.serviceSessionToken.Token(),
		ApiSessionToken: terminator.serviceSessionToken.ApiSessionToken.Token(),
		Fingerprints:    terminator.edgeClientConn.fingerprints.Prints(),
		PeerData:        terminator.hostData,
		Cost:            uint32(terminator.cost),
		Precedence:      terminator.precedence,
		InstanceId:      terminator.instance,
		InstanceSecret:  terminator.instanceSecret,
	}

	if xgress_common.IsBearerToken(request.SessionToken) {
		apiSession := state.GetApiSessionTokenFromCh(terminator.GetChannel())

		if apiSession == nil {
			return errors.New("could not find api session for channel, unable to process bind message")
		}

		request.ApiSessionToken = apiSession.Token()
	}

	ctrlCh := terminator.edgeClientConn.apiSessionToken.SelectModelUpdateCtrlCh(factory.ctrls)

	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log.WithField("ctrlId", ctrlCh.Id()).Info("sending create terminator v2 request")

	queued, err := ctrlCh.TrySend(protobufs.MarshalTyped(request).ToSendable())
	if err != nil {
		return err
	}
	if !queued {
		return errors.New("channel too busy")
	}
	return nil
}

func (self *hostedServiceRegistry) HandleCreateTerminatorResponse(msg *channel.Message, _ channel.Channel) {
	defer self.triggerEvaluates()

	log := pfxlog.Logger().WithField("routerId", self.env.GetRouterId().Token)

	response := &edge_ctrl_pb.CreateTerminatorV2Response{}

	if err := proto.Unmarshal(msg.Body, response); err != nil {
		log.WithError(err).Error("error unmarshalling create terminator v2 response")
		return
	}

	log = log.WithField("terminatorId", response.TerminatorId)

	terminator, found := self.Get(response.TerminatorId)
	if !found {
		log.Error("no terminator found for id")
		return
	}

	log = log.WithField("lifetime", time.Since(terminator.createTime)).
		WithField("connId", terminator.MsgChannel.Id())

	if response.Result == edge_ctrl_pb.CreateTerminatorResult_FailedBusy {
		log.Info("controller too busy to handle create terminator, retrying later")
		if rateLimitCallback := terminator.GetAndClearRateLimitCallback(); rateLimitCallback != nil {
			rateLimitCallback.Backoff()
		}
		self.queueEstablishTerminatorAsync(terminator)
		return
	}

	if response.Result != edge_ctrl_pb.CreateTerminatorResult_Success {
		if rateLimitCallback := terminator.GetAndClearRateLimitCallback(); rateLimitCallback != nil {
			rateLimitCallback.Success()
		}

		terminator.operationActive.Store(false)
		terminator.NotifyEstablished(response.Result)

		// If the retry hint is start over or permanent
		if edge.RetryHint(response.GetRetryHint()) == edge.RetryDefault {
			if terminator.failureCount.Add(1) == 3 {
				edgeErr := &EdgeError{
					Message:   response.Msg,
					Code:      response.ErrorCode,
					RetryHint: edge.RetryHint(response.RetryHint),
				}
				reason := fmt.Sprintf("received error from controller: %s", response.Msg)
				self.ensureSdkCloseSent(terminator, reason, edgeErr)
				terminator.close(self, false, reason)
			}
		} else {
			retryHint := edge.RetryHint(response.GetRetryHint())
			edgeErr := &EdgeError{
				Message:   response.Msg,
				Code:      response.ErrorCode,
				RetryHint: retryHint,
			}
			reason := response.Msg
			self.ensureSdkCloseSent(terminator, reason, edgeErr)
			terminator.close(self, false, reason)
		}

		return
	}

	terminator.failureCount.Store(0)
	self.markEstablished(terminator, "create notification received")

	// notify the sdk that the terminator was established
	terminator.NotifyEstablished(response.Result)

	// post-create inspect is queued by markEstablished -> markEstablishedEvent.handle
}

func (self *hostedServiceRegistry) HandleReconnect() {
	var reestablishList []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.updateState(xgress_common.TerminatorStateEstablished, xgress_common.TerminatorStateEstablishing, "reconnecting") {
			reestablishList = append(reestablishList, terminator)
		}
	})

	// wait for verify terminator events to come in
	time.Sleep(10 * time.Second)

	for _, terminator := range reestablishList {
		if terminator.state.Load() == xgress_common.TerminatorStateEstablishing {
			self.queueEstablishTerminatorAsync(terminator)
		}
	}
}

func (self *hostedServiceRegistry) Inspect(timeout time.Duration) *inspect.SdkTerminatorInspectResult {
	evt := &inspectTerminatorsEvent{
		result: atomic.Pointer[[]*inspect.SdkTerminatorInspectDetail]{},
		done:   make(chan struct{}),
	}

	// if we can't queue, grab the results in a non-thread-safe fashion
	if err := self.queueWithTimeout(evt, timeout); err != nil {
		evt.handle(self)
	}

	result := &inspect.SdkTerminatorInspectResult{}

	var err error
	result.Entries, err = evt.GetResults(timeout)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}

func (self *hostedServiceRegistry) checkForExistingListenerId(terminator *edgeTerminator) (listenerIdCheckResult, error) {
	defaultResult := listenerIdCheckResult{
		replaceExisting: false,
		terminator:      terminator,
	}

	if terminator.listenerId == "" {
		self.Put(terminator)
		return defaultResult, nil
	}

	event := &findMatchingEvent{
		terminator: terminator,
		resultC:    make(chan listenerIdCheckResult, 1),
	}
	self.queue(event)

	select {
	case result := <-event.resultC:
		return result, nil
	case <-self.env.GetCloseNotify():
		return defaultResult, errors.New("registry stopped")
	case <-time.After(100 * time.Millisecond):

		// if processing has already started, we need to wait for it to finish
		// otherwise, we can return here
		if event.cancelGate.CompareAndSwap(false, true) {
			pfxlog.Logger().WithField("terminatorId", terminator.terminatorId).
				WithField("listenerId", terminator.listenerId).
				Info("unable to check for existing terminators with matching listenerId")

			self.Put(terminator)
			return defaultResult, nil
		}

		select {
		case result := <-event.resultC:
			return result, nil
		case <-self.env.GetCloseNotify():
			return defaultResult, errors.New("registry stopped")
		}
	}
}

func (self *hostedServiceRegistry) handleSdkReturnedInvalid(terminator *edgeTerminator) invalidTerminatorRemoveResult {
	event := &sdkTerminatorInvalidEvent{
		terminator: terminator,
		resultC:    make(chan invalidTerminatorRemoveResult, 1),
	}
	self.queue(event)

	select {
	case result := <-event.resultC:
		return result
	case <-time.After(250 * time.Millisecond): // Failsafe in case the hosting registry event loop gets too busy
		//                                     // If we bail out, the notify process will be retried
		if !self.terminators.Has(terminator.terminatorId) {
			return invalidTerminatorRemoveResult{
				existed: false,
				removed: false,
			}
		}

		return invalidTerminatorRemoveResult{
			err: errors.New("operation timed out"),
		}
	}
}

func (self *hostedServiceRegistry) getTerminatorsForService(serviceId string) []*edgeTerminator {
	var result []*edgeTerminator
	self.terminators.IterCb(func(key string, v *edgeTerminator) {
		if v.serviceSessionToken.ServiceId == serviceId {
			result = append(result, v)
		}
	})
	return result
}

type inspectTerminatorsEvent struct {
	result atomic.Pointer[[]*inspect.SdkTerminatorInspectDetail]
	done   chan struct{}
}

func (self *inspectTerminatorsEvent) handle(registry *hostedServiceRegistry) {
	var result []*inspect.SdkTerminatorInspectDetail
	registry.terminators.IterCb(func(key string, terminator *edgeTerminator) {
		detail := &inspect.SdkTerminatorInspectDetail{
			Key:             key,
			Id:              terminator.terminatorId,
			State:           terminator.state.Load().String(),
			Token:           terminator.serviceSessionToken.Token(),
			ListenerId:      terminator.listenerId,
			Instance:        terminator.instance,
			Cost:            terminator.cost,
			Precedence:      terminator.precedence.String(),
			AssignIds:       terminator.assignIds,
			UseSdkXgress:    terminator.useSdkXgress,
			SupportsInspect: terminator.supportsInspect,
			OperationActive: terminator.operationActive.Load(),
			CreateTime:      terminator.createTime.Format("2006-01-02 15:04:05"),
			LastAttempt:     terminator.lastAttempt.Format("2006-01-02 15:04:05"),
		}
		result = append(result, detail)
	})

	self.result.Store(&result)
	close(self.done)
}

func (self *inspectTerminatorsEvent) GetResults(timeout time.Duration) ([]*inspect.SdkTerminatorInspectDetail, error) {
	select {
	case <-self.done:
		return *self.result.Load(), nil
	case <-time.After(timeout):
		return nil, errors.New("timed out waiting for result")
	}
}

type channelClosedEvent struct {
	ch channel.Channel
}

func (self *channelClosedEvent) handle(registry *hostedServiceRegistry) {
	registry.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.MsgChannel.GetChannel() == self.ch {
			// we're iterating the map right now, so the terminator can't have changed
			registry.queueRemoveTerminatorUnchecked(terminator, "channel closed")
		}
	})
}

type listenerIdCheckResult struct {
	replaceExisting bool
	terminator      *edgeTerminator
	previous        *edgeTerminator
}

type findMatchingEvent struct {
	terminator *edgeTerminator
	resultC    chan listenerIdCheckResult
	cancelGate atomic.Bool
}

func (self *findMatchingEvent) handle(registry *hostedServiceRegistry) {
	if !self.cancelGate.CompareAndSwap(false, true) {
		return
	}

	// NOTE: We need to store the terminator in the map before we exit. If we do it later,
	// another process for the same listener id might be in this code, and then we've got
	// a race condition for which will end up in the map

	var existingList []*edgeTerminator
	registry.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.listenerId == self.terminator.listenerId &&
			terminator.getIdentityId() == self.terminator.getIdentityId() {
			existingList = append(existingList, terminator)
		}
	})

	if len(existingList) == 0 {
		registry.Put(self.terminator)
		self.resultC <- listenerIdCheckResult{
			terminator:      self.terminator,
			replaceExisting: false,
		}
		return
	}

	log := pfxlog.ContextLogger(self.terminator.edgeClientConn.ch.GetChannel().Label()).
		WithField("token", self.terminator.serviceSessionToken).
		WithField("routerId", self.terminator.edgeClientConn.listener.id.Token).
		WithField("listenerId", self.terminator.listenerId).
		WithField("connId", self.terminator.MsgChannel.Id()).
		WithField("terminatorId", self.terminator.terminatorId)

	for _, existing := range existingList {
		matches := self.terminator.edgeClientConn == existing.edgeClientConn &&
			self.terminator.serviceSessionToken == existing.serviceSessionToken &&
			self.terminator.MsgChannel.Id() == existing.MsgChannel.Id() &&
			existing.state.Load() != xgress_common.TerminatorStateDeleting

		if matches {
			log = log.WithField("existingTerminatorId", existing.terminatorId)
			log.Info("duplicate bind request")
			self.resultC <- listenerIdCheckResult{
				terminator:      self.terminator,
				replaceExisting: true,
				previous:        existing,
			}
			return
		}
	}

	for _, existing := range existingList {
		if !existing.IsDeleting() {
			// If this bind is on the same connection as the existing one but has a lower connId,
			// it's a stale bind that was reordered. The existing bind is actually newer.
			// Discard the stale bind to avoid replacing the valid existing terminator.
			if self.terminator.edgeClientConn == existing.edgeClientConn &&
				self.terminator.MsgChannel.Id() < existing.MsgChannel.Id() {
				log = log.WithField("existingTerminatorId", existing.terminatorId).
					WithField("existingConnId", existing.MsgChannel.Id())
				log.Info("discarding stale bind, existing bind has newer connId")
				self.terminator.setState(xgress_common.TerminatorStateDeleting, "stale bind, existing bind is newer")
				self.resultC <- listenerIdCheckResult{
					terminator:      self.terminator,
					replaceExisting: false,
				}
				return
			}

			self.terminator.replace(existing)
			registry.Put(self.terminator)

			// sometimes things happen close together. we need to try to notify replaced terminators
			// that they're being closed in case they're the newer, still open connection
			reason := "found a newer terminator for listener id"
			edgeErr := &EdgeError{
				Message:   "newer terminator found for listener id",
				Code:      edge.ErrorCodeInternal,
				RetryHint: edge.RetryDefault,
			}
			registry.ensureSdkCloseSentInner(existing, reason, edgeErr, true)
			existing.close(registry, false, reason)
			existing.setState(xgress_common.TerminatorStateDeleting, "newer terminator found for listener id")

			log = log.WithField("existingTerminatorId", existing.terminatorId)
			log.Info("taking over terminator from existing bind")

			self.resultC <- listenerIdCheckResult{
				terminator:      self.terminator,
				replaceExisting: true,
				previous:        existing,
			}
			return
		}
	}

	// can't reuse terminator ID, if the existing terminator is being deleted
	log.Info("existing terminator being deleted, need to establish new terminator")
	registry.Put(self.terminator)
	self.resultC <- listenerIdCheckResult{
		terminator:      self.terminator,
		replaceExisting: false,
		previous:        existingList[0],
	}
}

type invalidTerminatorRemoveResult struct {
	err     error
	existed bool
	removed bool
}

type sdkTerminatorInvalidEvent struct {
	terminator *edgeTerminator
	resultC    chan invalidTerminatorRemoveResult
}

func (self *sdkTerminatorInvalidEvent) handle(registry *hostedServiceRegistry) {
	self.resultC <- self.removeTerminator(registry)
}

func (self *sdkTerminatorInvalidEvent) removeTerminator(registry *hostedServiceRegistry) invalidTerminatorRemoveResult {
	existing, _ := registry.terminators.Get(self.terminator.terminatorId)

	if existing == nil {
		return invalidTerminatorRemoveResult{
			existed: false,
			removed: false,
		}
	}

	if existing != self.terminator {
		return invalidTerminatorRemoveResult{
			existed: true,
			removed: false,
		}
	}

	existed := false
	removed := registry.terminators.RemoveCb(self.terminator.terminatorId, func(key string, v *edgeTerminator, exists bool) bool {
		existed = exists
		return v == self.terminator
	})
	if removed {
		pfxlog.Logger().WithField("terminatorId", self.terminator.terminatorId).
			WithField("reason", "query to sdk indicated terminator is invalid").
			Info("terminator removed from router set")
	}

	return invalidTerminatorRemoveResult{
		existed: existed,
		removed: removed,
	}
}

type markEstablishedEvent struct {
	terminator *edgeTerminator
	reason     string
}

func (self *markEstablishedEvent) handle(registry *hostedServiceRegistry) {
	log := pfxlog.Logger().
		WithField("routerId", registry.env.GetRouterId().Token).
		WithField("terminatorId", self.terminator.terminatorId).
		WithField("lifetime", time.Since(self.terminator.createTime)).
		WithField("connId", self.terminator.MsgChannel.Id())

	if rateLimitCallback := self.terminator.GetAndClearRateLimitCallback(); rateLimitCallback != nil {
		rateLimitCallback.Success()
	}

	if !self.terminator.updateState(xgress_common.TerminatorStateEstablishing, xgress_common.TerminatorStateEstablished, self.reason) {
		log.Info("received additional terminator created notification")
	} else {
		log.Info("terminator established")
		// If establishment took a long time, the SDK may have timed out waiting for BindSuccess
		// and closed the listener. The initial post-create inspect (sent right after bind) would
		// have confirmed validity before the timeout. Re-inspect to catch this case.
		if self.terminator.supportsInspect && time.Since(self.terminator.createTime) > 30*time.Second {
			log.Info("establishment took >30s, queuing post-establish inspect")
			registry.postCreateInspectSet[self.terminator.terminatorId] = &pendingPostCreateInspect{
				terminator:  self.terminator,
				requestSeqs: map[int32]struct{}{},
			}
		}
	}

	self.terminator.operationActive.Store(false)
}
