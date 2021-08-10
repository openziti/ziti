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

package xgress_edge

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"strings"
	"time"
)

type dialer struct {
	factory *Factory
	options *Options
}

func (dialer dialer) IsTerminatorValid(id string, destination string) bool {
	destParts := strings.Split(destination, ":")
	if len(destParts) != 2 {
		return false
	}

	if destParts[0] != "hosted" {
		return false
	}

	token := destParts[1]

	pfxlog.Logger().Debug("looking up hosted service conn")
	_, found := dialer.factory.hostedServices.Get(token)
	return found
}

func newDialer(factory *Factory, options *Options) xgress.Dialer {
	txd := &dialer{
		factory: factory,
		options: options,
	}
	return txd
}

func (dialer *dialer) Dial(destination string, circuitId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler, ctx logcontext.Context) (xt.PeerData, error) {
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).
		WithField("binding", "edge").
		WithField("destination", destination)

	destParts := strings.Split(destination, ":")
	if len(destParts) != 2 {
		return nil, fmt.Errorf("destination '%v' format is incorrect", destination)
	}

	if destParts[0] != "hosted" {
		return nil, fmt.Errorf("unsupported destination type: '%v'", destParts[0])
	}

	token := destParts[1]

	log.Debugf("looking up hosted service conn for token %v", token)
	listenConn, found := dialer.factory.hostedServices.Get(token)
	if !found {
		return nil, fmt.Errorf("host for token '%v' not found", token)
	}
	log = log.WithField("bindConnId", listenConn.Id())

	callerId := ""
	if circuitId.Data != nil {
		if callerIdBytes, found := circuitId.Data[edge.CallerIdHeader]; found {
			callerId = string(callerIdBytes)
		}
	}

	log.Debug("dialing sdk client hosting service")
	dialRequest := edge.NewDialMsg(listenConn.Id(), token, callerId)
	if pk, ok := circuitId.Data[edge.PublicKeyHeader]; ok {
		dialRequest.Headers[edge.PublicKeyHeader] = pk
	}

	appData, hasAppData := circuitId.Data[edge.AppDataHeader]
	if hasAppData {
		dialRequest.Headers[edge.AppDataHeader] = appData
	}

	if listenConn.assignIds {
		connId := listenConn.nextDialConnId()
		log = log.WithField("connId", connId)
		log.Debugf("router assigned connId %v for dial", connId)
		dialRequest.PutUint32Header(edge.RouterProvidedConnId, connId)

		conn, err := listenConn.newConnection(connId)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create edge xgress conn for token %v", token)
		}

		// On the terminator, which this is, this only starts the txer, which pulls data from the link
		// Since the opposing xgress doesn't start until this call returns, nothing should be coming this way yet
		x := xgress.NewXgress(circuitId, address, conn, xgress.Terminator, &dialer.options.Options)
		bindHandler.HandleXgressBind(x)
		x.Start()

		log.Debug("xgress start, sending dial to SDK")
		reply, err := listenConn.SendPrioritizedAndWaitWithTimeout(dialRequest, channel2.Highest, 5*time.Second)
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
			msg := fmt.Sprintf("failed to establish connection with token %v. error: (%v)", token, result.Message)
			conn.close(false, msg)
			x.Close()
			return nil, fmt.Errorf(msg)
		}
		log.Debug("dial success")

		return nil, nil
	} else {
		log.Debug("router not assigning connId for dial")
		reply, err := listenConn.SendPrioritizedAndWaitWithTimeout(dialRequest, channel2.Highest, 5*time.Second)
		if err != nil {
			return nil, err
		}

		result, err := edge.UnmarshalDialResult(reply)
		if err != nil {
			return nil, err
		}

		if !result.Success {
			return nil, fmt.Errorf("failed to establish connection with token %v. error: (%v)", token, result.Message)
		}

		conn, err := listenConn.newConnection(result.NewConnId)
		if err != nil {
			startFail := edge.NewStateConnectedMsg(result.ConnId)
			startFail.ReplyTo(reply)

			if sendErr := listenConn.SendState(startFail); sendErr != nil {
				log.Debug("failed to send state disconnected")
			}

			return nil, errors.Wrapf(err, "failed to create edge xgress conn for token %v", token)
		}

		x := xgress.NewXgress(circuitId, address, conn, xgress.Terminator, &dialer.options.Options)
		bindHandler.HandleXgressBind(x)
		x.Start()

		start := edge.NewStateConnectedMsg(result.ConnId)
		start.ReplyTo(reply)
		return nil, listenConn.SendState(start)
	}
}
