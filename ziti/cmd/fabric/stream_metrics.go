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
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sort"
	"time"
)

type streamMetricsAction struct {
	api.Options
}

func NewStreamMetricsCmd(p common.OptionsProvider) *cobra.Command {
	action := streamMetricsAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	streamMetricsCmd := &cobra.Command{
		Use:   "metrics <metrics regex> <source regex>",
		Short: "Stream fabric metrics",
		Args:  cobra.MaximumNArgs(2),
		Run:   action.streamMetrics,
	}

	action.AddCommonFlags(streamMetricsCmd)

	return streamMetricsCmd
}

func (self *streamMetricsAction) streamMetrics(_ *cobra.Command, args []string) {
	closeNotify := make(chan struct{})

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_StreamMetricsEventType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
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
	if err = requestMsg.WithTimeout(time.Duration(self.Timeout) * time.Second).SendAndWaitForWire(ch); err != nil {
		panic(err)
	}

	<-closeNotify
}

func (self *streamMetricsAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	response := &mgmt_pb.StreamMetricsEvent{}
	err := proto.Unmarshal(msg.Body, response)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%v - source(%v)\n", self.format(response.Timestamp), response.SourceId)
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
		fmt.Printf("%v: (%v) -> (%v)\n", bucket.Name, self.format(bucket.IntervalStartUTC), self.format(bucket.IntervalEndUTC))
		for name, value := range bucket.Values {
			fmt.Printf("\t%v=%v\n", name, value)
		}
	}

	fmt.Println()
}

func (self *streamMetricsAction) format(protobufTS *timestamppb.Timestamp) string {
	return protobufTS.AsTime().Format(time.RFC3339)
}
