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
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	streamCircuitsClient = NewMgmtClient(streamCircuitsCmd)
	streamCmd.AddCommand(streamCircuitsCmd)
}

var streamCircuitsCmd = &cobra.Command{
	Use:   "circuits",
	Short: "Stream circuit data as circuits are created/destroyed",
	Run:   streamCircuits,
}
var streamCircuitsClient *mgmtClient

func streamCircuits(*cobra.Command, []string) {
	ch, err := streamCircuitsClient.Connect()
	if err != nil {
		panic(err)
	}

	request := &mgmt_pb.StreamCircuitsRequest{}
	body, err := proto.Marshal(request)
	if err != nil {
		panic(err)
	}

	ch.AddReceiveHandler(&circuitsHandler{})
	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_StreamCircuitsRequestType), body)

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

type circuitsHandler struct{}

func (*circuitsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamCircuitsEventType)
}

func (*circuitsHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
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
