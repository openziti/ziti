/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/orcaman/concurrent-map"
)

type Router struct {
	Id                 string
	Fingerprint        string
	AdvertisedListener transport.Address
	Control            channel2.Channel
	CostFactor         int
}

func NewRouter(id, fingerprint string) *Router {
	return &Router{
		Id:          id,
		Fingerprint: fingerprint,
	}
}

func newRouter(id string, fingerprint string, advLstnr transport.Address, ctrl channel2.Channel) *Router {
	return &Router{
		Id:                 id,
		Fingerprint:        fingerprint,
		AdvertisedListener: advLstnr,
		Control:            ctrl,
		CostFactor:         1,
	}
}

type routerController struct {
	connected cmap.ConcurrentMap // map[string]*Router
}

func newRouterController() *routerController {
	return &routerController{
		connected: cmap.New(),
	}
}

func (c *routerController) add(r *Router) {
	c.connected.Set(r.Id, r)
}

func (c *routerController) has(id string) bool {
	return c.connected.Has(id)
}

func (c *routerController) get(id string) (*Router, bool) {
	if t, found := c.connected.Get(id); found {
		return t.(*Router), true
	}
	return nil, false
}

func (c *routerController) all() []*Router {
	routers := make([]*Router, 0)
	for i := range c.connected.IterBuffered() {
		routers = append(routers, i.Val.(*Router))
	}
	return routers
}

func (c *routerController) count() int {
	return c.connected.Count()
}

func (c *routerController) remove(r *Router) {
	c.connected.Remove(r.Id)
}
