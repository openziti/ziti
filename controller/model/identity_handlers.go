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
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-edge/crypto"
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

func (handler *IdentityHandler) Create(identityModel *Identity) (string, error) {
	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identityModel.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = validation.NewFieldError("typeId not found", "typeId", identityModel.IdentityTypeId)
		apiErr.AppendCause = true
		return "", apiErr
	}

	identityModel.IdentityTypeId = identityType.Id

	return handler.createEntity(identityModel)
}

func (handler *IdentityHandler) CreateWithEnrollments(identityModel *Identity, enrollmentsModels []*Enrollment) (string, []string, error) {
	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identityModel.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", nil, err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = validation.NewFieldError("identityTypeId not found", "identityTypeId", identityModel.IdentityTypeId)
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

			enrollmentId, err := handler.env.GetHandlers().Enrollment.createEntityInTx(ctx, enrollmentModel)

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

func (handler *IdentityHandler) Update(identity *Identity) error {
	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = validation.NewFieldError("identityTypeId not found", "identityTypeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return apiErr
	}

	identity.IdentityTypeId = identityType.Id

	return handler.updateEntity(identity, handler)
}

func (handler *IdentityHandler) Patch(identity *Identity, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	if checker.IsUpdated("type") {
		identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)
		if err != nil && !util.IsErrNotFoundErr(err) {
			return err
		}

		if identityType == nil {
			apiErr := apierror.NewNotFound()
			apiErr.Cause = validation.NewFieldError("identityTypeId not found", "identityTypeId", identity.IdentityTypeId)
			apiErr.AppendCause = true
			return apiErr
		}

		identity.IdentityTypeId = identityType.Id
	}

	return handler.patchEntity(identity, combinedChecker)
}

func (handler *IdentityHandler) Delete(id string) error {
	identity, err := handler.Read(id)

	if err != nil {
		return nil
	}

	if identity.IsDefaultAdmin {
		return apierror.NewEntityCanNotBeDeleted()
	}

	return handler.deleteEntity(id, nil)
}

func (handler IdentityHandler) IsUpdated(field string) bool {
	return field != "Authenticators" && field != "Enrollments" && field != "IsDefaultAdmin"
}

func (handler *IdentityHandler) Read(id string) (*Identity, error) {
	entity := &Identity{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *IdentityHandler) readInTx(tx *bbolt.Tx, id string) (*Identity, error) {
	identity := &Identity{}
	if err := handler.readEntityInTx(tx, id, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (handler *IdentityHandler) ReadDefaultAdmin() (*Identity, error) {
	return handler.ReadOneByQuery("isDefaultAdmin = true")
}

func (handler *IdentityHandler) ReadOneByQuery(query string) (*Identity, error) {
	result, err := handler.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Identity), nil
}

func (handler *IdentityHandler) InitializeDefaultAdmin(username, password, name string) error {
	identity, err := handler.ReadDefaultAdmin()

	if err != nil && !util.IsErrNotFoundErr(err) {
		pfxlog.Logger().Panic(err)
	}

	if identity != nil {
		return errors.New("already initialized: Ziti Edge default admin already defined")
	}

	identityType, err := handler.env.GetHandlers().IdentityType.ReadByName(IdentityTypeUser)

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

	if _, err := handler.Create(defaultAdmin); err != nil {
		return err
	}

	if _, err := handler.env.GetHandlers().Authenticator.Create(authenticator); err != nil {
		return err
	}

	return nil
}

func (handler *IdentityHandler) CollectAuthenticators(id string, collector func(entity *Authenticator) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := handler.readInTx(tx, id)
		if err != nil {
			return err
		}
		association := handler.store.GetLinkCollection(persistence.FieldIdentityAuthenticators)
		for _, authenticatorId := range association.GetLinks(tx, id) {
			authenticator := &Authenticator{}
			err := handler.env.GetHandlers().Authenticator.readEntity(authenticatorId, authenticator)
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

func (handler *IdentityHandler) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (handler *IdentityHandler) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := handler.readInTx(tx, id)
	if err != nil {
		return err
	}

	association := handler.store.GetLinkCollection(persistence.FieldIdentityEnrollments)
	for _, enrollmentId := range association.GetLinks(tx, id) {
		enrollment, err := handler.env.GetHandlers().Enrollment.readInTx(tx, enrollmentId)
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

func (handler *IdentityHandler) CreateWithAuthenticator(identity *Identity, authenticator *Authenticator) (string, string, error) {
	if identity.Id == "" {
		identity.Id = uuid.New().String()
	}

	if authenticator.Id == "" {
		authenticator.Id = uuid.New().String()
	}

	if authenticator.IdentityId == "" {
		authenticator.IdentityId = identity.Id
	}

	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", "", err
	}

	if identityType == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = validation.NewFieldError("typeId not found", "typeId", identity.IdentityTypeId)
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

func (handler *IdentityHandler) CollectEdgeRouterPolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeEdgeRouterPolicies, handler.env.GetHandlers().EdgeRouterPolicy, collector)
}

func (handler *IdentityHandler) CollectServicePolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeServicePolicies, handler.env.GetHandlers().ServicePolicy, collector)
}
