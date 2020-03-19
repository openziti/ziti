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
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/orcaman/concurrent-map"
)

type session struct {
	Id         *identity.TokenId
	ClientId   *identity.TokenId
	Service    *Service
	Terminator *Terminator
	Circuit    *Circuit
}

func (s *session) latency() int64 {
	var latency int64
	for _, l := range s.Circuit.Links {
		latency += l.SrcLatency
		latency += l.DstLatency
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

func (c *sessionController) add(sn *session) {
	c.sessions.Set(sn.Id.Token, sn)

	if !c.sessionsByService.Has(sn.Service.Id) {
		c.sessionsByService.Set(sn.Service.Id, cmap.New())
	}
	t, _ := c.sessionsByService.Get(sn.Service.Id)
	sessionsForService := t.(cmap.ConcurrentMap)
	sessionsForService.Set(sn.Id.Token, sn)
}

func (c *sessionController) get(id *identity.TokenId) (*session, bool) {
	if t, found := c.sessions.Get(id.Token); found {
		return t.(*session), true
	}
	return nil, false
}

func (c *sessionController) all() []*session {
	sessions := make([]*session, 0)
	for i := range c.sessions.IterBuffered() {
		sessions = append(sessions, i.Val.(*session))
	}
	return sessions
}

func (c *sessionController) remove(sn *session) {
	c.sessions.Remove(sn.Id.Token)

	if t, found := c.sessionsByService.Get(sn.Service.Id); found {
		sessionsForService := t.(cmap.ConcurrentMap)
		sessionsForService.Remove(sn.Id.Token)
		if sessionsForService.Count() < 1 {
			c.sessionsByService.Remove(sn.Service.Id)
		}
	}
}
