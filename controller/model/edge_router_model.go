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
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-sdk-golang/ziti/config"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type EdgeRouter struct {
	BaseModelEntityImpl
	Name                string
	RoleAttributes      []string
	IsVerified          bool
	Fingerprint         *string
	CertPem             *string
	EnrollmentToken     *string
	Hostname            *string
	EnrollmentJwt       *string
	EnrollmentCreatedAt *time.Time
	EnrollmentExpiresAt *time.Time
	EdgeRouterProtocols map[string]string
}

func (entity *EdgeRouter) ToBoltEntityForCreate(_ *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	et := uuid.New().String()

	boltEntity := &persistence.EdgeRouter{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		RoleAttributes:     entity.RoleAttributes,
		Fingerprint:        nil,
		IsVerified:         false,
		EnrollmentToken:    &et,
	}
	env := handler.GetEnv()

	now := time.Now()
	exp := now.Add(env.GetConfig().Enrollment.EdgeRouter.DurationMinutes)

	boltEntity.EnrollmentCreatedAt = &now
	boltEntity.EnrollmentExpiresAt = &exp

	enrollmentMethod := "erott"

	claims := &config.EnrollmentClaims{
		EnrollmentMethod: enrollmentMethod,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: exp.Unix(),
			Id:        *boltEntity.EnrollmentToken,
			Issuer:    fmt.Sprintf(`https://%s`, env.GetConfig().Api.Advertise),
			Subject:   boltEntity.Id,
		},
	}
	mapClaims, err := claims.ToMapClaims()

	if err != nil {
		return nil, fmt.Errorf("could not convert edge router enrollment claims to interface map: %s", err)
	}

	jwtJson, err := env.GetEnrollmentJwtGenerator().Generate(boltEntity.Id, boltEntity.Id, mapClaims)

	if err != nil {
		return nil, err
	}

	boltEntity.EnrollmentJwt = &jwtJson

	return boltEntity, nil
}

func (entity *EdgeRouter) ToBoltEntityForUpdate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.EdgeRouter{
		BaseEdgeEntityImpl:  *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:                entity.Name,
		RoleAttributes:      entity.RoleAttributes,
		IsVerified:          entity.IsVerified,
		Fingerprint:         entity.Fingerprint,
		CertPem:             entity.CertPem,
		EnrollmentToken:     entity.EnrollmentToken,
		Hostname:            entity.Hostname,
		EnrollmentJwt:       entity.EnrollmentJwt,
		EnrollmentCreatedAt: entity.EnrollmentCreatedAt,
		EnrollmentExpiresAt: entity.EnrollmentExpiresAt,
		EdgeRouterProtocols: entity.EdgeRouterProtocols,
	}, nil
}

func (entity *EdgeRouter) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForUpdate(tx, handler)
}

func (entity *EdgeRouter) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltEdgeRouter, ok := boltEntity.(*persistence.EdgeRouter)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model edge router", reflect.TypeOf(boltEntity))
	}

	entity.fillCommon(boltEdgeRouter)
	entity.Name = boltEdgeRouter.Name
	entity.RoleAttributes = boltEdgeRouter.RoleAttributes
	entity.IsVerified = boltEdgeRouter.IsVerified
	entity.Fingerprint = boltEdgeRouter.Fingerprint
	entity.CertPem = boltEdgeRouter.CertPem
	entity.EnrollmentToken = boltEdgeRouter.EnrollmentToken
	entity.Hostname = boltEdgeRouter.Hostname
	entity.EnrollmentJwt = boltEdgeRouter.EnrollmentJwt
	entity.EnrollmentCreatedAt = boltEdgeRouter.EnrollmentCreatedAt
	entity.EnrollmentExpiresAt = boltEdgeRouter.EnrollmentExpiresAt
	entity.EdgeRouterProtocols = boltEdgeRouter.EdgeRouterProtocols

	return nil
}
