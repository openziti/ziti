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
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/raft"
	"github.com/openziti/ziti/controller/xt"
	"regexp"
	"strings"
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
				ctx.InspectLocal(requested)
			}
			close(notifier)
		}()
	} else {
		log.Debug("inspect not matched")
	}
}

func (ctx *inspectRequestContext) InspectLocal(name string) {
	lc := strings.ToLower(name)

	if lc == "stackdump" {
		result := debugz.GenerateStack()
		ctx.handleLocalStringResponse(name, &result, nil)
	} else if strings.HasPrefix(lc, "metrics") {
		msg := ctx.network.metricsRegistry.Poll()
		ctx.handleLocalJsonResponse(name, msg)
	} else if lc == "config" {
		if rc, ok := ctx.network.config.(renderConfig); ok {
			val, err := rc.RenderJsonConfig()
			ctx.handleLocalStringResponse(name, &val, err)
		}
	} else if lc == "cluster-config" {
		if src, ok := ctx.network.Dispatcher.(renderConfig); ok {
			val, err := src.RenderJsonConfig()
			ctx.handleLocalStringResponse(name, &val, err)
		}
	} else if lc == "connected-routers" {
		var result []map[string]any
		for _, r := range ctx.network.Router.AllConnected() {
			status := map[string]any{}
			status["Id"] = r.Id
			status["Name"] = r.Name
			status["Version"] = r.VersionInfo.Version
			status["ConnectTime"] = r.ConnectTime.Format(time.RFC3339)
			result = append(result, status)
		}
		ctx.handleLocalJsonResponse(name, result)
	} else if lc == "connected-peers" {
		if raftController, ok := ctx.network.Dispatcher.(*raft.Controller); ok {
			members, err := raftController.ListMembers()
			if err != nil {
				ctx.appendError(ctx.network.GetAppId(), err.Error())
				return
			}
			ctx.handleLocalJsonResponse(name, members)
		}
	} else if lc == "router-messaging" {
		routerMessagingState, err := ctx.network.RouterMessaging.Inspect()
		if err != nil {
			ctx.appendError(ctx.network.GetAppId(), err.Error())
			return
		}
		ctx.handleLocalJsonResponse(name, routerMessagingState)
	} else if strings.HasPrefix(lc, "terminator-costs") {
		state := &inspect.TerminatorCostDetails{}
		xt.GlobalCosts().IterCosts(func(terminatorId string, cost xt.Cost) {
			state.Terminators = append(state.Terminators, cost.Inspect(terminatorId))
		})
		ctx.handleLocalJsonResponse(name, state)
	} else if lc == "identity-connection-state" {
		result := ctx.network.env.GetManagers().Identity.GetConnectionTracker().Inspect()
		ctx.handleLocalJsonResponse(name, result)
	} else {
		for _, inspectTarget := range ctx.network.inspectionTargets.Value() {
			if handled, val, err := inspectTarget(lc); handled {
				ctx.handleLocalStringResponse(name, val, err)
			}
		}
	}
}

func (ctx *inspectRequestContext) handleLocalJsonResponse(key string, val interface{}) {
	js, err := json.Marshal(val)
	if err != nil {
		ctx.appendError(ctx.network.GetAppId(), fmt.Errorf("failed to marshall %s to json (%w)", key, err).Error())
	} else {
		ctx.appendValue(ctx.network.GetAppId(), key, string(js))
	}
}

func (ctx *inspectRequestContext) handleLocalStringResponse(key string, val *string, err error) {
	if err != nil {
		ctx.appendError(ctx.network.GetAppId(), err.Error())
	} else if val != nil {
		ctx.appendValue(ctx.network.GetAppId(), key, *val)
	}
}

func (ctx *inspectRequestContext) inspectRouter(router *model.Router) {
	log := pfxlog.Logger().
		WithField("appRegex", ctx.appRegex).
		WithField("routerId", router.Id).
		WithField("values", ctx.requestedValues).
		WithField("timeout", ctx.timeout)

	if ctx.regex.MatchString(router.Id) || ctx.regex.MatchString(router.Name) {
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
