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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/openziti/edge-api/rest_management_api_client"
	rest_mgmt "github.com/openziti/edge-api/rest_management_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/ziti/util"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
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

func NewClient() (*rest_management_api_client.ZitiEdgeManagement, error) {
	cachedCreds, _, loadErr := util.LoadRestClientConfig()
	if loadErr != nil {
		return nil, loadErr
	}

	cachedId := cachedCreds.EdgeIdentities[cachedCreds.Default] //only support default for now
	if cachedId == nil {
		return nil, errors.New("no identity found")
	}
	
	caPool := x509.NewCertPool()
	if _, cacertErr := os.Stat(cachedId.CaCert); cacertErr == nil {
		rootPemData, err := os.ReadFile(cachedId.CaCert)
		if err != nil {
			return nil, err
		}
		caPool.AppendCertsFromPEM(rootPemData)
	} else {
		return nil, errors.New("CA cert file not found in config file")
	}
	
	tlsConfig := &tls.Config{
		RootCAs: caPool,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	// Assign the transport to the default HTTP client
	http.DefaultClient = &http.Client{
		Transport: transport,
	}
	c, e := rest_util.NewEdgeManagementClientWithToken(http.DefaultClient, cachedId.Url, cachedId.Token)
	if e != nil {
		return nil, e
	}

	apiSessionParams := &rest_mgmt.GetCurrentAPISessionParams{
		Context: context.Background(),
	}
	_, authErr := c.CurrentAPISession.GetCurrentAPISession(apiSessionParams, nil)
	if authErr != nil {
		return nil, errors.New("client not authenticated. login with 'ziti edge login' before executing")
	}
	return c, nil
}