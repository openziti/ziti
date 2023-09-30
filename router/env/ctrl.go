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
	"github.com/openziti/foundation/v2/versions"
	"sync/atomic"
	"time"

	"github.com/openziti/channel/v2"
)

type NetworkController interface {
	Channel() channel.Channel
	Address() string
	Latency() time.Duration
	HeartbeatCallback() channel.HeartbeatCallback
	IsUnresponsive() bool
	isMoreResponsive(other NetworkController) bool
	GetVersion() *versions.VersionInfo
}

type networkCtrl struct {
	ch               channel.Channel
	address          string
	heartbeatOptions *HeartbeatOptions
	lastTx           int64
	lastRx           int64
	latency          atomic.Int64
	unresponsive     atomic.Bool
	versionInfo      *versions.VersionInfo
}

func (self *networkCtrl) HeartbeatCallback() channel.HeartbeatCallback {
	return self
}

func (self *networkCtrl) Channel() channel.Channel {
	return self.ch
}

func (self *networkCtrl) GetVersion() *versions.VersionInfo {
	return self.versionInfo
}

func (self *networkCtrl) Address() string {
	return self.address
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
