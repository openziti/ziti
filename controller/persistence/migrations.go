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
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
)

const (
	CurrentDbVersion = 4
	FieldVersion     = "version"
)

type Migrations struct {
	stores   *Stores
	dbStores *migration.Stores
}

func RunMigrations(db boltz.Db, stores *Stores, dbStores *migration.Stores) error {
	migrations := &Migrations{
		stores:   stores,
		dbStores: dbStores,
	}

	mm := boltz.NewMigratorManager(db)
	return mm.Migrate("edge", CurrentDbVersion, migrations.migrate)
}

func (m *Migrations) migrate(step *boltz.MigrationStep) int {
	if step.CurrentVersion == 0 {
		return m.initialize(step)
	}

	if step.CurrentVersion == 1 {
		m.createEdgeRouterPoliciesV2(step)
		return 2
	}

	if step.CurrentVersion == 2 {
		m.createServicePoliciesV3(step)
		return 3
	}

	if step.CurrentVersion == 3 {
		m.createInitialTunnelerConfigTypes(step)
		m.createInitialTunnelerConfigs(step)
		return 4
	}

	if step.CurrentVersion == 4 {
		m.createEnrollmentsForEdgeRouters(step)
		return 5
	}
	// current version
	if step.CurrentVersion == 5 {
		return 5
	}

	step.SetError(errors.Errorf("Unsupported edge datastore version: %v", step.CurrentVersion))
	return 0
}
