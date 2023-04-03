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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
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
		baseEntityManager: newBaseEntityManager(env, env.GetStores().PostureCheck),
		cache:             cache,
	}
	manager.impl = manager
	network.RegisterManagerDecoder[*PostureCheck](env.GetHostController().GetNetwork().GetManagers(), manager)

	evictF := func(i ...interface{}) {
		postureCheck := i[0].(*persistence.PostureCheck)
		manager.cache.Remove(postureCheck.Id)
	}

	manager.Store.AddListener(boltz.EventUpdate, evictF)
	manager.Store.AddListener(boltz.EventDelete, evictF)

	return manager
}

type PostureCheckManager struct {
	baseEntityManager
	cache *lru.Cache[string, *PostureCheck]
}

func (self *PostureCheckManager) newModelEntity() edgeEntity {
	return &PostureCheck{}
}

func (self *PostureCheckManager) Create(entity *PostureCheck) error {
	return network.DispatchCreate[*PostureCheck](self, entity)
}

func (self *PostureCheckManager) ApplyCreate(cmd *command.CreateEntityCommand[*PostureCheck]) error {
	_, err := self.createEntity(cmd.Entity)
	return err
}

func (self *PostureCheckManager) Update(entity *PostureCheck, checker fields.UpdatedFields) error {
	return network.DispatchUpdate[*PostureCheck](self, entity, checker)
}

func (self *PostureCheckManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*PostureCheck]) error {
	var checker boltz.FieldChecker = self
	if cmd.UpdatedFields != nil {
		checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
	}
	return self.updateEntity(cmd.Entity, checker)
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
	return strings.EqualFold(field, persistence.FieldName) ||
		strings.EqualFold(field, boltz.FieldTags) ||
		strings.EqualFold(field, persistence.FieldRoleAttributes) ||
		strings.EqualFold(field, persistence.FieldPostureCheckOsType) ||
		strings.EqualFold(field, persistence.FieldPostureCheckOsVersions) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMacAddresses) ||
		strings.EqualFold(field, persistence.FieldPostureCheckDomains) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessFingerprint) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessOs) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessPath) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessHashes) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaPromptOnWake) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaPromptOnUnlock) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaTimeoutSeconds) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaIgnoreLegacyEndpoints) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiOsType) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiHashes) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiPath) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiSignerFingerprints) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiProcesses) ||
		strings.EqualFold(field, persistence.FieldSemantic)
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
