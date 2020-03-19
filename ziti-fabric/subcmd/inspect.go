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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	inspectClient = NewMgmtClient(inspectCmd)
	Root.AddCommand(inspectCmd)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect appIdRegex [name]+",
	Short: "Inspect runtime application values",
	Args:  cobra.MinimumNArgs(2),
	Run:   inspect,
}
var inspectClient *mgmtClient

func inspect(cmd *cobra.Command, args []string) {
	if ch, err := inspectClient.Connect(); err == nil {
		request := &mgmt_pb.InspectRequest{
			AppRegex:        args[0],
			RequestedValues: args[1:],
		}
		body, err := proto.Marshal(request)
		if err != nil {
			panic(err)
		}
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_InspectRequestType), body)
		waitCh, err := ch.SendAndWait(requestMsg)
		if err != nil {
			panic(err)
		}
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_InspectResponseType) {
				response := &mgmt_pb.InspectResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err != nil {
					panic(err)
				}
				if response.Success {
					fmt.Printf("\nResults: (%d)\n", len(response.Values))
					for _, value := range response.Values {
						fmt.Printf("%v.%v\n", value.AppId, value.Name)
						fmt.Printf("%v\n\n", value.Value)
					}
				} else {
					fmt.Printf("\nEncountered errors: (%d)\n", len(response.Errors))
					for _, err := range response.Errors {
						fmt.Printf("\t%v\n", err)
					}
				}
			} else {
				panic(errors.New("unexpected response"))
			}
		case <-time.After(10 * time.Second):
			panic(errors.New("timeout"))
		}
	} else {
		panic(err)
	}
}
