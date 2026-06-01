# Logging refactor — progress and phases

Companion to [agent-capabilities.md](agent-capabilities.md) and
[logging-refactor.md](logging-refactor.md). The design docs describe
the *what* and *why*; this doc breaks the work into ordered phases
so we know what to land first, what each phase delivers, and what
gates each phase before the next can start.

## Branches and where each phase lands

- **openziti/df** (mirror, insurance only) — phase 1.
- **ziti `agent-capabilities` branch** — phase 2. Absorbs the
  `github.com/openziti/agent` library into `common/agent` and builds
  the capabilities primitive + v2 commands on it, all in-repo. PR
  target: `main`.
- **ziti `logging-refactor` branch** — phases 3–7 (the foundation).
  Rebased on top of `agent-capabilities`. PR target: `main`.
- **After the foundation** — switch back to `gossip-links`, rebase
  it on the landed logging work, and proceed from there. The
  proof-of-pattern conversion, contention benchmark, developer
  note, and chaos validation happen as part of that rebase. Not
  planned in detail here; see "After the foundation" below.

## Phases

### Phase 1 — Mirror df under openziti (insurance only)

> **Superseded.** PR review of the foundation found behavior gaps in
> dl's PrettyHandler that its options cannot reach (blank labels for
> Fatal/Panic/Trace, color not actually gateable, color keyed on
> stdout while we log to stderr). We replaced it with a hand-rolled
> `logging.PrettyHandler` and removed the df dependency entirely; see
> "Why we hand-roll the pretty handler" in
> [logging-refactor.md](logging-refactor.md). The mirror can stay or
> be archived; nothing depends on it.

**Scope.** Create `github.com/openziti/df` as a pristine mirror of
`github.com/michaelquigley/df`, including the `v1.0.0` tag and full
history. ziti depends on **upstream df directly**; the mirror exists
only as a fallback for if upstream ever breaks backwards
compatibility or disappears.

**Why a mirror and not a real fork.** Keeping the mirror's module
path identical to upstream (`github.com/michaelquigley/df`, no
rewrite) makes the fallback a one-line `replace` directive in
ziti's `go.mod` with zero source changes. Depending on upstream
directly keeps import paths normal
(`github.com/michaelquigley/df/dl`) and avoids perpetual re-pathing
on every upstream sync.

**Deliverables.**
- Repo created at `github.com/openziti/df` via a mirror push, so all
  upstream tags (including `v1.0.0`) and history come over unchanged.
- Module path left as `github.com/michaelquigley/df`. No divergence;
  tags stay byte-identical to upstream so the fallback can pin any
  upstream version.
- Optional short README note (on `main` only, leaving tags pristine)
  explaining the mirror's purpose: insurance, not an active fork;
  ziti uses upstream directly.
- Fallback documented (a `MAINTAINING.md` or README note): if
  upstream breaks or vanishes, add
  `replace github.com/michaelquigley/df => github.com/openziti/df vX.Y.Z`
  to ziti's `go.mod`. No import-path or source edits required.

**Acceptance gate.**
- The mirror exists with the `v1.0.0` tag present and resolvable.
- A trivial Go module that imports `github.com/michaelquigley/df/dl`
  (upstream, the path ziti will actually use) compiles and runs the
  PrettyHandler example.
- Fallback dry-run: the same module with
  `replace github.com/michaelquigley/df => github.com/openziti/df v1.0.0`
  still compiles and produces identical output, proving the mirror
  is a drop-in.

**Dependencies.** None.

---

### Phase 2 — ziti agent-capabilities branch: absorb agent lib + capabilities + v2 commands

**Scope.** All in-repo, on the ziti `agent-capabilities` branch.
Absorb the `github.com/openziti/agent` library into ziti as
`common/agent` (mirroring the recent storage-library absorption),
consolidate the duplicated channel plumbing there, and build the
capabilities primitive and v2 log-level commands on top. There is no
separate library release and no dependency bump: it is one in-repo PR
series. See [agent-capabilities.md](agent-capabilities.md) for the
design.

**Deliverables.**
- **Absorb the agent library.** Move the agent source (framed
  protocol, AppInfo, listener) into `common/agent`; drop the external
  `github.com/openziti/agent` dependency; re-point the ~21 in-repo
  importers. Dependency-neutral: the agent's deps are already present,
  so the only `go.mod` change is removing the agent require.
