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
	"sort"
	"time"
)

func init() {
	listCircuitsClient = NewMgmtClient(listCircuitsCmd)
	listCmd.AddCommand(listCircuitsCmd)
}

var listCircuitsCmd = &cobra.Command{
	Use:   "circuits",
	Short: "List circuits",
	Run:   listCircuits,
}
var listCircuitsClient *mgmtClient

func listCircuits(cmd *cobra.Command, args []string) {
	if ch, err := listCircuitsClient.Connect(); err == nil {
		request := &mgmt_pb.ListCircuitsRequest{}
		body, err := proto.Marshal(request)
		if err != nil {
			panic(err)
		}
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListCircuitsRequestType), body)
		waitCh, err := ch.SendAndWait(requestMsg)
		if err != nil {
			panic(err)
		}
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListCircuitsResponseType) {
				response := &mgmt_pb.ListCircuitsResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					out := fmt.Sprintf("\nCircuits: (%d)\n\n", len(response.Circuits))
					out += fmt.Sprintf("%-12s | %-12s | %-12s | %s\n", "Id", "Client", "Service", "Path")
					sort.Slice(response.Circuits, func(i, j int) bool {
						if response.Circuits[i].ServiceId == response.Circuits[j].ServiceId {
							return response.Circuits[i].Id < response.Circuits[j].Id
						}
						return response.Circuits[i].ServiceId < response.Circuits[j].ServiceId
					})
					for _, s := range response.Circuits {
						out += fmt.Sprintf("%-12s | %-12s | %-12s | %s", s.Id, s.ClientId, s.ServiceId, s.Path.CalculateDisplayPath())
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
