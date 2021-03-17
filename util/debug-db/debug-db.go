package main

import (
	"fmt"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("must specify path to bbolt database")
		os.Exit(1)
	}

	run(os.Args[1])
}

func noError(err error) {
	if err != nil {
		panic(err)
	}
}

type dbProvider struct {
	db          boltz.Db
	stores      *db.Stores
	controllers *network.Controllers
}

func (provider *dbProvider) GetDb() boltz.Db {
	return provider.db
}

func (provider *dbProvider) GetServiceCache() network.Cache {
	panic("implement me")
}

func (provider *dbProvider) GetStores() *db.Stores {
	return provider.stores
}

func (provider *dbProvider) GetControllers() *network.Controllers {
	return provider.controllers
}

func run(dbFile string) {
	boltDb, err := db.Open(dbFile, false)
	noError(err)

	fabricStores, err := db.InitStores(boltDb)
	noError(err)

	controllers := network.NewControllers(boltDb, fabricStores)

	dbProvider := &dbProvider{
		db:          boltDb,
		stores:      fabricStores,
		controllers: controllers,
	}

	stores, err := persistence.NewBoltStores(dbProvider)
	noError(err)

	id := "7dbd3fc9-e4c8-489a-ab8f-4bbb3d768f57"
	err = dbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
		identity, _ := stores.Identity.LoadOneById(tx, id)
		if identity == nil {
			identity = &persistence.Identity{
				BaseExtEntity:  boltz.BaseExtEntity{Id: id},
				Name:           "DebugAdmin",
				IdentityTypeId: "577104f2-1e3a-4947-a927-7383baefbc9a",
				IsDefaultAdmin: false,
				IsAdmin:        true,
			}
			ctx := boltz.NewMutateContext(tx)
			if err = stores.Identity.Create(ctx, identity); err != nil {
				return err
			}

			authHandler := model.AuthenticatorHandler{}
			result := authHandler.HashPassword("admin")
			authenticator := &persistence.AuthenticatorUpdb{
				Authenticator: persistence.Authenticator{
					BaseExtEntity: boltz.BaseExtEntity{
						Id: eid.New(),
					},
					Type:       "updb",
					IdentityId: id,
				},
				Username: "admin",
				Password: result.Password,
				Salt:     result.Salt,
			}
			authenticator.SubType = authenticator

			if err = stores.Authenticator.Create(ctx, authenticator); err != nil {
				return err
			}
		}

		return nil
	})
	noError(err)
}
