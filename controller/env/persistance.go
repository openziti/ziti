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

package env

import (
	"database/sql"
	"fmt"
	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database"
	"github.com/golang-migrate/migrate/database/postgres"
	"github.com/golang-migrate/migrate/source"
	_ "github.com/golang-migrate/migrate/source/file"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/config"
	"github.com/netfoundry/ziti-edge/controller/packrsource"
	"github.com/netfoundry/ziti-edge/migration/certOLD"
	"github.com/netfoundry/ziti-edge/migration/updbOLD"
	"net/url"
)

const PackrPrefix = "embedded"
const MigrationPathPostgres = "embedded://db/postgres/migrations"

type DriverGenerator interface {
	AppDriver(cs string) (*gorm.DB, error)
	MigrationDriver(cs string) (database.Driver, error)
	Name() string
}

type PostgresDriverGenerator struct {
}

func (PostgresDriverGenerator) Name() string {
	return "postgres"
}

func (pg *PostgresDriverGenerator) AppDriver(cs string) (*gorm.DB, error) {
	appCs, err := pg.appString(cs)

	if err != nil {
		return nil, err
	}

	return gorm.Open("postgres", appCs)
}

func (pg *PostgresDriverGenerator) MigrationDriver(cs string) (database.Driver, error) {
	migCs := pg.migrationString(cs)

	s, err := sql.Open("postgres", migCs)

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("error during sql.Open")
	}
	return postgres.WithInstance(s, &postgres.Config{
		MigrationsTable: "ziti_edge_version",
	})
}

func (pg *PostgresDriverGenerator) appString(s string) (string, error) {
	u, err := url.Parse(s)

	if err != nil {
		return "", fmt.Errorf("connection string must be a well formatted URL: %s", err)
	}

	q := u.Query()
	q.Set("search_path", "ziti_edge,public")

	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (pg *PostgresDriverGenerator) migrationString(s string) string {
	return s
}

func InitPersistence(p *config.Persistence) *gorm.DB {
	if p == nil || p.Postgres.ConnectionUrl == "" {
		return nil
	}

	for _, migration := range certOLD.GetCertMigrations() {
		p.AddMigrationBox(migration)
	}

	for _, migration := range updbOLD.GetUpdbMigrations() {
		p.AddMigrationBox(migration)
	}

	source.Register(PackrPrefix, packrsource.NewPakrSource(p.GetBoxes()...))

	log := pfxlog.Logger()

	var dg DriverGenerator
	var cs string

	switch {
	case p.Postgres != nil:
		dg = &PostgresDriverGenerator{}
		cs = p.Postgres.ConnectionUrl
	default:
		log.Fatal("no supported persistence configuration defined")
	}

	md, err := dg.MigrationDriver(cs)

	if err != nil {
		log.Fatalf("failed to connect to data store (%s) to check schema version: %+v", dg.Name(), err)
	}

	defer func() {
		err := md.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	ad, err := dg.AppDriver(cs)

	ad.LogMode(true)
	ad.SetLogger(&logrusLogger{})

	if err != nil {
		log.Fatal(err)
	}

	migrateDb(p, md)

	return ad
}

func migrateDb(p *config.Persistence, drv database.Driver) {
	log := pfxlog.Logger()

	m, err := migrate.NewWithDatabaseInstance(MigrationPathPostgres, p.Postgres.DbName, drv)

	if err != nil {
		log.Fatal("could not configure migration client: ", err)
	}

	log.Info("checking database schema version...")

	defer func() {
		err := recover()
		if err != nil {
			log.Fatalf("Failure while migrating database: %+v", err)
		}
	}()
	err = m.Up()

	if err != nil {
		if err != migrate.ErrNoChange {
			log.Fatal("...could not migrate database: ", err)
		}
		log.Info("...no database upgrade required")
	} else {
		log.Info("...database upgraded")
	}
}

type logrusLogger struct{}

func (logrusLogger) Print(v ...interface{}) {
	pfxlog.Logger().WithField("type", "sql").Trace(v...)
}
