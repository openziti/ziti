# P2P Direct Connections for OpenZiti

## Status

Early design. The prerequisites are in place; this doc plans the work
on top.

## Goal

Enable OpenZiti SDK endpoints to establish direct peer-to-peer UDP
connections when possible, bypassing the router data plane for lower
latency and reduced infrastructure cost, while retaining the full
OpenZiti circuit as a fallback.

## Foundation relevant to P2P

- **Xgress runs in the SDK.** Sequencing, ACK, retransmit, and flow
  control are end-to-end SDK→SDK; the router is a relay in the data
  path, not a participant. Without this a direct transport would need
  to reimplement all of the reliability layer.
- **`DataPlaneAdapter` is a clean seam.**
  `sdk-golang/xgress/xgress.go:84-100` defines the full contract
  (`ForwardPayload`, `ForwardAcknowledgement`, `ForwardControlMessage`,
  `RetransmitPayload`). A second implementation pointed at a DTLS
  transport satisfies the same contract — no xgress changes needed.
- **Payload wire format is transport-agnostic.** Circuit ID and
  sequence are baked into `xgress.Payload`. The same bytes work over
  any transport; a DTLS pipe doesn't need edge-channel framing.
- **Circuit-id-keyed mux dispatch exists, but unused.**
  `ConnMuxImpl.circuitSinks` + `AddByCircuitId` / `RemoveByCircuitId`
  are defined, and `HandleReceive` checks `CircuitIdHeader` first.
  Nothing currently calls `AddByCircuitId`. Not required for this
  feature — a DTLS path can feed payloads directly into its bound
  xgress without going through the mux — but worth wiring up as a
  general cleanup later.

## Architecture

```
  SDK-A                                              SDK-B
  ┌──────────┐                                  ┌──────────┐
  │ App      │                                  │ App      │
  │  ↕       │                                  │  ↕       │
  │ edgeConn │                                  │ edgeConn │
  │  ↕       │                                  │  ↕       │
  │ Xgress   │──── P2P Direct (DTLS/UDP) ──────│ Xgress   │
  │  ↕       │                                  │  ↕       │
  │ Router   │──── OpenZiti Circuit ────────────│ Router   │
  │ Channel  │    (fallback / signaling)        │ Channel  │
  └──────────┘                                  └──────────┘
        │                                            │
        └──── control plane (routers/ctrl) ──────────┘
```

The xgress layer on each SDK dispatches to one or both data paths via
a multi-path `DataPlaneAdapter`. The direct DTLS path is preferred
when available; the circuit path is always maintained as fallback and
as the signaling channel for the P2P setup itself.

## Design decisions

### Settled

- **Circuit-first, then upgrade.** Application gets a working
  connection immediately. P2P is a transparent optimization that can
  fail silently; no new failure modes on the critical dial path.
- **Multi-path via the existing `DataPlaneAdapter` seam.** The xgress
  layer keeps handing payloads to one adapter; the adapter hides the
  path-selection detail. ACK reconciliation stays sequence-number-
  based and path-independent.
- **Socket reuse across STUN → hole punch → DTLS.** Single
  `net.UDPConn` owned for the lifetime of the P2P attempt; preserves
  the NAT mapping from STUN through the DTLS session.
- **Standard STUN (RFC 5389)** via `pion/stun`.
- **Dual-path mode v1: direct-preferred, circuit hot fallback.**
  Primary data flows over direct DTLS; circuit absorbs retransmits
  for fast recovery from direct-path loss.
- **Minimum ctrl/router version: 1.0.0** (SDK already enforces for
  connect-v2).
- **Initiator = DTLS client.**
- **mTLS via cert exchange over the circuit.** The two SDKs swap
  identity cert fingerprints through the signaling channel, then pin
  them for the DTLS handshake. Removes any trust-anchor-discovery
  dependency at DTLS time and works even when the peers weren't
  issued by the same controller CA.

### Tentative

- **Signaling rides xgress `Payload`, not `Control`.** Control
  messages can be dropped and the xgress layer doesn't retransmit
  them, which is unacceptable for setup messaging. Discriminate
  signaling from app data via a header/flag in the payload. Fallback
  plan if we change our minds: make xgress control messages
  retransmittable and use them instead; bigger lift.
- **Capability gate is SDK-config-only for MVP.** Controller-level
  per-service P2P policy is deferred — needs controller + CLI +
  model work that's out of scope for the initial cut.
