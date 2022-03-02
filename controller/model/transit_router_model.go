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
	"reflect"

	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type TransitRouter struct {
	models.BaseEntity
	Name                  string
	Fingerprint           *string
	IsVerified            bool
	IsBase                bool
	UnverifiedFingerprint *string
	UnverifiedCertPem     *string
	Cost                  uint16
	NoTraversal           bool
}

func (entity *TransitRouter) toBoltEntityForCreate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	boltEntity := &persistence.TransitRouter{
		Router: db.Router{
			BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
			Name:          entity.Name,
			Fingerprint:   entity.Fingerprint,
			Cost:          entity.Cost,
			NoTraversal:   entity.NoTraversal,
		},
		IsVerified: false,
	}

	return boltEntity, nil
}

func (entity *TransitRouter) toBoltEntityForUpdate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	ret := &persistence.TransitRouter{
		Router: db.Router{
			BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
			Name:          entity.Name,
			Fingerprint:   entity.Fingerprint,
			Cost:          entity.Cost,
			NoTraversal:   entity.NoTraversal,
		},
		IsVerified:            entity.IsVerified,
		UnverifiedFingerprint: entity.UnverifiedFingerprint,
		UnverifiedCertPem:     entity.UnverifiedCertPem,
	}

	return ret, nil
}

func (entity *TransitRouter) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}

func (entity *TransitRouter) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltTransitRouter, ok := boltEntity.(*persistence.TransitRouter)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model transitRouter", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltTransitRouter)
	entity.Name = boltTransitRouter.Name
	entity.IsVerified = boltTransitRouter.IsVerified
	entity.IsBase = boltTransitRouter.IsBase
	entity.Fingerprint = boltTransitRouter.Fingerprint
	entity.UnverifiedFingerprint = boltTransitRouter.UnverifiedFingerprint
	entity.UnverifiedCertPem = boltTransitRouter.UnverifiedCertPem
	entity.Cost = boltTransitRouter.Cost
	entity.NoTraversal = boltTransitRouter.NoTraversal

	return nil
}
