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
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"net/http"
	"time"
)

const (
	ClientCertHeader       = "X-Client-CertPem"
	EdgeRouterProxyRequest = "X-Edge-Router-Proxy-Request"

	ZitiAuthenticatorExtendRquested   = "ziti-authenticator-extend-requested"
	ZitiAuthenticatorRollKeyRequested = "ziti-authenticator-extend-requested"
)

var _ AuthProcessor = &AuthModuleCert{}

type AuthModuleCert struct {
	env    Env
	method string
}

func NewAuthModuleCert(env Env) *AuthModuleCert {
	return &AuthModuleCert{
		env:    env,
		method: db.MethodAuthenticatorCert,
	}
}

func (module *AuthModuleCert) CanHandle(method string) bool {
	return method == module.method
}

// verifyClientCerts will verify a set of x509.Certificates provided by a client during the TLS handshake. It is
// required, as it is required by the TLS spec, that the first certificate is the client's identity. Any additional
// certificates may be provided in order to provide intermediate CAs that map back to a known root CA in the `roots`
// argument. The result is an array of valid chains or an error.
//
// Note: this function does not validate expiration times specifically to allow for situations where expired
// certificates are allowed by authentication policy. Due to the way certificate authentication works, we may
// not know the authentication policy until after the signing root CA is determined.
func (module *AuthModuleCert) verifyClientCerts(clientCerts []*x509.Certificate, roots *x509.CertPool) ([][]*x509.Certificate, error) {
	clientCert := clientCerts[0]

	//time checks are done manually based on authentication policy
	origNotBefore := clientCert.NotBefore
	origNotAfter := clientCert.NotAfter
	clientCert.NotBefore = time.Now().Add(-1 * time.Hour)
	clientCert.NotAfter = time.Now().Add(1 * time.Hour)

	intermediates := x509.NewCertPool()
	for _, curCert := range clientCerts[1:] {
		intermediates.AddCert(curCert)
	}

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	chains, err := clientCert.Verify(opts)

	clientCert.NotBefore = origNotBefore
	clientCert.NotAfter = origNotAfter

	return chains, err
}

// isCertExpirationValid returns true if the provided certificates validations period is currently valid.
func (module *AuthModuleCert) isCertExpirationValid(clientCert *x509.Certificate) bool {
	now := time.Now()
	return now.Before(clientCert.NotAfter) && now.After(clientCert.NotBefore)
}

