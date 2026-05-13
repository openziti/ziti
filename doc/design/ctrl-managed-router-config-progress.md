# Controller-Managed Router Config: MVP Implementation Plan

## Context

The design doc (`doc/design/ctrl-managed-router-config.md`) describes managing router configuration
from the controller via the existing config type infrastructure. The MVP targets `router.link.v1` to
prove the full pipeline end-to-end. This plan covers the implementation phases in dependency order.

The work spans two repos:
- `edge-api` at `/home/plorenz/work/4/edge-api` (OpenAPI spec + generated models)
- `ziti` at `/home/plorenz/work/4/ziti` (everything else)

---

## Phase 1a: Add `target` field to ConfigType -- DONE

**Goal**: Distinguish router config types from service config types.

**Status**: Complete. Branch `config-type-target`.

### What was implemented

**DB store** (`controller/db/config_type_store.go`):
- `Target *string` field on `ConfigType` struct
- Constants `ConfigTypeTargetService` and `ConfigTypeTargetRouter`
- Symbol, FillEntity, PersistEntity for the new field

**Model** (`controller/model/config_type_model.go`):
- `Target *string` field, wired through `toBoltEntity` and `fillFrom`

**Manager** (`controller/model/config_type_manager.go`):
- `allowedFieldsChecker` excludes `target` from updates (immutable after creation)
- `ApplyUpdate` uses `AndFieldChecker` to enforce the whitelist
- Marshall/Unmarshall updated for raft replication

**Protobuf**:
- `optional string target = 5` in `edge_cmd_pb.ConfigType` (raft replication)
- `optional string target = 3` in `edge_ctrl_pb.DataState.ConfigType` (RDM)
- `newConfigType()` in `sync_instant.go` includes `Target`

**REST API** (`controller/internal/routes/config_type_api_model.go`):
- `Target` set on create and detail response
- Not included in update or patch (immutable)

**edge-api** (`source/management/config-types.yml`):
- `target` field on `configTypeCreate` and `configTypeDetail` (nullable string, enum: service/router)
- Not on `configTypeUpdate` or `configTypePatch`

**CLI** (`ziti/cmd/edge/`):
- `create config-type`: `--target` flag
- `list config-types`: `Target` column in output

**Built-in config types** (`controller/db/migration_initialize.go`):
- All 8 built-in types set to `target = "service"`

**Migration** (`controller/db/migration_v45.go`):
- Iterates all config types, sets `target = "service"` on any without a target
- Covers built-in and user-created types

**Validation**:
- Services require configs with `target = "service"` (`edge_service_model.go`)
- Identity service config overrides require `target = "service"` (`identity_manager.go`)
- Nil target is rejected for both

**Integration tests** (`tests/`):
- Config type CRUD with nil, service, router, and invalid targets
- Immutability verification on update and patch
- Service config target validation (nil/service/router)
- Identity service config override target validation (nil/service/router)

---

## Phase 1b: Add `configs` field to Router -- DONE

**Goal**: Let routers reference config instances, with one-config-per-type enforcement.

