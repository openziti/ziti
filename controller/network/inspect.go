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

package network

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/util/concurrenz"
	"regexp"
	"strings"
	"sync"
	"time"
)

type InspectResultValue struct {
	AppId string
	Name  string
	Value interface{}
}

type InspectResult struct {
	Success bool
	Errors  []string
	Results []*InspectResultValue
}

type InspectionsController struct {
	network *Network
}

func (self *InspectionsController) Inspect(appRegex string, values []string) *InspectResult {
	ctx := &inspectRequestContext{
		network:         self.network,
		timeout:         time.Second * 10,
		requestedValues: values,
		waitGroup:       concurrenz.NewWaitGroup(),
		appRegex:        appRegex,
		response:        InspectResult{Success: true},
	}

	var err error
	ctx.regex, err = regexp.Compile(appRegex)
	if err != nil {
		ctx.appendError(self.network.GetAppId(), err.Error())
		return &ctx.response
	}

	return ctx.runInspections()
}

type inspectRequestContext struct {
	network         *Network
	timeout         time.Duration
	requestedValues []string
	waitGroup       concurrenz.WaitGroup
	response        InspectResult
	appRegex        string
	regex           *regexp.Regexp
	lock            sync.Mutex
	complete        bool
}

func (ctx *inspectRequestContext) runInspections() *InspectResult {
	log := pfxlog.Logger().
		WithField("appRegex", ctx.appRegex).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	ctx.inspectLocal()

	for _, router := range ctx.network.AllConnectedRouters() {
		ctx.inspectRouter(router)
	}

	if !ctx.waitGroup.WaitForDone(ctx.timeout) {
		log.Info("inspect timed out, some values may be missing")
	}

	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	ctx.complete = true
	return &ctx.response
}

func (ctx *inspectRequestContext) inspectLocal() {
	log := pfxlog.Logger().
		WithField("appRegex", ctx.appRegex).
		WithField("appId", ctx.network.GetAppId()).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	if ctx.regex.MatchString(ctx.network.GetAppId()) {
		log.Debug("inspect matched")
		notifier := make(chan struct{})
		ctx.waitGroup.AddNotifier(notifier)
		go func() {
			for _, requested := range ctx.requestedValues {
				result := ctx.network.Inspect(requested)
				if result != nil {
					ctx.appendValue(ctx.network.GetAppId(), requested, result)
				}
			}
			close(notifier)
		}()
	} else {
		log.Debug("inspect not matched")
	}
}

func (ctx *inspectRequestContext) inspectRouter(router *Router) {
	log := pfxlog.Logger().
		WithField("appRegex", ctx.appRegex).
		WithField("appId", router.Id).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	if ctx.regex.MatchString(router.Id) {
		log.Debug("inspect matched")
		notifier := make(chan struct{})
		ctx.waitGroup.AddNotifier(notifier)

		go ctx.handleRouterMessaging(router, notifier)
	} else {
		log.Debug("inspect not matched")
	}
}

func (ctx *inspectRequestContext) handleRouterMessaging(router *Router, notifier chan struct{}) {
	defer close(notifier)

	request := &ctrl_pb.InspectRequest{RequestedValues: ctx.requestedValues}
	resp := &ctrl_pb.InspectResponse{}
	respMsg, err := protobufs.MarshalTyped(request).WithTimeout(ctx.timeout).SendForReply(router.Control)
	err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err)
	if err != nil {
		ctx.appendError(router.Id, err.Error())
		return
	}

	for _, err := range resp.Errors {
		ctx.appendError(router.Id, err)
	}

	for _, val := range resp.Values {
		handled := false
		if strings.HasPrefix(val.Value, "{") {
			mapVal := map[string]interface{}{}
			if err := json.Unmarshal([]byte(val.Value), &mapVal); err == nil {
				ctx.appendValue(router.Id, val.Name, mapVal)
				handled = true
			}
		} else if strings.HasPrefix(val.Value, "[") {
			var arrayVal []interface{}
			if err := json.Unmarshal([]byte(val.Value), &arrayVal); err == nil {
				ctx.appendValue(router.Id, val.Name, arrayVal)
				handled = true
			}
		}
		if !handled {
			ctx.appendValue(router.Id, val.Name, val.Value)
		}
	}
}

func (ctx *inspectRequestContext) appendValue(appId string, name string, value interface{}) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	if !ctx.complete {
		ctx.response.Results = append(ctx.response.Results, &InspectResultValue{
			AppId: appId,
			Name:  name,
			Value: value,
		})
	}
}

func (ctx *inspectRequestContext) appendError(appId string, err string) {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()
	if !ctx.complete {
		ctx.response.Success = false
		ctx.response.Errors = append(ctx.response.Errors, fmt.Sprintf("%v: %v", appId, err))
	}
}
