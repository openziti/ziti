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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/identity"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"time"
)

type EnrollmentManager struct {
	baseEntityManager[*Enrollment, *db.Enrollment]
	enrollmentStore db.EnrollmentStore
}

func NewEnrollmentManager(env Env) *EnrollmentManager {
	manager := &EnrollmentManager{
		baseEntityManager: newBaseEntityManager[*Enrollment, *db.Enrollment](env, env.GetStores().Enrollment),
		enrollmentStore:   env.GetStores().Enrollment,
	}

	manager.impl = manager

	RegisterManagerDecoder[*Enrollment](env, manager)
	RegisterCommand(env, &ReplaceEnrollmentWithAuthenticatorCmd{}, &edge_cmd_pb.ReplaceEnrollmentWithAuthenticatorCmd{})
	RegisterCommand(env, &ReEnrollEdgeRouterCmd{}, &edge_cmd_pb.ReEnrollEdgeRouterCmd{})

	return manager
}

func (self *EnrollmentManager) Create(entity *Enrollment, ctx *change.Context) error {
	return DispatchCreate[*Enrollment](self, entity, ctx)
}

func (self *EnrollmentManager) ApplyCreate(cmd *command.CreateEntityCommand[*Enrollment], ctx boltz.MutateContext) error {
	model := cmd.Entity

	if model.EdgeRouterId != nil || model.TransitRouterId != nil {
		_, err := self.createEntity(model, ctx)
		return err
	}

	if model.IdentityId == nil {
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError("identity not found", "identityId", model.IdentityId))
	}

	identity, err := self.env.GetManagers().Identity.Read(*model.IdentityId)

	if err != nil || identity == nil {
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError("identity not found", "identityId", model.IdentityId))
	}

	if model.ExpiresAt.Before(time.Now()) {
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError("expiresAt must be in the future", "expiresAt", model.ExpiresAt))
	}

	expiresAt := model.ExpiresAt.UTC()
	model.ExpiresAt = &expiresAt

	switch model.Method {
	case db.MethodEnrollOttCa:
		if model.CaId == nil {
			return apierror.NewBadRequestFieldError(*errorz.NewFieldError("ca not found", "caId", model.CaId))
		}

		ca, err := self.env.GetManagers().Ca.Read(*model.CaId)

		if err != nil || ca == nil {
			return apierror.NewBadRequestFieldError(*errorz.NewFieldError("ca not found", "caId", model.CaId))
		}
	case db.MethodAuthenticatorUpdb:
		if model.Username == nil || *model.Username == "" {
			return apierror.NewBadRequestFieldError(*errorz.NewFieldError("username not provided", "username", model.Username))
		}
	case db.MethodEnrollOtt:
	default:
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError("unsupported enrollment method", "method", model.Method))
	}

	enrollments, err := self.Query(fmt.Sprintf(`identity="%s"`, identity.Id))

	if err != nil {
		return err
	}

	for _, enrollment := range enrollments {
		if enrollment.Method == model.Method {
			return apierror.NewEnrollmentExists(model.Method)
		}
	}

	if err := model.FillJwtInfoWithExpiresAt(self.env, identity.Id, *model.ExpiresAt); err != nil {
		return err
	}

	_, err = self.createEntity(model, ctx)
	return err
}

func (self *EnrollmentManager) Update(entity *Enrollment, checker fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*Enrollment](self, entity, checker, ctx)
}

func (self *EnrollmentManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Enrollment], ctx boltz.MutateContext) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

func (self *EnrollmentManager) newModelEntity() *Enrollment {
	return &Enrollment{}
}

func (self *EnrollmentManager) getEnrollmentMethod(ctx EnrollmentContext) (string, error) {
	method := ctx.GetMethod()

	if method == db.MethodEnrollCa {
		return method, nil
	}

	token := ctx.GetToken()

	// token present, assumes all other enrollment methods
	enrollment, err := self.ReadByToken(token)

	if err != nil {
		return "", err
	}

	if enrollment == nil {
		return "", apierror.NewInvalidEnrollmentToken()
	}

	method = enrollment.Method

	return method, nil
}

