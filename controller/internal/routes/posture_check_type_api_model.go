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
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
)

const EntityNamePostureCheckType = "posture-check-types"

var PostureCheckTypeLinkFactory = NewBasicLinkFactory(EntityNamePostureCheckType)

func MapPostureCheckTypeToRestEntity(_ *env.AppEnv, _ *response.RequestContext, postureCheckType *model.PostureCheckType) (interface{}, error) {
	return MapPostureCheckTypeToRestModel(postureCheckType), nil
}

func MapPostureCheckTypeToRestModel(postureCheckType *model.PostureCheckType) *rest_model.PostureCheckTypeDetail {
	operatingSystems := []*rest_model.OperatingSystem{}

	for _, os := range postureCheckType.OperatingSystems {
		osType := rest_model.OsType(os.OsType)

		newOs := &rest_model.OperatingSystem{
			Type:     &osType,
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
