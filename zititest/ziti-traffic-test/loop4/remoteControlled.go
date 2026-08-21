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
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	edgeApis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/sdk-golang/v2/ziti"
	loop4Pb "github.com/openziti/ziti/zititest/ziti-traffic-test/loop4/pb"
	"github.com/spf13/cobra"
)

const (
	HeaderClientId = 100
)

func init() {
	loop4Cmd.AddCommand(newRemoteControlledCmd())
}

// scenarioResultSendTimeout bounds how long a finished scenario keeps trying to report its result
// while the sim is reconnecting. Past it the controller's own scenario timeout takes over.
const scenarioResultSendTimeout = 60 * time.Second

// completedScenarioCacheSize is how many finished scenarios are remembered for replay. The controller
// runs one scenario at a time, so a resend more than a handful of scenarios stale cannot occur.
const completedScenarioCacheSize = 16

type remoteControlledCmd struct {
	*Sim
	notifyClose chan struct{}

	// controlCh is the live channel to the sim controller. A reconnect replaces it, so anything
	// reporting to the controller must read it at send time rather than capture it: a scenario often
	// outlives the channel its run request arrived on.
	controlCh concurrenz.AtomicValue[channel.Channel]

	scenarioLock sync.Mutex
	// runningScenarioId is the scenario currently being run, empty when idle.
	runningScenarioId string
	// completed holds recent scenario outcomes for replay, oldest first in completedIds.
	completed    map[string]scenarioOutcome
	completedIds []string
}

// scenarioOutcome is a scenario run's reportable result.
type scenarioOutcome struct {
	success bool
	message string
}

// scenarioAction is what to do with an incoming run request.
type scenarioAction int

const (
	// actionRun means the sim is claimed for the scenario and the caller should run it.
	actionRun scenarioAction = iota
	// actionIgnore means this scenario is already running and will report on its own.
	actionIgnore
	// actionReplay means this scenario already finished; answer from its recorded outcome.
	actionReplay
	// actionReject means a different scenario holds the sim.
	actionReject
)

// scenarioDecision resolves one run request. runningId is set for actionReject, outcome for
// actionReplay.
type scenarioDecision struct {
	action    scenarioAction
	runningId string
	outcome   scenarioOutcome
}

// decideScenario resolves a run request for scenarioId, claiming the sim when it returns actionRun.
//
// Only one scenario runs at a time: two would reset and record the same sim-wide metrics and drive the
// same workloads, so their results would interfere.
func (cmd *remoteControlledCmd) decideScenario(scenarioId string) scenarioDecision {
	cmd.scenarioLock.Lock()
	defer cmd.scenarioLock.Unlock()

	if cmd.runningScenarioId == scenarioId {
		return scenarioDecision{action: actionIgnore}
	}
	// Ahead of the busy check on purpose: a resend for a scenario that already finished has to be
	// answered from the cache even when a later scenario now holds the sim, or it reruns work the
	// controller has already accepted and then fails the scenario that follows it.
	if outcome, found := cmd.completed[scenarioId]; found {
		return scenarioDecision{action: actionReplay, outcome: outcome}
	}
	if cmd.runningScenarioId != "" {
		// Recorded here, under the lock that saw the conflict, so the rejection is terminal. Left
		// unrecorded, a resend of this scenario would find the sim idle and start it, and the run would
		// still be going when the controller moved on, rejecting the scenario after it in turn.
		outcome := scenarioOutcome{
			success: false,
			message: fmt.Sprintf("sim is busy running scenario %s", cmd.runningScenarioId),
		}
		cmd.recordOutcome(scenarioId, outcome)
		return scenarioDecision{action: actionReject, runningId: cmd.runningScenarioId, outcome: outcome}
	}
	cmd.runningScenarioId = scenarioId
	return scenarioDecision{action: actionRun}
}

// recordOutcome caches scenarioId's terminal outcome, evicting the oldest entry when full. The first
// outcome recorded for a scenario wins. Callers must hold scenarioLock.
func (cmd *remoteControlledCmd) recordOutcome(scenarioId string, outcome scenarioOutcome) {
	if _, found := cmd.completed[scenarioId]; found {
		return
	}
	cmd.completed[scenarioId] = outcome
	cmd.completedIds = append(cmd.completedIds, scenarioId)
	if len(cmd.completedIds) > completedScenarioCacheSize {
		delete(cmd.completed, cmd.completedIds[0])
		cmd.completedIds = cmd.completedIds[1:]
	}
}

// finishScenario releases the sim and records scenarioId's outcome, so a resend that crosses with the
// result is replayed rather than run again.
func (cmd *remoteControlledCmd) finishScenario(scenarioId string, outcome scenarioOutcome) {
	cmd.scenarioLock.Lock()
	defer cmd.scenarioLock.Unlock()

	if cmd.runningScenarioId == scenarioId {
		cmd.runningScenarioId = ""
	}
	cmd.recordOutcome(scenarioId, outcome)
}

