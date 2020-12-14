package persistence

import (
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

var identityTypesMap = map[string]string{
	"577104f2-1e3a-4947-a927-7383baefbc9a": "User",
	"5b53fb49-51b1-4a87-a4e4-edda9716a970": "Device",
	"c4d66f9d-fe18-4143-85d3-74329c54282b": "Service",
}

// Update identity types
func (m *Migrations) updateIdentityTypes(step *boltz.MigrationStep) {
	for id := range identityTypesMap {
		m.migrateIdentityType(step, id)
	}

	newType := &IdentityType{
		BaseExtEntity: boltz.BaseExtEntity{
			Id: RouterIdentityType,
		},
		Name: RouterIdentityType,
	}

	if step.SetError(m.stores.IdentityType.Create(step.Ctx, newType)) {
		return
	}

	identityStore := m.stores.Identity

	for cursor := identityStore.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		current := cursor.Current()
		identity, err := identityStore.LoadOneById(step.Ctx.Tx(), string(current))
		if step.SetError(err) {
			return
		}
		updatedIdentityTypeId, ok := identityTypesMap[identity.IdentityTypeId]
		if !ok {
			step.SetError(errors.Errorf("no updating identity id mapping found for identity type id %v", identity.IdentityTypeId))
			return
		}

		logrus.Debugf("updating identity %v type id from %v -> %v", identity.Id, identity.IdentityTypeId, updatedIdentityTypeId)

		identity.IdentityTypeId = updatedIdentityTypeId
		err = identityStore.Update(step.Ctx, identity, boltz.MapFieldChecker{
			"identityTypeId": struct{}{},
		})
		if step.SetError(err) {
			return
		}
	}

	for id := range identityTypesMap {
		if step.SetError(m.stores.IdentityType.DeleteById(step.Ctx, id)) {
			return
		}
	}
}

func (m *Migrations) migrateIdentityType(step *boltz.MigrationStep, id string) {
	idType, err := m.stores.IdentityType.LoadOneById(step.Ctx.Tx(), id)
	if step.SetError(err) {
		return
	}

	name := idType.Name

	idType.Name = name + "Migration"
	if step.SetError(m.stores.IdentityType.Update(step.Ctx, idType, nil)) {
		return
	}

	now := time.Now()

	newType := &IdentityType{
		BaseExtEntity: boltz.BaseExtEntity{
			Id:        name,
			CreatedAt: idType.CreatedAt,
			UpdatedAt: now,
			Migrate:   true,
		},
		Name: name,
	}

	step.SetError(m.stores.IdentityType.Create(step.Ctx, newType))
}