// Process will inspect the provided AuthContext and attempt to verify the client certificates provided during
// a TLS handshake. Authentication via client certificates follows these steps:
//
// 1) obtain client certificates
// 2) verify client certificates against known CAs
// 3) link a CA certificate back to a model.Ca if possible
// 4) obtain the target identity by authenticator (cert fingerprint) or by external id (claims stuffed into a x509.Certificate resolved by model.Ca)
// 5) verify identity status (disabled)
// 6) obtain the target identity's auth policy
// 7) verify according to auth policy
func (module *AuthModuleCert) Process(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", module.method)

	certs, err := module.getClientCerts(context)

	if err != nil {
		logger.WithError(err).Error("error obtaining client certificates")
		return nil, err
	}

	if len(certs) == 0 {
		logger.Error("no client certificates found")
		return nil, apierror.NewInvalidAuth()
	}

	clientCert := certs[0]

	activeAuthTrustAnchorPool := module.env.GetManagers().Ca.GetActiveAuthTrustAnchorPool()
	activeAuthCas := module.env.GetManagers().Ca.GetActiveAuthCas()

	chains, err := module.verifyClientCerts(certs, activeAuthTrustAnchorPool)

	if err != nil {
		logger.WithError(err).Error("error verifying client certificate")
		return nil, apierror.NewInvalidAuth()
	}

	if len(chains) == 0 {
		logger.Error("failed to verify client, no valid roots")
		return nil, apierror.NewInvalidAuth()
	}

	targetCa := getCaByChain(activeAuthCas, chains, module.env.GetFingerprintGenerator())

	externalId := ""
	if targetCa != nil {
		externalId, err = targetCa.GetExternalId(clientCert)
		if err != nil {
			logger.WithError(err).Error("encountered an error getting externalId from x509.Certificate")
		}
	}

	var identity *Identity
	var authenticator *Authenticator

	if externalId != "" {
		logger = logger.WithField("externalId", externalId)
		identity, _ = module.env.GetManagers().Identity.ReadByExternalId(externalId)

		if identity == nil {
			logger.Error("failed to find identity by externalId")
			return nil, apierror.NewInvalidAuth()
		}

		authenticator = module.authenticatorExternalId(identity.Id, clientCert)

	} else {
		fingerprint := module.env.GetFingerprintGenerator().FromCert(clientCert)
		logger = logger.WithField("fingerprint", fingerprint)

		authenticator, _ = module.env.GetManagers().Authenticator.ReadByFingerprint(fingerprint)

		if authenticator == nil {
			logger.Error("failed to find authenticator by fingerprint")
			return nil, apierror.NewInvalidAuth()
		}

		identity, _ = module.env.GetManagers().Identity.Read(authenticator.IdentityId)
	}

	if identity == nil {
		logger.Error("failed to find a valid identity for authentication")
		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("authenticatorId", authenticator.Id).
		WithField("authenticatorMethod", authenticator.Method).
		WithField("identityId", authenticator.IdentityId).
		WithField("authPolicyId", identity.AuthPolicyId)

	if identity.Disabled {
		logger.
			WithField("disabledAt", identity.DisabledAt).
			WithField("disabledUntil", identity.DisabledUntil).
			Error("authentication failed, identity is disabled")
		return nil, apierror.NewInvalidAuth()
	}

	authPolicy, _ := module.env.GetManagers().AuthPolicy.Read(identity.AuthPolicyId)

	if authPolicy == nil {
		logger.Error("failed to obtain authPolicy by id")
		return nil, apierror.NewInvalidAuth()
	}

	if !authPolicy.Primary.Cert.Allowed {
		logger.Error("invalid certificate authentication, not allowed by auth policy")
		return nil, apierror.NewInvalidAuth()
	}

	if !authPolicy.Primary.Cert.AllowExpiredCerts {
		if !module.isCertExpirationValid(clientCert) {
			logger.Error("failed to verify expiration period of client certificate")
			return nil, apierror.NewInvalidAuth()
		}
	}

	if authenticator.Method == db.MethodAuthenticatorCert {
		module.ensureAuthenticatorCertPem(authenticator, clientCert, context.GetChangeContext())
	}

	return &AuthResultBase{
		identity:        identity,
		authenticatorId: authenticator.Id,
		authenticator:   authenticator,
		sessionCerts:    []*x509.Certificate{clientCert},
		authPolicy:      authPolicy,
		env:             module.env,
	}, nil
}

func (module *AuthModuleCert) isEdgeRouter(clientCert *x509.Certificate) bool {

	fingerprint := module.env.GetFingerprintGenerator().FromCert(clientCert)

	router, err := module.env.GetManagers().EdgeRouter.ReadOneByFingerprint(fingerprint)

	if router != nil {
		return true
	}

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not read edge router by fingerprint %s", fingerprint)
	}

	return false
}

// getClientCerts will return the client certificates that should be used to verify the authentication
// request. The client certificates may be directly provided by the TLS handshake or proxied from a trusted
// source (i.e. edge routers)
func (module *AuthModuleCert) getClientCerts(ctx AuthContext) ([]*x509.Certificate, error) {
	peerCerts := ctx.GetCerts()

	if len(peerCerts) == 0 {
		return nil, nil
	}

	if proxyHeader := ctx.GetHeaders()[EdgeRouterProxyRequest]; proxyHeader != nil {
		return module.getProxiedClientCerts(ctx)
	}

	return peerCerts, nil
}

// ensureAuthenticatorCertPem ensures that a client's certificate is stored in `cert` authenticators. Older versions
// of Ziti did not store this information on enrollment.
func (module *AuthModuleCert) ensureAuthenticatorCertPem(authenticator *Authenticator, clientCert *x509.Certificate, ctx *change.Context) {
	if authCert, ok := authenticator.SubType.(*AuthenticatorCert); ok {
		if authCert.Pem == "" {
			certPem := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: clientCert.Raw,
			})

			authCert.Pem = string(certPem)
			if err := module.env.GetManagers().Authenticator.Update(authenticator, false, nil, ctx); err != nil {
				pfxlog.Logger().WithError(err).Errorf("error during cert auth attempting to update PEM")
			}
		}
	}
}

