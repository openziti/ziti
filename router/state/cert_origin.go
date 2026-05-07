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
)

// CertOrigin indicates whether a client certificate was issued by the internal (first-party) CA
// or a third-party CA. This distinction gates SPIFFE ID matching, which is only valid for
// first-party certificates.
type CertOrigin int

const (
	CertOriginFirstParty CertOrigin = iota
	CertOriginThirdParty
)

// controllerRootCache caches the root CA extracted from a controller's cert chain.
type controllerRootCache struct {
	mu       sync.RWMutex
	rootPool *x509.CertPool
	inited   bool
}

// IsFirstPartyCert reports whether the leaf of peerCerts chains to a controller-trusted
// root CA. peerCerts is a TLS peer chain with the leaf at index 0; remaining entries are
// used as intermediates. Time validity is not checked; callers enforce expiry.
func (self *ManagerImpl) IsFirstPartyCert(peerCerts []*x509.Certificate) bool {
	if len(peerCerts) == 0 {
		return false
	}

	pool := self.getControllerRootPool()
	if pool == nil {
		return false
	}

	// Shallow-copy the cert so we can bypass time checks without mutating the original.
	// This determines CA origin only; expiry is enforced by the caller.
	certCopy := *peerCerts[0]
	certCopy.NotBefore = time.Now().Add(-1 * time.Hour)
	certCopy.NotAfter = time.Now().Add(1 * time.Hour)

	intermediates := x509.NewCertPool()
	for _, c := range peerCerts[1:] {
		intermediates.AddCert(c)
	}

	opts := x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	if _, err := certCopy.Verify(opts); err == nil {
		return true
	}

	return false
}

func (self *ManagerImpl) getControllerRootPool() *x509.CertPool {
	self.ctrlRootCache.mu.RLock()
	if self.ctrlRootCache.inited {
		pool := self.ctrlRootCache.rootPool
		self.ctrlRootCache.mu.RUnlock()
		return pool
	}
	self.ctrlRootCache.mu.RUnlock()

	self.ctrlRootCache.mu.Lock()
	defer self.ctrlRootCache.mu.Unlock()

	// double-check after acquiring write lock
	if self.ctrlRootCache.inited {
		return self.ctrlRootCache.rootPool
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
	rootPool := x509.NewCertPool()
	for _, cert := range certs {
		if identity.IsRootCa(cert) {
			rootPool.AddCert(cert)
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
					root := chain[len(chain)-1]
					rootPool.AddCert(root)
				}
			}
		}
	}

	self.ctrlRootCache.inited = true
	self.ctrlRootCache.rootPool = rootPool
	return rootPool
}
