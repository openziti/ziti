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
	listRoutersClient = NewMgmtClient(listRouters)
	listCmd.AddCommand(listRouters)
}

var listRouters = &cobra.Command{
	Use:   "routers",
	Short: "List routers enrolled on the fabric",
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := listRoutersClient.Connect(); err == nil {
			query := "true limit none"
			if len(args) > 0 {
				query = args[0]
			}
			request := &mgmt_pb.ListRoutersRequest{
				Query: query,
			}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListRoutersRequestType), body)
			waitCh, err := ch.SendAndWait(requestMsg)
			if err != nil {
				panic(err)
			}
			select {
			case responseMsg := <-waitCh:
				if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListRoutersResponseType) {
					response := &mgmt_pb.ListRoutersResponse{}
					err := proto.Unmarshal(responseMsg.Body, response)
					if err == nil {
						out := fmt.Sprintf("\nRouters: (%d)\n\n", len(response.Routers))
						out += fmt.Sprintf("%-12s | %-30s | %-40s | %s\n", "Id", "Name", "Fingerprint", "Status")
						for _, r := range response.Routers {
							status := ""
							if r.Connected {
								status += "Connected"
							}
							if r.ListenerAddress != "" {
								status += " (" + r.ListenerAddress + ")"
							}
							out += fmt.Sprintf("%-12s | %-30s | %-40s | %s\n", r.Id, r.Name, r.Fingerprint, status)
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
	},
}
var listRoutersClient *mgmtClient
