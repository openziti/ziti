/*
	Copyright NetFoundry, Inc.

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

package network

import (
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/info"
	"sync"
	"sync/atomic"
)

type Link struct {
	Id         *identity.TokenId
	Src        *Router
	Dst        *Router
	state      []*LinkState
	down       bool
	StaticCost int32
	SrcLatency int64
	DstLatency int64
	Cost       int64
	usable     concurrenz.AtomicBoolean
	lock       sync.Mutex
}

func newLink(id *identity.TokenId) *Link {
	l := &Link{
		Id:         id,
		state:      make([]*LinkState, 0),
		down:       false,
		StaticCost: 1,
		Cost:       1,
	}
	l.addState(&LinkState{Mode: Pending, Timestamp: info.NowInMilliseconds()})
	return l
}

func (link *Link) CurrentState() *LinkState {
	link.lock.Lock()
	defer link.lock.Unlock()
	if link.state == nil || len(link.state) < 1 {
		return nil
	}
	return link.state[0]
}

func (link *Link) addState(s *LinkState) {
	link.lock.Lock()
	defer link.lock.Unlock()

	if link.state == nil {
		link.state = make([]*LinkState, 0)
	}
	link.state = append([]*LinkState{s}, link.state...)
	link.recalculateUsable()
}

func (link *Link) SetDown(down bool) {
	link.lock.Lock()
	defer link.lock.Unlock()
	link.down = down
	link.recalculateUsable()
}

func (link *Link) IsDown() bool {
	link.lock.Lock()
	defer link.lock.Unlock()
	return link.down
}

func (link *Link) recalculateUsable() {
	if link.down {
		link.usable.Set(false)
	} else if len(link.state) < 1 || link.state[0].Mode != Connected {
		link.usable.Set(false)
	} else {
		link.usable.Set(true)
	}
}

func (link *Link) IsUsable() bool {
	return link.usable.Get()
}

func (link *Link) GetStaticCost() int32 {
	return atomic.LoadInt32(&link.StaticCost)
}

func (link *Link) SetStaticCost(cost int32) {
	atomic.StoreInt32(&link.StaticCost, cost)
	link.recalculateCost()
}

func (link *Link) GetSrcLatency() int64 {
	return atomic.LoadInt64(&link.SrcLatency)
}

func (link *Link) SetSrcLatency(latency int64) {
	atomic.StoreInt64(&link.SrcLatency, latency)
	link.recalculateCost()
}

func (link *Link) GetDstLatency() int64 {
	return atomic.LoadInt64(&link.DstLatency)
}

func (link *Link) SetDstLatency(latency int64) {
	atomic.StoreInt64(&link.DstLatency, latency)
	link.recalculateCost()
}

func (link *Link) recalculateCost() {
	cost := int64(link.GetStaticCost()) + link.GetSrcLatency()/1_000_000 + link.GetDstLatency()/1_000_000
	atomic.StoreInt64(&link.Cost, cost)
}

func (link *Link) GetCost() int64 {
	return atomic.LoadInt64(&link.Cost)
}

type LinkMode byte

const (
	Pending LinkMode = iota
	Connected
	Failed
)

func (t LinkMode) String() string {
	if t == Pending {
		return "Pending"
	} else if t == Connected {
		return "Connected"
	} else if t == Failed {
		return "Failed"
	} else {
		return ""
	}
}

type LinkState struct {
	Mode      LinkMode
	Timestamp int64
}

func newLinkState(mode LinkMode) *LinkState {
	return &LinkState{
		Mode:      mode,
		Timestamp: info.NowInMilliseconds(),
	}
}
