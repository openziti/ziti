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
	"github.com/netfoundry/ziti-edge/edge/controller/env"
	"github.com/netfoundry/ziti-edge/edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/edge/controller/response"
	"github.com/netfoundry/ziti-edge/edge/migration"

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

	v := reflect.ValueOf(ae.Stores).Elem()

	//todo: this used to remove updb/cert stores, not necessary anymore?
	filter := map[string]bool{}

	for i := 0; i < v.NumField(); i++ {
		is := v.Field(i).Interface()

		if store, ok := is.(migration.Store); ok {
			stats, err := store.BaseStatsList(&migration.QueryOptions{})
			if err != nil {
				rc.RequestResponder.RespondWithError(err)
				return
			}

			if filter[store.PluralEntityName()] {
				continue
			}

			_, err = data.SetP(stats.Count, store.PluralEntityName())

			if err != nil {
				rc.RequestResponder.RespondWithError(err)
				return
			}
		}
	}

	rc.RequestResponder.RespondWithOk(data.Data(), nil)
}
