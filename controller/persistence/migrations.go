/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldVersion   = "version"
	currentVersion = 2
)

type Migrations struct {
	dbVersion     uint32
	versionBucket *boltz.TypedBucket
}

type MigrationContext struct {
	Ctx      boltz.MutateContext
	Stores   *Stores
	DbStores *migration.Stores
}

func RunMigrations(dbProvider DbProvider, stores *Stores, dbStores *migration.Stores) error {
	migrations := &Migrations{}
	return dbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		mtx := &MigrationContext{
			Ctx:      ctx,
			Stores:   stores,
			DbStores: dbStores,
		}
		return migrations.run(mtx)
	})
}

func (m *Migrations) run(mtx *MigrationContext) error {
	m.versionBucket = boltz.GetOrCreatePath(mtx.Ctx.Tx(), boltz.RootBucket)
	if m.versionBucket.Err != nil {
		return m.versionBucket.Err
	}
	version := m.versionBucket.GetInt64(FieldVersion)
	if version == nil {
		m.dbVersion = 0
	} else {
		m.dbVersion = uint32(*version)
	}
	pfxlog.Logger().Infof("bolt storage at version %v", m.dbVersion)
	var migrations bool
	baseVersion := m.dbVersion
	if m.dbVersion == 0 {
		migrations = true
		m.createDefaultData(mtx)

		if mtx.DbStores != nil {
			m.upgradeToV1FromPG(mtx)
		} else {
			pfxlog.Logger().Info("no postgres configured, skipping migration from postgres")
		}
		m.setVersion(1)
	}

	if m.dbVersion == 1 {
		// Only want to migrate existing database, if it's a fresh DB, leave it alone
		if baseVersion == 1 {
			migrations = true
			m.upgradeToV2FromV1(mtx)
		}
		m.setVersion(2)
	}

	if migrations {
		pfxlog.Logger().Infof("bolt storage at version %v", m.dbVersion)
	}

	if m.versionBucket.Err == nil && m.dbVersion != currentVersion {
		return errors.Errorf("unable to migrate from schema version %v to %v", m.dbVersion, currentVersion)
	}

	return m.versionBucket.Err
}

func (m *Migrations) setVersion(version uint32) {
	m.versionBucket.SetInt64(FieldVersion, int64(version), nil)
	if m.versionBucket.Err == nil {
		m.dbVersion = version
	}
}

func (m *Migrations) createDefaultData(mtx *MigrationContext) {
	creators := []func(mtx *MigrationContext) error{
		createGeoRegionsV1,
		createIdentityTypesV1,
	}

	for _, creator := range creators {
		if err := creator(mtx); err != nil {
			m.versionBucket.SetError(err)
		}
	}
}

func (m *Migrations) upgradeToV1FromPG(mtx *MigrationContext) {
	if err := upgradeToV1FromPG(mtx); err != nil {
		m.versionBucket.SetError(err)
	}
}

func (m *Migrations) upgradeToV2FromV1(mtx *MigrationContext) {
	if err := createEdgeRouterPoliciesV2(mtx); err != nil {
		m.versionBucket.SetError(err)
	}
}
