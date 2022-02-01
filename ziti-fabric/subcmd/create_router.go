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
	"crypto/sha1"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/identity/certtools"
	"github.com/spf13/cobra"
	"time"
)

var createRouterName string

func init() {
	createRouter.Flags().StringVar(&createServiceName, "name", "", "Router name. If not provided defaults to the router ID")
	createRouterClient = NewMgmtClient(createRouter)
	createCmd.AddCommand(createRouter)
}

var createRouter = &cobra.Command{
	Use:   "router <certPath>",
	Short: "Enroll router",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := createRouterClient.Connect(); err == nil {
			if cert, err := certtools.LoadCertFromFile(args[0]); err == nil {
				request := &mgmt_pb.CreateRouterRequest{
					Router: &mgmt_pb.Router{
						Id:          cert[0].Subject.CommonName,
						Name:        createRouterName,
						Fingerprint: fmt.Sprintf("%x", sha1.Sum(cert[0].Raw)),
					},
				}
				if body, err := proto.Marshal(request); err == nil {
					requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_CreateRouterRequestType), body)
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
	},
}
var createRouterClient *mgmtClient
