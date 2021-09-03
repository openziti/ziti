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
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel2"
	trace_pb "github.com/openziti/foundation/trace/pb"
	"sync"
	"time"
)

type traceTogglePipeHandler struct {
	network *network.Network
}

func newTogglePipeTracesHandler(network *network.Network) *traceTogglePipeHandler {
	return &traceTogglePipeHandler{network: network}
}

func (*traceTogglePipeHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_TogglePipeTracesRequestType)
}

func (handler *traceTogglePipeHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &trace_pb.TogglePipeTracesRequest{}

	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	matchers, result := trace.NewPipeToggleMatchers(request)

	if !result.Success {
		handler.complete(msg, ch, result)
		return
	}

	resultChan := make(chan trace.ToggleApplyResult)

	verbosity := trace.GetVerbosity(request.Verbosity)

	if checkMatch(handler.network.GetAppId(), matchers, verbosity, result) {
		if request.Enable {
			handler.network.GetTraceController().EnableTracing(trace.SourceTypePipe, matchers.PipeMatcher, resultChan)
		} else {
			handler.network.GetTraceController().DisableTracing(trace.SourceTypePipe, matchers.PipeMatcher, resultChan)
		}
		getApplyResults(resultChan, verbosity, result)
	}

	if !result.Success {
		handler.complete(msg, ch, result)
		return
	}

	remoteResultChan := make(chan *remoteToggleResult)
	waitGroup := &sync.WaitGroup{}

	for _, router := range handler.network.AllConnectedRouters() {
		if checkMatch(router.Id, matchers, verbosity, result) {
			msg := channel2.NewMessage(int32(ctrl_pb.ContentType_TogglePipeTracesRequestType), msg.Body)
			respCh, err := router.Control.SendAndWait(msg)
			if err != nil {
				result.Success = false
				result.Append(fmt.Sprintf("Trace enable of Router %v failed with: %v", router.Id, err))
			} else {
				waitGroup.Add(1)
				go handleResponse(router.Id, respCh, remoteResultChan, waitGroup)
			}
		}
	}

	// Close chan once all results have been queued
	go func() {
		waitGroup.Wait()
		close(remoteResultChan)
	}()

	for remoteToggleResult := range remoteResultChan {
		if !remoteToggleResult.success {
			result.Success = false
		}
		result.Message.WriteString(remoteToggleResult.message)
	}

	handler.complete(msg, ch, result)
}

func (handler *traceTogglePipeHandler) complete(msg *channel2.Message, ch channel2.Channel, result *trace.ToggleResult) {
	if result.Success {
		handler_common.SendSuccess(msg, ch, result.Message.String())
	} else {
		handler_common.SendFailure(msg, ch, result.Message.String())
	}
}

func checkMatch(appId string, matchers *trace.PipeToggleMatchers, verbosity trace.ToggleVerbosity, result *trace.ToggleResult) bool {
	appMatches := matchers.AppMatcher.Matches(appId)
	applyResult := &trace.ToggleApplyResultImpl{
		Matched: appMatches,
		Message: fmt.Sprintf("App %v matched? %v", appId, appMatches),
	}
	applyResult.Append(result, verbosity)
	return appMatches
}

func getApplyResults(resultChan chan trace.ToggleApplyResult, verbosity trace.ToggleVerbosity, result *trace.ToggleResult) {
	timout := time.After(time.Second * 5)
	for {
		select {
		case applyResult, chanOpen := <-resultChan:
			if !chanOpen {
				return
			}
			applyResult.Append(result, verbosity)
		case <-timout:
			result.Success = false
			result.Append("Timed out waiting for toggle to be applied to controller")
			return
		}
	}
}

func handleResponse(targetId string, respCh <-chan *channel2.Message, msgsCh chan<- *remoteToggleResult, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()

	timeout := time.After(time.Second * 5)

	select {
	case response, ok := <-respCh:
		if !ok {
			msg := fmt.Sprintf("No response from router %v\n", targetId)
			msgsCh <- &remoteToggleResult{false, msg}
		} else if response.ContentType == channel2.ContentTypeResultType {
			result := channel2.UnmarshalResult(response)
			msgsCh <- &remoteToggleResult{result.Success, result.Message}
		} else {
			msg := fmt.Sprintf("Unexpected response type from router %v: %v\n", targetId, response.ContentType)
			msgsCh <- &remoteToggleResult{false, msg}
		}
	case <-timeout:
		msg := fmt.Sprintf("Timed out waiting for response from router %v\n", targetId)
		msgsCh <- &remoteToggleResult{false, msg}
	}
}

type remoteToggleResult struct {
	success bool
	message string
}
