/*
	Copyright NetFoundry, Inc.

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

package xctrl

import (
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"time"
)

type Capabilities struct {
	channel.Channel
}

func (capabilities *Capabilities) Get(timeout time.Duration) ([]string, error) {
	request := &ctrl_pb.InspectRequest{RequestedValues: []string{"capability"}}
	response := &ctrl_pb.InspectResponse{}
	respMsg, err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendForReply(capabilities.Channel)
	if err = protobufs.TypedResponse(response).Unmarshall(respMsg, err); err != nil {
		return nil, err
	}
	var result []string
	for _, value := range response.Values {
		if value.Name == "capability" {
			result = append(result, value.Value)
		}
	}
	return result, nil
}

func (capabilities *Capabilities) Has(capability string, timeout time.Duration) (bool, error) {
	capabilityList, err := capabilities.Get(timeout)
	if err != nil {
		return false, err
	}
	for _, cap := range capabilityList {
		if cap == capability {
			return true, nil
		}
	}
	return false, nil
}

func (capabilities *Capabilities) IsEdgeCapable(timeout time.Duration) (bool, error) {
	return capabilities.Has("ziti.edge", timeout)
}
