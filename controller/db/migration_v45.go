package db

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
)

func (m *Migrations) setConfigTypeTargets(step *boltz.MigrationStep) {
	// Set target = "service" on all config types that don't already have a target.
	// Before this migration, no config types had a target field. All existing
	// config types were used for services, so "service" is the correct default.
	serviceTarget := ConfigTypeTargetService
	cursor := m.stores.ConfigType.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue)
	for cursor.IsValid() {
		id := string(cursor.Current())
		ct, err := m.stores.ConfigType.LoadById(step.Ctx.Tx(), id)
		if step.SetError(err) {
			return
		}
		if ct.Target == nil {
			ct.Target = &serviceTarget
			step.SetError(m.stores.ConfigType.Update(step.Ctx, ct, boltz.MapFieldChecker{
				FieldConfigTypeTarget: struct{}{},
			}))
		}
		cursor.Next()
	}
}
