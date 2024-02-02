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
	"container/heap"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/router/env"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

func newHostedServicesRegistry(env routerEnv.RouterEnv) *hostedServiceRegistry {
	result := &hostedServiceRegistry{
		services:        sync.Map{},
		events:          make(chan terminatorEvent),
		env:             env,
		retriesPending:  false,
		terminatorQueue: &terminatorHeap{},
	}
	go result.run()
	return result
}

type hostedServiceRegistry struct {
	services        sync.Map
	events          chan terminatorEvent
	env             routerEnv.RouterEnv
	retriesPending  bool
	terminatorQueue *terminatorHeap
}

type terminatorEvent interface {
	handle(registry *hostedServiceRegistry)
}

func (self *hostedServiceRegistry) run() {
	queueCheckTicker := time.NewTicker(100 * time.Millisecond)
	defer queueCheckTicker.Stop()

	longQueueCheckTicker := time.NewTicker(time.Second)
	defer longQueueCheckTicker.Stop()

	for {
		var retryChan <-chan time.Time
		if self.retriesPending {
			retryChan = queueCheckTicker.C
		}

		select {
		case <-self.env.GetCloseNotify():
			return
		case event := <-self.events:
			event.handle(self)
		case <-retryChan:
			self.evaluateLinkStateQueue()
		case <-longQueueCheckTicker.C:
			self.scanForRetries()
		}
	}
}

func (self *hostedServiceRegistry) evaluateLinkStateQueue() {
	now := time.Now()
	for len(*self.terminatorQueue) > 0 {
		next := (*self.terminatorQueue)[0]
		if now.Before(next.nextAttempt) {
			return
		}
		heap.Pop(self.terminatorQueue)
		self.evaluateTerminator(next)
	}
	self.retriesPending = false
}

type establishTerminatorEvent struct {
	terminator *edgeTerminator
}

func (self *establishTerminatorEvent) handle(registry *hostedServiceRegistry) {
	registry.evaluateTerminator(self.terminator)
}

type calculateRetry struct {
	terminator  *edgeTerminator
	queueFailed bool
}

func (self *calculateRetry) handle(registry *hostedServiceRegistry) {
	self.terminator.calculateRetry(self.queueFailed)
	registry.retriesPending = true
}

func (self *hostedServiceRegistry) EstablishTerminator(terminator *edgeTerminator) {
	self.Put(terminator.terminatorId.Load(), terminator)
	self.queue(&establishTerminatorEvent{
		terminator: terminator,
	})
}

func (self *hostedServiceRegistry) queue(event terminatorEvent) {
	select {
	case self.events <- event:
	case <-self.env.GetCloseNotify():
		pfxlog.Logger().Error("unable to queue terminator event, hosted service registry has been shutdown")
	}
}

func (self *hostedServiceRegistry) scheduleRetry(terminator *edgeTerminator, queueFailed bool) {
	terminator.establishActive.Store(false)
	if terminator.state.CompareAndSwap(TerminatorStateEstablished, TerminatorStatePendingEstablishment) {
		self.queue(&calculateRetry{
			terminator:  terminator,
			queueFailed: queueFailed,
		})
	}
}

func (self *hostedServiceRegistry) scanForRetries() {
	self.services.Range(func(key, value any) bool {
		terminator := value.(*edgeTerminator)
		if terminator.state.Load() == TerminatorStatePendingEstablishment {
			self.evaluateTerminator(terminator)
		}
		return true
	})
}

func (self *hostedServiceRegistry) Put(hostId string, conn *edgeTerminator) {
	self.services.Store(hostId, conn)
}

func (self *hostedServiceRegistry) Get(hostId string) (*edgeTerminator, bool) {
	val, ok := self.services.Load(hostId)
	if !ok {
		return nil, false
	}
	ch, ok := val.(*edgeTerminator)
	return ch, ok
}

func (self *hostedServiceRegistry) GetTerminatorForListener(listenerId string) *edgeTerminator {
	var result *edgeTerminator
	self.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.listenerId == listenerId {
			result = terminator
			return false
		}
		return true
	})
	return result
}

func (self *hostedServiceRegistry) Delete(hostId string) {
	self.services.Delete(hostId)
}

func (self *hostedServiceRegistry) cleanupServices(proxy *edgeClientConn) {
	self.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.edgeClientConn == proxy {
			terminator.close(false, "") // don't notify, channel is already closed, we can't send messages
			self.services.Delete(key)
		}
		return true
	})
}

