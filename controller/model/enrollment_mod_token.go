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
	"errors"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
)

const (
	AuthorizationHeader = "authorization"
	TargetTokenIssuerId = "ziti-token-issuer-id"
)

type EnrollModuleToken struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleToken(env Env) *EnrollModuleToken {
	return &EnrollModuleToken{
		env:                  env,
		method:               db.MethodEnrollToken,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}
}

func (module *EnrollModuleToken) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleToken) Process(ctx EnrollmentContext) (*EnrollmentResult, error) {
	ctx.GetChangeContext().
		SetChangeAuthorType(change.AuthorTypeController).
		SetChangeAuthorId(module.env.GetId()).
		SetChangeAuthorName(module.env.GetId())

	verificationResult, err := module.verifyToken(ctx.GetHeaders())

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not verify token")
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if !verificationResult.IsValid() {
		pfxlog.Logger().WithError(err).Errorf("token failed verification")
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if verificationResult.NameClaimValue == "" {
		pfxlog.Logger().Error("token verified but name claim values was empty, cannot create identities with blank names")
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if verificationResult.IdClaimValue == "" {
		pfxlog.Logger().Error("token verified but identity id claim values was empty, cannot check for identities with a blank value")
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	err = module.checkForExistingIdentity(verificationResult.IdClaimValue)

	if err != nil {
		return nil, err
	}

	csrPem := ctx.GetData().ClientCsrPem
	isCertEnrollment := len(csrPem) != 0

	if isCertEnrollment && !verificationResult.TokenIssuer.EnrollToCertEnabled() {
		pfxlog.Logger().
			WithField("issuerId", verificationResult.TokenIssuer.Id()).
			WithField("issuerName", verificationResult.TokenIssuer.Name()).
			Errorf("token enrollment attempted but issuer %s - %s does not allow cert enrollment", verificationResult.TokenIssuer.Name(), verificationResult.TokenIssuer.Id())
		return nil, apierror.NewInvalidEnrollmentNotAllowed()
	}

	if !isCertEnrollment && !verificationResult.TokenIssuer.EnrollToTokenEnabled() {
		pfxlog.Logger().
			WithField("issuerId", verificationResult.TokenIssuer.Id()).
			WithField("issuerName", verificationResult.TokenIssuer.Name()).
			Errorf("token enrollment attempted but issuer %s - %s does not allow token enrollment", verificationResult.TokenIssuer.Name(), verificationResult.TokenIssuer.Id())
		return nil, apierror.NewInvalidEnrollmentNotAllowed()
	}

	authPolicyId := db.DefaultAuthPolicyId

	if verificationResult.TokenIssuer.EnrollmentAuthPolicyId() != "" {
		authPolicyId = verificationResult.TokenIssuer.EnrollmentAuthPolicyId()
	}

	authPolicy, err := module.env.GetManagers().AuthPolicy.Read(authPolicyId)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not read auth policy %s", authPolicyId)
		return nil, errorz.NewUnhandled(errors.New("could not read auth policy"))
	}

	if isCertEnrollment && !authPolicy.Primary.Cert.Allowed {
		pfxlog.Logger().Errorf("token enrollment attempted but auth policy %s does not allow cert authentication", authPolicyId)
		return nil, apierror.NewInvalidEnrollmentNotAllowed()
	}

	if !isCertEnrollment {
		if !authPolicy.Primary.ExtJwt.Allowed {
			pfxlog.Logger().Errorf("token enrollment attempted but auth policy %s does not allow ext-jwt authentication", authPolicyId)
			return nil, apierror.NewInvalidEnrollmentNotAllowed()
		}

		if !authPolicy.Primary.ExtJwt.AllowAllSigners && !stringz.Contains(authPolicy.Primary.ExtJwt.AllowedExtJwtSigners, verificationResult.TokenIssuer.Id()) {
			pfxlog.Logger().Errorf("token enrollment attempted but auth policy %s does not allow ext-jwt authentication from the matching token issue limited to %v given %s", authPolicyId, authPolicy.Primary.ExtJwt.AllowedExtJwtSigners, verificationResult.TokenIssuer.Id())
			return nil, apierror.NewInvalidEnrollmentNotAllowed()
		}
	}

	newIdentity := &Identity{
		BaseEntity: models.BaseEntity{
			Id: eid.New(),
		},
		Name:           verificationResult.NameClaimValue,
		IdentityTypeId: db.DefaultIdentityType,
		RoleAttributes: verificationResult.AttributeClaimValue,
		AuthPolicyId:   authPolicyId,
		ExternalId:     &verificationResult.IdClaimValue,
	}

	var newAuthenticator *Authenticator = nil
	clientChainPem := ""

	if isCertEnrollment {
		csr, err := cert.ParseCsrPem(csrPem)

		if err != nil {
			apiErr := apierror.NewCouldNotProcessCsr()
			apiErr.Cause = err
			apiErr.AppendCause = true
			return nil, apiErr
		}

		certRaw, err := module.env.GetApiClientCsrSigner().SignCsr(csr, &cert.SigningOpts{})

		if err != nil {
			apiErr := apierror.NewCouldNotProcessCsr()
			apiErr.Cause = err
			apiErr.AppendCause = true
			return nil, apiErr
		}

		fp := module.fingerprintGenerator.FromRaw(certRaw)

		clientChainPem, err = module.env.GetManagers().Enrollment.GetCertChainPem(certRaw)

		if err != nil {
			return nil, err
		}

		newAuthenticator = &Authenticator{
			BaseEntity: models.BaseEntity{
				Id: eid.New(),
			},
			Method:     db.MethodAuthenticatorCert,
			IdentityId: newIdentity.Id,
			SubType: &AuthenticatorCert{
				Fingerprint:       fp,
				Pem:               clientChainPem,
				IsIssuedByNetwork: true,
			},
		}
	}

	var content any
	var textContent []byte

	if newAuthenticator != nil {
		_, _, err = module.env.GetManagers().Identity.CreateWithAuthenticators(newIdentity, []*Authenticator{newAuthenticator}, ctx.GetChangeContext())

		if err != nil {
			pfxlog.Logger().WithError(err).Error("failed to create identity with authenticator")
			return nil, errorz.NewUnhandled(errors.New("could not create identity with authenticator"))
		}

		content = &rest_model.EnrollmentCerts{
			Cert: clientChainPem,
			Ca:   string(module.env.GetConfig().Edge.CaPems()),
		}
		textContent = []byte(clientChainPem)
	} else {
		err = module.env.GetManagers().Identity.Create(newIdentity, ctx.GetChangeContext())

		if err != nil {
			pfxlog.Logger().WithError(err).Error("failed to create identity")
			return nil, errorz.NewUnhandled(errors.New("could not create identity"))
		}

		content = &rest_model.Empty{}
		textContent = []byte("")
	}

	return &EnrollmentResult{
		Identity:      newIdentity,
		Authenticator: newAuthenticator,
		Content:       content,
		TextContent:   textContent,
		Status:        200,
	}, nil
}

func (module *EnrollModuleToken) verifyToken(headers Headers) (*TokenVerificationResult, error) {
	candidateTokens := headers.GetStrings(AuthorizationHeader)

	if len(candidateTokens) == 0 {
		return nil, errors.New("0 candidate tokens were supplied in the enrollment request header")
	}

	targetTokenIssuerIds := headers.GetStrings(TargetTokenIssuerId)

	if len(targetTokenIssuerIds) > 0 {
		return module.verifyTokenByTokenIssuerId(targetTokenIssuerIds[0], candidateTokens)
	} else {
		return module.verifyTokenIssuerByInspection(candidateTokens)
	}
}

func (module *EnrollModuleToken) verifyTokenByTokenIssuerId(targetTokenIssuerId string, candidateTokens []string) (*TokenVerificationResult, error) {
	tokenIssuer := module.env.GetTokenIssuerCache().GetById(targetTokenIssuerId)

	if tokenIssuer == nil {
		return nil, errors.New("no token issuer with id " + targetTokenIssuerId + " was found")
	}

	if !tokenIssuer.IsEnabled() {
		return nil, errors.New("token issuer with id " + targetTokenIssuerId + " is not enabled")
	}

	for i, candidateToken := range candidateTokens {
		if !strings.HasPrefix(candidateToken, "Bearer ") {
			pfxlog.Logger().Debugf("candidate token at header index %d was not a bearer token", i)
			continue
		}

		candidateToken = strings.TrimPrefix(candidateToken, "Bearer ")

		result, err := tokenIssuer.VerifyToken(candidateToken)

		if err != nil {
			pfxlog.Logger().WithError(err).Debugf("could not verify candidate token at header index %d", i)
			continue
		}

		if !result.Token.Valid {
			pfxlog.Logger().Debugf("candidate token at header index %d was not valid", i)
			continue
		}

		return result, nil
	}

	return nil, errors.New("no valid candidate tokens were found")
}

func (module *EnrollModuleToken) verifyTokenIssuerByInspection(candidateTokens []string) (*TokenVerificationResult, error) {
	knownIssuers := module.env.GetTokenIssuerCache().GetIssuerStrings()
	var seenIssuers []string

	for i, candidateToken := range candidateTokens {
		if !strings.HasPrefix(candidateToken, "Bearer ") {
			pfxlog.Logger().Debugf("candidate token at header index %d was not a bearer token", i)
			continue
		}

		candidateToken = strings.TrimPrefix(candidateToken, "Bearer ")
		candidateToken = strings.TrimSpace(candidateToken)

		if !IsJwt(candidateToken) {
			pfxlog.Logger().Debugf("candidate token at header index %d was not a jwt", i)
			continue
		}

		tokenVerificationResult, err := module.env.GetTokenIssuerCache().VerifyTokenByInspection(candidateToken)

		if err != nil {
			pfxlog.Logger().WithError(err).Debugf("could not verify candidate token at header index %d, error encountered", i)
			continue
		}

		if !tokenVerificationResult.IsValid() {
			pfxlog.Logger().Debugf("candidate token at header index %d is not valid", i)
			continue
		}

		return tokenVerificationResult, nil
	}

	pfxlog.Logger().WithField("knownIssuers", knownIssuers).WithField("seenIssuers", seenIssuers).Warnf("no valid candidate tokens were found")
	return nil, errors.New("no valid candidate tokens were found")
}

func (module *EnrollModuleToken) checkForExistingIdentity(id string) error {

	existingIdentity, err := module.env.GetManagers().Identity.ReadByExternalId(id)

	if err != nil {
		resultErr := errors.New("could not read identity by external id to check for duplicate identities")
		pfxlog.Logger().WithError(err).Error(resultErr.Error())
		return errorz.NewUnhandled(resultErr)
	}

	if existingIdentity != nil {
		pfxlog.Logger().Errorf("duplicate identity found for external id %s", id)
		return apierror.NewEnrollmentIdentityAlreadyEnrolled()
	}

	return nil
}

func IsJwt(token string) bool {
	if strings.HasPrefix(token, "e") {
		parts := strings.Split(token, ".")
		return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
	}

	return false
}
