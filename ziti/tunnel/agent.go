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

package tunnel

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/identity"
	sdkInspect "github.com/openziti/sdk-golang/inspect"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/agentid"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

const (
	AgentAppId byte = 3
	AgentDump  byte = 128
)

var tunnelContexts = cmap.New[ziti.Context]()

func RegisterContext(name string, ctx ziti.Context) {
	tunnelContexts.Set(name, ctx)
}

func handleAgentDump(conn net.Conn) error {
	c := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	var results []*sdkInspect.ContextInspectResult
	tunnelContexts.IterCb(func(key string, ctx ziti.Context) {
		results = append(results, ctx.Inspect())
	})

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal tunnel dump")
		_, _ = c.WriteString("error: " + err.Error() + "\n")
		return c.Flush()
	}

	_, _ = c.Write(data)
	_, _ = c.WriteString("\n")
	return c.Flush()
}

// HandleAgentAsyncOp handles channel-based agent operations for the tunnel, enabling commands
// like inspect that use typed protobuf messages.
func HandleAgentAsyncOp(conn net.Conn) error {
	appIdBuf := []byte{0}
	if _, err := io.ReadFull(conn, appIdBuf); err != nil {
		return err
	}
	appId := appIdBuf[0]

	if appId != agentid.AppIdAny && appId != AgentAppId {
		return errors.New("invalid operation for tunnel")
	}

	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	listener := channel.NewExistingConnListener(&identity.TokenId{Token: "tunnel"}, conn, nil)
	_, err := channel.NewChannel("agent", listener, channel.BindHandlerF(bindAgentChannel), options)
	return err
}

func bindAgentChannel(binding channel.Binding) error {
	binding.AddReceiveHandlerF(int32(ctrl_pb.ContentType_InspectRequestType), handleAgentInspect)
	return nil
}

func handleAgentInspect(m *channel.Message, ch channel.Channel) {
	log := pfxlog.Logger()

	request := &ctrl_pb.InspectRequest{}
	if err := proto.Unmarshal(m.Body, request); err != nil {
		log.WithError(err).Error("failed to unmarshal inspect request")
		return
	}

	response := &ctrl_pb.InspectResponse{Success: true}

	for _, requested := range request.RequestedValues {
		lc := strings.ToLower(requested)
		if lc == "stackdump" {
			val := debugz.GenerateStack()
			response.Values = append(response.Values, &ctrl_pb.InspectResponse_InspectValue{
				Name:  requested,
				Value: val,
			})
		} else if lc == "sdk" {
			var results []*sdkInspect.ContextInspectResult
			tunnelContexts.IterCb(func(key string, ctx ziti.Context) {
				results = append(results, ctx.Inspect())
			})
			data, err := json.Marshal(results)
			if err != nil {
				response.Success = false
				response.Errors = append(response.Errors, err.Error())
			} else {
				response.Values = append(response.Values, &ctrl_pb.InspectResponse_InspectValue{
					Name:  requested,
					Value: string(data),
				})
			}
		}
	}

	body, err := proto.Marshal(response)
	if err != nil {
		log.WithError(err).Error("failed to marshal inspect response")
		return
	}

	responseMsg := channel.NewMessage(int32(ctrl_pb.ContentType_InspectResponseType), body)
	responseMsg.ReplyTo(m)
	if err := ch.Send(responseMsg); err != nil {
		log.WithError(err).Error("failed to send inspect response")
	}
}
