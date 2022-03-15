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
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/spf13/cobra"
	"sort"
	"time"
)

func init() {
	streamMetricsClient = NewMgmtClient(streamMetricsCmd)
	streamCmd.AddCommand(streamMetricsCmd)
}

var streamMetricsCmd = &cobra.Command{
	Use:   "metrics <metrics regex> <source regex>",
	Short: "Stream fabric metrics",
	Args:  cobra.MaximumNArgs(2),
	Run:   streamMetrics,
}
var streamMetricsClient *mgmtClient

func streamMetrics(cmd *cobra.Command, args []string) {
	cw := newCloseWatcher()

	bindHandler := func(binding channel.Binding) error {
		binding.AddTypedReceiveHandler(&metricsHandler{})
		binding.AddCloseHandler(cw)
		return nil
	}

	ch, err := streamCircuitsClient.ConnectAndBind(channel.BindHandlerF(bindHandler))
	if err != nil {
		panic(err)
	}

	var matchers []*mgmt_pb.StreamMetricsRequest_MetricMatcher

	if len(args) > 0 {
		matcher := &mgmt_pb.StreamMetricsRequest_MetricMatcher{NameRegex: args[0]}
		if len(args) > 1 {
			matcher.SourceIDRegex = args[1]
		}
		matchers = append(matchers, matcher)
	}

	request := &mgmt_pb.StreamMetricsRequest{Matchers: matchers}
	body, err := proto.Marshal(request)
	if err != nil {
		panic(err)
	}

	requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamMetricsRequestType), body)
	if err = requestMsg.WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
		panic(err)
	}

	cw.waitForChannelClose()
}

type metricsHandler struct{}

func (*metricsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamMetricsEventType)
}

func (*metricsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	response := &mgmt_pb.StreamMetricsEvent{}
	err := proto.Unmarshal(msg.Body, response)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%v - source(%v)\n", formattedTimestamp(response.Timestamp), response.SourceId)
	fmt.Printf("\tTags: %v\n", response.Tags)

	var keys []string
	var outputMap = make(map[string]string)
	for name, value := range response.IntMetrics {
		outputMap[name] = fmt.Sprintf("%v=%v", name, value)
		keys = append(keys, name)
	}

	for name, value := range response.FloatMetrics {
		outputMap[name] = fmt.Sprintf("%v=%v", name, value)
		keys = append(keys, name)
	}

	sort.Strings(keys)

	for _, key := range keys {
		fmt.Println(outputMap[key])
	}

	for _, bucket := range response.IntervalMetrics {
		fmt.Printf("%v: (%v) -> (%v)\n", bucket.Name, formattedTimestamp(bucket.IntervalStartUTC), formattedTimestamp(bucket.IntervalEndUTC))
		for name, value := range bucket.Values {
			fmt.Printf("\t%v=%v\n", name, value)
		}
	}

	fmt.Println()
}

func formattedTimestamp(protobufTS *timestamp.Timestamp) string {
	ts, err := ptypes.Timestamp(protobufTS)
	if err != nil {
		panic(err)
	}
	return ts.Format(time.RFC3339)
}
