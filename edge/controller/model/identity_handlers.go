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
	"encoding/base64"
	"errors"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/edge/controller/util"
	"github.com/netfoundry/ziti-edge/edge/crypto"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

type IdentityHandler struct {
	baseHandler
	allowedFieldsChecker boltz.FieldChecker
}

func NewIdentityHandler(env Env) *IdentityHandler {
	handler := &IdentityHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().Identity,
		},
		allowedFieldsChecker: boltz.MapFieldChecker{
			persistence.FieldName:                   struct{}{},
			persistence.FieldIdentityIsDefaultAdmin: struct{}{},
			persistence.FieldIdentityIsAdmin:        struct{}{},
			persistence.FieldIdentityType:           struct{}{},
			persistence.FieldTags:                   struct{}{},
		},
	}
	handler.impl = handler
	return handler
}

func (handler IdentityHandler) NewModelEntity() BaseModelEntity {
	return &Identity{}
}

func (handler *IdentityHandler) HandleCreate(identityModel *Identity) (string, error) {
	identityType, err := handler.env.GetHandlers().IdentityType.HandleReadByIdOrName(identityModel.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = NewFieldError("typeId not found", "typeId", identityModel.IdentityTypeId)
		apiErr.AppendCause = true
		return "", apiErr
	}

	identityModel.IdentityTypeId = identityType.Id

	return handler.create(identityModel, nil)
}

func (handler *IdentityHandler) HandleCreateWithEnrollments(identityModel *Identity, enrollmentsModels []*Enrollment) (string, []string, error) {
	identityType, err := handler.env.GetHandlers().IdentityType.HandleReadByIdOrName(identityModel.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", nil, err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = NewFieldError("identityTypeId not found", "identityTypeId", identityModel.IdentityTypeId)
		apiErr.AppendCause = true
		return "", nil, apiErr
	}

	identityModel.IdentityTypeId = identityType.Id

	if identityModel.Id == "" {
		identityModel.Id = uuid.New().String()
	}
	var enrollmentIds []string

	err = handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEntity, err := identityModel.ToBoltEntityForCreate(tx, handler.impl)
		if err != nil {
			return err
		}
		if err := handler.GetStore().Create(ctx, boltEntity); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", handler.store.GetEntityType())
			return err
		}

		for _, enrollmentModel := range enrollmentsModels {
			enrollmentModel.IdentityId = identityModel.Id

			err := enrollmentModel.FillJwtInfo(handler.env)

			if err != nil {
				return err
			}

			enrollmentId, err := handler.env.GetHandlers().Enrollment.createInTx(ctx, enrollmentModel, nil)

			if err != nil {
				return err
			}

			enrollmentIds = append(enrollmentIds, enrollmentId)
		}
		return nil
	})

	if err != nil {
		return "", nil, err
	}

	return identityModel.Id, enrollmentIds, nil
}

func (handler *IdentityHandler) HandleUpdate(identity *Identity) error {
	identityType, err := handler.env.GetHandlers().IdentityType.HandleReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = NewFieldError("identityTypeId not found", "identityTypeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return apiErr
	}

	identity.IdentityTypeId = identityType.Id

	return handler.update(identity, handler, nil)
}

func (handler *IdentityHandler) HandlePatch(identity *Identity, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	identityType, err := handler.env.GetHandlers().IdentityType.HandleReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = NewFieldError("identityTypeId not found", "identityTypeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return apiErr
	}

	identity.IdentityTypeId = identityType.Id

	return handler.patch(identity, combinedChecker, nil)
}

func (handler *IdentityHandler) HandleDelete(id string) error {
	identity, err := handler.HandleRead(id)

	if err != nil {
		return nil
	}

	if identity.IsDefaultAdmin == true {
		return apierror.NewEntityCanNotBeDeleted()
	}

	return handler.delete(id, nil, nil)
}

func (handler IdentityHandler) IsUpdated(field string) bool {
	return field != "Authenticators" && field != "Enrollments" && field != "IsDefaultAdmin"
}

