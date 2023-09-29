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

package api

import (
	"encoding/json"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/controller/fields"
)

func GetFields(body []byte) (fields.UpdatedFields, error) {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal(body, &jsonMap)

	if err != nil {
		return nil, apierror.GetJsonParseError(err, body)
	}

	resultMap := fields.UpdatedFieldsMap{}
	GetJsonFields("", jsonMap, resultMap)
	return resultMap, nil
}

func GetJsonFields(prefix string, m map[string]interface{}, result fields.UpdatedFieldsMap) {
	for k, v := range m {
		name := k
		if subMap, ok := v.(map[string]interface{}); ok {
			GetJsonFields(prefix+name+".", subMap, result)
		} else if v != nil {
			result[prefix+name] = struct{}{}
		}
	}
}
