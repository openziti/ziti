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
	"fmt"
	"time"

	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	versionsBucket   = "versions"
	migrationsBucket = "migrations"

	fieldMigrationVersion   = "version"
	fieldMigrationAppliedAt = "appliedAt"
	fieldMigrationAppliedBy = "appliedBy"
)

// namedMigration is a registered edge migration; see Migrations.migrate.
type namedMigration struct {
	Name    string
	Version string
	Apply   func(step *boltz.MigrationStep)
}

// AppliedMigration is the record of a named migration applied to the edge datastore.
type AppliedMigration struct {
	Name string
	// Version is the major.minor of the release line the migration originated on.
	Version string
	// AppliedAt is when the migration was applied, or zero when that isn't known.
	AppliedAt time.Time
	// AppliedBy is the version of the controller that applied the migration. For a migration that ran as
	// a numbered step, it is the version of the controller that recorded it, followed by the numbered
	// datastore version that step brought the datastore to.
	AppliedBy string
}

// ReadEdgeVersion returns the stored edge datastore version, or 0 for a new datastore.
// NamedMigrationsDbVersion means the datastore is managed by named migrations, which
// LoadAppliedMigrations lists.
func ReadEdgeVersion(tx *bbolt.Tx) int {
	if versions := boltz.Path(tx, RootBucket, versionsBucket); versions != nil {
		if version := versions.GetInt64(edgeComponent); version != nil && *version != 0 {
			return int(*version)
		}
	}
	return legacyRootVersion(tx)
}

// legacyRootVersion returns the version kept in the root bucket by datastores that predate the
// versions bucket, or 0.
func legacyRootVersion(tx *bbolt.Tx) int {
	root := boltz.Path(tx, RootBucket)
	if root == nil {
		return 0
	}
	if rootVersion := root.GetInt64(FieldVersion); rootVersion != nil {
		return int(*rootVersion)
	}
	return 0
}

// LoadAppliedMigrations returns the named migrations recorded as applied to the edge datastore, in
// name order. A datastore at a numbered version has none.
func LoadAppliedMigrations(tx *bbolt.Tx) ([]*AppliedMigration, error) {
	records := boltz.Path(tx, RootBucket, migrationsBucket, edgeComponent)
	if records == nil {
		return nil, nil
	}

	var result []*AppliedMigration
	cursor := records.Cursor()
	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		if value != nil {
			return nil, fmt.Errorf("unexpected value under edge migration record %q", string(key))
		}
		record := records.GetBucket(string(key))
		applied := &AppliedMigration{
			Name:      string(key),
			Version:   record.GetStringWithDefault(fieldMigrationVersion, ""),
			AppliedBy: record.GetStringWithDefault(fieldMigrationAppliedBy, ""),
		}
		if appliedAt := record.GetTime(fieldMigrationAppliedAt); appliedAt != nil {
			applied.AppliedAt = *appliedAt
		}
		result = append(result, applied)
	}
	return result, nil
}

// recordApplied records migration as applied to the edge datastore by appliedBy, replacing any record
// with the same name. A zero appliedAt is left unrecorded. tx must be writable.
func (m *Migrations) recordApplied(tx *bbolt.Tx, migration *namedMigration, appliedBy string, appliedAt time.Time) error {
	record := boltz.GetOrCreatePath(tx, RootBucket, migrationsBucket, edgeComponent, migration.Name)
	record.SetString(fieldMigrationVersion, migration.Version, nil)
	record.SetString(fieldMigrationAppliedBy, appliedBy, nil)
	if !appliedAt.IsZero() {
		record.SetTime(fieldMigrationAppliedAt, appliedAt, nil)
	}
	return record.GetError()
}

// writeVersion stores version as the edge datastore version. tx must be writable.
func (m *Migrations) writeVersion(tx *bbolt.Tx, version int) error {
	versions := boltz.GetOrCreatePath(tx, RootBucket, versionsBucket)
	versions.SetInt64(edgeComponent, int64(version), nil)
	return versions.GetError()
}