**Status**: Complete. Commit `d870f75` (issue #3780), with follow-ups for stricter validation
and config-delete cleanup.

### What was implemented

The `Configs` field lives on the base `Router`, not on `EdgeRouter`/`TransitRouter`. This keeps
the field reachable from the fabric router endpoints as well, and avoids having to plumb it
through the child stores separately. EdgeRouter and TransitRouter inherit it via their embedded
`Router` struct.

**DB store** (`controller/db/router_store.go`, `config_store.go`):
- `Configs []string` on the base `Router` struct.
- `symbolConfigs` (FK set) on `routerStoreImpl`, with the bidirectional link collection
  `router.configs <-> config.routers` registered in both stores' `initializeLinked()`.
- `FillEntity` reads via `bucket.GetStringList(EntityTypeConfigs)`.
- `PersistEntity` writes via `ctx.SetLinkedIds(EntityTypeConfigs, entity.Configs)`.
- `configStoreImpl.DeleteById` rewrites `router.Configs` and re-`Update`s every referencing
  router (mirroring the existing service loop) so future entity-update listeners see the
  change.

**Model** (`controller/model/router_model.go`, `edge_router_model.go`, `transit_router_model.go`):
- `Configs []string` field on `Router`, `EdgeRouter`, `TransitRouter`.
- Single `validateRouterConfigs(tx, env, configs, checker)` helper, called from each router
  type's `toBoltEntityForCreate`/`toBoltEntityForUpdate`.
- Validation is **strict**: every config must resolve to an existing config type, and that
  type's `Target` must equal `db.ConfigTypeTargetRouter`. Unlike the edge-service path
  (which tolerates a missing config type), routers reject the reference outright.
- Validation is skipped when `checker` is non-nil and the `configs` field is not in the
  field set being updated. This avoids re-loading every config and config type on unrelated
  updates (e.g. `UpdateCtrlChanListeners`).
- Same one-config-per-type rule as services.
- `fillFrom` reads `Configs` from the bolt entity.

**Manager** (`controller/model/router_manager.go`, `edge_router_manager.go`, `transit_router_manager.go`):
- `EntityTypeConfigs` added to the allowed-fields whitelist for edge and transit router
  PATCH paths.
- Marshall/Unmarshall (raft replication) carry `Configs` through `cmd_pb.Router`,
  `edge_cmd_pb.EdgeRouter`, and `edge_cmd_pb.TransitRouter`.
- `RouterManager.UpdateCachedRouter` refreshes the cached router's `Configs`.

**Wire formats** (`cmd.proto`, `edge_cmd.proto`, `swagger.yml`, `rest_model/router_*.go`):
- `repeated string configs` added to `Router`, `EdgeRouter`, `TransitRouter` proto messages.
- `configs` array added to `routerCreate`, `routerUpdate`, `routerPatch`, `routerDetail`
  swagger schemas, and the corresponding edge-api edge-router schemas.

**REST mappers** (`controller/internal/routes/{edge,fabric,router}_router_api_model.go`):
- `Configs` populated on map-to-model and model-to-REST in all directions.

**CLI** (`ziti/cmd/edge`, `ziti/cmd/fabric`):
- `--config` repeatable flag added to `create`/`update` commands for edge, transit, and
  fabric routers.

**Integration tests** (`tests/edge_router_test.go`, `fabric_router_test.go`, `entities.go`):
- `Test_EdgeRouterConfigs`, `Test_TransitRouterConfigs`, `Test_FabricRouterConfigs` cover
  create/update/patch/clear with router-target configs, plus rejection of service-target
  configs and duplicate-type configs.

---

## Phase 1c: Define `router.link.v1` config type and schema -- DONE

**Goal**: Create the built-in `router.link.v1` config type with a JSON Schema.

**Status**: Complete.

### What was implemented

**Type definition** (`controller/db/migration_initialize.go`):
- `RouterLinkV1TypeId = "router.link.v1"` constant.
- `routerLinkV1ConfigType` `var` with `Target = ConfigTypeTargetRouter` and a
  JSON Schema covering listeners, dialers, heartbeats, queue sizes, channel
  options, and backoff settings. `additionalProperties: false` everywhere
  to surface typos.

**Schema notes**:
- `listener.binding` and `dialer.binding` are optional with documented default
  `"transport"`. Phase 4b's link config handler is responsible for substituting
  the default at apply time. JSON Schema's `default` keyword does not enforce.
- `dialer.bindInterface` (renamed from the YAML's `dialer.bind`) for parity
  with `listener.bindInterface`. The YAML loader still reads `bind`; the
  config handler in Phase 4b will reconcile.
- `channel.Options` is modeled prescriptively. When `openziti/channel` adds or
  changes a field on the loaded shape, the schema must be updated alongside it
  and the version bumped if the change is breaking.
- `connectTimeout` is a duration string in the schema, not the milliseconds
  integer that `channel.LoadOptions` reads (`connectTimeoutMs`). Phase 4b
  converts before calling `LoadOptions`.
- Dropped from the v1 schema: `listener.costTags` (unused in practice) and
  `dialer.split` (only consulted as fallback when the peer doesn't support
  multi-underlay; v1 routers always use multi-underlay).

**Registration**:
- `migration_initialize.go` `initialize()` calls
  `m.createConfigType(step, routerLinkV1ConfigType)` for fresh installs.
- `migrations.go` `CurrentDbVersion` bumped 45 -> 46. New v46 step calls
  `m.createOrUpdateConfigType(step, routerLinkV1ConfigType)` for upgrades.

**Tests** (`controller/db/config_type_store_test.go` `Test_RouterLinkV1Builtin`):
- Asserts existence, identity, and `Target = router`.
- Compiles the schema via `gojsonschema` (catches malformed `$ref`/`oneOf`
  before any user can hit them via config-create).
- Validates a representative known-good payload.
- Verifies binding is optional on both listener and dialer.
- Confirms rejection of: extra top-level key, listener missing `bind`,
  `maxDefaultConnections: 0`, `retryBackoffFactor: 0.5`, `groups` of wrong
  type, `channelOptions.maxQueuedConnects: 0`.

---

## Phase 2a: Add Router to DataState protobuf -- DONE

**Goal**: Let the RDM carry router entity events.

**Status**: Complete.

### What was implemented

In `common/pb/edge_ctrl_pb/edge_ctrl.proto`:

- New nested message:
  ```protobuf
  message Router {
    string id = 1;
    string name = 2;
    string fingerprint = 3;
    repeated string configs = 4;
    bool disabled = 5;
  }
  ```
- New `Router router = 19` entry in the `Event.Model` oneof.
- Regenerated `edge_ctrl.pb.go` via `go generate`.

### Field choices

Minimal set — only fields with a clear router-side consumer:

- `id`, `fingerprint`, `configs` — link-peer validation and config dispatch.
- `name` — diagnostics.
- `disabled` — lets a router refuse incoming links from peers the controller
  has marked disabled (router-side enforcement is a later phase, but the wire
  field is in place now to avoid a future proto bump).

Deliberately omitted: `cost`, `noTraversal`. Routers consume those today via
`PeerStateChanges`; nothing in the RDM consumer path needs them. Additive-only
within v1 means we can add later if a use case appears.

### What this phase did NOT touch

- `RouterDataModelSender.Handle()` — the type-switch silently ignores the new
  `Router` variant until Phase 2b wires it.
- Router-side RDM `Handle` — same; Phase 3c wires it.
- No new tests — there's no logic to exercise yet, just a wire format.

---

## Phase 2b: Populate routers in RouterDataModelSender -- DONE

**Goal**: Load routers into the RDM and generate events on changes.

**Status**: Complete.

### What was implemented

**`common/router_data_model_sender.go`**:
- New `Routers cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Router]` field,
  initialized in `NewRouterDataModelSender`.
- `Handle()` switch extended with a `*edge_ctrl_pb.DataState_Event_Router` case
  dispatching to `HandleRouterEvent`.
- `HandleRouterEvent` mirrors the simple Set/Remove pattern of
  `HandleConfigEvent` / `HandleServiceEvent`.
- `getDataStateAlreadyLocked` (full snapshot builder) emits a `Create` event
  for every cached router.
- `GetEntityCounts` adds a `routers` entry.

**`controller/sync_strats/sync_instant.go`**:
- New `routerHandler` constraint registered on the **base `Router` store** in
  `Initialize()`. Subscribing on the parent (not edge/transit child stores)
  catches changes from any of the three router APIs because child store
  persistence delegates to the parent's `PersistEntity`. Verified by the
  existing `routerStore.AddEntityIdListener` use in `router_manager.go`.
- `RouterCreate` / `RouterUpdate` / `RouterDelete` thin dispatchers to
  `handleRouter(index, action, entity)`.
- `handleRouter` builds the `DataState_Event_Router` and calls
  `addToChangeSet`.
- `BuildRouters(index, tx, rdm)` iterates the `Router` store and dispatches a
  `Create` event per row into the RDM. Wired into `BuildAll` after
  `BuildPostureChecks`.
- `newRouter(*db.Router)` and `newRouterById(tx, ae, id)` helpers, mirroring
  `newService`/`newServiceById`. `newRouter` flattens the `*string`
  fingerprint to `""` for un-enrolled routers (the wire field is non-nullable
  `string`).

### Field projection notes

- `db.Router.Fingerprint` is `*string` on the controller side but flat
  `string` on the wire. Empty string represents "not yet enrolled". Phase 3c's
  link-peer validation logic will need to handle this case (skip vs reject).
- `cost` and `noTraversal` are not propagated (per the field-set decision in
  Phase 2a).

### Validation

`ValidateRouters` follows the existing pattern of `ValidateServices` /
`ValidateConfigs`, comparing the controller's `Router` store rows against
`rdm.Routers` on name, fingerprint, disabled, and configs. To accommodate
`RouterStore` (which embeds `boltz.EntityStore[*Router]` directly rather than
the local `db.Store[E]`), the `ValidateType` generic was relaxed from
`db.Store[T]` to `boltz.EntityStore[T]`. The helper only uses `IterateIds` /
`LoadById`, both of which live on `boltz.EntityStore`, so the looser bound
is strictly more permissive — every existing caller still satisfies it.

### Deliberately not in this phase

- **No filtering**. `Router` events broadcast to all connected routers, and
  the snapshot includes all routers. Per-router config filtering is Phase 2c.
- **No router-side handling**. Phase 3c teaches the router-side RDM to apply
  `Router` events.

---

## Phase 2c: Per-router config filtering in RouterSender -- DONE

**Goal**: Each router only receives its own configs, but all router entities.

**Status**: Complete.

The motivation is blast radius (per-router secrets shouldn't leak to peers), not memory — for
realistic networks the in-memory RDM is well under 1 GB per router. We do not filter
identities, services, policies, or router entities; only `Config` events get scoped per-router.

### What was implemented

All filtering lives in `controller/sync_strats/rtx.go`. The shared `RouterDataModelSender`
remains a global event log; each `RouterSender` filters events for its own connected router as
they leave.

**Filter** (`RouterSender.filterEventsForRouter`):
- `*edge_ctrl_pb.DataState_Event_Config`: keep iff the receiving router's Configs list (read
  from `rtx.routerDataModel.Routers.Get(rtx.Router.Id).Configs`) contains the config ID.
  Reading from the RDM cache instead of `rtx.Router.Configs` avoids a race: the RDM cache is
  updated synchronously by `Handle()` *before* `sendEvent` fans out to listeners, while the
  `*model.Router` cache update via `UpdateCachedRouter` is a separate post-commit hook with
  no ordering guarantee.
- `*edge_ctrl_pb.DataState_Event_Router` for any router: pass through.
- For the **self**-Router event (action != Delete) when `synthesizeMissing == true`
  (delta path): before the Router event, append a synthetic `Config Create` event for each
  config currently in `Router.Configs`, sourced from `rtx.routerDataModel.Configs`. Marked
  `IsSynthetic: true`. This guarantees the receiving router has the entity data alongside the
  assignment, even if it had previously filtered the config out.
- All other event types: pass through.

The filter does a fast pre-scan and returns `(events, false)` unchanged when nothing needs
to be filtered or synthesized. The vast majority of change sets fall into this case (most
events have no Config payload, or the configs all belong to the receiving router). The
caller uses the `changed` flag to skip allocating a per-router change-set rebuild on the
delta path, sending the original change-set pointer when both `!changed` and
`curEvent.PreviousIndex == rtx.currentIndex` hold.

**Delta replay** (`handleModelChange`):
- For each replayed change set, call `filterEventsForRouter(changes, true)`.
- If filtered list is empty, advance `rtx.currentIndex` and skip the wire send. This is the
  per-router gap absorption the design doc anticipated.
- Otherwise, build a per-router `DataState_ChangeSet` whose `PreviousIndex` is rewritten to
  `rtx.currentIndex` so the receiver sees a continuous chain even when intermediate change
  sets were entirely filtered out. `Index`, `IsSynthetic`, and `TimestampId` are preserved.
  Update `rtx.currentIndex` after a successful send.

**Full sync** (`handleModelChange` fallback):
- After `GetDataState()`, call `filterEventsForRouter(dataState.Events, false)` and assign back.
- `EndIndex` and `TimelineId` unchanged. The snapshot already includes every relevant Config
  event globally, so synthesis is unnecessary on this path.
- Receiver-side: orphaned `Config` entities cached from earlier associations get pruned by
  the full-sync diff (per the agreed cleanup policy in the design doc).

### Two-trigger emission

1. **Router's `Configs` list changes**: the constraint handler emits a Router event into the
   change set. When that change set flows to the affected router, the filter synthesizes
   accompanying Config Create events. Other routers see only the Router event (they care about
   peer fingerprints, not per-router configs). Newly-removed IDs require no extra work — the
   receiver derives "what to apply" from its own `Router.Configs` list and drops anything not
   in it; the orphan `Config` entity stays cached harmlessly.
2. **A config's data changes**: the constraint handler emits a Config event. The filter
   delivers it to every router whose `Router.Configs` contains the config's ID.

### Tests

`controller/sync_strats/rtx_test.go` covers the filter:
- Drops unrelated configs, passes related ones.
- Pass-through for non-Config events (services, identities).
- Self-Router updates trigger Config synthesis in delta mode.
- Other-router events do NOT synthesize.
- Full-sync mode does NOT synthesize.
- Self-Router Delete does NOT synthesize.
- Synthesis silently skips IDs not yet in the RDM cache.

### Risks / follow-ups

- **Synthesis overhead**: every self-Router event re-emits all configs, even on unrelated
  updates (e.g. router rename). Acceptable cost — the receiver applies idempotently. Future
  optimization: diff `InitialState` vs `FinalState` in the constraint handler so we only
  synthesize on actual `Configs` changes.
- The `RouterStore` interface still embeds `boltz.EntityStore[*Router]` directly rather than
  the local `db.Store[E]`; the `ValidateType` generic was relaxed in 2b to accept the broader
  bound. Worth normalizing at some point but not blocking 2c.

---

## Phase 2d: Validate router configs end-to-end via the fablab RDM test -- DONE

**Goal**: Exercise the controller-side router-config plumbing against the
`router-data-model-test` fablab smoke test, and fix the gaps that surfaced
when planning that.

**Status**: Complete.

### What was implemented

**Filter only router-target configs** (`common/router_data_model_sender.go`,
`controller/sync_strats/rtx.go`):
- Phase 2c's filter dropped *all* configs not in a router's `Configs` list,
  including service-target configs that need to broadcast. The filter now
  consults `rdm.ConfigTypes` and only filters when `Target == "router"`.
  Service-target (and any unknown-type) configs pass through unchanged.

**Reusable filter on `RouterDataModelSender`**:
- Moved the filter logic from `RouterSender.filterEventsForRouter` down to
  `RouterDataModelSender.FilterEventsForRouter(routerId, events, synthesize)`.
  The rtx wrapper is now a one-line delegate.
- Added `RouterDataModelSender.GetDataStateForRouter(routerId)` which returns
  a per-router-filtered snapshot. The rtx full-sync path uses it directly.
- Added `common.ConfigTypeTargetRouter = "router"` to mirror the controller-db
  constant without introducing an import cycle.

**Validation flow filtered too**
(`controller/handler_mgmt/validate_router_data_model.go`): the
`ValidateRouterDataModelOnRouter` path used to send the global, unfiltered
snapshot to every router for diffing — which would have produced false
positives once any router had configs. It now calls `GetDataStateForRouter`
per-router. The "snapshot once and reuse" optimization is gone (each router
needs a different filtered view); the existing concurrency cap of 10 keeps
the cost bounded.

**`ValidateRouters` wired into `ValidateAll`**
(`controller/sync_strats/sync_instant.go`): the controller-side validator
now compares `db.Router` rows against `rdm.Routers`, catching name /
fingerprint / configs / disabled drift. (We added `ValidateRouters` in
Phase 2b but never called it.)

**Filter unit tests** (`controller/sync_strats/rtx_test.go`):
- `passesServiceTargetConfigs`: service-target config flows through even with
  no router association.
- `filtersOnlyRouterTargetConfigs`: in a mixed change set, router-target
  configs are filtered while service-target ones pass through.
- `unknownTypePassesThrough`: defensive — if a Config event references a
  ConfigType not in the cache, broadcast (rather than silently dropping).
- Existing tests adapted: `newTestRtx` now seeds router-target and
  service-target test ConfigTypes, and `cfgEvent` defaults to the
  router-target type.

**Fablab bootstrap extension**
(`zititest/models/router-data-model-test/main.go`,
`validation.go`): a new bootstrap step creates a router-target config type
with a permissive schema, a pool of 20 router configs, and patches each
edge router to associate two of them. Helpers added:
- `createRouterTargetConfigType(ctrl) (typeId, error)`.
- `createNewRouterConfig(ctrl, configTypeId) parallel.LabeledTask`.
- `associateRouterConfigs(ctrl, routerId, configIds, perRouter) error`.
- `models.PatchEdgeRouter(clients, id, patch, timeout)` in
  `zititest/zitilab/models/api.go`.

The existing `validateRouterDataModel` flow exercises the new code paths
end-to-end: each router's RDM should match the per-router-filtered
snapshot, and the controller-side `ValidateRouters` confirms the DB matches
`rdm.Routers`.

### Verification

```bash
go build ./...
go test ./controller/... ./common/...      # all green
cd zititest && go build ./models/router-data-model-test/
# In a fablab session:
kitty test router-data-model-test          # bootstrap + validate
```

### Out of scope

- Per-router inspect-based assertion (walk every router via
  `ziti fabric inspect router-data-model` and confirm Configs map shape) —
  the existing diff-based validation is sufficient for now.
- Router-side `Routers` map dispatch through allow-list / handler registry —
  that's the rest of Phase 3c.

---

## Phase 2e: Router-side Routers map -- DONE

**Goal**: Receive and store the `Router` events Phase 2a/2b emit, so the
fablab test's existing diff-based validation actually checks router-entity
propagation. Carved off from Phase 3c because it's purely receive-side
plumbing for an already-shipping wire format and doesn't depend on 3a (allow-
list) or 3b (handler registry).

**Status**: Complete.

### What was implemented

In `common/router_data_model.go`:
- New `Routers cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_Router]`
  field on `RouterDataModel`. Initialized in `NewBareRouterDataModel`,
  `NewReceiverRouterDataModel`, and `NewReceiverRouterDataModelFromDataState`.
  Inherited in `NewReceiverRouterDataModelFromExisting`.
- New `HandleRouterEvent(event, model)` method, mirroring the
  Set/Remove pattern of `HandleConfigEvent` etc.
- New `*edge_ctrl_pb.DataState_Event_Router` case in the `Handle()` switch.
- `Diff()` now calls `diffType("router", rdm.Routers, o.Routers, sink, ...)`,
  so the existing controller→router validation flow checks router entities.
- `GetEntityCounts()` includes `routers`.
- `inspect router-data-model` JSON now includes a `routers` map (id, name,
  fingerprint, configs, disabled per entry).

**Self-router config GC**: when the receiver processes its own (`selfRouterId`)
Router event with action != Delete, it iterates `rdm.Configs` and removes any
router-target entry whose ID is no longer in the new `Configs` list. This
replaces the originally proposed "leave orphans cached, prune on full sync"
policy: the receiver now keeps its Configs map aligned with its assignment
without requiring the controller to send synthetic remove events. The
`selfRouterId` is set at construction via the receiver constructors (Phase 2e
also added the `Target` field to the router-side `ConfigType` so the GC can
distinguish router-target from service-target configs). Service-target configs
broadcast as before and are never GC'd by this path.

The receiver constructors take a `routerId` argument:
`NewBareRouterDataModel(routerId)`, `NewReceiverRouterDataModel(routerId, closeNotify)`,
`NewReceiverRouterDataModelFromDataState(routerId, ...)`,
`NewReceiverRouterDataModelFromExisting(routerId, ...)`,
`NewReceiverRouterDataModelFromFile(routerId, ...)`. Pass `""` for non-receiver
views (controller-side construction, validate-snapshot parsing).

### What this phase did NOT touch

- No allow-list. The router accepts every Router event the controller sends.
  Phase 3a will gate this.
- No config dispatch through the handler registry. Phase 3b/3c.
- No router-side use of the Routers map for any behavior (e.g. filtering
  outbound link dials by `disabled`). The map is populated and validated;
  consumers come later.

---

## Phase 2f: Router-config chaos coverage in the fablab test -- DONE

**Goal**: Phase 2d set up router configs at bootstrap and left them static.
The chaos action only churned services / service-policies / identities /
service-target configs, so the dynamic router-config paths Phase 2c
specifically targeted were never exercised at scale. Phase 2f extends the
chaos pipeline to cover them.

**Status**: Complete.

### What was implemented

In `zititest/models/router-data-model-test/validation.go`:

- `taskGenerationContext.loadEntities` now splits configs and configTypes
  into service-target (existing chaos pool) and router-target (new pool)
  by inspecting `ConfigType.Target`. Also loads all edge routers.
- New fields on `taskGenerationContext`: `routerConfigTypes`,
  `routerConfigs`, `routers`, `routerConfigsDeleted`.
- New `generateRouterConfigTasks` chaos generator covering four operations:
  - **modify a router-config's data** — PATCH some `routerConfigs[i]` with
    fresh `Data`. Hits both currently-associated and currently-unassociated
    configs; the controller filter must deliver the modified config to the
    right routers and only the right routers.
  - **delete a router-config** every other iteration — exercises the
    `configStore.DeleteById` cleanup loop that strips the deleted ID from
    each referencing router's Configs slice (Phase 1b).
  - **create new router-configs** to keep the pool around 20 entries so
    re-shuffle has variety.
  - **re-shuffle every router's Configs** — picks a new random subset
    (1-3 entries) per router, with one router per iteration deliberately
    set to empty. Exercises both the synthesis-on-add path in the rtx
    delta filter and the leave-orphan-cached behavior on remove, plus the
    "router goes from some configs to none" case.
- Wired into `getServiceAndConfigChaosTasks` so each chaos cycle now
  includes router-config churn alongside service churn.

The existing `validateRouterDataModel` flow (with the Phase 2d filter fix
and the Phase 2e Diff extension) catches any drift introduced by the new
chaos.

### Coverage matrix

|                                    | Pre-2f | 2f |
|------------------------------------|:------:|:--:|
| Static config association          |   ✓    | ✓  |
| Add config to a router             |   -    | ✓  |
| Remove config from a router        |   -    | ✓  |
| Router goes from some to no configs|   -    | ✓  |
| Modify associated config data      |   -    | ✓  |
| Modify unassociated config data    |   -    | ✓  |
| Delete an associated config        |   -    | ✓  |

### Out of scope

- Multi-config-type chaos (the test uses one router-target type). Adding more
  types would exercise per-type uniqueness validation (one config per type per
  router) but that's already covered by the integration tests in `tests/`.

---

## Phase 3a: Managed config allow-list -- DONE

**Goal**: Router local config controls which config types it accepts from
the controller. This is the operator's "controller-can-do-this" boundary.

**Status**: Complete.

### What was implemented

**New types** (`router/env/config_managed.go`):
- `ManagedConfigOptions{Allow []string}`.
- `LoadManagedConfigFromMap(cfgmap)` parses the optional `managedConfig`
  section out of the router YAML map. Always returns a non-nil
  `*ManagedConfigOptions` so callers don't need a nil guard. Returns an
  error on malformed shape (non-map `managedConfig`, non-list `allow`).
- `(m *ManagedConfigOptions).IsAllowed(configType)` returns:
  - false when `m == nil` or `Allow` is empty (disabled — the safe default
    when the section is absent).
  - true if any entry equals `"all"`.
  - true on exact match (`entry == configType`) or family prefix match
    (`configType` starts with `entry + "."`). The trailing-dot guard
    prevents `router.link` from matching `router.linkx.v1`.
- `ManagedConfigAllowAll = "all"` constant.

**Wiring** (`router/env/config.go`):
- New `ManagedConfig *ManagedConfigOptions` field on `Config`.
- `LoadConfigWithOptions` calls `LoadManagedConfigFromMap` near the end of
  the parse, after the `interfaceDiscovery` block.

**Tests** (`router/env/config_managed_test.go`):
- Absent / nil / empty-list `managedConfig` -> disabled.
- `["all"]` -> all types accepted.
- Exact match (`router.link.v1`) only matches that one type.
- Family prefix (`router.link`) matches `router.link.v1`, `router.link.v2`,
  rejects `router.linker.v1` (trailing-dot guard) and unrelated families.
- Mixed list with `"all"` short-circuits to true.
- Invalid shapes (non-map `managedConfig`, non-list `allow`) error.
- Nil receiver `IsAllowed` returns false.

### Family-prefix matching rationale

The design doc's example uses `router.link`, not `router.link.v1`. Operators
typically want to allow a family of config types and have version bumps
"just work" without re-editing the YAML. Prefix-by-family supports this
while still accepting exact-match (operators can pin to a specific version
if they want).

### Out of scope

- Phase 3c will read `env.GetConfig().ManagedConfig.IsAllowed(...)` when
  dispatching `Config` events to the handler registry.
- Hot reload of the allow-list (Phase 4b's broader hot-reconfig story).

---

## Phase 3b: Config handler registry -- DONE

**Goal**: Define the contract every config-aware router subsystem implements,
plus the central registry that routes events with version selection and
rollback. The blocker for Phase 3c's RDM dispatch and Phase 4b's link
handler.

**Status**: Complete.

### What was implemented

**Naming convention**: every config type is `<baseType>.v<N>` where N is a
positive integer (e.g. `router.link.v2`). The registry parses incoming names
into `(baseType, version)`, keys all state by base, and selects the highest
version that's both supported by the handler and available from the
controller. This matches the design's versioning policy and keeps handler
APIs free of version-string parsing.

**New package `router/managedconfig/`**:

`handler.go` — `ConfigHandler` interface:
- `BaseType() string` — un-versioned family, e.g. `router.link`.
- `SupportedVersions() []int` — the integer versions this handler can
  apply; order doesn't matter (registry picks max).
- `Apply(version int, data string) error` — registry has chosen this
  version; reconcile your subsystem. Data is a raw JSON string (router
  configs are always JSON); use `[]byte(data)` if a parser needs bytes.
- `Remove() error` — nothing of yours is currently available; tear down.

`registry.go`:
- `ParseConfigType(name) (baseType, version, err)` — parses
  `router.link.v2` into `("router.link", 2)`. Rejects empty base, empty
  version, non-integer, non-positive, or absent `.vN` suffix.
- `NewRegistry(alert AlertCallback)` — alert defaults to a logger.
- `Register(handler)` — keyed by `BaseType()`. Rejects duplicate
  registrations for the same base (`ErrHandlerAlreadyRegistered`).
  Panics if called after `Seal()`.
- `Seal()` — marks the registration phase complete. Lifecycle is:
  construct → register handlers → Seal → process events → Close. The Seal
  point is a strict barrier: late `Register` calls **and** `Apply` / `Remove`
  before Seal are both programming errors and panic. Operationally, Phase
  3c will call Seal between "subsystems registered" and "RDM dispatch
  started," so production Apply / Remove always happen post-Seal.
- `ApplyController(configType, data)` / `ApplyLocal(configType, data)` —
  Phase 3c / Phase 4b entry points; parse the name, look up the handler's
  entry, update `entry.versions[source][version]` under a short critical
  section, then **spawn a goroutine** to reconcile. Panic if called
  pre-Seal. Return parse errors synchronously for malformed types, and
  `ErrNoHandlerRegistered` when no handler owns the base.
- `RemoveController(configType)` — drops a specific (base, version) entry
  from the controller-source set; other controller versions for the same
  base remain. Reconciles.
- `RemoveLocal(baseType)` — takes a base type, not a configType. There's
  at most one local entry per base, so the version is meaningless for
  removal; the asymmetry with `RemoveController` is deliberate.
- `ConfigSource` enum (`SourceController`, `SourceLocal`). Source
  precedence is **strict local-wins at the base level**: if any local data
  is set for `router.link`, the controller's versions are ignored
  entirely. If local's version isn't one the handler supports (e.g.
  YAML translator emitted vN but build only supports vN-1), the registry
  applies nothing and logs an error every reconcile — operator intent
  ("use my local config") must not be silently overridden by falling back
  to the controller's data.
- `Applied(configType) (source, version, found)` — diagnostics accessor.
  `AppliedVersion` keeps the source-agnostic shape for backward compat.
- `Handler(configType)` — lookup helper, routes by base.
- `AppliedVersion(configType)` — diagnostics / test accessor.
- `Close()` — marks the registry shut down, blocks until every in-flight
  reconcile goroutine has exited. After Close, Apply/Remove still update
  state but don't spawn new reconciles.
- `WaitForIdle()` — blocks until the WaitGroup of in-flight reconciles
  hits zero. For tests.
- `Inspect()` — snapshot of registry state for diagnostics. Returns
  `common/inspect.RouterConfigRegistryState`. Inspect types live in
  `common/inspect/managed_config_inspections.go` so they can be used by
  any caller. The registry walks handlers in `BaseType` order for
  deterministic output, and each `handlerEntry`/`localEntry`/`appliedState`
  has its own `inspect()` method that produces its slice of the snapshot.
  `RouterConfigVersionDetail.Data` is `any`: stored JSON strings are
  unmarshaled so the inspect output inlines the config as a nested object
  (`"data":{"hello":"world"}`) rather than an escaped string. On unmarshal
  failure the raw string is preserved so the diagnostic still shows what
  the registry actually holds.
- `reconcileAsync(entry)` (internal) — drives the four-case transition
  matrix:

  | prev    | next    | action                                                      |
  |---------|---------|-------------------------------------------------------------|
  | empty   | empty   | nothing                                                     |
  | empty   | vN      | Apply(N). On error: Remove + alert; applied=∅               |
  | vN      | empty   | Remove. On error: alert; applied stays vN                   |
  | vN      | vM (≠N) | Apply(M). On error: Apply(N) rollback. Both fail: Remove + alert |

  Rollback uses the previously-**successfully**-applied data tracked on
  the registry side, not whatever's currently in `availableData`, so
  "v2 broke and v2 was just removed" still rolls back to v1.

**Concurrency model**:

- `r.mu` (the global registry lock) is held only for short critical
  sections — reading/writing the shared maps and snapshotting state. It's
  never held across handler calls, so a slow handler can't stall any
  caller.
- Each registered handler owns a `handlerEntry.lock`. A reconcile
  goroutine acquires this lock, snapshots state under r.mu, releases r.mu,
  runs the transition matrix (calling the handler with no global locks
  held), then re-acquires r.mu briefly to update `applied[handler]`.
- Different handlers reconcile in parallel. Reconciles for the same
  handler serialize via the entry lock.
- Bursts of events for the same configType collapse: each Apply call
  spawns a goroutine and updates `availableData`. The first goroutine to
  acquire the entry lock snapshots the latest data and runs the handler;
  later goroutines snapshot the same already-applied state and exit
  without calling the handler.
- `Apply`/`Remove` return as soon as state is recorded; the handler call
  runs asynchronously. The channel-receive goroutine and RDM event pool
  are never blocked by handler latency.

**Tests** (`registry_test.go`):
- ParseConfigType: valid forms (single-segment base, multi-segment base,
  multi-digit version) and invalid forms (no suffix, empty version,
  non-integer, trailing junk, non-positive, empty base, no `.v`).
- Register: single, base-ownership-routes-by-base, duplicate, late-arrival
  apply.
- Apply/Remove parse errors return up to caller.
- Single-version flow: no-handler stash, first-time, update, no-op when
  identical, first-failure-triggers-remove, update-failure-rollback-succeeds,
  update-failure-rollback-fails-triggers-remove.
- Multi-version flow: highest-wins, handler-supports-subset (v2 ignored
  when handler only supports v1), fallback-on-remove, fallback-on-apply-
  failure, remove-last-available, out-of-order arrival.
- Remove: no-handler drops data, handler error keeps applied state.
- Default alert smoke.

- Concurrency: different handlers reconcile in parallel even when one is
  slow.
- Close: drains in-flight reconciles; prevents new spawns after.

- Seal lifecycle: panics on late Register, on Apply before Seal, on Remove
  before Seal; Apply still works after Seal.
- Apply/Remove: returns `ErrNoHandlerRegistered` when no handler owns the
  base.
- Source tracking: ApplyLocal beats ApplyController at the base level;
  RemoveLocal falls back to controller data; RemoveController is a no-op
  when local is set; ConfigSource.String for diagnostics.

42 unit tests, all green (race-clean under `go test -race`). Tests use a
`newSealedRegistry(t, handlers...)` helper to reduce lifecycle boilerplate.

**Router wiring**:

- `RouterEnv.GetRouterConfigRegistry()` exposes the registry to the rest
  of the router.
- `router.Router` constructs the registry unconditionally via
  `managedconfig.NewRegistry(nil)`; the default alert callback logs.
- `router/inspect/inspect.go` dispatches `router-config-registry` to
  `registry.Inspect()`, addressable from the CLI as
  `ziti fabric inspect router-config-registry`. The inspect key is the
  `inspect.RouterConfigRegistryKey` constant in `common/inspect/`.

### Out of scope

- Phase 3c will plug the registry into the router-side RDM event dispatch
  and pair it with Phase 3a's allow-list (allow-list filters before
  hitting the registry).
- Backoff / retry-throttling on persistently-failing configs. The
  registry retries on every reconcile cycle, alerting each time. Operator
  problem to fix.
- Phase 4b will be the first concrete handler (`router.link.v1`).

---

## Phase 3c: Handle Router and Config events in router-side RDM -- DONE

**Goal**: Wire the router-side RDM through Phase 3a's allow-list and Phase 3b's
registry, so Config events arriving from the controller actually drive the
registry instead of just landing in `rdm.Configs`. With this in place, Phase
4b's link handler will receive events end-to-end as soon as it registers.

**Status**: Complete.

**Depends on**: 2a (Router protobuf), 2e (router-side Routers map), 3a
(allow-list), 3b (registry).

### What was implemented

**New subscriber interface in `common/subscriber.go`**:

```go
type RouterConfigEventSubscriber interface {
    OnRouterConfigApplied(configType string, data string)
    OnRouterConfigRemoved(configType string)
}
```

Single global broadcast hook (not per-entity). Existing `IdentitySubscription`
machinery is shaped for per-ID subscriptions with denormalized snapshots and
cascading events; router configs need none of that, and the only "subscriber"
is the registry's bridge. Naming borrowed from the existing subscriber
vocabulary; machinery is not.

**RDM-side dispatch in `common/router_data_model.go`**:

- `RouterDataModel` gains a `routerConfigSubscriber` field guarded by a
  dedicated `RWMutex`.
- `SetRouterConfigSubscriber(s)` / `RouterConfigSubscriber()` accessors.
- `HandleConfigEvent` dispatches to the subscriber whenever the affected
  Config's `TypeId` resolves to a `ConfigType` with `Target ==
  ConfigTypeTargetRouter`:
  - Create / Update → `OnRouterConfigApplied(configType.Name, dataJson)`
  - Delete → `OnRouterConfigRemoved(configType.Name)` — `TypeId` is captured
    inside the `RemoveCb` before the Config is removed; the `ConfigType`
    lookup still succeeds because `ConfigType` records outlive the `Config`s
    that reference them.

Non-router-target Configs (services, etc.) flow through `HandleConfigEvent`
unchanged; the dispatch is gated on target. Unknown `TypeId`s, nil
subscriber, and Configs without a matching ConfigType all short-circuit
safely.

**Router-side bridge in `router/state/router_config_subscriber.go`**:

`RouterConfigSubscriber` translates RDM events into registry calls:

- `OnRouterConfigApplied(configType, data)`: gate on `allow.IsAllowed`;
  reject → info log + drop; allow → `registry.ApplyController(configType,
  data)`. Errors logged.
- `OnRouterConfigRemoved(configType)`: gate on `allow.IsAllowed`; allow →
  `registry.RemoveController(configType)`. (Rejected removes are no-ops; the
  registry was never told about that type to begin with.)

The subscriber takes a narrow `configAllowList` interface (one method,
`IsAllowed`) and a `*managedconfig.Registry`, not the full `RouterEnv`. The
public `NewRouterConfigSubscriber(env)` resolves both off the env;
`newRouterConfigSubscriberFromParts` is the unexported test-friendly
constructor.

**State-manager lifecycle in `router/state/manager.go`**:

The state manager owns the subscriber across full-state resyncs (which
construct a fresh RDM and call `SetRouterDataModel(new)`).

- `SetRouterConfigSubscriber(s)` on the `Manager` interface — stores the
  subscriber, attaches it to the current RDM (if any), and bootstraps by
  walking the current `rdm.Configs` and dispatching `OnRouterConfigApplied`
  for every router-target entry. Treats "before subscriber was set" as an
  empty prior state, so the bootstrap is just "everything in current."
- `SetRouterDataModel(new, ...)`:
  1. Attach the subscriber to the new model before the swap, so any
     `HandleConfigEvent` on the new RDM dispatches correctly the moment it
     becomes reachable.
  2. Store the new RDM and clear `rmdReplaceInProgress`.
  3. Run `dispatchRouterConfigDiff(existing, new, sub)` — by now the new
     model is authoritative, so a subscriber that calls `RouterDataModel()`
     during dispatch sees the post-swap state. This matches the contract of
     `HandleConfigEvent`, which dispatches *after* `rdm.Configs` is updated.
     - For every router-target type present in old but not new:
       `OnRouterConfigRemoved`.
     - For every router-target type in new whose data differs from old (or
       is brand new): `OnRouterConfigApplied`. Unchanged entries are
       skipped — each dispatched Apply takes registry locks and spawns a
       reconcile goroutine, so skipping the no-ops meaningfully cheapens the
       common case of a full-state resync where most configs are stable.

The registry no-ops same-data Applies, so dispatching everything in `new`
on the apply pass is cheap and idempotent. The diff is the load-bearing
piece — without it, configs that vanish during a full-state resync would
never surface as removes (the new RDM's `HandleConfigEvent` runs inside the
constructor, before the subscriber is attached).

**Router startup wiring in `router/router.go`**:

In `Run()`, right after `registerComponents`/`registerPlugins` and before
`startControlPlane`:

```go
self.configRegistry.Seal()
self.stateManager.SetRouterConfigSubscriber(state.NewRouterConfigSubscriber(self))
```

Seal happens after all subsystem-side `Register` calls (today: none; Phase
4b's link handler will be the first registrant). The subscriber is wired
between Seal and the controller connection so events from any inbound bulk
or delta state will route through the allow-list and into the registry.

**Tests**:

`common/router_data_model_test.go` adds 5 `HandleConfigEvent` cases — dispatch
on router-target Apply / Remove, no-dispatch for non-router-target,
no-panic with nil subscriber, no-dispatch when `ConfigType` is unknown.

`router/state/router_config_subscriber_test.go` adds 4 allow-list cases —
allowed Apply hits the registry, disallowed Apply is dropped, allowed
Remove hits the registry, disallowed Remove is dropped — plus 7 cases for
`dispatchRouterConfigDiff` covering nil RDMs, all-new, all-old (full
remove), mixed add/remove/keep, and service-target filtering.

All tests race-clean under `go test -race ./router/managedconfig/...
./router/state/... ./common/`.

### Pre-Seal events

A subtle race-safety property fell out of the design: pre-Seal events never
reach the registry. `Seal()` happens before the subscriber is attached;
events arriving before Seal (e.g. a fast controller pushing bulk state
during router startup) flow through `HandleConfigEvent` but find no
subscriber set on the RDM, so they store in `rdm.Configs` without
dispatching. The subsequent `SetRouterConfigSubscriber` call walks the
already-loaded `rdm.Configs` and dispatches as a bootstrap pass. No queue,
no drops, no panic — events are simply replayed once.

### Out of scope

- A real handler. Phase 4b implements `router.link.v1`.
- Local YAML translation feeding `ApplyLocal` — the local-config side of
  Phase 3b's strict-local-wins semantics is also Phase 4b's job.
- Alert-callback transport from the registry back to the controller. The
  callback hook is in place (default logs); payload/protocol is a later
  phase.
- Inspection of subscriber state. Today the registry's `Inspect()` shows
  applied state per handler; the subscriber itself is stateless beyond the
  allow-list it closes over.

---

## Phase 4a: Make listeners safely closeable mid-run -- DONE

**Goal**: Let Phase 4b's link handler reconcile a `router.link.v1` config change
by closing the current listeners and constructing fresh ones from the new
config, without leaking goroutines and without disturbing any already-
established `Xlink`s.

**Status**: Complete.

### What was implemented

**`xlink_transport/listener.go`**:

- Added `stopC chan struct{}` and `closeOnce sync.Once` to the listener
  struct. Initialized in the factory.
- `Close()` is now idempotent (via `closeOnce`). It closes `stopC`, closes
  the listening socket (with nil guard for "Listen never called"), and
  drains `pendingLinks` by calling `Close()` on each not-yet-fully-accepted
  link's underlay. Accepted Xlinks are independent and **not** touched.
- `cleanupExpiredPartialLinks` now selects on `stopC` in addition to
  `env.GetCloseNotify()`. Closing a single listener mid-run exits its
  cleanup goroutine without affecting the router-wide shutdown path.

**`xlink_transport/listener_close_test.go`** (new):

Four unit tests using a minimal LinkEnv stub (panic-on-unused methods,
only `GetCloseNotify` actually wired):

- `Test_Listener_Close_StopsCleanupGoroutine` — Close() makes the cleanup
  goroutine exit within 2s.
- `Test_Listener_Close_RouterShutdownAlsoStopsCleanupGoroutine` — sanity
  check that the existing router-wide close path still works.
- `Test_Listener_Close_Idempotent` — multiple Close() calls don't panic
  (would otherwise close an already-closed `stopC`).
- `Test_Listener_OpenClose_NoCleanupGoroutineLeak` — 25 open/close cycles
  leave zero `cleanupExpiredPartialLinks` goroutines (verified via runtime
  stack snapshot grep). Catches future leaks.

All race-clean under `go test -race`.

**Reconciliation strategy — rebuild, don't diff**:

After surveying the listener and dialer lifecycles, the simplest workable
approach for Phase 4b is "tear down all listeners, replace dialer config
structs, build new listeners." No per-entry diff, no identity-keyed
bookkeeping. The reasoning:

- **Dialers have no persistent state.** Each `Dial()` creates fresh sockets
  and channels; the dialer struct is a config holder plus refs to the
  acceptor, env, etc. Replacing a dialer is "swap the struct pointer" — no
  Close() needed, the old struct is GC'd. `AdoptBinding` state re-establishes
  naturally when the new listener registers.
- **Listeners do hold a listening socket and a cleanup goroutine**, but
  `Listener.Close()` is fast (it closes one socket; cleanup goroutine exits;
  accepted `Xlink`s are independent `channel.MultiChannel`s that survive).
  Close-then-reopen on the same TCP port works immediately on Linux —
  TIME_WAIT is for connected sockets, not listening sockets.
- **Brief accept gap during reconfig** is acceptable. Link
  re-establishment retries already exist; reconfig events are rare; the gap
  is sub-second. The simplicity is worth it.

Established `Xlink`s are *never* closed by config-driven reconciliation.
Operators use `ziti fabric delete link <id>` (which sends a `LinkFault`
control-plane message to both routers — see
`controller/network/network.go:1029` — and tears the link down cleanly) for
explicit cleanup.

### Tasks

- **Per-listener close signal in `xlink_transport/listener.go`**: today
  `cleanupExpiredPartialLinks` only exits on `env.GetCloseNotify()`, so
  closing a single listener mid-run leaks its cleanup goroutine. Add a
  per-listener `stopC chan struct{}`, close it from `Listener.Close()`, and
  select on it in the cleanup loop. Make `Close()` idempotent (multiple
  closes don't double-close `stopC`).
- **Tests** (`xlink_transport/listener_test.go`): (1) open-then-close cycle
  N times — verify no goroutine leak via a runtime stack snapshot looking
  for `cleanupExpiredPartialLinks` frames; (2) close listener while it has
  pending partial links — confirm cleanup goroutine exits and the listener
  doesn't deadlock; (3) double-close is a no-op.

That's the entire surface area for 4a. No new interface methods, no
`Dialer.Close()`, no registry API changes, no handler-side bookkeeping —
all of which Phase 4b also avoids by following the rebuild-don't-diff
strategy above.

### Out of scope (deferred admin tooling)

- **Stale-link GC tool**: an operator-facing command to scan currently
  established links against the active config and report (and optionally
  close) links that no longer have any reachable listener-or-dialer pair
  under the current config. Two-phase usage: a default dry-run flag that
  prints the link IDs and reasons, plus an explicit `--apply` (or `--gc`)
  flag that performs the cleanup. Until this exists, operators clean up
  with `ziti fabric delete link <id>` after consulting `ziti fabric list
  links`. The tool is the *answer* to "we let everything drain — how do I
  reclaim?" — important enough to track here even though it's not part of
  the config-reconciliation work itself.
- **Anti-affinity / quarantine** ("stop talking to router X right now,
  kill the link"). Different feature from config reconciliation; if needed,
  add as a separate command rather than overloading the link config.

---

## Phase 4b: Implement `router.link.v1` config handler -- DONE

**Status**: Complete.

**Depends on**: 3b, 4a.

**Goal**: A single object owns the router's link subsystem configuration — the
factories for each binding, the currently-applied `router.link.v1` config,
and the listener / dialer instances built from it. Local YAML is translated
to JSON at startup and fed in as `ApplyLocal`, which preempts any controller
config via the registry's strict-local-wins semantics. Controller-managed
link configs only take effect when the operator has *not* set listeners /
dialers in their local YAML.

### What was implemented

**`router/link/config.go`** — typed `Config`, `ListenerConfig`,
`DialerConfig`, `ChannelOptions`, `BackoffConfig`, `HeartbeatsConfig`,
matching the `router.link.v1` schema. `Groups` is a slice-typed alias with
custom `UnmarshalJSON` that accepts either string or array (per schema)
and `MarshalJSON` that always emits the array form. Duration fields stay
as strings (e.g. `"30s"`) so they round-trip cleanly into the existing
`channel.LoadOptions` parser.

**`router/link/factory_registry.go`** — `FactoryRegistry`:

- Owns `factories map[string]xlink.Factory`, the active `config`,
  `listeners`, `dialers`. `RWMutex`-guarded; accessor methods return slice
  copies so callers can iterate safely.
- `Register(binding, factory)` — pre-Seal phase. Idempotent for the same
  factory; errors on a different factory for the same binding.
- `BaseType()` → `"router.link"`. `SupportedVersions()` → `[1]`.
- `Apply(1, data)` — parses JSON, builds new listeners and dialers from
  the typed config (errors out without mutating state if any
  construction fails), closes the *old* listener slice, swaps in the new
  state, runs `setDefaultDialerBinding` for the single-listener
  single-dialer compatibility case, then calls `Listen()` on the new
  listeners. Established `Xlink`s on old listeners survive — Phase 4a
  made `Listener.Close()` safe for that.
- `Remove()` — closes listeners and clears all state.
- `Listeners()` / `Dialers()` / `GetConfig()` — snapshot accessors.
- Typed-struct → `transport.Configuration` map conversion (mirroring what
  YAML would produce) so the existing `xlink.Factory.CreateListener` /
  `CreateDialer` interface (and any third-party plugin factories) keeps
  working unchanged.

**`router/link/local_config.go`** — `ConfigFromLocalYaml`:

- Translates the router env's `config.Link.{Listeners,Dialers,
  Heartbeats,PayloadSenderQueueSize,AckSenderQueueSize}` into the typed
  `Config`, then JSON-marshals it.
- Returns `("", nil)` when no local link config is set — caller skips
  `ApplyLocal` and lets the controller manage instead.
- Type-strict per-field readers (`yamlString`, `yamlInt`, `yamlFloat`,
  `yamlGroups`, `yamlChannelOptions`, `yamlBackoff`) with clear errors;
  unknown keys silently ignored (the factory revalidates downstream).

**`router/router.go` wiring**:

- Removed `xlinkFactories map[string]xlink.Factory`, `xlinkListeners`,
  `xlinkDialers` fields. Replaced with `linkSubsystem *link.FactoryRegistry`.
- Removed `startXlinkListeners()`, `startXlinkDialers()`, and
  `setDefaultDialerBindings()` — their work happens inside
  `linkSubsystem.Apply()` now.
- `GetXlinkListeners()` / `GetXlinkDialers()` delegate to
  `linkSubsystem.Listeners()` / `.Dialers()`.
- `registerComponents()` registers the built-in transport factory via
  `linkSubsystem.Register("transport", ...)` instead of into a map.
- New `applyLocalLinkConfig()` — translates local YAML, calls
  `configRegistry.ApplyLocal(link.ConfigTypeV1, json)`, then
  `WaitForIdle()` so listeners are up before the rest of startup.
- Startup order in `Start()`:
  1. `registerComponents` (registers factories with `linkSubsystem`).
  2. `configRegistry.Register(linkSubsystem)` — pre-Seal.
  3. `configRegistry.Seal()`.
  4. `applyLocalLinkConfig()` — drives listeners up via the handler.
  5. `startXgressListeners()`.
  6. Start web services.
  7. `stateManager.SetRouterConfigSubscriber(...)` — controller events
     can now flow in.
- Shutdown closes listeners via `linkSubsystem.Listeners()`.

### Tests

`router/link/factory_registry_test.go` — 15 race-clean tests using fake
factory / listener / dialer doubles:

- Factory registration: rejects different factory for same binding;
  same-factory re-register is a no-op.
- `Apply` builds listeners and dialers; calls `Listen()`; single
  listener + single dialer auto-adoption works.
- `Apply` closes old listeners on rebuild.
- `Apply` errors out without mutating state on listener-create failure
  (verified previous listener stays uncllosed).
- `Apply` errors on unknown binding, malformed JSON, unsupported
  version.
- `Remove` tears down listeners and clears all state.
- Accessors return stable snapshots — a slice returned before a later
  `Apply` still holds the old contents.
- `Groups` JSON unmarshal accepts both single-string and array forms;
  marshal always emits array.
- `ConfigFromLocalYaml` round-trip preserves listeners, dialers,
  channel options, heartbeats, and queue sizes through YAML map →
  typed `Config` → JSON → re-parsed `Config`.

### Out of scope (deferred follow-ups)

- **Graceful link replacement on settings change**: dial replacement
  links with the new connection settings, transition traffic onto them,
  then close the originals — disrupting nothing. Today an `Apply` that
  changes link-level connection parameters takes effect only for *new*
  links. Worth exploring as a separate feature; need to validate that
  link-identity hand-off is supported by the protocol.
- **Hot reload of local YAML**: file-watch the local config and call
  `ApplyLocal` on change. Same machinery, just trigger plumbing.
- **Stale-link GC tool**: already documented under Phase 4a's "deferred
  admin tooling."

### Design choice — `link.FactoryRegistry` as the handler

Today the router has a plain `map[string]xlink.Factory` field
(`Router.xlinkFactories`) plus separate `[]xlink.Listener` /
`[]xlink.Dialer` slices. These three things together describe the link
subsystem's configurable surface; nothing else owns them. Phase 4b
promotes them to a single typed object — `link.FactoryRegistry` — which
also implements `managedconfig.ConfigHandler`:

```go
type FactoryRegistry struct {
    factories map[string]xlink.Factory  // binding -> factory

    mu        sync.RWMutex
    config    *Config             // current applied (or nil)
    listeners []xlink.Listener
    dialers   []xlink.Dialer

    // refs needed to construct listeners/dialers from config
    routerId *identity.TokenId
    // ... acceptor, bind handler factory, etc.
}

// Register binds a factory to a binding name. Called during router init,
// before any Apply.
func (fr *FactoryRegistry) Register(binding string, f xlink.Factory)

// ConfigHandler
func (fr *FactoryRegistry) BaseType() string                  // "router.link"
func (fr *FactoryRegistry) SupportedVersions() []int          // [1]
func (fr *FactoryRegistry) Apply(version int, data string) error
func (fr *FactoryRegistry) Remove() error

// Replace Router.xlinkListeners / Router.xlinkDialers
func (fr *FactoryRegistry) Listeners() []xlink.Listener
func (fr *FactoryRegistry) Dialers() []xlink.Dialer

func (fr *FactoryRegistry) Inspect() inspect.LinkSubsystemDetail
```

Bundling these in one object makes the lifecycle obvious: factories are
registered before Seal; `Apply` rebuilds listeners/dialers from the parsed
config; `Remove` tears down. Inspect surfaces it as a single unit.

### Reconciliation strategy — rebuild, don't diff

(Per Phase 4a's analysis.) On `Apply(1, data)`:

1. Parse `data` into the typed `link.Config` struct.
2. Close every current listener (`xlink.Listener.Close()` — Phase 4a made
   this safe mid-run).
3. Replace the dialer slice with freshly-constructed dialers from the new
   config. Old dialer structs are GC'd; they hold no persistent state.
4. Build + `Listen()` new listeners from the new config. Established
   `Xlink`s accepted by the old listeners survive — they're independent
   channels at this point.

On `Remove()`: close listeners, clear dialers, set `config = nil`. The
router has no link surface until the next `Apply`.

The factory interface (`xlink.Factory.CreateListener(id, transport.Configuration)`)
takes a `map[interface{}]interface{}`. Phase 4b parses JSON into the typed
struct, then converts each listener / dialer entry back into the map shape
when calling the factory. Keeps the factory interface (and third-party
plugin factories) unchanged.

### YAML translation for `ApplyLocal`

`config.Link.{Listeners,Dialers,Heartbeats,...}` is the YAML-decoded form.
The shape matches the JSON schema's properties closely (same field names,
same nesting). A small translator builds a `link.Config`, marshals it to
JSON, and calls `registry.ApplyLocal("router.link.v1", json)` at startup.
Strict local-wins ensures any later controller `router.link.v1` is ignored
as long as the local entry is present.

### Wiring into `router.Start()`

- `Router` constructs `*link.FactoryRegistry` early (before
  `registerComponents`).
- `registerComponents` calls `factoryRegistry.Register("transport", ...)`
  for the built-in transport factory, and (eventually) for plugin
  factories.
- `startXlinkListeners()` / `startXlinkDialers()` go away. Instead:
  - Translate local YAML to JSON.
  - Call `configRegistry.Register(factoryRegistry)`.
  - If local data exists, call `configRegistry.ApplyLocal("router.link.v1", localJson)`.
  - Existing `configRegistry.Seal()` + `SetRouterConfigSubscriber` happen
    as today.
- `Router.GetXlinkListeners()` / `Router.GetXlinkDialers()` delegate to
  the factory registry.

### Tests

- Handler-side reconcile: `Apply` with new config rebuilds listeners /
  dialers; `Apply` with same data is a registry-level no-op (already
  covered in Phase 3b tests but worth integration-checking); `Remove`
  tears down; `Apply` with malformed JSON returns an error; `Apply` with
  an unknown binding returns an error.
- YAML → JSON translator: round-trip a representative YAML config and
  assert the JSON shape conforms to the schema.
- Factory registration: registering after Seal panics (lifecycle
  enforcement).
- Listener/dialer accessor stability: while `Apply` is replacing the
  slices, concurrent `Listeners()` / `Dialers()` reads return consistent
  snapshots (covered by the RWMutex).

### Out of scope (deferred follow-ups)

- **Graceful link replacement on settings change**: today, an `Apply`
  that changes link-level connection parameters (e.g. heartbeat interval,
  queue sizes) only takes effect for *new* links. The follow-up idea:
  dial replacement links with the new settings, transition traffic onto
  them, then close the originals — disrupting nothing. Worth exploring
  as a separate feature; need to check whether the protocol supports
  link-identity hand-off cleanly. Documented here for traceability.
- **Hot reload of local YAML**: file-watch the local config and call
  `ApplyLocal` on change. Same machinery; just trigger plumbing.
- **Plugin factories registered after Seal**: today plugin registration
  happens before Seal; if a plugin needs to add a binding later, that's
  a lifecycle change worth designing separately.
- **Stale-link GC tool**: already documented under Phase 4a's "deferred
  admin tooling."

---

## Phase 5: Integration tests -- DONE

Apitests in `tests/` (in-process controller + routers, `//go:build apitests`)
covering the end-to-end controller → router managed config path.

### Tests

All in `tests/`, all use the in-process controller + a single edge router
(see Phase 6 for the reason multi-router scenarios live elsewhere):

- **`Test_ManagedConfigAlert_EndToEnd`** (`managed_config_alert_test.go`).
  Bad controller-side config (malformed JSON) → router's registry alerts
  → `ctrl_pb.Alert` reaches the controller's `event.Dispatcher`. Verifies
  the wiring added via `newManagedConfigAlertCallback`.
- **`Test_ManagedConfig_LocalWinsOverController`**
  (`managed_config_local_wins_test.go`). Local seed via `ApplyLocal`;
  controller pushes a `router.link.v1` Config via management API +
  edge-router PATCH; assert `Applied` stays `SourceLocal` with both
  sources visible in `Inspect`.
- **`Test_ManagedConfig_ControllerAppliesListener`**
  (`managed_config_apply_test.go`). Controller pushes a config with a
  listener; assert `Applied` source = Controller AND the listener's bind
  port is actually open (TCP connect succeeds).
- **`Test_ManagedConfig_DelayedConfigStartsListener`**
  (same file). Router starts with no config; assert no listeners / no
  Applied state / port closed. Push config; assert listener comes up and
  port opens.
- **`Test_ManagedConfig_DefaultsBindingToTransport`** (same file).
  Config omits the `binding` field on a listener entry; assert the
  listener still comes up on `transport`. Exercises `defaultBinding()`.
- **`Test_ManagedConfig_UpdateRebindsListener`** (same file). Config
  with port A → port A opens. Update the same config to use port B →
  port B opens and port A *closes*. Verifies the rebuild-on-Apply path
  in `link.FactoryRegistry` closes old listeners after building new
  ones, and old sockets really do release.
- **`Test_ManagedConfig_BadUpdateRollsBack`** (same file). Good config
  applied → listener bound. Push an Update to the same Config with an
  unknown binding (schema-valid, apply-failing). Assert: alert fires
  with `configBaseType: router.link`, original listener still bound
  (rollback to previous-good), `Applied` still Controller v1.

All race-clean. Each completes in <2s due to the in-process harness.

Tests require `cfg.ManagedConfig.Allow = []string{"router.link"}` via
cfgTweaks; routers default to empty allow-list which drops everything.

### Discoveries during integration test work

- **`MarkRouterDataModelRequired` triggered by allow-list opt-in** in
  `router.Start()`: gated on `len(cfg.ManagedConfig.Allow) > 0`. Edge
  routers separately mark RDM required via the edge xgress factory, so
  for the common edge case this is a no-op; the gate matters for
  non-edge routers that opt in via allow-list. The allow-list serves
  as the unified opt-in: empty = managed configs disabled, no RDM
  subscription attempted.
- **`ConfigFromLocalYaml` heuristic fixed**: heartbeats and queue-size
  alone no longer count as "local content." The env loader auto-fills
  heartbeat defaults on every router, so the previous heuristic
  suppressed controller management for every router without an explicit
  `link:` YAML section.
- **edge-api `v0.31.0` adds `Configs` to `EdgeRouterCreate`/`Update`/`Patch`.**
  Previous `v0.25.x` had the field on `RouterCreate` (transit) only, which
  made the feature unusable for edge routers via the REST API.

### Deliberately not tested in `tests/`

- **Two-router link establishment.** The `tests/` harness builds around a
  single edge router (port allocations, identity files, the
  `ctx.edgeRouterEntity` singleton). Standing up two concurrent edge
  routers is a multi-day infra change for marginal gain — the per-router
  config delivery + listener bring-up is already covered above. The
  multi-router link handshake is fabric/SDK behavior, not managed-config
  behavior, and is exercised in fablab scale tests.

### Fablab smoketest coverage (Tier 1)

`zititest/models/smoke/` has two of its three edge routers
(`router-east-2`, `router-west`) converted to controller-managed link
config. They carry the same listener/dialer settings the local YAML
would have produced; the third router (`router-east-1`) stays on local
YAML as a known-good control.

Setup:
- The two routers get a `ctrl-managed-link` tag in `smoketest.go`.
- `configs/router.yml.tmpl` uses `{{if .Component.HasTag
  "ctrl-managed-link"}}` to swap the local `link:` block for a
  `managedConfig: { allow: [router.link] }` block on those routers.
- `actions/managed_link_configs.go` defines a bootstrap-time action
  that iterates `.ctrl-managed-link` components, creates a
  `router.link.v1` config with listener (`tls:0.0.0.0:6000`, advertise
  `tls:<router_ip>:6000`) and dialer matching the YAML defaults, then
  assigns the config to the matching edge router via
  `ziti edge update edge-router --configs`.
- Hooked into `actions/bootstrap.go` after `InitEdgeRouters` and
  before router processes start.

If managed-config delivery breaks at fablab scale, the converted
routers fail to come up with their links and the smoketest fails
loud. Tier 2/3 fablab tests (config-change-under-traffic, restart
resilience, HA failover) are deferred until Phase 6 lands.

---

## Phase 6: Managed config delivery for non-edge routers

**Status**: Not started. Deferred from Phase 4b.

**Goal**: Let the controller deliver managed router configs to **transit**
(non-edge) routers, not just edge routers. The RDM transport is in place;
the gate is on the controller side.

### Current limitation

`controller/env/broker.go:RouterConnected` only invokes
`routerSyncStrategy.RouterConnected(edgeRouter, router)` when an
EdgeRouter record is matched by fingerprint. Transit routers fall into
the `else` branch and are never added to the strategy's `rtxMap`. The
side effects: transit routers' RDM subscribe requests are rejected with
*"received subscribe from router that is currently not tracked by the
strategy, dropping subscribe"*, no Config events flow to them.

Historically, the sync strategy was designed for distributing edge data
(identities, services, policies, edge-target configs). Phase 2a/2b/2c
made the RDM also the channel for general router configuration, but the
broker gate hasn't been relaxed.

### Approach (deferred)

Two options sketched during Phase 5 work:

**Option A — type-aware single strategy (smaller change, recommended for
first cut):**

- Broker calls `strategy.RouterConnected(maybeEdgeRouter, router)`
  unconditionally; `maybeEdgeRouter` may be nil.
- `InstantStrategy.RouterConnected` handles nil edgeRouter: creates the
  rtx entry, drives RDM subscription, skips edge-only setup (identity
  tracking, tunneler flag, etc.).
- `filterEventsForRouter` extends to drop edge-only event types
  (`Identity`, `ServicePolicy`, `Service`, `PostureCheck`,
  `Revocation`) when the destination router is non-edge.
- Edge-only event handlers (`ApiSessionAdded`, etc.) keep working —
  they only fire from edge-side managers and already walk `rtxMap`
  selecting edge routers.

Net: one strategy, one RDM, one `rtxMap`. Transit routers join the
tracker but get a stripped feed.

**Option B — split concerns (cleaner, do when complexity grows):**

- Split `RouterSyncStrategy` interface into `RouterStateSync` (general
  router state + Router/Config events) and `EdgeDataSync` (identity/
  policy/posture/revocation).
- Two implementation files; `sync_instant.go` becomes a thin composer.
- Broker calls both as appropriate.

### Out of scope until Phase 6

Until this work happens, managed router configs only flow to edge
routers. Operators using transit routers must continue to configure
links via local YAML.

---

## Phase 7: Friendly CLI for router.link.v1 configs

**Status**: Not started.

**Goal**: Replace hand-typed JSON with an ergonomic CLI for creating and
editing `router.link.v1` configs. Today the only path is:

```bash
ziti edge create config my-link router.link.v1 '{"listeners":[{"binding":"transport","bind":"tls:0.0.0.0:6000","advertise":"tls:1.2.3.4:6000"}],"dialers":[{"binding":"transport","options":{"connectTimeout":"30s"}}]}'
```

That's fiddly: nested JSON, multiple sections (listeners / dialers /
heartbeats / queue sizes), nested objects for channelOptions and backoff,
duration strings, easy to typo a key. Schema validation only fires after
submit; operator iterates by trial-and-error.

