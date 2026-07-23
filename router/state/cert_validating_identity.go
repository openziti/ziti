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
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

// certValidatingIdentity wraps an identity.Identity to add client certificate chain verification
// at the TLS level via a VerifyConnection callback. It dynamically builds a CA pool from the
// router data model's client cert validation PublicKeys, ensuring the latest trust anchors
// are always used.
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

// buildClientCertRoots builds the trust anchor pool for client certificate verification
// from the router data model's public keys. Keys with the FirstPartyX509CertValidation or
// ThirdPartyX509CertValidation usage are trust anchors; when no key carries either usage
// (controller predates the first/third-party split), keys with the deprecated
// ClientX509CertValidation usage are used instead. Intermediates published on the selected
// keys are returned separately so callers can add them to their intermediate pool.
// parseCert converts a key's anchor data to a certificate, allowing callers to supply caching.
func buildClientCertRoots(rdm *common.RouterDataModel, parseCert func(kid string, data []byte) (*x509.Certificate, error)) (roots *x509.CertPool, intermediates []*x509.Certificate, rootCount int) {
	roots = x509.NewCertPool()

	var anchors []*edge_ctrl_pb.DataState_PublicKey
	var fallback []*edge_ctrl_pb.DataState_PublicKey
	for keysTuple := range rdm.PublicKeys.IterBuffered() {
		publicKey := keysTuple.Val
		if contains(publicKey.Usages, edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation) ||
			contains(publicKey.Usages, edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation) {
			anchors = append(anchors, publicKey)
		} else if contains(publicKey.Usages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation) {
			fallback = append(fallback, publicKey)
		}
	}

	if len(anchors) == 0 {
		anchors = fallback
	}

	for _, publicKey := range anchors {
		anchor, err := parseCert(publicKey.Kid, publicKey.GetData())
		if err != nil {
			pfxlog.Logger().WithField("kid", publicKey.Kid).WithError(err).Error("could not parse x509 certificate data for client cert verification")
			continue
		}
		roots.AddCert(anchor)
		rootCount++

		for _, intermediateDer := range publicKey.Intermediates {
			intermediate, err := x509.ParseCertificate(intermediateDer)
			if err != nil {
				pfxlog.Logger().WithField("kid", publicKey.Kid).WithError(err).Error("could not parse intermediate certificate data for client cert verification")
				continue
			}
			intermediates = append(intermediates, intermediate)
		}
	}

	return roots, intermediates, rootCount
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

	rootPool, publishedIntermediates, certCount := buildClientCertRoots(rdm, func(_ string, data []byte) (*x509.Certificate, error) {
		return x509.ParseCertificate(data)
	})

	if certCount == 0 {
		return errors.New("no trusted CA certificates available in router data model")
	}

	// Start the intermediate pool from the router identity's CA bundle. The RDM anchors are
	// typically root CAs, but client certs may be signed by an intermediate CA that is part
	// of the router's trust bundle rather than published on an anchor.
	intermediatePool := x509.NewCertPool()
	if idCa := self.Identity.CA(); idCa != nil {
		intermediatePool = idCa.Clone()
	}

	for _, intermediate := range publishedIntermediates {
		intermediatePool.AddCert(intermediate)
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
