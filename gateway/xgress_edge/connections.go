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
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/gateway/internal/fabric"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/sdk-golang/ziti/edge"
	"time"
)

type sessionConnectionHandler struct {
	stateManager fabric.StateManager
}

func newSessionConnectHandler(stateManager fabric.StateManager) *sessionConnectionHandler {
	return &sessionConnectionHandler{stateManager: stateManager}
}

func (handler *sessionConnectionHandler) BindChannel(ch channel2.Channel) error {
	ch.AddCloseHandler(handler)

	if byteToken, ok := ch.Underlay().Headers()[edge.SessionTokenHeader]; ok {
		token := string(byteToken)

		certificates := ch.Certificates()

		if len(certificates) == 0 {
			return errors.New("no client certificates provided")
		}

		fpg := cert.NewFingerprintGenerator()
		fingerprints := fpg.FromCerts(certificates)

		sessionCh := handler.stateManager.GetSession(token)
		var session *edge_ctrl_pb.ApiSession
		select {
		case session = <-sessionCh:
		case <-time.After(250 * time.Millisecond):
			return errors.New("session token lookup timeout")
		}

		if session == nil {
			_ = ch.Close()
			return fmt.Errorf("no api session found for token [%s]", token)
		}

		for _, fingerprint := range session.CertFingerprints {
			if fingerprints.Contains(fingerprint) {
				removeListener := handler.stateManager.AddSessionRemovedListener(token, func(token string) {
					if !ch.IsClosed() {
						err := ch.Close()

						if err != nil {
							pfxlog.Logger().WithError(err).Error("could not close channel during session removal")
						}
					}
				})

				handler.stateManager.AddConnectedSession(token, removeListener, ch)

				return nil
			}
		}
		_ = ch.Close()
		return errors.New("invalid client certificate for session")
	}
	_ = ch.Close()
	return errors.New("no token attribute provided")
}

func (handler *sessionConnectionHandler) HandleClose(ch channel2.Channel) {
	token := ""
	if byteToken, ok := ch.Underlay().Headers()[edge.SessionTokenHeader]; ok {
		token = string(byteToken)

		handler.stateManager.RemoveConnectedSession(token, ch)
	} else {
		pfxlog.Logger().
			WithField("id", ch.Id()).
			Error("session connection handler encountered a HandleClose that did not have a SessionTokenHeader")
	}

}