- **Symmetric-NAT early-bail is deferred.** A symmetric NAT allocates
  a different external port per destination, so the port learned
  from a STUN probe won't be the port the peer sees — hole punching
  fails after a multi-second timeout. Detection is cheap: query two
  different STUN-capable routers from the same local socket; if the
  mapped ports differ, bail before even exchanging addresses.
  Optimization, not correctness, so post-MVP.
- **Path performance measurement: passive, path-tagged.**
  `LinkSendBuffer` already tracks in-flight sequence numbers; extend
  each in-flight entry with the path the payload was sent on. When an
  ACK arrives (regardless of which path it returns over), attribute
  RTT to that path's moving average. Retransmits count as loss
  against the originating path, not the path we retransmit over. A
  combined per-path health score with hysteresis drives path
  selection.

  No active probing in the MVP beyond the NAT keepalive we need on
  the direct path anyway — piggyback health on that. No bandwidth
  measurement in the MVP; infer implicitly from window evolution and
  revisit post-MVP if it matters.

  Open details: whether the receiver sends per-path ACKs or
  aggregates across paths (per-path is simpler for attribution but
  doubles ACK traffic during dual-path operation); and whether a
  circuit reroute counts as "the same path" for attribution purposes
  (MVP: yes, it's still just "the circuit path").

## Implementation plan

Five milestones. Each is shippable on its own; M1 is the refactor
that unlocks the feature, M2–M4 light it up, M5 is polish.
Everything before M4 is invisible to applications.

### M1 — New multi-path adapter alongside `XgAdapter`

**Why first.** P2P needs outbound payloads to dispatch across two
transports; path-tagged in-flight tracking is needed for per-path
metrics. Rather than retrofit `XgAdapter`, introduce a separate
`MultiPathAdapter` that implements `DataPlaneAdapter` and composes
smaller `Path` units. `XgAdapter` stays untouched.

**Changes.**

- New `Path` interface in the xgress package: four outbound methods
  (payload / ack / control / retransmit), no `Env` embedding.
- New `MultiPathAdapter` in `sdk-golang/ziti/edge/network/`
  implementing `DataPlaneAdapter`, composing a slice of `Path` + a
  `PathSelector`. v1 selector picks the primary path.
- `RouterChannelPath` — trivial wrapper around the existing
  `XgAdapter` sender (or extracted from it). Used as the primary
  `Path` for V2 connections.
- V2 connect paths (`ConnectV2` and the terminator side) construct
  a `MultiPathAdapter` with one `RouterChannelPath` and pass that
  to xgress instead of an `XgAdapter`.
- `MultiPathAdapter` tags each in-flight sequence with its send
  path. ACK attribution and per-path retransmit counting land here.

**Retransmit handling.** When `LinkSendBuffer` decides to retransmit,
the loss counts against the payload's original path (from the tag).
Selector picks where to retransmit, using the circuit path as the
fallback-ahead-of-timeout lane once both paths are live.

**Test.** Existing V2 round-trip tests pass unchanged. Unit tests
for `PathSelector` behavior with a mocked `Path`, and for per-path
attribution with two synthetic paths.

**Risk.** Low. `XgAdapter` is untouched, so the V1 path and any
regression-sensitive V2 path carries the same adapter as before
until we actively switch it over.

### M2 — Signaling primitive + capability on CircuitStart

**Why before STUN/DTLS.** The two SDKs need a reliable way to
exchange P2P setup messages over the existing circuit: "I'm
P2P-capable", "here's my mapped address", "punch succeeded", etc.
Build that rail first; light it up with real content later.

**Changes.**

- **Capability advertisement on `CircuitStart`.** Initiator sets a
  `P2PCapable` header (and supporting state — protocol version,
  cert fingerprint) on the `CircuitStart` payload emitted from its
  own `xgress.Start()`. End-to-end over the circuit; no router
  changes needed (routers forward xgress payloads opaquely).
- **Terminator acknowledgement.** On receipt of a `CircuitStart`
  with `P2PCapable`, if the terminator SDK also supports P2P, it
  emits an empty-body payload on its xgress stream with its own
  `P2PCapable` + state headers. Regular payload = reliable, ACKed,
  retransmitted.
- **Signaling discriminator.** A dedicated payload header (e.g.
  `P2PSignalingType`) marks a payload as carrying signaling content.
  Body length is whatever the message needs (often zero). Used for
  all subsequent signaling — `P2PCapable`, and the messages M3–M4
  will introduce.
- **Interception point.** In `edgeConnXgress`'s read path, peek at
  each payload for the `P2PSignalingType` header. If present,
  dispatch to the P2P coordinator instead of the app read buffer.
  Signaling runs through the same ACK / retransmit loop as data —
  reliable by construction.
- **P2P coordinator.** One per `edgeConnXgress`. Owns the upgrade
  state machine. For M2 its only states are `awaiting-capability`,
  `capability-confirmed`, `dead` (peer doesn't support P2P or
  timeout elapsed).
- **Wire format.** Headers are enough for capability + fingerprint.
  If later messages outgrow headers, we can escalate to protobuf in
  the payload body.

**Open note (revisit during impl).** The specific shape of the
signaling-dispatch hook — a dedicated coordinator vs. a generic
xgress-extension mechanism modeled on channel's `ReceiveHandler` —
is deferred. Dedicated coordinator is simpler for the MVP; a
generic hook could be cleaner if we see other candidates for xgress
payload interception. Worth a look during implementation but not a
gate on M2.

**Non-goals for M2.** No STUN, no mapped-address exchange, no
DTLS. After M2 both SDKs have confirmed each other's P2P capability
and have the peer's cert fingerprint in hand; the upgrade itself is
stubbed out.

**Test.** Integration test with in-memory controller + router + two
SDKs. Dial a service end-to-end; verify both sides reach
`capability-confirmed`; verify fingerprints are exchanged; verify
no signaling bytes leak into the app read buffer.

**Risk.** Low — no new network path, no transport work. Main
subtlety is correctly intercepting the signaling header in the
read path without disrupting existing app-data flow.

### M3 — Address discovery

**Why now.** Before two SDKs can hole-punch, each needs to learn
its own public mapped address. Router (or controller, eventually)
acts as a STUN reflector; SDK exchanges discovered addresses with
its peer over the M2 signaling channel.

**Changes.**

- **Standalone STUN component.** New top-level `./stun/` package in
  the ziti repo — self-contained, embeddable. Exposes a `Server`
  type with config + lifecycle methods. Keeps the router and
  controller integration paths symmetric; when we later decide to
  host STUN from the controller, it's an import + wire-up, not a
  rewrite.
- **Router embeds the STUN server.** UDP binding speaking RFC 5389
  via `pion/stun`. Default port 3479 (`ziti-stun`), overridable in
  router config. **Disabled by default** — operators opt in. No
  auth; same posture as standard STUN.
- **STUN capability advertisement.** Router sets a `STUNCapable`
  bit in its hello headers *only when STUN is enabled*. SDK's
  `routerConn` exposes `SupportsSTUN() bool` mirroring
  `SupportsConnectV2()`.
- **ER list includes capabilities.** Extend
  `rest_model.CommonEdgeRouterProperties` (or a parallel field on
  the client-side `ServiceEdgeRouters` response) with a
  capabilities set — the same set the router advertises in its
  hello. Controller populates from the broker's per-router state.
  SDK can then filter and rank ERs by capability without
  speculatively connecting. Becomes the canonical way to expose
  any future router capability.
- **SDK STUN client**
  (`sdk-golang/ziti/edge/network/stun_client.go`). Owns the local
  `net.UDPConn` for the P2P attempt — same socket is later handed
  to hole-punch and DTLS so the NAT mapping is preserved. Single
  `DiscoverMappedAddress(ctx, routerAddr)` entry point.
- **Router selection.** Prefer the SDK's current router if it's
  STUN-capable. Otherwise pick the lowest-latency STUN-capable ER
  from the already-cached service ER list.
- **Coordinator state machine.** After M2's capability
  confirmation: run STUN → get mapped address → send
  `P2PMappedAddress` over the signaling rail → receive peer's
  address → move to `ready-to-punch`. Upgrade remains stubbed.

**Rollout note.** Because STUN is off by default, early P2P tests
need at least one router with it explicitly enabled. Worth
documenting in the feature-enablement guide when M3 ships.

**Dependencies.** Add (or promote) `pion/stun` as a direct SDK dep.
Likely transitively present via the `pion/dtls` lineage — worth
confirming.

**Non-goals.** No hole-punching, no DTLS, no direct traffic. End
state is diagnostic: both SDKs know each other's mapped addresses.

**Test.**

- `./stun` unit tests: binding request against the embedded server,
  verify reflected address matches source.
- Router integration: same thing through a running router with STUN
  enabled.
- End-to-end: two SDKs on distinct public addresses dial a
  P2P-capable service, both reach `ready-to-punch`, both see the
  peer's address.

**Risk.** Medium. Touches config, router hello, controller ER-list
response, SDK ER filtering, and new SDK STUN client. Each piece is
small individually; the new `./stun` package is a greenfield.

### M4 — Hole punch + DTLS + direct path live

**Why this is the payoff.** Everything before is plumbing. M4 is
where the direct path actually carries traffic.

**Changes.**

- **Socket-per-circuit.** Each P2P coordinator owns one
  `net.UDPConn` for the lifetime of its circuit. Phases are strictly
  sequential (STUN → punch → DTLS), so at any moment only one
  packet type is expected on the socket; anything else is dropped.
  No runtime demux. SDK-wide shared socket + mux is a post-MVP
  efficiency item if concurrent P2P circuit counts become a
  resource concern.
- **Hole punch via STUN probes.** After M3 exchanges mapped
  addresses, each side sends STUN binding requests to the peer's
  mapped address on the shared socket. Bounded by a short timeout
  (low seconds). Success = bidirectional receipt confirmed. Reuses
  `pion/stun` already in the tree; STUN's magic cookie means the
  probes are unambiguously ours.
- **DTLS handshake.** Initiator = client, terminator = server.
  Library: start with `openziti/transport/v2/dtls` — it already
  provides cert/interface-binding utilities that other DTLS paths
  may want to share. If the shape doesn't fit cert-pinning needs
  cleanly, fall back to `pion/dtls` directly during impl. The
  transport package is a reasonable home to land any new P2P-
  related DTLS utilities that could benefit other consumers.
- **mTLS cert pinning.** DTLS handshake presents each side's SDK
  identity cert. Verification callback compares the peer's
  presented-cert fingerprint against the fingerprint received via
  M2 signaling. Mismatch → fail closed. No reliance on chain
  validation.
- **`DirectPath` implementation.** New
  `sdk-golang/ziti/edge/network/direct_path.go` implementing the
  `Path` interface from M1. Channel framing rides on top of the DTLS
  session: each `Path.Send*` method produces a `*channel.Message`
  (via `payload.Marshall()` etc., the same marshalling the router
  channel uses) and writes it through a `channel.Channel` wrapping
  the DTLS net.Conn. Channel framing carries the `ContentType`
  discriminator (Payload / Acknowledgement / Control) and any
  headers, so:
  - The send side reuses the existing xgress marshalling unchanged.
  - The receive side is a normal `channel.Channel` dispatch — same
    parser as the router connection — delivering payloads / acks /
    controls directly to the bound `edgeConnXgress` (one
    `DirectPath` serves exactly one xgress).
  - `Retransmit` is wired but the `PathSelector` steers retransmits
    over the circuit path per the dual-path mode decision.
- **Path selector flip.** On successful DTLS handshake,
  `MultiPathAdapter.AddPath(directPath)`. Selector starts picking
  direct for new payloads; circuit stays up as retransmit-lane and
  signaling channel.
- **Keepalive.** DTLS heartbeat (RFC 6520) if `pion/dtls`
  implements it — verify during impl. Fall back to a low-rate empty
  signaling payload over the direct path otherwise. Interval ~15s
  — low enough to hold common NAT mappings (typical UDP timeout
  30–60s).
- **Failure modes (all fall back to circuit silently):**
  - Punch timeout → abandon, stay on circuit.
  - DTLS handshake timeout / failure → abandon.
  - Cert fingerprint mismatch → abandon, log loudly.
  - Post-upgrade: consecutive-loss threshold on direct path →
    `RemovePath(directPath)`, all traffic shifts to circuit. No
    re-upgrade attempt in the MVP.

**Non-goals for M4.** Re-upgrade after fallback, symmetric-NAT
detection, active-active, shared UDP socket, per-path metrics
beyond what the loss-threshold trigger needs.

**Test.**

- LAN: two SDKs on the same machine over loopback — skip punch
  nuances, verify DTLS handshake + direct-path traffic + ACK
  reconciliation across paths.
- Real NATs: two SDKs behind non-symmetric NATs with a STUN-enabled
  router. Full punch, handshake, traffic.
- Cert-pin failure: swap one side's expected fingerprint, verify
  handshake fails and the circuit path keeps serving traffic.
- Direct-path loss: simulate loss on the DTLS socket, verify
  threshold trips, verify traffic continues on circuit.

**Risk.** High. Biggest milestone. Hole punching is inherently
fussy; DTLS + cert pinning + keepalive have a few moving parts
that need to line up. Budget extra time for NAT-interaction
debugging on real networks.

### M5 — Polish

**Why last.** M4 delivers a working P2P upgrade. M5 takes it from
"works in a lab" to "safe to ship."

**Changes.**

- **Config knobs.** Expose under SDK options:
  - `P2PEnabled bool` — top-level on/off. Default TBD; leaning
    on-when-available for the initial release so the feature
    actually gets exercised.
  - `P2PSTUNTimeout`, `P2PPunchTimeout`, `P2PDTLSHandshakeTimeout`
    — tunable per-phase deadlines with sensible defaults.
  - `P2PKeepaliveInterval` — overrides the ~15s default.
  - `P2PFallbackLossThreshold` — consecutive-loss count that trips
    the direct → circuit fallback.
- **Observability.**
  - Per-path metrics on the `MultiPathAdapter`: RTT moving average,
    retransmit count, bytes sent/received, active-path indicator.
    Emitted via the existing SDK metrics registry.
  - State-transition events on the P2P coordinator:
    `p2p.capability_confirmed`, `p2p.stun_succeeded`,
    `p2p.punch_succeeded`, `p2p.upgrade_complete`,
    `p2p.fallback_triggered`, `p2p.abandoned`. Routed through the
    existing `EventEmmiter`.
  - Structured log lines at each phase transition — enough for an
    operator tailing logs to see "dial X → P2P attempted → punched
    → DTLS up → upgrade complete" without needing instrumentation.
- **Inspect integration.** Extend the SDK's `Inspect()` result to
  include per-circuit P2P state (capability, phase, active path,
  per-path health). Useful for the CLI `ziti fabric inspect` flow.
- **Load and compatibility testing.**
  - Many-concurrent-circuits soak test: verify socket-per-circuit
    doesn't exhaust fds or NAT mappings in realistic counts.
  - Mixed-version: run with one-side-upgraded, verify graceful
    degradation (peer doesn't advertise P2P → no upgrade attempt,
    circuit continues).
  - Router without STUN in the path: verify SDK gracefully skips
    P2P when no STUN-capable ER is reachable.
- **Docs.**
  - Operator guide: how to enable STUN on a router, what ports need
    to be open, what NAT behaviors are supported and which aren't
    (symmetric bail note).
  - SDK user guide: the `P2P*` options, what they do, expected
    behavior and fallback semantics.
  - One minimal end-to-end example / sample app exercising the
    upgrade path and showing active-path observability.

**Non-goals.** Anything in the "Future work" section below.

**Test.** Covered by load tests, compat matrix, and docs-example
running in CI.

**Risk.** Low. All additive; doesn't change M4 behavior.

## Future work (explicitly deferred from the MVP)

- **Symmetric-NAT early-bail.** Query two STUN-capable routers from
  the same socket; if mapped ports differ, abandon before
  signaling. Cheap optimization; saves the punch timeout in
  corporate/carrier-NAT deployments.
- **Re-upgrade after fallback.** Today's MVP doesn't retry a
  direct-path upgrade after it falls back to circuit. Re-upgrade
  would need a cooldown + retry policy + observability to avoid
  oscillation.
- **Active-active dual-path.** Both paths carry data, receiver
  dedups by sequence. Maximum reliability at double bandwidth cost.
  Useful for critical connections; not worth the complexity for
  MVP.
- **Multi-candidate / ICE-lite gathering.** Currently one candidate
  (server-reflexive from one router). Host candidates, multi-router
  candidates, relayed candidates would improve punch success on
  awkward NATs.
- **SDK-wide shared UDP socket.** Multiplex all P2P circuits on one
  socket if the socket-per-circuit count becomes a resource issue.
  Requires STUN transaction / DTLS session demux we deferred.
- **Controller-side STUN.** The `./stun` package is structured to
  make this an easy port. Useful when a deployment has
  controller-visible edges that are better STUN vantage points
  than the routers.
- **Controller-level per-service P2P policy.** Gate P2P at the
  service level via policy; today it's SDK-config-only.
- **Bandwidth measurement.** Active probing or duplicate-and-race
  between paths. MVP infers from window evolution.
- **Mobile handoff.** Automatic re-punch after a network change
  (WiFi → cellular, IP change). The fallback circuit provides
  continuity while the new P2P is established.
- **SDK-level circuit rerouting.** Unrelated to P2P but directly
  enabled by the multi-path adapter work in M1. A circuit reroute
  becomes "swap the circuit path underneath the adapter" without
  touching the xgress or app.
- **Wire up `circuit_sinks` mux dispatch.** `AddByCircuitId` is
  defined but unused today; wiring it up is a general cleanup
  (decouples sink lookup from connId allocation) but not required
  for this feature.
