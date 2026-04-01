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

package federation

import (
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/forwarder"
	"github.com/openziti/ziti/v2/router/handler_ctrl"
)

// ConnectClientNetwork establishes control channel connections to a federated client
// network's controllers. It creates a transit bind handler for forwarding, sets up a
// ClientDialEnv with the host router's configuration and the client network's identity,
// dials the client network's controller endpoints, and registers the resulting
// NetworkControllers with the multi-network controller registry.
func ConnectClientNetwork(ni *NetworkIdentity, networkId uint16, hostConfig *env.Config,
	routerEnv env.RouterEnv, fwd *forwarder.Forwarder,
	multiCtrls *forwarder.MultiNetworkControllers,
) (env.NetworkControllers, error) {
	heartbeatOpts := routerEnv.GetHeartbeatOptions()

	clientDialEnv := NewClientDialEnv(hostConfig, ni.Id, nil)
	clientCtrls := env.NewNetworkControllers(clientDialEnv, &heartbeatOpts)

	bindHandler, err := handler_ctrl.NewTransitBindHandler(networkId, routerEnv, fwd, clientCtrls)
	if err != nil {
		return nil, err
	}
	clientDialEnv.bindHandler = bindHandler

	clientCtrls.ConnectToInitialEndpoints(ni.Endpoints)
	multiCtrls.AddNetwork(networkId, clientCtrls)

	return clientCtrls, nil
}
