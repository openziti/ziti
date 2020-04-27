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

var createTerminatorClient *mgmtClient
var createTerminatorBinding string

func init() {
	createTerminator.Flags().StringVar(&createTerminatorBinding, "binding", "transport", "Terminator binding")
	createTerminatorClient = NewMgmtClient(createTerminator)
	createCmd.AddCommand(createTerminator)
}

var createTerminator = &cobra.Command{
	Use:   "terminator <service> <router> <address>",
	Short: "Create a new fabric service terminator",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := createTerminatorClient.Connect(); err == nil {
			request := &mgmt_pb.CreateTerminatorRequest{
				Terminator: &mgmt_pb.Terminator{
					ServiceId: args[0],
					RouterId:  args[1],
					Binding:   createTerminatorBinding,
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
