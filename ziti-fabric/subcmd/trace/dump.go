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

package trace

import (
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/trace"
	"github.com/openziti/foundation/trace/pb"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
)

func init() {
	dumpCmd.Flags().BoolVarP(&dumpCmdTimeDeltas, "timedeltas", "t", false, "Display time deltas (not timestamps)")
	dumpCmd.Flags().StringVar(&dumpCmdDecoderFilter, "decoderfilter", "", "Decoder to filter on")
	dumpCmd.Flags().StringVar(&dumpCmdMessageFilter, "messagefilter", "", "Message to filter on")
	traceCmd.AddCommand(dumpCmd)
}

var dumpCmd = &cobra.Command{
	Use:   "dump <traceFile>",
	Short: "Dump contents of trace",
	Args:  cobra.ExactArgs(1),
	Run:   dump,
}
var dumpCmdTimeDeltas bool
var dumpCmdDecoderFilter string
var dumpCmdMessageFilter string

func dump(cmd *cobra.Command, args []string) {
	if err := trace.Read(args[0], &dumpHandler{lastTimestamp: -1}); err != nil {
		panic(err)
	}
}

type dumpHandler struct {
	lastTimestamp int64
}

func (h *dumpHandler) Handle(msg interface{}) error {
	switch msg.(type) {
	case *trace_pb.ChannelState:
		s := msg.(*trace_pb.ChannelState)
		event := "Connect"
		if s.Connected == false {
			event = "Close"
		}
		fmt.Printf("%8s: %-16s %8s %-7s %s\n", h.formatTimestamp(s.Timestamp), s.Identity, s.Channel, event, s.RemoteAddress)

		return nil

	case *trace_pb.ChannelMessage:
		t := msg.(*trace_pb.ChannelMessage)
		match, err := filterMatch(t)
		if err != nil {
			return err
		}
		if match {
			flow := "->"
			if t.IsRx {
				flow = "<-"
			}
			replyFor := ""
			if t.ReplyFor != -1 {
				replyFor = fmt.Sprintf(">%d", t.ReplyFor)
			}
			fmt.Printf("%8s: %-16s %8s %s #%-5d %5s | %s\n", h.formatTimestamp(t.Timestamp), t.Identity, t.Channel, flow, t.Sequence, replyFor, channel2.DecodeTraceAndFormat(t.Decode))

		} else {
			h.formatTimestamp(t.Timestamp)
		}

		return nil

	default:
		return errors.New("unexpected message")
	}
}

func (h *dumpHandler) formatTimestamp(timestamp int64) string {
	if dumpCmdTimeDeltas {
		if h.lastTimestamp == -1 {
			h.lastTimestamp = timestamp
			return "0"
		}
		out := fmt.Sprintf("%d", (timestamp - h.lastTimestamp) / 1000)
		h.lastTimestamp = timestamp
		return out
	}
	return fmt.Sprintf("%d", timestamp)
}

func filterMatch(trace *trace_pb.ChannelMessage) (bool, error) {
	meta := make(map[string]interface{})
	err := json.Unmarshal(trace.Decode, &meta)
	if err != nil {
		return false, err
	}

	channelMatch := true
	if dumpCmdDecoderFilter != "" {
		channelMatch = dumpCmdDecoderFilter == meta[channel2.DecoderFieldName]
	}

	messageMatch := true
	if dumpCmdMessageFilter != "" {
		messageMatch = dumpCmdMessageFilter == meta[channel2.MessageFieldName]
	}

	return channelMatch && messageMatch, nil
}