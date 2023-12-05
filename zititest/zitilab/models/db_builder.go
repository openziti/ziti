package models

import (
	"fmt"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
)

type ZitiDbBuilderStrategy interface {
	GetDbFile(m *model.Model) string
	GetSite(router *db.EdgeRouter) (string, bool)
	PostProcess(router *db.EdgeRouter, c *model.Component)
	ProcessDbModel(tx *bbolt.Tx, m *model.Model, builder *ZitiDbBuilder) error
}

type ZitiDbBuilder struct {
	Strategy ZitiDbBuilderStrategy
	zitiDb   boltz.Db
	stores   *db.Stores
}

func (self *ZitiDbBuilder) GetDb() boltz.Db {
	return self.zitiDb
}

func (self *ZitiDbBuilder) GetStores() *db.Stores {
	return self.stores
}

func (self *ZitiDbBuilder) GetManagers() *network.Managers {
	panic("should not be needed")
}

func (self *ZitiDbBuilder) Build(m *model.Model) error {
	dbFile := self.Strategy.GetDbFile(m)

	var err error
	self.zitiDb, err = db.Open(dbFile)
	if err != nil {
		return errors.Wrapf(err, "unable to open ziti bbolt db [%v]", dbFile)
	}

	defer func() {
		if err = self.zitiDb.Close(); err != nil {
			panic(err)
		}
	}()

	self.stores, err = db.InitStores(self.zitiDb)
	if err != nil {
		return errors.Wrapf(err, "unable to init fabric stores using db [%v]", dbFile)
	}

	return self.zitiDb.View(func(tx *bbolt.Tx) error {
		return self.Strategy.ProcessDbModel(tx, m, self)
	})
}

func (self *ZitiDbBuilder) CreateEdgeRouterHosts(tx *bbolt.Tx, m *model.Model) error {
	ids, _, err := self.stores.EdgeRouter.QueryIds(tx, "true limit none")
	if err != nil {
		return err
	}

	for _, id := range ids {
		er, err := self.stores.EdgeRouter.LoadOneById(tx, id)
		if err != nil {
			return err
		}

		if site, useEdgeRouter := self.Strategy.GetSite(er); useEdgeRouter {
			regionId := site[:len(site)-1]

			var region *model.Region
			for _, r := range m.Regions {
				if r.Site == site {
					region = r
					break
				}
			}

			if region == nil {
				if _, found := m.Regions[site]; found {
					return errors.Errorf("trying to add region for site %v, but one exists, with different site", site)
				}
				region = &model.Region{
					Scope:  model.Scope{Tags: model.Tags{}},
					Region: regionId,
					Site:   site,
					Hosts:  model.Hosts{},
				}
				m.Regions[site] = region
			}

			host := &model.Host{
				Scope:      model.Scope{Tags: model.Tags{}},
				Region:     region,
				Components: model.Components{},
			}
			id = strings.ReplaceAll(er.Id, ".", "_")
			region.Hosts["router_"+id] = host

			component := &model.Component{
				Scope: model.Scope{Tags: model.Tags{}},
				Type:  &zitilab.RouterType{},
				Host:  host,
			}

			host.Components[er.Id] = component
			self.Strategy.PostProcess(er, component)
		}
	}
	return nil
}

func (self *ZitiDbBuilder) DefaultGetSite(er *db.EdgeRouter) (string, bool) {
	if val, found := er.Tags["fablab.site"]; found {
		return fmt.Sprintf("%v", val), true
	}
	return "", false
}