### Design options

**Flag-based subcommand** (most discoverable, easiest to script):

```bash
ziti edge create router-link-config my-link \
  --listener "transport@tls:0.0.0.0:6000=tls:1.2.3.4:6000" \
  --listener "ws@ws:0.0.0.0:8080=ws://router1:8080,groups=mesh" \
  --dialer "transport,connect-timeout=30s" \
  --heartbeat-send-interval 5s
```

Or with explicit flag groups:

```bash
ziti edge create router-link-config my-link \
  --listener-binding transport \
  --listener-bind tls:0.0.0.0:6000 \
  --listener-advertise tls:1.2.3.4:6000 \
  --dialer-binding transport \
  --dialer-connect-timeout 30s
```

Tradeoff: multiple listeners/dialers need repeated `--listener=...`
shorthand parsing OR a from-file fallback. Probably both: simple
single-listener common case via flags, complex multi-section via
`--from-file` or `--from-yaml`.

**Wizard mode** (most beginner-friendly):

```bash
ziti edge create router-link-config --wizard
# How many listeners? [1]
# Listener 1 binding [transport]:
# Listener 1 bind address: tls:0.0.0.0:6000
# Listener 1 advertise address: tls:1.2.3.4:6000
# Add another listener? [N]
# How many dialers? [1]
# ...
```

