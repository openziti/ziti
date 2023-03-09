package boltz

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
)

type MigrationStep struct {
	errorz.ErrorHolderImpl
	Component      string
	Ctx            MutateContext
	CurrentVersion int
}

type Migrator func(step *MigrationStep) int

type MigrationManager interface {
	GetComponentVersion(component string) (int, error)
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

func (m *migrationManager) GetComponentVersion(component string) (int, error) {
	version := 0
	err := m.db.Update(nil, func(ctx MutateContext) error {
		rootBucket, err := m.db.RootBucket(ctx.Tx())
		if err != nil {
			return err
		}
		typedBucket := newRootTypedBucket(rootBucket)
		versionsBucket := typedBucket.GetOrCreateBucket("versions")
		if versionsBucket.HasError() {
			return versionsBucket.GetError()
		}
		versionP := versionsBucket.GetInt64(component)
		if versionP != nil {
			version = int(*versionP)
		}

		return nil
	})

	return version, err
}

func (m *migrationManager) Migrate(component string, targetVersion int, migrator Migrator) error {
	return m.db.Update(nil, func(ctx MutateContext) error {
		rootBucket, err := m.db.RootBucket(ctx.Tx())
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
			if err := m.db.Snapshot(ctx.Tx()); err != nil {
				return fmt.Errorf("failed to create bolt db snapshot: %w", err)
			}
		}

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
