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
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	edgeCert "github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"reflect"
	"strings"
	"time"
)

const updateUnrestricted = 1

type AuthenticatorManager struct {
	baseEntityManager[*Authenticator, *db.Authenticator]
	authStore db.AuthenticatorStore
}

func NewAuthenticatorManager(env Env) *AuthenticatorManager {
	manager := &AuthenticatorManager{
		baseEntityManager: newBaseEntityManager[*Authenticator, *db.Authenticator](env, env.GetStores().Authenticator),
		authStore:         env.GetStores().Authenticator,
	}

	manager.impl = manager

	network.RegisterManagerDecoder[*Authenticator](env.GetHostController().GetNetwork().GetManagers(), manager)

	return manager
}

func (self *AuthenticatorManager) newModelEntity() *Authenticator {
	return &Authenticator{}
}

func (self *AuthenticatorManager) IsUpdated(field string) bool {
	return !strings.EqualFold(field, "method") && !strings.EqualFold(field, "identityId")
}

func (self *AuthenticatorManager) Authorize(authContext AuthContext) (AuthResult, error) {
	authModule := self.env.GetAuthRegistry().GetByMethod(authContext.GetMethod())

	if authModule == nil {
		return nil, apierror.NewInvalidAuthMethod()
	}

	return authModule.Process(authContext)
}

func (self *AuthenticatorManager) ReadFingerprints(authenticatorId string) ([]string, error) {
	var authenticator *db.Authenticator

	err := self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		authenticator, err = self.authStore.LoadOneById(tx, authenticatorId)
		return err
	})

	if err != nil {
		return nil, err
	}

	return authenticator.ToSubType().Fingerprints(), nil
}

func (self *AuthenticatorManager) Read(id string) (*Authenticator, error) {
	modelEntity := &Authenticator{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *AuthenticatorManager) Create(entity *Authenticator, ctx *change.Context) error {
	return network.DispatchCreate[*Authenticator](self, entity, ctx)
}

func (self *AuthenticatorManager) ApplyCreate(cmd *command.CreateEntityCommand[*Authenticator], ctx boltz.MutateContext) error {
	authenticator := cmd.Entity
	if authenticator.Method != db.MethodAuthenticatorUpdb && authenticator.Method != db.MethodAuthenticatorCert {
		return errorz.NewFieldError("method must be updb or cert", "method", authenticator.Method)
	}

	queryString := fmt.Sprintf(`method = "%s"`, authenticator.Method)
	query, err := ast.Parse(self.GetStore(), queryString)
	if err != nil {
		return err
	}
	result, err := self.ListForIdentity(authenticator.IdentityId, query)

	if err != nil {
		return err
	}

	if result.GetMetaData().Count > 0 {
		return apierror.NewAuthenticatorMethodMax()
	}

	if authenticator.Method == db.MethodAuthenticatorUpdb {
		if updb, ok := authenticator.SubType.(*AuthenticatorUpdb); ok {
			hashResult := self.HashPassword(updb.Password)
			updb.Password = hashResult.Password
			updb.Salt = hashResult.Salt
		}
	}

	if authenticator.Method == db.MethodAuthenticatorCert {
		certs := nfpem.PemStringToCertificates(authenticator.ToCert().Pem)

		if len(certs) != 1 {
			err := apierror.NewCouldNotParsePem()
			err.Cause = errors.New("client pem must be exactly one certificate")
			err.AppendCause = true
			return err
		}

		cert := certs[0]
		fingerprint := self.env.GetFingerprintGenerator().FromCert(cert)
		authenticator.ToCert().Fingerprint = fingerprint

		opts := x509.VerifyOptions{
			Roots:         self.getRootPool(),
			Intermediates: x509.NewCertPool(),
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			CurrentTime:   cert.NotBefore,
		}

		if _, err := cert.Verify(opts); err != nil {
			return fmt.Errorf("error verifying client certificate [%s] did not verify against known CAs", fingerprint)
		}
	}

	_, err = self.createEntity(authenticator, ctx)
	return err
}

func (self *AuthenticatorManager) Update(entity *Authenticator, unrestricted bool, checker fields.UpdatedFields, ctx *change.Context) error {
	cmd := &command.UpdateEntityCommand[*Authenticator]{
		Context:       ctx,
		Updater:       self,
		Entity:        entity,
		UpdatedFields: checker,
	}
	if unrestricted {
		cmd.Flags = updateUnrestricted
	}
	return self.Dispatch(cmd)
}

func (self *AuthenticatorManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Authenticator], ctx boltz.MutateContext) error {
	authenticator := cmd.Entity
	if updb := authenticator.ToUpdb(); updb != nil {
		if cmd.UpdatedFields == nil || cmd.UpdatedFields.IsUpdated("password") {
			hashResult := self.HashPassword(updb.Password)
			updb.Password = hashResult.Password
			updb.Salt = hashResult.Salt
		}
	}

	if cert := authenticator.ToCert(); cert != nil {
		if cert.Pem != "" && (cmd.UpdatedFields == nil || cmd.UpdatedFields.IsUpdated(db.FieldAuthenticatorCertPem)) {
			if cert.Fingerprint = edgeCert.NewFingerprintGenerator().FromPem([]byte(cert.Pem)); cert.Fingerprint == "" {
				return apierror.NewCouldNotParsePem()
			}
		}
	}

	var checker boltz.FieldChecker = cmd.UpdatedFields
	if cmd.Flags != updateUnrestricted {
		if checker == nil {
			checker = self
		} else {
			checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
		}
	}
	return self.updateEntity(cmd.Entity, checker, ctx)
}

