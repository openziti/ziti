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

package database

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"github.com/spf13/cobra"
)

func NewAddDebugAdminAction() *cobra.Command {
	action := &addDebugAdminAction{}
	return &cobra.Command{
		Use:   "add-debug-admin </path/to/ziti-controller.db.file> <username> <password>",
		Short: "Adds an admin user to the given database file for debugging purposes",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			action.run(args[0], args[1], args[2])
		},
	}
}

type addDebugAdminAction struct {
	db       boltz.Db
	stores   *db.Stores
	managers *network.Managers
}

func (action *addDebugAdminAction) GetDb() boltz.Db {
	return action.db
}

func (action *addDebugAdminAction) GetStores() *db.Stores {
	return action.stores
}

func (action *addDebugAdminAction) GetManagers() *network.Managers {
	return action.managers
}

func (action *addDebugAdminAction) noError(err error) {
	if err != nil {
		panic(err)
	}
}

func (action *addDebugAdminAction) run(dbFile, username, password string) {
	boltDb, err := db.Open(dbFile)
	action.noError(err)

	fabricStores, err := db.InitStores(boltDb)
	action.noError(err)

	dispatcher := &command.LocalDispatcher{
		EncodeDecodeCommands: false,
	}
	controllers := network.NewManagers(nil, dispatcher, boltDb, fabricStores, nil)

	dbProvider := &addDebugAdminAction{
		db:       boltDb,
		stores:   fabricStores,
		managers: controllers,
	}

	stores, err := db.InitStores(boltDb)
	action.noError(err)

	id := "debug-admin"
	name := fmt.Sprintf("debug admin (%v)", uuid.NewString())
	ctx := change.New().SetChangeAuthorType("cli.debug-db").NewMutateContext()
	err = dbProvider.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		tx := ctx.Tx()
		identity, _ := stores.Identity.LoadOneById(tx, id)
		if identity != nil {
			if err = stores.Identity.DeleteById(ctx, id); err != nil {
				return err
			}
			fmt.Printf("removing existing identity with id '%v'\n", id)
		}

		identity = &db.Identity{
			BaseExtEntity:  boltz.BaseExtEntity{Id: id},
			Name:           name,
			IdentityTypeId: db.DefaultIdentityType,
			IsDefaultAdmin: false,
			IsAdmin:        true,
		}
		if err = stores.Identity.Create(ctx, identity); err != nil {
			return err
		}

		authHandler := model.AuthenticatorManager{}
		result := authHandler.HashPassword(password)
		authenticator := &db.AuthenticatorUpdb{
			Authenticator: db.Authenticator{
				BaseExtEntity: boltz.BaseExtEntity{
					Id: eid.New(),
				},
				Type:       "updb",
				IdentityId: id,
			},
			Username: username,
			Password: result.Password,
			Salt:     result.Salt,
		}
		authenticator.SubType = authenticator

		if err = stores.Authenticator.Create(ctx, &authenticator.Authenticator); err != nil {
			return err
		}

		fmt.Printf("added debug admin with username '%v'\n", username)
		return nil
	})
	action.noError(err)
}
