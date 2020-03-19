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
	trace_pb "github.com/netfoundry/ziti-foundation/trace/pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	streamTogglePipeTracesClient = NewMgmtClient(streamTogglePipeTracesCmd)
	toggleTracesCmd.AddCommand(streamTogglePipeTracesCmd)
}

var streamTogglePipeTracesCmd = &cobra.Command{
	Use:     "pipe [on|off] <app id regex> <link id regex>",
	Short:   "Toggle trace data to be generated for applications or specific links",
	Example: "pipe on",
	Args:    cobra.MinimumNArgs(1),
	Run:     togglePipeTraces,
}
var streamTogglePipeTracesClient *mgmtClient

func togglePipeTraces(cmd *cobra.Command, args []string) {
	ch, err := streamTracesClient.Connect()
	if err != nil {
		panic(err)
	}

	enable := true

	if args[0] == "off" {
		enable = false
	} else if args[0] != "on" {
		fmt.Println("first argument to toggle pipe must be on or off")
		return
	}
	request := &trace_pb.TogglePipeTracesRequest{Enable: enable, Verbosity: trace_pb.TraceToggleVerbosity_ReportAll, AppRegex: ".*", PipeRegex: ".*"}

	if len(args) > 1 {
		request.AppRegex = args[1]
	}
	if len(args) > 2 {
		request.PipeRegex = args[2]
	}

	if body, err := proto.Marshal(request); err == nil {
		requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_TogglePipeTracesRequestType), body)
		waitCh, err := ch.SendAndWait(requestMsg)
		if err != nil {
			panic(err)
		}
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == channel2.ContentTypeResultType {
				result := channel2.UnmarshalResult(responseMsg)
				if result.Success {
					fmt.Printf("\ntracing enabled successfully\n\n")
					fmt.Println(result.Message)
				} else {
					fmt.Printf("\ntracing enable failured [%s]\n\n", result.Message)
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
}
