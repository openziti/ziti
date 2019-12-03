/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/fabric/pb/mgmt_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	createService.Flags().StringVar(&createServiceBinding, "binding", "transport", "Xgress binding for service")
	createServiceClient = NewMgmtClient(createService)
	createCmd.AddCommand(createService)
}

var createService = &cobra.Command{
	Use:   "service <serviceId> <endpoint> <egress>",
	Short: "Create a new fabric service",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := createServiceClient.Connect(); err == nil {
			request := &mgmt_pb.CreateServiceRequest{
				Service: &mgmt_pb.Service{
					Id:              args[0],
					Binding:         createServiceBinding,
					EndpointAddress: args[1],
					Egress:          args[2],
				},
			}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_CreateServiceRequestType), body)
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

		} else {
			panic(err)
		}
	},
}
var createServiceClient *mgmtClient
var createServiceBinding string
