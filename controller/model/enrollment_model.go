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
	"fmt"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type Enrollment struct {
	models.BaseEntity
	Method          string
	IdentityId      *string
	TransitRouterId *string
	EdgeRouterId    *string
	Token           string
	IssuedAt        *time.Time
	ExpiresAt       *time.Time
	Jwt             string
	CaId            *string
	Username        *string
}

func (entity *Enrollment) FillJwtInfo(env Env, subject string) error {
	now := time.Now()
	expiresAt := now.Add(env.GetConfig().Enrollment.EdgeIdentity.Duration)
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
			Issuer:    fmt.Sprintf("https://%s", env.GetConfig().Api.Address),
			NotBefore: 0,
			Subject:   subject,
		},
	}

	mapClaims, err := enrollmentClaims.ToMapClaims()

	if err != nil {
		return err
	}

	signedJwt, err := env.GetJwtSigner().Generate(subject, entity.Id, mapClaims)

	if err != nil {
		return err
	}

	entity.Jwt = signedJwt

	return nil
}

func (entity *Enrollment) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltEnrollment, ok := boltEntity.(*persistence.Enrollment)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model authenticator", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltEnrollment)
	entity.Method = boltEnrollment.Method
	entity.IdentityId = boltEnrollment.IdentityId
	entity.TransitRouterId = boltEnrollment.TransitRouterId
	entity.EdgeRouterId = boltEnrollment.EdgeRouterId
	entity.CaId = boltEnrollment.CaId
	entity.Username = boltEnrollment.Username
	entity.Token = boltEnrollment.Token
	entity.IssuedAt = boltEnrollment.IssuedAt
	entity.ExpiresAt = boltEnrollment.ExpiresAt
	entity.Jwt = boltEnrollment.Jwt

	return nil
}

func (entity *Enrollment) toBoltEntity(handler Handler) (boltz.Entity, error) {
	if entity.Method == persistence.MethodEnrollOttCa {
		if entity.CaId == nil || *entity.CaId == "" {
			apiErr := errorz.NewNotFound()
			apiErr.Cause = errorz.NewFieldError("ca not found", "caId", *entity.CaId)
			apiErr.AppendCause = true
			return nil, apiErr
		}

		ca, _ := handler.GetEnv().GetHandlers().Ca.Read(*entity.CaId)

		if ca == nil {
			apiErr := errorz.NewNotFound()
			apiErr.Cause = errorz.NewFieldError("ca not found", "caId", *entity.CaId)
			apiErr.AppendCause = true
			return nil, apiErr
		}
	}

	boltEntity := &persistence.Enrollment{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Method:          entity.Method,
		IdentityId:      entity.IdentityId,
		EdgeRouterId:    entity.EdgeRouterId,
		TransitRouterId: entity.TransitRouterId,
		Token:           entity.Token,
		IssuedAt:        entity.IssuedAt,
		ExpiresAt:       entity.ExpiresAt,
		Jwt:             entity.Jwt,
		CaId:            entity.CaId,
		Username:        entity.Username,
	}

	return boltEntity, nil
}

func (entity *Enrollment) toBoltEntityForCreate(_ *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	return entity.toBoltEntity(handler)
}

func (entity *Enrollment) toBoltEntityForUpdate(_ *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	return entity.toBoltEntity(handler)

}

func (entity *Enrollment) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity(handler)
}
