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
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
	"net/http"
)

func init() {
	r := NewRootRouter()
	env.AddRouter(r)
}

type RootRouter struct {
	BasePath string
}

func NewRootRouter() *RootRouter {
	return &RootRouter{
		BasePath: "/",
	}
}

func (ir *RootRouter) Register(ae *env.AppEnv) {

	listHandler := ae.WrapHandler(ir.List, permissions.Always())

	ae.RootRouter.HandleFunc("", listHandler).Methods("GET")
	ae.RootRouter.HandleFunc("/", listHandler).Methods("GET")
}

func (ir *RootRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	data := gabs.New()

	authLinks := &response.Links{
		"certificate": &response.Link{
			Href:   "./authenticate?method=cert",
			Method: http.MethodPost,
		},
		"password": &response.Link{
			Href:   "./authenticate?method=password",
			Method: http.MethodPost,
		},
	}

	nlnks := map[string]*response.Links{
		"authenticate":     authLinks,
		"app-wans":         newCrudLinks("./app-wans", "<appwanId>"),
		"cas":              newCrudLinks("./cas", "<caId>"),
		"clusters":         newCrudLinks("./clusters", "<clusterId>"),
		"fabrics":          newCrudLinks("./fabrics", "<fabricId>"),
		"fabric-types":     newReadOnlyLinks("./fabric-types", "<fabricTypeId>"),
		"event-logs":       newReadOnlyLinks("./event-logs", "<eventLogId>"),
		"edge-routers":         newCrudLinks("./edge-routers", "<edgeRouterId>"),
		"geo-regions":      newReadOnlyLinks("./geo-regions", "<geoRegionId>"),
		"identities":       newCrudLinks("./identities", "<identityId>"),
		"identity-types":   newReadOnlyLinks("./identity-types", "<identityTypeId>"),
		"network-sessions": newCrudLinks("./network-sessions", "<networkSessionId>"),
		"protocols":        newReadOnlyLinks("./protocols", "<protocolId>"),
		"services":         newCrudLinks("./services", "<serviceId>"),
		"sessions":         newCrudLinks("./session", "<sessionId>"),
		"summary":          newListOnlyLinks("./summary"),
		"version":          newListOnlyLinks("./version"),
	}

	for n, lnks := range nlnks {
		if _, err := data.SetP(lnks, n); err != nil {
			pfxlog.Logger().WithField("cause", err).Error("could not set value by path")
		}
	}

	rc.RequestResponder.RespondWithOk(data.Data(), nil)
}

func newListOnlyLinks(baseUrl string) *response.Links {
	return &response.Links{
		"list": &response.Link{
			Href:   baseUrl,
			Method: http.MethodGet,
		},
	}
}

func newCrudLinks(baseUrl, urlIdProp string) *response.Links {
	idUrl := fmt.Sprintf("%s/%s", baseUrl, urlIdProp)
	return &response.Links{
		"create": &response.Link{
			Href:   baseUrl,
			Method: http.MethodGet,
		},
		"delete": &response.Link{
			Href:   idUrl,
			Method: http.MethodDelete,
		},
		"update": &response.Link{
			Href:   idUrl,
			Method: http.MethodPost,
		},
		"patch": &response.Link{
			Href:   idUrl,
			Method: http.MethodPatch,
		},
		"list": &response.Link{
			Href:   baseUrl,
			Method: http.MethodGet,
		},
		"detail": &response.Link{
			Href:   idUrl,
			Method: http.MethodGet,
		},
	}
}

func newReadOnlyLinks(baseUrl, urlIdProp string) *response.Links {
	idUrl := fmt.Sprintf("%s/%s", baseUrl, urlIdProp)
	return &response.Links{
		"list": &response.Link{
			Href:   baseUrl,
			Method: http.MethodGet,
		},
		"detail": &response.Link{
			Href:   idUrl,
			Method: http.MethodGet,
		},
	}
}
