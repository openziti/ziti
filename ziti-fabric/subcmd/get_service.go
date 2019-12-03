/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/fabric/pb/mgmt_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	getServiceClient = NewMgmtClient(getService)
	getCmd.AddCommand(getService)
}

var getService = &cobra.Command{
	Use:   "service <serviceId>",
	Short: "Retrieve a service definition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := getServiceClient.Connect(); err == nil {
			request := &mgmt_pb.GetServiceRequest{
				ServiceId: args[0],
			}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_GetServiceRequestType), body)
			waitCh, err := ch.SendAndWait(requestMsg)
			if err != nil {
				panic(err)
			}
			select {
			case responseMsg := <-waitCh:
				if responseMsg.ContentType == int32(mgmt_pb.ContentType_GetServiceResponseType) {
					response := &mgmt_pb.GetServiceResponse{}
					err := proto.Unmarshal(responseMsg.Body, response)
					if err == nil {
						out := fmt.Sprintf("\n%10s | %30s | %s\n", "Id", "Endpoint", "Egress")
						out += "-----------+--------------------------------+----------\n"
						svc := response.Service
						out += fmt.Sprintf("%10s | %30s | %s\n", svc.Id, svc.EndpointAddress, svc.Egress)
						out += "\n"
						fmt.Print(out)

					} else {
						out := fmt.Sprintf("Id not found\n")
						fmt.Print(out)
					}
				} else if responseMsg.ContentType == channel2.ContentTypeResultType {
					result := channel2.UnmarshalResult(responseMsg)
					if result.Success {
						fmt.Printf("\nsuccess\n\n")
					} else {
						fmt.Printf("\nfailure [%s]\n\n", result.Message)
					}
				} else {
					panic(fmt.Errorf("unexpected response type %v", responseMsg.ContentType))
				}

			case <-time.After(5 * time.Second):
				panic(errors.New("timeout"))
			}
		} else {
			panic(err)
		}
	},
}
var getServiceClient *mgmtClient