- **Consolidate channel plumbing.** Collapse the 4 duplicated
  conn-to-channel server bindings (controller, router, tunnel, demo)
  and the CLI client dialer into one server-side binding API
  (`HandleChannelConnection`) and the client-side `NewChannel` /
  `ConnToChannel` / `MakeChannelRequest` helpers in `common/agent`.
  These start importing `channel/v4` + `identity`, which are already
  root-module deps, so there is no `go.mod` or Go-version change.
  Existing channel commands unchanged on the wire.
- **Capability registry:** bit constant `CapabilityLoggingSlogLevels`,
  `agentCapabilityNames` table, `GetAgentCapabilitiesMask`,
  `GetAgentCapabilityStringList`, `AgentCapabilityBitFromString`.
  `common/agent` is the only writer.
- `RegisterAppCapabilities(names ...string)` for the embedding app to
  register its own capability strings. **Must be called before the
  agent listener starts accepting connections.** Calls after that
  point return an error (or panic — TBD during impl). Frozen-after-
  start keeps the capability set consistent across all connections.
- **`AppInfoV2` command** (net-new framed op) returning
  `agent_capabilities` and `app_capabilities` as `[]string`. The
  legacy `AppInfo` (op 0xa, `map[string]string`) is **untouched**, so
  old clients are unaffected. A new client that hits an old server
  reads a clean zero-byte EOF on the unknown op and falls back to
  `AppInfo` with an empty capability set.
- Client methods: `HasAgentCapability`, `HasAppCapability`, backed by
  one lazy `AppInfoV2` fetch (with legacy fallback) per connection.
- v2 message types `SetLogLevelV2`, `SetChannelLogLevelV2`,
  `ClearChannelLogLevelV2`, channel dispatch wired up, using
  content-type IDs in the agent-reserved **30000+** band (clear of
  ctrl_pb's 1000s and mgmt_pb's 10000s, both of which ride the agent
  channel).
- **Agent-local level enum.** `common/agent` defines `LogLevel` (the
  seven canonical values + `String()`/`ParseLogLevel`), parses the
  wire string once, and hands callbacks an `agent.LogLevel`. It does
  not know about slog or logrus.
- Callback registration API for the app to supply level-change side
  effects. Shape:

  ```go
  type LogLevelCallbacks struct {
      SetLogLevel          func(level LogLevel)  // LogLevel = agent-local enum
      SetChannelLogLevel   func(channel string, level LogLevel)
      ClearChannelLogLevel func(channel string)
  }

  func RegisterLogLevelHandlers(cbs LogLevelCallbacks) error
  ```

  Single registration call; advertises `logging.slog-levels` only
  when all three fields are non-nil. Zero-valued callback means "not
  provided." Both the v2 channel handlers and the legacy framed
  handlers route through these callbacks, so the seam the logging-
  refactor branch needs already exists.
