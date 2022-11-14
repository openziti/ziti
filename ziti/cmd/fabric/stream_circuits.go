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
	"time"
)

type streamCircuitsAction struct {
	api.Options
}

func NewStreamCircuitsCmd(p common.OptionsProvider) *cobra.Command {
	action := streamCircuitsAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	streamCircuitsCmd := &cobra.Command{
		Use:   "circuits",
		Short: "Stream circuit data as circuits are created/destroyed",
		Args:  cobra.MaximumNArgs(0),
		Run:   action.streamCircuits,
	}

	action.AddCommonFlags(streamCircuitsCmd)

	return streamCircuitsCmd
}

func (self *streamCircuitsAction) streamCircuits(*cobra.Command, []string) {
	closeNotify := make(chan struct{})

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_StreamCircuitsEventType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		panic(err)
	}

	requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamCircuitsRequestType), nil)
	if err = requestMsg.WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
		panic(err)
	}

	<-closeNotify
}

func (self *streamCircuitsAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	event := &mgmt_pb.StreamCircuitsEvent{}
	err := proto.Unmarshal(msg.Body, event)
	if err != nil {
		panic(err)
	}

	eventType := mgmt_pb.StreamCircuitEventType_name[int32(event.EventType)]
	if event.EventType == mgmt_pb.StreamCircuitEventType_CircuitDeleted {
		fmt.Printf("%v: circuitId: %v\n", eventType, event.CircuitId)
	} else if event.EventType == mgmt_pb.StreamCircuitEventType_CircuitCreated {
		fmt.Printf("%v: circuitId: %v, clientId: %v, serviceId: %v, path: %v\n",
			eventType, event.CircuitId, event.ClientId, event.ServiceId, event.Path.CalculateDisplayPath())
	} else if event.EventType == mgmt_pb.StreamCircuitEventType_PathUpdated {
		fmt.Printf("%v: circuitId: %v, path: %v\n",
			eventType, event.CircuitId, event.Path.CalculateDisplayPath())
	}
}
