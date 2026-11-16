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

package model

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/config"
)

// routerEnrollTestEnv supplies the CSR signers the router enrollment modules need, which the plain
// TestContext does not provide.
type routerEnrollTestEnv struct {
	*TestContext
	serverSigner cert.Signer
	clientSigner cert.Signer
}

func (self *routerEnrollTestEnv) GetApiServerCsrSigner() cert.Signer {
	return self.serverSigner
}

func (self *routerEnrollTestEnv) GetControlClientCsrSigner() cert.Signer {
	return self.clientSigner
}

// interceptingSigner runs beforeFirstSign once, right before the first CSR is signed. The enrollment
// modules sign after they have read the enrollment and before they store anything, so the hook runs at
// exactly the point where a second request using the same token can overtake the first one.
type interceptingSigner struct {
	*cert.ClientSigner
	beforeFirstSign func()
}

func (self *interceptingSigner) SignCsr(csr *x509.CertificateRequest, opts *cert.SigningOpts) ([]byte, error) {
	if hook := self.beforeFirstSign; hook != nil {
		self.beforeFirstSign = nil
		hook()
	}
	return self.ClientSigner.SignCsr(csr, opts)
}

func (ctx *TestContext) newRouterEnrollTestEnv() (*routerEnrollTestEnv, *interceptingSigner) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ctx.NoError(err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "router enrollment test ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	caDer, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	ctx.NoError(err)

	caCert, err := x509.ParseCertificate(caDer)
	ctx.NoError(err)

	// the test context's edge config has no CA buffer, which building the certificate chains requires
	edgeConfig := config.NewEdgeConfig()
	edgeConfig.Enrollment = ctx.config.Edge.Enrollment
	ctx.config.Edge = edgeConfig

	clientSigner := &interceptingSigner{ClientSigner: cert.NewClientSigner(caCert, caKey)}

	return &routerEnrollTestEnv{
		TestContext:  ctx,
		serverSigner: cert.NewServerSigner(caCert, caKey),
		clientSigner: clientSigner,
	}, clientSigner
}

func (ctx *TestContext) newRouterEnrollmentContext(token string, method string, commonName string) *EnrollmentContextHttp {
	newCsrPem := func() []byte {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		ctx.NoError(err)

		csrDer, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
			Subject: pkix.Name{CommonName: commonName},
		}, key)
		ctx.NoError(err)

		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDer})
	}

	return &EnrollmentContextHttp{
		Token:  token,
		Method: method,
		Data: &EnrollmentData{
			ServerCsrPem: newCsrPem(),
			ClientCsrPem: newCsrPem(),
		},
		ChangeContext: change.New(),
	}
}

// requireRouterEnrollmentRace lets a second request with the same token complete while the first one is
// between reading the enrollment and storing its result. It returns the certificates handed to the request
// that completed and the error the overtaken request was rejected with.
func (ctx *TestContext) requireRouterEnrollmentRace(module EnrollmentProcessor, clientSigner *interceptingSigner, token string, method string, routerId string) (*rest_model.EnrollmentCerts, error) {
	var winnerResult *EnrollmentResult
	var winnerErr error

	clientSigner.beforeFirstSign = func() {
		winnerResult, winnerErr = module.Process(ctx.newRouterEnrollmentContext(token, method, routerId))
	}

	loserResult, loserErr := module.Process(ctx.newRouterEnrollmentContext(token, method, routerId))

	ctx.NoError(winnerErr)
	ctx.NotNil(winnerResult)

	ctx.Nil(loserResult, "a request overtaken by another one using the same token must not get certificates")
	ctx.Error(loserErr)

	enrollment, err := ctx.managers.Enrollment.ReadByToken(token)
	ctx.NoError(err)
	ctx.Nil(enrollment, "the enrollment must be consumed")

	winnerCerts, ok := winnerResult.Content.(*rest_model.EnrollmentCerts)
	ctx.True(ok)

	return winnerCerts, loserErr
}

func (ctx *TestContext) requireInvalidEnrollmentToken(err error) {
	apiErr := &errorz.ApiError{}
	ctx.ErrorAs(err, &apiErr)
	ctx.Equal(apierror.InvalidEnrollmentTokenCode, apiErr.AppCode)
}

func (ctx *TestContext) requireRouterEnrollmentToken(collect func(collector func(entity *Enrollment) error) error) string {
	var token string
	ctx.NoError(collect(func(entity *Enrollment) error {
		token = entity.Token
		return nil
	}))
	ctx.NotEmpty(token)
	return token
}

func TestEdgeRouterEnrollmentConsumesTokenOnce(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	testEnv, clientSigner := ctx.newRouterEnrollTestEnv()
	module := NewEnrollModuleEdgeRouterOtt(testEnv)

	edgeRouter := ctx.requireNewEdgeRouter()
	token := ctx.requireRouterEnrollmentToken(func(collector func(entity *Enrollment) error) error {
		return ctx.managers.EdgeRouter.CollectEnrollments(edgeRouter.Id, collector)
	})

	winnerCerts, loserErr := ctx.requireRouterEnrollmentRace(module, clientSigner, token, MethodEnrollEdgeRouterOtt, edgeRouter.Id)

	edgeRouter, err := ctx.managers.EdgeRouter.Read(edgeRouter.Id)
	ctx.NoError(err)
	ctx.True(edgeRouter.IsVerified)
	ctx.NotNil(edgeRouter.Fingerprint)
	ctx.NotNil(edgeRouter.CertPem)

	ctx.Equal(cert.NewFingerprintGenerator().FromPem([]byte(winnerCerts.Cert)), *edgeRouter.Fingerprint,
		"the stored fingerprint must belong to the certificate that was handed out")
	ctx.Equal(winnerCerts.Cert, *edgeRouter.CertPem)

	ctx.requireInvalidEnrollmentToken(loserErr)
}

func TestTransitRouterEnrollmentConsumesTokenOnce(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	testEnv, clientSigner := ctx.newRouterEnrollTestEnv()
	module := NewEnrollModuleTransitRouterOtt(testEnv)

	transitRouter := &TransitRouter{
		Name: eid.New(),
	}
	ctx.NoError(ctx.managers.TransitRouter.Create(transitRouter, change.New()))

	token := ctx.requireRouterEnrollmentToken(func(collector func(entity *Enrollment) error) error {
		return ctx.managers.TransitRouter.CollectEnrollments(transitRouter.Id, collector)
	})

	winnerCerts, loserErr := ctx.requireRouterEnrollmentRace(module, clientSigner, token, MethodEnrollTransitRouterOtt, transitRouter.Id)

	transitRouter, err := ctx.managers.TransitRouter.Read(transitRouter.Id)
	ctx.NoError(err)
	ctx.True(transitRouter.IsVerified)
	ctx.NotNil(transitRouter.Fingerprint)

	ctx.Equal(cert.NewFingerprintGenerator().FromPem([]byte(winnerCerts.Cert)), *transitRouter.Fingerprint,
		"the stored fingerprint must belong to the certificate that was handed out")

	ctx.requireInvalidEnrollmentToken(loserErr)
}