func (self *EnrollmentManager) Enroll(ctx EnrollmentContext) (*EnrollmentResult, error) {
	method, err := self.getEnrollmentMethod(ctx)

	if err != nil {
		return nil, err
	}

	enrollModule := self.env.GetEnrollRegistry().GetByMethod(method)

	if enrollModule == nil {
		return nil, apierror.NewInvalidEnrollMethod()
	}

	return enrollModule.Process(ctx)
}

func (self *EnrollmentManager) ReadByToken(token string) (*Enrollment, error) {
	enrollment := &Enrollment{}

	err := self.env.GetDb().View(func(tx *bbolt.Tx) error {
		boltEntity, err := self.env.GetStores().Enrollment.LoadOneByToken(tx, token)

		if err != nil {
			return err
		}

		if boltEntity == nil {
			enrollment = nil
			return nil
		}

		return enrollment.fillFrom(self.env, tx, boltEntity)
	})

	if err != nil {
		return nil, err
	}

	return enrollment, nil
}

func (self *EnrollmentManager) ReplaceWithAuthenticator(enrollmentId string, authenticator *Authenticator, ctx *change.Context) error {
	return self.Dispatch(&ReplaceEnrollmentWithAuthenticatorCmd{
		manager:       self,
		enrollmentId:  enrollmentId,
		authenticator: authenticator,
		ctx:           ctx,
	})
}

// GetCertChainPem parses a given certificate in raw DER and attempt to provide string in PEM format of the
// original certificate followed by each signing intermediate up to but not including the root CA.
func (self *EnrollmentManager) GetCertChainPem(certRaw []byte) (string, error) {
	targetCert, err := x509.ParseCertificate(certRaw)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("error parsing cert raw during enrollment, attempting to assemble chain")
		return "", err
	}

	var targetChainPem []byte

	pool := identity.NewCaPool(self.env.GetConfig().Edge.CaCerts())
	targetChain := pool.GetChainMinusRoot(targetCert)

	for _, c := range targetChain {
		pemData, err := cert.RawToPem(c.Raw)
		if err != nil {
			return "", err
		}
		targetChainPem = append(targetChainPem, pemData...)
	}

	return string(targetChainPem), nil
}

func (self *EnrollmentManager) ApplyReplaceEncoderWithAuthenticatorCommand(cmd *ReplaceEnrollmentWithAuthenticatorCmd, ctx boltz.MutateContext) error {
	return self.env.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		err := self.env.GetStores().Enrollment.DeleteById(ctx, cmd.enrollmentId)
		if err != nil {
			return err
		}

		_, err = self.env.GetManagers().Authenticator.createEntityInTx(ctx, cmd.authenticator)
		return err
	})
}

func (self *EnrollmentManager) readInTx(tx *bbolt.Tx, id string) (*Enrollment, error) {
	modelEntity := &Enrollment{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *EnrollmentManager) Read(id string) (*Enrollment, error) {
	entity := &Enrollment{}
	if err := self.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *EnrollmentManager) RefreshJwt(id string, expiresAt time.Time, ctx *change.Context) error {
	enrollment, err := self.Read(id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			return errorz.NewNotFound()
		}

		return err
	}

	if enrollment.Jwt == "" {
		return apierror.NewInvalidEnrollMethod()
	}

	if expiresAt.Before(time.Now()) {
		return errorz.NewFieldError("must be after the current date and time", "expiresAt", expiresAt)
	}

	if err = enrollment.FillJwtInfoWithExpiresAt(self.env, *enrollment.IdentityId, expiresAt); err != nil {
		return err
	}

	return self.Update(enrollment, fields.UpdatedFieldsMap{
		db.FieldEnrollmentJwt:       struct{}{},
		db.FieldEnrollmentExpiresAt: struct{}{},
		db.FieldEnrollmentIssuedAt:  struct{}{},
	}, ctx)
}

