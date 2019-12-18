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
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
)

type ServicePolicy struct {
	BaseModelEntityImpl
	Name          string
	PolicyType    string
	IdentityRoles []string
	ServiceRoles  []string
}

func (entity *ServicePolicy) ToBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	if !strings.EqualFold(entity.PolicyType, persistence.PolicyTypeDialName) && !strings.EqualFold(entity.PolicyType, persistence.PolicyTypeBindName) {
		msg := fmt.Sprintf("invalid policy type. valid types are '%v' and '%v'", persistence.PolicyTypeDialName, persistence.PolicyTypeBindName)
		return nil, NewFieldError(msg, "policyType", entity.PolicyType)
	}

	policyType := persistence.PolicyTypeInvalid
	if strings.EqualFold(entity.PolicyType, persistence.PolicyTypeDialName) {
		policyType = persistence.PolicyTypeDial
	} else if strings.EqualFold(entity.PolicyType, persistence.PolicyTypeBindName) {
		policyType = persistence.PolicyTypeBind
	}

	return &persistence.ServicePolicy{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		PolicyType:         policyType,
		IdentityRoles:      entity.IdentityRoles,
		ServiceRoles:       entity.ServiceRoles,
	}, nil
}

func (entity *ServicePolicy) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *ServicePolicy) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *ServicePolicy) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltServicePolicy, ok := boltEntity.(*persistence.ServicePolicy)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model cluster", reflect.TypeOf(boltEntity))
	}

	policyType := "Invalid"
	if boltServicePolicy.PolicyType == persistence.PolicyTypeDial {
		policyType = persistence.PolicyTypeDialName
	} else if boltServicePolicy.PolicyType == persistence.PolicyTypeBind {
		policyType = persistence.PolicyTypeBindName
	}

	entity.fillCommon(boltServicePolicy)
	entity.Name = boltServicePolicy.Name
	entity.PolicyType = policyType
	entity.ServiceRoles = boltServicePolicy.ServiceRoles
	entity.IdentityRoles = boltServicePolicy.IdentityRoles
	return nil
}
