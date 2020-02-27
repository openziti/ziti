/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"strings"
	"time"
)

type dialer struct {
	factory *Factory
	options *Options
}

func newDialer(factory *Factory, options *Options) xgress.Dialer {
	txd := &dialer{
		factory: factory,
		options: options,
	}
	return txd
}

func (dialer *dialer) Dial(destination string, sessionId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler) error {
	log := pfxlog.Logger().WithField("sessionId", sessionId.Token)
	destParts := strings.Split(destination, ":")
	if len(destParts) != 2 {
		return fmt.Errorf("destination '%v' format is incorrect", destination)
	}

	if destParts[0] != "hosted" {
		return fmt.Errorf("unsupported destination type: '%v'", destParts[0])
	}

	token := destParts[1]

	log.Debug("looking up hosted service conn")
	listenConn, found := dialer.factory.hostedServices.Get(token)
	if !found {
		return fmt.Errorf("host for token '%v' not found", token)
	}

	log.Debug("dialing sdk client hosting service")
	dialRequest := edge.NewDialMsg(listenConn.Id(), token)
	dialRequest.Headers[edge.PublicKeyHeader] = sessionId.Data[edge.PublicKeyHeader]

	reply, err := listenConn.SendAndWaitWithTimeout(dialRequest, 5*time.Second)
	if err != nil {
		return err
	}
	result, err := edge.UnmarshalDialResult(reply)

	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("failed to establish connection with token %v. error: (%v)", token, result.Message)
	}

	conn := listenConn.newSink(result.NewConnId, dialer.options)

	x := xgress.NewXgress(sessionId, address, conn, xgress.Terminator, &dialer.options.Options)
	bindHandler.HandleXgressBind(sessionId, address, xgress.Terminator, x)

	start := edge.NewStateConnectedMsg(result.ConnId)
	start.ReplyTo(reply)
	return listenConn.SendState(start)
}