func (self *hostedServiceRegistry) cleanupDuplicates(newest *edgeTerminator) {
	self.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator != newest && newest.token == terminator.token && newest.instance == terminator.instance {
			terminator.close(false, "duplicate terminator") // don't notify, channel is already closed, we can't send messages
			self.services.Delete(key)
			pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
				WithField("token", terminator.token).
				WithField("instance", terminator.instance).
				WithField("terminatorId", terminator.terminatorId.Load()).
				WithField("duplicateOf", newest.terminatorId.Load()).
				Info("duplicate removed")
		}
		return true
	})
}

func (self *hostedServiceRegistry) unbindSession(connId uint32, sessionToken string, proxy *edgeClientConn) bool {
	atLeastOneRemoved := false
	self.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.MsgChannel.Id() == connId && terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			terminator.close(false, "unbind successful") // don't notify, sdk asked us to unbind
			self.services.Delete(key)
			pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
				WithField("token", sessionToken).
				WithField("terminatorId", terminator.terminatorId.Load()).
				Info("terminator removed")
			atLeastOneRemoved = true
		}
		return true
	})
	return atLeastOneRemoved
}

func (self *hostedServiceRegistry) getRelatedTerminators(sessionToken string, proxy *edgeClientConn) []*edgeTerminator {
	var result []*edgeTerminator
	self.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			result = append(result, terminator)
		}
		return true
	})
	return result
}

func (self *hostedServiceRegistry) evaluateTerminator(terminator *edgeTerminator) {
	log := logrus.
		WithField("terminatorId", terminator.terminatorId.Load()).
		WithField("state", terminator.state.Load()).
		WithField("token", terminator.token)

	if terminator.edgeClientConn.ch.IsClosed() {
		log.Info("terminator sdk channel closed, not trying to establish")
		return
	}

	if terminator.terminatorId.Load() == "" {
		log.Info("terminator has been closed, not trying to establish")
		return
	}

	tryEstablish := terminator.state.Load() == TerminatorStatePendingEstablishment && terminator.nextAttempt.Before(time.Now())

	if tryEstablish && terminator.establishActive.CompareAndSwap(false, true) {
		if !terminator.state.CompareAndSwap(TerminatorStatePendingEstablishment, TerminatorStateEstablishing) {
			log.Infof("terminator in state %s, not pending establishment, not queueing", terminator.state.Load())
			return
		}

		log.Info("queuing terminator to send create")

		err := self.env.GetRateLimiterPool().QueueOrError(func() {
			defer func() {
				self.scheduleRetry(terminator, false)
			}()

			if err := self.establishTerminator(terminator); err != nil {
				log.WithError(err).Error("error establishing terminator")
			}
		})

		if err != nil {
			log.Info("rate limited: unable to queue to establish")
			self.scheduleRetry(terminator, true)
		}
	}
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

	if response.Result == edge_ctrl_pb.CreateTerminatorResult_FailedBusy {
		log.Info("controller too busy to handle create terminator, retrying later")
		return
	}

	if response.Result != edge_ctrl_pb.CreateTerminatorResult_Success {
		terminator.close(true, response.Msg)
		return
	}

	if terminator.state.CompareAndSwap(TerminatorStateEstablishing, TerminatorStateEstablished) {
		log.Info("received terminator created notification")
	} else {
		log.Info("received additional terminator created notification")
	}

	isValid := true
	if terminator.postValidate {
		if result, err := terminator.inspect(true); err != nil {
			log.WithError(err).Error("error validating terminator after create")
		} else if result.Type != edge.ConnTypeBind {
			log.WithError(err).Error("terminator invalid in sdk after create, closed")
			isValid = false
		} else {
			log.Info("terminator validated successfully")
		}
	}

	if isValid && terminator.notifyEstablished {
		notifyMsg := channel.NewMessage(edge.ContentTypeBindSuccess, nil)
		notifyMsg.PutUint32Header(edge.ConnIdHeader, terminator.MsgChannel.Id())

		if err := notifyMsg.WithTimeout(time.Second * 30).Send(terminator.MsgChannel.Channel); err != nil {
			log.WithError(err).Error("failed to send bind success")
		}
	}
}

func (self *hostedServiceRegistry) HandleReconnect() {
	var restablishList []*edgeTerminator
	self.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.state.CompareAndSwap(TerminatorStateEstablished, TerminatorStatePendingEstablishment) {
			restablishList = append(restablishList, terminator)
		}
		return true
	})

	// wait for verify terminator events to come in
	time.Sleep(10 * time.Second)

	for _, terminator := range restablishList {
		if terminator.state.Load() == TerminatorStatePendingEstablishment {
			self.EstablishTerminator(terminator)
		}
	}
}
