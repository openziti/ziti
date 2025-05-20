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
	"fmt"
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

	ZitiAuthenticatorExtendRequested  = "ziti-authenticator-extend-requested"
	ZitiAuthenticatorRollKeyRequested = "ziti-authenticator-extend-requested"
)

var _ AuthProcessor = &AuthModuleCert{}

type AuthModuleCert struct {
	BaseAuthenticator
}

func NewAuthModuleCert(env Env) *AuthModuleCert {
	return &AuthModuleCert{
		BaseAuthenticator: BaseAuthenticator{
			env:    env,
			method: db.MethodAuthenticatorCert,
		},
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
	logger := pfxlog.Logger().WithField("authMethod", module.method).WithField("changeContext", context.GetChangeContext().Attributes)

	bundle := &AuthBundle{}

	certs, err := module.getClientCerts(context)

	if err != nil {
		reason := fmt.Sprintf("error obtaining client certificates: %v", err)
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.WithError(err).Error("error obtaining client certificates")
		return nil, err
	}

	if len(certs) == 0 {
		reason := "no client certificates found"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.Error(reason)
		return nil, apierror.NewInvalidAuth()
	}

	clientCert := certs[0]

	trustCache := module.env.GetManagers().Ca.GetTrustCache()

	trustCache.RLock()
	defer trustCache.RUnlock()

	var targetThirdPartyCa *Ca
	chains, err := module.verifyClientCerts(certs, trustCache.staticFirstPartyRootPool)

	if err != nil {
		logger.WithError(err).Error("error verifying client certificate via static root pool, trying other pools")
	}

	failedRootOnlyBundle := false

	if len(chains) == 0 {
		failedRootOnlyBundle = true

		logger.Debug("certificate validation failed via root only pool, trying other pools")
		chains, err = module.verifyClientCerts(certs, trustCache.staticFirstPartyTrustAnchorPool)

		if err != nil {
			logger.WithError(err).Error("error verifying client certificate via static trust anchor pool, trying other pools")
		}

		if len(chains) == 0 {
			logger.Debug("certificate validation failed via root+intermediate pool, trying third party pool")
			chains, err = module.verifyClientCerts(certs, trustCache.thirdPartyTrustAnchorPool)

			if err == nil {
				targetThirdPartyCa = getCaByChain(trustCache.activeThirdPartyCas, chains, module.env.GetFingerprintGenerator())

				if len(chains) == 0 {
					logger.Debug("certificate validation failed via third party pool")
				}
			} else {
				logger.WithError(err).Error("error verifying client certificate via third party anchor pool")
			}
		}
	}

	if len(chains) == 0 {
		reason := "failed to verify client via all pools, no valid roots"
		logger.Error(reason)

		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		return nil, apierror.NewInvalidAuth()
	}

	externalId := ""
	if targetThirdPartyCa != nil {
		externalId, err = targetThirdPartyCa.GetExternalId(clientCert)
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
			reason := "failed to find identity by externalId"
			failEvent := module.NewAuthEventFailure(context, bundle, reason)
			module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

			logger.Error(reason)

			return nil, apierror.NewInvalidAuth()
		}

		authenticator = module.authenticatorExternalId(identity.Id, clientCert)

	} else {
		fingerprint := module.env.GetFingerprintGenerator().FromCert(clientCert)
		logger = logger.WithField("fingerprint", fingerprint)

		authenticator, _ = module.env.GetManagers().Authenticator.ReadByFingerprint(fingerprint)

		if authenticator == nil {
			reason := "failed to find authenticator by fingerprint"
			failEvent := module.NewAuthEventFailure(context, bundle, reason)
			module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

			logger.Error(reason)

			return nil, apierror.NewInvalidAuth()
		}

		identity, _ = module.env.GetManagers().Identity.Read(authenticator.IdentityId)
	}

	if identity == nil {
		reason := "failed to find a valid identity for authentication"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("authenticatorId", authenticator.Id).
		WithField("authenticatorMethod", authenticator.Method).
		WithField("identityId", authenticator.IdentityId).
		WithField("authPolicyId", identity.AuthPolicyId)

	if identity.Disabled {
		until := "forever"
		if identity.DisabledUntil != nil {
			until = identity.DisabledUntil.Format(time.RFC3339)
		}

		at := "unknown"
		if identity.DisabledAt != nil {
			at = identity.DisabledAt.Format(time.RFC3339)
		}
		reason := fmt.Sprintf("authentication failed, identity is disabled (disabledAt: %s, disabledUntil: %s)", at, until)
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.WithField("disabledUntil", until).WithField("disabledAt", at).Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	authPolicy, _ := module.env.GetManagers().AuthPolicy.Read(identity.AuthPolicyId)

	if authPolicy == nil {
		reason := "failed to obtain authPolicy by id"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.Error(reason)
		return nil, apierror.NewInvalidAuth()
	}

	if !authPolicy.Primary.Cert.Allowed {
		reason := "invalid certificate authentication, not allowed by auth policy"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	if !authPolicy.Primary.Cert.AllowExpiredCerts {
		if !module.isCertExpirationValid(clientCert) {
			reason := "failed to verify expiration period of client certificate"
			failEvent := module.NewAuthEventFailure(context, bundle, reason)
			module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

			logger.Error(reason)

			return nil, apierror.NewInvalidAuth()
		}
	}
	certAuth := authenticator.ToCert()

	if certAuth == nil {
		reason := "failed to convert authenticator to cert auth"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)
		module.env.GetEventDispatcher().AcceptAuthenticationEvent(failEvent)

		logger.Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	module.ensureAuthenticatorIsUpToDate(certAuth, clientCert, bundle.ImproperClientCertChain, context.GetChangeContext())

	improperClientCertChain := certAuth.IsIssuedByNetwork && failedRootOnlyBundle

	return &AuthResultBase{
		identity:                identity,
		authenticatorId:         authenticator.Id,
		authenticator:           authenticator,
		sessionCerts:            []*x509.Certificate{clientCert},
		authPolicy:              authPolicy,
		env:                     module.env,
		improperClientCertChain: improperClientCertChain,
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

// ensureAuthenticatorIsUpToDate ensures that a client's pem, public print, and root status is stored in `cert` authenticators.
func (module *AuthModuleCert) ensureAuthenticatorIsUpToDate(authCert *AuthenticatorCert, clientCert *x509.Certificate, verifiedToRoot bool, ctx *change.Context) {

	needsUpdate := false

	if authCert.Pem == "" {
		needsUpdate = true
		certPem := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: clientCert.Raw,
		})

		authCert.Pem = string(certPem)

	}

	if authCert.PublicKeyPrint == "" {
		needsUpdate = true
		authCert.PublicKeyPrint = PublicKeySha256(clientCert)
	}

	if authCert.IsIssuedByNetwork {
		if authCert.LastAuthResolvedToRoot != verifiedToRoot {
			needsUpdate = true
			authCert.LastAuthResolvedToRoot = verifiedToRoot
		}
	}

	if needsUpdate {
		if err := module.env.GetManagers().Authenticator.Update(authCert.Authenticator, false, nil, ctx); err != nil {
			pfxlog.Logger().WithError(err).Errorf("error during cert auth update attempt")
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
