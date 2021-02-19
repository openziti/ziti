package persistence

import (
	"fmt"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

//Removes all ApiSession and Session from the edge. Necessary from 0.18 -> 0.19
//as the id format changed and API Session sync'ing depends on monotonic ids.
func (m *Migrations) removeAllSessions(step *boltz.MigrationStep) {
	for cursor := m.stores.Session.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		current := cursor.Current()
		currentSessionId := string(current)

		if err := m.stores.Session.DeleteById(step.Ctx, currentSessionId); err != nil {
			step.SetError(fmt.Errorf("error cleaing up sessions for migration: %v", err))
			return
		}
	}

	for cursor := m.stores.ApiSession.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		current := cursor.Current()
		currentApiSessionId := string(current)

		if err := m.stores.ApiSession.DeleteById(step.Ctx, currentApiSessionId); err != nil {
			step.SetError(fmt.Errorf("error cleaing up api sessions for migration: %v", err))
			return
		}
	}
}
