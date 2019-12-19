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

package routes

import (
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/response"
	"go.etcd.io/bbolt"

	"github.com/Jeffail/gabs"
	"reflect"
)

func init() {
	r := NewSummaryRouter()
	env.AddRouter(r)
}

type SummaryRouter struct {
	BasePath string
}

func NewSummaryRouter() *SummaryRouter {
	return &SummaryRouter{
		BasePath: "/summary",
	}
}

func (ir *SummaryRouter) Register(ae *env.AppEnv) {

	listHandler := ae.WrapHandler(ir.List, permissions.Always())

	ae.RootRouter.HandleFunc(ir.BasePath, listHandler).Methods("GET")
	ae.RootRouter.HandleFunc(ir.BasePath+"/", listHandler).Methods("GET")
}

func (ir *SummaryRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	data := gabs.New()

	v := reflect.ValueOf(ae.BoltStores).Elem()

	err := ae.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for i := 0; i < v.NumField(); i++ {

			field := v.Field(i)

			if !field.CanInterface() {
				continue
			}

			is := field.Interface()

			if store, ok := is.(persistence.Store); ok {
				_, count, err := store.QueryIds(tx, "true limit 1")
				if err != nil {
					return err
				}

				if _, err = data.SetP(count, store.GetEntityType()); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
	} else {
		rc.RequestResponder.RespondWithOk(data.Data(), nil)
	}
}
