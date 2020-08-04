package persistence

import (
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

func (m *Migrations) fixServicePolicyTypes(step *boltz.MigrationStep) {
	checker := boltz.MapFieldChecker{"type": struct{}{}}
	for cursor := m.stores.ServicePolicy.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		id := cursor.Current()
		policy, _ := m.stores.ServicePolicy.LoadOneById(step.Ctx.Tx(), string(id))
		if policy != nil {
			if policy.PolicyType == 5 { // old value for Bind
				policy.PolicyType = PolicyTypeBind
			} else {
				policy.PolicyType = PolicyTypeDial
			}
			step.SetError(m.stores.ServicePolicy.Update(step.Ctx, policy, checker))
		}
	}
}