func (self *AuthenticatorManager) getRootPool() *x509.CertPool {
	roots := x509.NewCertPool()

	roots.AppendCertsFromPEM(self.env.GetConfig().CaPems())

	err := self.env.GetManagers().Ca.Stream("isVerified = true", func(ca *Ca, err error) error {
		if err != nil {
			//continue on err
			pfxlog.Logger().Errorf("error streaming cas for authentication: %vs", err)
			return nil
		}

		if ca != nil {
			roots.AppendCertsFromPEM([]byte(ca.CertPem))
		}

		return nil
	})

	if err != nil {
		return nil
	}

	return roots
}

func (self *AuthenticatorManager) ReadByUsername(username string) (*Authenticator, error) {
	query := fmt.Sprintf("%s = \"%v\"", db.FieldAuthenticatorUpdbUsername, username)

	entity, err := self.readEntityByQuery(query)

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

func (self *AuthenticatorManager) ReadByFingerprint(fingerprint string) (*Authenticator, error) {
	query := fmt.Sprintf("%s = \"%v\"", db.FieldAuthenticatorCertFingerprint, fingerprint)

	entity, err := self.readEntityByQuery(query)

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

func (self *AuthenticatorManager) UpdateSelf(authenticatorSelf *AuthenticatorSelf, ctx *change.Context) error {
	authenticator, err := self.ReadForIdentity(authenticatorSelf.IdentityId, authenticatorSelf.Id)

	if err != nil {
		return err
	}

	if authenticator == nil {
		return boltz.NewNotFoundError(self.authStore.GetSingularEntityType(), "id", authenticatorSelf.Id)
	}

	if authenticator.IdentityId != authenticatorSelf.IdentityId {
		return errorz.NewUnhandled(errors.New("authenticator does not match identity id for update"))
	}

	updbAuth := authenticator.ToUpdb()

	if updbAuth == nil {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	curHashResult := self.ReHashPassword(authenticatorSelf.CurrentPassword, updbAuth.DecodedSalt())

	if curHashResult.Password != updbAuth.Password {
		apiErr := errorz.NewUnauthorized()
		apiErr.Cause = errorz.NewFieldError("invalid current password", "currentPassword", authenticatorSelf.CurrentPassword)
		return apiErr
	}

	updbAuth.Username = authenticatorSelf.Username
	updbAuth.Password = authenticatorSelf.NewPassword
	updbAuth.Salt = ""
	authenticator.SubType = updbAuth

	return self.Update(authenticator, false, nil, ctx)
}

func (self *AuthenticatorManager) PatchSelf(authenticatorSelf *AuthenticatorSelf, checker fields.UpdatedFields, ctx *change.Context) error {
	if checker.IsUpdated("password") {
		checker.AddField("salt")
	}

	authenticator, err := self.ReadForIdentity(authenticatorSelf.IdentityId, authenticatorSelf.Id)

	if err != nil {
		return err
	}

	if authenticator == nil {
		return boltz.NewNotFoundError(self.authStore.GetSingularEntityType(), "id", authenticatorSelf.Id)
	}

	if authenticator.IdentityId != authenticatorSelf.IdentityId {
		return errorz.NewUnhandled(errors.New("authenticator does not match identity id for patch"))
	}

	updbAuth := authenticator.ToUpdb()

	if updbAuth == nil {
		return apierror.NewAuthenticatorCannotBeUpdated()
	}

	curHashResult := self.ReHashPassword(authenticatorSelf.CurrentPassword, updbAuth.DecodedSalt())

	if curHashResult.Password != updbAuth.Password {
		apiErr := errorz.NewUnauthorized()
		apiErr.Cause = errorz.NewFieldError("invalid current password", "currentPassword", authenticatorSelf.CurrentPassword)
		return apiErr
	}

	updbAuth.Username = authenticatorSelf.Username
	updbAuth.Password = authenticatorSelf.NewPassword
	updbAuth.Salt = ""
	authenticator.SubType = updbAuth

	return self.Update(authenticator, false, checker, ctx)
}

func (self *AuthenticatorManager) HashPassword(password string) *HashedPassword {
	newResult := Hash(password)
	b64Password := base64.StdEncoding.EncodeToString(newResult.Hash)
	b64Salt := base64.StdEncoding.EncodeToString(newResult.Salt)

	return &HashedPassword{
		RawResult: newResult,
		Salt:      b64Salt,
		Password:  b64Password,
	}
}

func (self *AuthenticatorManager) ReHashPassword(password string, salt []byte) *HashedPassword {
	newResult := ReHash(password, salt)
	b64Password := base64.StdEncoding.EncodeToString(newResult.Hash)
	b64Salt := base64.StdEncoding.EncodeToString(newResult.Salt)

	return &HashedPassword{
		RawResult: newResult,
		Salt:      b64Salt,
		Password:  b64Password,
	}
}

func (self *AuthenticatorManager) DecodeSalt(salt string) []byte {
	result, _ := DecodeSalt(salt)
	return result
}

func (self *AuthenticatorManager) ListForIdentity(identityId string, query ast.Query) (*models.EntityListResult[*Authenticator], error) {
	filterString := fmt.Sprintf(`identity = "%s"`, identityId)
	filter, err := ast.Parse(self.Store, filterString)
	if err != nil {
		return nil, err
	}

	if query != nil {
		query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), filter))
	} else {
		query = filter
	}

	return self.BasePreparedList(query)
}

