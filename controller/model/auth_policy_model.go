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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type AuthPolicy struct {
	models.BaseEntity
	Name      string
	Primary   AuthPolicyPrimary
	Secondary AuthPolicySecondary
}

type AuthPolicyPrimary struct {
	Cert   AuthPolicyCert
	Updb   AuthPolicyUpdb
	ExtJwt AuthPolicyExtJwt
}

type AuthPolicySecondary struct {
	RequireTotp          bool
	RequiredExtJwtSigner *string
}

type AuthPolicyCert struct {
	Allowed           bool
	AllowExpiredCerts bool
}

type AuthPolicyExtJwt struct {
	Allowed              bool
	AllowedExtJwtSigners []string
}

type AuthPolicyUpdb struct {
	Allowed                bool
	MinPasswordLength      int64
	RequireSpecialChar     bool
	RequireNumberChar      bool
	RequireMixedCase       bool
	MaxAttempts            int64
	LockoutDurationMinutes int64
}

func (entity *AuthPolicy) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltAuthPolicy, ok := boltEntity.(*persistence.AuthPolicy)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ca", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltAuthPolicy)
	entity.Name = boltAuthPolicy.Name
	entity.Primary = AuthPolicyPrimary{
		Cert: AuthPolicyCert{
			Allowed:           boltAuthPolicy.Primary.Cert.Allowed,
			AllowExpiredCerts: boltAuthPolicy.Primary.Cert.AllowExpiredCerts,
		},
		Updb: AuthPolicyUpdb{
			Allowed:                boltAuthPolicy.Primary.Updb.Allowed,
			MinPasswordLength:      boltAuthPolicy.Primary.Updb.MinPasswordLength,
			RequireSpecialChar:     boltAuthPolicy.Primary.Updb.RequireSpecialChar,
			RequireNumberChar:      boltAuthPolicy.Primary.Updb.RequireNumberChar,
			RequireMixedCase:       boltAuthPolicy.Primary.Updb.RequireMixedCase,
			MaxAttempts:            boltAuthPolicy.Primary.Updb.MaxAttempts,
			LockoutDurationMinutes: boltAuthPolicy.Primary.Updb.LockoutDurationMinutes,
		},
		ExtJwt: AuthPolicyExtJwt{
			Allowed:              boltAuthPolicy.Primary.ExtJwt.Allowed,
			AllowedExtJwtSigners: boltAuthPolicy.Primary.ExtJwt.AllowedExtJwtSigners,
		},
	}
	entity.Secondary = AuthPolicySecondary{
		RequireTotp:          boltAuthPolicy.Secondary.RequireTotp,
		RequiredExtJwtSigner: boltAuthPolicy.Secondary.RequiredExtJwtSigner,
	}

	return nil
}

func (entity *AuthPolicy) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	boltEntity := &persistence.AuthPolicy{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Primary: persistence.AuthPolicyPrimary{
			Cert: persistence.AuthPolicyCert{
				Allowed:           entity.Primary.Cert.Allowed,
				AllowExpiredCerts: entity.Primary.Cert.AllowExpiredCerts,
			},
			Updb: persistence.AuthPolicyUpdb{
				Allowed:                entity.Primary.Updb.Allowed,
				MinPasswordLength:      entity.Primary.Updb.MinPasswordLength,
				RequireSpecialChar:     entity.Primary.Updb.RequireSpecialChar,
				RequireNumberChar:      entity.Primary.Updb.RequireNumberChar,
				RequireMixedCase:       entity.Primary.Updb.RequireMixedCase,
				MaxAttempts:            entity.Primary.Updb.MaxAttempts,
				LockoutDurationMinutes: entity.Primary.Updb.LockoutDurationMinutes,
			},
			ExtJwt: persistence.AuthPolicyExtJwt{
				Allowed:              entity.Primary.ExtJwt.Allowed,
				AllowedExtJwtSigners: entity.Primary.ExtJwt.AllowedExtJwtSigners,
			},
		},
		Secondary: persistence.AuthPolicySecondary{
			RequireTotp:          entity.Secondary.RequireTotp,
			RequiredExtJwtSigner: entity.Secondary.RequiredExtJwtSigner,
		},
	}

	return boltEntity, nil
}

func (entity *AuthPolicy) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	return entity.toBoltEntityForCreate(tx, handler)
}

func (entity *AuthPolicy) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}
