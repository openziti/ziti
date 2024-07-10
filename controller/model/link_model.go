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

package model

import (
	"github.com/openziti/foundation/v2/concurrenz"
	"sync"
	"sync/atomic"
	"time"
)

type Link struct {
	SrcLatency  int64
	DstLatency  int64
	Cost        int64
	Id          string
	Iteration   uint32
	Src         *Router
	DstId       string
	Dst         concurrenz.AtomicValue[*Router]
	Protocol    string
	DialAddress string
	state       LinkState
	down        bool
	StaticCost  int32
	usable      atomic.Bool
	lock        sync.Mutex
}

func newLink(id string, linkProtocol string, dialAddress string, initialLatency time.Duration) *Link {
	l := &Link{
		Id:          id,
		Protocol:    linkProtocol,
		DialAddress: dialAddress,
		state: LinkState{
			Mode:      Pending,
			Timestamp: time.Now().UnixMilli(),
		},
		down:       false,
		StaticCost: 1,
		SrcLatency: initialLatency.Nanoseconds(),
		DstLatency: initialLatency.Nanoseconds(),
	}
	l.RecalculateCost()
	l.recalculateUsable()
	return l
}

func (link *Link) GetId() string {
	return link.Id
}

func (link *Link) GetDest() *Router {
	return link.Dst.Load()
}

func (link *Link) CurrentState() LinkState {
	link.lock.Lock()
	defer link.lock.Unlock()
	return link.state
}

func (link *Link) SetState(m LinkMode) {
	link.lock.Lock()
	defer link.lock.Unlock()

	link.state.Mode = m
	link.state.Timestamp = time.Now().UnixMilli()
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
		link.usable.Store(false)
	} else if link.state.Mode != Connected {
		link.usable.Store(false)
	} else {
		link.usable.Store(true)
	}
}

func (link *Link) IsUsable() bool {
	return link.usable.Load()
}

func (link *Link) GetStaticCost() int32 {
	return atomic.LoadInt32(&link.StaticCost)
}

func (link *Link) SetStaticCost(cost int32) {
	atomic.StoreInt32(&link.StaticCost, cost)
	link.RecalculateCost()
}

func (link *Link) GetSrcLatency() int64 {
	return atomic.LoadInt64(&link.SrcLatency)
}

func (link *Link) SetSrcLatency(latency int64) {
	atomic.StoreInt64(&link.SrcLatency, latency)
	link.RecalculateCost()
}

func (link *Link) GetDstLatency() int64 {
	return atomic.LoadInt64(&link.DstLatency)
}

func (link *Link) SetDstLatency(latency int64) {
	atomic.StoreInt64(&link.DstLatency, latency)
	link.RecalculateCost()
}

func (link *Link) RecalculateCost() {
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
	Duplicate
)

func (t LinkMode) String() string {
	if t == Pending {
		return "Pending"
	} else if t == Connected {
		return "Connected"
	} else if t == Failed {
		return "Failed"
	} else if t == Duplicate {
		return "Duplicate"
	} else {
		return ""
	}
}

type LinkState struct {
	Mode      LinkMode
	Timestamp int64
}
