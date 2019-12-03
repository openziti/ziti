/*
	Copyright 2019 Netfoundry, Inc.

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

package xgress_transport

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
)

type listener struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	options *xgress.Options
}

func newListener(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) xgress.XgressListener {
	return &listener{
		id:      id,
		ctrl:    ctrl,
		options: options,
	}
}

func (listener *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	txAddress, err := transport.ParseAddress(address)
	if err != nil {
		return fmt.Errorf("cannot listen on invalid address [%s] (%s)", address, err)
	}

	incomingPeers := make(chan transport.Connection)
	go txAddress.MustListen("tcp", listener.id, incomingPeers)
	go func() {
		for {
			select {
			case peer := <-incomingPeers:
				if peer != nil {
					go listener.handleConnect(peer, bindHandler)
				} else {
					return
				}
			}
		}
	}()

	return nil
}

func (listener *listener) handleConnect(peer transport.Connection, bindHandler xgress.BindHandler) {
	conn := &transportXgresscConn{peer}
	log := pfxlog.ContextLogger(conn.LogContext())

	request, err := xgress.ReceiveRequest(peer)
	if err == nil {
		response := xgress.CreateSession(listener.ctrl, conn, request, bindHandler, listener.options)
		err = xgress.SendResponse(response, peer.Writer())
		if err != nil {
			log.Errorf("error sending response (%s)", err)
		}

		if err != nil || !response.Success {
			if err := peer.Close(); err != nil {
				log.Errorf("error closing transport connection (%s)", err)
			}
		}
	} else {
		log.Errorf("error receiving request from peer (%s)", err)
		if err := peer.Close(); err != nil {
			log.Errorf("error closing transport connection (%s)", err)
		}
	}
}
