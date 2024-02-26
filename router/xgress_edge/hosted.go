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
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
	"time"
)

func newHostedServicesRegistry(env routerEnv.RouterEnv) *hostedServiceRegistry {
	result := &hostedServiceRegistry{
		terminators:  cmap.New[*edgeTerminator](),
		events:       make(chan terminatorEvent),
		env:          env,
		triggerEvalC: make(chan struct{}, 1),
	}
	go result.run()
	return result
}

type hostedServiceRegistry struct {
	terminators     cmap.ConcurrentMap[string, *edgeTerminator]
	events          chan terminatorEvent
	env             routerEnv.RouterEnv
	establishQueue  []*edgeTerminator
	deleteQueue     []*edgeTerminator
	outstandingReqs uint32
	maxOutstanding  uint32
	triggerEvalC    chan struct{}
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

		if !self.env.GetCtrlRateLimiter().IsRateLimited() {
			self.evaluateEstablishQueue()
		}

		if !self.env.GetCtrlRateLimiter().IsRateLimited() {
			self.evaluateDeleteQueue()
		}
	}
}

func (self *hostedServiceRegistry) trigerEvaluates() {
	select {
	case self.triggerEvalC <- struct{}{}:
	default:
	}
}

func (self *hostedServiceRegistry) evaluateEstablishQueue() {
	for len(self.establishQueue) > 0 {
		terminator := self.establishQueue[0]

		dequeue := func() {
			self.establishQueue = self.establishQueue[1:]
		}

		log := logrus.
			WithField("terminatorId", terminator.terminatorId.Load()).
			WithField("state", terminator.state.Load()).
			WithField("token", terminator.token)

		if terminator.edgeClientConn.ch.IsClosed() {
			log.Info("terminator sdk channel closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.terminatorId.Load() == "" {
			log.Info("terminator has been closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.state.Load() != TerminatorStateEstablishing {
			dequeue()
			continue
		}

		rateLimitCtrl, err := self.env.GetCtrlRateLimiter().RunRateLimited()
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
		terminator.SetRatLimitCallback(rateLimitCtrl)
		terminator.lastAttempt = time.Now()

		err = self.env.GetRateLimiterPool().QueueOrError(func() {
			if err := self.establishTerminator(terminator); err != nil {
				log.WithError(err).Error("error establishing terminator")
				self.queueEstablishTerminatorAsync(terminator)
				rateLimitCtrl.Failed()
			}
		})

		if err != nil {
			rateLimitCtrl.Failed()
			log.Info("rate limited: unable to queue to establish")
			self.queueEstablishTerminatorSync(terminator)
		}
	}
}

func (self *hostedServiceRegistry) dequeueNextDeleted() *edgeTerminator {
	if len(self.deleteQueue) == 0 {
		return nil
	}
	terminator := self.deleteQueue[0]
	self.deleteQueue = self.deleteQueue[1:]
	return terminator
}

func (self *hostedServiceRegistry) evaluateDeleteQueue() {
	var deleteList []*edgeTerminator

	for {
		terminator := self.dequeueNextDeleted()
		if terminator == nil {
			break
		}

		log := logrus.
			WithField("terminatorId", terminator.terminatorId.Load()).
			WithField("state", terminator.state.Load()).
			WithField("token", terminator.token)

		if terminator.state.Load() != TerminatorStateDeleting {
			continue
		}

		if !terminator.operationActive.Load() || time.Since(terminator.lastAttempt) > 30*time.Second {
			log.Info("added terminator to batch delete")
			deleteList = append(deleteList, terminator)
			if len(deleteList) >= 50 {
				self.RemoveTerminatorsRateLimited(deleteList)
				deleteList = nil
			}
		}
	}

	if len(deleteList) != 0 {
		self.RemoveTerminatorsRateLimited(deleteList)
	}
}

func (self *hostedServiceRegistry) RemoveTerminatorsRateLimited(terminators []*edgeTerminator) {
	rateLimitCtrl, err := self.env.GetCtrlRateLimiter().RunRateLimited()
	if err != nil {
		pfxlog.Logger().Debug("rate limiter hit, waiting for a slot to open before doing sdk terminator deletes")
	}

	if err == nil {
		for _, terminator := range terminators {
			terminator.operationActive.Store(true)
		}

		err = self.env.GetRateLimiterPool().QueueOrError(func() {
			var terminatorIds []string
			for _, terminator := range terminators {
				terminatorIds = append(terminatorIds, terminator.terminatorId.Load())
			}

			if err := self.RemoveTerminators(terminatorIds); err != nil {
				rateLimitCtrl.Backoff()
				for _, terminator := range terminators {
					self.queueRemoveTerminatorAsync(terminator)
				}
			} else {
				rateLimitCtrl.Success()
				for _, terminator := range terminators {
					terminator.operationActive.Store(false)
					self.terminators.Remove(terminator.terminatorId.Load())
				}
			}
		})

		if err != nil {
			rateLimitCtrl.Failed()
		}
	}

	if err != nil {
		for _, terminator := range terminators {
			self.queueRemoveTerminatorSync(terminator)
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

	if responseMsg != nil && responseMsg.ContentType == channel.ContentTypeResultType {
		result := channel.UnmarshalResult(responseMsg)
		if result.Success {
			log.Debug("successfully removed terminators")
		} else {
			log.Errorf("failure removing terminators (%v)", result.Message)
		}
	} else {
		log.Errorf("unexpected controller response, ContentType [%v]", responseMsg.ContentType)
	}
	return nil
}

type queueEstablishTerminator struct {
	terminator *edgeTerminator
}

func (self *queueEstablishTerminator) handle(registry *hostedServiceRegistry) {
	registry.queueEstablishTerminatorSync(self.terminator)
}

type queueRemoveTerminator struct {
	terminator *edgeTerminator
}

func (self *queueRemoveTerminator) handle(registry *hostedServiceRegistry) {
	registry.queueRemoveTerminatorSync(self.terminator)
}

func (self *hostedServiceRegistry) EstablishTerminator(terminator *edgeTerminator) {
	self.Put(terminator)
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
		self.establishQueue = append(self.establishQueue, terminator)
	}
}

func (self *hostedServiceRegistry) queueEstablishTerminatorAsync(terminator *edgeTerminator) {
	self.queue(&queueEstablishTerminator{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) queueRemoveTerminatorAsync(terminator *edgeTerminator) {
	self.queue(&queueRemoveTerminator{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) queueRemoveTerminatorSync(terminator *edgeTerminator) {
	terminator.state.Store(TerminatorStateDeleting)
	self.deleteQueue = append(self.deleteQueue, terminator)
}

func (self *hostedServiceRegistry) scanForRetries() {
	for entry := range self.terminators.IterBuffered() {
		terminator := entry.Val
		state := terminator.state.Load()
		if state.IsWorkRequired() && time.Since(terminator.lastAttempt) > 2*time.Minute {
			if state == TerminatorStateEstablishing {
				self.queueEstablishTerminatorSync(terminator)
			} else {
				self.queueRemoveTerminatorSync(terminator)
			}
		}
	}
}

func (self *hostedServiceRegistry) PutV1(token string, terminator *edgeTerminator) {
	self.terminators.Set(token, terminator)
}

func (self *hostedServiceRegistry) Put(terminator *edgeTerminator) {
	self.terminators.Set(terminator.terminatorId.Load(), terminator)
}

func (self *hostedServiceRegistry) Get(terminatorId string) (*edgeTerminator, bool) {
	return self.terminators.Get(terminatorId)
}

func (self *hostedServiceRegistry) Delete(terminatorId string) {
	self.terminators.Remove(terminatorId)
}

func (self *hostedServiceRegistry) cleanupServices(ch channel.Channel) {
	self.queue(&channelClosedEvent{
		ch: ch,
	})
}

func (self *hostedServiceRegistry) cleanupDuplicates(newest *edgeTerminator) {
	for entry := range self.terminators.IterBuffered() {
		terminator := entry.Val
		if terminator != newest && newest.token == terminator.token && newest.instance == terminator.instance {
			terminator.close(self, false, true, "duplicate terminator") // don't notify, channel is already closed, we can't send messages
			pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
				WithField("token", terminator.token).
				WithField("instance", terminator.instance).
				WithField("terminatorId", terminator.terminatorId.Load()).
				WithField("duplicateOf", newest.terminatorId.Load()).
				Info("duplicate removed")
		}
	}
}

func (self *hostedServiceRegistry) unbindSession(connId uint32, sessionToken string, proxy *edgeClientConn) bool {
	atLeastOneRemoved := false
	for entry := range self.terminators.IterBuffered() {
		terminator := entry.Val
		if terminator.MsgChannel.Id() == connId && terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			terminator.close(self, false, true, "unbind successful") // don't notify, sdk asked us to unbind
			pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
				WithField("token", sessionToken).
				WithField("terminatorId", terminator.terminatorId.Load()).
				Info("terminator removed")
			atLeastOneRemoved = true
		}
	}
	return atLeastOneRemoved
}

func (self *hostedServiceRegistry) getRelatedTerminators(sessionToken string, proxy *edgeClientConn) []*edgeTerminator {
	var result []*edgeTerminator
	for entry := range self.terminators.IterBuffered() {
		terminator := entry.Val
		if terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			result = append(result, terminator)
		}
	}
	return result
}

func (self *hostedServiceRegistry) establishTerminator(terminator *edgeTerminator) error {
	factory := terminator.edgeClientConn.listener.factory

	log := pfxlog.Logger().
		WithField("routerId", factory.env.GetRouterId().Token).
		WithField("terminatorId", terminator.terminatorId.Load()).
		WithField("token", terminator.token)

	terminatorId := terminator.terminatorId.Load()
	if terminatorId == "" {
		return fmt.Errorf("edge link is closed, stopping terminator creation for terminator %s", terminatorId)
	}

	request := &edge_ctrl_pb.CreateTerminatorV2Request{
		Address:        terminatorId,
		SessionToken:   terminator.token,
		Fingerprints:   terminator.edgeClientConn.fingerprints.Prints(),
		PeerData:       terminator.hostData,
		Cost:           uint32(terminator.cost),
		Precedence:     terminator.precedence,
		InstanceId:     terminator.instance,
		InstanceSecret: terminator.instanceSecret,
	}

	timeout := factory.ctrls.DefaultRequestTimeout()
	ctrlCh := factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log.Info("sending create terminator v2 request")

	return protobufs.MarshalTyped(request).WithTimeout(timeout).SendAndWaitForWire(ctrlCh)
}

func (self *hostedServiceRegistry) HandleCreateTerminatorResponse(msg *channel.Message, _ channel.Channel) {
	defer self.trigerEvaluates()

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

	log = log.WithField("lifetime", time.Since(terminator.createTime))

	rateLimitCallback := terminator.GetAndClearRateLimitCallback()

	if response.Result == edge_ctrl_pb.CreateTerminatorResult_FailedBusy {
		log.Info("controller too busy to handle create terminator, retrying later")
		if rateLimitCallback != nil {
			rateLimitCallback.Backoff()
		}
		self.queueEstablishTerminatorAsync(terminator)
		return
	}

	if rateLimitCallback != nil {
		rateLimitCallback.Success()
	}

	if response.Result != edge_ctrl_pb.CreateTerminatorResult_Success {
		terminator.operationActive.Store(false)
		if terminator.establishCallback != nil {
			terminator.establishCallback(response.Result)
		}
		terminator.close(self, true, false, response.Msg)
		return
	}

	if terminator.state.CompareAndSwap(TerminatorStateEstablishing, TerminatorStateEstablished) {
		log.Info("received terminator created notification")
	} else {
		log.Info("received additional terminator created notification")
	}

	terminator.operationActive.Store(false)

	isValid := true
	if terminator.postValidate {
		if result, err := terminator.inspect(self, true); err != nil {
			log.WithError(err).Error("error validating terminator after create")
		} else if result.Type != edge.ConnTypeBind {
			log.WithError(err).Error("terminator invalid in sdk after create, closed")
			isValid = false
		} else {
			log.Info("terminator validated successfully")
		}
	}

	if isValid {
		terminator.establishCallback(response.Result)
	}
}

func (self *hostedServiceRegistry) HandleReconnect() {
	var restablishList []*edgeTerminator
	for entry := range self.terminators.IterBuffered() {
		terminator := entry.Val
		if terminator.state.CompareAndSwap(TerminatorStateEstablished, TerminatorStateEstablishing) {
			restablishList = append(restablishList, terminator)
		}
	}

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

func (self *hostedServiceRegistry) checkForExistingListenerId(terminator *edgeTerminator) listenerIdCheckResult {
	if terminator.listenerId == "" {
		return listenerIdCheckResult{
			replaceExisting: false,
		}
	}

	event := &findMatchingEvent{
		terminator: terminator,
		resultC:    make(chan listenerIdCheckResult, 1),
	}
	self.queue(event)

	select {
	case result := <-event.resultC:
		return result
	case <-time.After(10 * time.Millisecond):
		return listenerIdCheckResult{
			replaceExisting: false,
		}
	}
}

type inspectTerminatorsEvent struct {
	result atomic.Pointer[[]*inspect.SdkTerminatorInspectDetail]
	done   chan struct{}
}

func (self *inspectTerminatorsEvent) handle(registry *hostedServiceRegistry) {
	var result []*inspect.SdkTerminatorInspectDetail
	for entry := range registry.terminators.IterBuffered() {
		id := entry.Key
		terminator := entry.Val

		detail := &inspect.SdkTerminatorInspectDetail{
			Id:              id,
			State:           terminator.state.Load().String(),
			Token:           terminator.token,
			ListenerId:      terminator.listenerId,
			Instance:        terminator.instance,
			Cost:            terminator.cost,
			Precedence:      terminator.precedence.String(),
			AssignIds:       terminator.assignIds,
			V2:              terminator.v2,
			PostValidate:    terminator.postValidate,
			OperationActive: terminator.operationActive.Load(),
			CreateTime:      terminator.createTime.Format("2006-01-02 15:04:05"),
			LastAttempt:     terminator.lastAttempt.Format("2006-01-02 15:04:05"),
		}
		result = append(result, detail)
	}

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
	for entry := range registry.terminators.IterBuffered() {
		terminator := entry.Val
		if terminator.MsgChannel.Channel == self.ch {
			if terminator.v2 {
				terminator.state.Store(TerminatorStateDeleting)
				registry.deleteQueue = append(registry.deleteQueue, terminator)
			} else {
				// don't notify, channel is already closed, we can't send messages
				terminator.close(registry, false, true, "channel closed")
			}
		}
	}
}

type listenerIdCheckResult struct {
	replaceExisting bool
	previous        *edgeTerminator
}

type findMatchingEvent struct {
	terminator *edgeTerminator
	resultC    chan listenerIdCheckResult
}

func (self *findMatchingEvent) handle(registry *hostedServiceRegistry) {
	var existingList []*edgeTerminator
	for entry := range registry.terminators.IterBuffered() {
		terminator := entry.Val
		if terminator.v2 && terminator.listenerId == self.terminator.listenerId {
			existingList = append(existingList, terminator)
		}
	}

	if len(existingList) == 0 {
		self.resultC <- listenerIdCheckResult{replaceExisting: false}
		return
	}

	log := pfxlog.ContextLogger(self.terminator.edgeClientConn.ch.Label()).
		WithField("token", self.terminator.token).
		WithField("routerId", self.terminator.edgeClientConn.listener.id.Token).
		WithField("listenerId", self.terminator.listenerId)

	for _, existing := range existingList {
		matches := self.terminator.edgeClientConn == existing.edgeClientConn &&
			self.terminator.token == existing.token &&
			existing.state.Load() != TerminatorStateDeleting

		if matches {
			log = log.WithField("terminatorId", existing.terminatorId.Load())
			log.Info("duplicate bind request")
			self.resultC <- listenerIdCheckResult{
				replaceExisting: true,
				previous:        existing,
			}
			return
		}
	}

	for _, existing := range existingList {
		if !existing.IsDeleting() {
			self.terminator.terminatorId.Store(existing.terminatorId.Load())
			self.terminator.state.Store(existing.state.Load())
			registry.Put(self.terminator)
			existing.close(registry, true, false, "terminator replaced")

			log = log.WithField("terminatorId", existing.terminatorId.Load())
			log.Info("taking over bind from existing bind")

			self.resultC <- listenerIdCheckResult{
				replaceExisting: true,
				previous:        existing,
			}
			return
		}
	}

	// can't re-use terminator ID, if the existing terminator is being deleted
	log = log.WithField("terminatorId", self.terminator.terminatorId.Load())
	log.Info("existing terminator being deleted, need to establish new terminator")
	self.resultC <- listenerIdCheckResult{
		replaceExisting: false,
		previous:        existingList[0],
	}
}
