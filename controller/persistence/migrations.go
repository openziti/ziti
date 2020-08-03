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

package persistence

import (
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
)

const (
	CurrentDbVersion = 8
	FieldVersion     = "version"
)

type Migrations struct {
	stores *Stores
}

func RunMigrations(db boltz.Db, stores *Stores) error {
	migrations := &Migrations{
		stores: stores,
	}

	mm := boltz.NewMigratorManager(db)
	return mm.Migrate("edge", CurrentDbVersion, migrations.migrate)
}

func (m *Migrations) migrate(step *boltz.MigrationStep) int {
	if step.CurrentVersion == 0 {
		return m.initialize(step)
	}

	if step.CurrentVersion > CurrentDbVersion {
		step.SetError(errors.Errorf("Unsupported edge datastore version: %v", step.CurrentVersion))
		return step.CurrentVersion
	}

	if step.CurrentVersion < 4 {
		m.createInitialTunnelerConfigTypes(step)
		m.createInitialTunnelerConfigs(step)
	}

	if step.CurrentVersion < 5 {
		m.createEnrollmentsForEdgeRouters(step)
	}

	if step.CurrentVersion < 6 {
		m.fixIdentityBuckets(step)
	}

	if step.CurrentVersion < 7 {
		m.moveTransitRouters(step)
		m.moveEdgeRoutersUnderFabricRouters(step)
		m.copyNamesToParent(step, m.stores.EdgeService)
		m.copyNamesToParent(step, m.stores.EdgeRouter)
		m.copyNamesToParent(step, m.stores.TransitRouter)
		m.fixAuthenticatorCertFingerprints(step)
	}

	if step.CurrentVersion < 8 {
		m.denormalizePolicies(step)
		m.fixNameIndices(step)
	}

	// current version
	if step.CurrentVersion <= CurrentDbVersion {
		return CurrentDbVersion
	}

	step.SetError(errors.Errorf("Unsupported edge datastore version: %v", step.CurrentVersion))
	return 0
}
