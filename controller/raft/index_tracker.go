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

package raft

import (
	"github.com/pkg/errors"
	"sync"
	"time"
)

type IndexTracker interface {
	Index() uint64
	WaitForIndex(index uint64, deadline time.Time) error
	NotifyOfIndex(index uint64)
}

func NewIndexTracker() IndexTracker {
	return &indexTrackerImpl{
		notify: make(chan struct{}),
	}
}

type indexTrackerImpl struct {
	sync.Mutex
	index  uint64
	notify chan struct{}
}

func (self *indexTrackerImpl) Index() uint64 {
	idx, _ := self.getState()
	return idx
}

func (self *indexTrackerImpl) getState() (uint64, <-chan struct{}) {
	self.Lock()
	defer self.Unlock()
	return self.index, self.notify
}

func (self *indexTrackerImpl) WaitForIndex(index uint64, deadline time.Time) error {
	for {
		currentIndex, notifier := self.getState()
		if currentIndex >= index {
			return nil
		}
		now := time.Now()
		if !deadline.After(now) {
			return errors.New("timed out")
		}

		select {
		case <-notifier:
		case <-time.After(deadline.Sub(now)):
		}
	}
}

func (self *indexTrackerImpl) NotifyOfIndex(index uint64) {
	self.Lock()
	defer self.Unlock()

	if index > self.index {
		self.index = index
	}

	close(self.notify)
	self.notify = make(chan struct{})
}
