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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type PostureCheck struct {
	models.BaseEntity
	Name           string
	TypeId         string
	Version        int64
	RoleAttributes []string
	SubType        PostureCheckSubType
}

type PostureCheckSubType interface {
	TypeId() string
	toBoltEntityForCreate(tx *bbolt.Tx, env Env) (db.PostureCheckSubType, error)
	fillFrom(env Env, tx *bbolt.Tx, check *db.PostureCheck, subType db.PostureCheckSubType) error
	Evaluate(apiSessionId string, pd *PostureData) bool
	FailureValues(_ string, pd *PostureData) PostureCheckFailureValues
	GetTimeoutSeconds() int64
	GetTimeoutRemainingSeconds(apiSessionId string, pd *PostureData) int64

	fillProtobuf(msg *edge_cmd_pb.PostureCheck)
	fillFromProtobuf(msg *edge_cmd_pb.PostureCheck) error

	// LastUpdatedAt returns the last time the posture state changed or nil if not supported.
	LastUpdatedAt(id string, pd *PostureData) *time.Time
}

type PostureCheckFailureValues interface {
	Expected() interface{}
	Actual() interface{}
}

type newPostureCheckSubType func() PostureCheckSubType

const (
	PostureCheckTypeOs           = "OS"
	PostureCheckTypeDomain       = "DOMAIN"
	PostureCheckTypeProcess      = "PROCESS"
	PostureCheckTypeProcessMulti = "PROCESS_MULTI"
	PostureCheckTypeMAC          = "MAC"
	PostureCheckTypeMFA          = "MFA"
)

var postureCheckSubTypeMap = map[string]newPostureCheckSubType{
	PostureCheckTypeOs:           newPostureCheckOperatingSystem,
	PostureCheckTypeDomain:       newPostureCheckWindowsDomains,
	PostureCheckTypeProcess:      newPostureCheckProcess,
	PostureCheckTypeProcessMulti: newPostureCheckProcessMulti,
	PostureCheckTypeMAC:          newPostureCheckMacAddresses,
	PostureCheckTypeMFA:          newPostureCheckMfa,
}

func newSubType(typeId string) PostureCheckSubType {
	if factory, ok := postureCheckSubTypeMap[typeId]; ok {
		return factory()
	}
	return nil
}

func (entity *PostureCheck) fillFrom(env Env, tx *bbolt.Tx, boltPostureCheck *db.PostureCheck) error {
	entity.FillCommon(boltPostureCheck)
	entity.Name = boltPostureCheck.Name
	entity.TypeId = boltPostureCheck.TypeId
	entity.Version = boltPostureCheck.Version
	entity.RoleAttributes = boltPostureCheck.RoleAttributes

	subType := newSubType(entity.TypeId)

	if subType == nil {
		return fmt.Errorf("cannot create posture check subtype [%v]", entity.TypeId)
	}

	if err := subType.fillFrom(env, tx, boltPostureCheck, boltPostureCheck.SubType); err != nil {
		return fmt.Errorf("error filling posture check subType [%v]: %v", entity.TypeId, err)
	}

	entity.SubType = subType

	return nil
}

func (entity *PostureCheck) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.PostureCheck, error) {
	boltEntity := &db.PostureCheck{
		BaseExtEntity:  *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:           entity.Name,
		TypeId:         entity.TypeId,
		Version:        1,
		RoleAttributes: entity.RoleAttributes,
	}

	var err error
	if boltEntity.SubType, err = entity.SubType.toBoltEntityForCreate(tx, env); err != nil {
		return nil, fmt.Errorf("error converting to bolt posture check subType [%v] for create: %v", entity.TypeId, err)
	}

	return boltEntity, nil
}

func (entity *PostureCheck) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.PostureCheck, error) {
	return entity.toBoltEntityForCreate(tx, env)
}

func (entity *PostureCheck) Evaluate(apiSessionId string, pd *PostureData) (bool, *PostureCheckFailure) {
	if !entity.SubType.Evaluate(apiSessionId, pd) {
		return false, &PostureCheckFailure{
			PostureCheckId:            entity.Id,
			PostureCheckName:          entity.Name,
			PostureCheckType:          entity.TypeId,
			PostureCheckFailureValues: entity.SubType.FailureValues(apiSessionId, pd),
		}
	}

	return true, nil
}

func (entity *PostureCheck) TimeoutSeconds() int64 {
	return entity.SubType.GetTimeoutSeconds()
}

func (entity *PostureCheck) TimeoutRemainingSeconds(apiSessionId string, pd *PostureData) int64 {
	return entity.SubType.GetTimeoutRemainingSeconds(apiSessionId, pd)
}

// LastUpdatedAt returns the last time posture state changed for a specific posture check.
// If the posture state does not report changes, nil is returned.
func (entity *PostureCheck) LastUpdatedAt(apiSessionId string, pd *PostureData) *time.Time {
	return entity.SubType.LastUpdatedAt(apiSessionId, pd)
}
