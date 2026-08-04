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
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/v2/ziti"
	loop4Pb "github.com/openziti/ziti/zititest/ziti-traffic-test/loop4/pb"
	cmap "github.com/orcaman/concurrent-map/v2"
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
	closed   atomic.Bool

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
				if self.closed.Load() {
					log.Info("listener closed, exiting accept loop")
					return
				}
				log.WithError(err).Error("error accepting connection, continuing")
				continue
			}

			if err = self.handleConnection(conn); err != nil {
				log.WithError(err).Error("error channelizing connection")
			}
		}
	}()

	return nil
}

func (self *RemoteController) Close() error {
	self.closed.Store(true)
	return self.listener.Close()
}

func (self *RemoteController) handleConnection(conn net.Conn) error {
	// handleConnection owns conn: on success it is owned by the channel wrapping it (whose Close closes
	// it), so every error path must close it here or the accepted conn and its circuit leak.
	tokenId, err := GetSdkIdentity(self.client)
	if err != nil {
		_ = conn.Close()
		return err
	}
	listener := channel.NewExistingConnListener(tokenId, conn, nil)
	options := channel.DefaultOptions()

	var ch channel.Channel
	ch, err = channel.NewSingleChannel("control", listener, channel.BindHandlerF(self.BindChannel), options)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("unable to establish connection from sim (%w)", err)
	}

	clientId := string(ch.Headers()[HeaderClientId])
	// Install the new channel and capture any channel it supersedes. A reconnecting sim opens a new
	// channel while its old one is still registered; without closing the old one it (and its underlying
	// edge conn and fabric circuit) leaks, since nothing else ever closes it. A leaked circuit stays
	// valid to the fabric forever, so the peer never receives a close and can block on it indefinitely.
	// The old channel must be closed after Upsert returns, not inside the callback: Upsert holds the
	// shard lock across the callback, and Close runs the close handler synchronously, which calls
	// RemoveCb and would deadlock trying to re-acquire that same lock.
	var superseded channel.Channel
	self.clients.Upsert(clientId, ch, func(exists bool, oldCh channel.Channel, newCh channel.Channel) channel.Channel {
		if exists && oldCh != nil && oldCh != newCh {
			superseded = oldCh
		}
		return newCh
	})
	if superseded != nil {
		_ = superseded.Close()
	}

	pfxlog.Logger().WithField("id", clientId).Info("new sim connection established")

	self.resendPendingScenarios(clientId, ch)

	return nil
}

// resendPendingScenarios re-sends the run request for every in-flight scenario that still expects a
// result from this client. A sim that reconnects mid-scenario gets a fresh channel that never received
// the original request; without re-sending, its result would never arrive and the scenario would block
// until it timed out.
func (self *RemoteController) resendPendingScenarios(clientId string, ch channel.Channel) {
	for _, results := range self.resultsTracker.Items() {
		if results.done.Load() || results.completed.Load() {
			continue
		}
		if _, expected := results.expected[clientId]; !expected {
			continue
		}
		if results.results.Has(clientId) {
			continue
		}
		log := pfxlog.Logger().WithField("scenarioId", results.id).WithField("clientId", clientId)
		if err := self.sendScenarioRequest(results.id, ch); err != nil {
			log.WithError(err).Error("failed to re-send scenario request to reconnected sim")
		} else {
			log.Info("re-sent scenario request to reconnected sim")
		}
	}
}

func (self *RemoteController) BindChannel(binding channel.Binding) error {
	binding.AddReceiveHandlerF(int32(loop4Pb.ContentType_RunScenarioResultType), self.handleScenarioResult)
	if self.cb != nil {
		binding.AddReceiveHandlerF(int32(loop4Pb.ContentType_RequestDiagnostic), self.cb.DiagnosticRequested)
	}
	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		clientId := string(ch.Headers()[HeaderClientId])
		// Only remove the entry if it still points at this channel. A reconnecting sim registers
		// its new channel before the old channel's close handler runs; removing unconditionally
		// would evict the live replacement and make a healthy sim look disconnected.
		removed := self.clients.RemoveCb(clientId, func(_ string, v channel.Channel, _ bool) bool {
			return v == ch
		})
		if removed {
			pfxlog.Logger().WithField("id", clientId).Info("sim client channel closed, removed from clients map")
		} else {
			pfxlog.Logger().WithField("id", clientId).Info("stale sim channel closed; superseded by newer connection, not removing")
		}
	}))
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

		// Ignore results from clients that were not part of this scenario's dispatch set (e.g. a sim that
		// connected after the scenario started). Counting them would corrupt the completion check.
		if _, expected := results.expected[clientId]; !expected {
			pfxlog.Logger().WithField("scenarioId", id).WithField("clientId", clientId).
				Info("ignoring scenario result from client not in scenario's expected set")
			return
		}

		success, _ := msg.GetBoolHeader(int32(loop4Pb.HeaderType_ScenarioSuccess))

		pfxlog.Logger().
			WithField("scenarioId", id).
			WithField("clientId", clientId).
			WithField("success", success).
			Info("scenario result message received")

		result := &ScenarioResult{
			success: success,
			message: string(msg.Body),
		}
		results.results.Set(clientId, *result)
		if results.results.Count() == len(results.expected) {
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
		ch, ok := self.clients.Get(c.Id)
		if !ok || ch.IsClosed() {
			result = append(result, c.Id)
		}
	}
	return result
}

func (self *RemoteController) StartSimScenarios() (*ScenarioResults, error) {
	scenarioId := uuid.NewString()
	log := pfxlog.Logger().WithField("scenarioId", scenarioId)

	clients := self.clients.Items()
	expected := make(map[string]struct{}, len(clients))
	for clientId := range clients {
		expected[clientId] = struct{}{}
	}

	results := &ScenarioResults{
		id:       scenarioId,
		results:  cmap.New[ScenarioResult](),
		complete: make(chan struct{}),
		expected: expected,
	}

	self.resultsTracker.Set(scenarioId, results)

	for clientId, client := range clients {
		if err := self.sendScenarioRequest(scenarioId, client); err != nil {
			return nil, err
		}
		log.WithField("clientId", clientId).Info("scenario run request sent")
	}

	return results, nil
}

// sendScenarioRequest sends one run-scenario request for the given scenario to a single sim channel.
func (self *RemoteController) sendScenarioRequest(scenarioId string, client channel.Channel) error {
	msg := channel.NewMessage(int32(loop4Pb.ContentType_RunScenarioRequestType), nil)
	msg.PutStringHeader(int32(loop4Pb.HeaderType_ScenarioId), scenarioId)
	return msg.WithTimeout(10 * time.Second).SendAndWaitForWire(client)
}

type ScenarioResult struct {
	success bool
	message string
}

type ScenarioResults struct {
	id        string
	results   cmap.ConcurrentMap[string, ScenarioResult]
	complete  chan struct{}
	completed atomic.Bool
	// expected holds the client ids the scenario was dispatched to. It is populated before the results
	// tracker is published and never mutated afterward, so it is safe for concurrent reads.
	expected map[string]struct{}
	// done marks the scenario as no longer awaited (GetResults has returned), so a reconnecting sim
	// stops getting the run request re-sent for it.
	done atomic.Bool
}

func (self *ScenarioResults) GetResults(timeout time.Duration) error {
	defer self.done.Store(true)
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
