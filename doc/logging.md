# Logging in ziti

ziti's logging foundation is `log/slog` under the hood, with a `logrus`
bridge so legacy call sites keep working unchanged. Output looks the
same as it did before (pretty by default, JSON with
`--log-formatter=json`). The reason for the change is operator
control: with slog, the agent can set the global level and lift any
*named channel* above the global without restarting the process, and
the bridge keeps logrus's mutex out of the per-record hot path for
call sites that have migrated.

This note covers what you need to know to write new code, migrate
existing code, and avoid the rough edges.

## How to write a new log line

In new code, pick a channel name for your package or subsystem and
hold a logger at package scope:

```go
package link

import "github.com/openziti/ziti/v2/common/logging"

// channelName is the agent-facing name for this package's log records.
// Operators can set its level at runtime with
//   ziti agent set-channel-log-level router.link debug
var log = logging.For("router.link")

func dial(ctx context.Context, remote *Identity) error {
    log.Info("dialing", "remote", remote.Id, "underlay", remote.Underlay)
    if err := remote.Connect(ctx); err != nil {
        log.Warn("dial failed", "remote", remote.Id, "error", err)
        return err
    }
    log.Debug("dialed", "remote", remote.Id, "elapsed", time.Since(start))
    return nil
}
```

`logging.For(name)` returns a `*slog.Logger` whose handler binds
`channel: name` as the first attr, so every record carries the
channel it came from. Loggers are cached per name, so subsequent
calls return the same pointer.

If you have nothing meaningful to channel-name (e.g. one-off CLI
glue), use `slog.Default()`. Anything that participates in the
operator's per-channel control surface should go through `logging.For`.

## Channel naming

Use `subsystem.area` form, lowercased, dot-separated. Examples that
match the structure of the codebase:

- `router.link`, `router.xgress`, `router.forwarder`
- `controller.gossip`, `controller.fabric`
- `fabric.ctrl`, `fabric.router`
- `edge.api`, `edge.identity`
- `transport.tls`

Pick the name at a *subsystem boundary*, not per-method. Method-level
channels make sense only when you've already discovered that the
subsystem channel is too coarse for triage; the default is one
channel per package or per logical area.

## The hot-path rule

Do **not** introduce Warn/Error log lines at per-event rate. A line
that fires once per packet, per circuit message, per gossip tick, or
per connection is a hot path. At those rates, every log call goes
through the leaf handler's write lock, and Warn/Error in particular
defeat the bridge's drop-summary path because they sit above the
default block threshold.

If you need to see hot-path detail, write it at Debug or Trace and
let the operator enable the channel on demand:

```go
log.Debug("payload received", "circuit", c.Id, "len", len(p.Data))
```

Reviewers will bounce conversion PRs that introduce new Warn/Error at
hot-path rates.

## Operator surface

Once a package uses `logging.For(name)`, operators can drive that
channel via the agent:

```
ziti agent set-log-level info             # global level, both slog and logrus
ziti agent set-log-level debug            # raise everything
ziti agent set-channel-log-level router.link debug   # lift one channel
ziti agent clear-channel-log-level router.link       # drop back to global
```

`set-log-level` moves both worlds in lockstep (the slog Registry's
global AND `logrus.SetLevel`), so legacy pfxlog call sites observe the
same global threshold as slog ones. `set-channel-log-level` is
slog-only by design: it lifts records emitted through
`logging.For(name)` above the global, but `pfxlog.Logger()` /
`pfxlog.ChannelLogger(name)` calls keep observing the global level
until the call site migrates. This is the migration carrot, not an
oversight.

## Migrating an existing package

Conversion is mechanical:

1. Add a package-scoped logger: `var log = logging.For("subsystem.area")`.
2. Document the channel name at the top of the package (godoc
   comment).
3. Replace `pfxlog.Logger()` and `pfxlog.ContextLogger(...)` call
   sites with `log` (or a context-derived child, e.g.
   `log.With("circuit", c.Id)`).
4. Keep every line at the level it had before. Conversion is *not* a
   level audit.
5. Don't introduce new Warn/Error at per-event rate.
6. Update tests as needed. slog uses positional `key, value` pairs
   rather than `pfxlog.WithField(...)`.

Until a package is migrated, its `pfxlog.Logger()` calls still work
(via the bridge) but the package has no per-channel agent control.

## Tunables

`AsyncOptions` controls the bridge's queue. The defaults are fine for
production; the flags exist so operators can adjust under
investigation:

| Flag | Default | What it controls |
|---|---|---|
| `--log-queue-size` | 4096 | Bounded capacity of the async log queue |
| `--log-block-threshold` | `warn` | Lowest level that blocks under queue saturation (records below this drop and bump a summary counter) |
| `--log-summary-interval` | 5s | Cadence of the drop-summary record when records have been dropped |

If you see the bridge dropping records (the periodic summary line
mentions it), the question is usually "what's emitting so much?",
not "is the queue too small?" — but the knob is there.

## Architecture, briefly

- [`common/logging`](../common/logging) is the slog foundation:
  `AsyncHandler`, the named-logger Registry, the logrus bridge, level
  helpers, and the format handlers (pfxlog-shape JSON via
  `BuildHandler`, pfxlog-shape pretty via `BuildPrettyHandler`).
- [`common/agentlog`](../common/agentlog) wires the agent's
  transport-neutral log-level commands onto
  `logging.SetGlobalLevel` / `SetNamedLevel` / `ClearNamedLevel`. The
  controller, router, and tunnel binaries each register this from
  their `PreRun`.
- Design docs:
  [logging-refactor.md](design/logging-refactor.md) and
  [logging-refactor-progress.md](design/logging-refactor-progress.md).

## Deferred, not missing

The current implementation deliberately does not include:

- PC-based method/file level overrides. The agent commands operate at
  the channel level only.
- An OTel adapter. Records still flow through the local handler
  chain; export to OTel is a future addition.
- Persistent yaml-driven level overrides. Level changes via the agent
  are in-memory and reset on restart.
- pfxlog removal. Legacy `pfxlog.Logger()` calls keep working through
  the bridge; the conversion to `logging.For` is incremental and per
  package.

These are tracked in the design doc and are explicitly out of scope
for the current foundation work.
