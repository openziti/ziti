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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
)

type ServicePolicy struct {
	models.BaseEntity
	Name              string
	PolicyType        string
	Semantic          string
	IdentityRoles     []string
	ServiceRoles      []string
	PostureCheckRoles []string
}

func (entity *ServicePolicy) validatePolicyType() error {
	if !strings.EqualFold(entity.PolicyType, persistence.PolicyTypeDialName) && !strings.EqualFold(entity.PolicyType, persistence.PolicyTypeBindName) {
		msg := fmt.Sprintf("invalid policy type. valid types are '%v' and '%v'", persistence.PolicyTypeDialName, persistence.PolicyTypeBindName)
		return errorz.NewFieldError(msg, "policyType", entity.PolicyType)
	}
	return nil
}

func (entity *ServicePolicy) toBoltEntity() (boltz.Entity, error) {
	policyType := persistence.PolicyTypeInvalid
	if strings.EqualFold(entity.PolicyType, persistence.PolicyTypeDialName) {
		policyType = persistence.PolicyTypeDial
	} else if strings.EqualFold(entity.PolicyType, persistence.PolicyTypeBindName) {
		policyType = persistence.PolicyTypeBind
	}

	return &persistence.ServicePolicy{
		BaseExtEntity:     *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:              entity.Name,
		PolicyType:        policyType,
		Semantic:          entity.Semantic,
		IdentityRoles:     entity.IdentityRoles,
		ServiceRoles:      entity.ServiceRoles,
		PostureCheckRoles: entity.PostureCheckRoles,
	}, nil
}

func (entity *ServicePolicy) toBoltEntityForCreate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ServicePolicy) toBoltEntityForUpdate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ServicePolicy) toBoltEntityForPatch(*bbolt.Tx, EntityManager, boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ServicePolicy) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltServicePolicy, ok := boltEntity.(*persistence.ServicePolicy)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service policy", reflect.TypeOf(boltEntity))
	}

	policyType := "Invalid"
	if boltServicePolicy.PolicyType == persistence.PolicyTypeDial {
		policyType = persistence.PolicyTypeDialName
	} else if boltServicePolicy.PolicyType == persistence.PolicyTypeBind {
		policyType = persistence.PolicyTypeBindName
	}

	entity.FillCommon(boltServicePolicy)
	entity.Name = boltServicePolicy.Name
	entity.PolicyType = policyType
	entity.Semantic = boltServicePolicy.Semantic
	entity.ServiceRoles = boltServicePolicy.ServiceRoles
	entity.IdentityRoles = boltServicePolicy.IdentityRoles
	entity.PostureCheckRoles = boltServicePolicy.PostureCheckRoles
	return nil
}
