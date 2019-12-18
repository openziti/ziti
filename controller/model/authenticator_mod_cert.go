/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"net/http"
)

const (
	ClientCertHeader       = "X-Client-CertPem"
	EdgeRouterProxyRequest = "X-Edge-Router-Proxy-Request"
)

type AuthModuleCert struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewAuthModuleCert(env Env) *AuthModuleCert {
	handler := &AuthModuleCert{
		env:                  env,
		method:               persistence.MethodAuthenticatorCert,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}
	return handler
}

func (module *AuthModuleCert) CanHandle(method string) bool {
	return method == module.method
}

func (module *AuthModuleCert) Process(context AuthContext) (string, error) {
	fingerprints, err := module.GetFingerprints(context)

	if err != nil {
		return "", err
	}

	for fingerprint := range fingerprints {
		authenticator, err := module.env.GetHandlers().Authenticator.HandleReadByFingerprint(fingerprint)

		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("error during cert auth read by fingerprint %s", fingerprint)
		}

		if authenticator != nil {
			return authenticator.IdentityId, nil
		}
	}

	return "", apierror.NewInvalidAuth()
}

func (module *AuthModuleCert) isEdgeRouter(certs []*x509.Certificate) bool {

	for _, cert := range certs {
		fingerprint := module.fingerprintGenerator.FromCert(cert)

		router, err := module.env.GetHandlers().EdgeRouter.HandleReadOneByFingerprint(fingerprint)

		if router != nil {
			return true
		}

		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not read edge router by fingerprint %s", fingerprint)
		}
	}
	return false
}

func (module *AuthModuleCert) GetFingerprints(ctx AuthContext) (cert.Fingerprints, error) {
	peerCerts := ctx.GetCerts()
	authCerts := peerCerts
	proxiedRaw64 := ""

	if proxiedRaw64Interface := ctx.GetHeaders()[ClientCertHeader]; proxiedRaw64Interface != nil {
		proxiedRaw64 = proxiedRaw64Interface.(string)
	}

	isProxied := false

	if proxyHeader := ctx.GetHeaders()[EdgeRouterProxyRequest]; proxyHeader != nil {
		isProxied = true
	}

	if isProxied && proxiedRaw64 == "" {
		return nil, apierror.NewInvalidAuth()
	}

	if proxiedRaw64 != "" {

		isValid := module.isEdgeRouter(ctx.GetCerts())

		if !isValid {
			return nil, apierror.NewInvalidAuth()
		}

		var proxiedRaw []byte
		_, err := base64.StdEncoding.Decode(proxiedRaw, []byte(proxiedRaw64))

		if err != nil {
			return nil, &apierror.ApiError{
				Code:    apierror.CouldNotDecodeProxiedCertCode,
				Message: apierror.CouldNotDecodeProxiedCertMessage,
				Cause:   err,
				Status:  http.StatusBadRequest,
			}
		}

		proxiedCerts, err := x509.ParseCertificates(proxiedRaw)

		if err != nil {
			return nil, &apierror.ApiError{
				Code:    apierror.CouldNotParseX509FromDerCode,
				Message: apierror.CouldNotParseX509FromDerMessage,
				Cause:   err,
				Status:  http.StatusBadRequest,
			}
		}

		authCerts = proxiedCerts
	}

	return module.fingerprintGenerator.FromCerts(authCerts), nil
}