- **ziti registration:** ziti calls `RegisterLogLevelHandlers(...)`
  with callbacks that drive `logrus.SetLevel` / per-channel logrus
  level (matching today's framed-command behavior). Phase 7 extends
  the same callbacks to also drive the slog side.
- ziti's `common/capabilities` is **left alone.** It keeps its
  Controller*/Router* bits for the controller↔router channel
  protocol; agent IPC capabilities live in `common/agent`. Different
  transports and audiences, no merging.
- No ziti-side app capabilities registered yet; placeholder for
  future use.
- **Test coverage:**
  - `AppInfoV2` round-trip (capability fields present/absent) plus
    the old-server fallback (clean EOF → legacy `AppInfo`, empty
    caps), and old clients (parsing `map[string]string`) unaffected.
  - Consolidated channel transport: server binding + client dialer.
  - Each v2 message round-trips; channel-hello bitmask decodes back
    to the same string set; app caps are not in the bitmask.
  - **In-process integration test** that starts the agent, registers
    the log-level handlers, sends each v2 command plus the legacy
    framed equivalents, and asserts side effects via captured stderr.
    This is the "did the level actually change" surface, chosen over
    a fablab smoke step (brittle without a synthetic emitter) and an
    introspection command (overkill).
  - Backfill tests for existing agent functions where coverage adds
    confidence cheaply (judgment call during impl).

**Acceptance gate.**
- `ziti agent set-log-level info` works end-to-end via the v2 path
  against a freshly-built ziti binary; same for `set-channel-log-level`
  and `clear-channel-log-level`.
- An old `ziti agent` CLI (built before this branch) still works
  against the new ziti binary: capability absent → legacy framed path,
  and `ps`/`list` app-info still parses.
- No duplicated channel-plumbing copy remains outside `common/agent`;
  the external `github.com/openziti/agent` dependency is gone from
  `go.mod`.
- All touched-package unit tests pass.

**Dependencies.** None on Phase 1 (df is consumed later, in the
foundation phases); ordered after it only for branch sequencing. This
is the merge-point of the `agent-capabilities` branch.

---

### Phase 3 — `common/logging` core: AsyncHandler, levels, SyncEmit

**Scope.** Foundation Go code in `ziti/common/logging/`.

**Deliverables.**
- `AsyncHandler` with bounded queue, drain goroutine, per-level
  drop counters, summary-line emission.
- `AsyncOptions` (`QueueSize`, `BlockThreshold`, `SummaryInterval`)
  plus a `Validate() error` method that rejects nonsense values
  (`QueueSize < 1`, `BlockThreshold` outside the canonical level
  range, `SummaryInterval <= 0`). `NewAsyncHandler` calls
  `Validate` and returns the error.
- `queuedRecord` struct carrying both `slog.Record` and
  `context.Context`.
- `downstreamMu` shared between drain and `SyncEmit`.
- `Close()` non-blocking; bounded-drain contract; `drainDone`
  exposed for test use only.
- Custom `slog.Level` constants: `LevelTrace=-8`, `LevelFatal=12`,
  `LevelPanic=16`.
- `LevelName(slog.Level) string` and `ParseLevel(string)
  (slog.Level, error)` — single source of truth for level names.
- `SyncEmit(ctx, r)` for fatal/panic durability.
- `WithGroup("")` no-op rule documented in code (implemented as
  part of Phase 4's handler chain).
- **Drop-summary record shape**: emitted by the drain on each
  `SummaryInterval` tick when any per-level counter is non-zero.
  - `msg = "logging queue full, message drop summary"`
  - Attrs: one per non-zero level (`debug: 12345`, `info: 678`,
    ...), plus a `since` timestamp marking the start of the
    summary window. Counters atomically snapshot-and-zero in the
    drain just before building the record.
- **`OptionsFromFlags(fs *pflag.FlagSet) Options`** and
  **`AddFlags(fs *pflag.FlagSet)`** live in `common/logging` so
  the package's own tests cover the flag → options path. Imports
  are limited to `spf13/pflag` (already a transitive dep of ziti
  via cobra), no cobra dependency in the package itself. If real
  package bleed shows up during impl, the flag-binding code moves
  to the CLI layer and `common/logging` exposes only the Options
  struct.

**Acceptance gate.** All `common/logging/` tests pass:
- Normal flow: enqueue, drain, in-order output.
- Full-queue drop: sub-threshold records drop with counter
  increment; summary record appears on the next tick with the
  exact message and attr shape above.
- Full-queue block at/above threshold: Warn+ records block until
  drain advances; never panic.
- `Close()` returns immediately; concurrent second `Close()`
  returns immediately; drain completes within bounded time
  (test waits on `drainDone`).
- Concurrent `Handle` racing with `Close()` never panics.
- `LevelName` returns canonical lowercase strings for all seven
  canonical levels; falls back to lowercase offset form for
  non-canonical values. `ParseLevel` round-trips with `LevelName`.
- `SyncEmit` writes synchronously through the downstream handler;
  concurrent `SyncEmit` calls serialize through `downstreamMu`.
- `AsyncOptions.Validate()` rejects all the documented bad
  values and accepts all valid ones.
- `OptionsFromFlags` reads parsed flag values into Options;
  flag defaults match the Options zero-default that
  `Validate()` accepts.

**Dependencies.** None on prior ziti phases; Phase 2 not required.
Phases 3–7 can start in parallel with phase 2 if convenient, but
they all merge under the `logging-refactor` branch which rebases
on `agent-capabilities` (Phase 2) before going to `main`.

---

### Phase 4 — slog.Handler composition: chain of wrappers

**Scope.** The handler-chain pieces of `common/logging/` that
implement slog's `WithAttrs` / `WithGroup` ordering.

**Deliverables.**
- `AsyncHandler` (the queue/drain owner from Phase 3) directly
  implements `slog.Handler`. No separate `rootHandler` type — the
  exported `AsyncHandler` *is* the root of the wrapper chain.
  `WithAttrs` returns a `boundHandler`; `WithGroup` returns a
  `groupedHandler`; `WithGroup("")` returns the receiver
  unchanged.
- `boundHandler{parent, attrs}` (package-private) — prepends bound
  attrs to records in `Handle`. `WithAttrs` extends the same
  `boundHandler` (single struct allocation per chain segment, not
  per `With` call). `WithGroup` returns a `groupedHandler`.
  `WithGroup("")` returns the receiver. `Enabled` delegates to
  parent.
- `groupedHandler{parent, name}` (package-private) — wraps record
  attrs in `slog.Group(name, ...)` in `Handle`. `WithAttrs`
  returns a `boundHandler` wrapping self (so attrs land *inside*
  the group). `WithGroup` extends; `WithGroup("")` returns the
  receiver. `Enabled` delegates to parent.
- No `Enabled` in this chain does level gating. `AsyncHandler.Enabled`
  returns true; the real check lives upstream in Phase 5's
  registry-bound `namedHandler.Enabled` (per-name override or live
  global level), and logrus pre-filters bridged records by level
  before the bridge fires.
- `Handle` always returns `nil` from the chain wrappers and the
  root. Errors are not propagated synchronously because the
  actual `downstream.Handle` runs on the drain goroutine, after
  the caller has returned.
- **Drain-side error reporting**: when the drain's
  `downstream.Handle(ctx, record)` returns a non-nil error, the
  drain writes a short message directly to `os.Stderr`
  (bypassing slog to avoid recursion) and increments a
  `drain_errors` counter that appears in the summary line.

**Acceptance gate.**
- `With("a",1).WithGroup("g").With("b",2).Info("msg","c",3)` →
  `{msg, a:1, g:{b:2, c:3}}`.
- `WithGroup("g").With("a",1).Info("msg","b",2)` →
  `{msg, g:{a:1, b:2}}` — bound attrs inside the group.
- `WithGroup("a").WithGroup("b").Info("msg","c",3)` →
  `{msg, a:{b:{c:3}}}`.
- `WithGroup("").Info(...)` identical to `Info(...)` on all three
  handler types.
- `With("a",1).With("b",2)` produces a single `boundHandler` with
  both attrs, not nested wrappers.
- All composition cases also pass when the record travels through
  the async queue.
- Drain-side error injection test: a downstream that returns a
  forced error causes the drain to write to `os.Stderr` (test
  captures it), increment the `drain_errors` counter, and keep
  draining subsequent records.

**Dependencies.** Phase 3.

---

### Phase 5 — Named-logger registry + runtime level control

**Scope.** Registry that backs `logging.For(name)` and the
per-name override map.

**Deliverables.**
- Registry with global `*slog.LevelVar` + per-name override map
  (`map[string]*slog.LevelVar`) guarded by an `RWMutex`. Reads
  (Enabled checks) take RLock; writes (Set/Clear) take the write
  lock. Level changes are operator actions and not a hot path, so
  the lock cost on Set/Clear is fine.
- `HandlerFor(name) slog.Handler` returns a registry-bound child
  handler whose `Enabled` reads the override if present, otherwise
  reads `globalLevel.Level()` live.
- `For(name) *slog.Logger` wraps `HandlerFor(name)` with
  `slog.String("channel", name)` bound via `WithAttrs`. **Caches
  the constructed `*slog.Logger` per name** so hot-path callers do
  not allocate on every `For(...)` lookup. Panics on empty name -
  almost certainly a caller bug, caught in review when it's a
  literal, caught early in dev when it's not.
- `SetNamedLevel(name, level)` creates/updates the override under
  the write lock.
- `ClearNamedLevel(name)` deletes the map entry under the write
  lock. Crucially **does not copy the current global** - removing
  the binding means future global changes propagate to that name
  automatically.
- Per-name override does not change the async drop threshold; sub-
  Warn records still drop under saturation (already documented in
  the design).
- Channel attr survives the bridge: pfxlog's existing
  `Entry.Data["channel"]` field is copied onto the slog.Record by
  the bridge as an attr, so bridged records carry it for display
  even though per-channel override gating is slog-only.

**Acceptance gate.**
- Test matrix: global set / channel set below global / clear /
  second global set, all visible on the direct-slog path
  (`logging.For(name)`). Per-channel override applied to a name X
  is **not** visible on `pfxlog.ChannelLogger("X")` calls below
  global (slog-only rule).
- A `logging.For(name)` instance created *before* a global level
  change reflects the new level immediately.
- `For("")` panics.
- `For("X")` called twice returns the same cached `*slog.Logger`
  pointer (caching test).
- Composed test:
  `logging.For("router.link").WithGroup("g").With("k","v").Info("msg","x",1)`
  carries `channel:"router.link"` at the top level and nests `k`,
  `x`, and any further attrs under `g`. Both directly and through
  the async queue.

**Dependencies.** Phase 3 (registry uses AsyncHandler).

---

### Phase 6 — Bridge, Install, and format compatibility

**Scope.** The pieces that wire logrus into the new sink.

**Deliverables.**
- `noopFormatter` for logrus.
- `slogBridge` hook: builds a `slog.Record` from the logrus entry
  (no `Enabled` gate, no registry lookup — logrus already
  pre-filtered by global level, per-channel overrides are
  slog-only). Branches at `LevelFatal`: `SyncEmit` for fatal/panic,
  otherwise dispatches to `logging.RootHandler().Handle(...)`.
- `logging.Install(handler, initialLevel)`:
  - `logrus.SetOutput(io.Discard)`, `SetFormatter(noopFormatter{})`,
    `SetLevel(slogToLogrus(initialLevel))`, `AddHook(slogBridge)`.
  - Guarded by `sync.Once`; calling Install twice panics. If this
    complicates tests too much during impl, fall back to a boolean
    short-circuit on the second call (no panic). Test code in
    practice does not call Install at all (see test approach
    below).
- `OptionsFromFlags(fs *pflag.FlagSet) Options` and
  `BuildHandler(opts Options) slog.Handler` from Phase 3 are now
  wired into the actual cobra commands for `ziti controller run`
  and `ziti router run`.
- `RootHandler() slog.Handler` returns the live `AsyncHandler` for
  use by the bridge.
- `ReplaceAttr` callback for JSON output: renames `LevelKey` value
  through `LevelName(...)` and flattens `source` to flat `file` /
  `func` top-level keys (pfxlog shape).
- **Format default**: pretty handler when stderr is a TTY, JSON
  otherwise. Detected via `term.IsTerminal(int(os.Stderr.Fd()))`.
  Matches today's pfxlog default. `--log-formatter` flag forces a
  specific choice; default is "auto" which uses the TTY check.
- **Install call site**: in `main()` (for both `ziti-controller`
  and `ziti-router`), immediately after flag parsing, *before*
  `controller.Run` / `router.Run`. Any logging code path that runs
  before Install (very early flag parsing) will use logrus's
  uninstalled defaults; that divergence is short-lived and
  acceptable.
- Golden-file format test (`common/logging/format_test.go`) with
  the full fixture set: every-level rows (parsed), embedded-
  character row (raw), nested-group row (parsed, direct-slog
  only), reserved-key collision row (raw), bool/numeric/error/
  Stringer/nil/time.Time rows (parsed).
- Startup invariant test: captures `logrus.GetLevel()`,
  `StandardLogger().Out`, formatter type after Install; emits one
  bridged line through `pfxlog.Logger()`; re-captures; asserts the
  three values are unchanged and that
  `logrus.GetLevel() == slogToLogrus(globalLevel.Level())`.

**Test approach.** Logging-system tests construct an
`AsyncHandler` directly (via `NewAsyncHandler` from Phase 3) and
exercise the slog APIs without ever calling `Install`. This keeps
the test surface free of the production `sync.Once` machinery,
and the bridge-integration test is the one place that actually
mutates logrus's standard logger (via a per-test setup that
captures stderr, installs the hook, runs the assertions, and
cleans up).

**Acceptance gate.**
- All bridged-logrus call sites in the codebase land in the async
  sink with the correct shape.
- Golden test passes on both bridged-logrus and direct-slog paths,
  including the reserved-key collision row, the embedded-character
  raw row, and one record at each of the seven canonical levels.
- Startup invariant test catches any later mutation of logrus
  output or formatter; passes when no such mutation occurs.
- TTY-vs-redirected output selection: piped stderr produces JSON,
  TTY stderr produces pretty. Verified in `_test.go` using a fake
  os.Stderr (a pipe).
- pfxlog `GlobalInit` is no longer called anywhere in the codebase.

**Dependencies.** Phases 3, 4, 5.

---

### Phase 7 — Agent v2 handlers extend to slog

**Scope.** Update the level-change callbacks ziti registered in
Phase 2 so they drive both logrus and the slog global level /
named-logger registry.

**Deliverables.**
- `common/logging` exposes the level-control surface as plain
  exported functions:
  - `SetGlobalLevel(level slog.Level)` — calls
    `globalLevel.Set(level)` *and*
    `logrus.SetLevel(slogToLogrus(level))` in lockstep. This is the
    one entry point that drives both worlds; the agent handler
    calls it.
  - `SetNamedLevel(name string, level slog.Level)` — already from
    Phase 5.
  - `ClearNamedLevel(name string)` — already from Phase 5.
  Open to any caller (not just the agent handler); supports future
  use cases like boot-time programmatic level setup.
- ziti's `SetLogLevel` callback (registered via Phase-2's
  `agent.RegisterLogLevelHandlers`): parses the level string via
  `ParseLevel`, calls `logging.SetGlobalLevel(slogLevel)`. logrus
  pre-filters below-threshold records (saving the global mutex
  acquire), the bridge dispatches what gets through.
