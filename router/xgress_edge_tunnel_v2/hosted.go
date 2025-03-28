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

package xgress_edge_tunnel_v2

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/sdk-golang/ziti"
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
		terminators:  cmap.New[*tunnelTerminator](),
		events:       make(chan terminatorEvent),
		env:          env,
		stateManager: stateManager,
		triggerEvalC: make(chan struct{}, 1),
		establishSet: map[string]*tunnelTerminator{},
		deleteSet:    map[string]*tunnelTerminator{},
	}

	return result
}

type hostedServiceRegistry struct {
	terminators  cmap.ConcurrentMap[string, *tunnelTerminator]
	events       chan terminatorEvent
	env          routerEnv.RouterEnv
	stateManager state.Manager
	establishSet map[string]*tunnelTerminator
	deleteSet    map[string]*tunnelTerminator
	triggerEvalC chan struct{}

	connectedToLeader atomic.Bool
	started           atomic.Bool
}

type terminatorEvent interface {
	handle(registry *hostedServiceRegistry)
}

func (self *hostedServiceRegistry) Start() {
	if self.started.CompareAndSwap(false, true) {
		self.env.GetNetworkControllers().AddChangeListener(routerEnv.CtrlEventListenerFunc(self.NotifyOfCtrlChange))
		go self.run()
	}
}

