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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/ziti/common/handler_common"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/command"
	routerEnv "github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress_common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
	"time"
)

func newHostedServicesRegistry(env routerEnv.RouterEnv, stateManager state.Manager) *hostedServiceRegistry {
	result := &hostedServiceRegistry{
		terminators:  cmap.New[*edgeTerminator](),
		events:       make(chan terminatorEvent),
		env:          env,
		stateManager: stateManager,
		triggerEvalC: make(chan struct{}, 1),
		establishSet: map[string]*edgeTerminator{},
		deleteSet:    map[string]*edgeTerminator{},
	}
	go result.run()
	return result
}

type hostedServiceRegistry struct {
	terminators  cmap.ConcurrentMap[string, *edgeTerminator]
	events       chan terminatorEvent
	env          routerEnv.RouterEnv
	stateManager state.Manager
	establishSet map[string]*edgeTerminator
	deleteSet    map[string]*edgeTerminator
	triggerEvalC chan struct{}
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
			WithField("token", terminator.token)

		if terminator.edgeClientConn.ch.IsClosed() {
			self.Remove(terminator, "sdk connection is closed")
			log.Infof("terminator sdk channel closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.terminatorId == "" {
			log.Info("terminator has been closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.state.Load() != TerminatorStateEstablishing {
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
			WithField("token", terminator.token)

		delete(self.deleteSet, terminatorId)

		if terminator.state.Load() != TerminatorStateDeleting {
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
					pfxlog.Logger().WithField("terminatorId", terminator.terminatorId).
						Error("terminator was replaced after being put into deleting state?!")
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
			terminator.setState(TerminatorStateDeleting, "deleting")
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
	if existing == nil || existing == terminator && terminator.state.Load() == TerminatorStateDeleting { // make sure we're still the current terminator
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
	terminator.setState(TerminatorStateDeleting, reason)
	terminator.operationActive.Store(false)
	self.deleteSet[terminator.terminatorId] = terminator
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
		state := terminator.state.Load()
		if state.IsWorkRequired() && time.Since(terminator.lastAttempt) > 2*time.Minute {
			retryList = append(retryList, terminator)
		}
	})

	for _, terminator := range retryList {
		state := terminator.state.Load()
		if state == TerminatorStateEstablishing {
			self.queueEstablishTerminatorSync(terminator)
		} else if state == TerminatorStateDeleting {
			self.requeueRemoveTerminatorSync(terminator)
		}
	}
}

func (self *hostedServiceRegistry) PutV1(token string, terminator *edgeTerminator) {
	self.terminators.Set(token, terminator)
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
		if terminator != newest && newest.token == terminator.token && newest.instance == terminator.instance {
			toClose = append(toClose, terminator)
		}
	})

	for _, terminator := range toClose {
		terminator.close(self, false, true, "duplicate terminator") // don't notify, channel is already closed, we can't send messages
		pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
			WithField("token", terminator.token).
			WithField("instance", terminator.instance).
			WithField("terminatorId", terminator.terminatorId).
			WithField("duplicateOf", newest.terminatorId).
			Info("duplicate removed")
	}
}

func (self *hostedServiceRegistry) unbindSession(connId uint32, sessionToken string, proxy *edgeClientConn) bool {
	var toClose []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.MsgChannel.Id() == connId && terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			toClose = append(toClose, terminator)
		}
	})

	atLeastOneRemoved := false
	for _, terminator := range toClose {
		terminator.close(self, false, true, "unbind successful") // don't notify, sdk asked us to unbind
		pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
			WithField("token", sessionToken).
			WithField("connId", connId).
			WithField("terminatorId", terminator.terminatorId).
			Info("terminator removed")
		atLeastOneRemoved = true
	}
	return atLeastOneRemoved
}

func (self *hostedServiceRegistry) getRelatedTerminators(connId uint32, sessionToken string, proxy *edgeClientConn) []*edgeTerminator {
	var related []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.MsgChannel.Id() == connId && terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			related = append(related, terminator)
		}
	})

	return related
}