- ziti's `SetChannelLogLevel` callback: calls
  `logging.SetNamedLevel(channel, level)`. Does **not** touch
  pfxlog's channel-level mechanism.
- ziti's `ClearChannelLogLevel` callback: calls
  `logging.ClearNamedLevel(channel)`.
- Legacy framed handlers update similarly: global handler routes
  through `SetGlobalLevel`; channel handlers route through
  `SetNamedLevel`/`ClearNamedLevel`. Old clients see the same
  slog-side behavior as new clients.
- Old clients see the same global-level behavior as new clients;
  for per-channel control they need migrated code that uses
  `logging.For(name)`.

**Acceptance gate.**
- End-to-end test through `ziti agent set-log-level info`: prior
  debug filtered, subsequent info seen on both bridged-logrus and
  direct-slog paths.
- `TestPerChannelOverride_AppliesToSlogOnly_NotPfxlog`:
  `set-channel-log-level network.gossip debug` while global is
  info — `logging.For("network.gossip").Debug(...)` is seen; the
  matching `pfxlog.ChannelLogger("network.gossip").Debug(...)`
  call is filtered. Dedicated test name makes the slog-only rule
  obvious at-a-glance in failure output.
- Throughout the test, the Install invariant is intact: output is
  `io.Discard`, formatter is the noop, and
  `logrus.GetLevel() == slogToLogrus(globalLevel.Level())`.