func newRemoteControlledCmd() *cobra.Command {
	dialer := &remoteControlledCmd{
		Sim:         NewSim(),
		notifyClose: make(chan struct{}, 1),
		completed:   map[string]scenarioOutcome{},
	}

	cmd := &cobra.Command{
		Use:   "remote-controlled <scenarioFile>",
		Short: "Start loop4 in remote controlled mode",
		Args:  cobra.ExactArgs(1),
		Run:   dialer.runRemoteControlled,
	}

	return cmd
}

func (cmd *remoteControlledCmd) runRemoteControlled(_ *cobra.Command, args []string) {
	defer close(cmd.closeNotify)

	if err := cmd.InitScenario(args[0]); err != nil {
		panic(err)
	}

	log := pfxlog.Logger().
		WithField("service", cmd.scenario.RemoteControlled.Service).
		WithField("connector", cmd.scenario.RemoteControlled.Connector)

	if cmd.scenario.RemoteControlled.Connector == "" {
		log.Fatal("connector for remote controller must be specified")
	}

	if cmd.scenario.RemoteControlled.Service == "" {
		log.Fatal("service for remote controller must be specified")
	}

	sdkClient := cmd.sdkClients[cmd.scenario.RemoteControlled.Connector]
	if sdkClient == nil {
		log.Fatalf("invalid connector name '%s' provided for remote controller", cmd.scenario.RemoteControlled.Connector)
		return
	}

	var lastLog time.Time

	attempt := 1
	for {
		log = log.WithField("attempt", attempt)
		conn, err := sdkClient.Dial(cmd.scenario.RemoteControlled.Service)
		if err != nil {
			if time.Since(lastLog) > 5*time.Minute {
				log.Errorf("unable to dial remote controller")
				lastLog = time.Now()
			}
			time.Sleep(1 * time.Second)
			attempt++
			continue
		}

		if err = cmd.handleRemoteControlConn(sdkClient, conn); err != nil {
			log.WithError(err).Error("unable to channelize remote controller connection")
			// Close the dialed conn before retrying. Otherwise a failed channelize (e.g. a hello
			// that times out during controller churn) leaves the edge conn and its fabric circuit
			// open with no reader, leaking a circuit on every retry.
			_ = conn.Close()
			time.Sleep(1 * time.Second)
			attempt++
			continue
		}

		<-cmd.notifyClose
	}
}

func (cmd *remoteControlledCmd) handleRemoteControlConn(sdk ziti.Context, conn net.Conn) error {
	tokenId, err := GetSdkIdentity(sdk)
	if err != nil {
		return err
	}

	currentIdentity, err := sdk.GetCurrentIdentity()
	if err != nil {
		return err
	}

	dialer := channel.NewExistingConnDialer(tokenId, conn, map[int32][]byte{
		HeaderClientId: []byte(*currentIdentity.Name),
	})
	options := channel.DefaultOptions()

	_, err = channel.NewSingleChannel("control", dialer, channel.BindHandlerF(cmd.BindChannel), options)
	if err != nil {
		return fmt.Errorf("unable to establish connection to sim controller (%w)", err)
	}

	return nil
}

func (cmd *remoteControlledCmd) BindChannel(binding channel.Binding) error {
	// Publish the channel before any handler can fire, so a request arriving immediately still has a
	// channel to answer on.
	cmd.controlCh.Store(binding.GetChannel())
	binding.AddReceiveHandlerF(int32(loop4Pb.ContentType_RunScenarioRequestType), cmd.HandleRunScenario)
	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		select {
		case cmd.notifyClose <- struct{}{}:
		default:
		}
	}))
	return nil
}

// HandleRunScenario starts a scenario on request. The channel the request arrived on is deliberately
// unused: results go to whatever channel is live when the run finishes, which a reconnect may change.
//
// Nothing here sends a result inline. This runs on the channel's receive loop, and sendScenarioResult
// retries for up to a minute, which would stall every read on that channel, heartbeats included, and
// so risk killing the connection the result still has to go out on. The replay and reject paths are
// reached during reconnect and resend handling, which is exactly when that channel is least able to
// absorb it.
func (cmd *remoteControlledCmd) HandleRunScenario(msg *channel.Message, _ channel.Channel) {
	scenarioId, _ := msg.GetStringHeader(int32(loop4Pb.HeaderType_ScenarioId))
	if scenarioId == "" {
		pfxlog.Logger().Error("run scenario request missing scenario id, ignoring")
		return
	}
	log := pfxlog.Logger().WithField("scenarioId", scenarioId)

	switch decision := cmd.decideScenario(scenarioId); decision.action {
	case actionIgnore:
		// A re-send after this sim reconnected. The run already in progress reports for it, over
		// whatever channel is live when it finishes.
		log.Info("scenario already running, ignoring duplicate run request")
		return
	case actionReplay:
		log.Info("scenario already finished, replaying its result")
		go cmd.sendScenarioResult(scenarioId, decision.outcome)
		return
	case actionReject:
		// Answer instead of running. Staying silent would leave the controller waiting out its
		// scenario timeout for a result that is never coming.
		log.WithField("runningScenarioId", decision.runningId).
			Info("another scenario is running, rejecting run request")
		go cmd.sendScenarioResult(scenarioId, decision.outcome)
		return
	}

	go func() {
		cmd.finishScenario(scenarioId, cmd.runRemoteScenario(scenarioId, cmd.scenario))
	}()
}

