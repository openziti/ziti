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
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/stringz"
	"github.com/spf13/cobra"
	"time"
)

var createTerminatorClient *mgmtClient
var createTerminatorBinding string
var createTerminatorCost uint32
var createTerminatorPrecedence string
var createTerminatorIdentity string
var createTerminatorIdentitySecret string

func init() {
	createTerminator.Flags().StringVar(&createTerminatorBinding, "binding", "transport", "Terminator binding")
	createTerminator.Flags().Uint32VarP(&createTerminatorCost, "cost", "c", 0, "Set the terminator cost")
	createTerminator.Flags().StringVarP(&createTerminatorPrecedence, "precedence", "p", "default", "Set the terminator precedence ('default', 'required' or 'failed')")
	createTerminator.Flags().StringVar(&createTerminatorIdentity, "identity", "", "Set the terminator identity")
	createTerminator.Flags().StringVar(&createTerminatorIdentitySecret, "identity-secret", "", "Set the terminator identity secret")

	createTerminatorClient = NewMgmtClient(createTerminator)
	createCmd.AddCommand(createTerminator)
}

var createTerminator = &cobra.Command{
	Use:   "terminator <service> <router> <address>",
	Short: "Create a new fabric service terminator",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		validValues := []string{"default", "required", "failed"}
		if !stringz.Contains(validValues, createTerminatorPrecedence) {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Invalid precedence %v. Must be one of %+v\n", createTerminatorPrecedence, validValues); err != nil {
				panic(err)
			}
			return
		}

		precedence := mgmt_pb.TerminatorPrecedence_Default
		if createTerminatorPrecedence == "required" {
			precedence = mgmt_pb.TerminatorPrecedence_Required
		} else if createTerminatorPrecedence == "failed" {
			precedence = mgmt_pb.TerminatorPrecedence_Failed
		}

		if ch, err := createTerminatorClient.Connect(); err == nil {
			request := &mgmt_pb.CreateTerminatorRequest{
				Terminator: &mgmt_pb.Terminator{
					ServiceId:      args[0],
					RouterId:       args[1],
					Binding:        createTerminatorBinding,
					Address:        args[2],
					Identity:       createTerminatorIdentity,
					IdentitySecret: []byte(createTerminatorIdentitySecret),
					Precedence:     precedence,
					Cost:           createTerminatorCost,
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
