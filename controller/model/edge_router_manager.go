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
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
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
)

func NewEdgeRouterManager(env Env) *EdgeRouterManager {
	manager := &EdgeRouterManager{
		baseEntityManager: newBaseEntityManager[*EdgeRouter, *db.EdgeRouter](env, env.GetStores().EdgeRouter),
		allowedFieldsChecker: fields.UpdatedFieldsMap{
			db.FieldName:                        struct{}{},
			db.FieldEdgeRouterIsTunnelerEnabled: struct{}{},
			db.FieldRoleAttributes:              struct{}{},
			boltz.FieldTags:                     struct{}{},
			db.FieldRouterCost:                  struct{}{},
			db.FieldRouterNoTraversal:           struct{}{},
			db.FieldRouterDisabled:              struct{}{},
			db.FieldEdgeRouterAppData:           struct{}{},
		},
	}

	manager.impl = manager

	isConnectedSymbol := boltz.NewBoolFuncSymbol(env.GetStores().EdgeRouter, "connected", func(id string) bool {
		return env.GetManagers().Router.IsConnected(id)
	})

	isOnlineSymbol := boltz.NewBoolFuncSymbol(env.GetStores().EdgeRouter, "online", func(id string) bool {
		return env.IsEdgeRouterOnline(id)
	})

	if store, ok := env.GetStores().EdgeRouter.(boltz.ConfigurableStore); ok {
		store.AddEntitySymbol(isConnectedSymbol)
		store.MakeSymbolPublic(isConnectedSymbol.GetName())

		store.AddEntitySymbol(isOnlineSymbol)
		store.MakeSymbolPublic(isOnlineSymbol.GetName())
	} else {
		panic("edge-router store is not boltz.ConfigurableStore")
	}

	RegisterCommand(env, &CreateEdgeRouterCmd{}, &edge_cmd_pb.CreateEdgeRouterCmd{})
	RegisterUpdateDecoder[*EdgeRouter](env, manager)
	RegisterDeleteDecoder(env, manager)

	return manager
}

type EdgeRouterManager struct {
	baseEntityManager[*EdgeRouter, *db.EdgeRouter]
	allowedFieldsChecker fields.UpdatedFieldsMap
}

func (self *EdgeRouterManager) GetEntityTypeId() string {
	return "edgeRouters"
}

func (self *EdgeRouterManager) newModelEntity() *EdgeRouter {
	return &EdgeRouter{}
}

func (self *EdgeRouterManager) Create(edgeRouter *EdgeRouter, ctx *change.Context) error {
	if edgeRouter.Id == "" {
		edgeRouter.Id = eid.New()
	}

	enrollment := &Enrollment{
		BaseEntity:   models.BaseEntity{Id: eid.New()},
		Method:       MethodEnrollEdgeRouterOtt,
		EdgeRouterId: &edgeRouter.Id,
	}

	cmd := &CreateEdgeRouterCmd{
		manager:    self,
		edgeRouter: edgeRouter,
		enrollment: enrollment,
		ctx:        ctx,
	}

	return self.Dispatch(cmd)
}

func (self *EdgeRouterManager) ApplyCreate(cmd *CreateEdgeRouterCmd, ctx boltz.MutateContext) error {
	edgeRouter := cmd.edgeRouter
	enrollment := cmd.enrollment

	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		boltEdgeRouter, err := edgeRouter.toBoltEntityForCreate(ctx.Tx(), self.env)
		if err != nil {
			return err
		}

		if err = self.ValidateNameOnCreate(ctx.Tx(), boltEdgeRouter); err != nil {
			return err
		}

		if err := self.GetStore().Create(ctx, boltEdgeRouter); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", self.GetStore().GetSingularEntityType())
			return err
		}

		if err = enrollment.FillJwtInfo(self.env, edgeRouter.Id); err != nil {
			return err
		}

		_, err = self.env.GetManagers().Enrollment.createEntityInTx(ctx, enrollment)
		return err
	})
}

func (self *EdgeRouterManager) Update(entity *EdgeRouter, unrestricted bool, checker fields.UpdatedFields, ctx *change.Context) error {
	cmd := &command.UpdateEntityCommand[*EdgeRouter]{
		Updater:       self,
		Entity:        entity,
		UpdatedFields: checker,
		Context:       ctx,
	}
	if unrestricted {
		cmd.Flags = updateUnrestricted
	}
	return self.Dispatch(cmd)
}

