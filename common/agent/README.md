# common/agent

This package is the in-tree home of ziti's IPC agent: the `gops`-style framed
protocol, the channel-transport upgrade, and the capability-advertisement
primitive. It was absorbed from `github.com/openziti/agent`; see
[doc/design/agent-capabilities.md](../../doc/design/agent-capabilities.md) for
the full design and rationale. This note is the practical "how do I extend it"
guide.

## Capabilities: two tiers

A process advertises what it supports through two capability lists, returned by
the `AppInfoV2` command and (for agent capabilities) carried as a bitmask in the
channel hello:

- **Agent capabilities** are owned by this package. They describe features the
  agent machinery itself provides, like the v2 log-level commands. They have a
  stable bit position (for the hello bitmask) and a stable hierarchical-dotted
  string name (for `AppInfoV2`). Today there is one: `logging.slog-levels`.
- **App capabilities** are owned by the embedding application. The package does
  not interpret them, it just passes the strings through. ziti can register
  strings of its own (e.g. `ziti.something`) at startup; none are registered
  yet.

The two live in separate fields, so even an identical string in both namespaces
cannot collide.

A capability is only advertised when its handler is actually wired up. This
keeps a binary from claiming a feature it did not install: a controller built
without the logging wiring simply does not advertise `logging.slog-levels`, and
capability-aware clients fall back to the legacy commands.

## Adding an agent capability

Agent capabilities are defined here, in `capabilities.go`:

1. Add a bit constant. Bits are append-only and never renamed; if a capability
   needs to change shape, add a new one.

   ```go
   const (
       CapabilityLoggingSlogLevels int = 1
       CapabilitySomethingNew      int = 2 // new
   )
   ```

2. Add its canonical name to the `agentCapabilityNames` table. The string is the
   identifier clients match on; the prefix should name the subsystem
   (`logging.*`, `ipc.*`, ...).

   ```go
   var agentCapabilityNames = map[int]string{
       CapabilityLoggingSlogLevels: "logging.slog-levels",
       CapabilitySomethingNew:      "something.new",
   }
   ```

3. Mark it active only when its handler is registered, by calling
   `markAgentCapabilityActive(bit)` from the registration entry point (the way
   `RegisterLogLevelHandlers` does for `CapabilityLoggingSlogLevels`). That is what
   makes advertisement conditional. Registration must happen before
   `Listen`; the capability set freezes once the listener starts.

Clients check an agent capability by bit:

```go
if opts.HasAgentCapability(agent.CapabilityLoggingSlogLevels) { ... }
```

## Registering an app capability

App capabilities need no changes here. The application keeps its own constants
in its own package and registers the strings at startup, before `agent.Listen`:

```go
agent.RegisterAppCapabilities("ziti.something")
```

App capabilities are strings only, with no bit position. Registering a new
name after the listener has started panics, because the advertised set must
stay consistent across every connection; re-registering a name that is already
known is a no-op. Clients check an app capability by name:

```go
if opts.HasAppCapability("ziti.something") { ... }
```

## Adding a v2 channel command

The v2 log-level commands (`loglevel_commands.go`) are the worked example. To
add another channel command:

- Reserve a content-type ID in the agent band. Application protocols already use
  the 1000s (ctrl_pb) and 10000s (mgmt_pb) on the agent channel, so this package
  reserves **30000-30999** for its own messages. Carry parameters as string
  channel headers in the same band (see `LogLevelHeader` / `LogChannelHeader`).
- Bind the server handler so it rides every agent channel. The log-level handlers
  are bound automatically by `HandleChannelConnection` when callbacks are
  registered; follow that pattern rather than making each application bind it.
- Gate the new command behind a capability so clients can detect it, and tie that
  capability's `markAgentCapabilityActive` call to the same registration that
  installs the handler.

## Wire compatibility

Never re-encode an existing command. New behavior goes in net-new commands. The
legacy framed commands (including `AppInfo`, which old clients parse as
`map[string]string`) keep their wire shape forever, so a new server stays
readable by old clients. Capability discovery is the net-new `AppInfoV2`
command: an old server has no handler for it, closes the connection with no
write, and the client reads a clean EOF and falls back to `AppInfo` with an
empty capability set.
