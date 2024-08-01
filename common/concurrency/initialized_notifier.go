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

package concurrency

import (
	"github.com/michaelquigley/pfxlog"
	"sync/atomic"
)

type InitState interface {
	WaitTillInitialized()
	MarkInitialized()
}

func NewInitState() InitState {
	return &channelInitState{
		c: make(chan struct{}),
	}
}

type channelInitState struct {
	c           chan struct{}
	initialized atomic.Bool
}

func (self *channelInitState) WaitTillInitialized() {
	<-self.c
}

func (self *channelInitState) MarkInitialized() {
	if self.initialized.CompareAndSwap(false, true) {
		close(self.c)
	} else {
		pfxlog.Logger().Panic("initialized marked complete more than once")
	}
}
