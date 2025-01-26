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

package env

import (
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openziti/channel/v3"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
)

type CtrlEventListener interface {
	NotifyOfCtrlEvent(event CtrlEvent)
}

type CtrlEventListenerFunc func(event CtrlEvent)

func (self CtrlEventListenerFunc) NotifyOfCtrlEvent(event CtrlEvent) {
	self(event)
}

type CtrlEventType string

const (
	ControllerAdded       CtrlEventType = "Added"
	ControllerReconnected CtrlEventType = "Reconnected"
	ControllerRemoved     CtrlEventType = "Removed"
)

type CtrlEvent struct {
	Type       CtrlEventType
	Controller NetworkController
}

type NetworkControllers interface {
	UpdateControllerEndpoints(endpoints []string) bool
	UpdateLeader(leaderId string)
	GetAll() map[string]NetworkController
	GetNetworkController(ctrlId string) NetworkController
	AnyCtrlChannel() channel.Channel
	GetModelUpdateCtrlChannel() channel.Channel
	GetIfResponsive(ctrlId string) (channel.Channel, bool)
	AllResponsiveCtrlChannels() []channel.Channel
	AnyValidCtrlChannel() channel.Channel
	GetCtrlChannel(ctrlId string) channel.Channel
	DefaultRequestTimeout() time.Duration
	ForEach(f func(ctrlId string, ch channel.Channel))
	Close() error
	Inspect() *inspect.ControllerInspectDetails
	AddChangeListener(listener CtrlEventListener)
	NotifyOfReconnect(ctrlId string)
	GetExpectedCtrlCount() uint32
}

type CtrlDialer func(address transport.Address, bindHandler channel.BindHandler) error

func NewNetworkControllers(defaultRequestTimeout time.Duration, dialer CtrlDialer, heartbeatOptions *HeartbeatOptions) NetworkControllers {
	return &networkControllers{
		ctrlDialer:            dialer,
		heartbeatOptions:      heartbeatOptions,
		defaultRequestTimeout: defaultRequestTimeout,
		ctrlEndpoints:         cmap.New[struct{}](),
	}
}

type networkControllers struct {
	lock                  sync.Mutex
	ctrlDialer            CtrlDialer
	heartbeatOptions      *HeartbeatOptions
	defaultRequestTimeout time.Duration
	ctrlEndpoints         cmap.ConcurrentMap[string, struct{}]
	ctrls                 concurrenz.CopyOnWriteMap[string, NetworkController]
	leaderId              concurrenz.AtomicValue[string]
	ctrlChangeListeners   concurrenz.CopyOnWriteSlice[CtrlEventListener]
	expectedCtrlCount     atomic.Uint32
}

func (self *networkControllers) AddChangeListener(listener CtrlEventListener) {
	self.ctrlChangeListeners.Append(listener)
}

func (self *networkControllers) GetExpectedCtrlCount() uint32 {
	return self.expectedCtrlCount.Load()
}

func (self *networkControllers) UpdateControllerEndpoints(addresses []string) bool {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.expectedCtrlCount.Store(uint32(len(addresses)))
	changed := false

	log := pfxlog.Logger()
	endpoints := map[string]struct{}{}
	for _, endpoint := range addresses {
		endpoints[endpoint] = struct{}{}
	}

	for _, endpoint := range self.ctrlEndpoints.Keys() {
		if _, ok := endpoints[endpoint]; ok {
			// already known endpoint, don't need to try and connect in next step
			delete(endpoints, endpoint)
		} else {
			// existing endpoint is no longer valid, close and remove it
			log.WithField("endpoint", endpoints).Info("removing old ctrl endpoint")
			changed = true
			self.CloseAndRemoveByAddress(endpoint)
		}
	}

	for endpoint := range endpoints {
		log.WithField("endpoint", endpoints).Info("adding new ctrl endpoint")
		changed = true
		self.connectToControllerWithBackoff(endpoint)
	}

	return changed
}

func (self *networkControllers) UpdateLeader(leaderId string) {
	self.leaderId.Store(leaderId)
}

func (self *networkControllers) connectToControllerWithBackoff(endpoint string) {
	log := pfxlog.Logger().WithField("endpoint", endpoint)

	addr, err := transport.ParseAddress(endpoint)
	if err != nil {
		log.WithField("endpoint", endpoint).WithError(err).Error("unable to parse endpoint address, ignoring")
		return
	}

	self.ctrlEndpoints.Set(endpoint, struct{}{})

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 50 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = 100 * 365 * 24 * time.Hour

	bindHandler := channel.BindHandlerF(func(binding channel.Binding) error {
		id := binding.GetChannel().Id()
		binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CurrentIndexMessageType), func(m *channel.Message, ch channel.Channel) {
			if idx, ok := m.GetUint64Header(int32(edge_ctrl_pb.Header_RouterDataModelIndex)); ok {
				if ctrl := self.GetNetworkController(id); ctrl != nil {
					ctrl.updateDataModelIndex(idx)
				}
			}
		})

		return self.Add(endpoint, binding.GetChannel())
	})

	operation := func() error {
		if !self.ctrlEndpoints.Has(endpoint) {
			return backoff.Permanent(errors.New("controller removed before connection established"))
		}
		err := self.ctrlDialer(addr, bindHandler)
		if err != nil {
			log.WithError(err).Error("unable to connect controller")
		}
		return err
	}

	log.Info("starting connection attempts")

	go func() {
		if err := backoff.Retry(operation, expBackoff); err != nil {
			log.WithError(err).Error("unable to connect controller, stopping retries.")
		} else {
			log.Info("successfully connected to controller")
		}
	}()
}

