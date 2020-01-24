/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/trace"
	"github.com/netfoundry/ziti-foundation/trace/pb"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/spf13/cobra"
	"os"
	"sort"
)

func init() {
	traceCmd.AddCommand(mergeCmd)
}

var mergeCmd = &cobra.Command{
	Use:   "merge <outFile> <inFile1>...<inFileN>",
	Short: "Merge traces",
	Args:  cobra.MinimumNArgs(3),
	Run:   merge,
}

func merge(cmd *cobra.Command, args []string) {
	handler := &mergeHandler{items: make([]*mergeItem, 0)}
	for i := 1; i < len(args); i++ {
		pfxlog.Logger().Infof("reading from [%s]", args[i])
		if err := trace.Read(args[i], handler); err != nil {
			panic(err)
		}
	}
	sort.Slice(handler.items, func(i, j int) bool {
		return handler.items[i].timestamp < handler.items[j].timestamp
	})
	f, err := os.OpenFile(args[0], os.O_WRONLY|os.O_TRUNC|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	for _, i := range handler.items {
		switch i.msg.(type) {
		case *trace_pb.ChannelState:
			if err := trace.WriteChannelState(i.msg.(*trace_pb.ChannelState), f); err != nil {
				panic(err)
			}

		case *trace_pb.ChannelMessage:
			if err := trace.WriteChannelMessage(i.msg.(*trace_pb.ChannelMessage), f); err != nil {
				panic(err)
			}

		default:
			panic(errors.New("unexpected message"))
		}
	}
	pfxlog.Logger().Infof("merged [%d] items", len(handler.items))
}

type mergeItem struct {
	timestamp int64
	msg interface{}
}

type mergeHandler struct{
	items []*mergeItem
}

func (h *mergeHandler) Handle(msg interface{}) error {
	switch msg.(type) {
	case *trace_pb.ChannelState:
		h.items = append(h.items, &mergeItem{msg.(*trace_pb.ChannelState).Timestamp, msg})

	case *trace_pb.ChannelMessage:
		h.items = append(h.items, &mergeItem{msg.(*trace_pb.ChannelMessage).Timestamp, msg})

	default:
		return errors.New("unexpected message")
	}

	return nil
}