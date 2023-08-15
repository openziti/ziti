/*
	(c) Copyright NetFoundry Inc.

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

package link

import (
	"container/heap"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xlink"
	"time"
)

const (
	StatusPending     linkStatus = "pending"
	StatusDialing     linkStatus = "dialing"
	StatusQueueFailed linkStatus = "queueFailed"
	StatusDialFailed  linkStatus = "dialFailed"
	StatusLinkFailed  linkStatus = "linkFailed"
	StatusDestRemoved linkStatus = "destRemoved"
	StatusEstablished linkStatus = "established"
)

type linkStatus string

func (self linkStatus) String() string {
	return string(self)
}

type linkDest struct {
	id          string
	version     string
	healthy     bool
	unhealthyAt time.Time
	linkMap     map[string]*linkState
}

func (self *linkDest) update(update *linkDestUpdate) {
	if self.healthy && !update.healthy {
		self.unhealthyAt = time.Now()
	}

	self.healthy = update.healthy

	if update.healthy {
		self.version = update.version
	}
}

type linkState struct {
	linkKey        string
	linkId         string
	status         linkStatus
	dialAttempts   uint
	connectedCount uint
	retryDelay     time.Duration
	nextDial       time.Time
	dest           *linkDest
	listener       *ctrl_pb.Listener
	dialer         xlink.Dialer
	allowedDials   int
}

func (self *linkState) GetLinkKey() string {
	return self.linkKey
}

func (self *linkState) GetLinkId() string {
	return self.linkId
}

func (self *linkState) GetRouterId() string {
	return self.dest.id
}

func (self *linkState) GetAddress() string {
	return self.listener.Address
}

func (self *linkState) GetLinkProtocol() string {
	return self.listener.Protocol
}

func (self *linkState) GetRouterVersion() string {
	return self.dest.version
}

func (self *linkState) dialFailed(registry *linkRegistryImpl) {
	if self.allowedDials > 0 {
		self.allowedDials--
	}

	if self.allowedDials == 0 {
		delete(self.dest.linkMap, self.linkKey)
		return
	}

	backoffConfig := self.dialer.GetHealthyBackoffConfig()
	if !self.dest.healthy {
		backoffConfig = self.dialer.GetUnhealthyBackoffConfig()
	}

	self.retryDelay = time.Duration(float64(self.retryDelay) * backoffConfig.GetRetryBackoffFactor())
	if self.retryDelay < backoffConfig.GetMinRetryInterval() {
		self.retryDelay = backoffConfig.GetMinRetryInterval()
	}

	if self.retryDelay > backoffConfig.GetMaxRetryInterval() {
		self.retryDelay = backoffConfig.GetMaxRetryInterval()
	}

	self.nextDial = time.Now().Add(self.retryDelay)

	heap.Push(registry.linkStateQueue, self)
}
