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

package handler_mgmt

import (
	"crypto/x509"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/identity"
	"github.com/pkg/errors"
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
	if len(certificates) == 0 {
		return errors.New("no certificates provided, unable to verify dialer")
	}

	config := self.identity.ServerTLSConfig()

	opts := x509.VerifyOptions{
		Roots:         config.RootCAs,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	var errorList errorz.MultipleErrors

	for _, cert := range certificates {
		if _, err := cert.Verify(opts); err == nil {
			return nil
		} else {
			errorList = append(errorList, err)
		}
	}

	//goland:noinspection GoNilness
	return errorList.ToError()
}
