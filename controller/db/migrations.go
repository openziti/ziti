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
	"crypto/x509"
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/version"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
)

const (
	// LegacyDbVersion is the last numbered edge migration. Numbered migrations are frozen at a version
	// every supported release line shares; later changes are named migrations, see registerMigrations.
	LegacyDbVersion = 41

	// maxLegacyDbVersion is the highest numbered version this release line wrote. A datastore above
	// LegacyDbVersion and at or below this version ran numbered steps that are now the named migrations
	// in legacyNamedMigrations.
	maxLegacyDbVersion = 44

	// NamedMigrationsDbVersion is the edge version stored once named migrations manage a datastore. It is
	// higher than any numbered version, so a controller that predates named migrations refuses the
	// datastore instead of running numbered migrations against it.
	NamedMigrationsDbVersion = 100

	FieldVersion = "version"

	edgeComponent = "edge"
)

// legacyNamedMigrations maps each named migration that was a numbered step on this release line to the
// version that step brought a datastore to.
var legacyNamedMigrations = map[string]int{
	"session.clear-pre-1.7": 44,
}

type Migrations struct {
	stores      *Stores
	signingCert *x509.Certificate
	named       []*namedMigration
}

func newMigrations(stores *Stores, signingCert *x509.Certificate) *Migrations {
	result := &Migrations{
		stores:      stores,
		signingCert: signingCert,
	}
	result.registerMigrations()
	return result
}

// registerMigrations lists the named migrations in the order they run. A new migration is appended. A
// migration carried to another release line keeps its name and version, and its position relative to
// the migrations that line has.
func (m *Migrations) registerMigrations() {
	m.migrate("session.clear-pre-1.7", "1.7", m.clearLegacySessions)
	m.migrate("policy.role-attribute-indexes", "2.1", m.backfillPolicyRoleAttributeIndexes)
}

// migrate registers a named migration. version is the major.minor of the release line it originated on.
// It panics if name is already registered.
func (m *Migrations) migrate(name, version string, apply func(step *boltz.MigrationStep)) {
	for _, existing := range m.named {
		if existing.Name == name {
			panic(fmt.Sprintf("edge migration %q registered twice", name))
		}
	}
	m.named = append(m.named, &namedMigration{Name: name, Version: version, Apply: apply})
}

// RunMigrations brings the edge datastore up to date, then syncs the system config types. It runs on
// every startup and writes only what is out of date. Before changing an existing datastore, it saves a
// copy alongside the db file. It fails without writing if the datastore records a named migration this
// controller doesn't have.
func RunMigrations(db boltz.Db, stores *Stores, signingCert *x509.Certificate) error {
	migrations := newMigrations(stores, signingCert)

	var existing, snapshotted bool
	err := db.Update(nil, func(ctx boltz.MutateContext) error {
		var err error
		existing, snapshotted, err = migrations.apply(db, ctx)
		return err
	})
	if err != nil {
		return err
	}

	return migrations.syncSystemConfigTypes(db, existing && !snapshotted)
}

// apply brings the datastore to NamedMigrationsDbVersion with every registered migration applied. It
// reports whether the datastore already existed and whether a snapshot was taken.
func (m *Migrations) apply(db boltz.Db, ctx boltz.MutateContext) (existing bool, snapshotted bool, err error) {
	log := pfxlog.Logger().WithField("component", edgeComponent)
	tx := ctx.Tx()

	records, err := LoadAppliedMigrations(tx)
	if err != nil {
		return false, false, err
	}
	if err = m.checkAppliedAreKnown(records); err != nil {
		return false, false, err
	}
	applied := map[string]*AppliedMigration{}
	for _, record := range records {
		applied[record.Name] = record
	}

	dbVersion := ReadEdgeVersion(tx)
	if dbVersion == 0 {
		return false, false, m.initializeDatastore(ctx)
	}

	ranAsNumbered, err := legacyApplied(dbVersion)
	if err != nil {
		return true, false, err
	}

	var pending []*namedMigration
	for _, migration := range m.named {
		if applied[migration.Name] == nil && !ranAsNumbered[migration.Name] {
			pending = append(pending, migration)
		}
	}

	if dbVersion == NamedMigrationsDbVersion && len(pending) == 0 {
		log.Info("edge datastore is up to date")
		return true, false, nil
	}

	if _, _, err = db.SnapshotInTx(tx, db.GetDefaultSnapshotPath()); err != nil {
		return true, false, fmt.Errorf("failed to create bolt db snapshot (%w)", err)
	}

	if dbVersion < LegacyDbVersion {
		step := &boltz.MigrationStep{Component: edgeComponent, Ctx: ctx, CurrentVersion: dbVersion}
		m.migrateNumbered(step)
		if step.HasError() {
			return true, true, step.GetError()
		}
		log.Infof("migrated edge datastore from version %d to %d", dbVersion, LegacyDbVersion)
	}

	for _, migration := range m.named {
		if ranAsNumbered[migration.Name] && applied[migration.Name] == nil {
			appliedBy := fmt.Sprintf("%s (edge datastore version %d)", version.GetVersion(), legacyNamedMigrations[migration.Name])
			if err = m.recordApplied(tx, migration, appliedBy, time.Time{}); err != nil {
				return true, true, err
			}
		}
	}

	for _, migration := range pending {
		step := &boltz.MigrationStep{Component: edgeComponent, Ctx: ctx, CurrentVersion: dbVersion}
		migration.Apply(step)
		if step.HasError() {
			return true, true, fmt.Errorf("edge migration %q failed (%w)", migration.Name, step.GetError())
		}
		if err = m.recordApplied(tx, migration, version.GetVersion(), time.Now()); err != nil {
			return true, true, err
		}
		log.WithField("migration", migration.Name).WithField("origin", migration.Version).Info("applied edge migration")
	}

	return true, true, m.writeVersion(tx, NamedMigrationsDbVersion)
}

