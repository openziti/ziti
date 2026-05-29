# Agent IPC capabilities

Branch: `agent-capabilities` (planned). Precursor to `logging-refactor`,
which rebases on top.

For reviewers in a hurry: read [Status quo](#status-quo) for the
problem, [Goals](#goals) for what success looks like, and
[High-level overview](#high-level-overview) for the architecture. The
remaining sections are supporting detail.

## Status quo

ziti's agent IPC (used by `ziti agent <command>` and the in-process
agent listener) has no protocol-versioning primitive. Each command
has a fixed wire format. Today this happens to work because every
command's encoding has been stable since it was introduced.

The existing framed commands work today. `set-channel-log-level`
already carries a `logrus.Level` byte, and `logrus.Level` covers
all seven levels we care about (panic/fatal/error/warn/info/debug/
trace). There is no immediate forced break on the wire.

What we have instead is structural debt that is about to start
costing us:

- **Wire contract welded to logrus.** The byte encoding bakes
  `logrus.Level` enum values into the IPC protocol. As we move
  the codebase to slog (see [logging-refactor.md](logging-refactor.md)),
  every framed command that carries a level is a small tax: the
  server still has to keep a logrus-shaped wire decoder forever,
  and any future field that wants a slog concept has nowhere
  natural to go.
- **No primitive for future command additions.** Every framed
  command has a hand-rolled wire shape. The next extension that
  wants new fields, structured payloads, or back-compat
  negotiation gets to invent its own scheme. We have done this
  ad-hoc in the past and have ad-hoc compatibility shims to
  prove it.
- **The controller↔router protocol already has the answer.** That
  side of the system runs on `channel/v4` with typed messages, a
  hello-time capability handshake (`common/capabilities`), and
  graceful coexistence between old and new peers. Bringing the
  same vocabulary to agent IPC means one mental model across the
  codebase rather than two.
- **Channel-based agent IPC is duplicated across ziti, and its base
  lives in an external library nobody else uses.** The base agent
  machinery (the framed protocol, AppInfo, the `gops`-style listener)
  lives in `github.com/openziti/agent`. The channel-upgrade plumbing
  on top of it (read appId byte, wrap the conn in a channel, bind
  handlers) is reimplemented in four places inside ziti (controller,
  router, tunnel, demo) plus the client dialer in the CLI. So the
  same pattern is copied four times in-repo while the thing it builds
  on sits in a separate repo. That separate repo is
  openziti-owned but, per the GitHub dependency graph, used only by
  ziti: every other importer is an archived project or a ziti fork.
  A library boundary that serves no external consumer just taxes us
  with a cross-repo release dance for every change.

The cleanest move is to **pull the agent library into ziti** (as
`common/agent`, mirroring the recent storage-library absorption) and
consolidate the four duplicated channel-plumbing copies there, then
land capability advertisement and the v2 log-level commands on top -
all in one in-repo branch. Doing the consolidation now, before any
new extension, means every subsequent change rides on one primitive
in one place rather than each carrying its own ad-hoc back-compat
code or its own copy of channel plumbing. The v2 log-level commands
are the first user, not the reason: they would otherwise be the
first of *N* future extensions that each invent their own protocol
shim.

## Goals

For this branch specifically:

- Let agent IPC advertise per-process capabilities to clients so
  callers can pick the right command set per target. Per-feature
  capability flags only - no transport-level meta capabilities.
- Pull the agent library into ziti as `common/agent`, and
  consolidate the channel-transport baseline (currently duplicated
  across four server copies plus the client dialer) into that
  one package, so there is a single in-repo home for channel-based
  agent commands and capability discovery. The agent library is
  openziti-owned and, per the dependency graph, used only by ziti, so
  the separate-repo boundary buys nothing and costs a release dance.
- Ship channel-based replacements for the log-level commands
  (`set-log-level`, `set-channel-log-level`, `clear-channel-log-level`)
  as the first capability-gated feature on the consolidated machinery,
  so the logging-refactor branch can ride on top with typed
  payloads instead of grafting new wire shapes onto the existing
  single-byte commands.
- Land it in a backwards-compatible way: old clients see no change;
  old servers continue to look exactly as they do today. Capability
  discovery is a net-new `AppInfoV2` command, so the legacy `AppInfo`
  wire shape is never touched.
- Reuse the `common/capabilities` mental model: stable string names
  per capability, with optional bit assignments when capabilities are
  carried as channel-header masks.

## High-level overview

```
  Client                                Server (agent listener)
  ──────                                ───────────────────────

  1. Open agent connection (existing UDS).
  2. Call AppInfoV2 (new command).     ──→  New server: respond with
                                            version + name
                                            + agent_capabilities []string
                                            + app_capabilities []string.
                                            Old server: unknown op, so
                                            it closes the conn with no
                                            write (clean EOF).
  3. On clean EOF, fall back to the
     legacy AppInfo command. Cache
     both lists per target until the
     connection drops.

  4. For commands that have both a
     legacy framed form and a v2
     channel form, pick by capability:

       if HasAgentCapability("logging.slog-levels"):
           send SetLogLevelV2 (channel)
       else:
           send legacy set-log-level (framed)
```

- **No change to legacy framed commands.** Old commands keep their
  current wire shape forever, including `AppInfo`. New behavior lives
  in net-new commands; we never re-encode an existing one.
- **Capability discovery is a net-new `AppInfoV2` command.** A new
  client calls `AppInfoV2` first. A new server answers with both
  capability lists. An old server has no handler for the op, so it
  closes the connection without writing anything; the client reads a
  clean zero-byte EOF, treats it as "V2 unsupported," and falls back
  to the legacy `AppInfo` command (empty capability set). This keeps
  the legacy `AppInfo` response byte-identical for old clients, which
  parse it as `map[string]string` and would otherwise choke on an
  array-valued field.
- **Channel transport is existing infrastructure.** The agent
  listener already accepts `channel/v4` connections; this branch
  does not introduce that mechanism. What is new is the `AppInfoV2`
  capabilities lists (agent and app) and the first feature-scoped
  capability that gates a channel-based command set.
- **No new round trip in the steady state.** Clients cache the
  AppInfoV2 result per agent connection. The agent process owns the
  connection lifetime; when the process dies the connection drops and
  the cache invalidates. No TTL.
- **Capabilities are feature-scoped, not transport-scoped.** Each
  capability has a stable hierarchical-dotted string name (e.g.,
  `"logging.slog-levels"`) tied to a concrete feature the caller
  can use. A "channel is available" meta-capability would convey
  no actionable information because callers care about specific
  commands, not the transport. May optionally have an integer bit
  position for use in channel headers; strings are the canonical
  identifier.
- **Out of scope this branch:** migrating other commands beyond the
  three log-level commands. A general "describe this command"
  mechanism. Per-command versioning beyond v2.
  All listed in [Deferred](#what-we-are-deliberately-deferring).

## AppInfoV2 command

The existing `AppInfo` command (op `0xa`) returns process identity as
a `map[string]string` (name, version, type, id, alias). We do **not**
extend it. Old clients parse that response into a `map[string]string`,
and adding an array-valued field would make their `json.Unmarshal`
fail outright, blanking the app info they rely on for `ziti agent
ps`/`list` filtering. Instead we add a net-new framed command,
`AppInfoV2` (a new op byte), that returns a richer struct:

```jsonc
{
  "name": "ziti-controller",
  "version": "v2.x.y",
  // ... existing identity fields ...
  "agent_capabilities": [
    "logging.slog-levels"
    // library-defined entries appended here
  ],
  "app_capabilities": [
    // application-defined entries appended here, e.g.
    // "ziti.router.something"
  ]
}
```

**Old-server fallback.** A new client calls `AppInfoV2` first. An old
server has no handler for the op: its framed dispatch falls through,
writes nothing, and closes the connection, so the client reads a
clean zero-byte EOF. The client treats that as "V2 unsupported" and
falls back to the legacy `AppInfo` command, treating the capability
set as empty. Zero-bytes-then-EOF is the unambiguous signal; a
partial read or a dial error is a genuine failure, not a fallback
trigger. Because every agent request uses a fresh connection, the
fallback is just "dial again, send `AppInfo`."

**Two fields, two namespaces.** Capabilities have two natural
sources: the agent package itself (the consolidated `common/agent`)
and the embedding application (what ziti, or any other in-tree
consumer, layers on top). Splitting them into two fields keeps each
namespace self-contained:

- `agent_capabilities` is populated by `common/agent` from a
  package-owned registry. Consumers don't write to this.
  Example: `logging.slog-levels`.
- `app_capabilities` is populated by the application via a
  registration API. `common/agent` does not interpret these strings;
  it just passes them through. Example: `ziti.something`.

This removes the collision risk a single shared list would have:
if both the package and the application picked the same string,
which one "owned" it would be ambiguous. With two fields, even
identical strings live in different scopes and can't conflict.

Capabilities are feature-scoped, not transport-scoped. The agent
already supports channel/v4 connections on its listener; "channel
is available" is the existing baseline rather than a discoverable
capability. Each capability advertises a specific feature the
caller can use - in this branch, `logging.slog-levels` signals
"the v2 log-level commands are present." Future extensions get
their own feature-level entries in whichever field is appropriate.

- Field is `[]string`. Absent → empty set. Old servers never emit
  it because they have no `AppInfoV2` handler at all.
- Strings use hierarchical-dotted naming. The prefix should
  identify the subsystem so the namespace stays organized as more
  capabilities accumulate (`logging.*`, `ipc.*`, `state.*`, ...).
- Strings never get renamed. If a capability needs to evolve, it
  becomes a new entry; the old entry may stay listed for a transition
  period.

## Capability registry

### Agent-package capabilities

The agent-owned capability registry lives in `common/agent` (the
consolidated home for capability machinery). `common/agent` is the
only writer; consumers only read.

```go
// in common/agent, e.g. common/agent/capabilities.go

const (
    // CapabilityLoggingSlogLevels indicates the agent supports the channel-
    // based v2 log-level commands (SetLogLevelV2,
    // SetChannelLogLevelV2, ClearChannelLogLevelV2) which carry
    // slog-style string level names.
    CapabilityLoggingSlogLevels int = 1
)

// agentCapabilityNames maps each agent bit to the canonical
// hierarchical-dotted string used in AppInfo.agent_capabilities.
// The two encodings (int bit position for channel hello bitmask,
// string for AppInfo JSON) stay in sync via this single table.
var agentCapabilityNames = map[int]string{
    CapabilityLoggingSlogLevels: "logging.slog-levels",
}

// GetAgentCapabilitiesMask returns the bitmask form, used in the
// channel hello.
func GetAgentCapabilitiesMask() *big.Int { ... }

// GetAgentCapabilityStringList returns the string-list form, used
// in AppInfo.agent_capabilities. Iteration order is deterministic
// (sorted by bit position) so the JSON shape is stable.
func GetAgentCapabilityStringList() []string { ... }

// AgentCapabilityBitFromString is the client-side inverse used
// when reading AppInfo agent_capabilities back into a bitmask
// check.
func AgentCapabilityBitFromString(s string) (bit int, ok bool) { ... }
```

The string list (carried in AppInfoV2.agent_capabilities) and the
bit positions (carried in the channel hello) are two encodings of
the same logical set. `common/agent` owns both views; clients pick
whichever the transport uses.

**Advertisement is conditional on handler installation.** An
agent-defined capability is emitted in `agent_capabilities` and
the channel hello bitmask **only when the corresponding server-
side handler is registered**. For `logging.slog-levels`, this
means the three v2 message handlers (`SetLogLevelV2`,
`SetChannelLogLevelV2`, `ClearChannelLogLevelV2`) must be wired up
before the capability appears in any AppInfoV2 response. A binary
that omits handler registration (a controller built without the
logging wiring, say) advertises no capability, so capability-
conditional clients correctly fall back to legacy framed commands.
This prevents a consumer from accidentally advertising a feature it
has not implemented.

### App-specific capabilities

`common/agent` exposes a small registration API. Applications that
embed the agent listener can register capability strings at startup;
`common/agent` aggregates them and emits them as
AppInfoV2.app_capabilities.

```go
// in common/agent, public API

// RegisterAppCapabilities adds capability strings to the
// app_capabilities list emitted in subsequent AppInfo responses.
// The library does not interpret these strings; it passes them
// through. Idempotent on duplicate adds.
func RegisterAppCapabilities(names ...string)
```

Applications keep their own constants in their own package. For
ziti specifically, ziti-defined capabilities live in
`common/agentcaps/` (or similar) and ziti calls
`agent.RegisterAppCapabilities(...)` once at process start, before
the agent listener begins accepting connections. `common/agent` does
no string validation beyond deduplication; the application owns
its namespace.

App capabilities are strings only — no bit-position encoding. The
reason agent caps have bit positions is that they ride in the
channel hello payload, which is a fast path. App caps are checked
through the AppInfoV2 string list, which is plenty for the rare
client paths that care. If a future app capability ever needs
hello-payload representation, that gets opted into separately.

## Client caching

The client side (the `ziti agent ...` CLI plus any in-process
callers) keeps a per-connection cache of the discovered capabilities.

```go
type agentClient struct {
    conn         net.Conn
    agentCaps    []string  // AppInfoV2.agent_capabilities, cached
    appCaps      []string  // AppInfoV2.app_capabilities, cached
    capsFetched  bool
}

func (c *agentClient) fetchCapsOnce() {
    if c.capsFetched { return }
    info, err := c.callAppInfoV2()  // falls back to legacy AppInfo on clean EOF
    if err != nil {
        c.capsFetched = true  // safe default: legacy behavior
        return
    }
    c.agentCaps = info.AgentCapabilities
    c.appCaps = info.AppCapabilities
    c.capsFetched = true
}

func (c *agentClient) HasAgentCapability(name string) bool {
    c.fetchCapsOnce()
    return slices.Contains(c.agentCaps, name)
}

func (c *agentClient) HasAppCapability(name string) bool {
    c.fetchCapsOnce()
    return slices.Contains(c.appCaps, name)
}
```

Two checking methods, one per field. Callers pick the namespace
their capability lives in - typically the agent-defined ones
(like `agent.CapabilityLoggingSlogLevels`) go through
`HasAgentCapability`; application-defined ones go through
`HasAppCapability`.

Connection lifetime equals process lifetime on both ends. No TTL,
no invalidation API. If the agent process restarts, the next call
fails to connect, the client reconnects (existing behavior), and
the next `HasCapability` call refetches.

## Channel transport (consolidate into common/agent)

Channel transport for agent IPC exists today, but in too many
places: the base agent machinery lives in the external
`github.com/openziti/agent` library, while the channel-upgrade
plumbing on top of it (server-side conn-to-channel binding, client-
side dialing) is copy-pasted into four spots inside ziti
(controller, router, tunnel, demo) plus the CLI dialer. The
external library is openziti-owned but, per the dependency graph,
used only by ziti.

This branch absorbs the agent library into ziti as `common/agent`
(the same move recently done for the storage library) and
consolidates the duplicated plumbing into that one package:

- The agent library's source (framed protocol, AppInfo, listener)
  moves to `common/agent`; the external dependency is dropped and
  the ~21 in-repo importers re-point to the in-tree path.
- The channel-binding mechanism the agent server uses, copied 4x,
  collapses into a single first-class server-side API in
  `common/agent` (`HandleChannelConnection`).
- The client-side channel-dialing helpers in the CLI move into
  `common/agent` as a first-class client-side API.
- The capability primitive (AppInfoV2 `[]string` fields, bitmask in
  channel hello) lands in `common/agent`.
- The new v2 typed log-level commands (`SetLogLevelV2` etc.) and
  their first capability (`logging.slog-levels`) land in
  `common/agent` as the first feature using the consolidated
  machinery.

ziti keeps using channel-based agent commands; they just go through
`common/agent` now instead of four hand-rolled copies. The legacy
framed protocol stays on its current path on both sides, and
capability advertisement rides the framed `AppInfoV2` command
because the v2 fetch is the first thing a client does.

In-repo implications (no cross-repo dance):

- This is a single ziti branch. There is no separate agent-library
  release to cut and no dependency bump to coordinate; absorbing the
  library and building on it happen in the same PR series.
- `common/agent` starts importing `channel/v4` + `identity` (the
  consolidated plumbing needs them). Both are already root-module
  requires and the root module's Go version already exceeds what
  `channel/v4` needs, so there is no `go.mod` entry and no Go-version
  bump. Neither `channel/v4` nor `identity` depends on the agent
  code, so there is no import cycle.
- Existing channel-based agent commands inside ziti switch to the
  consolidated `common/agent` APIs. Wire shape does not change for
  any existing command.

## First users: channel-based log-level commands

This branch adds three new channel-based commands, mirroring the
existing framed log-level commands but with typed message payloads
carrying string level names:

| Channel message | Payload | Replaces |
|---|---|---|
| `SetLogLevelV2` | `{ level: string }` | framed `set-log-level` (1-byte logrus.Level) |
| `SetChannelLogLevelV2` | `{ channel: string, level: string }` | framed `set-channel-log-level` |
| `ClearChannelLogLevelV2` | `{ channel: string }` | framed `clear-channel-log-level` |

**Content-type IDs.** These ride the same `"agent"` channel that
already carries application message types: ctrl_pb (the 1000s, e.g.
`InspectRequestType` = 1013) and mgmt_pb (the 10000s) are both
registered on it. To stay clear of both bands, `common/agent`
reserves a distinct high range for its own messages, **base
30000** (documented as "30000-30999 reserved for agent-package
messages"). The three v2 messages take the first IDs in that band.

**Level on the wire and at the callback.** `level` is a lowercase
string from the set `"debug"`, `"info"`, `"warn"`, `"error"`,
`"trace"`, `"fatal"`, `"panic"`. `common/agent` defines an
agent-local `LogLevel` enum with those seven values plus
`String()`/`ParseLogLevel(string)`; it parses the wire string once
and hands the registered callback an `agent.LogLevel`. It does
**not** know about slog or logrus. The embedding application's
callback maps `agent.LogLevel` onto its loggers: ziti maps it to
both `slog.Level` (including the custom trace/fatal/panic offsets
defined in the logging-refactor branch) and `logrus.Level`. This
keeps the custom slog offsets out of `common/agent` and avoids two
copies of the wire-name <-> level mapping drifting apart across the
package boundary.

Client selection logic:

```go
if c.HasAgentCapability("logging.slog-levels") {
    ch := c.openChannel()       // uses existing agent channel support
    ch.SendSetLogLevelV2(levelName)
} else {
    c.sendFramedSetLogLevel(logrusLevelByte)
}
```

(`HasAgentCapability` takes either a name string or a bit; tests
usually use the constant `agent.CapabilityLoggingSlogLevels` form.)

Legacy framed `set-log-level` / `set-channel-log-level` /
`clear-channel-log-level` stay on the server forever. Old clients
keep using them unchanged. New clients prefer the channel commands
when the capability is present.

The level handlers are app-supplied callbacks registered through
`common/agent` (see the logging-refactor progress doc for the
`RegisterLogLevelHandlers` shape). Both the v2 channel handlers and
the legacy framed handlers route through the same callbacks, so the
seam the logging-refactor branch needs already exists: in this
branch ziti's callback drives `logrus.SetLevel(...)` only; in the
logging-refactor branch the same callback also drives
`globalLevel.Set(...)`, with no change to `common/agent`.

## What we are deliberately deferring

- **Migrating other agent commands to channel transport.** This
  branch ships the three log-level v2 commands as the first capability-
  gated channel commands. Other framed commands (stack dump,
  profile, set GC percent, ...) stay on the legacy framed path
  until each one's owner decides to migrate it.
- **Per-command capability descriptors.** This branch lists features
  the binary supports as opaque strings. We are not adding a way to
  introspect *what* a capability provides at runtime (no "schema for
  set-channel-log-level-v2" endpoint). Strings are documented in
  the registry source; that is sufficient until a use case shows up.
- **Capability deprecation flow.** When we eventually need to remove
  a capability, we will define the policy then. For now, capabilities
  are append-only.
- **Negotiation beyond intersection.** A client checks whether the
  server has a capability; the server does not check the client's
  set. Two-sided negotiation would need a richer handshake; not
  needed for the first uses.

## Acceptance criteria

A reasonable bar for "this branch is done":

1. **Agent library absorbed into `common/agent`**: the external
   `github.com/openziti/agent` dependency is dropped, its source
   moves in-tree, and the ~21 importers re-point to the in-tree
   path. The package exposes a server-side channel-binding API (used
   by the agent listener) and a client-side channel-dialing helper.
   The 4 duplicated channel-plumbing copies (controller, router,
   tunnel, demo) collapse into that one API; no copy remains.
   Wire shape for any existing channel command is unchanged.
2. `common/agent` gains a capability registry: bit constant
   `CapabilityLoggingSlogLevels`, the `agentCapabilityNames` table,
   and the `GetAgentCapabilitiesMask`,
   `GetAgentCapabilityStringList`, `AgentCapabilityBitFromString`
   helpers. `common/agent` is the only writer of this registry.
3. `common/agent` exposes a `RegisterAppCapabilities(names ...string)`
   API for applications to register their own capabilities. It
   aggregates registrations and emits them in
   AppInfoV2.app_capabilities.
4. A net-new `AppInfoV2` command returns `agent_capabilities` (from
   the registry) and `app_capabilities` (from application
   registration). The legacy `AppInfo` command is untouched, so
   existing `AppInfo` callers (which parse `map[string]string`) see
   no change. A new client that hits an old server reads a clean
   zero-byte EOF on the unknown `AppInfoV2` op and falls back to
   `AppInfo` with an empty capability set.
5. Agent client has `HasAgentCapability(name string) bool` and
   `HasAppCapability(name string) bool` methods. The first call
   on a connection lazy-fetches AppInfoV2 (with legacy fallback) and
   caches both lists for the connection lifetime.
6. Three channel messages are defined and wired up in `common/agent`:
   `SetLogLevelV2`, `SetChannelLogLevelV2`, `ClearChannelLogLevelV2`,
   using content-type IDs in the agent-reserved 30000+ band.
   `common/agent` owns the message types and the channel dispatch;
   it parses the wire level string into an agent-local `LogLevel`
   enum and accepts a small callback registration so applications
   supply the actual side effects. Both v2 and legacy framed level
   handlers route through the same callbacks. Initial ziti
   registration drives logrus only (matching today's framed
   handlers); the logging-refactor branch extends those same
   callbacks to drive logrus plus slog.
7. **In-repo migration**: the existing ziti client and server code
   paths that talk channel for agent commands switch over to the
   consolidated `common/agent` APIs. No call site keeps its own copy
   of the plumbing. Any ziti-defined capabilities go through
   `agent.RegisterAppCapabilities(...)` at startup.
8. Tests cover, at minimum:
   - A legacy server (no `AppInfoV2` handler) makes a new client
     fall back cleanly to `AppInfo` and treat capabilities as empty;
     old clients are entirely unaffected.
   - `AppInfoV2` with `agent_capabilities: ["logging.slog-levels"]`
     lets a new client send `SetLogLevelV2` over channel and
     receive ack.
   - `AppInfoV2` without `logging.slog-levels` causes a new client to
     fall back to the legacy framed `set-log-level` command.
   - `app_capabilities` round-trips: an application registers a
     capability via `RegisterAppCapabilities`; a client reads it
     back via `HasAppCapability` and the result matches.
   - The same capability name registered as both an agent_capability
     and an app_capability is reported on its own field with no
     merging (proves namespace separation).
   - Each v2 channel command round-trips end-to-end.
   - Channel-hello capabilities (the `*big.Int` from
     `GetAgentCapabilitiesMask`) decode back to the same string set
     as `GetAgentCapabilityStringList`. App capabilities are *not*
     in the bitmask.
9. A short developer note explains the two-tier model: how to add
   an agent capability (pick a name, add the constant and bit
   position in `common/agent`) versus how to register an
   application capability (call `RegisterAppCapabilities` at
   startup; the name lives in your own package).
10. The full unit-test suite for the touched packages passes.

This branch absorbs the agent library, lands the AppInfoV2
capability primitive, and wires the three v2 log-level commands
through to today's logrus handlers. The logging-refactor branch
extends the same callbacks to drive `globalLevel.Set(...)` (slog)
in addition.

## Open questions for the reviewer

- The capability fields are `[]string` (settled). The alternative,
  `map[string]string` for per-capability version metadata (e.g.,
  `{"logging.slog-levels": "v1"}`), adds complexity now for a
  possible future need; the slice shape forces version bumps to be
  new entries (`"logging.slog-levels-v2"`), which is simpler and
  matches the `common/capabilities` bitmask pattern. Since AppInfoV2
  is a net-new struct, it can carry richer shapes later without any
  old-client concern, so we are not boxed in.
- `HasCapability` failure-mode: a clean zero-byte EOF on `AppInfoV2`
  means "old server," and the client silently falls back to the
  legacy `AppInfo` command (empty capability set). The open question
  is the *other* failure modes: should a non-EOF error (partial
  read, malformed JSON, dial failure) be hard, or also fall through
  to legacy? Silent fall-through is what the sketch does; a hard
  error would surface server-side issues but break clients that hit
  a transient failure.
