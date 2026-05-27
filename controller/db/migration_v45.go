package db

import (
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

// setConfigTypeTargets fills in target="service" on every config type that
// lacks a target value. Before this migration, no config types had a target
// field; all existing config types were used for services, so "service" is the
// correct default. The field is written directly via the entity bucket rather
// than going through the store's load/update path, because reloading a config
// type without a target value would fail validation in FillEntity.
func (m *Migrations) setConfigTypeTargets(step *boltz.MigrationStep) {
	cursor := m.stores.ConfigType.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue)
	for cursor.IsValid() {
		id := string(cursor.Current())
		bucket := m.stores.ConfigType.GetEntityBucket(step.Ctx.Tx(), []byte(id))
		if bucket == nil {
			cursor.Next()
			continue
		}
		existing := bucket.GetString(FieldConfigTypeTarget)
		if existing == nil || *existing == "" {
			bucket.SetString(FieldConfigTypeTarget, ConfigTypeTargetService, nil)
			if step.SetError(bucket.GetError()) {
				return
			}
		}
		cursor.Next()
	}
}
