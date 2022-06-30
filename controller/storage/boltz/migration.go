package boltz

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"go.etcd.io/bbolt"
)

type MigrationStep struct {
	errorz.ErrorHolderImpl
	Component      string
	Ctx            MutateContext
	CurrentVersion int
}

type Migrator func(step *MigrationStep) int

type MigrationManager interface {
	Migrate(component string, targetVersion int, migrator Migrator) error
}

func NewMigratorManager(db Db) MigrationManager {
	migrator := &migrationManager{
		db: db,
	}
	return migrator
}

type migrationManager struct {
	db Db
}

func (m *migrationManager) Migrate(component string, targetVersion int, migrator Migrator) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		rootBucket, err := m.db.RootBucket(tx)
		if err != nil {
			return err
		}
		typedBucket := newRootTypedBucket(rootBucket)
		versionsBucket := typedBucket.GetOrCreateBucket("versions")
		if versionsBucket.HasError() {
			return versionsBucket.GetError()
		}
		versionP := versionsBucket.GetInt64(component)
		version := 0
		if versionP != nil {
			version = int(*versionP)
		}

		if versionP != nil && version != targetVersion {
			if err := m.db.Snapshot(tx); err != nil {
				return fmt.Errorf("failed to create bolt db snapshot: %w", err)
			}
		}

		ctx := NewMutateContext(tx)
		for version != targetVersion {
			step := &MigrationStep{
				Component:      component,
				Ctx:            ctx,
				CurrentVersion: version,
			}
			newVersion := migrator(step)
			if step.HasError() {
				return step.GetError()
			}
			if version != newVersion {
				versionsBucket.SetInt64(component, int64(newVersion), nil)
				if versionsBucket.HasError() {
					return versionsBucket.GetError()
				}
				pfxlog.Logger().Infof("Migrated %v datastore from %v to %v", component, version, newVersion)
				version = newVersion
			}
		}
		pfxlog.Logger().Infof("%v datastore is up to date at version %v", component, version)
		return nil
	})
}