**Dependencies.** Phases 2, 3, 5, 6.

Phases 3–7 are the foundation and the merge content of the
`logging-refactor` branch.

---

## After the foundation

Once phases 1–7 are landed, switch back to the `gossip-links`
branch, rebase it on the landed logging work, and proceed from
there. The following are intentionally **not** planned in detail
yet - they get figured out against the actual rebase, where the
real chaos workload and the gossip-links changes are in front of
us:

- **Proof-of-pattern conversion.** Convert at least one
  representative hot-path call site (candidate:
  `linkState.updateStatus` → `logging.For("router.link")`) so the
  migration pattern is exercised end to end. Could land with the
  foundation or with the rebase; decide when we get there.
- **Contention benchmark.** Measure `pfxlog.Logger().Info(...)`
  lock-hold time, baseline vs bridged, to confirm the
  contention-relief expectation with numbers. Methodology TBD.
- **Fatal/Panic durability tests.** Subprocess tests proving
  fatal/panic records reach stderr before exit.
- **Developer note** (`docs/logging.md`): how to write a slog line,
  named-logger convention, the "no new Warn/Error in hot paths"
  rule, migration approach for future PRs.
- **Chaos validation.** Run a fablab chaos burst with the bridge
  in place; capture p99 logrus-mutex hold time, queue depth, drop
  counts by level. Decide per-site Warn/Error remedies (rate
  limit / sample / downgrade / global threshold change) from the
  remedy ladder only if measurement shows saturation.

