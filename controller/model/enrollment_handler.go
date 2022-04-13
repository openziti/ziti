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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"time"
)

type EnrollmentHandler struct {
	baseHandler
	enrollmentStore persistence.EnrollmentStore
}

func NewEnrollmentHandler(env Env) *EnrollmentHandler {
	handler := &EnrollmentHandler{
		baseHandler:     newBaseHandler(env, env.GetStores().Enrollment),
		enrollmentStore: env.GetStores().Enrollment,
	}

	handler.impl = handler
	return handler
}

func (handler *EnrollmentHandler) newModelEntity() boltEntitySink {
	return &Enrollment{}
}

func (handler *EnrollmentHandler) getEnrollmentMethod(ctx EnrollmentContext) (string, error) {
	method := ctx.GetMethod()

	if method == persistence.MethodEnrollCa {
		return method, nil
	}

	token := ctx.GetToken()

	// token present, assumes all other enrollment methods
	enrollment, err := handler.ReadByToken(token)

	if err != nil {
		return "", err
	}

	if enrollment == nil {
		return "", apierror.NewInvalidEnrollmentToken()
	}

	method = enrollment.Method

	return method, nil
}

func (handler *EnrollmentHandler) Enroll(ctx EnrollmentContext) (*EnrollmentResult, error) {
	method, err := handler.getEnrollmentMethod(ctx)

	if err != nil {
		return nil, err
	}

	enrollModule := handler.env.GetEnrollRegistry().GetByMethod(method)

	if enrollModule == nil {
		return nil, apierror.NewInvalidEnrollMethod()
	}

	return enrollModule.Process(ctx)
}

func (handler *EnrollmentHandler) ReadByToken(token string) (*Enrollment, error) {
	enrollment := &Enrollment{}

	err := handler.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		boltEntity, err := handler.env.GetStores().Enrollment.LoadOneByToken(tx, token)

		if err != nil {
			return err
		}

		if boltEntity == nil {
			enrollment = nil
			return nil
		}

		return enrollment.fillFrom(handler, tx, boltEntity)
	})

	if err != nil {
		return nil, err
	}

	return enrollment, nil
}

func (handler *EnrollmentHandler) ReplaceWithAuthenticator(enrollmentId string, authenticator *Authenticator) error {
	return handler.env.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)

		err := handler.env.GetStores().Enrollment.DeleteById(ctx, enrollmentId)
		if err != nil {
			return err
		}

		_, err = handler.env.GetHandlers().Authenticator.createEntityInTx(ctx, authenticator)
		return err
	})
}

func (handler *EnrollmentHandler) readInTx(tx *bbolt.Tx, id string) (*Enrollment, error) {
	modelEntity := &Enrollment{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EnrollmentHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *EnrollmentHandler) Read(id string) (*Enrollment, error) {
	entity := &Enrollment{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *EnrollmentHandler) RefreshJwt(id string, expiresAt time.Time) error {
	enrollment, err := handler.Read(id)

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

	if err := enrollment.FillJwtInfoWithExpiresAt(handler.env, *enrollment.IdentityId, expiresAt); err != nil {
		return err
	}

	err = handler.patchEntity(enrollment, boltz.MapFieldChecker{
		persistence.FieldEnrollmentJwt:       struct{}{},
		persistence.FieldEnrollmentExpiresAt: struct{}{},
		persistence.FieldEnrollmentIssuedAt:  struct{}{},
	})

	return err
}
