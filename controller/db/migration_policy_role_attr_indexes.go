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

package db

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
)

// backfillPolicyRoleAttributeIndexes populates the derived role-attribute
// indexes added to the three policy stores for role-attribute usage queries.
// These indexes are derived from existing persisted role fields via
// boltz.SetIndexValueTransform, so running each index's own integrity check
// with fix=true is sufficient to construct it from live data.
//
// Only the new role-attribute indexes are checked, not the entire store, so
// the migration neither inspects nor mutates any unrelated index. This keeps
// the per-index "fixed" counts meaningful: they reflect role-attribute index
// entries created during the backfill, not incidental repairs elsewhere.
//
// Logging policy: each fixed entry is counted (not logged individually) and a
// single per-index summary is emitted at Info. Unfixable entries are still
// logged individually at Error so that operator-actionable problems remain
// visible.
func (m *Migrations) backfillPolicyRoleAttributeIndexes(step *boltz.MigrationStep) {
	indexes := []boltz.SetReadIndex{
		m.stores.ServicePolicy.GetIdentityRoleAttributesIndex(),
		m.stores.ServicePolicy.GetServiceRoleAttributesIndex(),
		m.stores.ServicePolicy.GetPostureCheckRoleAttributesIndex(),
		m.stores.EdgeRouterPolicy.GetIdentityRoleAttributesIndex(),
		m.stores.EdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex(),
		m.stores.ServiceEdgeRouterPolicy.GetServiceRoleAttributesIndex(),
		m.stores.ServiceEdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex(),
	}

	for _, index := range indexes {
		symbol := index.GetSymbol()
		entityType := symbol.GetStore().GetEntityType()
		fieldName := symbol.GetName()

		// SetReadIndex is the read-only handle; the concrete set index also
		// implements Checkable, which is how we drive the backfill for just
		// this one index.
		checkable, ok := index.(boltz.Checkable)
		if !ok {
			step.SetError(errors.Errorf("role-attribute index %s.%s is not checkable, cannot backfill", entityType, fieldName))
			return
		}

		var fixed, unfixable int
		err := checkable.CheckIntegrity(step.Ctx, true, func(err error, wasFixed bool) {
			if wasFixed {
				fixed++
				return
			}
			unfixable++
			pfxlog.Logger().WithError(err).
				WithField("store", entityType).
				WithField("field", fieldName).
				Error("unfixable error during policy role-attribute index rebuild")
		})
		pfxlog.Logger().
			WithField("store", entityType).
			WithField("field", fieldName).
			WithField("fixed", fixed).
			WithField("unfixable", unfixable).
			Info("policy role-attribute index rebuild summary")
		// SetError records only the first error, and a failed migration rolls
		// the whole tx back anyway, so stop rather than churn through the
		// remaining indexes in a doomed transaction.
		if step.SetError(err) {
			return
		}
	}
}