Easy for first-time setup. Not scriptable.

**Edit mode** (round-trip from existing config):

```bash
ziti edge edit router-link-config my-link
# Opens $EDITOR with the YAML representation of the current config;
# parses on save; rejects if invalid against the schema.
```

### Recommendation

Hybrid:

1. **`ziti edge create router-link-config <name>`** with flags for the
   common single-listener single-dialer case. `--from-file <path>` for
   complex configs. Listeners and dialers can each be specified
   multiple times (`--listener=...`).
2. **`ziti edge edit router-link-config <name>`** opens the config as
   YAML in `$EDITOR`. Round-trip via the `router/link/config.go` typed
   struct + a YAML marshaler. Schema-validate on save.
3. **No wizard** in the first cut — flags + edit cover the cases.
   Wizard can come later if operators ask for it.

### Out of scope

- Editing the assignment of a config to a router. That's
  `ziti edge update edge-router --configs` already.
- Schema discovery (`ziti edge list config-types --schema
  router.link.v1`). Useful but separate.
- Auto-generation from local YAML (operator hands the CLI a router's
  YAML, it spits out the equivalent `router.link.v1` config).
  Also useful, also separate.

---

## Phase 4 design note: Incremental reconciliation

When a config update changes the link section, the reconciliation should be
**as seamless as possible** — touch only what actually changed:

