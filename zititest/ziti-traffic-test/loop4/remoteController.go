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

package loop4

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/ziti"
	loop4Pb "github.com/openziti/ziti/zititest/ziti-traffic-test/loop4/pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

type ControllerCallback interface {
	DiagnosticRequested(msg *channel.Message, ch channel.Channel)
}

func NewRemoteController(client ziti.Context, cb ControllerCallback) *RemoteController {
	return &RemoteController{
		client:         client,
		clients:        cmap.New[channel.Channel](),
		resultsTracker: cmap.New[*ScenarioResults](),
		cb:             cb,
	}
}

type RemoteController struct {
	client   ziti.Context
	clients  cmap.ConcurrentMap[string, channel.Channel]
	listener net.Listener
	cb       ControllerCallback

	resultsTracker cmap.ConcurrentMap[string, *ScenarioResults]
}

func (self *RemoteController) AcceptConnections(service string) error {
	var err error
	self.listener, err = self.client.Listen(service)
	if err != nil {
		return err
	}

	go func() {
		defer func() {
			_ = self.listener.Close()
		}()

		log := pfxlog.Logger().WithField("service", service)
		log.Info("listening for loop4.sim connections")

		for {
			conn, err := self.listener.Accept()
			if err != nil {
				log.WithError(err).Error("error accepting connection, exiting")
				return
			}

			if err = self.handleConnection(conn); err != nil {
				log.WithError(err).Error("error channelizing connection, exiting")
			}
		}
	}()

	return nil
}

func (self *RemoteController) handleConnection(conn net.Conn) error {
	tokenId, err := GetSdkIdentity(self.client)
	if err != nil {
		return err
	}
	listener := channel.NewExistingConnListener(tokenId, conn, nil)
	options := channel.DefaultOptions()

	var ch channel.Channel
	ch, err = channel.NewChannel("control", listener, channel.BindHandlerF(self.BindChannel), options)
	if err != nil {
		return fmt.Errorf("unable to establish connection from sim (%w)", err)
	}

	clientId := string(ch.Headers()[HeaderClientId])
	self.clients.Set(clientId, ch)

	pfxlog.Logger().WithField("id", clientId).Info("new sim connection established")

	return nil
}

func (self *RemoteController) BindChannel(binding channel.Binding) error {
	binding.AddReceiveHandlerF(int32(loop4Pb.ContentType_RunScenarioResultType), self.handleScenarioResult)
	binding.AddReceiveHandlerF(int32(loop4Pb.ContentType_RequestDiagnostic), self.cb.DiagnosticRequested)
	return nil
}

func (self *RemoteController) handleScenarioResult(msg *channel.Message, ch channel.Channel) {
	id, _ := msg.GetStringHeader(int32(loop4Pb.HeaderType_ScenarioId))
	if id == "" {
		pfxlog.Logger().Error("scenario result message missing scenario id")
	} else {
		results, _ := self.resultsTracker.Get(id)
		if results == nil {
			pfxlog.Logger().Errorf("scenario result message for scenario id [%s] received, but no results tracker found", id)
			return
		}

		clientId := string(ch.Headers()[HeaderClientId])

		pfxlog.Logger().
			WithField("scenarioId", id).
			WithField("clientId", clientId).
			WithField("success", success).
			Info("scenario result message received")

		success, _ := msg.GetBoolHeader(int32(loop4Pb.HeaderType_ScenarioSuccess))
		result := &ScenarioResult{
			success: success,
			message: string(msg.Body),
		}
		results.results.Set(clientId, *result)
		if results.results.Count() == results.expectedResults {
			if results.completed.CompareAndSwap(false, true) {
				close(results.complete)
			}
		}
	}
}

func (self *RemoteController) WaitForAllConnected(timeout time.Duration, components []*model.Component) error {
	start := time.Now()
	for time.Since(start) < timeout {
		if self.clients.Count() == len(components) {
			missing := self.MissingComponents(components)
			if len(missing) == 0 {
				return nil
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	missing := self.MissingComponents(components)
	return fmt.Errorf("timed out waiting for all components to connect, missing: %v", strings.Join(missing, ","))
}

func (self *RemoteController) MissingComponents(components []*model.Component) []string {
	var result []string
	for _, c := range components {
		if _, ok := self.clients.Get(c.Id); !ok {
			result = append(result, c.Id)
		}
	}
	return result
}

func (self *RemoteController) StartSimScenarios() (*ScenarioResults, error) {
	scenarioId := uuid.NewString()
	log := pfxlog.Logger().WithField("scenarioId", scenarioId)

	results := &ScenarioResults{
		id:              scenarioId,
		results:         cmap.New[ScenarioResult](),
		complete:        make(chan struct{}),
		expectedResults: self.clients.Count(),
	}

	self.resultsTracker.Set(scenarioId, results)

	for _, client := range self.clients.Items() {
		msg := channel.NewMessage(int32(loop4Pb.ContentType_RunScenarioRequestType), nil)
		msg.PutStringHeader(int32(loop4Pb.HeaderType_ScenarioId), scenarioId)
		if err := msg.WithTimeout(10 * time.Second).SendAndWaitForWire(client); err != nil {
			return nil, err
		}
		log.WithField("clientId", client.Id()).Info("scenario run request sent")
	}

	return results, nil
}

type ScenarioResult struct {
	success bool
	message string
}

type ScenarioResults struct {
	id              string
	results         cmap.ConcurrentMap[string, ScenarioResult]
	complete        chan struct{}
	completed       atomic.Bool
	expectedResults int
}

func (self *ScenarioResults) GetResults(timeout time.Duration) error {
	start := time.Now()
	var err error
	select {
	case <-self.complete:
		pfxlog.Logger().WithField("scenarioId", self.id).
			WithField("elapsed", time.Since(start)).
			Info("all scenario results gathered")
	case <-time.After(timeout):
		err = fmt.Errorf("timed out waiting for scenario results")
	}

	return self.buildResult(err)
}

func (self *ScenarioResults) buildResult(err error) error {
	var errList []error
	for id, result := range self.results.Items() {
		if !result.success {
			errList = append(errList, fmt.Errorf("client [%s] failed: %s", id, result.message))
		}
	}
	if err != nil {
		errList = append(errList, err)
	}
	return errors.Join(errList...)
}
