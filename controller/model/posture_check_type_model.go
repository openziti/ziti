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
	"github.com/openziti/ziti/controller/persistence"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

type PostureCheckType struct {
	models.BaseEntity
	Name             string
	OperatingSystems []OperatingSystem
}

func (entity *PostureCheckType) toBoltEntity() (*persistence.PostureCheckType, error) {
	var operatingSystems []persistence.OperatingSystem

	for _, os := range entity.OperatingSystems {
		operatingSystems = append(operatingSystems, persistence.OperatingSystem{
			OsType:     os.OsType,
			OsVersions: os.OsVersions,
		})
	}

	return &persistence.PostureCheckType{
		Name:             entity.Name,
		OperatingSystems: operatingSystems,
		BaseExtEntity:    *boltz.NewExtEntity(entity.Id, entity.Tags),
	}, nil
}

func (entity *PostureCheckType) toBoltEntityForCreate(*bbolt.Tx, Env) (*persistence.PostureCheckType, error) {
	return entity.toBoltEntity()
}

func (entity *PostureCheckType) toBoltEntityForUpdate(*bbolt.Tx, Env, boltz.FieldChecker) (*persistence.PostureCheckType, error) {
	return entity.toBoltEntity()
}

func (entity *PostureCheckType) fillFrom(_ Env, _ *bbolt.Tx, boltPostureCheckType *persistence.PostureCheckType) error {
	var operatingSystems []OperatingSystem

	for _, os := range boltPostureCheckType.OperatingSystems {
		operatingSystems = append(operatingSystems, OperatingSystem{
			OsType:     os.OsType,
			OsVersions: os.OsVersions,
		})
	}

	entity.FillCommon(boltPostureCheckType)
	entity.Name = boltPostureCheckType.Name
	entity.OperatingSystems = operatingSystems
	return nil
}
