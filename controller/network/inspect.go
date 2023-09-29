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

package network

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/foundation/v2/concurrenz"
	"regexp"
	"sync"
	"time"
)

type InspectResultValue struct {
	AppId string
	Name  string
	Value string
}

type InspectResult struct {
	Success bool
	Errors  []string
	Results []*InspectResultValue
}

func NewInspectionsManager(network *Network) *InspectionsManager {
	return &InspectionsManager{
		network: network,
	}
}

type InspectionsManager struct {
	network *Network
}

func (self *InspectionsManager) Inspect(appRegex string, values []string) *InspectResult {
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

	return ctx.RunInspections()
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

func (ctx *inspectRequestContext) RunInspections() *InspectResult {
	log := pfxlog.Logger().
		WithField("appRegex", ctx.appRegex).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	ctx.inspectLocal()

	for _, router := range ctx.network.AllConnectedRouters() {
		ctx.inspectRouter(router)
	}

	for _, ch := range ctx.network.Dispatcher.GetPeers() {
		ctx.inspectPeer(ch.Id(), ch)
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
				result, err := ctx.network.Inspect(requested)
				if err != nil {
					ctx.appendError(ctx.network.GetAppId(), err.Error())
				} else if result != nil {
					ctx.appendValue(ctx.network.GetAppId(), requested, *result)
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
		WithField("routerId", router.Id).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	if ctx.regex.MatchString(router.Id) {
		log.Debug("inspect matched")
		notifier := make(chan struct{})
		ctx.waitGroup.AddNotifier(notifier)

		go ctx.handleCtrlChanMessaging(router.Id, router.Control, notifier)
	} else {
		log.Debug("inspect not matched")
	}
}

func (ctx *inspectRequestContext) inspectPeer(id string, ch channel.Channel) {
	log := pfxlog.Logger().
		WithField("appRegex", ctx.appRegex).
		WithField("ctrlId", id).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	if ctx.regex.MatchString(id) {
		log.Debug("inspect matched")
		notifier := make(chan struct{})
		ctx.waitGroup.AddNotifier(notifier)

		go ctx.handleCtrlChanMessaging(id, ch, notifier)
	} else {
		log.Debug("inspect not matched")
	}
}

func (ctx *inspectRequestContext) handleCtrlChanMessaging(id string, ch channel.Channel, notifier chan struct{}) {
	defer close(notifier)

	request := &ctrl_pb.InspectRequest{RequestedValues: ctx.requestedValues}
	resp := &ctrl_pb.InspectResponse{}
	respMsg, err := protobufs.MarshalTyped(request).WithTimeout(ctx.timeout).SendForReply(ch)
	err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err)
	if err != nil {
		ctx.appendError(id, err.Error())
		return
	}

	for _, err := range resp.Errors {
		ctx.appendError(id, err)
	}

	for _, val := range resp.Values {
		ctx.appendValue(id, val.Name, val.Value)
	}
}

func (ctx *inspectRequestContext) appendValue(appId string, name string, value string) {
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
