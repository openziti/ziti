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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/router/enroll"
	"github.com/openziti/ziti/router/internal/edgerouter"
	routerEnv "github.com/openziti/ziti/router/env"
	"github.com/openziti/identity"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
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
	closeNotify  <-chan struct{}
	ctrls        routerEnv.NetworkControllers
	edgeConfig   *edgerouter.Config
	certsUpdated chan struct{}

	isRunning atomic.Bool

	isRequesting    atomic.Bool
	requestSentAt   time.Time
	timeoutDuration time.Duration

	extender CertExtender
}

func NewCertExpirationChecker(id *identity.TokenId, edgeConfig *edgerouter.Config, ctrls routerEnv.NetworkControllers, closeNotify <-chan struct{}) *CertExpirationChecker {
	ret := &CertExpirationChecker{
		id:              id,
		closeNotify:     closeNotify,
		ctrls:           ctrls,
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
	return self.isRequesting.Load()
}

func (self *CertExpirationChecker) SetIsRequesting(value bool) {
	self.isRequesting.Store(value)
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
				self.isRunning.Store(false)
				return nil
			}
		}

		durationToWait := 0 * time.Second
		if self.edgeConfig.ForceExtendEnrollment {
			self.edgeConfig.ForceExtendEnrollment = false
		} else {
			var err error
			durationToWait, err = self.getWaitTime()

			if err != nil {
				return fmt.Errorf("could not determine enrollent extension wait time: %v", err)
			}
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

func (self *CertExpirationChecker) AnyCtrlChannelWithTimeout(timeout time.Duration) (channel.Channel, bool) {
	startTime := time.Now()
	interval := 500 * time.Millisecond
	for {
		ctrlCh := self.ctrls.AnyCtrlChannel()

		if ctrlCh != nil && !ctrlCh.IsClosed() {
			return ctrlCh, false
		}

		select {
		case <-time.After(interval):
			break
		case <-self.closeNotify:
			return nil, true
		}

		if time.Since(startTime) >= timeout {
			break
		}
	}

	return nil, false
}

func (self *CertExpirationChecker) ExtendEnrollment() error {
	if !self.extender.IsRequestingCompareAndSwap(false, true) {
		return fmt.Errorf("could not send enrollment extension request, a request has already been sent")
	}

	var ctrlCh channel.Channel
	timeout := 10 * time.Second
	intervalWait := 1 * time.Minute
	for ctrlCh == nil {
		var closeNotified bool
		ctrlCh, closeNotified = self.AnyCtrlChannelWithTimeout(timeout)

		if closeNotified {
			return errors.New("router shutting down")
		}

		if ctrlCh == nil {
			pfxlog.Logger().Errorf("cannot send enrollment extension request, no controller is available, waited %s for a control channel, waiting %s to try again", timeout, intervalWait)
		}
	}

	self.requestSentAt = time.Now()

	clientCsrPem, err := enroll.CreateCsr(self.id.Cert().PrivateKey, x509.UnknownSignatureAlgorithm, &self.id.Cert().Leaf.Subject, self.edgeConfig.Csr.Sans)

	if err != nil {
		return fmt.Errorf("could not create client CSR for enrollment extension: %v", err)
	}

	serverCsrPem, err := enroll.CreateCsr(self.id.ServerCert()[0].PrivateKey, x509.UnknownSignatureAlgorithm, &self.id.Cert().Leaf.Subject, self.edgeConfig.Csr.Sans)

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

	if err = ctrlCh.Send(msg); err != nil {
		return fmt.Errorf("could not send enrollment extension request, error: %v", err)
	}

	return nil
}

func (self *CertExpirationChecker) getWaitTime() (time.Duration, error) {
	now := time.Now()

	if self.id.Cert().Leaf.NotAfter.Before(now) || self.id.Cert().Leaf.NotAfter == now {
		return 0, fmt.Errorf("client certificate has expired")
	}

	clientExpirationDuration := self.id.Cert().Leaf.NotAfter.Add(-7 * 24 * time.Hour).Sub(now) //1 week before client cert expires

	if clientExpirationDuration < 0 {
		return 0, nil
	}

	serverExpirationDuration := self.id.ServerCert()[0].Leaf.NotAfter.Add(-7 * 24 * time.Hour).Sub(now) // 1 week before server cert expires

	if serverExpirationDuration < 0 {
		return 0, nil
	}

	durationToWait := serverExpirationDuration
	if clientExpirationDuration < serverExpirationDuration {
		durationToWait = clientExpirationDuration
	}

	return durationToWait, nil
}