func (self *AuthenticatorManager) ReadForIdentity(identityId string, authenticatorId string) (*Authenticator, error) {
	authenticator, err := self.Read(authenticatorId)

	if err != nil {
		return nil, err
	}

	if authenticator.IdentityId == identityId {
		return authenticator, err
	}

	return nil, nil
}

func (self *AuthenticatorManager) ExtendCertForIdentity(identityId string, authenticatorId string, peerCerts []*x509.Certificate, csrPem string, ctx *change.Context) ([]byte, error) {
	authenticator, _ := self.Read(authenticatorId)

	if authenticator == nil {
		return nil, errorz.NewNotFound()
	}

	if authenticator.Method != db.MethodAuthenticatorCert {
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
		fingerprint := self.env.GetFingerprintGenerator().FromCert(cert)
		if fingerprint == authenticatorCert.Fingerprint {
			validClientCert = cert
			break
		}
	}

	if validClientCert == nil {
		return nil, errorz.NewUnauthorized()
	}

	caPool := x509.NewCertPool()
	config := self.env.GetConfig()
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

	newRawCert, err := self.env.GetApiClientCsrSigner().SignCsr(csr, opts)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	newFingerprint := self.env.GetFingerprintGenerator().FromRaw(newRawCert)
	newPemCert, err := edgeCert.RawToPem(newRawCert)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	authenticatorCert.UnverifiedPem = string(newPemCert)
	authenticatorCert.UnverifiedFingerprint = newFingerprint

	err = self.env.GetManagers().Authenticator.Update(authenticatorCert.Authenticator, false, fields.UpdatedFieldsMap{
		db.FieldAuthenticatorUnverifiedCertPem:         struct{}{},
		db.FieldAuthenticatorUnverifiedCertFingerprint: struct{}{},
	}, ctx)

	if err != nil {
		return nil, err
	}

	return newPemCert, nil
}

