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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/orcaman/concurrent-map"
	"go.etcd.io/bbolt"
)

type Service struct {
	Id              string
	Binding         string
	EndpointAddress string
	Egress          string
	PeerData        map[uint32][]byte
}

func (entity *Service) toBolt() *db.Service {
	return &db.Service{
		Id:              entity.Id,
		Binding:         entity.Binding,
		EndpointAddress: entity.EndpointAddress,
		Egress:          entity.Egress,
		PeerData:        entity.PeerData,
	}
}

type serviceController struct {
	cache  cmap.ConcurrentMap
	db     *db.Db
	stores *db.Stores
	store  db.ServiceStore
}

func newServiceController(db *db.Db, stores *db.Stores) *serviceController {
	return &serviceController{
		cache:  cmap.New(),
		db:     db,
		stores: stores,
		store:  stores.Service,
	}
}

func (c *serviceController) create(s *Service) error {
	err := c.db.Update(func(tx *bbolt.Tx) error {
		return c.store.Create(boltz.NewMutateContext(tx), s.toBolt())
	})
	if err != nil {
		return err
	}
	c.cache.Set(s.Id, s)
	return nil
}

func (c *serviceController) update(s *Service) error {
	err := c.db.Update(func(tx *bbolt.Tx) error {
		return c.store.Update(boltz.NewMutateContext(tx), s.toBolt(), nil)
	})
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
	var svc *Service
	err := c.db.View(func(tx *bbolt.Tx) error {
		boltSvc, err := c.store.LoadOneById(tx, id)
		if err != nil {
			return err
		}
		if boltSvc == nil {
			return fmt.Errorf("missing service '%s'", id)
		}
		svc = c.fromBolt(boltSvc)
		return nil
	})
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
	var services []*Service
	err := c.db.View(func(tx *bbolt.Tx) error {
		ids, _, err := c.store.QueryIds(tx, "true")
		if err != nil {
			return err
		}
		for _, id := range ids {
			service, err := c.store.LoadOneById(tx, id)
			if err != nil {
				return err
			}
			services = append(services, c.fromBolt(service))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return services, nil
}

func (c *serviceController) remove(id string) error {
	c.cache.Remove(id)
	return c.db.Update(func(tx *bbolt.Tx) error {
		return c.store.DeleteById(boltz.NewMutateContext(tx), id)
	})
}

func (c *serviceController) RemoveFromCache(id string) {
	c.cache.Remove(id)
}

func (c *serviceController) fromBolt(entity *db.Service) *Service {
	return &Service{
		Id:              entity.Id,
		Binding:         entity.Binding,
		EndpointAddress: entity.EndpointAddress,
		Egress:          entity.Egress,
		PeerData:        entity.PeerData,
	}
}
