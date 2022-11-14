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
	"encoding/json"
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/trace/pb"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"reflect"
	"sort"
	"time"
)

type streamTracesAction struct {
	api.Options
}

func NewStreamTracesCmd(p common.OptionsProvider) *cobra.Command {
	action := streamTracesAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	streamTracesCmd := &cobra.Command{
		Use:   "traces <except> [message type}",
		Short: "Stream trace data from systems where tracing is enabled",
		Run:   action.streamTraces,
	}

	action.AddCommonFlags(streamTracesCmd)

	return streamTracesCmd
}

func (self *streamTracesAction) streamTraces(_ *cobra.Command, args []string) {
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
			contentType, err := self.getContentType(filterArg)
			if err != nil {
				panic(err)
			}
			request.ContentTypes = append(request.ContentTypes, contentType)
		}
	}

	closeNotify := make(chan struct{})

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_StreamTracesEventType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		panic(err)
	}

	body, err := proto.Marshal(request)
	if err != nil {
		panic(err)
	}

	requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamTracesRequestType), body)

	if err = requestMsg.WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
		panic(err)
	}

	<-closeNotify
}

func (self *streamTracesAction) getContentType(name string) (int32, error) {
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

func (self *streamTracesAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
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
	meta := self.DecodeTraceAndFormat(event.Decode)
	if meta == "" {
		meta = fmt.Sprintf("missing decode, content-type=%v", event.ContentType)
	}
	fmt.Printf("%8d: %-16s %8s %s #%-5d %5s | %s\n",
		event.Timestamp, event.Identity, event.Channel, flow, event.Sequence, replyFor, meta)
}

func (self *streamTracesAction) DecodeTraceAndFormat(decode []byte) string {
	if len(decode) > 0 {
		meta := make(map[string]interface{})
		err := json.Unmarshal(decode, &meta)
		if err != nil {
			panic(err)
		}

		out := fmt.Sprintf("%-24s", fmt.Sprintf("%-8s %s", meta[channel.DecoderFieldName], meta[channel.MessageFieldName]))

		if len(meta) > 2 {
			keys := make([]string, 0)
			for k := range meta {
				if k != channel.DecoderFieldName && k != channel.MessageFieldName {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)

			out += " {"
			for i := 0; i < len(keys); i++ {
				k := keys[i]
				if i > 0 {
					out += " "
				}
				out += k
				out += "=["
				v := meta[k]
				switch v.(type) {
				case string:
					out += v.(string)
				case float64:
					out += fmt.Sprintf("%0.0f", v.(float64))
				case bool:
					out += fmt.Sprintf("%t", v.(bool))
				default:
					out += fmt.Sprintf("<%s>", reflect.TypeOf(v))
				}
				out += "]"
			}
			out += "}"
		}

		return out
	} else {
		return ""
	}
}
