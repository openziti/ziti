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

package db

import (
	"fmt"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
)

func (m *Migrations) setAuthenticatorIsIssuedByNetwork(step *boltz.MigrationStep) {
	cursor := m.stores.Authenticator.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue)

	if m.signingCert == nil {
		return //test instances do not load a config and do not have a signing cert
	}

	var toUpdate []string
	for cursor.IsValid() {
		id := string(cursor.Current())
		authenticator, err := m.stores.Authenticator.LoadById(step.Ctx.Tx(), id)

		if err != nil {
			step.SetError(fmt.Errorf("error loading authenticator[%s]: %s", id, err))
			break
		}

		if authenticator.Type == MethodAuthenticatorCert {
			cert := authenticator.ToCert()
			if cert == nil {
				step.SetError(fmt.Errorf("error converting authenticator [%s] to sub type certificate, got nil back", err))
				break
			}

			certs := nfpem.PemStringToCertificates(cert.Pem)
			if len(certs) == 0 {
				step.SetError(fmt.Errorf("certificate authenticator [%s] have a PEM that decoded to 0 x509.Certificates", id))
				break
			}

			if err := certs[0].CheckSignatureFrom(m.signingCert); err == nil {
				toUpdate = append(toUpdate, id)
			}
		}

		cursor.Next()
	}

	for _, id := range toUpdate {
		authenticator, err := m.stores.Authenticator.LoadById(step.Ctx.Tx(), id)

		if err != nil {
			step.SetError(fmt.Errorf("error loading authenticator[%s]: %s", id, err))
			return
		}

		if authenticator == nil {
			step.SetError(fmt.Errorf("authenticator [%s] not found", id))
			return
		}

		cert := authenticator.ToCert()
		cert.IsIssuedByNetwork = true

		err = m.stores.Authenticator.Update(step.Ctx, authenticator, boltz.MapFieldChecker{
			FieldAuthenticatorCertIsIssuedByNetwork: struct{}{},
		})

		if err != nil {
			step.SetError(fmt.Errorf("error updating authenticator[%s]: %s", id, err))
		}
	}
}
