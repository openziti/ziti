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
	removeCircuitClient = NewMgmtClient(removeCircuitCmd)
	removeCircuitCmd.Flags().BoolVar(&removeCircuitNow, "now", false, "Remove circuit now")
	removeCmd.AddCommand(removeCircuitCmd)
}

var removeCircuitCmd = &cobra.Command{
	Use:   "circuit <circuitId>",
	Short: "Remove a circuit from the fabric",
	Args:  cobra.ExactArgs(1),
	Run:   removeCircuit,
}

var removeCircuitNow bool
var removeCircuitClient *mgmtClient

func removeCircuit(cmd *cobra.Command, args []string) {
	if ch, err := removeCircuitClient.Connect(); err == nil {
		request := &mgmt_pb.RemoveCircuitRequest{
			CircuitId: args[0],
			Now:       removeCircuitNow,
		}
		body, err := proto.Marshal(request)
		if err != nil {
			panic(err)
		}
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_RemoveCircuitRequestType), body)
		waitCh, err := ch.SendAndWait(requestMsg)
		if err != nil {
			panic(err)
		}
		select {
		case responseMsg := <-waitCh:
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

		case <-time.After(5 * time.Second):
			panic(errors.New("timeout"))
		}
	}
}
