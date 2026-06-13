# Logging refactor

Branch: `logging-refactor`, off `fully-connected-mesh`. Precursor to
`gossip-links`. This document is the design proposal.

For reviewers in a hurry: read [Status quo](#status-quo) for the
problem, [Goals](#goals) for what success looks like in this branch,
and [High-level overview](#high-level-overview) for the architecture.
The remaining sections are supporting detail.

## Status quo

ziti logs through `github.com/michaelquigley/pfxlog`, which is a thin
prefix-and-caller wrapper around `github.com/sirupsen/logrus`. Every
log call in the codebase ultimately reaches `logrus.(*Entry).log()`,
which holds a single process-wide mutex (`MutexWrap.Lock` at
`logrus/logger.go:61`) across the format-then-write sequence. The lock
hold time is roughly `format duration + write syscall`. Under
contention, callers serialize behind this one mutex.

What recent fablab runs showed:

- During a chaos burst, every router thread that wanted to log piled
  up on this mutex. We have stack dumps showing 13 goroutines blocked
  on it at once, and an earlier dump showing a chain where the
  link-registry event loop (a single-threaded goroutine) was wedged on
  logrus, which back-pressured a bounded event channel, which caused a
  worker goroutine to block on `queueEvent` while holding the registry
  lock, which deadlocked 13 other workers.
- The dominant log volume came from a handful of sites: TLS
  handshake-EOF in `transport/v2/tls` (2700+ lines per router per
  burst), `linkState.updateStatus` (per link state transition), and
  `gossipClient.HandleDigest` (per incoming digest).
- The pattern repeats: any new feature that logs in a hot path is
  effectively buying mutex contention.

The contention is structural to the logger we use. We need a sink that
does not serialize every caller through one mutex.

## Goals

For this branch specifically:

- Resolve the global-mutex contention seen in pfxlog/logrus during
  chaos load — without forcing a whole-tree migration to land it.
- Land `log/slog` as the API contract at new and migrated call sites
  so future code uses the standard-library shape. Existing pfxlog/
  logrus call sites stay as-is and reach the same sink via the bridge.
- Provide an async write path with visible drop accounting and
  clean shutdown semantics.
- Preserve current JSON output shape so downstream parsers do not
  break.
- Keep operator log-level control working through the existing
  agent IPC, including per-name overrides for surgical debugging.
- Migrate one representative hot-path call site as a worked example.
  Do not migrate the rest in this branch.

## High-level overview

```
  New code:    slog.Info(...)
               logging.For(name).Info(...)
                          │
                          │
  Legacy:      pfxlog.Logger().Info(...)
               pfxlog.ChannelLogger("name").Info(...)
                          │
                          ▼
                   logrus.Hook  (entry → slog.Record;
                                  logrus formatter = noop,
                                  output = io.Discard)
                          │
                          ▼
                ┌──────────────────────────┐
                │     AsyncHandler         │
                │  bounded queue           │   single drain
                │  per-level drop counters │ ───goroutine───┐
                │  summary tick            │                │
                └──────────────────────────┘                │
                                                            ▼
                                            ┌─ JSONHandler + ReplaceAttr → stderr (prod)
                                            │
                                            └─ logging.PrettyHandler     → stderr (dev/console)
```

- **API contract:** vanilla slog at every call site (including code
  that uses our `logging.For(name)` helper). pfxlog/logrus stays in
  place for the ~3000 unchanged call sites; a single `logrus.Hook`
  copies every Entry into the same slog handler so both worlds land
  in one sink.
- **Single sink:** one `AsyncHandler` instance owns the bounded
  queue, per-level drop counters, and the only goroutine that calls
  the downstream handler. Legacy and new call sites interleave in
  the same destination in the order they arrive.
- **Output shape:** production JSON matches the current
  pfxlog/logrus shape via a `ReplaceAttr` coercion (lowercase
  level, flat `file`/`func`). Dev console uses
  `dl.NewPrettyHandler`, a direct port of pfxlog's pretty output.
- **Operator control:** existing `ziti agent set-log-level`
  updates both the slog global level and logrus global level.
  `set-channel-log-level <name> <level>` and
  `clear-channel-log-level <name>` update only the slog named-logger
  registry used by `logging.For(name)`; pfxlog/logrus channel
  loggers remain governed by the global logrus level.
- **Out of scope this branch:** migration beyond one site, PC-based
  method/file overrides, per-channel destination routing, logrus
  removal. All listed in [Deferred](#what-we-are-deliberately-deferring)
  with follow-up sketches.

The rest of this document is supporting detail for each piece of
the diagram, in roughly call-flow order.

## Foundation: slog directly

We use the standard `log/slog` package as the API contract — at every
call site and across our package boundary. Reasons:

- It is in the standard library and is the de facto destination for
  third-party loggers to converge on. Anything we build on top
  composes with future libraries.
- Handlers are an interface (`slog.Handler`) with a clear contract:
  `Enabled`, `Handle`, `WithAttrs`, `WithGroup`. We control formatting,
  buffering, output policy, and any drop logic by writing one handler
  type.
- We are not betting on a non-standard logging API surviving a Go
  ecosystem shift. If slog is the long-term winner (it appears so),
  call sites do not need to migrate again.

We write all handler pieces ourselves: async wrapper, JSON-shape
coercion, named-logger registry, override map, and the pretty
console handler.

### Why we hand-roll the pretty handler (and don't use df/dl)

`df/dl` is a slog-based logging package by the same author as
pfxlog (upstream: `github.com/michaelquigley/df`), and its
`PrettyHandler` is a direct port of pfxlog's pretty formatter. An
earlier revision of this branch used it for console output, with an
`openziti/df` mirror as a stability fallback. We dropped it after
review found behavior gaps that `dl.Options` cannot reach:

- The level switch in dl's `Handle` covers only the four standard
  slog levels; our custom Fatal, Panic, and Trace levels render a
  blank label. A post-Install `logrus.Fatal` losing its FATAL marker
  in operator logs was the blocking finding.
- Color sequences for the timestamp/function/fields segments are
  written unconditionally; `UseColor=false` only suppresses the
  resets, and `DefaultOptions()` bakes color into the level labels
  at construction time. The "text" (no color) format is therefore
  unachievable through options.
- Terminal detection stats `os.Stdout` while ziti logs to stderr,
  and df's defaults differ from pfxlog's (`StartTimestamp` is
  process start rather than start-of-day, no `TrimPrefix`).

`logging.PrettyHandler` (`common/logging/pretty.go`) replaces it:
the same pfxlog output shape, all seven level labels, color fully
gated and keyed on the actual output writer (with `PFXLOG_USE_COLOR`
still honored), `_channels`/`_context` rendering, record-time
timestamps, and a correct `WithAttrs`. It is ~150 lines, the size
the design always assumed a hand-rolled replacement would be.

Worth knowing about dl for context: its JSON path is *just*
`slog.NewJSONHandler` with no `ReplaceAttr` (stdlib shape, not
pfxlog-compatible), and its channel registry / `dl.Info` helpers
are a non-standard API surface we never wanted at call sites. So
nothing else in dl was load-bearing for us.

## Coexistence with pfxlog/logrus

This is the central design decision. We have ~3000 call sites that
use pfxlog. We will not touch them in this branch.

The trick is that **logrus and slog share the same sink**, and the
sink is the async slog handler. We achieve this with a `logrus.Hook`.

A single startup call owns the entire logrus-side configuration:

```go
// logging.Install is the only function that mutates logrus's
// output and formatter. Anything that previously called
// pfxlog.GlobalInit no longer does so - we skip pfxlog's init
// entirely.
//
// After Install, logrus.SetLevel is only ever called from the
// agent level handler (legacy framed and v2 commands), which
// also drives globalLevel.Set. The two stay in lockstep at the
// global level.
func Install(handler slog.Handler, initialLevel slog.Level) {
    logrus.SetOutput(io.Discard)          // logrus never writes directly
    logrus.SetFormatter(noopFormatter{})  // skip the format pass too
    logrus.SetLevel(slogToLogrus(initialLevel))  // matches global
    logrus.AddHook(slogBridge{handler: handler})
}
```

### Why no `pfxlog.GlobalInit()`

pfxlog's init configures the logrus formatter, output, level, and
some hooks. With the bridge in place:

- The logrus formatter never runs in earnest (we replace it with a
  noop). slog's handler does the rendering from the bridged record.
- Logrus's output is `io.Discard`. Anything pfxlog would have set
  here is overwritten.
- The level on logrus matches the slog global level (and gets
  updated only via the agent level handler, alongside
  `globalLevel.Set`). logrus's own `IsLevelEnabled` check pre-
  filters below-threshold legacy records *before* acquiring the
  global mutex - meaningful savings on high-volume Debug paths
  like fabric's per-payload logging.
- Field/prefix extraction at call sites uses pfxlog's `Logger()` /
  `ChannelLogger()` helpers, which construct `logrus.Entry` values
  directly. Those work without `GlobalInit` ever being called.

So we just don't call `pfxlog.GlobalInit()`. There is nothing in
it we still want after the bridge lands.

### Bridge install ordering

One rule: **`logging.Install()` is the only logrus-mutating call in
startup.** Anything else that calls `logrus.SetOutput`,
`logrus.SetFormatter`, `logrus.AddHook`, or `logrus.SetLevel` after
`Install` would silently undo part of the bridge. Code review owns
this; no runtime sentinel.

**Where Install's inputs come from.** Logging configuration in ziti
binaries is CLI-flag only today (no config-file logging block):
`--verbose`, `--log-formatter`, and any related flags are parsed by
the cobra command before `controller.Run`/`router.Run` is invoked.
By the time `Install` runs at the top of `Run`, all of its inputs
- initial global level, output destination, format selection
(JSON for prod, pretty for dev), `AsyncOptions.QueueSize`,
`AsyncOptions.BlockThreshold` - are already known. There is no
"start with safe defaults, reconfigure later" phase.

Concretely:

```go
// main / cobra command bind
opts := logging.OptionsFromFlags(flags)  // pure read of parsed flags

// inside controller.Run / router.Run
handler := logging.BuildHandler(opts)    // builds the slog handler
                                         // chain (Async wrapping
                                         // JSON or Pretty)
logging.Install(handler)                 // freezes logrus into the
                                         // bridge configuration
```

If a future need arises to drive logging from the config file
(per-name overrides at boot, JSON destination paths, etc.), it
lands as `logging.Reconfigure(opts)` that atomically swaps the
downstream slog handler held behind a mutex inside the
`AsyncHandler`. The queue, drain, and `SyncEmit` mutex all stay;
only the downstream handler pointer changes. Crucially, **`Reconfigure`
never touches logrus** - the bridge is configured exactly once by
`Install` and the per-record bridge `Fire` reads the downstream
through the same pointer. That preserves the invariant.

The call happens at the very top of `controller.Run()` and
`router.Run()`, ahead of anything else those functions trigger.
Concretely, `Install` must complete before:

- the first pfxlog or logrus log call from any goroutine,
- config loading (which logs as it parses),
- any subsystem initialization that constructs loggers or logs,
- any goroutine spawn.

Package `init()` functions in pfxlog/logrus only register types and
do not produce log output, so they are not part of the ordering
constraint - the constraint is on the first *use*, not on package
loading. Any code added later that logs from an `init()` would
violate the rule; the developer note for the package calls this
out as a thing not to do.

The hook implementation:

```go
func (b slogBridge) Fire(e *logrus.Entry) error {
    // logrus already pre-filtered this record by its global level,
    // which tracks slog's globalLevel. Per-channel overrides do
    // not apply to bridged records (see "Named-logger overrides"
    // below for the slog-only constraint), so there is no further
    // gate to evaluate here.
    level := mapLevel(e.Level)
    r := slog.NewRecord(e.Time, level, e.Message, /*pc=*/0)
    for k, v := range e.Data { r.AddAttrs(slog.Any(k, v)) }

    // Fatal and Panic must be durable before logrus exits the
    // process; see "Fatal/Panic flush contract" below.
    if level >= logging.LevelFatal {
        return logging.SyncEmit(e.Context(), r)
    }
    return logging.RootHandler().Handle(e.Context(), r)
}
```

Three things this pins down:

- **logrus level tracks the slog global level.** Install sets
  logrus to the operator's level; the agent level handler updates
  both `logrus.SetLevel` and `globalLevel.Set` together. logrus's
  own `IsLevelEnabled` check pre-filters below-threshold records
  before the global mutex is touched, which is the cheap
  filtering path that matters under high-volume Debug workloads
  (e.g. fabric's per-payload logs).
- **The bridge does no per-channel routing.** Bridged records go
  straight to the root async handler. Per-channel level overrides
  are a slog-only feature, exposed only through `logging.For(name)`.
  pfxlog.ChannelLogger("X") records are filtered solely by the
  global level; the operator command `set-channel-log-level X
  debug` only changes the behavior of `logging.For("X")` callers.
  See [Named-logger overrides](#named-logger-overrides-this-branch)
  for the constraint and its rationale.
- **The bridge does not call `Enabled`.** Because logrus has
  already filtered by global level, the only records reaching
  `Fire` are ones we want to emit. Skipping the `Enabled` check
  saves one atomic load per bridged record.

### Fatal/Panic flush contract

`logrus.Fatal` calls `os.Exit(1)` immediately after hooks return.
`logrus.Panic` re-panics. In both cases the process is on its way
out before the AsyncHandler drain can write the record downstream.
A non-blocking enqueue would lose the very record an operator most
wants in the post-mortem.

The contract:

- At levels `>= logging.LevelFatal` (fatal and panic), the bridge
  and any direct slog caller bypass the async queue and write
  synchronously to the downstream handler before returning.
- `logging.SyncEmit(ctx, r)` is the entry point: it formats and
  writes the record through the same downstream JSON handler the
  async drain uses, on the calling goroutine. After it returns,
  the record is on disk (or whatever stderr is wired to).
- For direct slog callers, `slog.Log(ctx, logging.LevelFatal, ...)`
  routes through the registry handler whose `Handle` does the same
  level check and dispatches to `SyncEmit` for fatal/panic.

Synchronization considerations:

- The downstream JSON handler is guarded by `downstreamMu`, shared
  with the drain goroutine. `SyncEmit` takes `downstreamMu`,
  writes, drops it. Contention is negligible because fatal/panic
  records are rare; under normal load the drain holds the mutex
  uncontended.
- The async queue is *not* drained as part of a fatal call. We
  could pre-drain to keep ordering tight, but it adds latency on
  the exit path and risks deadlock if the drain goroutine is stuck.
  Out-of-order final lines are an acceptable trade vs. losing the
  fatal record entirely.
- `SyncEmit` writes regardless of `Enabled`. The bridge / direct
  caller has already done the gate; `SyncEmit`'s job is only "get
  this record out durably."

Tests:

- A subprocess test (`logging_fatal_test.go`) spawns a child
  process that calls `logrus.Fatal("expected fatal")`, captures
  the child's stderr, asserts the line is present and parses as
  the expected JSON shape. Same shape for `logrus.Panic` (panic
  recovered or not, the bridged record must be visible before the
  panic propagates).
- Direct slog version: child calls
  `slog.Log(ctx, logging.LevelFatal, "expected fatal slog")`
  followed by `os.Exit(1)`; assert the record made it to stderr.
- Negative: a non-fatal record (Info) is *not* synchronously
  emitted; the test confirms it goes through the async queue and
  is delivered on close.

This fatal/panic path follows the same bridge contract described
above: logrus tracks the slog global level, bridged records are
already globally filtered before `Fire`, and per-channel overrides
remain a direct-slog-only feature exposed through `logging.For(name)`.

Field iteration order: `logrus.Entry.Data` is a Go map, so the
attrs land on the slog record in random order. Downstream JSON
parsers do not care about key order, so we do not pay the cost of
sorting on every bridged record. The format-compatibility test
parses both expected and actual output as JSON and compares
key-by-key (see [Format compatibility](#format-compatibility)).

### Bridge enqueue policy

The bridge and direct slog callers share the same handler entry
point (`Handle`) and the same `BlockThreshold` rule. Bridge `Fire`
holds the logrus global mutex throughout, so if the queue is
saturated and the record's level is above the block threshold,
the mutex stays held while `Fire` waits for queue space. That is
deliberate: a blocked Warn/Error is the contract we want, and
under realistic load it only happens if some Warn/Error site is
flooding the queue faster than the drain can sustain.

We do not preemptively downgrade existing Warn/Error sites to
avoid this. The math (mutex-held time per call drops from
`format+write` to `hook+enqueue`) plus the 4096-deep queue gives
substantial headroom over the volume that broke the system today.
The expected outcome is that the foundation absorbs typical
bursts without saturating; per-site remedies (rate limit, sample,
downgrade) are reserved for sites that measurement proves still
contend. See
[Where the drop decision lives](#where-the-drop-decision-lives)
for the measurement plan and the remedy ladder.

Properties of this design:

- The logrus global mutex is **still held** during `Fire`. What
  changes is what runs inside the lock: today the mutex covers
  level filter + hooks + formatter + write to stderr. After the
  bridge it covers level filter + hooks (the bridge being one of
  them) + a noop formatter call + a `Write` to `io.Discard`. The
  bridge itself does record allocation, field copy, and a
  non-blocking send into our queue - microseconds at most. The
  *reduction* in lock-hold time is meaningful (no format work, no
  syscall write), but it is a reduction, not an elimination. Some
  logrus framing remains and is version-sensitive; we should not
  claim the mutex effectively goes away.
- Both legacy pfxlog/logrus call sites and new slog call sites land in
  the same destination, in the same order they arrived at the sink.
- We can migrate any individual call site at any time without
  visible behavior change.
- If we ever want to delete logrus, we just remove the hook and the
  legacy dep; new code is already direct slog calls.

**Caller info via the bridge.** pfxlog already records the caller's
file and function in `logrus.Entry.Data` (keys `file` and `func`). The
bridge copies those as `slog.Attr`s on the bridged `slog.Record`. No
second stack walk; legacy call sites keep their caller info for the
cost of a map lookup. New code uses slog's native PC mechanism (see
"Async handler" below).

## Format compatibility

Downstream consumers (log aggregation, alerting, dashboards) parse the
JSON shape that pfxlog/logrus currently emits. We do not want this
refactor to break that contract.

Target shape, fixed:

```
{"time":"2026-05-19T...","level":"info","msg":"...","file":"foo.go:42",
 "func":"package.Func","customField":"...","..."}
```

Notably:

- `level` is lowercase. slog's default is uppercase (`"INFO"`).
- `file` and `func` are flat top-level string keys. slog's default
  groups them under `source` as a nested object.
- `time`, `level`, `msg` are the canonical first three keys; custom
  attrs follow.

The shape is enforced via `slog.HandlerOptions.ReplaceAttr`:

```go
slog.NewJSONHandler(out, &slog.HandlerOptions{
    AddSource: true,
    Level:     globalLevel,
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        switch a.Key {
        case slog.LevelKey:
            // Always go through logging.LevelName: slog.Level.String()
            // returns "DEBUG-4" / "ERROR+4" / "ERROR+8" for our custom
            // trace/fatal/panic values. strings.ToLower would emit
            // "debug-4" which is not a valid level name and breaks
            // downstream filters.
            lvl := a.Value.Any().(slog.Level)
            return slog.String(a.Key, logging.LevelName(lvl))
        case slog.SourceKey:
            // flatten source.{function,file,line} to flat func/file
            src := a.Value.Any().(*slog.Source)
            return slog.Attr{} // suppress the source attr itself
            // file/func are added separately below in the handler wrapper
        }
        return a
    },
})
```

`logging.LevelName` is the single source of truth for level names
on the wire. It is the same function the agent IPC handler uses to
parse incoming level strings (its inverse is `logging.ParseLevel`).
Mapping:

```go
// Defined in common/logging/levels.go next to the custom Level
// constants. Returns one of: "trace", "debug", "info", "warn",
// "error", "fatal", "panic".
func LevelName(l slog.Level) string {
    switch l {
    case LevelTrace: return "trace"   // custom: slog.LevelDebug - 4
    case slog.LevelDebug: return "debug"
    case slog.LevelInfo:  return "info"
    case slog.LevelWarn:  return "warn"
    case slog.LevelError: return "error"
    case LevelFatal: return "fatal"   // custom: slog.LevelError + 4
    case LevelPanic: return "panic"   // custom: slog.LevelError + 8
    }
    // For values in between (a caller using slog.LevelDebug+1 etc.):
    // fall back to slog's offset rendering, lowercased, so the wire
    // never carries silent garbage.
    return strings.ToLower(l.String())
}

func ParseLevel(name string) (slog.Level, error) { /* inverse */ }
```

Anywhere the codebase emits a level name on the wire - JSON output,
agent IPC responses, structured fields - it must call
`LevelName`. The shared function makes "ReplaceAttr and the agent
handler agree" a property of the type system rather than a thing
to remember.

The handler wraps source-flattening so the final record carries
`file` and `func` as top-level keys, matching pfxlog's shape.

**Reserved-key collisions.** If a caller logs an attr named `time`,
`level`, `msg`, `file`, or `func`, the handler emits *both* the
built-in key and the caller's attr. The resulting JSON has
duplicate keys for that name. This is slog's standard
`JSONHandler` behavior - it deliberately does not sanitize, rename,
or warn. We adopt the same policy: do not add a slog-rewriting
layer to suppress collisions. Downstream JSON parsers vary in how
they resolve duplicates (most take last-wins); relying on
collision behavior in caller code is a caller bug. The format-test
fixture pins this: one record exercises a reserved-key collision
and is captured byte-for-byte in the golden file (see test
strategy below).

**Regression test (golden file).** A `common/logging/format_test.go`
emits a fixed sequence of log lines through both `pfxlog.Logger().Info`
(through the bridge) and direct `slog.Info` calls, and captures
stderr. For each line, both the captured output and the
corresponding golden entry are decoded as JSON with
`json.Decoder.UseNumber()` (so numeric values land as `json.Number`
strings rather than `float64`), and the resulting maps are compared
key-by-key after normalizing volatile fields (timestamps, PCs).
Two reasons for `UseNumber`:

- Wide integers (greater than 2^53) survive comparison. Without
  `UseNumber`, an expected `9007199254740993` and an actual
  `9007199254740992` both decode to the same `float64` and the
  test passes silently on drift.
- Numeric type drift (an integer emitted as `42.0` instead of
  `42`) shows up as a token mismatch rather than vanishing into
  float coercion.

Parsed-map comparison absorbs the random key-order in the bridge's
iteration over `logrus.Entry.Data` without forcing per-record
sorting. The same test double-serves as a contract document for
downstream consumers - the golden file *is* the spec.

**Two-mode comparison.** Each golden fixture row is tagged either
`parsed` (default) or `raw`. Parsed rows go through the
UseNumber-decode-and-compare-maps path described above. Raw rows
are compared byte-for-byte against the captured line after
timestamp and PC normalization - no JSON parsing, no map merging.
The raw mode is used for fixture rows that intentionally produce
output a parsed-map comparison would mishandle, specifically the
reserved-key collision rows where slog emits duplicate JSON keys.
Tagging is a one-character marker in the fixture; most rows are
`parsed`.

**Fixture coverage.** Map-key comparison catches missing or extra
keys, but it will not catch differences in how slog and logrus
render specific attr value types or how custom levels are named.
The fixture must include at least one line exercising each of:

- **Every level name on the wire**: one record at each of `trace`,
  `debug`, `info`, `warn`, `error`, `fatal`, `panic`. Catches the
  custom-level rendering trap (`slog.Level.String()` returns
  `"DEBUG-4"` / `"ERROR+4"` / `"ERROR+8"` for our trace/fatal/panic
  values; only `LevelName` produces the canonical strings). The
  panic and fatal rows go through the `SyncEmit` path; the test
  asserts they appear in the output regardless.

- **String** with embedded characters (tagged `raw`): a double
  quote, a newline, a backslash, a tab. Catches escape-rule drift
  between encoders, which parsed-map comparison cannot - decoded
  strings collapse `\t`, a literal tab, and `	` to the same
  byte. This row also doubles as the pin for canonical key order
  (`time`, `level`, `msg` first), since byte-equal comparison
  preserves position. If the encoder later changes escape form or
  field order, the raw row diverges from the golden file and the
  test fails loudly.
- **Numeric**: an integer that fits in int64, a float with a
  non-trivial fractional part, and an integer wider than 2^53
  (catches naive float coercion).
- **Boolean**: `true` and `false`.
- **`error`**: a non-nil error value whose `Error()` method returns
  a string. The shape should be the same as logrus emits today
  (typically the string, not a wrapping object).
- **`fmt.Stringer`**: a custom type whose `String()` returns a known
  value. Confirms slog and logrus agree on calling `String()`.
- **Nil**: an explicit `nil` value passed as an attr. Both sides
  should render the same shape (typically JSON `null`).
- **Time**: a `time.Time` value as an attr (separate from the
  record's own `time`). Catches RFC3339 format drift.
- **Nested group**: a record emitted through
  `logger.WithGroup("g").Info("msg", "k", "v")` so the fixture
  pins the nested-object shape on the slog path (no logrus
  equivalent; the bridge will never emit this since groups are
  slog-only).
- **Reserved-key collision** (tagged `raw`): one record that logs
  an attr with each of the reserved key names (`time`, `level`,
  `msg`, `file`, `func` — combined into a single line is fine).
  The golden line captures the duplicate-keys-in-JSON output that
  slog produces. This row uses byte-for-byte comparison after
  normalization, not parsed-map comparison, because decoding the
  line into `map[string]any` would silently collapse the
  duplicates.

The fixture entries for the slog-only cases (grouped attr) live
only on the direct-slog side of the test matrix; bridged-logrus
entries skip those lines.

## Async handler

The new package lives at `common/logging/` and exposes:

```go
package logging

// AsyncHandler wraps a downstream slog.Handler with a bounded queue
// and a drain goroutine. Handle() does a non-blocking send onto the
// queue; behavior under saturation depends on the level vs. the
// configured block threshold.
type AsyncHandler struct { /* ... */ }

func NewAsyncHandler(downstream slog.Handler, opts AsyncOptions) *AsyncHandler

type AsyncOptions struct {
    QueueSize        int               // bounded channel capacity, e.g. 4096
    SummaryInterval  time.Duration     // how often to emit drop-summary line, e.g. 5s
    // Records at or above BlockThreshold block when the queue is full;
    // records below it are dropped and counted toward the next summary.
    // Default: slog.LevelWarn.
    BlockThreshold   slog.Level
}
```

Drain semantics:

- One drain goroutine per `AsyncHandler` instance. Loop reads
  `slog.Record` from queue, calls `downstream.Handle(ctx, r)`
  under a small `downstreamMu`.
- The downstream handler is one of: a `slog.NewJSONHandler` wrapped
  with our pfxlog-shape `ReplaceAttr` (production); a
  `dl.NewPrettyHandler` (dev/console). Both write to `os.Stderr`
  matching what we ship to fablab today.
- The downstream handler has two writers: the drain goroutine for
  normal records, and `SyncEmit` for Fatal/Panic records that must
  not wait for the queue. Both take `downstreamMu` before calling
  `downstream.Handle(...)`. Contention is negligible because
  SyncEmit is rare (only at fatal/panic, which by definition
  terminate the process); under normal load the drain holds the
  mutex uncontended.

Drop accounting:

- Atomic counter per level, incremented on drop (cheap, by the
  producer).
- The summary line is emitted by the **same drain goroutine** that
  writes data records, not by a separate ticker. The drain's
  `select` includes a `summaryTicker.C` arm; on each tick the drain
  snapshots and zeroes the counters, builds a summary record, and
  calls `downstream.Handle(...)` from the same goroutine that
  handles every other record:

  ```go
  for {
      select {
      case q := <-h.queue:
          h.downstream.Handle(q.ctx, q.record)
      case <-summaryTicker.C:
          if drops := h.snapshotDrops(); drops.any() {
              h.downstream.Handle(context.Background(), buildSummaryRecord(drops))
          }
      case <-h.closeNotify:
          // see Shutdown below
      }
  }
  ```

  Each queued record carries the originating call's `context.Context`,
  so downstream handlers that consult context values (tracing, OTel)
  see them across the async hop. Summary records are synthetic and
  have no originating call, so they use `context.Background()`.

  Both the data-record write and the summary write happen on the
  drain goroutine, both under `downstreamMu`. `SyncEmit` (rare) is
  the only other writer and takes the same mutex. The downstream
  handler does not need to be concurrency-safe on its own; the
  mutex provides the serialization. Tradeoff: drain blocks for the
  duration of a summary build (microseconds) between data records
  on tick boundaries. Negligible at any realistic
  `SummaryInterval`.

Shutdown:

The queue channel is **never closed**. Producer-side log calls can
race with shutdown arbitrarily, and a `send on closed channel` panic
during teardown is exactly the failure mode this design must avoid.
Instead, lifecycle is managed by two cheap signals:

```go
type AsyncHandler struct {
    queue       chan slog.Record  // bounded, never closed
    closeNotify chan struct{}     // closed by Close()
    drainDone   chan struct{}     // closed by drain on exit
    closed      atomic.Bool
    // ...
}
```

- **`Handle` after Close.** Producers check `h.closed.Load()` first
  and return `nil` if already closed; the record is silently
  dropped. This is documented best-effort behavior: shutdown drops
  late writes rather than panic. There is one residual race - a
  producer that passes the `closed.Load()` check just before Close
  fires can reach the queue send. In the non-blocking enqueue arm
  that send either succeeds or hits `default` and counts as a drop.
  In the blocking arm (level at or above `BlockThreshold`), the
  select includes `<-closeNotify` so it cannot block forever.

  **A successful enqueue during shutdown does not guarantee
  delivery.** A non-blocking send that wins the race against the
  drain's final flush may land in the queue after the drain has
  already emptied it and exited. Those records are abandoned, not
  written downstream. Callers that need at-least-once delivery
  cannot rely on a returned `nil` from `Handle` during shutdown -
  the contract is "best-effort during Close." In practice, log
  output during shutdown is already on a best-effort footing
  everywhere upstream of us; this just makes the boundary explicit.
- **Drain goroutine.** Loops on
  `select { case r := <-queue: ... case <-closeNotify: ... }`. On
  `closeNotify`, the drain switches to a final-flush mode: it keeps
  doing non-blocking receives from `queue` until one returns "not
  ready," then emits a final drop-summary if any drops are pending
  and closes `drainDone`.

  **The drain must continue consuming until the queue is empty
  *after* `closeNotify` fires.** A producer in the blocking-arm
  select can be parked on `case queue <- r:`; if the drain stops
  reading from `queue` the moment `closeNotify` fires, that
  producer parks forever and blocks the goroutine that is calling
  `Handle`. The flush loop pattern (`for { select { case
  <-queue: ... default: return } }`) reads everything that has been
  enqueued or is in-flight, then exits. This guarantees no
  producer goroutine remains blocked on the queue at the moment
  the handler is considered closed.
- **`Close()`** calls `closed.CompareAndSwap(false, true)`, closes
  `closeNotify`, and returns immediately. Idempotent: a second
  Close also returns immediately (CAS short-circuits). Wired into
  the existing controller/router shutdown notify chain so callers
  do not need to invoke it directly.

  The contract is "shutdown is initiated; the drain will complete
  in bounded time," not "drain has completed when Close returns."
  Bounded time = current queue depth / drain rate, typically a
  few hundred milliseconds with a 4096-deep queue and ~10k
  records/sec drain. Close does not wait so process shutdown is
  not held up; pending Info/Debug at exit is best-effort and may
  be lost if the process exits before the drain finishes.
  Fatal/Panic records bypass the queue entirely via
  [SyncEmit](#fatalpanic-flush-contract), so the records that
  matter most at exit are already durable before Close is ever
  called.

  `drainDone` is exposed only for test infrastructure that needs
  to assert "all enqueued records have been written." Production
  code does not wait on it.

Caller info:

- `slog.NewRecord` captures the caller PC cheaply (one
  `runtime.Callers` call, ~hundreds of ns). The PC rides along with
  the record on the queue.
- PC → file/func/line decoding happens in the **downstream handler**
  on the **drain goroutine**, not on the caller. The decode is the
  expensive part (`runtime.FuncForPC` + `Frames.Next`); pushing it
  off the hot path is one of the structural wins of going async.
- Today's pfxlog does this decode synchronously on every call. New
  slog call sites are strictly faster here.

### slog.Handler contract

`AsyncHandler` implements `slog.Handler`. Two parts of the contract
need explicit treatment because the async-queue shape makes them
non-trivial:

**`Handle(ctx, r)` and Record retention.** slog passes `Record` by
value, so we get a stack-copied struct (Time, Message, Level, PC,
inline 5-attr array, and the `back []Attr` slice header). The
backing array of `back` is shared with the caller. Cloning the
record with `Record.Clone()` would be defensive against callers who
retain and mutate the record after `Handle` returns, but in standard
slog usage (`slog.Info(...)`, `logger.With(...).Info(...)`) the
record is built per call and goes out of scope immediately. We
**document the assumption** rather than clone:

> Callers must not retain references to or mutate a `slog.Record`
> after passing it to `Handle`. Pooled-Record patterns where a
> single record is reused across multiple `Handle` calls are
> unsupported. If a future caller needs that, `Handle` adds a
> `Clone()` then.

This saves the per-call clone allocation. The risk window is
exotic-Handler-wrapping behavior we control.

**`WithAttrs` and `WithGroup`.** These are called when callers do
`logger.With(...)` or `logger.WithGroup(...)`. The returned handler
must remember the bound attrs/groups **in the order they were
bound** and apply them to every record that flows through.

The naive design - store `attrs []slog.Attr` and `groups []string`
as two flat slices on one handler - loses the interleaving between
them. `With("a",1).WithGroup("g")` and `WithGroup("g").With("a",1)`
should produce different output (the first puts `a` at the root,
the second nests `a` inside `g`) but with flat slices they store
identical state. Silent wrong output.

Correct shape: a chain of small handler wrappers. Each
`With`/`WithGroup` call returns a wrapper that knows only its own
scope. The chain itself encodes the binding order. At `Handle`
time, each wrapper transforms the record and delegates to its
parent; the root enqueues. Three handler types:

```go
// 1. AsyncHandler is the exported root. One per process. Owns the
//    queue, drain, shutdown, drop counters, and the global-level
//    Enabled check. Also the leaf of the wrapper chain - there is
//    no separate "root" type behind it.
type AsyncHandler struct {
    queue       chan queuedRecord
    closeNotify chan struct{}
    drainDone   chan struct{}
    closed      atomic.Bool
    dropCounts  [numLevels]atomic.Int64
    downstream  slog.Handler
    // ...
}

// queuedRecord carries both the record and the context from the
// originating Handle call. Most downstream handlers (JSON, pretty)
// do not consult ctx, but tracing or OTel-style handlers wired in
// later may read context values (span ID, trace ID, etc.). Holding
// the ctx on the queued record preserves that path through the
// async hop.
type queuedRecord struct {
    ctx    context.Context
    record slog.Record
}

func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &boundHandler{parent: h, attrs: slices.Clone(attrs)}
}
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
    if name == "" { return h }
    return &groupedHandler{parent: h, name: name}
}

// 2. Carries bound attrs that prefix subsequent records.
type boundHandler struct {
    parent slog.Handler
    attrs  []slog.Attr
}

func (h *boundHandler) Handle(ctx context.Context, r slog.Record) error {
    r2 := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
    r2.AddAttrs(h.attrs...)                    // bound attrs first
    r.Attrs(func(a slog.Attr) bool {           // record attrs after
        r2.AddAttrs(a)
        return true
    })
    return h.parent.Handle(ctx, r2)
}
func (h *boundHandler) WithAttrs(more []slog.Attr) slog.Handler {
    // Extend in place rather than wrap-wrap-wrap; same parent.
    combined := make([]slog.Attr, 0, len(h.attrs)+len(more))
    combined = append(combined, h.attrs...)
    combined = append(combined, more...)
    return &boundHandler{parent: h.parent, attrs: combined}
}
func (h *boundHandler) WithGroup(name string) slog.Handler {
    return &groupedHandler{parent: h, name: name}
}
func (h *boundHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.parent.Enabled(ctx, level)
}

// 3. Wraps subsequent attrs in a named group.
type groupedHandler struct {
    parent slog.Handler
    name   string
}

func (h *groupedHandler) Handle(ctx context.Context, r slog.Record) error {
    // Pull all of r's attrs and re-emit them inside a slog.Group.
    var items []any
    r.Attrs(func(a slog.Attr) bool {
        items = append(items, a)
        return true
    })
    r2 := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
    r2.AddAttrs(slog.Group(h.name, items...))
    return h.parent.Handle(ctx, r2)
}
// Bound attrs go INSIDE this group: wrap self in a boundHandler so
// the bound attrs land at this scope when grouped.Handle runs.
func (h *groupedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &boundHandler{parent: h, attrs: slices.Clone(attrs)}
}
func (h *groupedHandler) WithGroup(name string) slog.Handler {
    return &groupedHandler{parent: h, name: name}
}
func (h *groupedHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.parent.Enabled(ctx, level)
}
```

**`WithGroup("")` is a no-op.** slog convention: an empty group
name does not open a new scope. All three `WithGroup`
implementations above return the receiver unchanged when `name`
is `""`:

```go
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
    if name == "" { return h }
    return &groupedHandler{parent: h, name: name}
}
// same guard in boundHandler.WithGroup and groupedHandler.WithGroup
```

This matches what `slog`'s standard handlers do and avoids
allocating an empty-name `groupedHandler` that would later try to
emit `slog.Group("", ...)` (which itself flattens to its children
in the standard implementation but creates an awkward extra
layer here). A unit test confirms
`logger.WithGroup("").Info(...)` produces the same output as
`logger.Info(...)`.

Worked example:

```
log.With("a", 1).WithGroup("g").Info("msg", "b", 2)

  chain:  AsyncHandler
            ← boundHandler{attrs=[a:1]}
              ← groupedHandler{name="g"}
                ← Info builds Record{attrs=[b:2]}

  Handle traversal (leaf to root):
    groupedHandler.Handle:  wraps [b:2] in Group("g", b:2)
                            -> Record{attrs=[g:{b:2}]}
    boundHandler.Handle:    prepends [a:1]
                            -> Record{attrs=[a:1, g:{b:2}]}
    AsyncHandler.Handle:       enqueue

  output: {msg, a:1, g:{b:2}}     ✓
```

Reverse order:

```
log.WithGroup("g").With("a", 1).Info("msg", "b", 2)

  chain:  AsyncHandler
            ← groupedHandler{name="g"}
              ← boundHandler{attrs=[a:1]}      (created by group.WithAttrs)
                ← Info builds Record{attrs=[b:2]}

  Handle traversal:
    boundHandler.Handle:    prepends [a:1] -> Record{attrs=[a:1, b:2]}
    groupedHandler.Handle:  wraps in Group("g", a:1, b:2)
                            -> Record{attrs=[g:{a:1, b:2}]}
    AsyncHandler.Handle:       enqueue

  output: {msg, g:{a:1, b:2}}     ✓
```

Nested groups (`log.WithGroup("a").WithGroup("b").Info(...)`)
compose the same way: each `groupedHandler.Handle` re-emits the
inner state under its own group name, producing
`{a:{b:{...}}}`. The downstream JSON handler renders the nesting
naturally; nothing else needs to be group-aware.

Costs:

- `With`/`WithGroup` allocate one small wrapper (~32-64 bytes plus
  the attrs slice). Paid once per logger creation - typically at
  startup or first use of `logging.For(name)`, not per log call.
- `Handle` allocates one new `slog.Record` per wrapper in the chain
  that does work. Only callers actually using `With`/`WithGroup`
  pay this; direct `slog.Info` on the root or on a registry-bound
  handler with no `.With(...)` chain bypasses the wrapper code path
  entirely.
- `Enabled` delegates straight to the parent. The async root's
  `Enabled` is the only one that does real work (the registry-bound
  level check); wrapper `Enabled` is one pointer hop per layer.

Tests must cover at least:

- `slog.Default().With("k", v).Info(...)` - the bound attr appears
  in the output.
- `slog.Default().WithGroup("g").Info("msg", "b", 2)` - the attr
  is nested under group `g`.
- `slog.Default().With("a",1).WithGroup("g").With("b",2).Info("msg","c",3)`
  - `a` at root, `b` and `c` inside `g`.
- `slog.Default().WithGroup("a").WithGroup("b").Info("msg","c",3)`
  - `c` inside `{a:{b:...}}`.
- `logging.For("network.gossip").Info(...)` - the `channel` attr
  appears in the output.
- All of the above with the async path enabled (record traveling
  through the queue and out the drain) to confirm the wrapper
  chain survives the queue serialization.

## Where the drop decision lives

The user-facing question: should hot paths call `log.LogOrDrop(...)`
explicitly, or should every `slog.Info` participate in drop semantics
implicitly?

We propose **implicit, level-based**. Rationale:

- Asking every caller to remember "this is a hot path, use the drop
  variant" is the kind of policy that breaks down in practice. The
  first time someone forgets, we are back to contention. The level
  conveys most of the same information already: Debug and Info are
  droppable under pressure; Warn and Error are not.
- `slog`'s API is a known surface. Adding a parallel
  `TryInfo`/`LogOrDrop` doubles the API and leaks the buffering
  implementation into call sites.
- Operators care about the summary line ("dropped 12k Info since
  23:14:01") more than about the individual dropped record.

Concretely: `BlockThreshold` defaults to `slog.LevelWarn`. Records
at Warn level or above block when the queue is full; Debug and
Info drop and are counted toward the next summary. Warn is
included on the block side because warn signals "something is
off" - silently dropping those is the worse failure mode. The
threshold is tunable in config but should not need site-by-site
overrides.

This rule applies to both the bridge and direct slog callers; the
two paths share `Handle` and the same enqueue semantics.

### Levels mean something: measure first, downgrade only if needed

The Warn-block rule has one load-bearing assumption: Warn/Error
volume stays below the rate the async drain can sustain. If a
site emits Warn or Error at chaos-burst rate and the drain cannot
keep up, queue saturation makes the bridge block on the logrus
mutex and recreates the original contention.

The math changes meaningfully with this branch in place. Today
each call holds the logrus mutex for `format + write` (often
hundreds of microseconds). After Install, mutex-held time per
call is "hook fire + try-or-block enqueue" (microseconds in the
unsaturated case). With a 4096-deep queue and a drain that
sustains roughly 10k records/sec writing JSON to stderr, the
observed 2700/burst/router rate does not saturate unless the
burst is both intense and sustained. So the conditions for
"we're back where we started" require materially higher
Warn/Error volume than what has been measured.

This means **we should not preemptively downgrade existing
Warn/Error sites**. Per-event Warn/Error lines have real
diagnostic value (which router reconnected, which IP attempted
the handshake) that matters most during exactly the chaotic
scenarios where they are loudest. Preemptive downgrade trades
real-time visibility for a problem we have not confirmed exists
under the new architecture. The DoS case is the sharpest: a real
attack is when you most want Error-level visibility, and a system
trained to be quiet there is trained to be quiet at the worst
moment.

Instead, **measure under realistic chaos load before changing any
levels**. The gossip-links rebase is the natural gate for this:

- Run a fablab chaos burst with the bridge in place.
- Capture: p99 logrus-mutex hold time, queue depth distribution,
  drop counts by level.
- If queue saturates and Warn/Error blocks materially (mutex hold
  time creeps back toward today's numbers), then identify the
  specific site driving the volume. The remedy ladder, in order of
  preference:
  1. **Per-site rate limit or sample** (e.g., "log at most 10/s,
     plus a `(suppressed N)` summary"). Preserves the diagnostic
     signal at a sustainable rate.
  2. **Per-site downgrade to Debug**. Acceptable when the line is
     genuinely not operator-actionable, but a last resort because
     it makes the line silently droppable.
  3. **Move `BlockThreshold` to `slog.LevelError`** as a global
     config change. Strong contract change; only if multiple
     Warn sites prove untenable and individual fixes are not
     practical.

Forward-looking rule, applied today regardless of measurement:
**any new Warn/Error in a per-event hot path is a review-blocker.**
This costs nothing to enforce on new code because new code has no
established diagnostic value yet, and the asymmetric "we need it
loud during DoS" argument does not apply to a line that has never
been deployed.

This decision is reversible. If real usage shows we need finer
control, we add a `logging.TryInfo(...)` helper in front of the same
handler. Call sites that adopt it opt into stricter drop. The default
path stays implicit.

## Runtime level control

Operators need to change log levels on a running process without
restart. Two tiers (global and named-logger overrides).

**Transport prerequisite.** This section assumes the agent IPC
capabilities work from [agent-capabilities.md](agent-capabilities.md)
has landed. That branch added channel/v4 transport to the agent
listener plus three new channel-based commands:
`SetLogLevelV2 { level: string }`,
`SetChannelLogLevelV2 { channel: string, level: string }`,
`ClearChannelLogLevelV2 { channel: string }`. Level names are
lowercase strings (`"debug"`, `"info"`, `"warn"`, `"error"`,
`"trace"`, `"fatal"`, `"panic"`). Legacy framed commands stay
as single-byte logrus.Level payloads, used by old clients only.

The logging-refactor branch does not change wire formats. It
extends the v2 handlers (which today only call `logrus.SetLevel`)
to also drive the slog side via `globalLevel.Set(...)` and the
named-logger registry. Legacy framed handlers receive the same
slog-side extension so old clients see consistent behavior on the
slog path even though they keep their byte payloads.

### Level mapping

slog has four native levels (Debug=-4, Info=0, Warn=4, Error=8) but
the level type is `type Level int`, so we define custom values to
preserve logrus's full enum without information loss:

| Name on wire | slog.Level | logrus.Level |
|---|---|---|
| `trace` | -8 (custom) | TraceLevel |
| `debug` | -4 (LevelDebug) | DebugLevel |
| `info` | 0 (LevelInfo) | InfoLevel |
| `warn` | 4 (LevelWarn) | WarnLevel |
| `error` | 8 (LevelError) | ErrorLevel |
| `fatal` | 12 (custom) | FatalLevel |
| `panic` | 16 (custom) | PanicLevel |

The custom slog values (`-8`, `12`, `16`) live as constants in
`common/logging/` next to `AsyncHandler`. Display strings (lowercase
canonical names) and parse functions are exported so the agent
command handler and any other consumer share one source of truth.

### Global level

Backed by `slog.LevelVar`, which the slog handlers consult on every
`Enabled` check. Mutation is just `Set(...)` — immediate effect, no
relock, no handler recreation:

```go
var globalLevel = new(slog.LevelVar)  // default LevelInfo

handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
    Level: globalLevel,
})
// ... wrapped in AsyncHandler etc.

// runtime:
globalLevel.Set(slog.LevelDebug)
```

The agent's level handlers - both legacy framed and channel v2 -
map their incoming level (logrus.Level byte or slog level name) to
the canonical `slog.Level`, then drive both `globalLevel.Set(slogLevel)`
and `logrus.SetLevel(slogToLogrus(slogLevel))`. The two stay in
lockstep at the global level: logrus pre-filters below-threshold
records (saving the global-mutex acquire), and the bridge then
emits the records logrus lets through to the slog sink. Bridged-
logrus and direct-slog callers see the new level immediately,
regardless of which transport the client used.

### Named-logger overrides (this branch)

The override granularity in this branch is **logger name**, not
method or file. Reason: `slog.Handler.Enabled` is called by slog
with no `Record` and no PC, so a per-PC override map in the handler
cannot re-enable a sub-global-level call - slog filters it before
our handler ever sees it. Logger-name overrides work because the
registry-bound handler's `Enabled` knows the name when called.

**Registry contract.** A name has an override **or it does not**.
There is no per-name `LevelVar` shadowing the global by default.
This matters: when no override exists, the registry's `Enabled`
reads `globalLevel.Level()` *live*, so a later `set-log-level debug`
takes effect on every previously-created named logger without any
relink step.

```go
type registry struct {
    mu        sync.RWMutex
    overrides map[string]*slog.LevelVar  // present only when set
    global    *slog.LevelVar              // shared with the root handler
    root      slog.Handler                // async handler, shared
}

// HandlerFor returns a handler that:
//   - if an override exists for name, uses that override's level
//   - else uses the live global level
// It also binds `slog.String("channel", name)` so output records
// carry the logger name.
func (r *registry) HandlerFor(name string) slog.Handler {
    return &namedHandler{registry: r, name: name}
}

func (h *namedHandler) Enabled(_ context.Context, level slog.Level) bool {
    h.registry.mu.RLock()
    override, ok := h.registry.overrides[h.name]
    h.registry.mu.RUnlock()
    if ok {
        return level >= override.Level()
    }
    return level >= h.registry.global.Level()
}

// For is the public entry point. It wraps HandlerFor in a *slog.Logger.
func For(name string) *slog.Logger {
    return slog.New(reg.HandlerFor(name))
}
```

**Set / clear semantics:**

- `SetNamedLevel(name, level)`: creates the override (or overwrites
  it) and stores it in the map. The override is a fresh `LevelVar`
  the registry owns.
- `ClearNamedLevel(name)`: deletes the entry from the map. From that
  point, `Enabled` for that name falls back to reading
  `globalLevel.Level()` live. Crucially, **clear is not "set to
  current global"** - it removes the binding entirely so future
  global changes apply automatically.

**Operator-controlled per-name level uses the new channel v2
commands** from agent-capabilities.md:

| Channel message | Effect |
|---|---|
| `SetLogLevelV2 { level }` | `globalLevel.Set(level)` **and** `logrus.SetLevel(slogToLogrus(level))` |
| `SetChannelLogLevelV2 { channel, level }` | `SetNamedLevel(channel, level)` (slog registry only) |
| `ClearChannelLogLevelV2 { channel }` | `ClearNamedLevel(channel)` (slog registry only) |

**Per-channel overrides are slog-only.** The legacy framed
`set-channel-log-level` (preserved for old clients) and the new
v2 `SetChannelLogLevelV2` both operate on the slog registry
exclusively. They do **not** drive pfxlog's existing channel-
level mechanism. Consequence: `pfxlog.ChannelLogger("X").Debug(...)`
is filtered by the global logrus level only - the operator's
`set-channel-log-level X debug` no longer affects it.

Why this constraint:

- pfxlog's channel-level mechanism is not load-bearing in
  production and is rarely used; deferring it simplifies the
  invariant.
- It establishes a clear migration carrot: code converted to
  `logging.For(name)` becomes operator-overrideable at the
  per-name level; code that stays on pfxlog gets global-level
  control only.
- The bridge fast-path (no per-record `Enabled` check, no
  registry lookup) is only sound because per-channel overrides
  don't need to be evaluated against bridged records.

Granularity is "whatever name the call site chose." A subsystem
that wants method-level granularity registers a method-specific
name (e.g., `logging.For("router.link.applyLink")`). The runtime
cost on the no-override path is the same as the standard slog
`Enabled` check - one `LevelVar.Level()` load. With an override it
adds one map lookup under RLock per call from that logger; that is
the price of having the override path at all and is in line with
slog's `WithAttrs`-bound logger overhead.

The registry stays in memory only. Runtime overrides are gone on
restart; that is a feature for short-lived surgical debugging.

**Overrides re-enable filtering; they do not raise the drop
threshold.** An operator who runs `set-channel-log-level
network.gossip debug` against a saturated process will see debug
records pass the `Enabled` check, but the async queue still drops
sub-Warn records (debug, info) when full per the implicit
level-based drop policy. Those drops are counted by the
per-level summary line, so loss remains visible. If an operator
needs guaranteed retention of a noisy debug stream, the right move
is the override plus a deliberate reduction in upstream volume,
not a threshold change.

PC-based method/file overrides (where the operator targets a call
site whose code did not pre-register a named logger) are **deferred
to a follow-up branch** - see "What we are deliberately deferring"
below for the technical sketch.

## Proof-of-pattern conversion

Pick **one** site for this branch. Candidates, ranked by contention
impact observed in fablab runs:

1. `router/link/linkState.updateStatus` Info line. Single-threaded
   event loop in the link registry hits this per state transition.
   Convert and benchmark. In-repo, well-bounded, and the call site
   sits inside the link registry event loop which we already
   understand well.
2. `router/gossipClient.HandleDigest` Info lines (gossip.go:446 and
   :452). New site identified in today's stack dumps.

Recommend (1): if we get the speedup we expect, it justifies the
migration pattern. If we do not, the assumption was wrong and we
need to revisit before expanding.

**What this branch validates — and what it does not.** Converting
`linkState.updateStatus` proves the *migration pattern* and the
*async-handler shape* work on a representative hot-path site. It
does **not** prove end-to-end contention relief under fablab chaos
load. The validation gate for "did the foundation actually fix
the problem" is the gossip-links rebase, which runs a chaos burst
with the bridge in place and measures p99 logrus-mutex hold time,
queue depth, and drop counts. If those measurements show queue
saturation or sustained blocking, the
[remedy ladder](#levels-mean-something-measure-first-downgrade-only-if-needed)
applies to the specific site driving the volume. Treat this
branch as foundation-and-pattern; treat the gossip-links rebase
as the integration that gets validated end to end.

## What we are deliberately deferring

- **Per-channel destination routing** ("logs from subsystem X go to
  a different file"). Our channel attribute on records makes this
  filterable downstream, but the in-process side writes everything
  to one destination. Add only when a concrete operational need
  shows up.
- **Sampling at the source** (drop the 100th identical
  handshake-failed within 1s). Probably the right answer for log-spam
  cases, but it is a separate concern from the contention fix and
  has its own design.
- **OpenTelemetry log signal**. Out of scope; if we adopt OTel later
  it is a downstream slog handler.
- **Removing pfxlog**. The bridge means we can leave it in place
  indefinitely; deletion is a follow-up that someone does when bored.
- **Long-term persistent overrides.** Surgical overrides are
  in-memory only. If we end up wanting "set debug on this method
  every time this process starts" we add a yaml-driven defaults
  layer separately.
- **PC-based method/file overrides.** Deferred to a follow-up
  branch. The technical reason is that `slog.Handler.Enabled` runs
  before the `Record` is built and receives no PC, so a per-PC cache
  cannot decide enablement against sub-global levels - slog filters
  the call before our handler ever sees it. The follow-up will
  implement it via a *conditional* `Enabled`: when no PC overrides
  are configured, `Enabled` honors the global level normally (zero
  overhead, identical to today). When at least one PC override
  exists, `Enabled` returns true for the affected level range so the
  handler can filter by PC in `Handle`. That trade pays a
  per-debug-call construction cost only while a PC override is
  active - an operator-initiated, short-lived state. Operational
  guardrails (TTL on overrides, `log-level resolve` to surface
  active ones) are part of that follow-up.

## Acceptance criteria

A reasonable bar for "this branch is done" is:

1. `common/logging/` package exists with `AsyncHandler` plus tests
   covering: normal flow, full-queue drop with summary, full-queue
   block at/above threshold (same rule applies to bridge and
   direct callers), shutdown flushes, concurrent `Handle` calls
   racing with `Close()` never panic and always return cleanly,
   and the slog Handler contract -
   `WithAttrs`/`WithGroup` return child handlers that share the
   root and bound attrs/groups apply to every record from a
   derived logger (including the `channel` attr from
   `logging.For(name)`). The named-logger composition test
   covers in particular
   `logging.For("router.link").WithGroup("g").With("k", "v").Info("msg", "x", 1)`
   and asserts that the output carries `channel:"router.link"` at
   the top level (bound by the registry before any composition),
   with `k`, `x`, and any further attrs nested under group `g`.
   Same record traveling through the async queue produces the
   same shape end-to-end.
2. JSON output shape matches the existing pfxlog/logrus shape
   exactly. A golden-file test (`format_test.go`) emits a fixed
   sequence through both bridged-logrus and direct-slog paths,
   decodes each captured line and its matching golden line as JSON
   with `json.Decoder.UseNumber()` (so numeric values compare as
   `json.Number` tokens rather than coerced `float64`), and
   compares them key-by-key after normalizing volatile fields
   (timestamps, PCs). Parsed-map comparison absorbs the random
   key-order in the bridge's iteration over `logrus.Entry.Data`
   without forcing per-record sorting.
3. The logrus → slog bridge is installed at controller + router
   startup. Existing pfxlog/logrus call sites continue to work, land
   in the new sink, and preserve caller info via Entry.Data
   passthrough. Subprocess tests prove that `logrus.Fatal(...)` and
   `logrus.Panic(...)` records reach stderr before the process
   exits (synchronous `SyncEmit` path), and that
   `slog.Log(ctx, logging.LevelFatal, ...)` followed by
   `os.Exit(1)` does the same on the direct slog path. A negative
   subprocess test confirms that an `Info` record relies on async
   delivery and is *not* emitted synchronously. A startup test
   passes representative CLI flags (`--verbose`, `--log-formatter
   json|pretty`) through `OptionsFromFlags` → `BuildHandler` →
   `Install` and asserts the resulting handler produces output in
   the requested format at the requested level on the very first
   bridged log line.
4. Runtime level control works end-to-end through the agent IPC
   (both the legacy framed commands and the new channel v2 commands
   from the agent-capabilities branch). Tests cover, in this order:
   - `set-log-level info` against a freshly started process: a
     prior `debug` record is filtered; a subsequent `info` record
     is seen. Verified on both bridged-logrus and direct-slog paths.
   - `set-log-level debug`: a debug record on either path is now
     seen, including from a `logging.For(name)` instance that was
     created **before** the level change (proves named loggers
     track the live global level when no override is in place).
   - `set-channel-log-level network.gossip debug` while global is
     info: debug records via `logging.For("network.gossip")` are
     seen; debug via `pfxlog.ChannelLogger("network.gossip")` is
     **filtered** (per-channel overrides are slog-only); debug via
     `pfxlog.Logger()` or `logging.For("other")` are filtered.
   - `clear-channel-log-level network.gossip` reverts that name to
     the live global level: a debug on the cleared name is filtered
     again, and a subsequent `set-log-level debug` makes it visible
     once more.
   - Throughout, the Install invariant is intact:
     `logrus.StandardLogger().Out` is `io.Discard`, the formatter
     is the bridge-owned noop, and `logrus.GetLevel()` matches
     `slogToLogrus(globalLevel.Level())`. A separate startup test
     captures these three values after `logging.Install()` returns,
     emits one bridged pfxlog line and confirms it reaches the
     async sink, then captures them again and confirms no later
     init code mutated `Out` or the formatter and that the level
     equality still holds.
5. Exactly one call site converted to direct slog as a worked
   example (proposed: `linkState.updateStatus`), and that conversion
   uses a named logger (`logging.For("router.link")` or similar) so
   the per-name override path is exercised.
6. **Rollout-discipline checklist** is documented:
   - No new Warn/Error site introduced in a per-event hot path by
     this branch or by gossip-links (review-blocker rule documented
     in the developer note).
   - The rebase from this branch into gossip-links includes a
     check step that greps for new Warn/Error in hot paths.
   - The gossip-links chaos run captures, at minimum, p99
     logrus-mutex hold time, queue depth distribution, and drop
     counts by level. Existing Warn/Error sites are left at their
     current levels; if measurement shows queue saturation or
     sustained blocking, the per-site remedy is chosen from the
     ladder in
     [Levels mean something](#levels-mean-something-measure-first-downgrade-only-if-needed).
7. A short developer note (`docs/logging.md`) explaining how to write
   a slog log line in this codebase, the named-logger convention, the
   migration approach for future PRs, and the "no new Warn/Error in
   hot paths" rule.
8. The full unit-test suite for the touched packages passes. No
   fablab-run requirement for this branch — that lands in the
   gossip-links branch which rebases on top.
9. A microbenchmark in `common/logging/` measures
   `pfxlog.Logger().Info(...)` lock-hold time under two
   configurations: baseline (logrus default formatter + stderr
   writer) and bridged (noop formatter + `io.Discard` + slog hook).
   Bridged must be measurably faster; we record the actual ratio in
   the doc so the contention-relief expectation is grounded in
   numbers rather than intuition. The benchmark doubles as a
   regression guard: anyone who later re-enables logrus's formatter
   or output will fail it.

## Open questions for the reviewer

- Is the logrus-Hook bridge the right shape, or is there a cleaner
  cut (e.g., replace pfxlog's underlying writer wholesale via a fork)?
  Both have downsides; the hook is the smaller change.
- Is `slog.LevelWarn` the right default block threshold? Resting
  on the assumption that async + fast bridge absorbs typical
  Warn/Error volume without queue saturation. Gossip-links chaos
  measurement is the gate; if it does saturate, the remedy ladder
  in [Levels mean something](#levels-mean-something-measure-first-downgrade-only-if-needed)
  picks the smallest fix that works. Moving the threshold to
  `slog.LevelError` is the last-resort option.
- Anything in the deferred list that you think actually has to be in
  this branch?
