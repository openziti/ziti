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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_client"
	"github.com/openziti/edge-api/rest_client_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	internalconsts "github.com/openziti/ziti/internal/rest/consts"
)

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