- **Add a listener** -> create the new listener; leave existing ones running.
- **Change advertise address only** -> update the running listener in place if
  possible; don't tear it down and rebuild.
- **Two listeners, only one changes binding** -> stop and recreate that one;
  leave the other one alone.
- **Remove a listener** -> stop just that listener.
- **No-op fields touched** -> no work at all.

The naive "tear everything down on any config change" approach disrupts
established links unnecessarily. Worse, it does so for changes that are
meaningful only to *future* connections (e.g. advertise-address updates).

This is the same diff-and-apply pattern used by Kubernetes-style controllers
and applies to any subsystem with a set of identifiable items: link
listeners, link dialers, ctrl-channel listeners, xgress listeners, edge
listeners. We should write the diff helper once and reuse.

A general-purpose helper might look like:

```go
// ReconcileSet drives an incremental update from oldItems to newItems. For
// each key:
//   - present in new but not old        -> onAdd
//   - present in both with diff content -> onUpdate (handler decides whether
//                                          to hot-update or stop+recreate)
//   - present in old but not new        -> onRemove
//   - present in both with same content -> nothing
//
// K is whatever stable identity the items use (listener bind address, dialer
// binding name, etc.). The handler picks K so identity-shape is per-domain.
func ReconcileSet[K comparable, V any](
    oldItems, newItems map[K]V,
    equal func(a, b V) bool,
    onAdd    func(K, V) error,
    onUpdate func(K, oldV, newV V) error,
    onRemove func(K) error,
) error
```