func (self *hostedServiceRegistry) run() {
	terminatorIdCacheTicker := time.NewTicker(4 * time.Hour)
	defer terminatorIdCacheTicker.Stop()

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
		case <-terminatorIdCacheTicker.C:
			self.pruneTerminatorIdCache()
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

func (self *hostedServiceRegistry) pruneTerminatorIdCache() {
	cache := self.env.GetRouterDataModel().GetTerminatorIdCache()

	var toRemove []string
	cache.IterCb(func(key string, v string) {
		if !self.terminators.Has(v) {
			toRemove = append(toRemove, key)
		}
	})

	for _, key := range toRemove {
		cache.Remove(key)
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
			WithField("terminatorId", terminator.id).
			WithField("state", terminator.state.Load())

		if terminator.id == "" {
			log.Info("terminator has been closed, not trying to establish")
			dequeue()
			continue
		}

		if terminator.state.Load() != xgress_common.TerminatorStateEstablishing {
			dequeue()
			continue
		}

		label := fmt.Sprintf("establish terminator %s", terminator.id)
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
	var deleteList []*tunnelTerminator

	for terminatorId, terminator := range self.deleteSet {
		log := logrus.
			WithField("terminatorId", terminator.id).
			WithField("state", terminator.state.Load())

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

func (self *hostedServiceRegistry) RemoveTerminatorsRateLimited(terminators []*tunnelTerminator) bool {
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
				pfxlog.Logger().WithError(err).WithField("terminatorId", terminator.id).
					Error("remove terminator failed")
				self.requeueRemoveTerminatorAsync(terminator)
			}
			return
		}

		var terminatorIds []string
		for _, terminator := range terminators {
			terminatorIds = append(terminatorIds, terminator.id)
		}

		if ctrlId, err := self.RemoveTerminators(terminatorIds); err != nil {
			if command.WasRateLimited(err) {
				rateLimitCtrl.Backoff()
			} else {
				rateLimitCtrl.Failed()
			}

			for _, terminator := range terminators {
				pfxlog.Logger().WithError(err).WithField("terminatorId", terminator.id).
					WithField("ctrlId", ctrlId).
					Error("remove terminator failed")
				self.requeueRemoveTerminatorAsync(terminator)
			}
		} else {
			rateLimitCtrl.Success()
			for _, terminator := range terminators {
				pfxlog.Logger().WithField("terminatorId", terminator.id).
					WithField("ctrlId", ctrlId).
					Info("remove terminator succeeded")
				terminator.operationActive.Store(false)
				if !self.Remove(terminator, "controller delete success") {
					pfxlog.Logger().WithField("terminatorId", terminator.id).
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

func (self *hostedServiceRegistry) requeueForDeleteSync(terminators []*tunnelTerminator) {
	for _, terminator := range terminators {
		existing, _ := self.terminators.Get(terminator.id)
		if existing == nil || existing == terminator { // make sure we're still the current terminator
			terminator.setState(xgress_common.TerminatorStateDeleting, "deleting")
			terminator.operationActive.Store(false)
			self.deleteSet[terminator.id] = terminator
		}
	}
}

func (self *hostedServiceRegistry) RemoveTerminators(terminatorIds []string) (string, error) {
	log := pfxlog.Logger()
	request := &ctrl_pb.RemoveTerminatorsRequest{
		TerminatorIds: terminatorIds,
	}

	ctrls := self.env.GetNetworkControllers()
	ctrlCh := ctrls.AnyValidCtrlChannel()
	if ctrlCh == nil {
		return "", errors.New("no controller available")
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(ctrls.DefaultRequestTimeout()).SendForReply(ctrlCh)
	if err != nil {
		return ctrlCh.Id(), fmt.Errorf("failed to send RemoveTerminatorsRequest message (%w)", err)
	}

	if responseMsg.ContentType != channel.ContentTypeResultType {
		return ctrlCh.Id(), fmt.Errorf("failure deleting terminators (unexpected response content type: %d)", responseMsg.ContentType)
	}

	result := channel.UnmarshalResult(responseMsg)
	if result.Success {
		return ctrlCh.Id(), nil
	}

	if handler_common.WasRateLimited(responseMsg) {
		log.Errorf("failure removing terminators (%v)", result.Message)
		return ctrlCh.Id(), apierror.NewTooManyUpdatesError()
	}

	return ctrlCh.Id(), fmt.Errorf("failure deleting terminators (%s)", result.Message)
}

type queueEstablishTerminator struct {
	terminator *tunnelTerminator
}

func (self *queueEstablishTerminator) handle(registry *hostedServiceRegistry) {
	registry.queueEstablishTerminatorSync(self.terminator)
}

type requeueRemoveTerminator struct {
	terminator *tunnelTerminator
}

func (self *requeueRemoveTerminator) handle(registry *hostedServiceRegistry) {
	registry.requeueRemoveTerminatorSync(self.terminator)
}

type queueRemoveTerminator struct {
	terminator *tunnelTerminator
	reason     string
}

func (self *queueRemoveTerminator) handle(registry *hostedServiceRegistry) {
	registry.queueRemoveTerminatorSync(self.terminator, self.reason)
}

func (self *hostedServiceRegistry) EstablishTerminator(terminator *tunnelTerminator) {
	self.terminators.Set(terminator.id, terminator)
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

func (self *hostedServiceRegistry) queueEstablishTerminatorSync(terminator *tunnelTerminator) {
	if terminator.IsEstablishing() {
		terminator.operationActive.Store(false)
		self.establishSet[terminator.id] = terminator
	}
}

func (self *hostedServiceRegistry) queueEstablishTerminatorAsync(terminator *tunnelTerminator) {
	self.queue(&queueEstablishTerminator{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) queueRemoveTerminatorAsync(terminator *tunnelTerminator, reason string) {
	self.queue(&queueRemoveTerminator{
		terminator: terminator,
		reason:     reason,
	})
}

func (self *hostedServiceRegistry) requeueRemoveTerminatorAsync(terminator *tunnelTerminator) {
	self.queue(&requeueRemoveTerminator{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) requeueRemoveTerminatorSync(terminator *tunnelTerminator) {
	existing, _ := self.terminators.Get(terminator.id)
	if existing == nil || existing == terminator && terminator.state.Load() == xgress_common.TerminatorStateDeleting { // make sure we're still the current terminator
		terminator.operationActive.Store(false)
		self.deleteSet[terminator.id] = terminator
	}
}

func (self *hostedServiceRegistry) queueRemoveTerminatorSync(terminator *tunnelTerminator, reason string) {
	existing, _ := self.terminators.Get(terminator.id)
	if existing == nil || existing == terminator { // make sure we're still the current terminator
		self.queueRemoveTerminatorUnchecked(terminator, reason)
	}
}

func (self *hostedServiceRegistry) queueRemoveTerminatorUnchecked(terminator *tunnelTerminator, reason string) {
	terminator.setState(xgress_common.TerminatorStateDeleting, reason)
	terminator.operationActive.Store(false)
	self.deleteSet[terminator.id] = terminator
}

func (self *hostedServiceRegistry) markEstablished(terminator *tunnelTerminator, reason string) {
	self.queue(&markEstablishedEvent{
		terminator: terminator,
		reason:     reason,
	})
}

func (self *hostedServiceRegistry) scanForRetries() {
	var retryList []*tunnelTerminator

	self.terminators.IterCb(func(_ string, terminator *tunnelTerminator) {
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

func (self *hostedServiceRegistry) Get(terminatorId string) (*tunnelTerminator, bool) {
	return self.terminators.Get(terminatorId)
}

func (self *hostedServiceRegistry) Remove(terminator *tunnelTerminator, reason string) bool {
	removed := self.terminators.RemoveCb(terminator.id, func(key string, v *tunnelTerminator, exists bool) bool {
		return v == terminator
	})
	if removed {
		pfxlog.Logger().WithField("terminatorId", terminator.id).
			WithField("reason", reason).
			Info("terminator removed from router set")
	}
	return removed
}

func (self *hostedServiceRegistry) establishTerminator(terminator *tunnelTerminator) error {
	start := time.Now().UnixMilli()
	log := pfxlog.Logger().
		WithField("routerId", self.env.GetRouterId().Token).
		WithField("service", terminator.context.ServiceName()).
		WithField("terminatorId", terminator.id)

	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if terminator.context.ListenOptions().Precedence == ziti.PrecedenceRequired {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Required
	} else if terminator.context.ListenOptions().Precedence == ziti.PrecedenceFailed {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
	}

	request := &edge_ctrl_pb.CreateTunnelTerminatorRequestV2{
		ServiceId:  terminator.context.ServiceId(),
		Address:    terminator.id,
		Cost:       uint32(terminator.context.ListenOptions().Cost),
		Precedence: precedence,
		InstanceId: terminator.context.ListenOptions().Identity,
		StartTime:  start,
	}

	ctrlCh := self.env.GetNetworkControllers().GetModelUpdateCtrlChannel()

	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log = log.WithField("ctrlId", ctrlCh.Id())

	log.Info("sending create tunnel terminator v2 request")

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

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponseV2{}

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
		return
	}

	if response.StartTime > 0 {
		elapsedTime := time.Since(time.UnixMilli(response.StartTime))
		self.env.GetMetricsRegistry().Timer("xgress_edge_tunnel.terminator.create_timer").Update(elapsedTime)
	}

	self.markEstablished(terminator, "create notification received")
}

func (self *hostedServiceRegistry) HandleReestablish() {
	pfxlog.Logger().Info("control channel reconnected, re-establishing hosted services")

	var reestablishList []*tunnelTerminator
	self.terminators.IterCb(func(_ string, terminator *tunnelTerminator) {
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

func (self *hostedServiceRegistry) NotifyOfCtrlChange(event routerEnv.CtrlEvent) {
	// only re-establish after we lose connection to the leader
	if event.Type == routerEnv.ControllerDisconnected && !self.env.GetNetworkControllers().IsLeaderConnected() {
		self.connectedToLeader.Store(false)
	} else if event.Type == routerEnv.ControllerReconnected {
		if !self.connectedToLeader.Load() {
			self.HandleReestablish()
		}
	}

	if self.env.GetNetworkControllers().IsLeaderConnected() {
		self.connectedToLeader.Store(true)
	}
}

func (self *hostedServiceRegistry) Inspect(timeout time.Duration) *inspect.ErtTerminatorInspectResult {
	evt := &inspectTerminatorsEvent{
		result: atomic.Pointer[[]*inspect.ErtTerminatorInspectDetail]{},
		done:   make(chan struct{}),
	}

	// if we can't queue, grab the results in a non-thread-safe fashion
	if err := self.queueWithTimeout(evt, timeout); err != nil {
		evt.handle(self)
	}

	result := &inspect.ErtTerminatorInspectResult{}

	var err error
	result.Entries, err = evt.GetResults(timeout)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}

type inspectTerminatorsEvent struct {
	result atomic.Pointer[[]*inspect.ErtTerminatorInspectDetail]
	done   chan struct{}
}

func (self *inspectTerminatorsEvent) handle(registry *hostedServiceRegistry) {
	var result []*inspect.ErtTerminatorInspectDetail
	registry.terminators.IterCb(func(key string, terminator *tunnelTerminator) {
		detail := &inspect.ErtTerminatorInspectDetail{
			Key:             key,
			Id:              terminator.id,
			State:           terminator.state.Load().String(),
			Instance:        terminator.context.ListenOptions().Identity,
			Cost:            terminator.context.ListenOptions().Cost,
			Precedence:      terminator.context.ListenOptions().Precedence.String(),
			OperationActive: terminator.operationActive.Load(),
			CreateTime:      terminator.createTime.Format("2006-01-02 15:04:05"),
			LastAttempt:     terminator.lastAttempt.Format("2006-01-02 15:04:05"),
		}
		result = append(result, detail)
	})

	self.result.Store(&result)
	close(self.done)
}

func (self *inspectTerminatorsEvent) GetResults(timeout time.Duration) ([]*inspect.ErtTerminatorInspectDetail, error) {
	select {
	case <-self.done:
		return *self.result.Load(), nil
	case <-time.After(timeout):
		return nil, errors.New("timed out waiting for result")
	}
}

type markEstablishedEvent struct {
	terminator *tunnelTerminator
	reason     string
}

func (self *markEstablishedEvent) handle(registry *hostedServiceRegistry) {
	log := pfxlog.Logger().
		WithField("routerId", registry.env.GetRouterId().Token).
		WithField("terminatorId", self.terminator.id).
		WithField("lifetime", time.Since(self.terminator.createTime))

	if rateLimitCallback := self.terminator.GetAndClearRateLimitCallback(); rateLimitCallback != nil {
		rateLimitCallback.Success()
	}

	if !self.terminator.updateState(xgress_common.TerminatorStateEstablishing, xgress_common.TerminatorStateEstablished, self.reason) {
		log.Info("received additional terminator created notification")
	} else {
		log.Info("terminator established")
	}

	self.terminator.operationActive.Store(false)
}
