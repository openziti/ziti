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
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"strings"
)

const (
	PostureCheckNoTimeout = int64(-1)
)

func NewPostureCheckManager(env Env) *PostureCheckManager {
	cache, err := lru.New[string, *PostureCheck](256)
	if err != nil {
		panic(err)
	}
	manager := &PostureCheckManager{
		baseEntityManager: newBaseEntityManager[*PostureCheck, *db.PostureCheck](env, env.GetStores().PostureCheck),
		cache:             cache,
	}
	manager.impl = manager
	network.RegisterManagerDecoder[*PostureCheck](env.GetHostController().GetNetwork().GetManagers(), manager)

	evictF := func(postureCheckId string) {
		manager.cache.Remove(postureCheckId)
	}

	manager.Store.AddEntityIdListener(evictF, boltz.EntityUpdated, boltz.EntityDeleted)

	return manager
}

type PostureCheckManager struct {
	baseEntityManager[*PostureCheck, *db.PostureCheck]
	cache *lru.Cache[string, *PostureCheck]
}

func (self *PostureCheckManager) newModelEntity() *PostureCheck {
	return &PostureCheck{}
}

func (self *PostureCheckManager) Create(entity *PostureCheck, ctx *change.Context) error {
	return network.DispatchCreate[*PostureCheck](self, entity, ctx)
}

func (self *PostureCheckManager) ApplyCreate(cmd *command.CreateEntityCommand[*PostureCheck], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *PostureCheckManager) Update(entity *PostureCheck, checker fields.UpdatedFields, ctx *change.Context) error {
	return network.DispatchUpdate[*PostureCheck](self, entity, checker, ctx)
}

func (self *PostureCheckManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*PostureCheck], ctx boltz.MutateContext) error {
	var checker boltz.FieldChecker = self
	if cmd.UpdatedFields != nil {
		checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
	}
	return self.updateEntity(cmd.Entity, checker, ctx)
}

func (self *PostureCheckManager) Read(id string) (*PostureCheck, error) {
	if postureCheck, ok := self.cache.Get(id); ok {
		return postureCheck, nil
	}
	modelEntity := &PostureCheck{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *PostureCheckManager) readInTx(tx *bbolt.Tx, id string) (*PostureCheck, error) {
	if postureCheck, ok := self.cache.Get(id); ok {
		return postureCheck, nil
	}

	modelEntity := &PostureCheck{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	self.cache.Add(id, modelEntity)

	return modelEntity, nil
}

func (self *PostureCheckManager) IsUpdated(field string) bool {
	return strings.EqualFold(field, db.FieldName) ||
		strings.EqualFold(field, boltz.FieldTags) ||
		strings.EqualFold(field, db.FieldRoleAttributes) ||
		strings.EqualFold(field, db.FieldPostureCheckOsType) ||
		strings.EqualFold(field, db.FieldPostureCheckOsVersions) ||
		strings.EqualFold(field, db.FieldPostureCheckMacAddresses) ||
		strings.EqualFold(field, db.FieldPostureCheckDomains) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessFingerprint) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessOs) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessPath) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessHashes) ||
		strings.EqualFold(field, db.FieldPostureCheckMfaPromptOnWake) ||
		strings.EqualFold(field, db.FieldPostureCheckMfaPromptOnUnlock) ||
		strings.EqualFold(field, db.FieldPostureCheckMfaTimeoutSeconds) ||
		strings.EqualFold(field, db.FieldPostureCheckMfaIgnoreLegacyEndpoints) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessMultiOsType) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessMultiHashes) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessMultiPath) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessMultiSignerFingerprints) ||
		strings.EqualFold(field, db.FieldPostureCheckProcessMultiProcesses) ||
		strings.EqualFold(field, db.FieldSemantic)
}

func (self *PostureCheckManager) Query(query string) (*PostureCheckListResult, error) {
	result := &PostureCheckListResult{manager: self}
	if err := self.ListWithHandler(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *PostureCheckManager) QueryPostureChecks(query ast.Query) (*PostureCheckListResult, error) {
	result := &PostureCheckListResult{manager: self}
	err := self.PreparedListWithHandler(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *PostureCheckManager) Marshall(entity *PostureCheck) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.PostureCheck{
		Id:             entity.Id,
		Name:           entity.Name,
		Tags:           tags,
		TypeId:         entity.TypeId,
		Version:        entity.Version,
		RoleAttributes: entity.RoleAttributes,
	}

	if entity.SubType != nil {
		entity.SubType.fillProtobuf(msg)
		msg.TypeId = entity.SubType.TypeId()
	}

	return proto.Marshal(msg)
}

func (self *PostureCheckManager) Unmarshall(bytes []byte) (*PostureCheck, error) {
	msg := &edge_cmd_pb.PostureCheck{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	var subType PostureCheckSubType
	if msg.Subtype != nil {
		subType = newSubType(msg.TypeId)
		if subType == nil {
			return nil, fmt.Errorf("cannot create posture check subtype [%v]", msg.TypeId)
		}

		if err := subType.fillFromProtobuf(msg); err != nil {
			return nil, err
		}
	}

	return &PostureCheck{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:           msg.Name,
		TypeId:         msg.TypeId,
		Version:        msg.Version,
		RoleAttributes: msg.RoleAttributes,
		SubType:        subType,
	}, nil
}

type PostureCheckListResult struct {
	manager       *PostureCheckManager
	PostureChecks []*PostureCheck
	models.QueryMetaData
}

func (result *PostureCheckListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.manager.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.PostureChecks = append(result.PostureChecks, entity)
	}
	return nil
}
