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

package handler_edge_ctrl

import (
	"time"

	sdkedge_pb "github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/controller/model"
)

// postureResponsesToModel is unimplemented: the HA posture-forwarding feature is delivered by the
// code patch. The signature exists so the tests compile and fail on assertions until then.
func postureResponsesToModel(postureResponses *sdkedge_pb.PostureResponses, now time.Time, singleProcessChecksByPath map[string][]string) []*model.PostureResponse {
	return nil
}
