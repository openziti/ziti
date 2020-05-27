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

package subcmd

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
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
			responseMsg, err := ch.SendAndWaitWithTimeout(requestMsg, 5*time.Second)
			if err != nil {
				panic(err)
			}
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_GetServiceResponseType) {
				response := &mgmt_pb.GetServiceResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					fmt.Printf("\n%10s | %30s\n", "Id", "Terminator Strategy")
					fmt.Printf("-----------+--------------------------------+----------\n")
					svc := response.Service
					fmt.Printf("%10s | %30s\n\n", svc.Id, svc.TerminatorStrategy)
					fmt.Printf("Terminators (%v)\n", len(svc.Terminators))
					fmt.Printf("\n%10s | %-12s| %v\n", "Id", "Binding", "Destination")
					for _, terminator := range svc.Terminators {
						fmt.Printf("%-10s | %-12s | %s\n", terminator.Id, terminator.Binding,
							fmt.Sprintf("%-12s -> %s", terminator.RouterId, terminator.Address))
					}
				} else {
					fmt.Printf("Id not found\n")
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
		} else {
			panic(err)
		}
	},
}
var getServiceClient *mgmtClient
