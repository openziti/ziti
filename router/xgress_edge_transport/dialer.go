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

package xgress_edge_transport

import (
	"fmt"
	"github.com/netfoundry/secretstream"
	"github.com/netfoundry/secretstream/kx"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/sdk-golang/ziti/edge"
)

type dialer struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	options *xgress.Options
}

func (txd *dialer) IsTerminatorValid(string, string) bool {
	return true
}

func newDialer(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) (xgress.Dialer, error) {
	txd := &dialer{
		id:      id,
		ctrl:    ctrl,
		options: options,
	}
	return txd, nil
}

type cryptoContext struct {
	keyPair *kx.KeyPair

	clientPublicKey []byte
	publicTxHeader  []byte

	rxKey []byte

	rx []byte
	tx []byte

	toClientEncryptor   secretstream.Encryptor
	fromClientDecryptor secretstream.Decryptor
}

func (ctx *cryptoContext) SetClientPublicKey(clientPublicKey []byte) error {
	var err error
	var tx []byte

	if ctx.rxKey, tx, err = ctx.keyPair.ServerSessionKeys(clientPublicKey); err != nil {
		return fmt.Errorf("failed key exchange: %v", err)
	}

	if ctx.toClientEncryptor, ctx.publicTxHeader, err = secretstream.NewEncryptor(tx); err != nil {
		return fmt.Errorf("failed to establish crypto stream: %v", err)
	}

	return nil
}

func newCryptoContext() (*cryptoContext, error) {
	var err error

	ctx := &cryptoContext{}
	ctx.keyPair, err = kx.NewKeyPair()

	if err != nil {
		return nil, fmt.Errorf("could not create new key pair: %v", err)
	}

	return ctx, nil
}

func (txd *dialer) Dial(destination string, sessionId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler) (xt.PeerData, error) {
	txDestination, err := transport.ParseAddress(destination)
	if err != nil {
		return nil, fmt.Errorf("cannot dial on invalid address [%s] (%s)", destination, err)
	}

	peer, err := txDestination.Dial("x/"+sessionId.Token, sessionId, txd.options.ConnectTimeout, nil)
	if err != nil {
		return nil, err
	}

	conn := newEdgeTransportXgressConn(peer)

	peerData := make(xt.PeerData, 1)
	if cryptoCtx, err := txd.prepareCrypto(sessionId); err != nil {
		return nil, fmt.Errorf("error preparing e2e crypto: %v", err)
	} else if cryptoCtx != nil {
		conn.cryptoCtx = cryptoCtx
		peerData[edge.PublicKeyHeader] = cryptoCtx.keyPair.Public()
	}

	x := xgress.NewXgress(sessionId, address, conn, xgress.Terminator, txd.options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return peerData, nil
}

func (txd *dialer) prepareCrypto(sessionId *identity.TokenId) (*cryptoContext, error) {
	var cryptoCtx *cryptoContext
	if clientPublicKey, ok := sessionId.Data[edge.PublicKeyHeader]; ok {
		var err error
		cryptoCtx, err = newCryptoContext()

		if err != nil {
			return nil, fmt.Errorf("error establishing crypto context: %v", err)
		}

		if err := cryptoCtx.SetClientPublicKey(clientPublicKey); err != nil {
			return nil, err
		}
	}

	return cryptoCtx, nil
}
