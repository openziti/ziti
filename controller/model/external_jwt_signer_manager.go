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
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func NewExternalJwtSignerManager(env Env) *ExternalJwtSignerManager {
	manager := &ExternalJwtSignerManager{
		baseEntityManager: newBaseEntityManager[*ExternalJwtSigner, *db.ExternalJwtSigner](env, env.GetStores().ExternalJwtSigner),
	}
	manager.impl = manager

	RegisterManagerDecoder[*ExternalJwtSigner](env, manager)

	return manager
}

type ExternalJwtSignerManager struct {
	baseEntityManager[*ExternalJwtSigner, *db.ExternalJwtSigner]
}

func (self *ExternalJwtSignerManager) newModelEntity() *ExternalJwtSigner {
	return &ExternalJwtSigner{}
}

func (self *ExternalJwtSignerManager) Create(entity *ExternalJwtSigner, ctx *change.Context) error {
	return DispatchCreate[*ExternalJwtSigner](self, entity, ctx)
}

func (self *ExternalJwtSignerManager) ApplyCreate(cmd *command.CreateEntityCommand[*ExternalJwtSigner], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *ExternalJwtSignerManager) Update(entity *ExternalJwtSigner, checker fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*ExternalJwtSigner](self, entity, checker, ctx)
}

func (self *ExternalJwtSignerManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*ExternalJwtSigner], ctx boltz.MutateContext) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

func (self *ExternalJwtSignerManager) Marshall(entity *ExternalJwtSigner) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.ExternalJwtSigner{
		Id:              entity.Id,
		Name:            entity.Name,
		Tags:            tags,
		CertPem:         entity.CertPem,
		JwksEndpoint:    entity.JwksEndpoint,
		Kid:             entity.Kid,
		Enabled:         entity.Enabled,
		ExternalAuthUrl: entity.ExternalAuthUrl,
		UseExternalId:   entity.UseExternalId,
		ClaimsProperty:  entity.ClaimsProperty,
		Issuer:          entity.Issuer,
		Audience:        entity.Audience,
		CommonName:      entity.CommonName,
		Fingerprint:     entity.Fingerprint,
		NotAfter:        timestamppb.New(entity.NotAfter),
		NotBefore:       timestamppb.New(entity.NotBefore),
		Scopes:          entity.Scopes,
		ClientId:        entity.ClientId,
	}

	return proto.Marshal(msg)
}

func (self *ExternalJwtSignerManager) Unmarshall(bytes []byte) (*ExternalJwtSigner, error) {
	msg := &edge_cmd_pb.ExternalJwtSigner{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	if msg.NotAfter == nil || msg.NotBefore == nil {
		return nil, errors.New("invalid msg, NotAfter or NotBefore is nil")
	}

	return &ExternalJwtSigner{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:            msg.Name,
		CertPem:         msg.CertPem,
		JwksEndpoint:    msg.JwksEndpoint,
		Kid:             msg.Kid,
		Enabled:         msg.Enabled,
		ExternalAuthUrl: msg.ExternalAuthUrl,
		UseExternalId:   msg.UseExternalId,
		ClaimsProperty:  msg.ClaimsProperty,
		Issuer:          msg.Issuer,
		Audience:        msg.Audience,
		CommonName:      msg.CommonName,
		Fingerprint:     msg.Fingerprint,
		NotAfter:        msg.NotAfter.AsTime(),
		NotBefore:       msg.NotBefore.AsTime(),
		ClientId:        msg.ClientId,
		Scopes:          msg.Scopes,
	}, nil
}

type ListExtJwtSignerResult struct {
	manager       *ExternalJwtSignerManager
	QueryMetaData models.QueryMetaData
	ExtJwtSigners []*ExternalJwtSigner
}

func (self *ExternalJwtSignerManager) PublicQuery(query ast.Query) (*ListExtJwtSignerResult, error) {
	queryStr := "enabled = true"
	enabledQuery, err := ast.Parse(self.Store, queryStr)
	if err != nil {
		return nil, err
	}

	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), enabledQuery.GetPredicate()))

	entityResult, err := self.BasePreparedList(query)

	if err != nil {
		return nil, err
	}

	result := &ListExtJwtSignerResult{
		manager:       self,
		QueryMetaData: entityResult.QueryMetaData,
		ExtJwtSigners: entityResult.Entities,
	}

	return result, nil
}
