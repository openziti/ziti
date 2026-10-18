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

package routes

import (
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/response"
	"github.com/openziti/ziti/v2/controller/rest_model"
)

const EntityNameLink = "links"

var LinkLinkFactory = NewLinkLinkFactory()

type LinkLinkFactoryIml struct {
	BasicFabricLinkFactory
}

func NewLinkLinkFactory() *LinkLinkFactoryIml {
	return &LinkLinkFactoryIml{
		BasicFabricLinkFactory: *NewBasicFabricLinkFactory(EntityNameLink),
	}
}

func (factory *LinkLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicFabricLinkFactory.Links(entity)
	return links
}

// routerRef renders a reference to one end of a link. router is nil for an end this controller cannot
// resolve, which is ordinary for a destination it has not caught up to; the reference then carries the id
// alone, since that is all that is known about it. Nil where even the id is unknown, which a link's source
// is when it holds no router: unlike the destination, a link does not record its source's id separately.
func routerRef(routerId string, router *model.Router) *rest_model.EntityRef {
	if router != nil {
		return ToFabricEntityRef(router.Name, router, FabricRouterLinkFactory)
	}
	if routerId == "" {
		return nil
	}
	return &rest_model.EntityRef{
		Entity: FabricRouterLinkFactory.EntityName(),
		ID:     routerId,
		Links: rest_model.Links{
			EntityNameSelf: FabricRouterLinkFactory.SelfLinkFromId(routerId),
		},
	}
}

func MapLinkToRestModel(ae *env.AppEnv, _ *response.RequestContext, link *model.Link) (interface{}, error) {
	n := ae.GetHostController().GetNetwork()
	iteration := int64(link.Iteration)
	staticCost := int64(link.StaticCost)
	linkStateStr := link.CurrentState().Mode.String()

	down := link.IsDown()

	destRouter := link.GetDest()
	if destRouter == nil {
		// A miss is not an error here: a link can name a destination this controller has not caught up to,
		// or one deleted since the link was recorded. The link exists and has to render either way, and
		// failing would fail the whole listing rather than this row.
		if loaded, err := n.Router.Read(link.DstId); err == nil {
			destRouter = loaded
		}
	}

	// A link records its destination's id but not its source's, so an unresolved source can only render as
	// absent.
	srcRouter := link.GetSrc()
	srcId := ""
	if srcRouter != nil {
		srcId = srcRouter.Id
	}

	ret := &rest_model.LinkDetail{
		Cost:          &link.Cost,
		DestLatency:   &link.DstLatency,
		DestRouter:    routerRef(link.DstId, destRouter),
		Down:          &down,
		ID:            &link.Id,
		SourceLatency: &link.SrcLatency,
		SourceRouter:  routerRef(srcId, srcRouter),
		State:         &linkStateStr,
		StaticCost:    &staticCost,
		Protocol:      &link.Protocol,
		Iteration:     &iteration,
	}

	if connState := link.GetConnsState(); connState != nil {
		for _, c := range connState.Conns {
			ret.Connections = append(ret.Connections, &rest_model.LinkConnection{
				Dest:   c.RemoteAddr,
				Source: c.LocalAddr,
				Type:   c.Type,
			})
		}
	}

	return ret, nil
}
