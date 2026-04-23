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

package state

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

// certValidatingIdentity wraps an identity.Identity to add client certificate chain verification
// at the TLS level via a VerifyConnection callback. It dynamically builds a CA pool from the
// router data model's PublicKeys with ClientX509CertValidation usage, ensuring the latest
// trust anchors are always used.
type certValidatingIdentity struct {
	identity.Identity
	stateManager Manager
}

// WrapIdentityWithCertValidation returns a new identity.TokenId with the same Token and Data
// but an Identity that adds TLS-level client certificate verification using the router's
// data model CA pool.
func WrapIdentityWithCertValidation(id *identity.TokenId, stateManager Manager) *identity.TokenId {
	return &identity.TokenId{
		Identity: &certValidatingIdentity{
			Identity:     id.Identity,
			stateManager: stateManager,
		},
		Token: id.Token,
		Data:  id.Data,
	}
}

func (self *certValidatingIdentity) ServerTLSConfig() *tls.Config {
	cfg := self.Identity.ServerTLSConfig()
	cfg.VerifyConnection = self.verifyConnection
	return cfg
}

// verifyConnection is a TLS VerifyConnection callback that verifies client certificates against
// the CA pool built from RDM PublicKeys.
func (self *certValidatingIdentity) verifyConnection(state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		// No client cert presented. The edge listener requires a cert (RequireAnyClientCert),
		// so this should not happen. Reject to be safe.
		return errors.New("no client certificate presented")
	}

	rdm := self.stateManager.RouterDataModel()
	if rdm == nil {
		return errors.New("router data model not yet available, cannot verify client certificate")
	}

	rootPool := x509.NewCertPool()
	intermediatePool := x509.NewCertPool()
	certCount := 0
	for keysTuple := range rdm.PublicKeys.IterBuffered() {
		if contains(keysTuple.Val.Usages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation) {
			parsed, err := x509.ParseCertificate(keysTuple.Val.GetData())
			if err != nil {
				pfxlog.Logger().WithField("kid", keysTuple.Val.Kid).WithError(err).Error("could not parse x509 certificate data for TLS client verification")
				continue
			}
			rootPool.AddCert(parsed)
			certCount++
		}
	}

	if certCount == 0 {
		return errors.New("no trusted CA certificates available in router data model")
	}

	// Add the router identity's CA bundle as intermediates. The RDM PublicKeys typically
	// contain only root CAs, but client certs may be signed by an intermediate CA that
	// is part of the router's trust bundle.
	if idCa := self.Identity.CA(); idCa != nil {
		intermediatePool = idCa.Clone()
	}

	// Also add any additional certs from the TLS peer chain as intermediates.
	for _, cert := range state.PeerCertificates[1:] {
		intermediatePool.AddCert(cert)
	}

	// Shallow-copy the leaf cert so we can override time fields without mutating the original.
	// Expiry enforcement is deferred to application code which has access to the API session
	// token and auth policy (z_cae claim).
	leafCopy := *state.PeerCertificates[0]
	leafCopy.NotBefore = time.Now().Add(-1 * time.Hour)
	leafCopy.NotAfter = time.Now().Add(1 * time.Hour)

	opts := x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: intermediatePool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	if _, err := leafCopy.Verify(opts); err != nil {
		return fmt.Errorf("client certificate not issued by a trusted CA: %w", err)
	}

	return nil
}
