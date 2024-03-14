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

const EntityNameController = "controllers"

var ControllerLinkFactory = NewBasicLinkFactory(EntityNameController)

func MapControllerToRestEntity(_ *env.AppEnv, _ *response.RequestContext, Controller *model.Controller) (interface{}, error) {
	return MapControllerToRestModel(Controller)
}

func MapControllerToRestModel(controller *model.Controller) (*rest_model.ControllerDetail, error) {
	ret := &rest_model.ControllerDetail{
		BaseEntity:  BaseEntityToRestModel(controller, ControllerLinkFactory),
		Name:        &controller.Name,
		Address:     &controller.Address,
		CertPem:     &controller.CertPem,
		Fingerprint: &controller.Fingerprint,
		IsOnline:    &controller.IsOnline,
	}

	if controller.LastJoinedAt != nil {
		ret.LastJoinedAt = toStrFmtDateTimeP(*controller.LastJoinedAt)
	}

	return ret, nil
}
