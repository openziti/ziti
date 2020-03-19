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
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	streamSessionsClient = NewMgmtClient(streamSessionsCmd)
	streamCmd.AddCommand(streamSessionsCmd)
}

var streamSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Stream session data as sessions are created/destroyed",
	Run:   streamSessions,
}
var streamSessionsClient *mgmtClient

func streamSessions(cmd *cobra.Command, args []string) {
	ch, err := streamSessionsClient.Connect()
	if err != nil {
		panic(err)
	}

	request := &mgmt_pb.StreamSessionsRequest{}
	body, err := proto.Marshal(request)
	if err != nil {
		panic(err)
	}

	ch.AddReceiveHandler(&sessionsHandler{})
	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_StreamSessionsRequestType), body)

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

type sessionsHandler struct{}

func (*sessionsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamSessionsEventType)
}

func (*sessionsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	event := &mgmt_pb.StreamSessionsEvent{}
	err := proto.Unmarshal(msg.Body, event)
	if err != nil {
		panic(err)
	}

	eventType := mgmt_pb.StreamSessionEventType_name[int32(event.EventType)]
	if event.EventType == mgmt_pb.StreamSessionEventType_SessionDeleted {
		fmt.Printf("%v: sessionId: %v\n", eventType, event.SessionId)
	} else if event.EventType == mgmt_pb.StreamSessionEventType_SessionCreated {
		fmt.Printf("%v: sessionId: %v, clientId: %v, serviceId: %v, path: %v\n",
			eventType, event.SessionId, event.ClientId, event.ServiceId, event.Circuit.CalculateDisplayPath())
	} else if event.EventType == mgmt_pb.StreamSessionEventType_CircuitUpdated {
		fmt.Printf("%v: sessionId: %v, path: %v\n",
			eventType, event.SessionId, event.Circuit.CalculateDisplayPath())
	}
}
