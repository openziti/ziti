/*
	Copyright NetFoundry Inc.

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

package fabric

import (
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/trace/pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"time"
)

type streamTogglePipeTracesAction struct {
	api.Options
}

func NewStreamTogglePipeTracesCmd(p common.OptionsProvider) *cobra.Command {
	action := streamTogglePipeTracesAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	streamTogglePipeTracesCmd := &cobra.Command{
		Use:     "pipe [on|off] <app id regex> <link id regex>",
		Short:   "Toggle trace data to be generated for applications or specific links",
		Example: "pipe on",
		Args:    cobra.MinimumNArgs(1),
		Run:     action.togglePipeTraces,
	}

	action.AddCommonFlags(streamTogglePipeTracesCmd)

	return streamTogglePipeTracesCmd
}

func (self *streamTogglePipeTracesAction) togglePipeTraces(_ *cobra.Command, args []string) {
	ch, err := api.NewWsMgmtChannel(nil)
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
		requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_TogglePipeTracesRequestType), body)
		responseMsg, err := requestMsg.WithTimeout(5 * time.Second).SendForReply(ch)
		if err != nil {
			panic(err)
		}
		if responseMsg.ContentType == channel.ContentTypeResultType {
			result := channel.UnmarshalResult(responseMsg)
			if result.Success {
				fmt.Printf("\ntracing enabled successfully\n\n")
				fmt.Println(result.Message)
			} else {
				fmt.Printf("\ntracing enable failured [%s]\n\n", result.Message)
			}
		} else {
			panic(fmt.Errorf("unexpected response type %v", responseMsg.ContentType))
		}
	} else {
		panic(err)
	}
}
