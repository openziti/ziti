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
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/spf13/cobra"
	"time"
)

var updateTerminatorClient *mgmtClient
var updateTerminatorBinding string
var updateTerminatorRouter string
var updateTerminatorAddress string
var updateTerminatorCost int
var updateTerminatorWeight uint8
var updateTerminatorPrecedence string

func init() {
	updateTerminator.Flags().StringVar(&updateTerminatorBinding, "binding", "transport", "Terminator binding")
	updateTerminator.Flags().StringVar(&updateTerminatorRouter, "router", "", "Terminator router")
	updateTerminator.Flags().StringVar(&updateTerminatorAddress, "address", "", "Terminator address")
	updateTerminator.Flags().IntVar(&updateTerminatorCost, "cost", 0, "Terminator cost")
	updateTerminator.Flags().Uint8Var(&updateTerminatorWeight, "cost", 0, "Terminator weight")
	updateTerminator.Flags().StringVar(&updateTerminatorPrecedence, "precedence", "", "Terminator precedence")
	updateTerminatorClient = NewMgmtClient(updateTerminator)
	updateCmd.AddCommand(updateTerminator)
}

var updateTerminator = &cobra.Command{
	Use:   "terminator <id>",
	Short: "Update a fabric service terminator",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := updateTerminatorClient.Connect(); err == nil {
			request := &mgmt_pb.CreateTerminatorRequest{
				Terminator: &mgmt_pb.Terminator{
					ServiceId: args[0],
					RouterId:  args[1],
					Binding:   updateTerminatorBinding,
					Address:   args[2],
				},
			}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_CreateTerminatorRequestType), body)
			responseMsg, err := ch.SendAndWaitWithTimeout(requestMsg, 5*time.Second)
			if err != nil {
				panic(err)
			}
			if responseMsg.ContentType == channel2.ContentTypeResultType {
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
