/*
	Copyright NetFoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
)

const EntityNameGeoRegion = "geo-regions"

var GeoRegionLinkFactory = NewBasicLinkFactory(EntityNameGeoRegion)

func MapGeoRegionToRestEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	geoRegion, ok := e.(*model.GeoRegion)

	if !ok {
		err := fmt.Errorf("entity is not a GeoRegion \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapGeoRegionToRestModel(geoRegion)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapGeoRegionToRestModel(geoRegion *model.GeoRegion) (*rest_model.GeoRegionDetail, error) {
	ret := &rest_model.GeoRegionDetail{
		BaseEntity: BaseEntityToRestModel(geoRegion, GeoRegionLinkFactory),
		Name:       &geoRegion.Name,
	}

	return ret, nil
}
