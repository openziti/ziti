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
	"github.com/openziti/fabric/rest_model"
)

const EntityNameLink = "links"

var LinkLinkFactory = NewLinkLinkFactory()

type LinkLinkFactoryIml struct {
	BasicLinkFactory
}

func NewLinkLinkFactory() *LinkLinkFactoryIml {
	return &LinkLinkFactoryIml{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameLink),
	}
}

func (factory *LinkLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	return links
}

func MapLinkToRestModel(_ *network.Network, _ api.RequestContext, link *network.Link) (*rest_model.LinkDetail, error) {
	staticCost := int64(link.StaticCost)
	linkState := link.CurrentState()
	linkStateStr := ""
	if linkState != nil {
		linkStateStr = linkState.Mode.String()
	}

	down := link.IsDown()

	ret := &rest_model.LinkDetail{
		Cost:          &link.Cost,
		DestLatency:   &link.DstLatency,
		DestRouter:    ToEntityRef(link.Dst.Name, link.Dst, RouterLinkFactory),
		Down:          &down,
		ID:            &link.Id,
		SourceLatency: &link.SrcLatency,
		SourceRouter:  ToEntityRef(link.Src.Name, link.Src, RouterLinkFactory),
		State:         &linkStateStr,
		StaticCost:    &staticCost,
		Protocol:      &link.Protocol,
	}
	return ret, nil
}
