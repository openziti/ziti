/*
	Copyright NetFoundry, Inc.

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
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/crypto"
	edgeCert "github.com/openziti/edge/internal/cert"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	nfpem "github.com/openziti/foundation/util/pem"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
	"time"
)

type AuthenticatorHandler struct {
	baseEntityManager
	authStore persistence.AuthenticatorStore
}

func (handler AuthenticatorHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler AuthenticatorHandler) IsUpdated(field string) bool {
	return !strings.EqualFold(field, "method") && !strings.EqualFold(field, "identityId")
}

func NewAuthenticatorHandler(env Env) *AuthenticatorHandler {
	handler := &AuthenticatorHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().Authenticator),
		authStore:         env.GetStores().Authenticator,
	}

	handler.impl = handler
	return handler
}

func (handler AuthenticatorHandler) newModelEntity() boltEntitySink {
	return &Authenticator{}
}

func (handler AuthenticatorHandler) Authorize(authContext AuthContext) (AuthResult, error) {

	authModule := handler.env.GetAuthRegistry().GetByMethod(authContext.GetMethod())

	if authModule == nil {
		return nil, apierror.NewInvalidAuthMethod()
	}

	return authModule.Process(authContext)
}

func (handler AuthenticatorHandler) ReadFingerprints(authenticatorId string) ([]string, error) {
	var authenticator *persistence.Authenticator

	err := handler.env.GetStores().DbProvider.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		authenticator, err = handler.authStore.LoadOneById(tx, authenticatorId)
		return err
	})

	if err != nil {
		return nil, err
	}

	return authenticator.ToSubType().Fingerprints(), nil
}

func (handler *AuthenticatorHandler) Read(id string) (*Authenticator, error) {
	modelEntity := &Authenticator{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *AuthenticatorHandler) Create(authenticator *Authenticator) (string, error) {
	if authenticator.Method != persistence.MethodAuthenticatorUpdb && authenticator.Method != persistence.MethodAuthenticatorCert {
		return "", errorz.NewFieldError("method must be updb or cert", "method", authenticator.Method)
	}

	queryString := fmt.Sprintf(`method = "%s"`, authenticator.Method)
	query, err := ast.Parse(handler.GetStore(), queryString)
	if err != nil {
		return "", err
	}
	result, err := handler.ListForIdentity(authenticator.IdentityId, query)

	if err != nil {
		return "", err
	}

	if result.GetMetaData().Count > 0 {
		return "", apierror.NewAuthenticatorMethodMax()
	}

	if authenticator.Method == persistence.MethodAuthenticatorUpdb {
		if updb, ok := authenticator.SubType.(*AuthenticatorUpdb); ok {
			hashResult := handler.HashPassword(updb.Password)
			updb.Password = hashResult.Password
			updb.Salt = hashResult.Salt
		}
	}

	if authenticator.Method == persistence.MethodAuthenticatorCert {
		certs := nfpem.PemStringToCertificates(authenticator.ToCert().Pem)

		if len(certs) != 1 {
			err := apierror.NewCouldNotParsePem()
			err.Cause = errors.New("client pem must be exactly one certificate")
			err.AppendCause = true
			return "", err
		}

		cert := certs[0]
		fingerprint := handler.env.GetFingerprintGenerator().FromCert(cert)
		authenticator.ToCert().Fingerprint = fingerprint

		opts := x509.VerifyOptions{
			Roots:         handler.getRootPool(),
			Intermediates: x509.NewCertPool(),
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			CurrentTime:   cert.NotBefore,
		}

		if _, err := cert.Verify(opts); err != nil {
			return "", fmt.Errorf("error verifying client certificate [%s] did not verify against known CAs", fingerprint)
		}
	}

	return handler.createEntity(authenticator)
}

func (handler AuthenticatorHandler) getRootPool() *x509.CertPool {
	roots := x509.NewCertPool()

	roots.AppendCertsFromPEM(handler.env.GetConfig().CaPems())

	err := handler.env.GetManagers().Ca.Stream("isVerified = true", func(ca *Ca, err error) error {
		if ca == nil && err == nil {
			return nil
		}

		if err != nil {
			//continue on err
			pfxlog.Logger().Errorf("error streaming cas for authentication: %vs", err)
			return nil
		}
		roots.AppendCertsFromPEM([]byte(ca.CertPem))

		return nil
	})

	if err != nil {
		return nil
	}

	return roots
}

func (handler AuthenticatorHandler) ReadByUsername(username string) (*Authenticator, error) {
	query := fmt.Sprintf("%s = \"%v\"", persistence.FieldAuthenticatorUpdbUsername, username)

	entity, err := handler.readEntityByQuery(query)

	if err != nil {
		return nil, err
	}

	if entity == nil {
		return nil, nil
	}

	authenticator, ok := entity.(*Authenticator)

	if !ok {
		return nil, fmt.Errorf("could not cast from %v to authenticator", reflect.TypeOf(entity))
	}

	return authenticator, nil
}

func (handler AuthenticatorHandler) ReadByFingerprint(fingerprint string) (*Authenticator, error) {
	query := fmt.Sprintf("%s = \"%v\"", persistence.FieldAuthenticatorCertFingerprint, fingerprint)

	entity, err := handler.readEntityByQuery(query)

	if err != nil {
		return nil, err
	}

	if entity == nil {
		return nil, nil
	}

	authenticator, ok := entity.(*Authenticator)

	if !ok {
		return nil, fmt.Errorf("could not cast from %v to authenticator", reflect.TypeOf(entity))
	}

	return authenticator, nil
}

func (handler AuthenticatorHandler) Update(authenticator *Authenticator) error {
	if updb := authenticator.ToUpdb(); updb != nil {
		hashResult := handler.HashPassword(updb.Password)
		updb.Password = hashResult.Password
		updb.Salt = hashResult.Salt
	}

	if cert := authenticator.ToCert(); cert != nil && cert.Pem != "" {
		cert.Fingerprint = edgeCert.NewFingerprintGenerator().FromPem([]byte(cert.Pem))
	}

	return handler.updateEntity(authenticator, handler)
}

func (handler AuthenticatorHandler) UpdateSelf(authenticatorSelf *AuthenticatorSelf) error {
	authenticator, err := handler.ReadForIdentity(authenticatorSelf.IdentityId, authenticatorSelf.Id)

	if err != nil {
		return err
	}

	if authenticator == nil {
		return boltz.NewNotFoundError(handler.authStore.GetSingularEntityType(), "id", authenticatorSelf.Id)
	}

	if authenticator.IdentityId != authenticatorSelf.IdentityId {
		return errorz.NewUnhandled(errors.New("authenticator does not match identity id for update"))
	}

	updbAuth := authenticator.ToUpdb()

	if updbAuth == nil {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	curHashResult := handler.ReHashPassword(authenticatorSelf.CurrentPassword, updbAuth.DecodedSalt())

	if curHashResult.Password != updbAuth.Password {
		apiErr := errorz.NewUnauthorized()
		apiErr.Cause = errorz.NewFieldError("invalid current password", "currentPassword", authenticatorSelf.CurrentPassword)
		return apiErr
	}

	updbAuth.Username = authenticatorSelf.Username
	updbAuth.Password = authenticatorSelf.NewPassword
	updbAuth.Salt = ""
	authenticator.SubType = updbAuth

	return handler.Update(authenticator)
}

func (handler AuthenticatorHandler) Patch(authenticator *Authenticator, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	return handler.PatchUnrestricted(authenticator, combinedChecker)
}

func (handler AuthenticatorHandler) PatchUnrestricted(authenticator *Authenticator, checker boltz.FieldChecker) error {
	if authenticator.Method == persistence.MethodAuthenticatorUpdb {
		if updb := authenticator.ToUpdb(); updb != nil {
			if checker.IsUpdated("password") {
				hashResult := handler.HashPassword(updb.Password)
				updb.Password = hashResult.Password
				updb.Salt = hashResult.Salt
			}
		}
	}

	if authenticator.Method == persistence.MethodAuthenticatorCert {
		if cert := authenticator.ToCert(); cert != nil {
			if checker.IsUpdated(persistence.FieldAuthenticatorCertPem) {
				if cert.Fingerprint = edgeCert.NewFingerprintGenerator().FromPem([]byte(cert.Pem)); cert.Fingerprint == "" {
					return apierror.NewCouldNotParsePem()
				}
			}
		}
	}

	return handler.patchEntity(authenticator, checker)
}

func (handler AuthenticatorHandler) PatchSelf(authenticatorSelf *AuthenticatorSelf, checker boltz.FieldChecker) error {
	if checker.IsUpdated("password") {
		checker = NewOrFieldChecker(checker, "salt", "password")
	}

	authenticator, err := handler.ReadForIdentity(authenticatorSelf.IdentityId, authenticatorSelf.Id)

	if err != nil {
		return err
	}

	if authenticator == nil {
		return boltz.NewNotFoundError(handler.authStore.GetSingularEntityType(), "id", authenticatorSelf.Id)
	}

	if authenticator.IdentityId != authenticatorSelf.IdentityId {
		return errorz.NewUnhandled(errors.New("authenticator does not match identity id for patch"))
	}

	updbAuth := authenticator.ToUpdb()

	if updbAuth == nil {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	curHashResult := handler.ReHashPassword(authenticatorSelf.CurrentPassword, updbAuth.DecodedSalt())

	if curHashResult.Password != updbAuth.Password {
		apiErr := errorz.NewUnauthorized()
		apiErr.Cause = errorz.NewFieldError("invalid current password", "currentPassword", authenticatorSelf.CurrentPassword)
		return apiErr
	}

	updbAuth.Username = authenticatorSelf.Username
	updbAuth.Password = authenticatorSelf.NewPassword
	updbAuth.Salt = ""
	authenticator.SubType = updbAuth

	return handler.Patch(authenticator, checker)
}

func (handler AuthenticatorHandler) HashPassword(password string) *HashedPassword {
	newResult := crypto.Hash(password)
	b64Password := base64.StdEncoding.EncodeToString(newResult.Hash)
	b64Salt := base64.StdEncoding.EncodeToString(newResult.Salt)

	return &HashedPassword{
		RawResult: newResult,
		Salt:      b64Salt,
		Password:  b64Password,
	}
}

func (handler AuthenticatorHandler) ReHashPassword(password string, salt []byte) *HashedPassword {
	newResult := crypto.ReHash(password, salt)
	b64Password := base64.StdEncoding.EncodeToString(newResult.Hash)
	b64Salt := base64.StdEncoding.EncodeToString(newResult.Salt)

	return &HashedPassword{
		RawResult: newResult,
		Salt:      b64Salt,
		Password:  b64Password,
	}
}

func (handler AuthenticatorHandler) ListForIdentity(identityId string, query ast.Query) (*AuthenticatorListQueryResult, error) {
	filterString := fmt.Sprintf(`identity = "%s"`, identityId)
	filter, err := ast.Parse(handler.Store, filterString)
	if err != nil {
		return nil, err
	}
	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), filter))
	result, err := handler.BasePreparedList(query)

	if err != nil {
		return nil, err
	}

	var authenticators []*Authenticator

	for _, entity := range result.GetEntities() {
		if auth, ok := entity.(*Authenticator); ok {
			authenticators = append(authenticators, auth)
		}
	}

	return &AuthenticatorListQueryResult{
		EntityListResult: result,
		Authenticators:   authenticators,
	}, nil
}

func (handler AuthenticatorHandler) ReadForIdentity(identityId string, authenticatorId string) (*Authenticator, error) {
	authenticator, err := handler.Read(authenticatorId)

	if err != nil {
		return nil, err
	}

	if authenticator.IdentityId == identityId {
		return authenticator, err
	}

	return nil, nil
}

func (handler AuthenticatorHandler) ExtendCertForIdentity(identityId string, authenticatorId string, peerCerts []*x509.Certificate, csrPem string) ([]byte, error) {
	authenticator, _ := handler.Read(authenticatorId)

	if authenticator == nil {
		return nil, errorz.NewNotFound()
	}

	if authenticator.Method != persistence.MethodAuthenticatorCert {
		return nil, apierror.NewAuthenticatorCannotBeUpdated()
	}

	if authenticator.IdentityId != identityId {
		return nil, errorz.NewUnauthorized()
	}

	authenticatorCert := authenticator.ToCert()

	if authenticatorCert == nil {
		return nil, errorz.NewUnhandled(fmt.Errorf("%T is not a %T", authenticator, authenticatorCert))
	}

	if authenticatorCert.Pem == "" {
		return nil, apierror.NewAuthenticatorCannotBeUpdated()
	}

	var validClientCert *x509.Certificate = nil
	for _, cert := range peerCerts {
		fingerprint := handler.env.GetFingerprintGenerator().FromCert(cert)
		if fingerprint == authenticatorCert.Fingerprint {
			validClientCert = cert
			break
		}
	}

	if validClientCert == nil {
		return nil, errorz.NewUnauthorized()
	}

	caPool := x509.NewCertPool()
	config := handler.env.GetConfig()
	caPool.AddCert(config.Enrollment.SigningCert.Cert().Leaf)

	validClientCert.NotBefore = time.Now().Add(-1 * time.Hour)
	validClientCert.NotAfter = time.Now().Add(+1 * time.Hour)

	validChain, err := validClientCert.Verify(x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	if len(validChain) == 0 || err != nil {
		return nil, apierror.NewAuthenticatorCannotBeUpdated()
	}

	csr, err := edgeCert.ParseCsrPem([]byte(csrPem))

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr

	}

	currentCerts := nfpem.PemStringToCertificates(authenticatorCert.Pem)

	if len(currentCerts) != 1 {
		return nil, errorz.NewUnhandled(errors.New("could not parse current certificates pem"))
	}
	currentCert := currentCerts[0]

	opts := &edgeCert.SigningOpts{
		DNSNames:       currentCert.DNSNames,
		EmailAddresses: currentCert.EmailAddresses,
		IPAddresses:    currentCert.IPAddresses,
		URIs:           currentCert.URIs,
	}

	newRawCert, err := handler.env.GetApiClientCsrSigner().SignCsr(csr, opts)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	newFingerprint := handler.env.GetFingerprintGenerator().FromRaw(newRawCert)
	newPemCert, err := edgeCert.RawToPem(newRawCert)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	authenticatorCert.UnverifiedPem = string(newPemCert)
	authenticatorCert.UnverifiedFingerprint = newFingerprint

	err = handler.env.GetManagers().Authenticator.Patch(authenticatorCert.Authenticator, boltz.MapFieldChecker{
		persistence.FieldAuthenticatorUnverifiedCertPem:         struct{}{},
		persistence.FieldAuthenticatorUnverifiedCertFingerprint: struct{}{},
	})

	if err != nil {
		return nil, err
	}

	return newPemCert, nil
}

func (handler AuthenticatorHandler) VerifyExtendCertForIdentity(identityId, authenticatorId string, verifyCertPem string) error {
	authenticator, _ := handler.Read(authenticatorId)

	if authenticator == nil {
		return errorz.NewNotFound()
	}

	if authenticator.Method != persistence.MethodAuthenticatorCert {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	if authenticator.IdentityId != identityId {
		return errorz.NewUnauthorized()
	}

	authenticatorCert := authenticator.ToCert()

	if authenticatorCert == nil {
		return errorz.NewUnhandled(fmt.Errorf("%T is not a %T", authenticator, authenticatorCert))
	}

	if authenticatorCert.Pem == "" {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	if authenticatorCert.UnverifiedPem == "" || authenticatorCert.UnverifiedFingerprint == "" {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	verifyPrint := nfpem.FingerprintFromPemString(verifyCertPem)

	if verifyPrint != authenticatorCert.UnverifiedFingerprint {
		return apierror.NewInvalidClientCertificate()
	}

	authenticatorCert.Pem = authenticatorCert.UnverifiedPem
	authenticatorCert.Fingerprint = authenticatorCert.UnverifiedFingerprint

	authenticatorCert.UnverifiedFingerprint = ""
	authenticatorCert.UnverifiedPem = ""

	err := handler.env.GetManagers().Authenticator.PatchUnrestricted(authenticatorCert.Authenticator, boltz.MapFieldChecker{
		persistence.FieldSessionCertFingerprint:                 struct{}{},
		persistence.FieldAuthenticatorUnverifiedCertPem:         struct{}{},
		persistence.FieldAuthenticatorUnverifiedCertFingerprint: struct{}{},

		persistence.FieldAuthenticatorCertPem:         struct{}{},
		persistence.FieldAuthenticatorCertFingerprint: struct{}{},
	})

	return err
}

// ReEnroll converts the given authenticator `id` back to an enrollment of the same type with the same
// constraints that expires at the time specified by `expiresAt`. The result is a string id of the new enrollment
// or an error.
func (handler AuthenticatorHandler) ReEnroll(id string, expiresAt time.Time) (string, error) {
	authenticator, err := handler.Read(id)

	enrollment := &Enrollment{
		IdentityId: &authenticator.IdentityId,
		Token:      uuid.NewString(),
	}
	switch authenticator.Method {
	case persistence.MethodAuthenticatorCert:
		certAuth := authenticator.ToCert()

		caId := getCaId(handler.env, certAuth)

		if caId != "" {
			enrollment.Method = persistence.MethodEnrollOttCa
			enrollment.CaId = &caId
		} else {
			enrollment.Method = persistence.MethodEnrollOtt
		}

	case persistence.MethodAuthenticatorUpdb:
		updbAuthenticator := authenticator.ToUpdb()
		enrollment.Method = persistence.MethodEnrollUpdb
		enrollment.IdentityId = &updbAuthenticator.IdentityId
		enrollment.Username = &updbAuthenticator.Username
		enrollment.Token = uuid.NewString()
	}

	if err := enrollment.FillJwtInfoWithExpiresAt(handler.env, authenticator.IdentityId, expiresAt); err != nil {
		return "", err
	}

	enrollmentId, err := handler.env.GetManagers().Enrollment.createEntity(enrollment)

	if err != nil {
		return "", err
	}

	if err = handler.Delete(id); err != nil {
		_ = handler.env.GetManagers().Enrollment.Delete(enrollmentId)
		return "", err
	}

	return enrollmentId, err
}

// getCaId returns the string id of the issuing Ziti 3rd Party CA or empty string
func getCaId(env Env, auth *AuthenticatorCert) string {
	certs := nfpem.PemStringToCertificates(auth.Pem)

	if len(certs) == 0 {
		return ""
	}

	cert := certs[0]

	caId := ""
	env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := env.GetStores().Ca.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
			ca, err := env.GetStores().Ca.LoadOneById(tx, string(cursor.Current()))
			if err != nil {
				continue
			}

			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM([]byte(ca.CertPem))
			chains, err := cert.Verify(x509.VerifyOptions{
				Roots: pool,
			})

			if err != nil {
				continue
			}

			if len(chains) > 0 {
				caId = ca.Id
				break
			}
		}
		return nil
	})

	return caId
}

type HashedPassword struct {
	RawResult *crypto.HashResult //raw byte hash results
	Salt      string             //base64 encoded hash
	Password  string             //base64 encoded hash
}

type AuthenticatorListQueryResult struct {
	*models.EntityListResult
	Authenticators []*Authenticator
}
