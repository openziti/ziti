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
	"sync/atomic"
	"time"

	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
)

func NewDefaultHeartbeatOptions() *HeartbeatOptions {
	return &HeartbeatOptions{
		HeartbeatOptions:  *channel.DefaultHeartbeatOptions(),
		UnresponsiveAfter: 5 * time.Second,
	}
}

func NewHeartbeatOptions(options *channel.HeartbeatOptions) (*HeartbeatOptions, error) {
	unresponsiveAfter, err := options.GetDuration("unresponsiveAfter")
	if err != nil {
		return nil, err
	}
	result := NewDefaultHeartbeatOptions()
	result.HeartbeatOptions = *options
	if unresponsiveAfter != nil {
		result.UnresponsiveAfter = *unresponsiveAfter
	}
	return result, nil
}

type HeartbeatOptions struct {
	channel.HeartbeatOptions
	UnresponsiveAfter time.Duration
}

type CtrlChannel interface {
	Channel() channel.Channel
	DefaultRequestTimeout() time.Duration
}

type NetworkController interface {
	Channel() channel.Channel
	Latency() time.Duration
	HeartbeatCallback() channel.HeartbeatCallback
	IsUnresponsive() bool
	isMoreResponsive(other NetworkController) bool
}

type NetworkControllers interface {
	Add(ch channel.Channel) (NetworkController, error)
	GetAll() map[string]NetworkController
	AnyCtrlChannel() channel.Channel
	GetCtrlChannel(ctrlId string) channel.Channel
	GetCtrlChannelByAddress(address string) (ctrlId string, ch channel.Channel)
	DefaultRequestTimeout() time.Duration
	ForEach(f func(ctrlId string, ch channel.Channel))
	Close() error
	CloseAndRemoveByAddress(address string) error
}

type networkCtrl struct {
	ch               channel.Channel
	heartbeatOptions *HeartbeatOptions
	lastTx           int64
	lastRx           int64
	latency          atomic.Int64
	unresponsive     atomic.Bool
}

func (self *networkCtrl) HeartbeatCallback() channel.HeartbeatCallback {
	return self
}

func (self *networkCtrl) Channel() channel.Channel {
	return self.ch
}

func (self *networkCtrl) Latency() time.Duration {
	return time.Duration(self.latency.Load())
}

func (self *networkCtrl) IsUnresponsive() bool {
	return self.unresponsive.Load()
}

func (self *networkCtrl) isMoreResponsive(other NetworkController) bool {
	if self.IsUnresponsive() {
		if !other.IsUnresponsive() {
			return false
		}
	} else if other.IsUnresponsive() {
		return true
	}
	return self.Latency() < other.Latency()
}

func (self *networkCtrl) HeartbeatTx(int64) {
	self.lastTx = time.Now().UnixMilli()
}

func (self *networkCtrl) HeartbeatRx(int64) {
}

func (self *networkCtrl) HeartbeatRespTx(int64) {
}

func (self *networkCtrl) HeartbeatRespRx(ts int64) {
	now := time.Now()
	self.lastRx = now.UnixMilli()
	self.latency.Store(now.UnixNano() - ts)
}

func (self *networkCtrl) CheckHeartBeat() {
	if time.Duration(self.latency.Load()) > self.heartbeatOptions.UnresponsiveAfter {
		// if latency is greater than 5 seconds, consider this channel unresponsive
		self.unresponsive.Store(true)
	} else if self.lastTx > 0 && self.lastRx < self.lastTx && (time.Now().UnixMilli()-self.lastTx) > 5000 {
		// if we've sent a heartbeat and not gotten a response in over 5s, consider ourselves unresponsive
		self.unresponsive.Store(true)
	} else {
		self.unresponsive.Store(false)
	}
}

func NewNetworkControllers(defaultRequestTimeout time.Duration, heartbeatOptions *HeartbeatOptions) NetworkControllers {
	return &networkControllers{
		heartbeatOptions:      heartbeatOptions,
		defaultRequestTimeout: defaultRequestTimeout,
	}
}

type networkControllers struct {
	heartbeatOptions      *HeartbeatOptions
	defaultRequestTimeout time.Duration
	ctrls                 concurrenz.CopyOnWriteMap[string, NetworkController]
}

func (self *networkControllers) Add(ch channel.Channel) (NetworkController, error) {
	ctrl := &networkCtrl{
		ch:               ch,
		heartbeatOptions: self.heartbeatOptions,
	}
	if existing := self.ctrls.Get(ctrl.Channel().Id()); existing != nil {
		return nil, fmt.Errorf("duplicate channel with id %v", ctrl.Channel().Id())
	}
	self.ctrls.Put(ctrl.Channel().Id(), ctrl)
	return ctrl, nil
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

func (self *networkControllers) GetCtrlChannel(controllerId string) channel.Channel {
	if ctrl := self.ctrls.Get(controllerId); ctrl != nil {
		return ctrl.Channel()
	}
	return nil
}

func (self *networkControllers) GetCtrlChannelByAddress(address string) (string, channel.Channel) {
	for k, v := range self.ctrls.AsMap() {
		if v.Channel().Underlay().GetLocalAddr().String() == address || v.Channel().Underlay().GetRemoteAddr().String() == address {
			return k, v.Channel()
		}
	}
	return "", nil
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
	var errors errorz.MultipleErrors
	self.ForEach(func(_ string, ch channel.Channel) {
		if err := ch.Close(); err != nil {
			errors = append(errors, err)
		}
	})
	return errors.ToError()
}

func (self *networkControllers) CloseAndRemoveByAddress(address string) error {
	ctrlId, ch := self.GetCtrlChannelByAddress(address)
	if ch != nil {
		delete(self.ctrls.AsMap(), ctrlId)
		if err := ch.Close(); err != nil {
			return err
		}
	}
	return nil
}
