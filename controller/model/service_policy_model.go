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
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
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
	if !strings.EqualFold(entity.PolicyType, db.PolicyTypeDialName) && !strings.EqualFold(entity.PolicyType, db.PolicyTypeBindName) {
		msg := fmt.Sprintf("invalid policy type. valid types are '%v' and '%v'", db.PolicyTypeDialName, db.PolicyTypeBindName)
		return errorz.NewFieldError(msg, "policyType", entity.PolicyType)
	}
	return nil
}

func (entity *ServicePolicy) toBoltEntity(checker boltz.FieldChecker) (*db.ServicePolicy, error) {
	if checker == nil || checker.IsUpdated(db.FieldServicePolicyType) {
		if err := entity.validatePolicyType(); err != nil {
			return nil, err
		}
	}

	policyType := db.PolicyTypeInvalid
	if strings.EqualFold(entity.PolicyType, db.PolicyTypeDialName) {
		policyType = db.PolicyTypeDial
	} else if strings.EqualFold(entity.PolicyType, db.PolicyTypeBindName) {
		policyType = db.PolicyTypeBind
	}

	return &db.ServicePolicy{
		BaseExtEntity:     *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:              entity.Name,
		PolicyType:        policyType,
		Semantic:          entity.Semantic,
		IdentityRoles:     entity.IdentityRoles,
		ServiceRoles:      entity.ServiceRoles,
		PostureCheckRoles: entity.PostureCheckRoles,
	}, nil
}

func (entity *ServicePolicy) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.ServicePolicy, error) {
	return entity.toBoltEntity(nil)
}

func (entity *ServicePolicy) toBoltEntityForUpdate(_ *bbolt.Tx, _ Env, checker boltz.FieldChecker) (*db.ServicePolicy, error) {
	return entity.toBoltEntity(checker)
}

func (entity *ServicePolicy) fillFrom(_ Env, _ *bbolt.Tx, boltServicePolicy *db.ServicePolicy) error {
	entity.FillCommon(boltServicePolicy)
	entity.Name = boltServicePolicy.Name
	entity.PolicyType = string(boltServicePolicy.PolicyType)
	entity.Semantic = boltServicePolicy.Semantic
	entity.ServiceRoles = boltServicePolicy.ServiceRoles
	entity.IdentityRoles = boltServicePolicy.IdentityRoles
	entity.PostureCheckRoles = boltServicePolicy.PostureCheckRoles
	return nil
}
