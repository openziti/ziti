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
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	listServicesClient = NewMgmtClient(listServices)
	listCmd.AddCommand(listServices)
}

var listServices = &cobra.Command{
	Use:   "services",
	Short: "Retrieve all service definitions",
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := listServicesClient.Connect(); err == nil {
			query := "true limit none"
			if len(args) > 0 {
				query = args[0]
			}
			request := &mgmt_pb.ListServicesRequest{
				Query: query,
			}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListServicesRequestType), body)
			responseMsg, err := ch.SendAndWaitWithTimeout(requestMsg, 5*time.Second)
			if err != nil {
				panic(err)
			}
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListServicesResponseType) {
				response := &mgmt_pb.ListServicesResponse{}
				if err := proto.Unmarshal(responseMsg.Body, response); err == nil {
					out := fmt.Sprintf("\nServices: (%d)\n\n", len(response.Services))
					out += fmt.Sprintf("%-12s | %-12s | %-12s | %s\n", "Id", "Name", "Terminator Strategy", "Destination(s)")
					for _, svc := range response.Services {
						if len(svc.Terminators) > 0 {
							out += fmt.Sprintf("%-12s | %-12s | %-12s | %s\n", svc.Id, svc.Name, svc.TerminatorStrategy,
								fmt.Sprintf("%-12s -> %s", svc.Terminators[0].RouterId, svc.Terminators[0].Address))
							for _, terminator := range svc.Terminators[1:] {
								out += fmt.Sprintf("%-12s | %-12s | %s\n", "", "",
									fmt.Sprintf("%-12s -> %s", terminator.RouterId, terminator.Address))
							}
						} else {
							out += fmt.Sprintf("%-12s | %-12s | %-12s \n", svc.Id, svc.Name, svc.TerminatorStrategy)
						}
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
var listServicesClient *mgmtClient
