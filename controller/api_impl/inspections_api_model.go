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

package api_impl

import (
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_model"
)

const EntityNameInspect = "inspections"

func MapInspectResultToRestModel(inspectResult *network.InspectResult) *rest_model.InspectResponse {
	resp := &rest_model.InspectResponse{
		Errors:  inspectResult.Errors,
		Success: &inspectResult.Success,
	}
	for _, val := range inspectResult.Results {
		resp.Values = append(resp.Values, &rest_model.InspectResponseValue{
			AppID: &val.AppId,
			Name:  &val.Name,
			Value: &val.Value,
		})
	}
	return resp
}
