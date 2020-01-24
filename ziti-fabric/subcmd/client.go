/*
	Copyright 2019 NetFoundry, Inc.

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

package subcmd

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/dotziti"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/spf13/cobra"
)

type mgmtClient struct {
	id           *identity.TokenId
	idName       string
	mgmtEndpoint string
}

func NewMgmtClient(cmd *cobra.Command) *mgmtClient {
	client := &mgmtClient{}
	cmd.Flags().StringVarP(&client.idName, "identityName", "i", "default", "dotzeet identity name")
	cmd.Flags().StringVarP(&client.mgmtEndpoint, "mgmtEndpoint", "e", "", "fabric management endpoint address")
	return client
}

func (c *mgmtClient) Connect() (channel2.Channel, error) {
	if endpoint, id, err := dotziti.LoadIdentity(c.idName); err == nil {
		c.id = id
		endpointStr := endpoint
		if c.mgmtEndpoint != "" {
			endpointStr = c.mgmtEndpoint
		}
		if mgmtAddress, err := transport.ParseAddress(endpointStr); err == nil {
			dialer := channel2.NewClassicDialer(c.id, mgmtAddress, nil)
			if ch, err := channel2.NewChannel("mgmt", dialer, nil); err == nil {
				return ch, nil
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}
