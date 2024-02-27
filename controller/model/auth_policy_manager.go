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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func NewAuthPolicyManager(env Env) *AuthPolicyManager {
	manager := &AuthPolicyManager{
		baseEntityManager: newBaseEntityManager[*AuthPolicy, *db.AuthPolicy](env, env.GetStores().AuthPolicy),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*AuthPolicy](env.GetHostController().GetNetwork().Managers, manager)

	return manager
}

type AuthPolicyManager struct {
	baseEntityManager[*AuthPolicy, *db.AuthPolicy]
}

func (self *AuthPolicyManager) Create(entity *AuthPolicy, ctx *change.Context) error {
	return network.DispatchCreate[*AuthPolicy](self, entity, ctx)
}

func (self *AuthPolicyManager) ApplyCreate(cmd *command.CreateEntityCommand[*AuthPolicy], ctx boltz.MutateContext) error {
	entity := cmd.Entity
	if entity.Secondary.RequiredExtJwtSigner != nil {
		if err := self.verifyExtJwt(*entity.Secondary.RequiredExtJwtSigner, "secondary.requiredExtJwtSigner"); err != nil {
			return err
		}
	}

	for i, extJwtId := range entity.Primary.ExtJwt.AllowedExtJwtSigners {
		if err := self.verifyExtJwt(extJwtId, fmt.Sprintf("primary.extJwt.allowedExtJwtSigners[%d]", i)); err != nil {
			return err
		}
	}

	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *AuthPolicyManager) Update(entity *AuthPolicy, checker fields.UpdatedFields, ctx *change.Context) error {
	return network.DispatchUpdate[*AuthPolicy](self, entity, checker, ctx)
}

func (self *AuthPolicyManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*AuthPolicy], ctx boltz.MutateContext) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

func (self *AuthPolicyManager) newModelEntity() *AuthPolicy {
	return &AuthPolicy{}
}

func (self *AuthPolicyManager) verifyExtJwt(id string, fieldName string) error {
	extJwtSigner, err := self.env.GetManagers().ExternalJwtSigner.Read(id)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return err
	}

	if extJwtSigner == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("not found", fieldName, id)
		apiErr.AppendCause = true
		return apiErr
	}

	return nil
}

func (self *AuthPolicyManager) Read(id string) (*AuthPolicy, error) {
	modelEntity := &AuthPolicy{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *AuthPolicyManager) Marshall(entity *AuthPolicy) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.AuthPolicy{
		Id:   entity.Id,
		Name: entity.Name,
		Primary: &edge_cmd_pb.AuthPolicy_Primary{
			Cert: &edge_cmd_pb.AuthPolicy_Primary_Cert{
				Allowed:           entity.Primary.Cert.Allowed,
				AllowExpiredCerts: entity.Primary.Cert.AllowExpiredCerts,
			},
			Updb: &edge_cmd_pb.AuthPolicy_Primary_Updb{
				Allowed:                entity.Primary.Updb.Allowed,
				MinPasswordLength:      entity.Primary.Updb.MinPasswordLength,
				RequireSpecialChar:     entity.Primary.Updb.RequireSpecialChar,
				RequireNumberChar:      entity.Primary.Updb.RequireNumberChar,
				RequireMixedCase:       entity.Primary.Updb.RequireMixedCase,
				MaxAttempts:            entity.Primary.Updb.MaxAttempts,
				LockoutDurationMinutes: entity.Primary.Updb.LockoutDurationMinutes,
			},
			ExtJwt: &edge_cmd_pb.AuthPolicy_Primary_ExtJwt{
				Allowed:              entity.Primary.ExtJwt.Allowed,
				AllowAllSigners:      entity.Primary.ExtJwt.AllowAllSigners,
				AllowedExtJwtSigners: entity.Primary.ExtJwt.AllowedExtJwtSigners,
			},
		},
		Secondary: &edge_cmd_pb.AuthPolicy_Secondary{
			RequireTotp:          entity.Secondary.RequireTotp,
			RequiredExtJwtSigner: entity.Secondary.RequiredExtJwtSigner,
		},
		Tags: tags,
	}

	return proto.Marshal(msg)
}

func (self *AuthPolicyManager) Unmarshall(bytes []byte) (*AuthPolicy, error) {
	msg := &edge_cmd_pb.AuthPolicy{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	if msg.Primary == nil {
		return nil, errors.Errorf("auth policy msg for id '%v' has nil Primary", msg.Id)
	}
	if msg.Primary.Cert == nil {
		return nil, errors.Errorf("auth policy msg for id '%v' has nil Primary.Cert", msg.Id)
	}
	if msg.Primary.Updb == nil {
		return nil, errors.Errorf("auth policy msg for id '%v' has nil Primary.Updb", msg.Id)
	}
	if msg.Primary.ExtJwt == nil {
		return nil, errors.Errorf("auth policy msg for id '%v' has nil Primary.ExtJwt", msg.Id)
	}
	if msg.Secondary == nil {
		return nil, errors.Errorf("auth policy msg for id '%v' has nil Secondary", msg.Id)
	}

	return &AuthPolicy{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name: msg.Name,
		Primary: AuthPolicyPrimary{
			Cert: AuthPolicyCert{
				Allowed:           msg.Primary.Cert.Allowed,
				AllowExpiredCerts: msg.Primary.Cert.AllowExpiredCerts,
			},
			Updb: AuthPolicyUpdb{
				Allowed:                msg.Primary.Updb.Allowed,
				MinPasswordLength:      msg.Primary.Updb.MinPasswordLength,
				RequireSpecialChar:     msg.Primary.Updb.RequireSpecialChar,
				RequireNumberChar:      msg.Primary.Updb.RequireNumberChar,
				RequireMixedCase:       msg.Primary.Updb.RequireMixedCase,
				MaxAttempts:            msg.Primary.Updb.MaxAttempts,
				LockoutDurationMinutes: msg.Primary.Updb.LockoutDurationMinutes,
			},
			ExtJwt: AuthPolicyExtJwt{
				Allowed:              msg.Primary.ExtJwt.Allowed,
				AllowAllSigners:      msg.Primary.ExtJwt.AllowAllSigners,
				AllowedExtJwtSigners: msg.Primary.ExtJwt.AllowedExtJwtSigners,
			},
		},
		Secondary: AuthPolicySecondary{
			RequireTotp:          msg.Secondary.RequireTotp,
			RequiredExtJwtSigner: msg.Secondary.RequiredExtJwtSigner,
		},
	}, nil
}
