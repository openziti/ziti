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
package mgmt

import (
	"context"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
	log "github.com/sirupsen/logrus"
	"time"
)

func IdentityFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.IdentityDetail {
	params := &identity.ListIdentitiesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(5 * time.Second)
	resp, err := client.Identity.ListIdentities(params, nil)
	if err != nil {
		log.Debugf("Could not obtain an ID for the identity with filter %s: %v", filter, err)
		return nil
	}

	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func ServiceFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.ServiceDetail {
	params := &service.ListServicesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(5 * time.Second)
	resp, err := client.Service.ListServices(params, nil)
	if err != nil {
		log.Debugf("Could not obtain an ID for the service with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func ServicePolicyFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.ServicePolicyDetail {
	params := &service_policy.ListServicePoliciesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(5 * time.Second)
	resp, err := client.ServicePolicy.ListServicePolicies(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the service with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func NameFilter(name string) string {
	return `name="` + name + `"`
}