We will plan these concretely when we pick up the gossip-links
rebase.

---

## Cross-cutting tracking

- **Dependency chain.** Phase 1 (df mirror, external) → Phase 2
  (ziti `agent-capabilities` branch: absorb agent lib + caps + v2,
  all in-repo) → Phases 3–7 (ziti `logging-refactor` foundation) →
  gossip-links rebase. Only the df mirror is out-of-repo; everything
  from Phase 2 on is in the ziti repo.
- **Branch ordering on ziti `main`.** `agent-capabilities` lands
  first, then `logging-refactor` rebases on it and lands, then
  `gossip-links` rebases on `logging-refactor`.
- **What is *not* in any phase here.** PC-based method/file
  overrides, OTel adapter, persistent yaml-driven overrides,
  pfxlog removal. All explicitly deferred in the design.

## Post-foundation roadmap: convert codebases to slog in chunks

The foundation (phases 1–7) leaves pfxlog in place behind the
bridge. Operator per-channel control is slog-only; the migration
carrot is "convert your code to `logging.For(name)` to gain
per-channel debug overrides." This roadmap sketches the chunked
conversion that follows the foundation.

This is **not in scope for the logging-refactor branch**. It's a
forward-looking plan for tracking separately.

### Conversion principles

- **One library or subsystem per PR.** Keeps diffs reviewable and
  blast radius bounded.
