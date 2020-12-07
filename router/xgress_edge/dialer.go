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
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/sdk-golang/ziti/edge"
	log "github.com/sirupsen/logrus"
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

	log.Debug("looking up hosted service conn")
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

func (dialer *dialer) Dial(destination string, sessionId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler) (xt.PeerData, error) {
	log := pfxlog.Logger().WithField("token", sessionId.Token)
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

	callerId := ""
	if sessionId.Data != nil {
		if callerIdBytes, found := sessionId.Data[edge.CallerIdHeader]; found {
			callerId = string(callerIdBytes)
		}
	}

	log.Debug("dialing sdk client hosting service")
	dialRequest := edge.NewDialMsg(listenConn.Id(), token, callerId)
	dialRequest.Headers[edge.PublicKeyHeader] = sessionId.Data[edge.PublicKeyHeader]
	appData, hasAppData := sessionId.Data[edge.AppDataHeader]
	if hasAppData {
		dialRequest.Headers[edge.AppDataHeader] = appData
	}

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

	conn := listenConn.newSink(result.NewConnId)

	x := xgress.NewXgress(sessionId, address, conn, xgress.Terminator, &dialer.options.Options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	start := edge.NewStateConnectedMsg(result.ConnId)
	start.ReplyTo(reply)
	return nil, listenConn.SendState(start)
}