Phase 4b uses this for link listeners and dialers. Future phases (e.g. an
xgress.v1 handler) reuse the same helper. The helper itself probably belongs
in `router/managedconfig/` (or a sibling utility package) so multiple
subsystems can pick it up without depending on each other.

Identity choice per subsystem (TBD, captured here for design memory):
- **Link listeners**: bind address (the user-facing identity; advertise can
  change without a tear-down).
- **Link dialers**: binding name.
- **Xgress listeners**: binding name.
- **Ctrl-channel listeners**: bind address.

---

## Phase 4 design note: Source tracking and local-config precedence

The design doc states two things that have to be reconciled mechanically:

1. **Local config always wins** (`Precedence` section).
2. **Handlers receive config events from either source — controller OR local
   file reload — via the same registry** (`Hot Reconfiguration` section).

We're going with **Option B: the registry tracks the source of each piece of
data and applies local-wins as a rule**. Reasons:

- Single code path for handlers. Apply is always "here's a config; reconcile."
  Handlers don't grow source-aware branching.
- Better introspection. The registry can report "v1 is active from a local
  config" vs "v2 was supplied by the controller but is overridden by the
  local v1," which surfaces operator intent in diagnostics.
- The `ziti agent router reload-config` story works out of the box: the
  YAML loader translates local config to the per-handler JSON form and pushes
  it through `Apply(..., SourceLocal)`. Handlers don't need to know that
  reload happened.

