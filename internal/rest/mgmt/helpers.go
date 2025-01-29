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
	"fmt"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_management_api_client/posture_checks"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_edge_router_policy"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/internal/rest/consts"
	log "github.com/sirupsen/logrus"
)

func IdentityFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.IdentityDetail {
	params := &identity.ListIdentitiesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
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
	params.SetTimeout(internal_consts.DefaultTimeout)
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
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.ServicePolicy.ListServicePolicies(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the service policy with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func AuthPolicyFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.AuthPolicyDetail {
	params := &auth_policy.ListAuthPoliciesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.AuthPolicy.ListAuthPolicies(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the auth policy with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func CertificateAuthorityFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.CaDetail {
	params := &certificate_authority.ListCasParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.CertificateAuthority.ListCas(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the certificate authority with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func ConfigTypeFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.ConfigTypeDetail {
	params := &config.ListConfigTypesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.Config.ListConfigTypes(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the config type with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func ConfigFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.ConfigDetail {
	params := &config.ListConfigsParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.Config.ListConfigs(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the config with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func ExternalJWTSignerFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.ExternalJWTSignerDetail {
	params := &external_jwt_signer.ListExternalJWTSignersParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.ExternalJWTSigner.ListExternalJWTSigners(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the external jwt signer with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func PostureCheckFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.PostureCheckDetail {
	params := &posture_checks.ListPostureChecksParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.PostureChecks.ListPostureChecks(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the posture check with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || len(resp.Payload.Data()) == 0 {
		return nil
	}
	return &resp.Payload.Data()[0]
}

func EdgeRouterPolicyFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.EdgeRouterPolicyDetail {
	params := &edge_router_policy.ListEdgeRouterPoliciesParams{
		Filter: &filter,
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.EdgeRouterPolicy.ListEdgeRouterPolicies(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the edge router policies with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func EdgeRouterFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.EdgeRouterDetail {
	params := &edge_router.ListEdgeRoutersParams{
		Filter: &filter,
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.EdgeRouter.ListEdgeRouters(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the edge routers with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func ServiceEdgeRouterPolicyFromFilter(client *rest_management_api_client.ZitiEdgeManagement, filter string) *rest_model.ServiceEdgeRouterPolicyDetail {
	params := &service_edge_router_policy.ListServiceEdgeRouterPoliciesParams{
		Filter: &filter,
	}
	params.SetTimeout(internal_consts.DefaultTimeout)
	resp, err := client.ServiceEdgeRouterPolicy.ListServiceEdgeRouterPolicies(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the ServiceEdgeRouterPolicy routers with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}

func NameFilter(name string) string {
	return fmt.Sprintf("name = \"%s\"", name)
}
