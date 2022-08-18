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

package persistence

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
)

const (
	CurrentDbVersion = 29
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

	if step.CurrentVersion < 13 {
		step.SetError(errors.Errorf("Unsupported edge datastore version: %v", step.CurrentVersion))
		return step.CurrentVersion
	}

	if step.CurrentVersion < 14 {
		m.createInterceptV1ConfigType(step)
		m.createHostV1ConfigType(step)
	}

	if step.CurrentVersion < 15 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, serverConfigTypeV1, nil))
	}

	if step.CurrentVersion < 16 {
		m.removeOrphanedOttCaEnrollments(step)
	}

	if step.CurrentVersion < 17 {
		m.removeAllSessions(step)
	}

	if step.CurrentVersion < 18 {
		m.setLastActivityAt(step)
	}

	if step.CurrentVersion < 19 {
		m.updateIdentityTypes(step)
	}

	if step.CurrentVersion < 20 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, serverConfigTypeV1, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Create(step.Ctx, hostV2ConfigType))
	}

	if step.CurrentVersion < 21 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, interceptV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV2ConfigType, nil))
	}

	if step.CurrentVersion < 22 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV2ConfigType, nil))
	}

	if step.CurrentVersion < 23 {
		m.addProcessMultiPostureCheck(step)
	}

	if step.CurrentVersion < 24 {
		m.addIdentityIdToSessions(step)
	}

	if step.CurrentVersion < 25 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, interceptV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV2ConfigType, nil))
	}

	if step.CurrentVersion < 26 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV1ConfigType, nil))
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV2ConfigType, nil))
	}

	if step.CurrentVersion < 27 {
		m.addSystemAuthPolicies(step)
	}

	if step.CurrentVersion < 28 {
		step.SetError(m.stores.ConfigType.Update(step.Ctx, hostV2ConfigType, nil))
	}

	if step.CurrentVersion < 29 {
		m.dropEntity(step, "geoRegions")
		m.dropEntity(step, "eventLogs")
	}

	// current version
	if step.CurrentVersion <= CurrentDbVersion {
		return CurrentDbVersion
	}

	step.SetError(errors.Errorf("Unsupported edge datastore version: %v", step.CurrentVersion))
	return 0
}

func (m *Migrations) dropEntity(step *boltz.MigrationStep, entityType string) {
	rootBucket := step.Ctx.Tx().Bucket([]byte(db.RootBucket))
	if rootBucket != nil {
		if rootBucket.Bucket([]byte(entityType)) != nil {
			step.SetError(rootBucket.DeleteBucket([]byte(entityType)))
			pfxlog.Logger().Infof("removed entity type: %v", entityType)
		} else {
			pfxlog.Logger().Infof("entity type not present, don't need to remove: %v", entityType)
		}
	} else {
		step.SetError(errors.New("can't get root bbolt bucket"))
	}
}
