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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

func NewTransitRouterManager(env Env) *TransitRouterManager {
	manager := &TransitRouterManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().TransitRouter),
		allowedFields: boltz.MapFieldChecker{
			persistence.FieldName: struct{}{},
			boltz.FieldTags:       struct{}{},
		},
	}
	manager.impl = manager

	network.RegisterUpdateDecoder[*TransitRouter](env.GetHostController().GetNetwork().Managers, manager)
	network.RegisterDeleteDecoder(env.GetHostController().GetNetwork().Managers, manager)
	RegisterCommand(env, &CreateTransitRouterCmd{}, &edge_cmd_pb.CreateTransitRouterCmd{})

	return manager
}

type TransitRouterManager struct {
	baseEntityManager
	allowedFields boltz.FieldChecker
}

func (self *TransitRouterManager) newModelEntity() edgeEntity {
	return &TransitRouter{}
}

func (self *TransitRouterManager) Create(txRouter *TransitRouter) error {
	if txRouter.Id == "" {
		txRouter.Id = eid.New()
	}

	enrollment := &Enrollment{
		BaseEntity:      models.BaseEntity{},
		Method:          MethodEnrollTransitRouterOtt,
		TransitRouterId: &txRouter.Id,
	}

	cmd := &CreateTransitRouterCmd{
		manager:    self,
		router:     txRouter,
		enrollment: enrollment,
	}

	return self.Dispatch(cmd)
}

func (self *TransitRouterManager) ApplyCreate(cmd *CreateTransitRouterCmd) error {
	txRouter := cmd.router
	enrollment := cmd.enrollment

	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEntity, err := txRouter.toBoltEntityForCreate(tx, self.impl)
		if err != nil {
			return err
		}
		if err := self.GetStore().Create(ctx, boltEntity); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", self.GetStore().GetSingularEntityType())
			return err
		}

		if err = enrollment.FillJwtInfo(self.env, txRouter.Id); err != nil {
			return err
		}

		_, err = self.env.GetManagers().Enrollment.createEntityInTx(ctx, enrollment)
		return err
	})
}

func (self *TransitRouterManager) Update(entity *TransitRouter, unrestricted bool, checker fields.UpdatedFields) error {
	curEntity, err := self.Read(entity.Id)

	if err != nil {
		return err
	}

	if curEntity.IsBase {
		return apierror.NewFabricRouterCannotBeUpdate()
	}

	cmd := &command.UpdateEntityCommand[*TransitRouter]{
		Updater:       self,
		Entity:        entity,
		UpdatedFields: checker,
	}
	if unrestricted {
		cmd.Flags = updateUnrestricted
	}
	return self.Dispatch(cmd)
}

func (self *TransitRouterManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*TransitRouter]) error {
	var checker boltz.FieldChecker = cmd.UpdatedFields
	if cmd.Flags != updateUnrestricted {
		if checker == nil {
			checker = self.allowedFields
		} else {
			checker = &AndFieldChecker{first: self.allowedFields, second: cmd.UpdatedFields}
		}
	}
	return self.updateEntity(cmd.Entity, checker)
}

func (self *TransitRouterManager) ReadOneByFingerprint(fingerprint string) (*TransitRouter, error) {
	return self.ReadOneByQuery(fmt.Sprintf(`%s = "%v"`, db.FieldRouterFingerprint, fingerprint))
}

func (self *TransitRouterManager) ReadOneByQuery(query string) (*TransitRouter, error) {
	result, err := self.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*TransitRouter), nil
}

