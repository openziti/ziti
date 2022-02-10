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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/spf13/cobra"
	"strconv"
	"time"
)

func init() {
	setLinkCostClient = NewMgmtClient(setLinkCostCmd)
	setCmd.AddCommand(setLinkCostCmd)
}

var setLinkCostCmd = &cobra.Command{
	Use:   "link-cost <linkId> <cost>",
	Short: "Set link cost",
	Args:  cobra.ExactArgs(2),
	Run:   setLinkCost,
}
var setLinkCostClient *mgmtClient

func setLinkCost(cmd *cobra.Command, args []string) {
	if ch, err := setLinkCostClient.Connect(); err == nil {
		if cost, err := strconv.Atoi(args[1]); err == nil {
			request := &mgmt_pb.SetLinkCostRequest{
				LinkId: args[0],
				Cost:   int32(cost),
			}
			if body, err := proto.Marshal(request); err == nil {
				requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_SetLinkCostRequestType), body)
				responseMsg, err := requestMsg.WithTimeout(5 * time.Second).SendForReply(ch)
				if err != nil {
					panic(err)
				}
				if responseMsg.ContentType == channel.ContentTypeResultType {
					result := channel.UnmarshalResult(responseMsg)
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
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}
}
