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

const EntityNamePostureCheckType = "posture-check-types"

var PostureCheckTypeLinkFactory = NewBasicLinkFactory(EntityNamePostureCheckType)

func MapPostureCheckTypeToRestEntity(_ *env.AppEnv, _ *response.RequestContext, postureCheckTypeModel models.Entity) (interface{}, error) {
	postureCheckType, ok := postureCheckTypeModel.(*model.PostureCheckType)

	if !ok {
		err := fmt.Errorf("entity is not a posture check type \"%s\"", postureCheckTypeModel.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel := MapPostureCheckTypeToRestModel(postureCheckType)

	return restModel, nil
}

func MapPostureCheckTypeToRestModel(postureCheckType *model.PostureCheckType) *rest_model.PostureCheckTypeDetail {

	operatingSystems := []*rest_model.OperatingSystem{}

	for _, os := range postureCheckType.OperatingSystems {

		newOs := &rest_model.OperatingSystem{
			Type:     rest_model.OsType(os.OsType),
			Versions: os.OsVersions,
		}
		operatingSystems = append(operatingSystems, newOs)
	}

	ret := &rest_model.PostureCheckTypeDetail{
		BaseEntity:       BaseEntityToRestModel(postureCheckType, PostureCheckTypeLinkFactory),
		Name:             &postureCheckType.Name,
		OperatingSystems: operatingSystems,
	}
	return ret
}