// initializeDatastore creates the initial data for a new datastore and records every named migration
// as applied, since initialization produces the data they migrate to.
func (m *Migrations) initializeDatastore(ctx boltz.MutateContext) error {
	step := &boltz.MigrationStep{Component: edgeComponent, Ctx: ctx}
	m.initialize(step)
	if step.HasError() {
		return step.GetError()
	}

	now := time.Now()
	for _, migration := range m.named {
		if err := m.recordApplied(ctx.Tx(), migration, version.GetVersion(), now); err != nil {
			return err
		}
	}
	return m.writeVersion(ctx.Tx(), NamedMigrationsDbVersion)
}

// checkAppliedAreKnown returns an error naming an applied migration this controller doesn't have, which
// means a newer controller migrated the datastore.
func (m *Migrations) checkAppliedAreKnown(applied []*AppliedMigration) error {
	known := map[string]struct{}{}
	for _, migration := range m.named {
		known[migration.Name] = struct{}{}
	}
	for _, record := range applied {
		if _, ok := known[record.Name]; !ok {
			return errors.Errorf("edge datastore has migration %q (from %v, applied by %v), which this "+
				"controller doesn't have. It was migrated by a newer controller", record.Name, record.Version, record.AppliedBy)
		}
	}
	return nil
}

// legacyApplied returns the named migrations a datastore at dbVersion has already run as numbered steps.
// It returns an error for a version this controller can't migrate from.
func legacyApplied(dbVersion int) (map[string]bool, error) {
	if dbVersion == NamedMigrationsDbVersion {
		return nil, nil
	}
	if dbVersion > maxLegacyDbVersion {
		return nil, errors.Errorf("Unsupported edge datastore version: %v", dbVersion)
	}
	result := map[string]bool{}
	for name, numberedVersion := range legacyNamedMigrations {
		if numberedVersion <= dbVersion {
			result[name] = true
		}
	}
	return result, nil
}

// clearLegacySessions drops the legacy service sessions, which are not compatible with controllers from
// 1.7 on. Running it again drops the sessions created since; clients request new ones.
func (m *Migrations) clearLegacySessions(step *boltz.MigrationStep) {
	m.dropEntity(step, EntityTypeSessions)
}

// migrateNumbered applies the numbered migrations to a datastore below LegacyDbVersion. Config type
// definitions are not migrated here: a change to one bumps its SystemVersion instead (see
// syncSystemConfigTypes).
func (m *Migrations) migrateNumbered(step *boltz.MigrationStep) {
	if step.CurrentVersion < 13 {
		step.SetError(errors.Errorf("Unsupported edge datastore version: %v", step.CurrentVersion))
		return
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

	if step.CurrentVersion < 23 {
		m.addProcessMultiPostureCheck(step)
	}

	if step.CurrentVersion < 24 {
		m.addIdentityIdToSessions(step)
	}

	if step.CurrentVersion < 27 {
		m.addSystemAuthPolicies(step)
	}

	if step.CurrentVersion < 29 {
		m.dropEntity(step, "geoRegions")
		m.dropEntity(step, "eventLogs")

		// migrateTerminatorIdentityFields was first added in fabric 0.19.0, which was released in 0.26.0
		// The change to remove event logs and geolocations was done in edge v0.22.7 was also released in 0.26.0
		// so we can call this in the same version update
		m.stores.internal.migrateTerminatorIdentityFields(step)
	}

	if step.CurrentVersion < 30 {
		m.dropEntity(step, EntityTypeSessions)
		m.dropEntity(step, EntityTypeApiSessions)
		m.dropEntity(step, EntityTypeApiSessionCertificates)
	}

	if step.CurrentVersion < 33 {
		m.migrateIdentityTypesToDefault(step)
	}

	if step.CurrentVersion < 36 {
		m.dropEntity(step, EntityTypeSessions)
		m.dropEntity(step, EntityTypeApiSessions)
		m.dropEntity(step, EntityTypeApiSessionCertificates)
	}

	if step.CurrentVersion < 37 {
		m.setAuthenticatorIsIssuedByNetwork(step)
	}

	if step.CurrentVersion < 41 {
		step.SetError(m.stores.Authenticator.CheckIntegrity(step.Ctx, true, func(err error, fixed bool) {
			log := pfxlog.Logger().WithError(err)
			if fixed {
				log.Info("authenticator updated during index rebuild")
			} else {
				log.Error("unfixable error during authenticator index rebuild:")
			}
		}))
	}
}

func (m *Migrations) dropEntity(step *boltz.MigrationStep, entityType string) {
	rootBucket := step.Ctx.Tx().Bucket([]byte(RootBucket))
	if rootBucket == nil {
		return
	}

	if rootBucket.Bucket([]byte(entityType)) != nil {
		step.SetError(rootBucket.DeleteBucket([]byte(entityType)))
		pfxlog.Logger().Infof("removed entity type: %v", entityType)
	} else {
		pfxlog.Logger().Infof("entity type not present, don't need to remove: %v", entityType)
	}

	indexesBucket := rootBucket.Bucket([]byte(boltz.IndexesBucket))
	if indexesBucket == nil {
		return
	}

	if indexesBucket.Bucket([]byte(entityType)) != nil {
		step.SetError(indexesBucket.DeleteBucket([]byte(entityType)))
		pfxlog.Logger().Infof("removed entity type indexes: %v", entityType)
	} else {
		pfxlog.Logger().Infof("entity type indexes not present, don't need to remove: %v", entityType)
	}
}