func (self *EdgeRouterManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*EdgeRouter], ctx boltz.MutateContext) error {
	var checker boltz.FieldChecker = cmd.UpdatedFields
	if cmd.Flags != updateUnrestricted {
		if checker == nil {
			checker = self.allowedFieldsChecker
		} else {
			checker = &AndFieldChecker{first: self.allowedFieldsChecker, second: cmd.UpdatedFields}
		}
	}
	return self.updateEntity(cmd.Entity, checker, ctx)
}

func (self *EdgeRouterManager) Read(id string) (*EdgeRouter, error) {
	modelEntity := &EdgeRouter{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *EdgeRouterManager) readInTx(tx *bbolt.Tx, id string) (*EdgeRouter, error) {
	modelEntity := &EdgeRouter{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *EdgeRouterManager) ReadOneByQuery(query string) (*EdgeRouter, error) {
	result, err := self.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*EdgeRouter), nil
}

func (self *EdgeRouterManager) ReadOneByFingerprint(fingerprint string) (*EdgeRouter, error) {
	return self.ReadOneByQuery(fmt.Sprintf(`fingerprint = "%v"`, fingerprint))
}

func (self *EdgeRouterManager) Query(query string) (*EdgeRouterListResult, error) {
	result := &EdgeRouterListResult{manager: self}
	err := self.ListWithHandler(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *EdgeRouterManager) ListForIdentityAndService(identityId, serviceId string) (*EdgeRouterListResult, error) {
	var list *EdgeRouterListResult
	var err error
	if txErr := self.env.GetDb().View(func(tx *bbolt.Tx) error {
		list, err = self.ListForIdentityAndServiceWithTx(tx, identityId, serviceId)
		return nil
	}); txErr != nil {
		return nil, txErr
	}

	return list, err
}

func (self *EdgeRouterManager) IsAccessToEdgeRouterAllowed(identityId, serviceId, edgeRouterId string) (bool, error) {
	var result bool
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		identityEdgeRouters := self.env.GetStores().Identity.GetRefCountedLinkCollection(db.EntityTypeRouters)
		serviceEdgeRouters := self.env.GetStores().EdgeService.GetRefCountedLinkCollection(db.FieldEdgeRouters)

		identityCount := identityEdgeRouters.GetLinkCount(tx, []byte(identityId), []byte(edgeRouterId))
		serviceCount := serviceEdgeRouters.GetLinkCount(tx, []byte(serviceId), []byte(edgeRouterId))
		result = identityCount != nil && *identityCount > 0 && serviceCount != nil && *serviceCount > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return result, nil
}

func (self *EdgeRouterManager) ListForIdentityAndServiceWithTx(tx *bbolt.Tx, identityId, serviceId string) (*EdgeRouterListResult, error) {
	query := fmt.Sprintf(`anyOf(identities) = "%v" and anyOf(services) = "%v" SORT BY connected DESC LIMIT 25`, identityId, serviceId)

	result := &EdgeRouterListResult{manager: self}
	if err := self.ListWithTx(tx, query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *EdgeRouterManager) IsSharedEdgeRouterPresent(identityId, serviceId string) (bool, error) {
	var result bool
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		identityEdgeRouters := self.env.GetStores().Identity.GetRefCountedLinkCollection(db.EntityTypeRouters)
		serviceEdgeRouters := self.env.GetStores().EdgeService.GetRefCountedLinkCollection(db.FieldEdgeRouters)

		cursor := identityEdgeRouters.IterateLinks(tx, []byte(identityId), true)
		for cursor.IsValid() {
			serviceCount := serviceEdgeRouters.GetLinkCount(tx, []byte(serviceId), cursor.Current())
			if result = serviceCount != nil && *serviceCount > 0; result {
				return nil
			}
			cursor.Next()
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return result, nil
}

func (self *EdgeRouterManager) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := self.env.GetStores().EdgeRouter.GetRoleAttributesIndex()
	return self.queryRoleAttributes(index, queryString)
}

func (self *EdgeRouterManager) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (self *EdgeRouterManager) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := self.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, db.EntityTypeEnrollments)
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

// ReEnroll creates a new JWT enrollment for an existing edge router. If the edge router already exists
// with a JWT, a new JWT is created. If the edge router was already enrolled, all record of the enrollment is
// reset and the edge router is disconnected forcing the edge router to complete enrollment before connecting.
func (self *EdgeRouterManager) ReEnroll(router *EdgeRouter, ctx *change.Context) error {
	cmd := &ReEnrollEdgeRouterCmd{
		ctx:          ctx,
		manager:      self.env.GetManagers().Enrollment,
		edgeRouterId: router.Id,
	}
	return self.Dispatch(cmd)
}

type ExtendedCerts struct {
	RawClientCert []byte
	RawServerCert []byte
}

func (self *EdgeRouterManager) ExtendEnrollment(router *EdgeRouter, clientCsrPem []byte, serverCertCsrPem []byte, ctx *change.Context) (*ExtendedCerts, error) {
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
	clientPem, _ := cert.RawToPem(clientCertRaw)
	clientPemString := string(clientPem)

	pfxlog.Logger().Debugf("extending enrollment for edge router %s, old fingerprint: %s new fingerprint: %s", router.Id, *router.Fingerprint, fingerprint)

	router.Fingerprint = &fingerprint
	router.CertPem = &clientPemString

	err = self.Update(router, true, &fields.UpdatedFieldsMap{
		db.FieldEdgeRouterCertPEM: struct{}{},
		db.FieldRouterFingerprint: struct{}{},
	}, ctx)

	if err != nil {
		return nil, err
	}

	return &ExtendedCerts{
		RawClientCert: clientCertRaw,
		RawServerCert: serverCertRaw,
	}, nil
}

func (self *EdgeRouterManager) ExtendEnrollmentWithVerify(router *EdgeRouter, clientCsrPem []byte, serverCertCsrPem []byte, ctx *change.Context) (*ExtendedCerts, error) {
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
	clientPem, _ := cert.RawToPem(clientCertRaw)
	clientPemString := string(clientPem)

	pfxlog.Logger().Debugf("extending enrollment for edge router %s, old fingerprint: %s new fingerprint: %s", router.Id, *router.Fingerprint, fingerprint)

	router.UnverifiedFingerprint = &fingerprint
	router.UnverifiedCertPem = &clientPemString

	err = self.Update(router, true, &fields.UpdatedFieldsMap{
		db.FieldEdgeRouterUnverifiedCertPEM:     struct{}{},
		db.FieldEdgeRouterUnverifiedFingerprint: struct{}{},
	}, ctx)

	if err != nil {
		return nil, err
	}

	return &ExtendedCerts{
		RawClientCert: clientCertRaw,
		RawServerCert: serverCertRaw,
	}, nil
}

func (self *EdgeRouterManager) ReadOneByUnverifiedFingerprint(fingerprint string) (*EdgeRouter, error) {
	return self.ReadOneByQuery(fmt.Sprintf(`%s = "%v"`, db.FieldEdgeRouterUnverifiedFingerprint, fingerprint))
}

func (self *EdgeRouterManager) ExtendEnrollmentVerify(router *EdgeRouter, ctx *change.Context) error {
	if router.UnverifiedFingerprint != nil && router.UnverifiedCertPem != nil {
		router.Fingerprint = router.UnverifiedFingerprint
		router.CertPem = router.UnverifiedCertPem

		router.UnverifiedFingerprint = nil
		router.UnverifiedCertPem = nil

		return self.Update(router, true, fields.UpdatedFieldsMap{
			db.FieldRouterFingerprint:               struct{}{},
			db.FieldCaCertPem:                       struct{}{},
			db.FieldEdgeRouterUnverifiedCertPEM:     struct{}{},
			db.FieldEdgeRouterUnverifiedFingerprint: struct{}{},
		}, ctx)
	}

	return errors.New("no outstanding verification necessary")
}

func (self *EdgeRouterManager) EdgeRouterToProtobuf(entity *EdgeRouter) (*edge_cmd_pb.EdgeRouter, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	appData, err := json.Marshal(entity.AppData)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.EdgeRouter{
		Id:                    entity.Id,
		Name:                  entity.Name,
		Tags:                  tags,
		RoleAttributes:        entity.RoleAttributes,
		IsVerified:            entity.IsVerified,
		Fingerprint:           entity.Fingerprint,
		CertPem:               entity.CertPem,
		Hostname:              entity.Hostname,
		IsTunnelerEnabled:     entity.IsTunnelerEnabled,
		AppData:               appData,
		UnverifiedFingerprint: entity.UnverifiedFingerprint,
		UnverifiedCertPem:     entity.UnverifiedCertPem,
		Cost:                  uint32(entity.Cost),
		NoTraversal:           entity.NoTraversal,
		Disabled:              entity.Disabled,
	}

	return msg, nil
}

func (self *EdgeRouterManager) Marshall(entity *EdgeRouter) ([]byte, error) {
	msg, err := self.EdgeRouterToProtobuf(entity)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

func (self *EdgeRouterManager) ProtobufToEdgeRouter(msg *edge_cmd_pb.EdgeRouter) (*EdgeRouter, error) {
	appData := map[string]interface{}{}
	if err := json.Unmarshal(msg.AppData, &appData); err != nil {
		return nil, err
	}

	return &EdgeRouter{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:                  msg.Name,
		RoleAttributes:        msg.RoleAttributes,
		IsVerified:            msg.IsVerified,
		Fingerprint:           msg.Fingerprint,
		CertPem:               msg.CertPem,
		Hostname:              msg.Hostname,
		IsTunnelerEnabled:     msg.IsTunnelerEnabled,
		AppData:               appData,
		UnverifiedFingerprint: msg.UnverifiedFingerprint,
		UnverifiedCertPem:     msg.UnverifiedCertPem,
		Cost:                  uint16(msg.Cost),
		NoTraversal:           msg.NoTraversal,
		Disabled:              msg.Disabled,
	}, nil
}

func (self *EdgeRouterManager) Unmarshall(bytes []byte) (*EdgeRouter, error) {
	msg := &edge_cmd_pb.EdgeRouter{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}
	return self.ProtobufToEdgeRouter(msg)
}

type EdgeRouterListResult struct {
	manager     *EdgeRouterManager
	EdgeRouters []*EdgeRouter
	models.QueryMetaData
}

func (result *EdgeRouterListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.manager.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.EdgeRouters = append(result.EdgeRouters, entity)
	}
	return nil
}

type CreateEdgeRouterCmd struct {
	manager    *EdgeRouterManager
	edgeRouter *EdgeRouter
	enrollment *Enrollment
	ctx        *change.Context
}

func (self *CreateEdgeRouterCmd) Apply(ctx boltz.MutateContext) error {
	return self.manager.ApplyCreate(self, ctx)
}

func (self *CreateEdgeRouterCmd) Encode() ([]byte, error) {
	edgeRouterMsg, err := self.manager.EdgeRouterToProtobuf(self.edgeRouter)
	if err != nil {
		return nil, err
	}

	enrollment, err := self.manager.GetEnv().GetManagers().Enrollment.EnrollmentToProtobuf(self.enrollment)
	if err != nil {
		return nil, err
	}

	cmd := &edge_cmd_pb.CreateEdgeRouterCmd{
		EdgeRouter: edgeRouterMsg,
		Enrollment: enrollment,
		Ctx:        ContextToProtobuf(self.ctx),
	}

	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *CreateEdgeRouterCmd) Decode(env Env, msg *edge_cmd_pb.CreateEdgeRouterCmd) error {
	self.manager = env.GetManagers().EdgeRouter
	edgeRouter, err := self.manager.ProtobufToEdgeRouter(msg.EdgeRouter)
	if err != nil {
		return err
	}

	enrollment, err := env.GetManagers().Enrollment.ProtobufToEnrollment(msg.Enrollment)
	if err != nil {
		return err
	}

	self.edgeRouter = edgeRouter
	self.enrollment = enrollment
	self.ctx = ProtobufToContext(msg.Ctx)

	return nil
}

func (self *CreateEdgeRouterCmd) GetChangeContext() *change.Context {
	return self.ctx
}
