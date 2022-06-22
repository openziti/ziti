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
	"encoding/json"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_model"
	"strings"
)

const EntityNameInspect = "inspections"

func MapInspectResultToRestModel(inspectResult *network.InspectResult) *rest_model.InspectResponse {
	resp := &rest_model.InspectResponse{
		Errors:  inspectResult.Errors,
		Success: &inspectResult.Success,
	}

	for _, val := range inspectResult.Results {
		var emitVal interface{}
		if strings.HasPrefix(val.Name, "metrics") {
			cmd := strings.Split(val.Name, ":")
			format := "json"

			if len(cmd) > 1 {
				format = cmd[1]
			}

			emitVal, _ = NewMetricsModelMapper(format, true).MapInspectResultValueToMetricsResult(val)
		} else {
			if strings.HasPrefix(val.Value, "{") {
				mapVal := map[string]interface{}{}
				if err := json.Unmarshal([]byte(val.Value), &mapVal); err == nil {
					emitVal = mapVal
				}
			} else if strings.HasPrefix(val.Value, "[") {
				var arrayVal []interface{}
				if err := json.Unmarshal([]byte(val.Value), &arrayVal); err == nil {
					emitVal = arrayVal
				}
			}
		}

		if emitVal == nil {
			emitVal = val.Value
		}

		resp.Values = append(resp.Values, &rest_model.InspectResponseValue{
			AppID: &val.AppId,
			Name:  &val.Name,
			Value: emitVal,
		})
	}
	return resp
}
