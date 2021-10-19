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
	"crypto/x509"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/enroll"
	"github.com/openziti/edge/router/internal/edgerouter"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"time"
)

type CertExpirationChecker struct {
	id           *identity.TokenId
	closeNotify  chan struct{}
	ctrl         channel2.Channel
	edgeConfig   *edgerouter.Config
	certsUpdated chan struct{}
}

func NewCertExpirationChecker(id *identity.TokenId, edgeConfig *edgerouter.Config, ctrl channel2.Channel, closeNotify chan struct{}) *CertExpirationChecker {
	return &CertExpirationChecker{
		id:           id,
		closeNotify:  closeNotify,
		ctrl:         ctrl,
		edgeConfig:   edgeConfig,
		certsUpdated: make(chan struct{}, 1),
	}
}

func (self *CertExpirationChecker) CertsUpdated() {
	self.certsUpdated <- struct{}{}
}

func (self *CertExpirationChecker) Run() {
	for {
		var durationToWait time.Duration = 0

		if self.edgeConfig.ExtendEnrollment {
			self.edgeConfig.ExtendEnrollment = false
			durationToWait = 0

			pfxlog.Logger().Info("enrollment extension was forced")
		} else {
			now := time.Now()

			clientExpirationDuration := self.id.Cert().Leaf.NotAfter.Add(-7 * 24 * time.Hour).Sub(now)       //1 week before client cert expires
			serverExpirationDuration := self.id.ServerCert().Leaf.NotAfter.Add(-7 * 24 * time.Hour).Sub(now) // 1 week before server cert expires

			if clientExpirationDuration == 0 {
				pfxlog.Logger().Panic("client cert has expired, cannot renew via enrollment extension")
			}

			durationToWait = serverExpirationDuration
			if clientExpirationDuration < serverExpirationDuration {
				durationToWait = clientExpirationDuration
			}
		}

		select {
		case <-self.certsUpdated:
			//loop
		case <-time.After(durationToWait):
			self.ExtendEnrollment()
		case <-self.closeNotify:
			return
		}
	}
}

func (self *CertExpirationChecker) ExtendEnrollment() {
	if self.ctrl.IsClosed() {
		pfxlog.Logger().Error("cannot request updates, control channel has closed")
		return
	}

	clientCsrPem, err := enroll.CreateCsr(self.id.Cert().PrivateKey, x509.UnknownSignatureAlgorithm, &self.id.Cert().Leaf.Subject, self.edgeConfig.Csr.Sans)

	if err != nil {
		pfxlog.Logger().Errorf("could not create client CSR for enrollment extension: %v", err)
		return
	}

	serverCsrPem, err := enroll.CreateCsr(self.id.ServerCert().PrivateKey, x509.UnknownSignatureAlgorithm, &self.id.Cert().Leaf.Subject, self.edgeConfig.Csr.Sans)

	if err != nil {
		pfxlog.Logger().Errorf("could not create server CSR for enrollment extension: %v", err)
		return
	}

	data := &edge_ctrl_pb.EnrollmentExtendRouterRequest{
		ClientCertCsr: clientCsrPem,
		ServerCertCsr: serverCsrPem,
	}

	body, err := proto.Marshal(data)

	if err != nil {
		pfxlog.Logger().Errorf("could not marshal enrollment extension request: %v", err)
		return
	}

	msg := channel2.NewMessage(env.EnrollmentExtendRouterRequestType, body)

	if err = self.ctrl.Send(msg); err != nil {
		pfxlog.Logger().Errorf("could not send enrollment extension request: %v", err)
	}
}
