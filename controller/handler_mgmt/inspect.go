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

package handler_mgmt

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/debugz"
	"regexp"
	"strings"
	"sync"
	"time"
)

type inspectHandler struct {
	network *network.Network
}

func newInspectHandler(network *network.Network) *inspectHandler {
	return &inspectHandler{network: network}
}

func (*inspectHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_InspectRequestType)
}

func (handler *inspectHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	context := &inspectRequestContext{
		handler:  handler,
		msg:      msg,
		ch:       ch,
		request:  &mgmt_pb.InspectRequest{},
		response: &mgmt_pb.InspectResponse{Success: true},
	}

	var err error
	if err = proto.Unmarshal(msg.Body, context.request); err != nil {
		context.appendError(handler.network.GetAppId().Token, err.Error())
	}

	context.regex, err = regexp.Compile(context.request.AppRegex)
	if err != nil {
		context.appendError(handler.network.GetAppId().Token, err.Error())
	}

	if !context.response.Success {
		context.sendResponse()
		return
	}

	context.processLocal()
	context.processRemote()
	context.sendResponse()
}

type inspectRequestContext struct {
	handler  *inspectHandler
	msg      *channel2.Message
	ch       channel2.Channel
	request  *mgmt_pb.InspectRequest
	response *mgmt_pb.InspectResponse
	regex    *regexp.Regexp
}

func (context *inspectRequestContext) processLocal() {
	if context.regex.MatchString(context.handler.network.GetAppId().Token) {
		for _, requested := range context.request.RequestedValues {
			if strings.ToLower(requested) == "stackdump" {
				context.appendValue(context.handler.network.GetAppId().Token, requested, debugz.GenerateStack())
			}
		}
	}
}

func (context *inspectRequestContext) processRemote() {
	routerRequest := &ctrl_pb.InspectRequest{RequestedValues: context.request.RequestedValues}
	body, err := proto.Marshal(routerRequest)
	if err != nil {
		context.appendError(context.handler.network.GetAppId().Token, err.Error())
		return
	}

	remoteResultChan := make(chan *remoteInspectResult)
	waitGroup := &sync.WaitGroup{}

	for _, router := range context.handler.network.AllConnectedRouters() {
		if context.regex.MatchString(router.Id) {
			msg := channel2.NewMessage(int32(ctrl_pb.ContentType_InspectRequestType), body)
			respCh, err := router.Control.SendAndWait(msg)
			if err != nil {
				context.appendError(router.Id, err.Error())
			} else {
				waitGroup.Add(1)
				go handleInspectResponse(router.Id, respCh, remoteResultChan, waitGroup)
			}
		}
	}

	// Close chan once all results have been queued
	go func() {
		waitGroup.Wait()
		close(remoteResultChan)
	}()

	for remoteResult := range remoteResultChan {
		for _, err := range remoteResult.result.Errors {
			context.appendError(remoteResult.appId, err)
		}
		for _, val := range remoteResult.result.Values {
			context.appendValue(remoteResult.appId, val.Name, val.Value)
		}
	}
}

func handleInspectResponse(appId string, respCh <-chan *channel2.Message, msgsCh chan<- *remoteInspectResult, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()

	timeout := time.After(time.Second * 5)

	select {
	case response, ok := <-respCh:
		if !ok {
			routerInspectFailed(appId, msgsCh, "no response from router")
		} else if response.ContentType == int32(ctrl_pb.ContentType_InspectResponseType) {
			result := &ctrl_pb.InspectResponse{}
			if err := proto.Unmarshal(response.Body, result); err != nil {
				routerInspectFailed(appId, msgsCh, fmt.Sprintf("Failed to decode response from router: %v", err))
			} else {
				msgsCh <- &remoteInspectResult{appId: appId, result: result}
			}
		} else {
			msg := fmt.Sprintf("Unexpected response type from router %v: %v\n", appId, response.ContentType)
			routerInspectFailed(appId, msgsCh, msg)
		}
	case <-timeout:
		routerInspectFailed(appId, msgsCh, "timed out waiting for response from router")
	}
}

func routerInspectFailed(appId string, msgsCh chan<- *remoteInspectResult, msg string) {
	msgsCh <- &remoteInspectResult{
		appId:  appId,
		result: &ctrl_pb.InspectResponse{Errors: []string{msg}},
	}
}

func (context *inspectRequestContext) sendResponse() {
	body, err := proto.Marshal(context.response)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error serializing InspectResponse (%s)", err)
		return
	}

	responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_InspectResponseType), body)
	responseMsg.ReplyTo(context.msg)
	if err := context.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending InspectResponse (%s)", err)
	}
}

func (context *inspectRequestContext) appendValue(appId string, name string, value string) {
	context.response.Values = append(context.response.Values, &mgmt_pb.InspectResponse_InspectValue{
		AppId: appId,
		Name:  name,
		Value: value,
	})
}

func (context *inspectRequestContext) appendError(appId string, err string) {
	context.response.Success = false
	context.response.Errors = append(context.response.Errors, fmt.Sprintf("%v: %v", appId, err))
}

type remoteInspectResult struct {
	appId  string
	result *ctrl_pb.InspectResponse
}