// sendScenarioResult reports a scenario's outcome, retrying over whatever control channel is current
// while the sim reconnects.
//
// A scenario routinely outlives the channel its run request arrived on: the controller replaces that
// channel when the sim reconnects and closes the old one. Reporting to the original channel would put
// the result nowhere, leaving the controller to wait out its scenario timeout for a run that had in
// fact finished.
func (cmd *remoteControlledCmd) sendScenarioResult(id string, outcome scenarioOutcome) {
	log := pfxlog.Logger().WithField("scenarioId", id)

	deadline := time.Now().Add(scenarioResultSendTimeout)
	for {
		msg := channel.NewMessage(int32(loop4Pb.ContentType_RunScenarioResultType), []byte(outcome.message))
		msg.PutStringHeader(int32(loop4Pb.HeaderType_ScenarioId), id)
		msg.PutBoolHeader(int32(loop4Pb.HeaderType_ScenarioSuccess), outcome.success)

		err := cmd.sendToController(msg)
		if err == nil {
			log.Info("scenario result successfully reported")
			return
		}
		if time.Now().After(deadline) {
			log.WithError(err).Errorf("giving up reporting scenario result after %s", scenarioResultSendTimeout)
			return
		}
		log.WithError(err).Info("unable to report scenario result, retrying on the current channel")
		time.Sleep(time.Second)
	}
}

func (cmd *remoteControlledCmd) sendDiagnosticRequest(requestId string) {
	log := pfxlog.Logger().WithField("requestId", requestId)
	msg := channel.NewMessage(int32(loop4Pb.ContentType_RequestDiagnostic), nil)
	msg.PutStringHeader(int32(loop4Pb.HeaderType_RequestIdHeader), requestId)
	if err := cmd.sendToController(msg); err != nil {
		log.WithError(err).Error("unable to send diagnostic request message")
	} else {
		log.Info("diagnostic successfully requested")
	}
}

// sendToController sends msg over the control channel that is live now, rather than one captured
// earlier, which a reconnect may since have replaced.
func (cmd *remoteControlledCmd) sendToController(msg *channel.Message) error {
	ch := cmd.controlCh.Load()
	if ch == nil {
		return errors.New("no control channel established")
	}
	if ch.IsClosed() {
		return errors.New("control channel is closed")
	}
	// Wait for the wire, not just the send queue: a queued result on a channel that then dies would be
	// reported as delivered, and the scenario retired as reported.
	return msg.WithTimeout(10 * time.Second).SendAndWaitForWire(ch)
}

var triggerInspectAtomic concurrenz.AtomicValue[func(circuitId string)]

func triggerInspect(circuitId string) {
	cb := triggerInspectAtomic.Load()
	if cb == nil {
		pfxlog.Logger().WithField("circuitId", circuitId).Info("trigger inspect not available")
		return
	}
	cb(circuitId)
}

func (cmd *remoteControlledCmd) runRemoteScenario(scenarioId string, scenario *Scenario) scenarioOutcome {
	log := pfxlog.Logger()

	triggerInspectAtomic.Store(func(circuitId string) {
		cmd.sendDiagnosticRequest(circuitId)
	})

	// reset metrics
	cmd.Sim.metrics.DisposeAll()

	time.AfterFunc(time.Second, func() {
		cmd.Sim.metrics.EachMetric(func(name string, metric metrics.Metric) {
			if histogram, ok := metric.(metrics.Histogram); ok {
				histogram.Clear()
			}
		})
	})

	err := cmd.runScenario(scenario)

	outcome := scenarioOutcome{success: true, message: "success"}
	if err != nil {
		outcome = scenarioOutcome{success: false, message: err.Error()}
		log.WithError(err).Errorf("scenario run unsuccessful")
	} else {
		log.Info("scenario run successful")
	}

	cmd.sendScenarioResult(scenarioId, outcome)
	return outcome
}

func GetSdkIdentity(sdk ziti.Context) (*identity.TokenId, error) {
	credentials := sdk.GetCredentials()
	var id identity.Identity
	if idProvider, ok := credentials.(edgeApis.IdentityProvider); ok {
		id = idProvider.GetIdentity()
	} else {
		return nil, errors.New("unable to get context identity, skd credentials instance is not an IdentityProvider")
	}

	tokenId := &identity.TokenId{
		Identity: id,
		Token:    id.Cert().Leaf.Subject.CommonName,
	}

	return tokenId, nil
}