- **Add channel names during conversion, not before.** Pick names
  by subsystem boundary (e.g., `router.link`, `controller.gossip`,
  `fabric.xgress`). Method-specific names only where surgical
  debug is genuinely useful.
- **Preserve log volume and level.** Conversion is mechanical, not
  a level audit. If a line is at Info today, it stays at Info.
- **Don't introduce Warn/Error in per-event hot paths.** This is
  already the standing rule in the design; conversion PRs that
  violate it get bounced.
- **Keep pfxlog imports compiling.** The bridge means we don't have
  to delete pfxlog references in one shot. Conversion can be
  incremental and interruptible.

### Suggested chunking order

Roughly highest-value-first, where "value" is "places operators
would most want per-channel debug control" and "places where
volume gives the bridge fast-path measurable benefit":

1. **router/link** — link state, dial, accept paths. The
   `linkState.updateStatus` proof-of-pattern conversion is the
   bootstrap; expand to the rest of `router/link`.
2. **router/xgress** — per-payload paths in xgress. Fabric Debug
   volume often originates here; conversion lets operators
   surgically enable Debug for one xgress flow without flooding
   everything.
3. **controller/gossip** and **router/forwarder** — the digest /
   forwarding paths that showed up in chaos traces.
4. **transport/v2** — cross-repo. The TLS handshake-EOF site lives
   here; conversion gives operators a way to enable Debug for the
   TLS subsystem specifically during DoS triage.
5. **fabric ctrl/router** — control-plane subsystems on each side.
6. **edge** — last, because it has the largest surface area and
   the most call sites; benefits from patterns established by
   earlier PRs.
7. **CLI tools and one-shot binaries** — convert as they get
   touched for other reasons; not worth a dedicated PR each.

### Per-conversion checklist (PR template)

- All `pfxlog.Logger()` and `pfxlog.ContextLogger(...)` calls in the
  package replaced by `logging.For(packageChannelName)` or by a
  package-scoped `var log = logging.For("...")`.
- Channel name documented at the top of the package (godoc
  comment).
- No Warn/Error introduced at per-event rate; reviewer confirms via
  grep.
- Tests touched as needed to use slog assertions instead of pfxlog
  ones.

### Tracking

Open question for later: do we want a tracking issue in this repo
that enumerates the chunks and check them off as PRs land? For
now, this section serves as the rough plan; we can formalize when
we start chunk #1, after the foundation and proof-of-pattern land.
