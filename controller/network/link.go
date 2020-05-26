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
	"github.com/openziti/foundation/util/info"
)

type Link struct {
	Id         *identity.TokenId
	Src        *Router
	Dst        *Router
	state      []*LinkState
	Down       bool
	Cost       int
	SrcLatency int64
	DstLatency int64
}

func newLink(id *identity.TokenId) *Link {
	l := &Link{
		Id:    id,
		state: make([]*LinkState, 0),
		Down:  false,
		Cost:  1,
	}
	l.addState(&LinkState{Mode: Pending, Timestamp: info.NowInMilliseconds()})
	return l
}

func (link *Link) CurrentState() *LinkState {
	if link.state == nil || len(link.state) < 1 {
		return nil
	}
	return link.state[0]
}

func (link *Link) addState(s *LinkState) {
	if link.state == nil {
		link.state = make([]*LinkState, 0)
	}
	link.state = append([]*LinkState{s}, link.state...)
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
