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
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
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

// FillJwtInfoForRouter populates the JWT information for router enrollment.
// It sets the expiration time based on the EdgeRouter enrollment duration configuration
// and delegates to FillJwtInfoWithExpiresAt for actual JWT generation.
func (entity *Enrollment) FillJwtInfoForRouter(env Env, subject string) error {
	expiresAt := time.Now().Add(env.GetConfig().Edge.Enrollment.EdgeRouter.Duration).UTC()
	return entity.FillJwtInfoWithExpiresAt(env, subject, expiresAt)
}

// FillJwtInfoForIdentity populates the JWT information for identity enrollment.
// It sets the expiration time based on the EdgeIdentity enrollment duration configuration
// and delegates to FillJwtInfoWithExpiresAt for actual JWT generation.
func (entity *Enrollment) FillJwtInfoForIdentity(env Env, subject string) error {
	expiresAt := time.Now().Add(env.GetConfig().Edge.Enrollment.EdgeIdentity.Duration).UTC()
	return entity.FillJwtInfoWithExpiresAt(env, subject, expiresAt)
}

// FillJwtInfoWithExpiresAt generates and populates JWT enrollment information with a custom expiration time.
// It creates enrollment JWT claims containing controller addresses, sets issued/expires timestamps,
// generates a unique token if not already set, and signs the JWT using the environment's enrollment signer.
// The generated JWT contains enrollment method, controller endpoints, and standard JWT claims.
func (entity *Enrollment) FillJwtInfoWithExpiresAt(env Env, subject string, expiresAt time.Time) error {
	now := time.Now().UTC()
	expiresAt = expiresAt.UTC()

	entity.IssuedAt = &now
	entity.ExpiresAt = &expiresAt

	if entity.Token == "" {
		entity.Token = uuid.New().String()
	}

	ctrls, err := env.GetManagers().Controller.BaseList("true limit none")
	if err != nil {
		return fmt.Errorf("could not get controllers to populate JWT %w", err)
	}

	var controllers []string
	for _, ctrl := range ctrls.Entities {
		controllers = append(controllers, ctrl.CtrlAddress)
	}

	if len(controllers) == 0 {
		if options := env.GetConfig().Ctrl.Options; options != nil {
			if advertiseAddr := env.GetConfig().Ctrl.Options.AdvertiseAddress; advertiseAddr != nil {
				controllers = append(controllers, (*advertiseAddr).String())
			}
		}
	}

	enrollmentClaims := &ziti.EnrollmentClaims{
		EnrollmentMethod: entity.Method,
		Controllers:      controllers,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  []string{""},
			ExpiresAt: &jwt.NumericDate{Time: expiresAt},
			ID:        entity.Token,
			Issuer:    fmt.Sprintf("https://%s", env.GetConfig().Edge.Api.Address),
			Subject:   subject,
		},
	}
	signer, err := env.GetEnrollmentJwtSigner()

	if err != nil {
		return fmt.Errorf("could not get enrollment signer: %v", err)
	}

	signedJwt, err := signer.Generate(enrollmentClaims)

	if err != nil {
		return err
	}

	entity.Jwt = signedJwt

	return nil
}

func (entity *Enrollment) fillFrom(_ Env, _ *bbolt.Tx, boltEnrollment *db.Enrollment) error {
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

func (entity *Enrollment) toBoltEntity(env Env) (*db.Enrollment, error) {
	if entity.Method == db.MethodEnrollOttCa {
		if entity.CaId == nil || *entity.CaId == "" {
			apiErr := errorz.NewNotFound()
			apiErr.Cause = errorz.NewFieldError("ca not found", "caId", *entity.CaId)
			apiErr.AppendCause = true
			return nil, apiErr
		}

		ca, _ := env.GetManagers().Ca.Read(*entity.CaId)

		if ca == nil {
			apiErr := errorz.NewNotFound()
			apiErr.Cause = errorz.NewFieldError("ca not found", "caId", *entity.CaId)
			apiErr.AppendCause = true
			return nil, apiErr
		}
	}

	boltEntity := &db.Enrollment{
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

func (entity *Enrollment) toBoltEntityForCreate(_ *bbolt.Tx, env Env) (*db.Enrollment, error) {
	return entity.toBoltEntity(env)
}

func (entity *Enrollment) toBoltEntityForUpdate(_ *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Enrollment, error) {
	return entity.toBoltEntity(env)
}
