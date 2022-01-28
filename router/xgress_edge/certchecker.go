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
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/enroll"
	"github.com/openziti/edge/router/internal/edgerouter"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/pkg/errors"
	"time"
)

const DefaultTimeoutDuration = 35 * time.Second

type CertExtender interface {
	ExtendEnrollment() error
	IsRequesting() bool
	IsRequestingCompareAndSwap(bool, bool) bool
	SetIsRequesting(bool)
}

type CertExpirationChecker struct {
	id           *identity.TokenId
	closeNotify  chan struct{}
	ctrl         channel.Channel
	edgeConfig   *edgerouter.Config
	certsUpdated chan struct{}

	isRunning concurrenz.AtomicBoolean

	isRequesting    concurrenz.AtomicBoolean
	requestSentAt   time.Time
	timeoutDuration time.Duration

	extender CertExtender
}

func NewCertExpirationChecker(id *identity.TokenId, edgeConfig *edgerouter.Config, ctrl channel.Channel, closeNotify chan struct{}) *CertExpirationChecker {
	ret := &CertExpirationChecker{
		id:              id,
		closeNotify:     closeNotify,
		ctrl:            ctrl,
		edgeConfig:      edgeConfig,
		certsUpdated:    make(chan struct{}, 1),
		timeoutDuration: DefaultTimeoutDuration,
	}

	ret.extender = ret

	return ret
}

func (self *CertExpirationChecker) IsRequestingCompareAndSwap(expected bool, value bool) bool {
	return self.isRequesting.CompareAndSwap(expected, value)
}

func (self *CertExpirationChecker) IsRequesting() bool {
	return self.isRequesting.Get()
}

func (self *CertExpirationChecker) SetIsRequesting(value bool) {
	self.isRequesting.Set(value)
}

func (self *CertExpirationChecker) CertsUpdated() {
	self.certsUpdated <- struct{}{}
}

func (self *CertExpirationChecker) Run() error {
	if !self.isRunning.CompareAndSwap(false, true) {
		return errors.New("already running")
	}

	for {
		//if we are already requesting then wait for the request to finish (certsUpdated), we give up based on
		//timeoutDuration, or we are told to shut down
		if self.extender.IsRequesting() {
			select {
			case <-self.certsUpdated:
				self.extender.SetIsRequesting(false)
			case <-time.After(self.timeoutDuration):
				self.extender.SetIsRequesting(false)
			case <-self.closeNotify:
				self.isRunning.Set(false)
				return nil
			}
		}

		durationToWait, err := self.getWaitTime()

		if err != nil {
			return fmt.Errorf("could not extend certificates: %v", err)
		}

		pfxlog.Logger().Infof("waiting %s to renew certificates", durationToWait)

		select {
		case <-self.certsUpdated:
			self.extender.SetIsRequesting(false)
		case <-time.After(durationToWait):
			if err := self.extender.ExtendEnrollment(); err != nil {
				pfxlog.Logger().Errorf("could not extend enrollment: %v", err)
			}
		case <-self.closeNotify:
			return nil
		}
	}
}

func (self *CertExpirationChecker) ExtendEnrollment() error {
	if !self.extender.IsRequestingCompareAndSwap(false, true) {
		return fmt.Errorf("could not send enrollment extension request, a request has already been sent")
	}

	if self.ctrl.IsClosed() {
		return errors.New("cannot request updates, control channel has closed")
	}

	self.requestSentAt = time.Now()

	clientCsrPem, err := enroll.CreateCsr(self.id.Cert().PrivateKey, x509.UnknownSignatureAlgorithm, &self.id.Cert().Leaf.Subject, self.edgeConfig.Csr.Sans)

	if err != nil {
		return fmt.Errorf("could not create client CSR for enrollment extension: %v", err)
	}

	serverCsrPem, err := enroll.CreateCsr(self.id.ServerCert().PrivateKey, x509.UnknownSignatureAlgorithm, &self.id.Cert().Leaf.Subject, self.edgeConfig.Csr.Sans)

	if err != nil {
		return fmt.Errorf("could not create server CSR for enrollment extension: %v", err)
	}

	data := &edge_ctrl_pb.EnrollmentExtendRouterRequest{
		ClientCertCsr:       clientCsrPem,
		ServerCertCsr:       serverCsrPem,
		RequireVerification: true,
	}

	body, err := proto.Marshal(data)

	if err != nil {
		return fmt.Errorf("could not marshal enrollment extension request: %v", err)
	}

	msg := channel.NewMessage(env.EnrollmentExtendRouterRequestType, body)

	if err = self.ctrl.Send(msg); err != nil {
		return fmt.Errorf("could not send enrollment extension request, error: %v", err)
	}

	return nil
}

func (self *CertExpirationChecker) getWaitTime() (time.Duration, error) {
	var durationToWait time.Duration = 0

	if self.edgeConfig.ExtendEnrollment {
		self.edgeConfig.ExtendEnrollment = false
		durationToWait = 0

		pfxlog.Logger().Info("enrollment extension was forced")
	} else {
		now := time.Now()

		if self.id.Cert().Leaf.NotAfter.Before(now) || self.id.Cert().Leaf.NotAfter == now {
			return 0, fmt.Errorf("client certificate has expired")
		}

		clientExpirationDuration := self.id.Cert().Leaf.NotAfter.Add(-7 * 24 * time.Hour).Sub(now) //1 week before client cert expires

		if clientExpirationDuration < 0 {
			return 0, nil
		}

		serverExpirationDuration := self.id.ServerCert().Leaf.NotAfter.Add(-7 * 24 * time.Hour).Sub(now) // 1 week before server cert expires

		if serverExpirationDuration < 0 {
			return 0, nil
		}

		durationToWait = serverExpirationDuration
		if clientExpirationDuration < serverExpirationDuration {
			durationToWait = clientExpirationDuration
		}
	}

	return durationToWait, nil
}
