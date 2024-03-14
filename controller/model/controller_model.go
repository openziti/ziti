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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type Controller struct {
	models.BaseEntity
	Name         string
	Address      string
	CertPem      string
	Fingerprint  string
	IsOnline     bool
	LastJoinedAt *time.Time
}

func (entity *Controller) toBoltEntity(tx *bbolt.Tx, env Env) (*db.Controller, error) {
	boltEntity := &db.Controller{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Address:       entity.Address,
		CertPem:       entity.CertPem,
		Fingerprint:   entity.Fingerprint,
		IsOnline:      entity.IsOnline,
		LastJoinedAt:  entity.LastJoinedAt,
	}

	return boltEntity, nil
}

func (entity *Controller) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Controller, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Controller) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Controller, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Controller) fillFrom(env Env, tx *bbolt.Tx, boltController *db.Controller) error {
	entity.FillCommon(boltController)
	entity.Name = boltController.Name
	entity.Address = boltController.Address
	entity.CertPem = boltController.CertPem
	entity.Fingerprint = boltController.Fingerprint
	entity.IsOnline = boltController.IsOnline
	entity.LastJoinedAt = boltController.LastJoinedAt

	return nil
}
