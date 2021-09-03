package persistence

import (
	"fmt"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

func (m *Migrations) addIdentityIdToSessions(step *boltz.MigrationStep) {
	cursor := m.stores.Session.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue)

	fieldChecker := boltz.MapFieldChecker{
		FieldSessionIdentity: struct{}{},
	}

	for cursor.IsValid() {
		sessionId := string(cursor.Current())
		session, err := m.stores.Session.LoadOneById(step.Ctx.Tx(), sessionId)

		if err != nil {
			step.SetError(fmt.Errorf("could no load session by id [%s]: %v", sessionId, err))
			return
		}

		if session == nil {
			step.SetError(fmt.Errorf("session [%s] load did not error but session is null", sessionId))
			return
		}

		if session.IdentityId == "" {
			if apiSession, err := m.stores.ApiSession.LoadOneById(step.Ctx.Tx(), session.ApiSessionId); err == nil {
				if apiSession != nil {
					session.IdentityId = apiSession.IdentityId
					if err = m.stores.Session.Update(step.Ctx, session, fieldChecker); err != nil {
						step.SetError(fmt.Errorf("could not update session [%s]: %v", session.Id, err))
						return
					}
				}
			}
		}

		cursor.Next()
	}
}