func (self *EnrollmentManager) Query(query string) ([]*Enrollment, error) {
	var enrollments []*Enrollment
	if err := self.ListWithHandler(query, func(tx *bbolt.Tx, ids []string, qmd *models.QueryMetaData) error {
		for _, id := range ids {
			enrollment, _ := self.readInTx(tx, id)

			if enrollment != nil {
				enrollments = append(enrollments, enrollment)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return enrollments, nil
}

func (self *EnrollmentManager) EnrollmentToProtobuf(entity *Enrollment) (*edge_cmd_pb.Enrollment, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Enrollment{
		Id:              entity.Id,
		Tags:            tags,
		Method:          entity.Method,
		IdentityId:      entity.IdentityId,
		TransitRouterId: entity.TransitRouterId,
		EdgeRouterId:    entity.EdgeRouterId,
		Token:           entity.Token,
		IssuedAt:        timePtrToPb(entity.IssuedAt),
		ExpiresAt:       timePtrToPb(entity.ExpiresAt),
		Jwt:             entity.Jwt,
		CaId:            entity.CaId,
		Username:        entity.Username,
	}

	return msg, nil
}

func (self *EnrollmentManager) Marshall(entity *Enrollment) ([]byte, error) {
	msg, err := self.EnrollmentToProtobuf(entity)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

func (self *EnrollmentManager) ProtobufToEnrollment(msg *edge_cmd_pb.Enrollment) (*Enrollment, error) {
	return &Enrollment{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Method:          msg.Method,
		IdentityId:      msg.IdentityId,
		TransitRouterId: msg.TransitRouterId,
		EdgeRouterId:    msg.EdgeRouterId,
		Token:           msg.Token,
		IssuedAt:        pbTimeToTimePtr(msg.IssuedAt),
		ExpiresAt:       pbTimeToTimePtr(msg.ExpiresAt),
		Jwt:             msg.Jwt,
		CaId:            msg.CaId,
		Username:        msg.Username,
	}, nil
}

func (self *EnrollmentManager) Unmarshall(bytes []byte) (*Enrollment, error) {
	msg := &edge_cmd_pb.Enrollment{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}
	return self.ProtobufToEnrollment(msg)
}

type ReEnrollEdgeRouterCmd struct {
	ctx          *change.Context
	manager      *EnrollmentManager
	edgeRouterId string
}

func (d *ReEnrollEdgeRouterCmd) Decode(env Env, msg *edge_cmd_pb.ReEnrollEdgeRouterCmd) error {
	d.edgeRouterId = msg.EdgeRouterId
	d.ctx = ProtobufToContext(msg.Ctx)
	d.manager = env.GetManagers().Enrollment

	return nil
}

func (d *ReEnrollEdgeRouterCmd) Apply(ctx boltz.MutateContext) error {
	return d.manager.ApplyReEnrollEdgeRouter(d, ctx)
}

func (d *ReEnrollEdgeRouterCmd) GetChangeContext() *change.Context {
	return d.ctx
}

func (d *ReEnrollEdgeRouterCmd) Encode() ([]byte, error) {
	msg := &edge_cmd_pb.ReEnrollEdgeRouterCmd{
		EdgeRouterId: d.edgeRouterId,
	}

	return cmd_pb.EncodeProtobuf(msg)
}

func (self *EnrollmentManager) ApplyReEnrollEdgeRouter(cmd *ReEnrollEdgeRouterCmd, ctx boltz.MutateContext) error {
	log := pfxlog.Logger().WithField("routerId", cmd.edgeRouterId)

	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		log.Info("re-enrolling edge router, removing existing enrollments, creating a new one")

		edgeRouter, _, err := self.env.GetStores().EdgeRouter.FindById(ctx.Tx(), cmd.edgeRouterId)

		if err != nil {
			return err
		}

		if edgeRouter == nil {
			return fmt.Errorf("could not find edge router with id %s", cmd.edgeRouterId)
		}

		enrollmentIds, _, err := self.GetEnv().GetStores().Enrollment.QueryIds(ctx.Tx(), fmt.Sprintf(`%s = "%s"`, db.FieldEnrollEdgeRouter, cmd.edgeRouterId))

		if err != nil {
			return err
		}

		for _, enrollmentId := range enrollmentIds {
			err := self.GetEnv().GetStores().Enrollment.DeleteById(ctx, enrollmentId)

			if err != nil {
				return err
			}
		}

		enrollment := &Enrollment{
			BaseEntity: models.BaseEntity{
				Id: eid.New(),
			},
			Method:       MethodEnrollEdgeRouterOtt,
			EdgeRouterId: &cmd.edgeRouterId,
		}

		if err := enrollment.FillJwtInfo(self.env, cmd.edgeRouterId); err != nil {
			return fmt.Errorf("unable to fill jwt info for re-enrolling edge router: %v", err)
		}

		dbEnrollment, err := enrollment.toBoltEntityForCreate(ctx.Tx(), self.env)

		if err != nil {
			return errors.Wrap(err, "could not convert to bolt entity for create")
		}

		if err := self.env.GetStores().Enrollment.Create(ctx, dbEnrollment); err != nil {
			return errors.Wrap(err, "could not create enrollment for re-enrolling edge router")
		} else {
			log.WithField("enrollmentId", enrollment.Id).Infof("edge router re-enrollment entity created")
		}

		edgeRouter.Fingerprint = nil
		edgeRouter.CertPem = nil
		edgeRouter.IsVerified = false

		if err := self.env.GetStores().EdgeRouter.Update(ctx, edgeRouter, fields.UpdatedFieldsMap{
			db.FieldRouterFingerprint:    struct{}{},
			db.FieldEdgeRouterCertPEM:    struct{}{},
			db.FieldEdgeRouterIsVerified: struct{}{},
		}); err != nil {
			log.WithError(err).Error("unable to update re-enrolling edge router")
			return errors.Wrap(err, "unable to update re-enrolling edge router")
		}

		return nil
	})
}

type ReplaceEnrollmentWithAuthenticatorCmd struct {
	ctx           *change.Context
	manager       *EnrollmentManager
	enrollmentId  string
	authenticator *Authenticator
}

func (self *ReplaceEnrollmentWithAuthenticatorCmd) Apply(ctx boltz.MutateContext) error {
	return self.manager.ApplyReplaceEncoderWithAuthenticatorCommand(self, ctx)
}

func (self *ReplaceEnrollmentWithAuthenticatorCmd) Encode() ([]byte, error) {
	authMsg, err := self.manager.GetEnv().GetManagers().Authenticator.AuthenticatorToProtobuf(self.authenticator)
	if err != nil {
		return nil, err
	}

	cmd := &edge_cmd_pb.ReplaceEnrollmentWithAuthenticatorCmd{
		Ctx:           ContextToProtobuf(self.ctx),
		EnrollmentId:  self.enrollmentId,
		Authenticator: authMsg,
	}
	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *ReplaceEnrollmentWithAuthenticatorCmd) Decode(env Env, msg *edge_cmd_pb.ReplaceEnrollmentWithAuthenticatorCmd) error {
	self.ctx = ProtobufToContext(msg.Ctx)
	self.manager = env.GetManagers().Enrollment
	self.enrollmentId = msg.EnrollmentId
	authenticator, err := env.GetManagers().Authenticator.ProtobufToAuthenticator(msg.Authenticator)
	if err != nil {
		return err
	}
	self.authenticator = authenticator
	return nil
}

func (self *ReplaceEnrollmentWithAuthenticatorCmd) GetChangeContext() *change.Context {
	return self.ctx
}
