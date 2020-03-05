/*
	Copyright 2020 NetFoundry, Inc.

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
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	listEndpointsClient = NewMgmtClient(listEndpoints)
	listCmd.AddCommand(listEndpoints)
}

var listEndpoints = &cobra.Command{
	Use:   "endpoints",
	Short: "Retrieve endpoint definitions",
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := listEndpointsClient.Connect(); err == nil {
			request := &mgmt_pb.ListEndpointsRequest{}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListEndpointsRequestType), body)
			responseMsg, err := ch.SendAndWaitWithTimeout(requestMsg, 5*time.Second)
			if err != nil {
				panic(err)
			}
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListEndpointsResponseType) {
				response := &mgmt_pb.ListEndpointsResponse{}
				if err := proto.Unmarshal(responseMsg.Body, response); err == nil {
					out := fmt.Sprintf("\nEndpoints: (%d)\n\n", len(response.Endpoints))
					out += fmt.Sprintf("%-12s | %-12s | %-12s | %s\n", "Id", "Service", "Binding", "Destination")
					for _, endpoint := range response.Endpoints {
						out += fmt.Sprintf("%-12s | %-12s | %-12s | %s\n", endpoint.Id, endpoint.ServiceId, endpoint.Binding,
							fmt.Sprintf("%-12s -> %s", endpoint.RouterId, endpoint.Address))
					}
					out += "\n"
					fmt.Print(out)
				} else {
					panic(err)
				}
			} else {
				panic(errors.New("unexpected response"))
			}
		} else {
			panic(err)
		}
	},
}
var listEndpointsClient *mgmtClient
