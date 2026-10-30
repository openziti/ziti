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

package xgress_common

import (
	"fmt"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/posture"
)

// CheckRouterDialAccess returns nil when the router identity routerId is enabled and a dial policy
// grants it access to serviceId. Routers submit no posture data, so a policy carrying posture
// checks denies. A non-nil error names the reason.
func CheckRouterDialAccess(rdm *common.RouterDataModel, routerId string, serviceId string) error {
	identity, found := rdm.Identities.Get(routerId)
	if !found {
		return fmt.Errorf("router identity '%s' not present in data model", routerId)
	}
	if identity.Disabled {
		return fmt.Errorf("router identity '%s' is disabled", routerId)
	}

	policy, err := posture.HasAccess(rdm, routerId, serviceId, nil, edge_ctrl_pb.PolicyType_DialPolicy)
	if err != nil || policy == nil {
		return fmt.Errorf("router does not have dial access to service '%s' (%w)", serviceId, err)
	}
	return nil
}
