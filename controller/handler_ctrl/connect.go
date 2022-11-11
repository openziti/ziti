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

package handler_ctrl

import (
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/identity"
	"github.com/pkg/errors"
	"time"
)

type ConnectHandler struct {
	identity identity.Identity
	network  *network.Network
}

func NewConnectHandler(identity identity.Identity, network *network.Network) *ConnectHandler {
	return &ConnectHandler{
		identity: identity,
		network:  network,
	}
}

func (self *ConnectHandler) HandleConnection(hello *channel.Hello, certificates []*x509.Certificate) error {
	if _, found := hello.Headers[channel.TypeHeader]; found {
		return nil
	}

	id := hello.IdToken

	log := pfxlog.Logger().WithField("routerId", id)

	// verify cert chain
	if len(certificates) == 0 {
		return errors.Errorf("no certificates provided, unable to verify dialer, routerId: %v", id)
	}

	config := self.identity.ServerTLSConfig()

	opts := x509.VerifyOptions{
		Roots:         config.RootCAs,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	var validFingerPrints []string
	var errorList errorz.MultipleErrors

	for _, cert := range certificates {
		if cert.IsCA {
			opts.Intermediates.AddCert(cert)
		}
	}

	for i, cert := range certificates {
		if !cert.IsCA {
			if _, err := cert.Verify(opts); err == nil {
				fingerprint := fmt.Sprintf("%x", sha1.Sum(cert.Raw))
				validFingerPrints = append(validFingerPrints, fingerprint)
				log.Debugf("%d): peer certificate fingerprint [%s]", i, fingerprint)
				log.Debugf("%d): peer common name [%s]", i, cert.Subject.CommonName)
			} else {
				errorList = append(errorList, err)
			}
		}
	}

	if len(validFingerPrints) == 0 && len(errorList) > 0 {
		return errorList.ToError()
	}

	log.Debugf("peer has [%d] valid certificates out of [%v] submitted", len(validFingerPrints), len(certificates))

	if router := self.network.GetConnectedRouter(id); router != nil {
		if time.Since(router.ConnectTime) < self.network.GetOptions().RouterConnectChurnLimit {
			log.WithField("routerName", router.Name).Error("router already connected and churn threshold not met")
			return errors.Errorf("router already connected id: %s, name: %s", id, router.Name)
		}
		log.WithField("routerName", router.Name).Warn("router already connected, but churn threshold met. replacing connection")
	}

	if r, err := self.network.GetRouter(id); err == nil {
		if r.Fingerprint == nil {
			log.Error("router enrollment incomplete")
			return errors.Errorf("router enrollment incomplete, routerId: %v", id)
		}
		if !stringz.Contains(validFingerPrints, *r.Fingerprint) {
			log.WithField("fp", *r.Fingerprint).WithField("givenFps", validFingerPrints).Error("router fingerprint mismatch")
			return errors.Errorf("incorrect fingerprint/unenrolled router, routerId: %v, given fingerprints: %v", id, validFingerPrints)
		}
	} else {
		log.Error("unknown/unenrolled router")
		return errors.Errorf("unknown/unenrolled router, routerId: %v", id)
	}

	return nil
}
