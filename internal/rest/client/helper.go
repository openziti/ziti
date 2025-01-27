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

package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_client"
	"github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_client_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	internalconsts "github.com/openziti/ziti/internal/rest/consts"
	"github.com/openziti/ziti/ziti/util"
	"net/http"
	"os"
)

func NewClientApiClient() (*rest_client_api_client.ZitiEdgeClient, error) {
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
		pfxlog.Logger().Warn("CA cert file not found in config file? Trying to authenticate with provided params")
	}

	tlsConfig := &tls.Config{
		RootCAs: caPool,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	http.DefaultClient = &http.Client{
		Transport: transport,
	}
	c, e := rest_util.NewEdgeClientClientWithToken(http.DefaultClient, cachedId.Url, cachedId.Token)
	if e != nil {
		return nil, e
	}

	apiSessionParams := current_api_session.NewGetCurrentAPISessionParams()
	_, authErr := c.CurrentAPISession.GetCurrentAPISession(apiSessionParams, nil)
	if authErr != nil {
		return nil, errors.New("client not authenticated. login with 'ziti edge login' before executing")
	}
	return c, nil
}

func ExternalJWTSignerFromFilter(client *rest_client_api_client.ZitiEdgeClient, filter string) *rest_model.ClientExternalJWTSignerDetail {
	params := &external_jwt_signer.ListExternalJWTSignersParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(internalconsts.DefaultTimeout)
	resp, err := client.ExternalJWTSigner.ListExternalJWTSigners(params)
	if err != nil {
		pfxlog.Logger().Errorf("Could not obtain an ID for the external jwt signer with filter %s: %v", filter, err)
		return nil
	}
	if resp == nil || resp.Payload == nil || resp.Payload.Data == nil || len(resp.Payload.Data) == 0 {
		return nil
	}
	return resp.Payload.Data[0]
}
