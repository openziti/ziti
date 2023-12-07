package db

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
)

// Primes API Session's lastActivityAt proper to their previous updatedAt value
func (m *Migrations) setLastActivityAt(step *boltz.MigrationStep) {
	for cursor := m.stores.ApiSession.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		if apiSession, err := m.stores.ApiSession.LoadOneById(step.Ctx.Tx(), string(cursor.Current())); err == nil {
			apiSession.LastActivityAt = apiSession.UpdatedAt
			step.SetError(m.stores.ApiSession.Update(step.Ctx, apiSession, UpdateLastActivityAtChecker{}))
		} else {
			step.SetError(err)
			return
		}
	}
}
