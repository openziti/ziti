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
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/orcaman/concurrent-map"
)

type Session struct {
	Id         *identity.TokenId
	ClientId   *identity.TokenId
	Service    *Service
	Terminator xt.Terminator
	Path       *Path
	Rerouting  concurrenz.AtomicBoolean
	PeerData   xt.PeerData
}

func (s *Session) latency() int64 {
	var latency int64
	for _, l := range s.Path.Links {
		latency += l.GetSrcLatency()
		latency += l.GetDstLatency()
	}
	return latency
}

type sessionController struct {
	sessions          cmap.ConcurrentMap // map[string]*Session
	sessionsByService cmap.ConcurrentMap // map[string]*Sessions
}

func newSessionController() *sessionController {
	return &sessionController{
		sessions:          cmap.New(),
		sessionsByService: cmap.New(),
	}
}

func (c *sessionController) add(sn *Session) {
	c.sessions.Set(sn.Id.Token, sn)

	if !c.sessionsByService.Has(sn.Service.Id) {
		c.sessionsByService.Set(sn.Service.Id, cmap.New())
	}
	t, _ := c.sessionsByService.Get(sn.Service.Id)
	sessionsForService := t.(cmap.ConcurrentMap)
	sessionsForService.Set(sn.Id.Token, sn)
}

func (c *sessionController) get(id *identity.TokenId) (*Session, bool) {
	if t, found := c.sessions.Get(id.Token); found {
		return t.(*Session), true
	}
	return nil, false
}

func (c *sessionController) all() []*Session {
	sessions := make([]*Session, 0)
	for i := range c.sessions.IterBuffered() {
		sessions = append(sessions, i.Val.(*Session))
	}
	return sessions
}

func (c *sessionController) remove(sn *Session) {
	c.sessions.Remove(sn.Id.Token)

	if t, found := c.sessionsByService.Get(sn.Service.Id); found {
		sessionsForService := t.(cmap.ConcurrentMap)
		sessionsForService.Remove(sn.Id.Token)
		if sessionsForService.Count() < 1 {
			c.sessionsByService.Remove(sn.Service.Id)
		}
	}
}
