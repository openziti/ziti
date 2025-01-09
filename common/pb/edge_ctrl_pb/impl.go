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

package edge_ctrl_pb

import (
	"fmt"
)

func (x *DataState_Identity) GetServiceConfigsAsMap() map[string]map[string]string {
	if x.ServiceConfigs == nil {
		return nil
	}

	result := map[string]map[string]string{}
	for k, v := range x.ServiceConfigs {
		m := map[string]string{}
		for configType, configId := range v.Configs {
			m[configType] = configId
		}
		result[k] = m
	}

	return result
}

func (request *RouterDataModelValidateRequest) GetContentType() int32 {
	return int32(ContentType_ValidateDataStateRequestType)
}

func (request *RouterDataModelValidateResponse) GetContentType() int32 {
	return int32(ContentType_ValidateDataStateResponseType)
}

func (diff *RouterDataModelDiff) ToDetail() string {
	return fmt.Sprintf("%s id: %s %s: %s", diff.EntityType, diff.EntityId, diff.DiffType, diff.Detail)
}

func (x *DataState_Event) Summarize() string {
	switch v := x.Model.(type) {
	case *DataState_Event_Config:
		return fmt.Sprintf("%s config %s", x.Action.String(), v.Config.Id)
	case *DataState_Event_ConfigType:
		return fmt.Sprintf("%s config-type %s", x.Action.String(), v.ConfigType.Id)
	case *DataState_Event_Identity:
		return fmt.Sprintf("%s identity %s", x.Action.String(), v.Identity.Id)
	case *DataState_Event_Service:
		return fmt.Sprintf("%s service %s", x.Action.String(), v.Service.Id)
	case *DataState_Event_ServicePolicy:
		return fmt.Sprintf("%s service-policy %s", x.Action.String(), v.ServicePolicy.Id)
	case *DataState_Event_PostureCheck:
		return fmt.Sprintf("%s posture-check %s", x.Action.String(), v.PostureCheck.Id)
	case *DataState_Event_PublicKey:
		return fmt.Sprintf("%s public-key %s", x.Action.String(), v.PublicKey.Kid)
	case *DataState_Event_Revocation:
		return fmt.Sprintf("%s revocation %s", x.Action.String(), v.Revocation.Id)
	case *DataState_Event_ServicePolicyChange:
		return fmt.Sprintf("%s service-policy-change %s add: %v, type: %s",
			x.Action.String(), v.ServicePolicyChange.PolicyId, v.ServicePolicyChange.Add, v.ServicePolicyChange.RelatedEntityType.String())

	default:
		return fmt.Sprintf("%s %T", x.Action.String(), x.Model)
	}
}