func (self *hostedServiceRegistry) establishTerminator(terminator *edgeTerminator) error {
	factory := terminator.edgeClientConn.listener.factory

	log := pfxlog.Logger().
		WithField("routerId", factory.env.GetRouterId().Token).
		WithField("terminatorId", terminator.terminatorId).
		WithField("token", terminator.token)

	request := &edge_ctrl_pb.CreateTerminatorV2Request{
		Address:        terminator.terminatorId,
		SessionToken:   terminator.token,
		Fingerprints:   terminator.edgeClientConn.fingerprints.Prints(),
		PeerData:       terminator.hostData,
		Cost:           uint32(terminator.cost),
		Precedence:     terminator.precedence,
		InstanceId:     terminator.instance,
		InstanceSecret: terminator.instanceSecret,
	}

	if xgress_common.IsBearerToken(request.SessionToken) {
		apiSession := self.stateManager.GetApiSessionFromCh(terminator.Channel)

		if apiSession == nil {
			return errors.New("could not find api session for channel, unable to process bind message")
		}

		request.ApiSessionToken = apiSession.Token
	}

	ctrlCh := terminator.edgeClientConn.apiSession.SelectModelUpdateCtrlCh(factory.ctrls)

	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log.Info("sending create terminator v2 request")

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
		if terminator.establishCallback != nil {
			terminator.establishCallback(response.Result)
		}
		terminator.close(self, true, false, response.Msg)
		return
	}

	self.markEstablished(terminator, "create notification received")

	// notify the sdk that the terminator was established
	terminator.establishCallback(response.Result)

	// we don't need to call inspect here, as the controller will be doing a post-create check
	// to verify that everything is still in place
}

func (self *hostedServiceRegistry) HandleReconnect() {
	var restablishList []*edgeTerminator
	self.terminators.IterCb(func(_ string, terminator *edgeTerminator) {
		if terminator.updateState(TerminatorStateEstablished, TerminatorStateEstablishing, "reconnecting") {
			restablishList = append(restablishList, terminator)
		}
	})

	// wait for verify terminator events to come in
	time.Sleep(10 * time.Second)

	for _, terminator := range restablishList {
		if terminator.state.Load() == TerminatorStateEstablishing {
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

func (self *hostedServiceRegistry) handleSdkReturnedInvalid(terminator *edgeTerminator, notifyCtrl bool) invalidTerminatorRemoveResult {
	event := &sdkTerminatorInvalidEvent{
		terminator: terminator,
		resultC:    make(chan invalidTerminatorRemoveResult, 1),
		notifyCtrl: notifyCtrl,
	}
	self.queue(event)

	select {
	case result := <-event.resultC:
		return result
	case <-time.After(100 * time.Millisecond):
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
			Token:           terminator.token,
			ListenerId:      terminator.listenerId,
			Instance:        terminator.instance,
			Cost:            terminator.cost,
			Precedence:      terminator.precedence.String(),
			AssignIds:       terminator.assignIds,
			V2:              terminator.v2,
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
		if terminator.MsgChannel.Channel == self.ch {
			if terminator.v2 {
				// we're iterating the map right now, so the terminator can't have changed
				registry.queueRemoveTerminatorUnchecked(terminator, "channel closed")
			} else {
				// don't notify, channel is already closed, we can't send messages
				go terminator.close(registry, false, true, "channel closed")
			}
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
		if terminator.v2 && terminator.listenerId == self.terminator.listenerId {
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

	log := pfxlog.ContextLogger(self.terminator.edgeClientConn.ch.Label()).
		WithField("token", self.terminator.token).
		WithField("routerId", self.terminator.edgeClientConn.listener.id.Token).
		WithField("listenerId", self.terminator.listenerId).
		WithField("connId", self.terminator.MsgChannel.Id()).
		WithField("terminatorId", self.terminator.terminatorId)

	for _, existing := range existingList {
		matches := self.terminator.edgeClientConn == existing.edgeClientConn &&
			self.terminator.token == existing.token &&
			self.terminator.MsgChannel.Id() == existing.MsgChannel.Id() &&
			existing.state.Load() != TerminatorStateDeleting

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
			self.terminator.replace(existing)
			registry.Put(self.terminator)

			// sometimes things happen close together. we need to try to notify replaced terminators
			// that they're being closed in case they're the newer, still open connection
			existing.close(registry, true, false, "found a newer terminator for listener id")
			existing.setState(TerminatorStateDeleting, "newer terminator found for listener id")

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
	notifyCtrl bool
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

	if self.notifyCtrl {
		registry.queueRemoveTerminatorSync(self.terminator, "query to sdk indicated terminator is invalid")
		return invalidTerminatorRemoveResult{
			existed: true,
			removed: true,
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

	if !self.terminator.updateState(TerminatorStateEstablishing, TerminatorStateEstablished, self.reason) {
		log.Info("received additional terminator created notification")
	} else {
		log.Info("terminator established")
	}

	self.terminator.operationActive.Store(false)
}
