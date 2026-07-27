/*
	(c) Copyright NetFoundry Inc.

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

package xlink_transport

import (
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/cert"
)

type ConnectionHandler struct {
	routerId identity.Identity
}

func (self *ConnectionHandler) HandleConnection(_ *channel.Hello, certificates []*x509.Certificate) error {
	if len(certificates) == 0 {
		return errors.New("no certificates provided, unable to verify dialer")
	}

	if _, err := cert.VerifyLeafCertChain(self.routerId.CA(), certificates); err != nil {
		return fmt.Errorf("unable to verify dialing router: %w", err)
	}

	return nil
}