func (self *AuthenticatorManager) VerifyExtendCertForIdentity(apiSessionId, identityId, authenticatorId string, verifyCertPem string, ctx *change.Context) error {
	authenticator, _ := self.Read(authenticatorId)

	if authenticator == nil {
		return errorz.NewNotFound()
	}

	if authenticator.Method != db.MethodAuthenticatorCert {
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
	verifyCerts := nfpem.PemStringToCertificates(verifyCertPem)
	if len(verifyCerts) != 1 {
		return apierror.NewInvalidClientCertificate()
	}

	verifyPrint := nfpem.FingerprintFromCertificate(verifyCerts[0])

	if verifyPrint != authenticatorCert.UnverifiedFingerprint {
		return apierror.NewInvalidClientCertificate()
	}

	oldFingerprint := authenticatorCert.Fingerprint

	authenticatorCert.Pem = authenticatorCert.UnverifiedPem
	authenticatorCert.Fingerprint = authenticatorCert.UnverifiedFingerprint

	authenticatorCert.UnverifiedFingerprint = ""
	authenticatorCert.UnverifiedPem = ""

	err := self.env.GetManagers().Authenticator.Update(authenticatorCert.Authenticator, true, fields.UpdatedFieldsMap{
		"fingerprint":                                  struct{}{},
		db.FieldAuthenticatorUnverifiedCertPem:         struct{}{},
		db.FieldAuthenticatorUnverifiedCertFingerprint: struct{}{},

		db.FieldAuthenticatorCertPem:         struct{}{},
		db.FieldAuthenticatorCertFingerprint: struct{}{},
	}, ctx)

	if err != nil {
		return err
	}

	sessionCert := &db.ApiSessionCertificate{
		BaseExtEntity: boltz.BaseExtEntity{
			Id: eid.New(),
		},
		ApiSessionId: apiSessionId,
		Subject:      verifyCerts[0].Subject.String(),
		Fingerprint:  verifyPrint,
		ValidAfter:   &verifyCerts[0].NotBefore,
		ValidBefore:  &verifyCerts[0].NotAfter,
		PEM:          verifyCertPem,
	}

	return self.env.GetDbProvider().GetDb().Update(ctx.NewMutateContext(), func(mutateCtx boltz.MutateContext) error {
		if err = self.env.GetStores().ApiSessionCertificate.Create(mutateCtx, sessionCert); err != nil {
			return err
		}
		return self.env.GetStores().ApiSessionCertificate.DeleteWhere(mutateCtx, fmt.Sprintf("%s=\"%s\"", db.FieldApiSessionCertificateFingerprint, oldFingerprint))
	})
}

// ReEnroll converts the given authenticator `id` back to an enrollment of the same type with the same
// constraints that expires at the time specified by `expiresAt`. The result is a string id of the new enrollment
// or an error.
func (self *AuthenticatorManager) ReEnroll(id string, expiresAt time.Time, ctx *change.Context) (string, error) {
	authenticator, err := self.Read(id)
	if err != nil {
		return "", err
	}

	enrollmentId := eid.New()
	enrollment := &Enrollment{
		BaseEntity: models.BaseEntity{
			Id: enrollmentId,
		},
		IdentityId: &authenticator.IdentityId,
		Token:      uuid.NewString(),
	}
	switch authenticator.Method {
	case db.MethodAuthenticatorCert:
		certAuth := authenticator.ToCert()

		caId := getCaId(self.env, certAuth)

		if caId != "" {
			enrollment.Method = db.MethodEnrollOttCa
			enrollment.CaId = &caId
		} else {
			enrollment.Method = db.MethodEnrollOtt
		}

	case db.MethodAuthenticatorUpdb:
		updbAuthenticator := authenticator.ToUpdb()
		enrollment.Method = db.MethodEnrollUpdb
		enrollment.IdentityId = &updbAuthenticator.IdentityId
		enrollment.Username = &updbAuthenticator.Username
		enrollment.Token = uuid.NewString()
	}

	if err := enrollment.FillJwtInfoWithExpiresAt(self.env, authenticator.IdentityId, expiresAt); err != nil {
		return "", err
	}

	if err = self.env.GetManagers().Enrollment.Create(enrollment, ctx); err != nil {
		return "", err
	}

	if err = self.Delete(id, ctx); err != nil {
		_ = self.env.GetManagers().Enrollment.Delete(enrollmentId, ctx)
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
	err := env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
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
	if err != nil {
		pfxlog.Logger().WithError(err).Error("error while getting CaId")
	}

	return caId
}

func (self *AuthenticatorManager) AuthenticatorToProtobuf(entity *Authenticator) (*edge_cmd_pb.Authenticator, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Authenticator{
		Id:         entity.Id,
		IdentityId: entity.IdentityId,
		Tags:       tags,
	}

	if cert := entity.ToCert(); cert != nil {
		msg.Subtype = &edge_cmd_pb.Authenticator_Cert_{
			Cert: &edge_cmd_pb.Authenticator_Cert{
				Fingerprint:           cert.Fingerprint,
				Pem:                   cert.Pem,
				UnverifiedFingerprint: cert.UnverifiedFingerprint,
				UnverifiedPem:         cert.UnverifiedPem,
			},
		}
	} else if updb := entity.ToUpdb(); updb != nil {
		msg.Subtype = &edge_cmd_pb.Authenticator_Updb_{
			Updb: &edge_cmd_pb.Authenticator_Updb{
				Username: updb.Username,
				Password: updb.Password,
				Salt:     updb.Salt,
			},
		}
	}
	return msg, nil
}

func (self *AuthenticatorManager) Marshall(entity *Authenticator) ([]byte, error) {
	msg, err := self.AuthenticatorToProtobuf(entity)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

func (self *AuthenticatorManager) Unmarshall(bytes []byte) (*Authenticator, error) {
	msg := &edge_cmd_pb.Authenticator{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}
	return self.ProtobufToAuthenticator(msg)
}

func (self *AuthenticatorManager) ProtobufToAuthenticator(msg *edge_cmd_pb.Authenticator) (*Authenticator, error) {
	authenticator := &Authenticator{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		IdentityId: msg.IdentityId,
		SubType:    nil,
	}

	if msg.Subtype == nil {
		return nil, errors.Errorf("no subtype provided for authenticator with id: %v", msg.Id)
	}

	switch st := msg.Subtype.(type) {
	case *edge_cmd_pb.Authenticator_Cert_:
		if st.Cert == nil {
			return nil, errors.Errorf("no cert data provided for authenticator with id: %v", msg.Id)
		}

		authenticator.SubType = &AuthenticatorCert{
			Authenticator:         authenticator,
			Fingerprint:           st.Cert.Fingerprint,
			Pem:                   st.Cert.Pem,
			UnverifiedFingerprint: st.Cert.UnverifiedFingerprint,
			UnverifiedPem:         st.Cert.UnverifiedPem,
		}
		authenticator.Method = db.MethodAuthenticatorCert
	case *edge_cmd_pb.Authenticator_Updb_:
		if st.Updb == nil {
			return nil, errors.Errorf("no updb data provided for authenticator with id: %v", msg.Id)
		}

		authenticator.SubType = &AuthenticatorUpdb{
			Authenticator: authenticator,
			Username:      st.Updb.Username,
			Password:      st.Updb.Password,
			Salt:          st.Updb.Salt,
		}
		authenticator.Method = db.MethodAuthenticatorUpdb
	}

	return authenticator, nil
}

type HashedPassword struct {
	RawResult *HashResult //raw byte hash results
	Salt      string      //base64 encoded hash
	Password  string      //base64 encoded hash
}

type AuthenticatorListQueryResult struct {
	*models.EntityListResult[*Authenticator]
	Authenticators []*Authenticator
}