### API additions (Phase 4b territory)

```go
type ConfigSource int

const (
    SourceController ConfigSource = iota
    SourceLocal
)

func (r *Registry) Apply(configType string, data []byte, source ConfigSource) error
func (r *Registry) Remove(configType string, source ConfigSource) error
```

### Selection semantics

Per handler, the registry stores `availableData[source][version] = data`.
Reconcile computes effective as:

- If `availableData[SourceLocal]` is non-empty: pick
  `max(supportedVersions ∩ localKeys)`.
- Else: pick `max(supportedVersions ∩ controllerKeys)`.

Local takes precedence at the **base level** — if the operator set anything
locally for `router.link`, the controller's versions are ignored for that
base. This matches operator intent: someone who set v1 locally probably
doesn't want the controller silently upgrading them to v2.

(Per-field merge — the richest interpretation of "fills in gaps where local
config is silent" — is **out of scope**. Wait for operator demand.)

### YAML → JSON translation

For each config type registered with the registry, the YAML loader needs a
translation function that maps the corresponding YAML section to the
controller-equivalent JSON. For `router.link.v1`:

- Drop fields removed in the v1 schema (`costTags`, `split`).
- Rename `dialer.bind` → `dialer.bindInterface`.
- Default `binding` if absent.

This is per-config-type code, lives next to the handler. The registry
remains agnostic to it.

---

## Phase 4 design note: Registry inspection

We need a way to introspect the registry for diagnostics — "why is the
router link subsystem configured this way?" The answer should surface:

- Which handlers are registered (base, supported versions).
- What data is available for each, by source and version.
- What is currently applied (source + version).
- Any recent alerts (parse failures, rollbacks, offline subsystems).

### Implementation

Add to `router/managedconfig/registry.go`:

```go
type RegistryInspect struct {
    Handlers []HandlerInspect `json:"handlers"`
}

type HandlerInspect struct {
    BaseType          string                       `json:"baseType"`
    SupportedVersions []int                        `json:"supportedVersions"`
    Available         map[string]map[int]int       `json:"available"` // source -> version -> byteCount
    Applied           *AppliedInspect              `json:"applied,omitempty"`
}

type AppliedInspect struct {
    Source  string `json:"source"`
    Version int    `json:"version"`
}

func (r *Registry) Inspect() RegistryInspect
```

`Available` reports byte counts rather than raw data — keeps the output
compact, sufficient for "yes this is present" diagnostics. A future
extension could include a hash or summary.

### Wiring (Phase 3c or 4b, easy to slot in)

Register an inspect target on the router's inspect handler so
`ziti fabric inspect <router> managed-config` returns the registry view.
Follows the same pattern as the existing `router-data-model` inspect
target (`router/inspect/inspect.go`).

---

## Dependency Graph

```
1a (DONE) ---|
1b (DONE) ---|
1c (DONE) ---|
              |
2a (DONE) ---|
2b (DONE) ---|
2c (DONE) ---|
2d (DONE) ---|
2e (DONE) ---|
2f (DONE) ---|
              |
3a (DONE) ---|
3b (DONE) ---|--> 3c
              |
4a -----------|--> 4b
              |
              └--> Phase 5 (testing)
```