// authenticatorExternalId returns an authenticator that represents a cert based CA authentication that uses
// `externalId` lookups.
func (module *AuthModuleCert) authenticatorExternalId(identityId string, clientCert *x509.Certificate) *Authenticator {
	authenticator := &Authenticator{
		BaseEntity: models.BaseEntity{
			Id:        "internal",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Tags:      nil,
			IsSystem:  true,
		},
		Method:     db.MethodAuthenticatorCertCaExternalId,
		IdentityId: identityId,
	}

	authenticator.SubType = AuthenticatorCert{
		Authenticator: authenticator,
		Fingerprint:   module.env.GetFingerprintGenerator().FromCert(clientCert),
		Pem:           nfpem.EncodeToString(clientCert),
	}

	return authenticator
}

func (module *AuthModuleCert) getProxiedClientCerts(ctx AuthContext) ([]*x509.Certificate, error) {
	peerCerts := ctx.GetCerts()

	if len(peerCerts) == 0 {
		return nil, apierror.NewInvalidAuth()
	}

	proxyRaw64 := ""

	if proxyRaw64Interface := ctx.GetHeaders()[ClientCertHeader]; proxyRaw64Interface != nil {
		proxyRaw64 = proxyRaw64Interface.(string)
	}

	if proxyRaw64 == "" {
		return nil, apierror.NewInvalidAuth()
	}

	if !module.isEdgeRouter(peerCerts[0]) {
		return nil, apierror.NewInvalidAuth()
	}

	var proxiedRaw []byte
	_, err := base64.StdEncoding.Decode(proxiedRaw, []byte(proxyRaw64))

	if err != nil {
		return nil, &errorz.ApiError{
			Code:    apierror.CouldNotDecodeProxiedCertCode,
			Message: apierror.CouldNotDecodeProxiedCertMessage,
			Cause:   err,
			Status:  http.StatusBadRequest,
		}
	}

	proxiedCerts, err := x509.ParseCertificates(proxiedRaw)

	if err != nil {
		return nil, &errorz.ApiError{
			Code:    apierror.CouldNotParseX509FromDerCode,
			Message: apierror.CouldNotParseX509FromDerMessage,
			Cause:   err,
			Status:  http.StatusBadRequest,
		}
	}

	return proxiedCerts, nil
}

func getCaByChain(activeCas map[string]*Ca, chains [][]*x509.Certificate, generator cert.FingerprintGenerator) *Ca {
	for _, chain := range chains {
		for _, curCert := range chain {
			fingerprint := generator.FromCert(curCert)

			if ca, ok := activeCas[fingerprint]; ok {
				return ca
			}
		}
	}

	return nil
}

func getAuthPolicyByIdentityId(env Env, authMethod string, authenticatorId string, identityId string) (*AuthPolicy, *Identity, error) {
	logger := pfxlog.Logger().
		WithField("authenticatorId", authenticatorId).
		WithField("identityId", identityId).
		WithField("authMethod", authMethod)
	identity, err := env.GetManagers().Identity.Read(identityId)

	if err != nil {
		logger.WithError(err).Errorf("encountered error during %s auth when looking up authenticator", authMethod)
		return nil, nil, apierror.NewInvalidAuth()
	}

	if identity == nil {
		logger.Errorf("identity not found by identityId")
		return nil, nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("authPolicyId", identity.AuthPolicyId)

	authPolicy, err := env.GetManagers().AuthPolicy.Read(identity.AuthPolicyId)

	if err != nil {
		logger.WithError(err).Errorf("encountered error during %s auth when looking up auth policy", authMethod)
		return nil, nil, apierror.NewInvalidAuth()
	}

	return authPolicy, identity, nil
}

func getAuthPolicyByExternalId(env Env, authMethod string, authenticatorId string, externalId string) (*AuthPolicy, *Identity, error) {
	logger := pfxlog.Logger().
		WithField("authenticatorId", authenticatorId).
		WithField("externalId", externalId).
		WithField("authMethod", authMethod)
	identity, err := env.GetManagers().Identity.ReadByExternalId(externalId)

	if err != nil {
		logger.WithError(err).Errorf("encountered error during %s auth when looking up authenticator", authMethod)
		return nil, nil, apierror.NewInvalidAuth()
	}

	if identity == nil {
		logger.Errorf("identity not found by externalId")
		return nil, nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("authPolicyId", identity.AuthPolicyId)

	authPolicy, err := env.GetManagers().AuthPolicy.Read(identity.AuthPolicyId)

	if err != nil {
		logger.WithError(err).Errorf("encountered error during %s auth when looking up auth policy", authMethod)
		return nil, nil, apierror.NewInvalidAuth()
	}

	return authPolicy, identity, nil
}
