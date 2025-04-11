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

package xgress_edge

import (
	"fmt"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/router/xgress_router"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/xt"
	"github.com/openziti/ziti/router/xgress"
	"github.com/pkg/errors"
)

type dialer struct {
	factory *Factory
	options *Options
}

func (dialer *dialer) IsTerminatorValid(id string, destination string) bool {
	valid, _ := dialer.InspectTerminator(id, destination, true)
	return valid
}

func (dialer *dialer) InspectTerminator(id string, destination string, fixInvalid bool) (bool, string) {
	terminatorAddress := strings.TrimPrefix(destination, "hosted:")
	pfxlog.Logger().Debug("looking up hosted service conn")
	terminator, found := dialer.factory.hostedServices.Get(terminatorAddress)
	if found && terminator.terminatorId == id {
		dialer.factory.hostedServices.markEstablished(terminator, "validation message received")
		result, err := terminator.inspect(dialer.factory.hostedServices, fixInvalid, false)
		if err != nil {
			return true, err.Error()
		}
		return result.Type == edge.ConnTypeBind, result.Detail
	}

	return false, "terminator not found"
}

func newDialer(factory *Factory, options *Options) xgress_router.Dialer {
	txd := &dialer{
		factory: factory,
		options: options,
	}
	return txd
}

func (dialer *dialer) Dial(params xgress_router.DialParams) (xt.PeerData, error) {
	terminatorAddress := params.GetDestination()
	circuitId := params.GetCircuitId()
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(params.GetLogContext()).
		WithField("binding", "edge").
		WithField("terminatorAddress", terminatorAddress)

	terminatorAddress = strings.TrimPrefix(terminatorAddress, "hosted:")

	log.Debugf("looking up hosted service conn for address %v", terminatorAddress)
	terminator, found := dialer.factory.hostedServices.Get(terminatorAddress)
	if !found {
		return nil, xgress.InvalidTerminatorError{InnerError: fmt.Errorf("host for terminator address '%v' not found", terminatorAddress)}
	}
	log = log.WithField("bindConnId", terminator.MsgChannel.Id())

	callerId := ""
	if circuitId.Data != nil {
		if callerIdBytes, found := circuitId.Data[uint32(edge.CallerIdHeader)]; found {
			callerId = string(callerIdBytes)
		}
	}

	log.Debug("dialing sdk client hosting service")
	dialRequest := edge.NewDialMsg(terminator.Id(), terminator.token, callerId)
	dialRequest.PutStringHeader(edge.CircuitIdHeader, params.GetCircuitId().Token)
	if pk, ok := circuitId.Data[uint32(edge.PublicKeyHeader)]; ok {
		dialRequest.Headers[edge.PublicKeyHeader] = pk
	}

	if marker, ok := circuitId.Data[uint32(edge.ConnectionMarkerHeader)]; ok {
		dialRequest.Headers[edge.ConnectionMarkerHeader] = marker
	}

	appData, hasAppData := circuitId.Data[uint32(edge.AppDataHeader)]
	if hasAppData {
		dialRequest.Headers[edge.AppDataHeader] = appData
	}

	if terminator.assignIds {
		connId := terminator.nextDialConnId()
		log = log.WithField("connId", connId)
		log.Debugf("router assigned connId %v for dial", connId)
		dialRequest.PutUint32Header(edge.RouterProvidedConnId, connId)

		conn, err := terminator.newConnection(connId)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create edge xgress conn for terminator address %v", terminatorAddress)
		}

		// On the terminator, which this is, this only starts the txer, which pulls data from the link
		// Since the opposing xgress doesn't start until this call returns, nothing should be coming this way yet
		x := xgress.NewXgress(circuitId.Token, params.GetCtrlId(), params.GetAddress(), conn, xgress.Terminator, &dialer.options.Options, params.GetCircuitTags())
		params.GetBindHandler().HandleXgressBind(x)
		conn.ctrlRx = x
		x.Start()

		log.Debug("xgress start, sending dial to SDK")
		to := 5 * time.Second

		timeToDeadline := time.Until(params.GetDeadline())
		if timeToDeadline > 0 && timeToDeadline < to {
			to = timeToDeadline
		}
		log.Info("sending dial request to sdk")
		reply, err := dialRequest.WithPriority(channel.Highest).WithTimeout(to).SendForReply(terminator.GetControlSender())
		if err != nil {
			conn.close(false, err.Error())
			x.Close()
			return nil, err
		}
		result, err := edge.UnmarshalDialResult(reply)

		if err != nil {
			conn.close(false, err.Error())
			x.Close()
			return nil, err
		}

		if !result.Success {
			msg := fmt.Sprintf("failed to establish connection with terminator address %v. error: (%v)", terminatorAddress, result.Message)
			log.Info(msg)
			conn.close(false, msg)
			x.Close()
			return nil, errors.New(msg)
		}
		log.Debug("dial success")

		return nil, nil
	} else {
		log.Debug("router not assigning connId for dial")
		reply, err := dialRequest.WithPriority(channel.Highest).WithTimeout(5 * time.Second).SendForReply(terminator.GetControlSender())
		if err != nil {
			return nil, err
		}

		result, err := edge.UnmarshalDialResult(reply)
		if err != nil {
			return nil, err
		}

		if !result.Success {
			return nil, fmt.Errorf("failed to establish connection with terminator address %v. error: (%v)", terminatorAddress, result.Message)
		}

		conn, err := terminator.newConnection(result.NewConnId)
		if err != nil {
			startFail := edge.NewStateConnectedMsg(result.ConnId)
			startFail.ReplyTo(reply)

			if sendErr := terminator.SendState(startFail); sendErr != nil {
				log.Debug("failed to send state disconnected")
			}

			return nil, errors.Wrapf(err, "failed to create edge xgress conn for terminator address %v", terminatorAddress)
		}

		x := xgress.NewXgress(circuitId.Token, params.GetCtrlId(), params.GetAddress(), conn, xgress.Terminator, &dialer.options.Options, params.GetCircuitTags())
		params.GetBindHandler().HandleXgressBind(x)
		conn.ctrlRx = x
		x.Start()

		start := edge.NewStateConnectedMsg(result.ConnId)
		start.ReplyTo(reply)
		return nil, terminator.SendState(start)
	}
}

func (dialer *dialer) Inspect(key string, timeout time.Duration) any {
	if key == inspect.SdkTerminatorsKey {
		return dialer.factory.hostedServices.Inspect(timeout)
	}
	return nil
}
