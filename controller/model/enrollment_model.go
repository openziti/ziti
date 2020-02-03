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
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-sdk-golang/ziti/config"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type Enrollment struct {
	BaseModelEntityImpl
	Method     string
	IdentityId string
	Token      string
	IssuedAt   *time.Time
	ExpiresAt  *time.Time
	Jwt        string
	CaId       *string
	Username   *string
}

func (entity *Enrollment) FillJwtInfo(env Env) error {
	now := time.Now()
	expiresAt := now.Add(env.GetConfig().Enrollment.EdgeIdentity.DurationMinutes)
	entity.IssuedAt = &now
	entity.ExpiresAt = &expiresAt

	if entity.Token == "" {
		entity.Token = uuid.New().String()
	}

	enrollmentClaims := config.EnrollmentClaims{
		EnrollmentMethod: entity.Method,
		StandardClaims: jwt.StandardClaims{
			Audience:  "",
			ExpiresAt: expiresAt.Unix(),
			Id:        entity.Token,
			Issuer:    fmt.Sprintf("https://%s", env.GetConfig().Api.Advertise),
			NotBefore: 0,
			Subject:   entity.IdentityId,
		},
	}

	mapClaims, err := enrollmentClaims.ToMapClaims()

	if err != nil {
		return err
	}

	signedJwt, err := env.GetEnrollmentJwtGenerator().Generate(entity.IdentityId, entity.Id, mapClaims)

	if err != nil {
		return err
	}

	entity.Jwt = signedJwt

	return nil
}

func (entity *Enrollment) fillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltEnrollment, ok := boltEntity.(*persistence.Enrollment)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model authenticator", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltEnrollment)
	entity.Method = boltEnrollment.Method
	entity.IdentityId = boltEnrollment.IdentityId
	entity.CaId = boltEnrollment.CaId
	entity.Username = boltEnrollment.Username
	entity.Token = boltEnrollment.Token
	entity.IssuedAt = boltEnrollment.IssuedAt
	entity.ExpiresAt = boltEnrollment.ExpiresAt
	entity.Jwt = boltEnrollment.Jwt

	return nil
}

func (entity *Enrollment) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	if entity.Method == persistence.MethodEnrollOttCa {
		if entity.CaId == nil || *entity.CaId == "" {
			apiErr := apierror.NewNotFound()
			apiErr.Cause = apierror.NewFieldError("ca not found", "caId", *entity.CaId)
			apiErr.AppendCause = true
			return nil, apiErr
		}

		ca, _ := handler.GetEnv().GetHandlers().Ca.Read(*entity.CaId)

		if ca == nil {
			apiErr := apierror.NewNotFound()
			apiErr.Cause = apierror.NewFieldError("ca not found", "caId", *entity.CaId)
			apiErr.AppendCause = true
			return nil, apiErr
		}
	}

	boltEntity := &persistence.Enrollment{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Method:             entity.Method,
		IdentityId:         entity.IdentityId,
		Token:              entity.Token,
		IssuedAt:           entity.IssuedAt,
		ExpiresAt:          entity.ExpiresAt,
		Jwt:                entity.Jwt,
		CaId:               entity.CaId,
		Username:           entity.Username,
	}

	return boltEntity, nil
}

func (entity *Enrollment) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.toBoltEntityForCreate(tx, handler)

}

func (entity *Enrollment) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}
