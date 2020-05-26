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
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	removeSessionClient = NewMgmtClient(removeSessionCmd)
	removeSessionCmd.Flags().BoolVar(&removeSessionNow, "now", false, "Remove session now")
	removeCmd.AddCommand(removeSessionCmd)
}

var removeSessionCmd = &cobra.Command{
	Use:   "session <sessionId>",
	Short: "Remove a session from the fabric",
	Args:  cobra.ExactArgs(1),
	Run:   removeSession,
}
var removeSessionNow bool
var removeSessionClient *mgmtClient

func removeSession(cmd *cobra.Command, args []string) {
	if ch, err := removeSessionClient.Connect(); err == nil {
		request := &mgmt_pb.RemoveSessionRequest{
			SessionId: args[0],
			Now:       removeSessionNow,
		}
		body, err := proto.Marshal(request)
		if err != nil {
			panic(err)
		}
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_RemoveSessionRequestType), body)
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
