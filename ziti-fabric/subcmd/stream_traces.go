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
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/trace/pb"
	"github.com/spf13/cobra"
	"sort"
	"time"
)

func init() {
	streamTracesClient = NewMgmtClient(streamTracesCmd)
	streamCmd.AddCommand(streamTracesCmd)

	streamTracesCmd.AddCommand(toggleTracesCmd)
}

var streamTracesCmd = &cobra.Command{
	Use:   "traces <except> [message type}",
	Short: "Stream trace data from systems where tracing is enabled",
	Run:   streamTraces,
}

var toggleTracesCmd = &cobra.Command{
	Use:   "toggle",
	Short: "Toggle traces on or off",
}

var streamTracesClient *mgmtClient

func streamTraces(_ *cobra.Command, args []string) {
	startIndex := 0
	request := &mgmt_pb.StreamTracesRequest{EnabledFilter: len(args) > 0}

	if request.EnabledFilter {
		request.EnabledFilter = true
		request.FilterType = mgmt_pb.TraceFilterType_EXCLUDE
		if len(args) == 0 || args[0] != "except" {
			request.FilterType = mgmt_pb.TraceFilterType_INCLUDE
		} else {
			startIndex = 1
		}

		for _, filterArg := range args[startIndex:] {
			contentType, err := getContentType(filterArg)
			if err != nil {
				panic(err)
			}
			request.ContentTypes = append(request.ContentTypes, contentType)
		}
	}

	ch, err := streamTracesClient.Connect()
	if err != nil {
		panic(err)
	}

	body, err := proto.Marshal(request)
	if err != nil {
		panic(err)
	}

	ch.AddReceiveHandler(&traceHandler{})
	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_StreamTracesRequestType), body)

	waitCh, err := ch.SendAndSync(requestMsg)
	if err != nil {
		panic(err)
	}
	select {
	case err := <-waitCh:
		if err != nil {
			panic(err)
		}
	case <-time.After(5 * time.Second):
		panic(errors.New("timeout"))
	}
	waitForChannelClose(ch)
}

func getContentType(name string) (int32, error) {
	val, ok := mgmt_pb.ContentType_value[name]
	if ok {
		return val, nil
	}

	val, ok = ctrl_pb.ContentType_value[name]
	if ok {
		return val, nil
	}

	val, ok = xgress.ContentTypeValue[name]
	if ok {
		return val, nil
	}

	var types []string
	for key := range mgmt_pb.ContentType_value {
		types = append(types, key)
	}
	for key := range ctrl_pb.ContentType_value {
		types = append(types, key)
	}
	for key := range xgress.ContentTypeValue {
		types = append(types, key)
	}
	sort.Strings(types)
	fmt.Println("Valid message types:")
	for idx, key := range types {
		fmt.Printf("%v: %v\n", idx, key)
	}

	return 0, fmt.Errorf("unknown message type %v", name)
}

type traceHandler struct{}

func (*traceHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamTracesEventType)
}

func (*traceHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	event := &trace_pb.ChannelMessage{}
	err := proto.Unmarshal(msg.Body, event)
	if err != nil {
		panic(err)
	}

	flow := "->"
	if event.IsRx {
		flow = "<-"
	}
	replyFor := ""
	if event.ReplyFor != -1 {
		replyFor = fmt.Sprintf(">%d", event.ReplyFor)
	}
	fmt.Printf("%8d: %-16s %8s %s #%-5d %5s | %s\n",
		event.Timestamp, event.Identity, event.Channel, flow, event.Sequence, replyFor, channel2.DecodeTraceAndFormat(event.Decode))
}
