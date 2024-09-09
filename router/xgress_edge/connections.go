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
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/spiffehlp"
	"github.com/openziti/ziti/router/state"
	"strings"
)

type sessionConnectionHandler struct {
	stateManager                     state.Manager
	options                          *Options
	invalidApiSessionToken           metrics.Meter
	invalidApiSessionTokenDuringSync metrics.Meter
}

func newSessionConnectHandler(stateManager state.Manager, options *Options, metricsRegistry metrics.Registry) *sessionConnectionHandler {
	return &sessionConnectionHandler{
		stateManager:                     stateManager,
		options:                          options,
		invalidApiSessionToken:           metricsRegistry.Meter("edge.invalid_api_tokens"),
		invalidApiSessionTokenDuringSync: metricsRegistry.Meter("edge.invalid_api_tokens_during_sync"),
	}
}

func (handler *sessionConnectionHandler) BindChannel(binding channel.Binding, edgeConn *edgeClientConn) error {
	ch := binding.GetChannel()
	binding.AddCloseHandler(handler)

	byteToken, ok := ch.Underlay().Headers()[edge.SessionTokenHeader]

	if !ok {
		_ = ch.Close()
		return errors.New("no token attribute provided")
	}

	certificates := ch.Certificates()

	if len(certificates) == 0 {
		return errors.New("no client certificates provided")
	}

	fpg := cert.NewFingerprintGenerator()
	fingerprint := fpg.FromCert(certificates[0])

	token := string(byteToken)

	apiSession := handler.stateManager.GetApiSessionWithTimeout(token, handler.options.lookupApiSessionTimeout)

	if apiSession == nil {
		_ = ch.Close()

		var subjects []string

		for _, curCert := range certificates {
			subjects = append(subjects, curCert.Subject.String())
		}

		handler.invalidApiSessionToken.Mark(1)
		if handler.stateManager.IsSyncInProgress() {
			handler.invalidApiSessionTokenDuringSync.Mark(1)
		}

		return fmt.Errorf("no api session found for token [%s], fingerprint: [%v], subjects [%v]", token, fingerprint, subjects)
	}

	edgeConn.apiSession = apiSession

	isValid := handler.validateBySpiffeId(apiSession, certificates[0])

	if !isValid {
		isValid = handler.validateByFingerprint(apiSession, fingerprint)
	}

	if isValid {
		if apiSession.Claims != nil {
			token = apiSession.Claims.ApiSessionId
		}

		removeListener := handler.stateManager.AddApiSessionRemovedListener(token, func(token string) {
			if !ch.IsClosed() {
				if err := ch.Close(); err != nil {
					pfxlog.Logger().WithError(err).Error("could not close channel during api session removal")
				}
			}

			handler.stateManager.RemoveActiveChannel(ch)
		})

		handler.stateManager.AddActiveChannel(ch, apiSession)
		handler.stateManager.AddConnectedApiSessionWithChannel(token, removeListener, ch)

		return nil
	}

	_ = ch.Close()
	return errors.New("invalid client certificate for api session")
}

func (handler *sessionConnectionHandler) validateByFingerprint(apiSession *state.ApiSession, clientFingerprint string) bool {
	for _, fingerprint := range apiSession.CertFingerprints {
		if clientFingerprint == fingerprint {
			return true
		}
	}

	return false
}

func (handler *sessionConnectionHandler) HandleClose(ch channel.Channel) {
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

func (handler *sessionConnectionHandler) validateBySpiffeId(apiSession *state.ApiSession, clientCert *x509.Certificate) bool {
	spiffeId, err := spiffehlp.GetSpiffeIdFromCert(clientCert)

	if err != nil {
		return false
	}

	if spiffeId == nil {
		return false
	}

	parts := strings.Split(spiffeId.Path, "/")

	if len(parts) != 6 {
		return false
	}

	if parts[0] != "identity" {
		return false
	}

	if parts[2] != "apiSession" {
		return false
	}

	if parts[4] != "apiSessionCertificate" {
		return false
	}

	if apiSession.Id == parts[3] {
		return true
	}

	return false
}
