# Changing the edge datastore

The controller changes what's stored in its edge datastore in two ways, and the right one depends on what
you're changing:

- **Built-in config types** (`host.v1`, `intercept.v1` and the rest) are synced on every startup. Changing
  one is a version bump, not a migration.
- **Everything else** goes through named migrations, which run once per datastore.

There's also an older mechanism, numbered migrations. Those are frozen. Don't add to them.

## Changing a built-in config type

The built-in config types are listed in `systemConfigTypes` (`config_type_sync.go`), and
`syncSystemConfigTypes` brings the stored types in line with them on every startup. Each definition carries
a `SystemVersion`. A stored type with a lower version gets the definition's schema.

To change a schema:

1. Edit the schema and bump the definition's `SystemVersion`.
2. Run `Test_SystemConfigTypeDefinitions`. It fails and prints the new schema's hash. Add that to
   `systemConfigTypeSchemaHashes` under the new version, and leave the existing entries alone.

To add a built-in config type, add its definition to `systemConfigTypes` at version 1, and record its hash
the same way.

A version number names exactly one schema, on every release line. Versions are bumped on main, and a
backport copies main's schema, version and hash as they are. Schema changes have to be backward
compatible. If yours isn't, it's a new config type.

## Adding a migration

Migrations are registered in `registerMigrations` (`migrations.go`):

```go
m.migrate("session.clear-pre-1.7", "1.7", m.clearLegacySessions)
```

The first argument is the migration's name, `<entity>.<change>` by convention. The second is the
major.minor of the release line it was written for. The function gets a `*boltz.MigrationStep` and reports
failure with `step.SetError`.

On startup, the controller runs each registered migration the datastore hasn't recorded, in registration
order, all in one transaction, after saving a snapshot beside the datastore file. If one fails, the
datastore is left as it was. Each one that succeeds is recorded under its name, along with the controller version that ran it.
`ziti ops db get-db-version` lists them.

### The rules

**Append.** New migrations go at the end of `registerMigrations`. Once a migration has shipped, nothing gets
inserted in front of it. A datastore that already ran it would run the newcomer afterwards, a newer one
would run it before, and you'd have two histories in the field.

**Names are permanent.** A controller refuses to start on a datastore that records a migration it doesn't
have, since that means a newer controller has migrated it. Renaming or removing a released migration trips
that check on every datastore that ran it. A migration that's become obsolete stays registered.

**A migration can run twice.** A migration has to leave a datastore it has already migrated as it is. A
datastore can reach a migration with some of the work already done, for example one converted from a
numbered version that ran an earlier form of it. `Test_NamedMigrationsRerunWithoutChange` runs every
migration a second time against a populated datastore and fails if anything changes, so make sure its
fixtures include the entities your migration touches. If a migration really can't be made safe to run
again, it goes in `rerunChangesData` with the reason that's acceptable. That should be rare.

**New datastores skip migrations.** A new datastore is built by `initialize` and records every migration as
applied without running any of them. If your migration adds data, such as a new built-in identity type or
auth policy, `initialize` has to add it too. A migration that only reshapes existing data needs nothing
there.

**Test it.** Set up the data the migration expects, run it, and check the result.

### Backports

A backported migration keeps its name and version. Register it in the same position relative to the
migrations the release line already has, so it runs in the same order everywhere. A datastore that ran it on
the release line doesn't run it again after upgrading.

### Numbered migrations

The numbered migrations (`migrateNumbered`) are frozen at `LegacyDbVersion`, the last version every
supported release line shares. The numbered steps a release line had above that are now named migrations,
and `legacyNamedMigrations` maps each to the numbered version that included it, so a datastore converted
from a numbered version records them as applied instead of running them. Neither gets new entries.

## Going backwards

Migrations only go forward. To go back to an earlier release, restore the snapshot the controller saved
before migrating. A datastore with named migrations stores version 100 (`NamedMigrationsDbVersion`), which
controllers from before named migrations refuse to open.

`ziti ops db set-db-version` is for development and testing. It sets the edge version to a numbered version
from `LegacyDbVersion` through `MaxLegacyDbVersion`, which can let an older controller open the datastore.
It doesn't undo anything: the applied migrations stay recorded, and the data stays migrated.