func (self *networkControllers) Add(address string, ch channel.Channel) error {
	ctrl := newNetworkCtrl(ch, address, self.heartbeatOptions)

	if versionValue, found := ch.Underlay().Headers()[channel.HelloVersionHeader]; found {
		if versionInfo, err := versions.StdVersionEncDec.Decode(versionValue); err == nil {
			ctrl.versionInfo = versionInfo
		} else {
			return errors.Wrap(err, "could not parse version info from controller hello, closing connection")
		}
	} else {
		return errors.New("no version header provided")
	}

	if existing := self.ctrls.Get(ch.Id()); existing != nil {
		if !existing.Channel().IsClosed() {
			return fmt.Errorf("duplicate channel with id %v", ctrl.Channel().Id())
		}
	}
	self.ctrls.Put(ch.Id(), ctrl)

	self.notifyOfChange(ctrl, ControllerAdded)

	return nil
}

func (self *networkControllers) NotifyOfReconnect(ctrlId string) {
	if ctrl := self.GetNetworkController(ctrlId); ctrl != nil {
		self.notifyOfChange(ctrl, ControllerReconnected)
	}
}

func (self *networkControllers) notifyOfChange(controller NetworkController, eventType CtrlEventType) {
	for _, l := range self.ctrlChangeListeners.Value() {
		l.NotifyOfCtrlEvent(CtrlEvent{
			Type:       eventType,
			Controller: controller,
		})
	}
}

func (self *networkControllers) GetAll() map[string]NetworkController {
	return self.ctrls.AsMap()
}

func (self *networkControllers) AnyCtrlChannel() channel.Channel {
	var current NetworkController
	for _, ctrl := range self.ctrls.AsMap() {
		if current == nil || ctrl.isMoreResponsive(current) {
			current = ctrl
		}
	}
	if current == nil {
		return nil
	}
	return current.Channel()
}

func (self *networkControllers) isLeader(controller NetworkController) bool {
	return self.leaderId.Load() == controller.Channel().Id()
}

func (self *networkControllers) GetModelUpdateCtrlChannel() channel.Channel {
	var current NetworkController
	for _, ctrl := range self.ctrls.AsMap() {
		if current == nil ||
			(ctrl.isMoreResponsive(current) && !self.isLeader(current)) ||
			(!ctrl.IsUnresponsive() && self.isLeader(ctrl)) {
			current = ctrl
		}
	}
	if current == nil {
		return nil
	}
	return current.Channel()
}

func (self *networkControllers) AllResponsiveCtrlChannels() []channel.Channel {
	var channels []channel.Channel
	for _, ctrl := range self.ctrls.AsMap() {
		if !ctrl.IsUnresponsive() {
			channels = append(channels, ctrl.Channel())
		}
	}
	return channels
}

func (self *networkControllers) GetIfResponsive(ctrlId string) (channel.Channel, bool) {
	ch := self.ctrls.Get(ctrlId)
	if ch == nil {
		return nil, false
	}
	if ch.IsConnected() && !ch.IsUnresponsive() {
		return ch.Channel(), true
	}
	return nil, true
}

func (self *networkControllers) AnyValidCtrlChannel() channel.Channel {
	delay := 10 * time.Millisecond
	for {
		result := self.AnyCtrlChannel()
		if result != nil {
			return result
		}
		time.Sleep(delay)
		delay = delay * 2
		if delay >= time.Minute {
			delay = time.Minute
		}
	}
}

func (self *networkControllers) GetCtrlChannel(controllerId string) channel.Channel {
	if ctrl := self.ctrls.Get(controllerId); ctrl != nil {
		return ctrl.Channel()
	}
	return nil
}

func (self *networkControllers) GetNetworkController(controllerId string) NetworkController {
	return self.ctrls.Get(controllerId)
}

func (self *networkControllers) DefaultRequestTimeout() time.Duration {
	return self.defaultRequestTimeout
}

func (self *networkControllers) ForEach(f func(controllerId string, ch channel.Channel)) {
	for controllerId, ctrl := range self.ctrls.AsMap() {
		f(controllerId, ctrl.Channel())
	}
}

func (self *networkControllers) Close() error {
	var errList errorz.MultipleErrors
	self.ForEach(func(_ string, ch channel.Channel) {
		if err := ch.Close(); err != nil {
			errList = append(errList, err)
		}
	})
	return errList.ToError()
}

func (self *networkControllers) CloseAndRemoveByAddress(address string) {
	self.ctrlEndpoints.Remove(address)

	for id, ctrl := range self.ctrls.AsMap() {
		if ctrl.Address() == address {
			self.ctrls.Delete(id)
			if err := ctrl.Channel().Close(); err != nil {
				pfxlog.Logger().WithField("ctrlId", id).WithField("endpoint", address).WithError(err).Error("error closing channel to controller")
			}
			self.notifyOfChange(ctrl, ControllerRemoved)
		}
	}
}

func (self *networkControllers) Inspect() *inspect.ControllerInspectDetails {
	result := &inspect.ControllerInspectDetails{
		Controllers: map[string]*inspect.ControllerInspectDetail{},
	}

	for id, ctrl := range self.ctrls.AsMap() {
		version := ""
		if ctrl.GetVersion() != nil {
			version = ctrl.GetVersion().Version
		}
		result.Controllers[id] = &inspect.ControllerInspectDetail{
			ControllerId:         id,
			IsConnected:          ctrl.IsConnected(),
			IsResponsive:         !ctrl.IsUnresponsive(),
			Address:              ctrl.Address(),
			Latency:              ctrl.Latency().String(),
			Version:              version,
			TimeSinceLastContact: ctrl.TimeSinceLastContact().String(),
		}
	}

	return result
}
