# Service-collapse migration: one-time validation

This directory holds the tooling and runbook for validating the fabric/edge
**service-collapse migration** (`collapseEdgeServices`) before it ships.

## Why this is a one-time, manual gate

The collapse migration is **run-once** code: it fires when a controller is
upgraded from a pre-collapse version to this one, moves each edge service's
fields and FK sub-buckets out of its `edge` child bucket up to the top level,
sets `isFabricOnly`, then **deletes** the edge sub-bucket and the old edge index.
It is forward-only and destructive, and once an install is past it the code never
runs again.

So the durable guarantee we need is "it was correct the one time it ran on real
data." That is what this runbook certifies. The in-CI tests in
`controller/db/migration_collapse_fabric_edge_services_test.go` (forward, hand
crafted) and `..._roundtrip_test.go` (round-trip via the real store) guard the
migration *logic* while the branch is in flight, but they run against synthetic
input. This runbook validates against data written by the **actual pre-collapse
binary**, which is the only thing that can certify the real on-disk layout.

> The two in-CI migration tests are scaffolding. After this gate passes and the
> migration is frozen, delete them before merge (the durable, long-lived coverage
> is `Test_FabricOnlyServiceBehavior` / `Test_FabricOnlyPolicyExclusion`, which
> test permanent fabric/edge behavior, not the migration).

## The tool

Build it from this branch (post-collapse):

```
cd zititest && go build -o /tmp/migration-test ./migration-test/
```

Subcommands (all operate on a bolt db **file**, so the controller must be
stopped — bolt takes an exclusive lock — and you should always run on a **copy**):

- `create-model.sh` — create the representative data model **through the `ziti`
  CLI / REST API** against a running pre-collapse controller. This is the truthful
  generator: it runs the same validation, defaulting, config-schema,
  policy-denormalization, and event paths a real install does. Its header
  documents the coverage matrix (every link cardinality 0/1/2, both refcount
  values, index multiplicities, encryption variants, fabric services). (There is
  intentionally no direct-store `populate` tool — store writes would bypass the
  validation/denorm paths that make the data and its migration realistic.)
- `migration-test query <db> <out.json>` — open the db (migrating it if it
  predates the collapse), then write a deterministic JSON snapshot: services
  (edge/fabric), their FK links, forward+reverse denorm **refcounts**,
  `identityServices`, configs (incl. `identityServices`), policies, SERPs, edge
  routers, and direct role-attribute **index** reads.
- `migration-test verify <db>` — open + migrate, then run the structural gate:
  `CheckIntegrity(fix=false)` is clean, the old `ziti/indexes/services/edge`
  bucket is gone, and re-running the migration is a no-op (idempotency). Prints
  the edge/fabric service tally and exits non-zero on any problem.

## Authentic validation procedure (real upgrade)

Use a controller built from a **pre-collapse** version (e.g. current `main`) and
one built from **this branch**. Work on a copy of the db; never the live file.

1. **Stand up a pre-collapse controller** (`ziti edge quickstart --home <dir>
   --no-router --ctrl-address localhost` is fine) and create a representative
   dataset **through the `ziti` CLI / REST API** by running `create-model.sh`
   (`ZITI=<branch-or-main-ziti> ZITI_CTRL=localhost:1280 ./create-model.sh`). Its
   coverage matrix exercises every link cardinality (0/1/2), both denorm refcount
   values, the role-attr index at 1 and 2+ members, both encryption settings, and
   fabric/management services. Using the API is the point: it runs the same
   validation, defaulting, config-schema, policy-denormalization, and event paths
   a real install does, so the data — and any migration problem it triggers — is
   realistic rather than an artifact of hand-built store entities.
2. **Capture the pre-migration baseline via the API** (controller running — the
   db file is locked): `ziti edge list services`, `service-policies`,
   `service-edge-router-policies`, `identities`, `configs`, `edge-routers`, and
   spot-check identity dial/bind access. Save the output. This is the oracle.
3. **Stop the pre-collapse controller** cleanly (consistent db, lock released)
   and **copy** its bolt db.
4. **Start a controller from this branch** against the db copy. It runs the
   collapse migration on startup. Confirm in the log: the
   `service collapse migration: migrated N edge ... M fabric-only` line, and
   **zero errors**.
5. **Capture the post-migration baseline via the API** (branch controller
   running): same `ziti edge list ...` commands. Compare to step 2 — every edge
   service present with identical fields/configs/policy membership/access;
   fabric-only services now carry `isFabricOnly` and remain visible via the
   fabric API but **absent from the edge service list**.
6. **Stop the controller** and run the structural gate on the db copy:
   ```
   migration-test verify <db-copy>
   migration-test query  <db-copy> post.json   # detailed snapshot, for the record
   ```
   `verify` must print `VERIFY OK`.

### Acceptance criteria

- Migration log shows the collapse ran with **no errors**.
- Step 2 (pre) and step 5 (post) logical state match (normalizing for
  pre-collapse separate edge/fabric stores vs the `isFabricOnly` partition).
- `migration-test verify` reports `VERIFY OK` (CheckIntegrity clean, edge index
  removed, idempotent).
- Record `post.json` + the `ziti edge list` captures as evidence.

## Notes / gotchas

- **bolt exclusive lock**: any file-level command (`query`, `verify`) needs the
  controller stopped. Capture live state via the `ziti` API instead.
- **`query`/`verify` migrate on open**: they apply the collapse to a pre-collapse
  db themselves, so they are post-migration tools — run them on a **copy**, and
  do not expect them to show you the pre-migration state.
- **Versioning**: the collapse is the latest edge migration; its number moves as
  other migrations land upstream. This runbook and the tool are written
  version-agnostically on purpose — there is nothing here to renumber.
