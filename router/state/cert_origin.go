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
	"crypto/x509"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

// CertOrigin indicates whether a client certificate was issued by the internal (first-party) CA
// or a third-party CA. This distinction gates SPIFFE ID matching, which is only valid for
// first-party certificates.
type CertOrigin int

const (
	CertOriginFirstParty CertOrigin = iota
	CertOriginThirdParty
)

// controllerRootCache caches the root CAs extracted from a controller's cert chain.
type controllerRootCache struct {
	mu     sync.RWMutex
	roots  []*x509.Certificate
	inited bool
}

// IsFirstPartyCert reports whether the leaf of peerCerts chains to a first-party trust
// anchor: a router data model public key with the FirstPartyX509CertValidation usage, or
// a root CA from the ctrl channel certificate chain (covers controllers that predate the
// first-party usage). peerCerts is a TLS peer chain with the leaf at index 0; remaining
// entries are used as intermediates, along with intermediates published on the data model
// keys. Time validity is not checked; callers enforce expiry.
func (self *ManagerImpl) IsFirstPartyCert(peerCerts []*x509.Certificate) bool {
	if len(peerCerts) == 0 {
		return false
	}

	roots := x509.NewCertPool()
	intermediates := x509.NewCertPool()
	rootCount := 0

	if rdm := self.routerDataModel.Load(); rdm != nil {
		for keysTuple := range rdm.PublicKeys.IterBuffered() {
			publicKey := keysTuple.Val
			if !contains(publicKey.Usages, edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation) {
				continue
			}

			anchor, err := self.getX509FromData(publicKey.Kid, publicKey.GetData())
			if err != nil {
				pfxlog.Logger().WithField("kid", publicKey.Kid).WithError(err).Error("could not parse x509 certificate data for first party public key")
				continue
			}
			roots.AddCert(anchor)
			rootCount++

			for _, intermediateDer := range publicKey.Intermediates {
				intermediate, err := x509.ParseCertificate(intermediateDer)
				if err != nil {
					pfxlog.Logger().WithField("kid", publicKey.Kid).WithError(err).Error("could not parse intermediate certificate data for first party public key")
					continue
				}
				intermediates.AddCert(intermediate)
			}
		}
	}

	for _, root := range self.getControllerRoots() {
		roots.AddCert(root)
		rootCount++
	}

	if rootCount == 0 {
		return false
	}

	// Shallow-copy the cert so we can bypass time checks without mutating the original.
	// This determines CA origin only; expiry is enforced by the caller.
	certCopy := *peerCerts[0]
	certCopy.NotBefore = time.Now().Add(-1 * time.Hour)
	certCopy.NotAfter = time.Now().Add(1 * time.Hour)

	for _, c := range peerCerts[1:] {
		intermediates.AddCert(c)
	}

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	if _, err := certCopy.Verify(opts); err == nil {
		return true
	}

	return false
}

func (self *ManagerImpl) getControllerRoots() []*x509.Certificate {
	self.ctrlRootCache.mu.RLock()
	if self.ctrlRootCache.inited {
		roots := self.ctrlRootCache.roots
		self.ctrlRootCache.mu.RUnlock()
		return roots
	}
	self.ctrlRootCache.mu.RUnlock()

	self.ctrlRootCache.mu.Lock()
	defer self.ctrlRootCache.mu.Unlock()

	// double-check after acquiring write lock
	if self.ctrlRootCache.inited {
		return self.ctrlRootCache.roots
	}

	ctrls := self.env.GetNetworkControllers()
	ctrlCh := ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		pfxlog.Logger().Warn("no ctrl channel available to determine controller root CA")
		return nil
	}

	certs := ctrlCh.GetChannel().Certificates()
	if len(certs) == 0 {
		pfxlog.Logger().Warn("ctrl channel has no certificates")
		return nil
	}

	// Walk the cert chain to find the root (self-signed) CA.
	var roots []*x509.Certificate
	for _, cert := range certs {
		if identity.IsRootCa(cert) {
			roots = append(roots, cert)
		}
	}

	// If no explicit root in the chain, use the identity's CA bundle to find the root
	// that signed the controller's leaf cert.
	if len(certs) > 0 {
		idCA := self.env.GetRouterId().CA()
		if idCA != nil {
			opts := x509.VerifyOptions{
				Roots:     idCA,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			}
			if chains, err := certs[0].Verify(opts); err == nil {
				for _, chain := range chains {
					roots = append(roots, chain[len(chain)-1])
				}
			}
		}
	}

	self.ctrlRootCache.inited = true
	self.ctrlRootCache.roots = roots
	return roots
}
