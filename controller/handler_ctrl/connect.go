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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/controller/network"
)

type ConnectHandler struct {
	identity identity.Identity
	network  *network.Network

	// separatelyValidatedTypes holds the control-channel type headers that are dispatched to a
	// separate, self-validating acceptor (currently the raft mesh, when clustering is enabled).
	separatelyValidatedTypes map[string]struct{}
}

func NewConnectHandler(identity identity.Identity, network *network.Network) *ConnectHandler {
	return &ConnectHandler{
		identity: identity,
		network:  network,
	}
}

// SetSeparatelyValidatedChannelTypes records the control-channel type headers that are dispatched to a
// separate, self-validating acceptor (e.g. the raft mesh). Connections carrying one of these types are
// skipped by HandleConnection; everything else - unrecognized types and legacy (no type header)
// connections, all of which the dispatcher routes to the router control acceptor - is validated here.
// This must be populated before the listener begins accepting.
func (self *ConnectHandler) SetSeparatelyValidatedChannelTypes(types map[string]struct{}) {
	self.separatelyValidatedTypes = types
}

// isSeparatelyValidated reports whether the connection's channel type is handled by a separate,
// self-validating acceptor and therefore must not be validated as a router control connection here.
func (self *ConnectHandler) isSeparatelyValidated(hello *channel.Hello) bool {
	underlayType, found := hello.Headers[channel.TypeHeader]
	if !found {
		return false
	}
	_, ok := self.separatelyValidatedTypes[string(underlayType)]
	return ok
}

func (self *ConnectHandler) HandleConnection(hello *channel.Hello, certificates []*x509.Certificate) error {
	// Connections whose channel type is handled by a separate, self-validating acceptor (e.g. the raft
	// mesh) are validated there, so skip them. Everything else - unrecognized types and legacy (no type
	// header) connections - is dispatched to the router control acceptor and must be validated here.
	if self.isSeparatelyValidated(hello) {
		return nil
	}

	id := hello.IdToken

	log := pfxlog.Logger().WithField("routerId", id)

	if len(certificates) == 0 {
		return fmt.Errorf("no certificates provided, unable to verify dialer, routerId: %v", id)
	}

	// Verify the peer's leaf certificate (certificates[0], the certificate whose private key the TLS
	// handshake proved) chains to the controller CA, and bind the router fingerprint check to that
	// verified leaf. Matching the enrolled fingerprint against any presented certificate would let a
	// peer present its own leaf followed by a target router's public certificate and pass without that
	// router's private key.
	leaf, err := cert.VerifyLeafCertChain(self.identity.CA(), certificates)
	if err != nil {
		return fmt.Errorf("unable to verify dialer, routerId: %v: %w", id, err)
	}
	fingerprint := fmt.Sprintf("%x", sha1.Sum(leaf.Raw))
	log.Debugf("peer leaf certificate fingerprint [%s], common name [%s]", fingerprint, leaf.Subject.CommonName)

	if router := self.network.GetConnectedRouter(id); router != nil {
		if time.Since(router.ConnectTime) < self.network.GetOptions().RouterConnectChurnLimit {
			log.WithField("routerName", router.Name).Error("router already connected and churn threshold not met")
			return fmt.Errorf("router already connected id: %s, name: %s", id, router.Name)
		}
		log.WithField("routerName", router.Name).Warn("router already connected, but churn threshold met. replacing connection")
	}

	if r, err := self.network.GetRouter(id); err == nil {
		if r.Fingerprint == nil {
			log.Error("router enrollment incomplete")
			return fmt.Errorf("router enrollment incomplete, routerId: %v", id)
		}
		if fingerprint != *r.Fingerprint {
			log.WithField("fp", *r.Fingerprint).WithField("givenFp", fingerprint).Error("router fingerprint mismatch")
			return fmt.Errorf("incorrect fingerprint/unenrolled router, routerId: %v, given fingerprint: %v", id, fingerprint)
		}
		if r.Disabled {
			log.Error("router disabled")
			return fmt.Errorf("router disabld, routerId: %v", id)
		}
	} else {
		log.Error("unknown/unenrolled router")
		return fmt.Errorf("unknown/unenrolled router, routerId: %v", id)
	}

	return nil
}
