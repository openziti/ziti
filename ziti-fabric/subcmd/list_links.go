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
	listLinksClient = NewMgmtClient(listLinksCmd)
	listCmd.AddCommand(listLinksCmd)
}

var listLinksCmd = &cobra.Command{
	Use:   "links",
	Short: "List links visible to the fabric",
	Run:   listLinks,
}
var listLinksClient *mgmtClient

func listLinks(cmd *cobra.Command, args []string) {
	if ch, err := listLinksClient.Connect(); err == nil {
		request := &mgmt_pb.ListLinksRequest{}
		body, err := proto.Marshal(request)
		if err != nil {
			panic(err)
		}
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListLinksRequestType), body)
		waitCh, err := ch.SendAndWait(requestMsg)
		if err != nil {
			panic(err)
		}
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListLinksResponseType) {
				response := &mgmt_pb.ListLinksResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					out := fmt.Sprintf("\nLinks: (%d)\n\n", len(response.Links))
					out += fmt.Sprintf("%-6s | %-24s -> %-24s | %-12s | %-4s | %-13s | %-12s | %-6s\n", "Id", "Src", "Dst", "State", "Cost", "Latency", "Full Cost", "Status")
					sort.Slice(response.Links, func(i, j int) bool {
						if response.Links[i].Src == response.Links[j].Src {
							return response.Links[i].Dst < response.Links[j].Dst
						}
						return response.Links[i].Src < response.Links[j].Src
					})
					for _, l := range response.Links {
						status := "up"
						if l.Down {
							status = "down"
						}
						// Convert nanoseconds to fractional seconds
						srcLatency := float64(l.SrcLatency) / 1_000_000_000.0
						dstLatency := float64(l.DstLatency) / 1_000_000_000.0
						cost := (l.SrcLatency / 1_000_000) + (l.DstLatency / 1_000_000) + int64(l.Cost)
						out += fmt.Sprintf("%-6s | %-24s -> %-24s | %-12s | %-4d | %-0.4f %-0.4f | %-12v | %-6v\n", l.Id, l.Src, l.Dst, l.State, l.Cost, srcLatency, dstLatency,
							cost, status)
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
