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
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/sdk-golang/ziti/edge"
)

type sessionConnectionHandler struct {
	stateManager                     fabric.StateManager
	options                          *Options
	invalidApiSessionToken           metrics.Meter
	invalidApiSessionTokenDuringSync metrics.Meter
}

func newSessionConnectHandler(stateManager fabric.StateManager, options *Options, metricsRegistry metrics.Registry) *sessionConnectionHandler {
	return &sessionConnectionHandler{
		stateManager:                     stateManager,
		options:                          options,
		invalidApiSessionToken:           metricsRegistry.Meter("edge.invalid_api_tokens"),
		invalidApiSessionTokenDuringSync: metricsRegistry.Meter("edge.invalid_api_tokens_during_sync"),
	}
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

		apiSession := handler.stateManager.GetApiSessionWithTimeout(token, handler.options.lookupApiSessionTimeout)

		if apiSession == nil {
			_ = ch.Close()

			var subjects []string

			for _, cert := range certificates {
				subjects = append(subjects, cert.Subject.String())
			}

			handler.invalidApiSessionToken.Mark(1)
			if handler.stateManager.IsSyncInProgress() {
				handler.invalidApiSessionTokenDuringSync.Mark(1)
			}

			return fmt.Errorf("no api session found for token [%s], fingerprints: [%v], subjects [%v]", token, fingerprints, subjects)
		}

		for _, fingerprint := range apiSession.CertFingerprints {
			if fingerprints.Contains(fingerprint) {
				removeListener := handler.stateManager.AddApiSessionRemovedListener(token, func(token string) {
					if !ch.IsClosed() {
						if err := ch.Close(); err != nil {
							pfxlog.Logger().WithError(err).Error("could not close channel during api session removal")
						}
					}
				})

				handler.stateManager.AddConnectedApiSessionWithChannel(token, removeListener, ch)

				return nil
			}
		}
		_ = ch.Close()
		return errors.New("invalid client certificate for api session")
	}
	_ = ch.Close()
	return errors.New("no token attribute provided")
}

func (handler *sessionConnectionHandler) HandleClose(ch channel2.Channel) {
	token := ""
	if byteToken, ok := ch.Underlay().Headers()[edge.SessionTokenHeader]; ok {
		token = string(byteToken)

		handler.stateManager.RemoveConnectedApiSessionWithChannel(token, ch)
	} else {
		pfxlog.Logger().
			WithField("id", ch.Id()).
			Error("session connection handler encountered a HandleClose that did not have a SessionTokenHeader")
	}

}