func (handler *IdentityHandler) HandleRead(id string) (*Identity, error) {
	entity := &Identity{}
	if err := handler.read(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *IdentityHandler) handleReadInTx(tx *bbolt.Tx, id string) (*Identity, error) {
	identity := &Identity{}
	if err := handler.readInTx(tx, id, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (handler *IdentityHandler) HandleReadDefaultAdmin() (*Identity, error) {
	return handler.HandleReadOneByQuery("isDefaultAdmin = true")
}

func (handler *IdentityHandler) HandleReadOneByQuery(query string) (*Identity, error) {
	result, err := handler.readByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Identity), nil
}

func (handler *IdentityHandler) HandleInitializeDefaultAdmin(username, password, name string) error {
	identity, err := handler.HandleReadDefaultAdmin()

	if err != nil && !util.IsErrNotFoundErr(err) {
		pfxlog.Logger().Panic(err)
	}

	if identity != nil {
		return errors.New("already initialized: Ziti Edge default admin already defined")
	}

	identityType, err := handler.env.GetHandlers().IdentityType.HandleReadByName(IdentityTypeUser)

	if err != nil {
		return err
	}

	identityId := uuid.New().String()
	authenticatorId := uuid.New().String()

	defaultAdmin := &Identity{
		BaseModelEntityImpl: BaseModelEntityImpl{
			Id: identityId,
		},
		Name:           name,
		IdentityTypeId: identityType.Id,
		IsDefaultAdmin: true,
		IsAdmin:        true,
	}

	newResult := crypto.Hash(password)
	b64Password := base64.StdEncoding.EncodeToString(newResult.Hash)
	b64Salt := base64.StdEncoding.EncodeToString(newResult.Salt)

	authenticator := &Authenticator{
		BaseModelEntityImpl: BaseModelEntityImpl{
			Id: authenticatorId,
		},
		Method:     persistence.MethodAuthenticatorUpdb,
		IdentityId: identityId,
		SubType: &AuthenticatorUpdb{
			Username: username,
			Password: b64Password,
			Salt:     b64Salt,
		},
	}

	if _, err := handler.HandleCreate(defaultAdmin); err != nil {
		return err
	}

	if _, err := handler.env.GetHandlers().Authenticator.HandleCreate(authenticator); err != nil {
		return err
	}

	return nil
}

func (handler *IdentityHandler) HandleCollectAuthenticators(id string, collector func(entity BaseModelEntity) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := handler.handleReadInTx(tx, id)
		if err != nil {
			return err
		}
		association := handler.store.GetLinkCollection(persistence.FieldIdentityAuthenticators)
		for _, authenticatorId := range association.GetLinks(tx, id) {
			authenticator := &Authenticator{}
			err := handler.env.GetHandlers().Authenticator.read(authenticatorId, authenticator)
			if err != nil {
				return err
			}
			if err = collector(authenticator); err != nil {
				return err
			}
		}
		return nil
	})
}

func (handler *IdentityHandler) HandleCollectEnrollments(id string, collector func(entity BaseModelEntity) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.handleCollectEnrollmentsInTx(tx, id, collector)
	})
}

func (handler *IdentityHandler) handleCollectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity BaseModelEntity) error) error {
	_, err := handler.handleReadInTx(tx, id)
	if err != nil {
		return err
	}

	association := handler.store.GetLinkCollection(persistence.FieldIdentityEnrollments)
	for _, enrollmentId := range association.GetLinks(tx, id) {
		enrollment, err := handler.env.GetHandlers().Enrollment.handleReadInTx(tx, enrollmentId)
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

func (handler *IdentityHandler) HandleCreateWithAuthenticator(identity *Identity, authenticator *Authenticator) (string, string, error) {
	if identity.Id == "" {
		identity.Id = uuid.New().String()
	}

	if authenticator.Id == "" {
		authenticator.Id = uuid.New().String()
	}

	if authenticator.IdentityId == "" {
		authenticator.IdentityId = identity.Id
	}

	identityType, err := handler.env.GetHandlers().IdentityType.HandleReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", "", err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = NewFieldError("typeId not found", "typeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return "", "", apiErr
	}

	err = handler.env.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltIdentity, err := identity.ToBoltEntityForCreate(tx, handler)

		if err != nil {
			return err
		}

		if err = handler.env.GetStores().Identity.Create(ctx, boltIdentity); err != nil {
			return err
		}

		boltAuthenticator, err := authenticator.ToBoltEntityForCreate(tx, handler.env.GetHandlers().Authenticator)

		if err != nil {
			return err
		}

		if err = handler.env.GetStores().Authenticator.Create(ctx, boltAuthenticator); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	return identity.Id, authenticator.Id, nil
}

func (handler *IdentityHandler) HandleCollectEdgeRouterPolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeEdgeRouterPolicies, handler.env.GetHandlers().EdgeRouterPolicy, collector)
}

func (handler *IdentityHandler) HandleCollectServicePolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeServicePolicies, handler.env.GetHandlers().ServicePolicy, collector)
}
