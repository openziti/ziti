package models

import (
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
)

type ZitiDbBuilderStrategy interface {
	GetDbFile() string
	GetSite(router *persistence.EdgeRouter) (string, bool)
	PostProcess(router *persistence.EdgeRouter, c *model.Component)
}

type ZitiDbBuilder struct {
	Strategy ZitiDbBuilderStrategy
}

type dbProvider struct {
	zitiDb boltz.Db
	stores *db.Stores
}

func (self *dbProvider) GetDb() boltz.Db {
	return self.zitiDb
}

func (self *dbProvider) GetStores() *db.Stores {
	return self.stores
}

func (self *dbProvider) GetManagers() *network.Managers {
	panic("should not be needed")
}

func (self *ZitiDbBuilder) Build(m *model.Model) error {
	dbFile := self.Strategy.GetDbFile()
	zitiDb, err := db.Open(dbFile)
	if err != nil {
		return errors.Wrapf(err, "unable to open ziti bbolt db [%v]", dbFile)
	}

	defer func() {
		if err = zitiDb.Close(); err != nil {
			panic(err)
		}
	}()

	fabricStore, err := db.InitStores(zitiDb)
	if err != nil {
		return errors.Wrapf(err, "unable to init fabric stores using db [%v]", dbFile)
	}

	provider := &dbProvider{
		zitiDb: zitiDb,
		stores: fabricStore,
	}

	edgeStores, err := persistence.NewBoltStores(provider)
	if err != nil {
		return errors.Wrapf(err, "unable to init edge stores using db [%v]", dbFile)
	}

	err = zitiDb.View(func(tx *bbolt.Tx) error {
		ids, _, err := edgeStores.EdgeRouter.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		for _, id := range ids {
			er, err := edgeStores.EdgeRouter.LoadOneById(tx, id)
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
				id := strings.ReplaceAll(er.Id, ".", "_")
				region.Hosts["router_"+id] = host

				component := &model.Component{
					Scope:      model.Scope{Tags: model.Tags{}},
					ConfigSrc:  "router.yml",
					ConfigName: fmt.Sprintf("router-%v.yml", er.Id),
					BinaryName: "ziti router",
					Host:       host,
				}

				host.Components[er.Id] = component
				self.Strategy.PostProcess(er, component)
			}
		}
		return nil
	})

	return err
}

func (self *ZitiDbBuilder) DefaultGetSite(er *persistence.EdgeRouter) (string, bool) {
	if val, found := er.Tags["fablab.site"]; found {
		return fmt.Sprintf("%v", val), true
	}
	return "", false
}