func (self *TransitRouterManager) Read(id string) (*TransitRouter, error) {
	result := &TransitRouter{}

	if err := self.readEntity(id, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (self *TransitRouterManager) readInTx(tx *bbolt.Tx, id string) (*TransitRouter, error) {
	modelEntity := &TransitRouter{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *TransitRouterManager) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (self *TransitRouterManager) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := self.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldTransitRouterEnrollments)
	for _, enrollmentId := range associationIds {
		enrollment, err := self.env.GetManagers().Enrollment.readInTx(tx, enrollmentId)
		if err != nil {
			return err
		}
		err = collector(enrollment)

		if err != nil {
			return err
		}
	}
	return nil
}

func (self *TransitRouterManager) ExtendEnrollment(router *TransitRouter, clientCsrPem []byte, serverCertCsrPem []byte) (*ExtendedCerts, error) {
	enrollmentModule := self.env.GetEnrollRegistry().GetByMethod("erott").(*EnrollModuleEr)

	clientCertRaw, err := enrollmentModule.ProcessClientCsrPem(clientCsrPem, router.Id)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	serverCertRaw, err := enrollmentModule.ProcessServerCsrPem(serverCertCsrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	fingerprint := self.env.GetFingerprintGenerator().FromRaw(clientCertRaw)

	pfxlog.Logger().Debugf("extending enrollment for router %s, old fingerprint: %s new fingerprint: %s", router.Id, *router.Fingerprint, fingerprint)

	router.Fingerprint = &fingerprint

	err = self.Update(router, true, &fields.UpdatedFieldsMap{
		persistence.FieldEdgeRouterCertPEM: struct{}{},
		db.FieldRouterFingerprint:          struct{}{},
	})

	if err != nil {
		return nil, err
	}

	return &ExtendedCerts{
		RawClientCert: clientCertRaw,
		RawServerCert: serverCertRaw,
	}, nil
}

func (self *TransitRouterManager) ExtendEnrollmentWithVerify(router *TransitRouter, clientCsrPem []byte, serverCertCsrPem []byte) (*ExtendedCerts, error) {
	enrollmentModule := self.env.GetEnrollRegistry().GetByMethod("erott").(*EnrollModuleEr)

	clientCertRaw, err := enrollmentModule.ProcessClientCsrPem(clientCsrPem, router.Id)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	serverCertRaw, err := enrollmentModule.ProcessServerCsrPem(serverCertCsrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	fingerprint := self.env.GetFingerprintGenerator().FromRaw(clientCertRaw)

	pfxlog.Logger().Debugf("extending enrollment for router %s, old fingerprint: %s new fingerprint: %s", router.Id, *router.Fingerprint, fingerprint)

	router.UnverifiedFingerprint = &fingerprint

	err = self.Update(router, true, &fields.UpdatedFieldsMap{
		persistence.FieldEdgeRouterUnverifiedCertPEM:     struct{}{},
		persistence.FieldEdgeRouterUnverifiedFingerprint: struct{}{},
	})

	if err != nil {
		return nil, err
	}

	return &ExtendedCerts{
		RawClientCert: clientCertRaw,
		RawServerCert: serverCertRaw,
	}, nil
}

func (self *TransitRouterManager) ReadOneByUnverifiedFingerprint(fingerprint string) (*TransitRouter, error) {
	return self.ReadOneByQuery(fmt.Sprintf(`%s = "%v"`, persistence.FieldEdgeRouterUnverifiedFingerprint, fingerprint))
}

func (self *TransitRouterManager) ExtendEnrollmentVerify(router *TransitRouter) error {
	if router.UnverifiedFingerprint != nil && router.UnverifiedCertPem != nil {
		router.Fingerprint = router.UnverifiedFingerprint

		router.UnverifiedFingerprint = nil
		router.UnverifiedCertPem = nil

		return self.Update(router, true, fields.UpdatedFieldsMap{
			db.FieldRouterFingerprint:                        struct{}{},
			persistence.FieldEdgeRouterUnverifiedCertPEM:     struct{}{},
			persistence.FieldEdgeRouterUnverifiedFingerprint: struct{}{},
		})
	}

	return errors.New("no outstanding verification necessary")
}

func (self *TransitRouterManager) TransitRouterToProtobuf(entity *TransitRouter) (*edge_cmd_pb.TransitRouter, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.TransitRouter{
		Id:                    entity.Id,
		Name:                  entity.Name,
		Tags:                  tags,
		IsVerified:            entity.IsVerified,
		Fingerprint:           entity.Fingerprint,
		UnverifiedFingerprint: entity.UnverifiedFingerprint,
		UnverifiedCertPem:     entity.UnverifiedCertPem,
		Cost:                  uint32(entity.Cost),
		NoTraversal:           entity.NoTraversal,
	}

	return msg, nil
}

func (self *TransitRouterManager) Marshall(entity *TransitRouter) ([]byte, error) {
	msg, err := self.TransitRouterToProtobuf(entity)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

func (self *TransitRouterManager) ProtobufToTransitRouter(msg *edge_cmd_pb.TransitRouter) (*TransitRouter, error) {
	return &TransitRouter{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:                  msg.Name,
		IsVerified:            msg.IsVerified,
		Fingerprint:           msg.Fingerprint,
		UnverifiedFingerprint: msg.UnverifiedFingerprint,
		UnverifiedCertPem:     msg.UnverifiedCertPem,
		Cost:                  uint16(msg.Cost),
		NoTraversal:           msg.NoTraversal,
	}, nil
}

func (self *TransitRouterManager) Unmarshall(bytes []byte) (*TransitRouter, error) {
	msg := &edge_cmd_pb.TransitRouter{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}
	return self.ProtobufToTransitRouter(msg)
}

type CreateTransitRouterCmd struct {
	manager    *TransitRouterManager
	router     *TransitRouter
	enrollment *Enrollment
}

func (self *CreateTransitRouterCmd) Apply() error {
	return self.manager.ApplyCreate(self)
}

func (self *CreateTransitRouterCmd) Encode() ([]byte, error) {
	transitRouterMsg, err := self.manager.TransitRouterToProtobuf(self.router)
	if err != nil {
		return nil, err
	}

	enrollment, err := self.manager.GetEnv().GetManagers().Enrollment.EnrollmentToProtobuf(self.enrollment)
	if err != nil {
		return nil, err
	}

	cmd := &edge_cmd_pb.CreateTransitRouterCmd{
		Router:     transitRouterMsg,
		Enrollment: enrollment,
	}

	return proto.Marshal(cmd)
}

func (self *CreateTransitRouterCmd) Decode(env Env, msg *edge_cmd_pb.CreateTransitRouterCmd) error {
	router, err := self.manager.ProtobufToTransitRouter(msg.Router)
	if err != nil {
		return err
	}

	enrollment, err := self.manager.GetEnv().GetManagers().Enrollment.ProtobufToEnrollment(msg.Enrollment)
	if err != nil {
		return err
	}

	self.manager = env.GetManagers().TransitRouter
	self.router = router
	self.enrollment = enrollment

	return nil
}
