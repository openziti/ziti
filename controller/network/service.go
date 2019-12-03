/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/orcaman/concurrent-map"
)

type Service struct {
	Id              string
	Binding         string
	EndpointAddress string
	Egress          string
}

type serviceController struct {
	cache cmap.ConcurrentMap
	store ServiceStore
}

func newServiceController(db *db.Db) (*serviceController, error) {
	return &serviceController{cache: cmap.New(), store: NewServiceStore(db)}, nil
}

func (c *serviceController) create(s *Service) error {
	err := c.store.create(s)
	if err != nil {
		return err
	}
	c.cache.Set(s.Id, s)
	return nil
}

func (c *serviceController) update(s *Service) error {
	err := c.store.update(s)
	if err != nil {
		return err
	}
	c.cache.Set(s.Id, s)
	return nil
}

func (c *serviceController) get(id string) (*Service, bool) {
	if t, found := c.cache.Get(id); found {
		return t.(*Service), true

	}
	svc, err := c.store.loadOneById(id)
	if err != nil {
		pfxlog.Logger().Errorf("failed loading service (%s)", err)
		return nil, false
	}
	if svc != nil {
		c.cache.Set(svc.Id, svc)
		return svc, true
	}
	return nil, false
}

func (c *serviceController) all() ([]*Service, error) {
	svcs, err := c.store.all()
	if err != nil {
		return nil, err
	}
	return svcs, nil
}

func (c *serviceController) remove(id string) error {
	c.cache.Remove(id)
	return c.store.remove(id)
}

func (c *serviceController) RemoveFromCache(id string) {
	c.cache.Remove(id)
}