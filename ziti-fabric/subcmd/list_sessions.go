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
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"sort"
	"time"
)

func init() {
	listSessionsClient = NewMgmtClient(listSessionsCmd)
	listCmd.AddCommand(listSessionsCmd)
}

var listSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List sessions",
	Run:   listSessions,
}
var listSessionsClient *mgmtClient

func listSessions(cmd *cobra.Command, args []string) {
	if ch, err := listSessionsClient.Connect(); err == nil {
		request := &mgmt_pb.ListSessionsRequest{}
		body, err := proto.Marshal(request)
		if err != nil {
			panic(err)
		}
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListSessionsRequestType), body)
		waitCh, err := ch.SendAndWait(requestMsg)
		if err != nil {
			panic(err)
		}
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListSessionsResponseType) {
				response := &mgmt_pb.ListSessionsResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					out := fmt.Sprintf("\nSessions: (%d)\n\n", len(response.Sessions))
					out += fmt.Sprintf("%-12s | %-12s | %-12s | %s\n", "Id", "Client", "Service", "Path")
					sort.Slice(response.Sessions, func(i, j int) bool {
						if response.Sessions[i].ServiceId == response.Sessions[j].ServiceId {
							return response.Sessions[i].Id < response.Sessions[j].Id
						}
						return response.Sessions[i].ServiceId < response.Sessions[j].ServiceId
					})
					for _, s := range response.Sessions {
						out += fmt.Sprintf("%-12s | %-12s | %-12s | %s", s.Id, s.ClientId, s.ServiceId, s.Circuit.CalculateDisplayPath())
					}
					out += "\n"
					fmt.Print(out)

				} else {
					panic(err)
				}
			} else {
				panic(errors.New("unexpected response"))
			}

		case <-time.After(5 * time.Second):
			panic(errors.New("timeout"))
		}
	} else {
		panic(err)
	}
}
