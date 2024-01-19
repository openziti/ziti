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
	"github.com/cenkalti/backoff/v4"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

func newHostedServicesRegistry(env routerEnv.RouterEnv) *hostedServiceRegistry {
	result := &hostedServiceRegistry{
		services:       sync.Map{},
		events:         make(chan terminatorEvent),
		env:            env,
		retriesPending: false,
		waits:          cmap.New[chan struct{}](),
	}
	go result.run()
	return result
}

type hostedServiceRegistry struct {
	services       sync.Map
	events         chan terminatorEvent
	env            routerEnv.RouterEnv
	retriesPending bool
	waits          cmap.ConcurrentMap[string, chan struct{}]
}

type terminatorEvent interface {
	handle(registry *hostedServiceRegistry)
}

func (self *hostedServiceRegistry) run() {
	retryTicker := time.NewTicker(50 * time.Millisecond)
	defer retryTicker.Stop()

	for {
		var retryChan <-chan time.Time
		if self.retriesPending {
			retryChan = retryTicker.C
		}

		select {
		case <-self.env.GetCloseNotify():
			return
		case event := <-self.events:
			event.handle(self)
		case <-retryChan:
			self.scanForRetries()
		}
	}
}

type establishTerminatorEvent struct {
	terminator *edgeTerminator
}

func (self *establishTerminatorEvent) handle(registry *hostedServiceRegistry) {
	registry.tryEstablish(self.terminator)
}

func (self *hostedServiceRegistry) EstablishTerminator(terminator *edgeTerminator) {
	event := &establishTerminatorEvent{
		terminator: terminator,
	}

	self.Put(terminator.terminatorId.Load(), terminator)

	select {
	case <-self.env.GetCloseNotify():
		pfxlog.Logger().WithField("terminatorId", terminator.terminatorId.Load()).
			Error("unable to establish terminator, hosted service registry has been shutdown")
	case self.events <- event:
	}
}

func (self *hostedServiceRegistry) scanForRetries() {
	self.services.Range(func(key, value any) bool {
		terminator := value.(*edgeTerminator)
		if terminator.state.Load() == TerminatorStatePendingEstablishment {
			self.tryEstablish(terminator)
		}
		return true
	})
}

func (self *hostedServiceRegistry) tryEstablish(terminator *edgeTerminator) {
	log := pfxlog.Logger().WithField("terminatorId", terminator.Id()).
		WithField("token", terminator.token).
		WithField("state", terminator.state.Load().String())

	if !terminator.state.CompareAndSwap(TerminatorStatePendingEstablishment, TerminatorStateEstablishing) {
		log.Info("terminator not pending, not going to try to establish")
		return
	}

	err := self.env.GetRateLimiterPool().QueueOrError(func() {
		self.establishTerminatorWithRetry(terminator)
	})
	if err != nil {
		log.Info("rate limited: unable to queue to establish")
		self.retriesPending = true
	}
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

func (self *hostedServiceRegistry) establishTerminatorWithRetry(terminator *edgeTerminator) {
	log := logrus.
		WithField("terminatorId", terminator.terminatorId.Load()).
		WithField("token", terminator.token)

	if state := terminator.state.Load(); state != TerminatorStateEstablishing {
		log.WithField("state", state.String()).Info("not attempting to establish terminator, not in establishing state")
		return
	}

	operation := func() error {
		if terminator.edgeClientConn.ch.IsClosed() {
			return backoff.Permanent(fmt.Errorf("edge link is closed, stopping terminator creation for terminator %s",
				terminator.terminatorId.Load()))
		}
		if state := terminator.state.Load(); state != TerminatorStateEstablishing {
			return backoff.Permanent(fmt.Errorf("terminator state is %v, stopping terminator creation for terminator %s",
				state.String(), terminator.terminatorId.Load()))
		}
		if terminator.terminatorId.Load() == "" {
			return backoff.Permanent(fmt.Errorf("terminator has been closed, stopping terminator creation"))
		}

		err := self.establishTerminator(terminator)
		if err != nil && terminator.state.Load() != TerminatorStateEstablishing {
			return backoff.Permanent(err)
		}
		return err
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 5 * time.Second
	expBackoff.MaxInterval = 5 * time.Minute

	if err := backoff.Retry(operation, expBackoff); err != nil {
		log.WithError(err).Error("stopping attempts to establish terminator, see error")
	} else if terminator.postValidate {
		if result, err := terminator.inspect(true); err != nil {
			log.WithError(err).Error("error validating terminator after create")
		} else if result.Type != edge.ConnTypeBind {
			log.WithError(err).Error("terminator invalid in sdk after create, closed")
		} else {
			log.Info("terminator validated successfully")
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

	err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendAndWaitForWire(ctrlCh)
	if err != nil {
		return err
	}

	if self.waitForTerminatorCreated(terminatorId, 10*time.Second) {
		return nil
	}

	// return an error to indicate that we need to check if a response has come back after the next interval,
	// and if not, re-send
	return errors.Errorf("timeout waiting for response to create terminator request for terminator %v", terminator.terminatorId.Load())
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

	if response.Result != edge_ctrl_pb.CreateTerminatorResult_Success {
		terminator.close(true, response.Msg)
		return
	}

	if terminator.state.CompareAndSwap(TerminatorStateEstablishing, TerminatorStateEstablished) {
		self.notifyTerminatorCreated(response.TerminatorId)
		log.Info("received terminator created notification")
	} else {
		log.Info("received additional terminator created notification")
	}

	if terminator.notifyEstablished {
		go func() {
			notifyMsg := channel.NewMessage(edge.ContentTypeBindSuccess, nil)
			notifyMsg.PutUint32Header(edge.ConnIdHeader, terminator.MsgChannel.Id())

			if err := notifyMsg.WithTimeout(time.Second * 30).Send(terminator.MsgChannel.Channel); err != nil {
				log.WithError(err).Error("failed to send bind success")
			}
		}()
	}
}

func (self *hostedServiceRegistry) waitForTerminatorCreated(id string, timeout time.Duration) bool {
	notifyC := make(chan struct{})
	defer self.waits.Remove(id)

	self.waits.Set(id, notifyC)
	select {
	case <-notifyC:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (self *hostedServiceRegistry) notifyTerminatorCreated(id string) {
	notifyC, _ := self.waits.Get(id)
	if notifyC != nil {
		close(notifyC)
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
