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

package api_impl

import (
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/network"

	"github.com/openziti/fabric/controller/rest_model"

	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/v2/stringz"
)

const EntityNameRouter = "routers"

var RouterLinkFactory = NewRouterLinkFactory()

type RouterLinkFactoryIml struct {
	BasicLinkFactory
}

func NewRouterLinkFactory() *RouterLinkFactoryIml {
	return &RouterLinkFactoryIml{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameRouter),
	}
}

func (factory *RouterLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	return links
}

func MapCreateRouterToModel(router *rest_model.RouterCreate) *network.Router {
	ret := &network.Router{
		BaseEntity: models.BaseEntity{
			Id:   stringz.OrEmpty(router.ID),
			Tags: TagsOrDefault(router.Tags),
		},
		Name:        stringz.OrEmpty(router.Name),
		Fingerprint: router.Fingerprint,
		Cost:        uint16(Int64OrDefault(router.Cost)),
		NoTraversal: BoolOrDefault(router.NoTraversal),
		Disabled:    BoolOrDefault(router.Disabled),
	}

	return ret
}

func MapUpdateRouterToModel(id string, router *rest_model.RouterUpdate) *network.Router {
	ret := &network.Router{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(router.Tags),
			Id:   id,
		},
		Name:        stringz.OrEmpty(router.Name),
		Fingerprint: router.Fingerprint,
		Cost:        uint16(Int64OrDefault(router.Cost)),
		NoTraversal: BoolOrDefault(router.NoTraversal),
		Disabled:    BoolOrDefault(router.Disabled),
	}

	return ret
}

func MapPatchRouterToModel(id string, router *rest_model.RouterPatch) *network.Router {
	ret := &network.Router{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(router.Tags),
			Id:   id,
		},
		Name:        router.Name,
		Fingerprint: router.Fingerprint,
		Cost:        uint16(Int64OrDefault(router.Cost)),
		NoTraversal: BoolOrDefault(router.NoTraversal),
		Disabled:    BoolOrDefault(router.Disabled),
	}

	return ret
}

type RouterModelMapper struct{}

func (RouterModelMapper) ToApi(n *network.Network, _ api.RequestContext, router *network.Router) (interface{}, error) {
	connected := n.GetConnectedRouter(router.Id)
	var restVersionInfo *rest_model.VersionInfo
	if connected != nil && connected.VersionInfo != nil {
		versionInfo := connected.VersionInfo
		restVersionInfo = &rest_model.VersionInfo{
			Arch:      versionInfo.Arch,
			BuildDate: versionInfo.BuildDate,
			Os:        versionInfo.OS,
			Revision:  versionInfo.Revision,
			Version:   versionInfo.Version,
		}
	}

	isConnected := connected != nil
	cost := int64(router.Cost)
	ret := &rest_model.RouterDetail{
		BaseEntity:  BaseEntityToRestModel(router, RouterLinkFactory),
		Fingerprint: router.Fingerprint,
		Name:        &router.Name,
		Connected:   &isConnected,
		VersionInfo: restVersionInfo,
		Cost:        &cost,
		NoTraversal: &router.NoTraversal,
		Disabled:    &router.Disabled,
	}

	if connected != nil {
		for _, listener := range connected.Listeners {
			advAddr := listener.GetAddress()
			linkProtocol := listener.GetProtocol()
			ret.ListenerAddresses = append(ret.ListenerAddresses, &rest_model.RouterListener{
				Address:  &advAddr,
				Protocol: &linkProtocol,
			})
		}
	}

	return ret, nil
}
