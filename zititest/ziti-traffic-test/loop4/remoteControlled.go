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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	edgeApis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	loop4Pb "github.com/openziti/ziti/zititest/ziti-traffic-test/loop4/pb"
	"github.com/spf13/cobra"
	"net"
	"time"
)

const (
	HeaderClientId = 100
)

func init() {
	loop4Cmd.AddCommand(newRemoteControlledCmd())
}

type remoteControlledCmd struct {
	*Sim
	notifyClose chan struct{}
}

func newRemoteControlledCmd() *cobra.Command {
	dialer := &remoteControlledCmd{
		Sim:         NewSim(),
		notifyClose: make(chan struct{}, 1),
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

	_, err = channel.NewChannel("control", dialer, channel.BindHandlerF(cmd.BindChannel), options)
	if err != nil {
		return fmt.Errorf("unable to establish connection to sim controller (%w)", err)
	}

	return nil
}

func (cmd *remoteControlledCmd) BindChannel(binding channel.Binding) error {
	binding.AddReceiveHandlerF(int32(loop4Pb.ContentType_RunScenarioRequestType), cmd.HandleRunScenario)
	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		select {
		case cmd.notifyClose <- struct{}{}:
		default:
		}
	}))
	return nil
}

func (cmd *remoteControlledCmd) HandleRunScenario(msg *channel.Message, ch channel.Channel) {
	scenarioId, _ := msg.GetStringHeader(int32(loop4Pb.HeaderType_ScenarioId))
	go cmd.runRemoteScenario(scenarioId, cmd.scenario, ch)
}

func (cmd *remoteControlledCmd) sendScenarioResult(ch channel.Channel, id string, success bool, result string) {
	log := pfxlog.Logger().WithField("scenarioId", id)

	msg := channel.NewMessage(int32(loop4Pb.ContentType_RunScenarioResultType), []byte(result))
	msg.PutStringHeader(int32(loop4Pb.HeaderType_ScenarioId), id)
	msg.PutBoolHeader(int32(loop4Pb.HeaderType_ScenarioSuccess), success)
	if err := msg.WithTimeout(10 * time.Second).Send(ch); err != nil {
		log.WithError(err).Error("unable to send scenario run result message")
	} else {
		log.Info("scenario result successfully reported")
	}
}

func (cmd *remoteControlledCmd) sendDiagnosticRequest(ch channel.Channel, requestId string) {
	log := pfxlog.Logger().WithField("requestId", requestId)
	msg := channel.NewMessage(int32(loop4Pb.ContentType_RequestDiagnostic), nil)
	msg.PutStringHeader(int32(loop4Pb.HeaderType_RequestIdHeader), requestId)
	if err := msg.WithTimeout(10 * time.Second).Send(ch); err != nil {
		log.WithError(err).Error("unable to send diagnostic request message")
	} else {
		log.Info("diagnostic successfully requested")
	}
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

func (cmd *remoteControlledCmd) runRemoteScenario(scenarioId string, scenario *Scenario, ch channel.Channel) {
	log := pfxlog.Logger()

	triggerInspectAtomic.Store(func(circuitId string) {
		cmd.sendDiagnosticRequest(ch, circuitId)
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

	runSucceeded := true
	resultMsg := "success"
	if err != nil {
		runSucceeded = false
		resultMsg = err.Error()
		log.WithError(err).Errorf("scenario run unsuccessful")
	} else {
		log.Info("scenario run successful")
	}

	cmd.sendScenarioResult(ch, scenarioId, runSucceeded, resultMsg)
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
