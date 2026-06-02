# SDK Rerouting

## Status

Early design. Captures the approach for enabling SDKs to survive loss of
their current ingress router by re-attaching circuits via a different
router, plus the evolutionary path toward fuller multi-path and
path/circuit separation.

## Summary

The work is phased A -> (B1 | B2 | B3 | B4) -> C, ordered fastest-to-slowest.
Phase A is the foundation; B1-B4 each build on it independently; C is the
strategic destination that subsumes the rest.

- **Phase A — Reroute-token (express lane):** the SDK survives ingress router
  loss by re-attaching the circuit to a different router with a controller-signed
  reroute token, while the controller holds the circuit in a grace-period `Limbo`
  state and re-splices the path without involving the dead router.
- **Phase B1 — Terminator-side reroute:** the symmetric case, where the
  terminator SDK reattaches the circuit's egress to a new router using the same
  token primitive (`side: Egress`); the new piece is signaling which terminators
  want their circuits reroutable.
- **Phase B2 — Pre-emptive multi-path:** the SDK proactively opens a second
  router channel and runs a parallel disjoint path, so traffic shifts on failure
  with no detection lag and the selector can hot-standby or load-distribute.
- **Phase B3 — Cross-controller takeover:** lifts Phase A's owner-reachable
  constraint with an explicit ownership-transfer protocol, so any controller in
  the cluster can validate a token and execute the splice, surviving a combined
  router + owning-controller failure.
- **Phase B4 — Router/controller connectivity resilience:** adds a
  router-presence grace (debounce) in front of Phase A's E2, so a transient
  controller<->router channel flap never cascades into reroute or teardown of
  circuits whose data plane was fine the whole time.
- **Phase C — Long-lived paths separated from circuits:** the strategic end
  state, where paths become reference-counted long-lived resources and circuits
  become lightweight records, making SDK reroute a path swap and enabling
  sub-millisecond local circuit creation.

## Goal

Today, when an SDK loses its connection to its current ingress router,
the circuit dies. The terminator's xgress sees forwarding faults; the
controller eventually unroutes; the SDK reports a connection error to
the application. Survivable failure modes — mobile network handoff,
router maintenance, transient network blips, SDK process migration —
all surface as application-visible disconnects.

The goal is to make those failures *recoverable* from the SDK side:
when the current router connection dies, the SDK can pick up the circuit
via a different router and continue without the application noticing,
preserving in-flight data via the existing end-to-end retransmit
machinery.

## Foundation

Two pieces are already in place from connect-v2 and the p2p work:

- **Xgress lives at the SDK** for V2 circuits. Sequence numbers, send/
  receive buffers, RTT estimation, retransmit scheduling — all of it
  survives in the SDK process across any transport-level event.
  `edgeConnXgress` (`sdk-golang/ziti/edge/network/conn_xgress.go`) holds
  the xgress; the router channel is just a `Path` underneath.
- **`MultiPathAdapter`** (`sdk-golang/ziti/edge/network/multi_path_adapter.go`)
  implements `xgress.DataPlaneAdapter` as a list of paths with a
  selector. The SDK-side path/conn refactor is in flight on the
  `move-xgress-up` branch and is not yet complete: today some router-
  specific state (connId, defaultSender, routerId, channelLabel,
  XgressAddress) still lives on `edgeConnXgress` rather than on the
  `RouterChannelPath`. The design in this doc assumes the target shape
  of that refactor — see "Path owns router-specific state" under Phase A.

  Path-identification interface: today `Path.ID()` returns the
  transport-class constant (e.g. `"router-channel"`). This is fine for
  Phase A (one path at a time) but breaks down in B2 where two
  router-channel paths exist concurrently and can't be told apart in
  logs or metrics. Before B2 the interface gets a `Type() string` /
  `ID() string` split: `Type` is the transport class (low-cardinality,
  stable, for metric aggregation), `ID` is the instance identifier
  (semantics defined by the transport — for router-channel paths it's
  the router id, for direct-DTLS it's the negotiated session/peer id,
  for phase C long-lived paths it's the controller-issued path id).

What's missing is the control protocol that lets the SDK tell the
controller "the circuit moved" without involving the old ingress router,
plus controller-side state to keep the circuit alive across the gap.

## Structural requirements

Three properties are load-bearing for every approach below:

1. **End-to-end reliability state survives the transport swap.** Already
   true for V2 circuits. The SDK's xgress doesn't care which transport
   carried the bytes; sequence numbers and retransmit state outlive any
   single router channel.
2. **The controller can re-splice a circuit without involving the
   old ingress router.** Smart reroute already does this for transit
   routers; extending it to also swap the ingress router is mechanically
   small but conceptually new. The controller must be willing to act on
   "this SDK says the circuit is at this new router" instead of only
   acting on its own cost calculations.
3. **The circuit identity persists during the gap.** If the controller
   tears the circuit down the instant the old ingress reports a
   forwarding fault, the SDK has nothing to attach to. A reroutable
   circuit needs a grace window during which the circuit object lives
   without an active ingress.

(3) is controller-side, not router-side. The old ingress router can die
however it wants — there's no SDK to deliver to anyway. What matters is
the circuit object on the controller staying alive long enough for the
SDK to surface elsewhere.

## Phased plan

A → (B1 | B2 | B3 | B4) → C, ordered fastest-to-slowest. Each phase
ships independently. B1, B2, B3, and B4 all build on A's machinery
and are independent of each other — sequence between them is a
question of which value lands first, not a dependency. C is the
strategic destination that subsumes the rest.

### Phase A — Reroute-token (express lane)

The simplest mechanism that gives the SDK survival across ingress
router loss.

**Wire additions:**

- **Capability bit** `RouterCapabilitySDKReroute` in the router hello.
  Same negotiation pattern as `RouterCapabilityConnectV2`. SDK consults
  via `edge.IsRouterCapable(...)`.
- **`RequestReroutable` flag** on `ContentTypeConnectV2`. SDK opts in
  per-circuit. Controller honors only on a router that also advertises
  the capability. Phase A wires only the *dialer* side of this flag;
  the terminator/bind-side use is reserved for B1 and shouldn't be
  plumbed until B1's signaling choice is made (bind flag vs dial
  response vs peer data — see B1). Reserving the header now is fine;
  implementing bind-side behavior is not a Phase A task.
- **Reroute token** in the `state_connected` reply, when the circuit
  was created reroutable. A controller-signed (JWT-like) token with
  claims `{circuitId, identityId, serviceId, iteration,
  ownerControllerId}`, verifiable by routers and controllers via
  cluster JWKS (see Token design below).
- **`Reason` field on `IngressFault`/`EgressFault`** — new enum on
  the `Fault` message. **Numbering is load-bearing** — the proto3
  zero value (what an absent field, or an old router that doesn't
  stamp it, decodes to) MUST be a teardown reason, never the
  Limbo-eligible one:

  ```
  enum FaultReason {
    ReasonUnspecified = 0;  // absent / old peer → teardown, never Limbo
    ChannelClosed     = 1;  // the ONLY Limbo-eligible reason
    XgClose           = 2;  // explicit SDK-initiated close → teardown
    AccessLoss        = 3;  // policy/RDM-driven closure → teardown
  }
  ```

  `ChannelClosed` is transport loss (SDK edge channel dropped) — the
  only recoverable case. The router's `cleanupXgressCircuit` stamps
  the reason at fault-emit time (it already knows which caller invoked
  it — `HandleClose` vs `CloseConn` vs `CloseForDialAccessLoss`).
  Limbo entry requires `reason == ChannelClosed` *explicitly set
  (=1)*; everything else, including `ReasonUnspecified` (0), takes
  the teardown path. This makes the compatibility story safe: an old
  router that doesn't set the field, or a new router that hasn't been
  updated, emits faults that decode to `ReasonUnspecified = 0` →
  teardown, never resurrection-eligible. (If the enum were declared
  in listing order with `ChannelClosed` at 0, absent/old faults would
  silently become Limbo-eligible — a revocation/explicit-close bypass.
  Hence the explicit numbering.) Additive: an old *controller* ignores
  the field and keeps its existing fault handling.

  Compatibility test required: a `Fault` with no `Reason` set MUST
  decode to `ReasonUnspecified` and take teardown, never Limbo.
- **`ContentTypeTakeoverCircuit`** — new message, SDK → router. Carries
  the reroute token (the SDK's circuit/identity/service/iteration are
  all signed claims *inside* the token — the SDK doesn't supply them
  separately, and the router reads them by verifying the token). Before
  sending it, the SDK opens the new router channel and pre-registers
  the (live) conn in the new router's mux under a freshly-allocated
  connId — so reverse-direction payloads have a sink the moment the
  new router's forwarder goes live. The new router verifies the token
  signature (cluster JWKS), checks `token.identityId` against the
  authenticated edge identity, fail-fasts on
  `checkAccess(token.serviceId, DialPolicy)`, allocates a
  router-assigned IngressId, registers an `xgEdgeForwarder` at it, then
  forwards a `TakeoverCircuitRequest` carrying `{connId, token,
  proposedIngressId, authenticatedIdentityId, apiSessionToken}` to the
  owning controller. (Router-assigned IngressId follows the same
  precedent as V3 router-assigned circuit IDs and lets the new ingress
  register its forwarder before the controller commits routes — see X1
  and C2. `authenticatedIdentityId` lets the controller re-check the
  identity match defense-in-depth, since it doesn't see the edge
  channel. `apiSessionToken` lets the controller's takeover-time authz
  check use the *same* inputs as `CreateCircuitV3`'s dial-time authz —
  identity + session + service policy + posture — so revocation parity
  with original dial is preserved across the Limbo gap, not just
  identity/service policy. The router already has the session token
  from the edge channel; the SDK doesn't need to do anything extra.)
- **Controller-side handler.** Verifies the token signature (cluster
  JWKS), re-checks identity match (`token.identityId ==
  authenticatedIdentityId`), iteration freshness (`token.iteration ==
  circuit.iteration`), and current dial authorization for
  `circuit.ServiceId` — all under the CAS guard (see X1). On
  success, computes a new path from the new ingress router to the
  unchanged terminator router and runs the existing
  `SmartRerouteAttempt`-style splice (full sequence in X1). Reply to
  the SDK is essentially a `state_connected` for the same circuit id,
  carrying a fresh reroute token at the new iteration.

**Takeover reply contract.** The reply travels SDK ← new router. The
new router originates transport-level codes (it can detect "no channel
to the owner" before the controller is involved); the controller
originates the rest. All arrive at the SDK as one reply whose
`TakeoverResultCodeHeader` enum the SDK maps to a disposition:

| Code | Origin | SDK disposition |
| --- | --- | --- |
| `Success` (+ confirmed IngressId, XgressCtrlId, fresh token, peer data) | controller | AddPath, resume |
| `OwnerUnreachable` (router has no channel to the owning controller) | router | retryable — try another router that may reach the owner |
| `Busy` (`ErrCircuitMutationInProgress`, CAS miss; optional retry-after) | controller | retryable — short backoff, retry |
| `RouteInstallFailed` (partial new-path install, rolled back per C3) | controller | retryable — try another router (different path) |
| `TokenRejected` (signature invalid, identity mismatch, stale iteration, or authz denied) | controller | fatal — abort recovery, surface closed |
| `NotReroutable` (circuit exists but isn't in Limbo) | controller | fatal — abort |
| `NotFound` (circuit gone — grace expired or removed) | controller | terminal — abort |

SDK maps `{OwnerUnreachable, Busy, RouteInstallFailed}` → retryable and
`{TokenRejected, NotReroutable, NotFound}` → fatal. Reuses the existing
edge result/error pattern — no new content-type plumbing beyond the
header enum. This is the concrete form of the retryable/fatal split the
recovery loop (Recovery candidate selection) and X1 failure modes refer
to.

**Mixed-version compatibility.** The capability bit + reroute-token
fallback gives a clean version-mismatch matrix:

| Combination | Result |
| --- | --- |
| Old SDK + new router/controller | No change. SDK doesn't know to set `RequestReroutable`; circuits aren't reroutable. |
| New SDK + old router (no `RouterCapabilitySDKReroute` in hello) | SDK detects missing capability via `edge.IsRouterCapable`, doesn't set `RequestReroutable`. Conn is non-reroutable. (The flag header is forward-compatible — if accidentally sent, an old router ignores unknown headers.) |
| New SDK + new router + old controller | Router forwards `RequestReroutable` in `CreateCircuitV3Request`. Old controller doesn't recognize it, creates a non-reroutable circuit, replies without a reroute token. SDK sees no token in `state_connected` → marks the conn non-reroutable. |
| New SDK + new router + new controller | Full Phase A. |
| Transit routers in any mix | Phase A has no transit-router surface. They forward bytes as today, no capability needed. |

Canonical SDK detection signal: **"no reroute token in `state_connected`
reply ⇒ not reroutable."** A single fail-closed check covers every
downgrade path; the SDK doesn't need to enumerate which component
declined. The SDK only attempts `TakeoverCircuit` against routers that
both advertise `RouterCapabilitySDKReroute` AND for which it has a
reroute token to present — the takeover RPC never fires against a
downgraded path.

**Controller-side state:**

- New `Reroutable` flag on `model.Circuit`. Set at creation time
  based on `RequestReroutable` + router capability. Single-router
  circuits (ingress router == terminator router) ARE reroutable — see
  C3 below — but a same-SDK self-dial (the same edge channel hosts
  both ends) is not, because both ends collide in the router's
  circuit-id-keyed `xgCircuits` index (a pre-existing limitation; see
  "Self-dial on one router" in Open questions).
- New circuit state `Limbo` (or extend `Rerouting`) with an associated
  deadline. Limbo means "some component this circuit depends on is
  temporarily unreachable but the rest of the path is intact; hold for
  a grace period instead of tearing down, and recover if possible."
  It has **two recovery mechanisms**, both riding on the end-to-end
  xgress reliability layer (a transient outage is just a window of loss
  the SDK↔SDK retransmits cover):
  - **SDK takeover** — the SDK reattaches the circuit via a router
    (a different one, or the same one once it comes back). Recovers
    ingress/edge-connection loss.
  - **Underlay recovery** — a failed component comes back on its own
    (a transit link redials via the xlink registry; an unreliable
    edge connection re-establishes). The controller retries reroute
    once topology recovers; no SDK action needed.

  This generalizes Limbo from "SDK lost its router" to "ride out
  transient underlay failures." On an unreliable underlay, tearing
  down and rebuilding on every blip is wasteful and disruptive; the
  grace period lets transient drops self-heal.

  **Which mechanism recovers which entry depends on whether the SDK
  has a local signal.** SDK takeover requires the SDK to *notice* it
  needs to act — which only happens when its own channel/path dies.
  Entries where the SDK is still connected to a healthy ingress
  recover via the underlay path or the controller's reroute-retry, not
  via SDK takeover:

  | Limbo entry | SDK local signal? | Recovery |
  | --- | --- | --- |
  | E1 (`ChannelClosed`) | yes — its channel died | SDK takeover |
  | E2, router died (SDK channel dropped too) | yes | SDK takeover |
  | E2, router lost only its ctrl channel (SDK still connected) | no | underlay / B4 presence grace |
  | E3 (`ForwardFault`, transit broke, SDK still connected) | no | underlay recovery (X3), else controller reroute-retry, else grace expiry |

  E3-with-SDK-connected is deliberately **not** an SDK-takeover case:
  the SDK has no trigger (its edge channel is fine), and the recovery
  is the controller's/underlay's job. If the broken transit never
  heals and the controller finds no alternative path, the circuit
  waits out grace and tears down → the SDK sees a closed conn and the
  app re-dials onto a fresh path. The one sliver this leaves
  unrecovered — transit dead, SDK still connected, no controller
  alternative, but a *different ingress* would have found a working
  path — is accepted for Phase A. Recovering it proactively would
  need either the SDK acting on a *weak* signal (acks stalled for a
  circuit despite a healthy edge channel → try migrating; heuristic,
  risks churn on transient stalls) or an explicit controller→SDK
  degraded-circuit notification. Both are deferred future work.

  Specific entry, in-Limbo, and exit behaviors:

  **Entry — E1 (SDK loses its ingress router, `IngressFault` with
  `Reason == ChannelClosed`):** the router's `cleanupXgressCircuit`
  sends a per-circuit `Fault { Subject: IngressFault, Reason:
  ChannelClosed, Id: <circuitId> }` and unregisters its V2 forwarder.
  Only the `ChannelClosed` reason is Limbo-eligible — it means the
  SDK's transport dropped, which is exactly what the SDK can recover
  from. Other reasons go to existing teardown:
  - `XgClose` — the SDK explicitly closed the conn. Normal close, not
    a candidate for resurrection. Tear down.
  - `AccessLoss` — policy/RDM revoked the identity's access. The
    circuit MUST die and MUST NOT be resurrectable via a stale token
    (see threat model). Tear down.
  - `ReasonUnspecified` (0) / absent — the proto3 zero value, which
    is what an old router or an unstamped fault decodes to. Conservative:
    teardown, never Limbo, no resurrection risk. (See wire additions
    for why the enum numbering pins this.)

  For `ChannelClosed` on a `Reroutable` circuit: transition to Limbo
  immediately; do NOT attempt `rerouteCircuit` (no path can avoid a
  dead source endpoint). Source router self-cleans its local state.
  Terminator-side and any transit routers keep their forward tables
  and xgress destinations; their state is exactly what the eventual
  takeover splice will reuse.

  **Endpoint-scoped cleanup (closes C3).** Today
  `cleanupXgressCircuit` calls circuit-scoped
  `forwarder.EndCircuit(circuitId)`, which removes *every* destination
  for the circuit. That's fine for a multi-router circuit (the dialer
  router only hosts the dialer endpoint), but wrong for a
  **single-router circuit** — ingress and terminator on the same
  router (`Path.Nodes[0] == Path.Nodes[len-1]`, no links), common in
  small/single-ER deployments where an SDK dials a service a peer
  hosts on the same router. There, circuit-scoped `EndCircuit` on the
  dialer's disconnect would also tear down the co-located terminator's
  endpoint — destroying exactly the state Limbo must preserve.

  Fix: make `cleanupXgressCircuit` *endpoint-scoped* — remove only the
  closing endpoint's `xgEdgeForwarder` (by address) and its
  forward-table entries; if it was the last endpoint for the circuit
  on this router, the local circuit state is effectively ended (same
  as today); if another endpoint remains (the co-located terminator),
  preserve it. Behaviorally identical for multi-router circuits; for
  single-router it preserves the terminator. Stays reason-aware:
  `ChannelClosed` → endpoint-scoped + Limbo; `XgClose`/`AccessLoss` →
  full circuit teardown. (This is arguably the more-correct general
  behavior: cleanup should retire the endpoint that closed, not the
  whole circuit.) Note `forwarder.RegisterDestination` is already
  keyed by address, so both ends coexist in the router-global
  forwarder; only the cleanup path was over-broad.

  With this, a single-router circuit reroutes by transforming into a
  two-router circuit: the SDK comes back via a different ER R2 (with a
  link to the terminator's router R), and the takeover splice computes
  the path R2→R, with the preserved terminator endpoint on R receiving
  forwarded traffic. Same-router reconnect (SDK back to R, R still up)
  keeps the path single-router and still reroutable. The one case it
  can't save: **R itself dying** — the terminator is pinned to R and
  dies with it; no dialer-side reroute can recover a vanished
  terminator (terminator-side reroute is B1).

  SDK side: the dead `RouterChannelPath` is removed from the conn's
  `MultiPathAdapter`; the conn enters recovery state; the xgress and
  its read/write adapters survive the gap. (See "Path owns router-
  specific state" earlier in this section.)

  **Entry — E2 (router on the path goes offline):** the controller's
  ctrl channel to the router closes. `Network.DisconnectRouter`
  removes the router's links, and `Network.RerouteLink` iterates
  every circuit using each removed link, calling `rerouteCircuit`.
  For dialer-side circuits where the dead router IS `Path.Nodes[0]`,
  `rerouteCircuit` fails (no path computable from a disconnected
  source). For other circuits where the dead router is transit,
  `rerouteCircuit` may succeed via a new transit path.

  No separate Limbo trigger handler needed: E2 flows through the
  same "rerouteCircuit fails → Limbo only if SDK-recoverable,
  RemoveCircuit otherwise" rule that E3 uses (see E3 for the
  terminator-reachable predicate). The Phase A change is at the
  failure-fallback callsite, applied uniformly in both
  `Network.fault()` and `Network.rerouteLink()`. So when the dead
  router is the terminator (`Path.Nodes[len-1]`), E2 tears down
  rather than entering Limbo.

  Single-router circuits and E2: a single-router circuit's only
  router going offline means the terminator (pinned to that router)
  is gone — unrecoverable by dialer-side reroute, so it tears down,
  same as any terminator-router loss. This is distinct from a
  single-router circuit whose router stays up but the SDK loses its
  *connection* to it: that's E1/`ChannelClosed`, recoverable via the
  endpoint-scoped cleanup described under E1 (transform to two-router
  via a new ER, or same-router reconnect). Don't conflate
  "single-router circuit" (ingress==terminator, reroutable while the
  router lives) with "single-ER deployment" (SDK has one ER but the
  terminator is elsewhere — an ordinary multi-router circuit).

  **Entry — E3 (`ForwardFault` on a Reroutable circuit, multiple
  possible causes):** a generic forwarding-failure signal. The
  controller can't tell from the message alone whether it's something
  it can fix or not. Causes include: lazy follow-up after E1's
  IngressFault already triggered Limbo; transit link broken;
  transit router went offline; ingress router can't reach its next
  hop while the SDK is still happily connected to it (no IngressFault
  gets sent); stale forwarder state; scanner-detected idle.

  Uniform handling regardless of cause: if the circuit is already in
  Limbo, log and ignore (idempotency). Otherwise attempt
  `rerouteCircuit` via the existing retry path. On success, the
  circuit moves to a new path transparently — the SDK doesn't know
  and doesn't need to know. On failure, fall through to Limbo ONLY
  if the failure is recoverable by SDK-side reroute — i.e. the
  terminator router (`Path.Nodes[len-1]`) is reachable, so a new
  dialer ingress could plausibly reach it. The predicate must be
  race-free against `DisconnectRouter`'s ordering (see C3 below);
  it threads the failing component (the disconnecting router id, or
  the failed link) into the decision rather than just snapshotting
  the connected-map:

  - reroute failed AND `Path.Nodes[len-1] != disconnectingRouter`
    AND the terminator router is still in the connected map → the
    dead component is the dialer ingress (or transit reachable only
    via it); SDK migrating to a new ingress can help → Limbo.
  - reroute failed AND (`Path.Nodes[len-1] == disconnectingRouter`
    OR terminator router not in the connected map) → the terminator
    endpoint is dead; Phase A has no terminator-side reroute, the
    SDK can't help → existing teardown (RemoveCircuit).

  This keeps the E3 fall-through from holding terminator-dead
  circuits alive until grace expiry when no recovery is possible.

  **C3 fix — race-free against `DisconnectRouter` ordering, two
  layers.** `DisconnectRouter` (`network.go:464`) today runs
  `RerouteLink` *before* `Router.MarkDisconnected`. During the
  cascade the disconnecting router still appears in the connected
  map, so a snapshot-based "terminator reachable" check would
  wrongly accept it as alive — which is also a latent pre-existing
  bug for `shortestPath` (it'd consider the dying router as a
  viable candidate).

  Fix both layers:
  1. **Reorder `DisconnectRouter`** so `Router.MarkDisconnected(r)`
     runs *before* the `RerouteLink` cascade. Makes the global
     router state truthful during reroute, so `shortestPath` and
     all connected-map reads see the dying router as gone — which
     is what they should have seen all along. Sole caller is
     `handler_ctrl/close.go`, contained change.
  2. **Thread the disconnecting router id into the fallback
     decision** (the explicit-cause check above). Makes the predicate
     self-evidently race-free regardless of any future ordering
     changes elsewhere — the explicit cause is authoritative even if
     a future refactor shuffles the cascade.

  Belt-and-suspenders: layer (1) is the principled global fix;
  layer (2) is the local invariant the predicate relies on. The
  `LinkFaulted`/`RerouteLink` path (called from link failures, not
  router disconnects) doesn't have the same race — link faulting
  doesn't toggle router-connected state, so a connected-map snapshot
  is truthful there — but threading the failure cause through the
  reroute helper anyway keeps the predicate uniform across both
  entry paths.

  Net effect for `Reroutable` circuits: the controller exhausts
  transit-repair options first; Limbo is the fallback when those
  fail AND the failure is SDK-recoverable. The pre-Phase-A pattern
  of "reroute then unroute on failure" becomes "reroute then Limbo
  on SDK-recoverable failure, unroute otherwise."

  **Reroute commit discipline (closes C2) — do NOT use the existing
  `rerouteCircuit` as-is.** The current `rerouteCircuit`
  (`controller/network/network.go:1116`) sets `circuit.Path = cq`
  *before* the sendRoute loop and, on a route failure, returns with
  `circuit.Path` left at the uncommitted new path and `oldPath`
  discarded. Falling back to Limbo from that state corrupts the
  baseline: grace-expiry teardown (X2) would unroute a path that was
  only partially installed, and a later takeover (X1) would compute
  its splice from a path that never committed. The Limbo-fallback
  reroute must instead use the **commit-after-all-routes-succeed +
  rollback** discipline already specified for X1: install to every
  path router tracking acceptance, set `circuit.Path = cq` only after
  all succeed, and on any failure roll back the accepted new routers
  (terminator-excluded, per X1) and leave `circuit.Path = oldPath`
  before entering Limbo. So Limbo's baseline is always the last
  fully-committed path. Factor this into a shared commit/rollback
  helper used by both the X1 takeover splice and the E2/E3
  fault/link reroute — one rollback discipline, not two.

  (Pre-existing hazard, flagged: `smartReroute` (`network.go:1152`)
  has the same mutate-`Path`-before-install structure and the same
  latent partial-install bug. It's out of strict Phase A scope, but
  the same shared helper would fix it; recommend folding it in as a
  cleanup when the helper lands.)

  **Tear-down case (not a Limbo entry) — `EgressFault` on a
  Reroutable circuit:** terminator SDK's edge channel to its router
  closed; the router's `cleanupXgressCircuit` sent
  `Fault { Subject: EgressFault, Id: <circuitId> }`. Phase A has no
  terminator-side reroute (that's phase B1), so the dialer-side
  Limbo machinery can't help — there's no splice that gets data to
  a dead terminator endpoint. Tear down the circuit via existing
  handling: unroute all surviving routers, remove the circuit
  object. The dialer's ingress router sends `ContentTypeStateClosed`
  to the dialer SDK on unroute; the application sees a normal
  connection-closed error.

  If the circuit was already in Limbo when EgressFault arrives
  (dialer-side died first via E1, then terminator died too): tear
  down anyway. The dialer SDK's eventual TakeoverCircuit attempt
  finds no circuit to claim and falls back to a fresh dial.

  In phase B1, EgressFault on a Reroutable circuit becomes
  symmetric to E1 — transitions to Limbo waiting for terminator-
  side takeover instead of tearing down.

  **During Limbo — D1 (faults arrive for the Limbo circuit):**

  - `ForwardFault` referencing the Limbo circuit: log and ignore.
    Covered in E3's handling; explicit here for symmetry.
  - `LinkFault` triggers `Network.rerouteLink`, which iterates
    circuits using the failed link. Add a guard: skip circuits in
    Limbo. `rerouteCircuit` itself early-returns on Limbo input as
    defensive coverage at a second callsite.
  - `IngressFault` referencing the Limbo circuit: shouldn't happen
    (the source router for this circuit's IngressFault was already
    the dead-channel router, which has self-cleaned). If it does
    arrive (e.g., the router came back briefly and re-sent), log
    and ignore.

  In-Limbo rule, stated once: faults and smart-reroute do not mutate
  a Limbo circuit. The only paths that may exit Limbo are the three
  defined exits — X1 (`TakeoverCircuit` success), X2 (grace expiry),
  and X3 (underlay recovery: a failed link redials, the controller
  retries reroute, and the circuit re-installs on the recovered
  topology). Any other event for a Limbo circuit is informational
  and must not change its state.

  **During Limbo — D2 (smart-reroute timer):** `Network.smart()`
  periodically considers all circuits for cost-driven path
  improvement. For a Limbo circuit, smart-reroute attempting to
  recompute and install a new path is at best wasted work
  (forwarding still fails because the SDK xgress is gone) and at
  worst races against an in-flight `TakeoverCircuit`. Simplest rule:
  `getRerouteCandidates` skips circuits whose state is Limbo. Limbo
  circuits re-enter the smart-reroute pool only after takeover
  exits Limbo (or never, if grace expires and the circuit is
  removed).

  **During Limbo — D3 (scanner `CircuitConfirmation` arrives):**
  Routers periodically send `CircuitConfirmation` for forward-table
  entries that have exceeded the local idle threshold. A Limbo
  circuit looks idle to every router on its surviving path — no
  data flows because the dialer's xgress endpoint is dead.

  Controller's existing handler
  (`controller/handler_ctrl/circuit_confirmation.go`) has two
  relevant paths:

  - Circuit exists and reporting router is on the path:
    `checkCircuitMaxIdle` runs. If `service.MaxIdleTime` is
    exceeded AND the reporting router is an endpoint,
    `RemoveCircuit` fires. **This is wrong for a Limbo circuit**
    — the terminator-side scanner will hit max-idle while the
    dialer is still mid-recovery, prematurely terminating Limbo.
  - Circuit doesn't exist or reporting router is off-path:
    `sendUnroute` is sent. Safe for Limbo (off-path routers
    aren't part of the surviving path anyway).

  Phase A change: `checkCircuitMaxIdle` early-returns if
  `circuit.State == Limbo`. Limbo's grace deadline is the only
  timeout that may retire a Limbo circuit. Normal max-idle resumes
  after Limbo exits (success or expiry).

  **Exit — X1 (`TakeoverCircuit` succeeds):** SDK has dialed a new
  router and sent `ContentTypeTakeoverCircuit` with the reroute
  token. The critical ordering difference from initial ConnectV2:
  the terminator's xgress is ALREADY live and may retransmit its
  unacked window the instant a route reaches it, so both sides must
  pre-register their forwarding state before the controller commits
  routes — otherwise reverse-direction payloads arrive with no
  destination and get dropped/faulted (see C2 reasoning).

  Pre-registration (before the controller does anything):
  - SDK, before sending `TakeoverCircuit`: opens the new router
    channel, allocates a connId on the new router's mux, and
    registers the (existing, live) conn under it. So reverse
    payloads delivered by the new router's `xgEdgeForwarder` have a
    mux sink to land on.
  - New router, on receiving `TakeoverCircuit`: allocates a new
    IngressId locally (router-assigned, same precedent as V3
    router-assigned circuit IDs), registers an `xgEdgeForwarder` at
    that IngressId in its forwarder, then sends
    `TakeoverCircuitRequest` carrying `{connId, token,
    proposedIngressId}` to the owning controller.

  Controller-side sequence:

  0. New router verifies + fail-fast checks (all on the
     controller-signed token, which the router can verify via cluster
     JWKS — see Token design):
     - Verify the token signature. Invalid → reject.
     - **Identity match (closes C2):** compare `token.identityId` to
       the authenticated edge-channel identity. Mismatch → reject. The
       router holds both the verified token and the authenticated
       identity, so this check is correct and local — no need to ship
       the identity to the controller for the primary check.
     - **Dial-access fail-fast:** `checkAccess(token.serviceId,
       DialPolicy)` for the authenticated identity via RDM (same as
       `processConnectV2`). The `serviceId` is the controller-signed
       claim, so the SDK can't substitute one. Denied → reject, no
       controller round trip.
     All three reject before any controller round trip.
  1. New router reads `ownerControllerId` from the token,
     dispatches `TakeoverCircuitRequest` carrying `{connId, token,
     proposedIngressId, authenticatedIdentityId}` to that controller.
  2. Controller verifies the token signature (via cluster JWKS) and,
     defense-in-depth, re-checks `token.identityId ==
     authenticatedIdentityId`. (Iteration freshness is checked in
     step 4 under the CAS guard.)
  3. Controller acquires the `circuit.Rerouting` CAS guard via the
     new `takeoverCircuit` variant, which returns a distinct
     `ErrCircuitMutationInProgress` on CAS miss (vs the existing
     `rerouteCircuit`'s nil-on-miss silent no-op). On miss, reply
     busy/conflict; new router relays a retryable error to the
     SDK.
  4. Re-validate state under the guard: circuit still in Limbo,
     `token.iteration == circuit.iteration`, `token.side == Ingress`
     (Phase A only does ingress-side takeover; rejecting any other
     side guards against future B1 tokens being misrouted to this
     handler), owning controller is still us.
  4a. **Authoritative authorization re-check (closes C1, equivalent
     to dial-time authz).** Still under the guard, before any route
     install, perform the **same authz check `CreateCircuitV3` does
     at dial time** — identity + API session + service policy +
     posture/RDM state — using `(token.identityId, circuit.ServiceId,
     apiSessionToken)` from the request. (Also asserts
     `token.serviceId == circuit.ServiceId` as a consistency check.)
     Evaluated against current state at the moment of revival, so it
     catches anything revoked between dial and revival: identity-on-
     service policy change, API session expiry, posture state change.
     The dial-time and takeover-time gates are the same code path,
     just invoked in different contexts. The router's step-0
     fail-fast catches most cases against its RDM view; this
     controller check is the lag-free authoritative backstop. Denied
     → fatal `TokenRejected`-class reply + tear the circuit down (do
     not revive a circuit whose original-dial authorization no longer
     holds).
  5. Compute the new path via a takeover-specific constructor
     `BuildTakeoverPath(oldPath, newIngressRouter, proposedIngressId)`
     — do NOT reuse `CreatePathWithNodes` (which mints fresh UUIDs
     for *both* `IngressId` and `EgressId`) or `UpdatePath` (which
     preserves both but assumes the source router is unchanged).
     Takeover replaces the ingress side while preserving the
     terminator side exactly:
     - `Nodes` from `shortestPath(newIngressRouter, oldPath.Nodes[len-1])`,
       `Links` from `setLinks` over those nodes.
     - `IngressId = proposedIngressId` (router-allocated, see
       pre-registration in step 1).
     - `EgressId = oldPath.EgressId` **preserved** — this is where
       the terminator's `xgEdgeForwarder` is registered; minting a
       fresh one here would silently break the circuit post-commit
       (route install succeeds, but reverse payloads target an
       address with no registered destination → drops/faults).
     - `TerminatorLocalAddr`, `TerminatorRemoteAddr`, terminator
       binding and peer data — preserved from `oldPath`.
     - `InitiatorLocalAddr`, `InitiatorRemoteAddr` — refreshed from
       the new edge channel.

     Required tests: multi-router takeover and single-router→
     two-router takeover (the C3 round-3 case where the SDK was on
     router R and comes back via R2→R), both asserting
     `newPath.EgressId == oldPath.EgressId`.
  6. Build route messages with `SmartRerouteAttempt` (suppresses
     the Egress block — terminator already established). Send to
     every router on the new path, **tracking which routers
     accept** (attendance map, mirroring `routeSender`). The new
     ingress's reverse-direction destination already exists
     (pre-registered), so no early-arrival window.

     On any install failure — ROLL BACK, do not commit:
     - Circuit-scoped Unroute to every accepted new-path router
       **except the terminator router** (see "rollback excludes
       the terminator" below).
     - New router tears down its speculative `xgEdgeForwarder`;
       SDK unwinds its mux registration (C2 pre-registration
       cleanup).
     - Leave the circuit in Limbo with the **old** `Path` still
       recorded, so grace expiry has a definite teardown target.
     - Reply retry-busy. STOP — steps 7+ do not run.
  7. (commit — point of no return) All new routes installed
     successfully → `circuit.Path = newPath`, exit Limbo, refresh
     `UpdatedAt`, increment `circuit.iteration`, mint a fresh
     reroute token at the new iteration.
  8. (post-commit cleanup, best-effort) Send Unroutes to old-path
     routers not on the new path (existing `unrouteRemovedPathNodes`
     pattern). The new path is already serving traffic; this is
     cleanup, not on the critical path. Routers on both paths keep
     their forward tables; stale old-IngressId entries on shared
     transit are inert.
  9. Reply to the new router's `TakeoverCircuitRequest` with the
     (now confirmed) IngressId, peer data, fresh token.
  10. Release the `Rerouting` CAS.
  11. New router replies to the SDK with state_connected-
      equivalent (its `xgEdgeForwarder` was already registered
      pre-request).
  12. Fire `CircuitUpdated` event with trigger `sdk-takeover`.

  **Rollback excludes the terminator.** A circuit-scoped Unroute
  fires `EndCircuit` → `UnregisterDestinations` → `Unrouted()` on
  the router's xgEdgeForwarder. On the terminator router that
  forwarder IS the terminator SDK's endpoint — the state Limbo
  exists to preserve. Unrouting it would send `StateClosed` to the
  terminator SDK and kill the circuit on a *failed* takeover, the
  opposite of intent. So rollback unroutes only the new-ingress and
  new-transit routers (which hold purely speculative new-path
  state), never the terminator. Circuit-scoped Unroute is safe on
  shared transit routers because the old path is already
  non-functional (its ingress is dead — that's why we're in
  Limbo); the next takeover recomputes a fresh path and re-installs
  whatever it needs.

  Tolerated wrinkle: if the terminator router accepted a new route
  before the failure, it keeps stale new-path forward entries
  through the rollback. These are inert in the forward direction
  (the SDK never `AddPath`'d, so nothing flows over the new
  IngressId) and produce only ignorable Limbo faults in the
  reverse direction (dropped per D1). They're corrected by the next
  successful takeover (its route message overwrites the
  terminator's forward entries) or by grace expiry (full
  circuit-scoped Unroute, when the circuit is meant to die). Eager
  scrubbing would need a path/address-scoped unroute primitive
  (deferred — see `routing-v2.md`); not worth it for Phase A.

  SDK side: on the reply, calls `MultiPathAdapter.AddPath` with the
  `RouterChannelPath` for the new router (the mux sink was already
  registered pre-request). The xgress flushes its unacked window
  over the new path; the terminator's retransmits fill the reverse
  direction.

  ACKs in the pre-`AddPath` window: because the new router's
  `xgEdgeForwarder` and the SDK's mux sink are pre-registered (C2),
  reverse payloads can be *ingested* by the SDK's xgress before the
  SDK calls `AddPath` — and the ACKs the xgress generates for them
  may have no healthy path in the adapter to send on yet (the new
  Path isn't added until the reply lands). Those early ACKs are
  allowed to be dropped; the normal xgress retransmit + duplicate-
  ACK dedup behavior recovers them. Worth keeping in mind for tests
  so a brief ACK-drop window after pre-registration isn't read as a
  regression.

  Failure modes after the SDK has sent TakeoverCircuit (all unwind
  the speculative pre-registrations — new router tears down its
  `xgEdgeForwarder` at the proposed IngressId, SDK removes its mux
  registration for the new connId):
  - Token validation fails (replay/stale iteration, identity
    mismatch): fatal reply; SDK marks the conn unrecoverable and
    surfaces connection-closed.
  - CAS miss: retry-busy reply; SDK retries after a short delay
    or tries another router.
  - Route install fails on some router on the new path: retry-busy
    reply; controller cleans up partial new-path routes (C3); SDK
    tries another new router.
  - Router has no channel to owning controller (Phase A
    constraint): immediate error reply; SDK tries another router
    that may have a channel to the owner.

  In all failure modes the circuit stays in Limbo until SDK gives
  up or grace expires.

  **Exit — X2 (grace timer expires with no takeover):**

  1. Grace timer fires for the Limbo circuit on the controller.
  2. Acquire `circuit.Rerouting` CAS guard. On miss, a takeover
     or other reroute is mid-flight; defer briefly and recheck.
  3. Under the guard, verify the circuit is still in Limbo. If
     not (takeover already exited Limbo, or another path cleaned
     up), no-op.
  4. Send Unroutes to all surviving routers on the path.
     Terminator router's xgEdgeForwarder unregisters; its
     `Unrouted()` callback sends `ContentTypeStateClosed` to the
     terminator SDK. Transit routers drop forward-table entries.
     The failed ingress router gets a best-effort unroute (likely
     a no-op since its state was already self-cleaned).
  5. Remove the circuit from `network.Circuit`. Fire
     `CircuitUpdated` event with trigger `grace-expired` (or
     `CircuitFailed`).
  6. Release the `Rerouting` CAS.

  SDK side: if the SDK is still attempting `TakeoverCircuit`, the
  controller replies "circuit not found"; SDK marks the conn
  unrecoverable, application sees a normal connection-closed
  error. If the SDK already gave up (its own retry exhaustion),
  grace expiry on the controller is just orphaned-state cleanup;
  the SDK side already closed the conn.

  Race with concurrent takeover: CAS guard prevents simultaneous
  mutation. Whichever side wins the CAS proceeds; the loser finds
  inconsistent state on its re-check and no-ops.

  **Exit — X3 (underlay recovery, no takeover):** the circuit
  entered Limbo because reroute failed with no alternative path
  (a transit link on its path went down, both endpoints fine — see
  E3). The xlink registry redials the failed link with backoff; when
  it re-establishes, the controller learns via `NotifyExistingLink` /
  `RouterReportedLink`. New hook: on a link being restored, the
  controller retries `rerouteCircuit` for Limbo circuits that were
  waiting on the topology. If reroute now succeeds (the recovered
  link makes a path computable), exit Limbo — re-install routes onto
  the recovered topology, or, if the link returned with the same id
  and forward tables still reference it, traffic may resume with no
  reroute at all. No SDK involvement; the SDK's xgress rode the gap
  via retransmits. If the link never returns within grace, X2
  (teardown) fires as normal.

  Scope clarification: Phase A's X3 is the **link-redial** restoration
  hook only. Controller↔router channel reconnect — a router that
  briefly lost its ctrl channel and comes back — is intentionally
  *not* an X3 trigger in Phase A; that's B4's job (router-presence
  debounce sits *in front of* `DisconnectRouter`, so a transient
  ctrl-channel flap never cascades into Limbo in the first place when
  B4 ships). Don't try to fold ctrl-channel-only debounce into Phase
  A's X3 hook — it's the wrong layer.
- Grace period: 10s default, configurable. The SDK's recovery window
  and the controller's grace MUST be coordinated — roughly equal,
  with the SDK window slightly shorter so it gives up just before
  the controller tears down (avoids a wasted `NotFound` round trip).
  Tunable *up together* for unreliable-underlay deployments: the
  earlier "don't exceed dial timeout" guidance applies to the normal
  case, but underlay recovery is "wait for the existing connection to
  heal," not "fresh dial," so a flaky-underlay operator legitimately
  wants a longer hold than a dial timeout. Shorter favors quick
  failure surfacing to the application; longer rides out
  higher-latency mobile-network handoffs, TLS re-handshake, and
  transient underlay outages.

**Takeover uses a distinct CAS-miss return.** The existing
`rerouteCircuit` (`controller/network/network.go:1116`) returns `nil`
on `circuit.Rerouting.CompareAndSwap` miss — treated as silent
no-op by its current callers (smart-reroute and fault-driven
reroute). The SDK-takeover path can't use that semantics: it must
distinguish "splice succeeded" from "circuit was being mutated by
something else." Introduce a new `takeoverCircuit` variant that
returns a distinct `ErrCircuitMutationInProgress` on CAS miss; the
takeover handler translates this to a retryable busy/conflict reply
to the SDK. Existing `rerouteCircuit` callsites are unchanged. The
new variant also re-validates `circuit.State == Limbo` and
`token.iteration == circuit.iteration` under the guard, returning
distinct errors for each precondition failure so the reply path
can surface the right disposition (retryable vs fatal) to the SDK.

**Limbo entry is also a CAS-guarded mutation (closes C4).** Limbo
entry is a circuit-state mutation and must be serialized with the
other reroute writers — otherwise a smart-reroute or fault-reroute
already in progress could commit a path change *after* Limbo entry,
violating the no-mutation-in-Limbo invariant (D2's "smart-reroute
skips Limbo" check would have run *before* Limbo entry, so it can't
catch this without the guard). Factor Limbo entry into a single
`enterLimbo(circuit, reason, cause, deadline)` helper called from
all entry triggers (E1, the E2/E3 fallback path, X3-equivalent
entries):

1. Acquire `circuit.Rerouting` CAS — the same guard takeover,
   smart-reroute, and fault-reroute use.
2. Re-validate under the guard: the circuit is still in a
   reroutable, non-Limbo state; the reason is still valid (e.g.
   for E2, the disconnecting router is still gone — using the
   threaded explicit cause from the C3 fix); the endpoint-liveness
   predicate still holds.
3. Record `state = Limbo`, the cause, the deadline. Emit
   `CircuitUpdated{trigger: limbo-entered, cause: ...}` (the
   round-2 visibility extension).
4. Release the CAS.

CAS-miss disposition: defer briefly and recheck. If another writer
already entered Limbo for an equivalent reason, no-op. If another
writer transitioned the circuit to a state no longer Limbo-eligible
(e.g. a concurrent reroute succeeded, restoring the circuit), no-op.
This makes Limbo entry peer to takeover and grace expiry, all
guarded by the same primitive — same discipline X1 already uses.

A reroute already in progress when E1/E2/E3 fires now completes
first (Limbo entry waits for the guard). When the reroute commits,
the Limbo-entry helper re-validates and either no-ops (the reroute
fixed it) or proceeds (the reroute didn't help). A reroute starting
*after* Limbo entry skips via D2's check, which now runs under the
guard against truthful state.

**Takeover requires the owning controller.** Phase A keeps the existing
single-owner model: a circuit's takeover splice runs only on the
controller that created the circuit. The token's `ownerControllerId`
claim (see Token design below) tells the new router where to dispatch:

- New router verifies the token signature (cluster JWKS), reads the
  `ownerControllerId` claim, and dispatches `TakeoverCircuit` directly
  to that controller, selecting from its existing ctrl channels. If
  the router has no channel to the owning controller, it replies
  immediately to the SDK with an error; SDK tries a different new
  router (which may have a channel to the owner) or falls back to
  a fresh dial after exhaustion.
- The owning controller verifies the token signature, confirms the
  identity match and `token.iteration == circuit.iteration` under the
  `Rerouting` CAS guard, then runs the splice. Mismatched bindings or
  stale iterations reply with a fatal error.

Cluster-wide takeover during owner-outage is **deferred to phase B3**.
That phase adds an explicit ownership-transfer protocol (epoch on
forward tables, route-message arbitration on stale epochs, returning-
controller reconciliation). The current code has no such protocol —
routes from a second controller silently merge into the existing forward
table without updating its `ctrlId`, faults still go to the original
owner, scanner confirmations still go to the original owner. Trying to
do cluster-wide takeover on top of that produces split-brain when the
original owner returns (restarted-controller scanner confirmations get
"unroute" replies; partition-healed controllers run smart-reroute
against state they don't actually own anymore). Owner-reachable is the
honest Phase A constraint.

**SDK-side state:**

- Reroute token stored on `edgeConnXgress` alongside `circuitId`.
- A reroutable conn can be registered in *multiple* router conn muxes
  simultaneously (also the Phase B2 model). See "Path owns router-
  specific state" below for what that means structurally.

**Path owns router-specific state.** A live `edgeConnXgress` is not
bound to a single router channel. Each `xgress.Path` owns the router-
specific state for one route to one router: its connId in that
router's mux, its `XgressAddress` / `XgressCtrlId` from the takeover
reply (or the original `state_connected`), its channel sender, its
mux-registration handle. `edgeConnXgress` itself holds only circuit-
scoped state — circuitId, the xgress instance, the MultiPathAdapter,
crypto state. A conn registered with N paths is registered in N
router muxes concurrently; each mux dispatches inbound payloads to
the Path that owns that connId on that router, and the Path delivers
them to the single underlying xgress regardless of origin.

(This same Path-as-owner-of-router-state model is what Phase B2 uses
for two concurrent router-channel paths and what the p2p design uses
for a direct-DTLS path alongside the router-channel path. Phase A
doesn't introduce new mux semantics; it surfaces the fact that the
SDK already needs to be multi-mux-capable for the multi-path roadmap.)

**Reroute sequence (recovery from total router loss):**

1. SDK detects the router channel is gone — the dead `RouterChannelPath`
   notifies the `MultiPathAdapter`, which removes the Path from its
   set. *Channel close on a reroutable conn must NOT close the xgress.*
   The xgress's reliability state and the app-facing read/write
   adapters must outlive the gap; only when the conn is unrecoverable
   does the xgress actually close.
2. If at least one healthy Path remains (Phase B2), no recovery is
   needed — the selector picks a surviving Path. Otherwise the conn
   enters recovery.
3. SDK picks a new ingress via the recovery candidate selection below.
   It allocates a connId on that router's mux and pre-registers the
   (live) conn under it, then sends `TakeoverCircuit` (carrying the
   connId + token). See C2: pre-registration plus the new router's
   router-allocated IngressId close the reverse-direction early-arrival
   window.
4. On a success reply, the SDK calls `MultiPathAdapter.AddPath` with the
   `RouterChannelPath` for the new router. The mux sink was registered
   in step 3, so inbound payloads already have somewhere to land; the
   selector now starts routing outbound over the new path.
5. xgress retransmits its unacked window over the new path; duplicate
   acks from the terminator are absorbed; data flow resumes.

**Recovery candidate selection.** The SDK proactively maintains
connections to the service's ERs — `refreshServiceEdgeRouters` warms a
connection per advertised ER URL and measures latency so the fastest
can be chosen quickly. Each connected router's hello headers, including
the `RouterCapabilitySDKReroute` bit, are retained on the channel and
read synchronously via `edge.IsRouterCapable(ch.GetChannel().Headers(),
...)` — the same path `SupportsConnectV2()` uses today. So recovery
needs no speculative handshakes in the common case:

1. The SDK reads `serviceId` from the (signed, verified) reroute token
   it stashed at dial time (see Token design — SDKs may read documented
   claims). Candidate set = the SDK's currently-connected routers for
   that service, capability already known from stored hellos, ordered
   by measured latency.
2. Filter to `RouterCapabilitySDKReroute`-capable routers.
3. Attempt `TakeoverCircuit` against the lowest-latency capable router
   first, then the next.
4. Outcome drives the loop (exact reply codes are C5's contract):
   - **Success** → AddPath, resume.
   - **Retryable** (busy/conflict, owner-unreachable-from-this-router,
     route-install-failed) → next capable candidate.
   - **Fatal** (bad token, identity mismatch, circuit-not-found) →
     abort the whole loop; no other router can succeed.
5. No capable candidate currently reachable → **don't give up
   immediately.** Enter a retry-reconnect loop with backoff over the
   recovery window: keep attempting to (re)establish connections to
   ERs from the cached list, giving a flaky underlay a chance to
   heal. This is the SDK-side mirror of X3 underlay recovery — it
   matters most when the SDK has no alternative router (single-ER
   deployment, or an uplink flap that dropped everything, including
   the only ER connection). The xgress stays alive throughout (the
   C1 "channel close doesn't close the xgress" rule extends to "all
   connections gone, but still within the recovery window"). If the
   cached ER list itself looks stale, the SDK can re-fetch it from
   the controller when reachable — best-effort, since an underlay
   outage often takes the controller with it.
6. A capable ER (re)connects → attempt `TakeoverCircuit`. It may be
   the *same* router as the original ingress — X1 handles that fine
   (the chosen new ingress isn't required to differ from the old).
7. Recovery deadline elapses with no successful takeover → surface
   the conn as closed.

The recovery deadline is necessary: if the owning controller is
entirely unreachable, every capable candidate returns
owner-unreachable (retryable) and the loop would otherwise spin. It
is coordinated with the controller grace period (SDK window ≈ grace,
slightly shorter — see Grace period). The grace period naturally
yields `circuit-not-found` (fatal) once it expires *if* at least one
candidate reaches the owner; when none can, only the SDK's deadline
stops it. Retry backoff keeps the SDK from hammering a flaky underlay
while it waits.

So the SDK rides out the loss of even its only ER connection,
giving that connection the full recovery window to come back, before
declaring the circuit lost — symmetric with the controller holding
the circuit in Limbo for the same window.

**Fall back = surface closed, app re-dials.** On any terminal outcome
the SDK surfaces the conn as a normal connection-closed error. The SDK
does NOT auto-create a fresh circuit under the same conn — that would
lose the xgress reliability state and the app's continuous-stream
expectation. A fresh dial is the application's call, identical to a
non-reroutable conn dying today.

**Data plane impact: none.** The forwarder, smart-reroute machinery, and
route-message protocol all work as-is. Same `CreateRouteMessages(...,
SmartRerouteAttempt, ...)` path, same per-router `forwardTable`
installation. The only data plane-adjacent observable is that the
recovery period after a swap will see the SDK's send buffer flush its
unacked window onto the new path in a burst, scheduled by the existing
retransmit timer.

**Visibility:** `CircuitEvent(event.CircuitUpdated, ...)` fires at
each state transition relevant to reroute (Limbo entry, takeover
success, grace expiry). New `trigger` field distinguishes
`smart` / `forwarding-fault` / `sdk-takeover` / `grace-expired`.
For Limbo entry specifically, include the entry cause so an operator
can tell *why* a circuit went into Limbo during an incident:
`ingress-fault` (E1, SDK lost its router), `forward-fault-reroute-failed`
(E3, transit-repair exhausted), or `link-reroute-failed` (E2, a
path router went offline). Limbo duration is observable via
successive CircuitUpdated event timestamps; not exposed as a
dedicated metric in Phase A.

Per-router byte counters reflect physical bytes through each
router. During a reroute the SDK's xgress retransmits its unacked
window over the new path; the new ingress router meters those
bytes as fresh outbound (the terminator side has already received
them once via the now-dead path and dedups at
`LinkReceiveBuffer`). Aggregate fabric-byte counters across the
network are inflated for the duration of the recovery window —
acceptable behavior under the stated observability scope but
worth noting for operators interpreting traffic metrics during
incidents.

**Effort:** weeks, not quarters. Controller: ~one new handler, modest
changes to `network.fault()` to respect the `Reroutable` flag, a
`takeoverCircuit()` method that's mostly a wrapper around
`rerouteCircuit()` with a different first node. Router: one new
message type, plumbed to the controller via existing ctrl channel.
SDK: token storage, recovery loop, `MultiPathAdapter.AddPath` /
`RemovePath` wiring.

**What this buys.** Survives ingress router loss, ingress network
change, SDK roaming between networks. Mobile WiFi→cellular. Edge router
maintenance restarts (operator drains the old router; SDK rolls over
gracefully).

**What this doesn't buy.** Doesn't help with mid-circuit performance
degradation that isn't catastrophic. Recovery time = detection latency +
new-router-dial RTT + takeover RTT + retransmit window flush. Probably
1-5 seconds end-to-end. Fine for survival, too slow for "seamless."

### Phase B1 — Terminator-side reroute

The mechanism is symmetric with phase A: when the *terminator's* router
dies, the terminator SDK reattaches the circuit's egress to a different
router using the same token primitive. The controller's splice operates
on the egress side rather than the ingress side; in-flight data recovers
via the same end-to-end retransmits in the opposite direction.
Everything from phase A — `Reroutable` flag, `Limbo` state, token
shape, `TakeoverCircuit` wire message, SDK recovery loop — applies
unchanged with "ingress" replaced by "egress".

**Token side claim.** B1 mints tokens with `side: Egress` (the
endpoint-scoping claim pre-baked in phase A's token design). The
controller's takeover handler dispatches on `token.side`: `Ingress`
goes to the ingress-replacement splice (phase A), `Egress` goes to
the egress-replacement splice (B1). Same handler, same CAS guard,
same authz check; the `side` claim picks which endpoint is being
replaced. Same-identity self-host becomes unambiguous (each end has
its own side-scoped token).

The new piece is **signaling**: how does the controller learn that a
given terminator wants its circuits reroutable? At least three viable
options:

- **Bind-time flag** on the bind message, meaning "all circuits landing
  here should be reroutable." Uniform per-terminator policy, one place
  to set it, controller knows at terminator-registration time so tokens
  can be minted at circuit creation. Simplest.
- **Per-circuit opt-in in the terminator's dial response.** The
  terminator decides per-incoming-dial whether to participate. More
  flexibility, but the controller has to learn the answer after the
  circuit already exists, which means tokens get issued retroactively
  or on demand.
- **Per-circuit opt-in via peer data on the dial.** Initiator signals
  "I want reroutable on both ends if available", terminator's peer-data
  response confirms support. Symmetric with how some other capabilities
  ride peer data today.

Tokens for reroutable terminator circuits get delivered to the
terminator SDK at circuit-establishment time, via the same "new child
connection" arrival that today brings the circuit id to
`hosting_conn.newChildConnection`. New header on that message.

**Why split from phase A.** Bundling terminator-side into phase A
delays everything until a signaling option is chosen and the related
bind/listen-path plan converges. Splitting lets the dialer-side
mechanism ship and burn in while the terminator-side signaling
question is decided. The wire protocol and token semantics established
in phase A apply directly; phase B1 is signaling plus the SDK-side
terminator-recovery loop, not a new mechanism.

**Effort.** Smaller than phase A. New: bind-side message handling
(whichever signaling option lands), controller bookkeeping for
terminator-reroutable state, SDK-side terminator-circuit recovery
loop. Reuses A's controller takeover handler, A's `Reroutable` /
`Limbo` machinery, A's token shape.

**Coordination.** A separate plan for the bind/listen path itself is
in flight elsewhere; this phase plugs into whichever signaling path
that plan produces.

### Phase B2 — Pre-emptive multi-path

Once A ships, extending it to proactive multi-path is mostly additive.

**Approach:** SDK doesn't wait for the primary router to fail. It
opens a *second* router channel after a successful primary dial,
and sends `TakeoverCircuit` with an `AdditionalPath` flag — meaning
"add this path, don't replace the existing one." Controller computes a
*second* disjoint path from the second ingress to the terminator,
installs it as a parallel path with its own `IngressId`. SDK's
`MultiPathAdapter` now has two `RouterChannelPath`s active. The
`PathSelector` becomes meaningful: hot-standby, lowest-RTT, or
load-distribute.

**Iteration semantics for `AdditionalPath` (decide when B2 is
promoted).** Phase A's `iteration` counter models a single current
path generation — a replacement takeover advances it and invalidates
the prior token. `AdditionalPath` doesn't fit that model cleanly:
attaching a second concurrent path isn't a replacement, so it's not
obvious the iteration should advance (if it does, the still-valid
first-path token goes stale; if it doesn't, two outstanding tokens
share one iteration). Don't inherit Phase A's replacement-only
semantics by default — explicitly choose, e.g. per-path iteration
counters, or an iteration that only advances on replacement and a
separate path-id for additional paths. Flagged here so it isn't
silently overfit to replacement when B2 is built.

**New pieces:**

- Controller now manages N paths per circuit instead of one. Path-level
  identity (currently just an `IngressId` per path) becomes a
  first-class concept. Path-level fault handling: if path A's link
  fails, path A is invalidated independently; the circuit remains alive
  on path B. Smart reroute applies per-path.
- Path planning: with N paths required, what's the constraint?
  Disjoint transit routers? Latency-balanced? Operator-configurable?
  This is genuine controller-side algorithm work, not just wiring.
  Default constraint is probably node-disjoint where feasible — the
  whole point of having two paths is not sharing a failure mode.
- Per-path metrics on the SDK side. `MultiPathAdapter` needs to tag
  each in-flight payload with its send path so ACKs can attribute RTT
  and retransmits to the originating path, not the path that happened
  to carry the recovery. This matches the M1 note from
  `doc/design/p2p.md` about "path-tagged in-flight tracking" — same
  work, different motivation. Metric tags use `(Type, ID)` from the
  Path interface (see Foundation): for two router-channel paths the
  IDs are the two router ids, giving each path a distinct metric
  series.
- The xgress `LinkSendBuffer` is unaware of paths today. Either it grows
  per-path window state (proper but invasive) or paths share one
  window and the adapter does soft per-path accounting on top
  (approximate but simple). The QUIC multipath spec keeps per-path
  congestion control; we likely want the same eventually, but the
  approximate version is shippable first.

**What this buys over A.** No detection lag — when one path's traffic
stops getting ACKs, traffic shifts to the other without waiting for the
edge channel to fully close. True load distribution if the selector
chooses to use both paths concurrently. Substrate for the "circuit
path alongside direct DTLS path" pattern from the p2p design (same
adapter machinery; different `Path` implementations).

**Effort over A.** Probably +1-2 months. Most of it is controller-side
path management and the per-path metrics work.

### Phase B3 — Cross-controller takeover

Lifts the Phase A constraint that the owning controller must be
reachable. Lets any controller in the cluster validate a takeover
token *and* execute the splice — required for SDK reroute to survive a
combined router + owning-controller failure.

The core protocol addition is an explicit ownership-transfer mechanism
spanning controllers and routers. The current code has none: routers
silently merge routes from any controller into the existing forward
table, the `ctrlId` is set on table creation and never updated, faults
and scanner confirmations go to the original `ctrlId` forever. Doing
cross-controller takeover on top of that produces split-brain when the
original owner returns. Phase B3 fixes this.

**Ownership state on the wire and on routers.**

- Add `(ownerId, iteration)` to each circuit. The `iteration`
  counter is the same one phase A introduced for token freshness
  (per-circuit monotonic uint64); B3 reuses it as the
  ownership-arbitration counter so there's a single per-circuit
  monotonic value, two uses.
- Forward tables store `(ownerId, iteration)` alongside the
  existing `ctrlId` (or replace `ctrlId` outright).
- Route, unroute, and fault messages carry the route's
  `(ownerId, iteration)`. Routers reject route/unroute messages
  whose iteration is older than the stored iteration.
- Takeover route messages bump the iteration and set `ownerId` to
  the takeover controller; routers update their forward-table
  state on receipt.
- Scanner `CircuitConfirmation` goes to the current `ownerId`, not
  the historical one.

**Returning-controller reconciliation.**

- When a previously-owning controller comes back (from restart or
  partition heal) and tries to send a route, unroute, fault-driven
  reroute, or smart-reroute message for a circuit it no longer owns,
  the router rejects with `OwnershipChanged { newOwnerId, newIteration }`.
- The returning controller responds by removing the circuit from its
  in-memory state. (It can't usefully act on the circuit anymore;
  someone else owns it.)
- For circuits the returning controller *does* still own (the common
  case after restart), nothing changes.

**Capability gating.** New behavior is gated by a `RouterCapabilityB3`
bit (name TBD) on the router hello. Mixed-version networks: a
non-capable router treats route messages as today (merges silently);
takeover targeting a circuit whose path includes non-capable routers
falls back to Phase A semantics (requires owner reachable for those
specific routers).

**Phase B3 is not on the critical path** for SDK rerouting capability —
Phase A handles the common case. Build it when the operational evidence
shows owner-outage scenarios produce enough reroute failures to justify
the protocol work.

**Effort.** Larger than B1 or B2. Touches the route / unroute / fault
wire schema, the forwarder's per-circuit ownership state, the
controller's circuit lifecycle (handle `OwnershipChanged` replies),
and the scanner's confirmation target. Mostly contained within the
existing routing protocol — does not require the wire breaks of
phase C.

### Phase B4 — Router/controller connectivity resilience

Principle: **a circuit should not be torn down because of transient
controller↔router connectivity loss.** The data plane — router↔router
links, forwarder state, the SDK and terminator xgress endpoints —
survives a controller losing its management channel to a router; only
the controller's ability to *manage* that router is temporarily lost
(see `[[feedback_circuit_ownership]]`: the data plane survives even
owning-controller loss). Today's reaction over-reacts.

Today: when the controller↔router channel closes, `DisconnectRouter`
immediately removes the router's links and reroutes-or-tears-down
every circuit using them (this is the E2 trigger in Phase A). A flaky
controller↔router channel thus triggers a reroute storm — and on a
single-controller deployment can tear down circuits whose data plane
was fine the whole time.

B4 adds a **router-presence grace** (debounce): on controller↔router
channel close, don't immediately declare the router gone. Hold a
presence-grace window. If the channel re-establishes within it, the
router's links and circuits are left untouched — no reroute, no
teardown, no per-circuit Limbo cascade. Only if the router stays gone
past the grace does `DisconnectRouter` run as today. B4 effectively
sits *in front of* Phase A's E2: with B4, a transient router
disconnect is absorbed by presence grace and never cascades into the
per-circuit reroute/Limbo path; only genuine, sustained router loss
does.

This is a different mechanism than circuit Limbo (router-presence
debouncing, not per-circuit hold), which is why it's a discrete phase
rather than folded into A. Open design questions that warrant the
separate phase:

- **HA/clustering presence semantics.** In a multi-controller
  cluster, a router that loses its channel to controller A may still
  be reachable via controller B. "Presence" should be a cluster-wide
  fact — a router isn't gone until no controller can reach it.
  Overlaps with B3's ownership machinery.
- **Link lifecycle.** Links must not be removed on transient router
  disconnect; the xlink redial/iteration machinery already handles
  reconnection, but the controller's link-removal-on-DisconnectRouter
  has to defer until presence grace expires.
- **Interaction with circuit Limbo.** Presence grace should resolve
  first; only sustained router loss cascades into the per-circuit
  Limbo/teardown logic. Avoid double-counting the grace (router
  presence grace + circuit Limbo grace stacking).
- **Data plane during the grace.** Forwarder state and links persist;
  router↔router traffic keeps flowing (the controller being out of
  contact doesn't stop forwarding). The grace purely defers the
  controller's *management* reaction.

**Effort.** Its own phase: router-presence is a controller-model
concept touching link lifecycle, HA presence semantics, and the
`DisconnectRouter` cascade — enough surface area to warrant separate
design rather than riding Phase A.

### Phase C — Long-lived paths separated from circuits

The strategic destination. Genuinely changes the routing model.

**The split:**

- A **Path** becomes a controller-managed, long-lived resource:
  `{pathId, [routers], [links], srcRouter, dstRouter}`. Reference-
  counted by circuits, idle-timed out, can be created proactively
  before any circuit needs it.
- A **Circuit** becomes lightweight: `{circuitId, pathId, terminator
  binding, ingressXgressId, egressXgressId}`. Multiple circuits can
  share a path.
- Routers can create circuits *locally* when a path already exists to
  the destination router and the SDK's identity is authorized for the
  service (via RDM, same authorization that already powers V2
  sessionless dial). No controller round-trip on the hot path of
  circuit creation in steady state.

**What this enables:**

- **Sub-millisecond circuit creation** in the common case (existing
  path, locally authorized).
- **Multi-path is structural**: a circuit can reference an ordered list
  of paths trivially.
- **SDK reroute becomes a path swap on the circuit record**: existing
  path X dying, switch the circuit to existing path Y. The controller's
  involvement is "update the circuit's pathId"; no new routes get
  installed if Y was already provisioned.
- **Smart reroute moves up a level**: it operates on paths, not
  circuits. Circuits ride along whichever path their circuit-record
  points at.

**Costs honestly:**

- **Wire protocol overhaul.** Route messages restructure; path
  lifecycle messages (`CreatePath`, `RetainPath`, `ReleasePath`) are
  new. A `routing-v2` capability gate exactly as proposed in
  `routing-v2.md` — both moves want the same gate.
- **Forwarder data model change.** Today `circuitId -> forwardTable
  -> (srcAddr -> dstAddr)`. New model: `pathId -> pathForwards` for
  the long-haul shared state, plus `circuitId -> (pathId,
  ingressXgress, egressXgress)` for the endpoint binding. Two-level
  lookup. Possibly worth doing the byte-encoding cleanup from
  `routing-v2.md` while you're in there.
- **Authorization delegation.** Routers creating circuits locally
  need to validate "this identity may use this service on this path"
  without a controller round-trip. RDM already provides the service-
  level authorization; path-level authorization is new (does every
  identity get to use every path?). Probably paths are tagged with
  authorization scopes when created.
- **Path lifecycle.** When do paths get torn down? Reference-counting
  on circuits + idle timeout is the obvious answer. Path-level fault
  handling. Path-level metrics independent of any circuit. Different
  operational model than today.
- **Visibility migration.** Today's circuit events report path nodes
  inline. Under split, circuit events report `pathId`; path events
  separately report path topology and bytes-flowing. Operator tooling
  needs to follow the join.

**Effort:** multi-quarter, possibly multi-year depending on scope.
Treat as the destination, not the next step. The point of building A
and B with this destination in mind is to avoid wire-protocol or token
decisions that would have to be reworked when C lands.

## Token design (QUIC-informed)

QUIC's connection migration story is the closest existing prior art.
Several specific design decisions from RFC 9000 sections 8, 9, and 21
inform the reroute token design here.

### What the token is

A **controller-signed token (JWT-like)**, issued by the owning
controller, verifiable by routers and any controller via the cluster's
published signing keys (JWKS) — the same mechanism routers already use
to verify API-session JWTs. Signed, not encrypted: the token holds no
secrets (see below), so we need integrity (unforgeable), not
confidentiality.

Claims:

- `circuitId` — the circuit this token authorizes action on.
- `identityId` — the identity that owns the circuit. The new router
  authenticates the SDK's edge-channel identity and compares it to
  this claim directly (it can verify and read the token). Token theft
  from a different identity is not exploitable without also
  compromising the original identity's cert.
- `serviceId` — the circuit's service. Lets the router do a *trusted*
  dial-access fail-fast (`checkAccess(serviceId, DialPolicy)`) against
  current policy — the service is controller-signed, so the SDK can't
  substitute a service it happens to have access to.
- `iteration` — monotonic per-circuit counter (uint64). Naming matches
  the existing convention used on links (`link.Iteration`,
  `Fault.Iteration`, link-fault stale-iteration rejection). The token
  is valid while `token.iteration == circuit.iteration`. Successful
  takeover increments `circuit.iteration` and mints a fresh token at
  the new value; the SDK swaps the stored token from the reply. No
  expiry field — the token is alive while its iteration matches and
  the circuit exists; both bounded by natural events (advancement on
  takeover, removal on teardown). Multiple retries of the same token
  against the same iteration are idempotent: a mid-flight failure
  does NOT advance the iteration, so the SDK can retry the same
  token against a different new router without replay rejection.
- `ownerControllerId` — the controller that owns this circuit. Lets
  the new router dispatch `TakeoverCircuit` directly to the owning
  controller's ctrl channel. Now a signed claim, so it's also
  tamper-evident (tampering breaks the signature).
- `purpose` — fixed string `"ziti-sdk-reroute"`. The reroute token
  shares JWT signing infrastructure with other controller-issued
  artifacts (API session tokens, identity tokens). Every handler that
  accepts a reroute token asserts `purpose == "ziti-sdk-reroute"`
  before reading other claims — defense against token confusion where
  a token signed by the same key for a different purpose is fed into
  the wrong validator.
- `version` — token schema version (uint32, starts at 1). The
  connect-v3-style upgrade path uses this as the explicit gate
  alongside the router-capability bit: an incompatible format change
  bumps `version`, capability negotiation chooses between old and new
  validators, mixed deployments coexist during rollout. Phase A ships
  `version: 1`.
- `side` — which endpoint the token authorizes the bearer to take
  over. Enum with pinned numbering, same fail-safe pattern as
  `FaultReason`:

  ```
  enum TokenSide {
    SideUnspecified = 0;  // reject — proto3 zero / old peer, never authorize
    Ingress         = 1;  // dialer-side reroute (Phase A)
    Egress          = 2;  // terminator-side reroute (Phase B1)
  }
  ```

  Phase A only ever mints `Ingress` tokens; the takeover handler
  asserts `token.side == Ingress`. B1 will mint `Egress` tokens and
  dispatch the controller's splice to the egress side based on the
  claim. Bakes the endpoint scope in now so B1 doesn't have to
  extend the token shape or overload `identityId` semantics later.
  Same-identity self-host (one identity that both dials and hosts a
  service) becomes unambiguous: each end gets a token with its own
  `side`, so a terminator's token can't be accepted by mistake for
  a dialer-side splice or vice versa.

**No secrets in the token.** Every claim is something the SDK already
knows about itself (its circuit, identity, service) or is non-sensitive
(iteration, owner). Signing alone suffices; there's nothing to hide.

**SDKs may read documented claims; format-breaking changes go through
a coordinated protocol upgrade.** The token is signed (unforgeable
without the controller key) but its claims are readable by anyone
holding the verification key — including the SDK. Rather than the SDK
storing token-resident fields redundantly on the conn, **the SDK is
permitted to read documented claims it needs.** Today the SDK reads
`serviceId` from the token for recovery candidate selection (closes
C4). Any other claim is opaque from the SDK's perspective until and
unless it has a documented reason to read it.

The token format is a coordinated contract among controller, router,
and SDK. Additive evolution (new claims that the SDK doesn't read) is
free — no SDK release needed. Format-breaking changes (different
envelope, removing/renaming a claim the SDK reads, changing the
signing scheme) go through the same evolution path the rest of the
protocol uses: a capability bit on the router hello, a connect-v3-
style negotiation, and a new token format alongside the old. SDKs
gate on the capability they detect. This is strictly cheaper than
the alternative "SDK stores everything redundantly so the token can
evolve unilaterally" — which costs every conn a duplicated copy of
token state and still needs the negotiation when something
incompatible changes anyway.

The SDK verifies the token signature on receipt (same JWKS the router
uses) so it never reads claims from an unverifiable blob.

This corresponds to QUIC's "address validation token" pattern (RFC 9000
§8.1) — controller-signed proof that "this identity holds rights to
this circuit," presented at the moment of migration to prove the
peer-identity continuity QUIC gets via CIDs.

### Path validation analogue

QUIC requires PATH_CHALLENGE / PATH_RESPONSE before committing to a new
path. The challenge prevents an off-path attacker from claiming
migration to redirect traffic to a victim address, and prevents
amplification attacks.

We get most of QUIC's path-validation benefit for free because the
underlying transport is different: the new router-SDK channel is an
authenticated TLS connection (mTLS, identity-verified). The SDK is
*demonstrably* at the new router by virtue of having completed the
handshake. There's no off-path migration scenario equivalent to QUIC's
UDP-spoofing threat: an attacker can't forge an authenticated edge
channel without the SDK's private key.

What we still need to defend against:

- **Token theft + replay** by an attacker on the same identity.
  Handled by the `iteration` field: a successful takeover advances
  `circuit.iteration`, invalidating the captured token. Resembles
  QUIC's stateless retry token (§8.1.4) but uses a monotonic
  counter instead of a single-use nonce so legitimate retries are
  idempotent.
- **Token theft + reuse across identities** is structurally prevented
  by the `identityId` field plus the new router's TLS authentication
  check. The new router MUST verify the authenticated identity matches
  the token's bound identity before forwarding the takeover request.
- **Malicious SDK floods takeover requests** to multiple controllers.
  Rate-limit per identity; cheap signature verification (and the
  router's local checks) reject forged tokens fast. Successful
  takeovers advance the iteration; replayed-but-validly-signed tokens
  are rejected as stale.
- **Controller-side amplification:** the controller's takeover reply
  could be large (state_connected with stickiness token, peer data,
  etc). Cap reply size; ensure controller doesn't send more than ~3x
  the request size to an unvalidated source — same anti-amplification
  posture as QUIC §8.1.

### What we deliberately don't take from QUIC

- **Multiple connection IDs per connection (RFC 9000 §5.1).** QUIC
  issues N CIDs so the client can use different CIDs on different
  paths, partly for unlinkability. Our circuit ID is already
  controller-issued; we don't have a privacy story that needs separate
  per-path IDs in phase A or B1. Phase B2 and C may want this;
  revisit then.
- **Preferred-address transport parameter (§9.6).** QUIC servers
  advertise an alternate address for clients to migrate to. Our
  equivalent — "consider these other routers if your current one
  fails" — is already covered by the cached service-ER list maintained
  by `refreshServiceEdgeRouters`. No separate mechanism needed.
- **Per-path packet number spaces.** QUIC multipath uses path-scoped
  packet numbers so AEAD nonces stay unique across paths. Our xgress
  sequence numbers are global per-circuit, and our crypto layer
  (secretstream) is per-circuit not per-path. The sequence-number
  globality is actually what makes phase A's "retransmit over the new
  path" work without protocol changes; we shouldn't lose that without
  reason.

### What phase B2/C may want from QUIC multipath

Once we have multiple paths active simultaneously, several things in
draft-ietf-quic-multipath become directly relevant:

- **Explicit path IDs**, distinct from connection IDs. Our equivalent
  is the per-path `IngressId`; we should make it first-class earlier
  rather than later.
- **PATH_ABANDON-equivalent** for explicit path teardown. Today we
  use route messages with unroute semantics; that translates cleanly
  to a path-level concept.
- **Per-path congestion control**. The xgress send buffer needs
  per-path window state once paths can have meaningfully different
  characteristics. Approximate version in phase B2; proper version in
  phase C.
- **3×PTO retention after abandon.** When a path is closed, in-flight
  packets may still arrive on it; the receiver should keep state long
  enough to ack them rather than discarding. Our equivalent: when the
  controller tears down a path, the affected routers' forwarders
  should keep the forward table entries alive for some short window
  (or just let the normal idle timer handle it, which is roughly the
  same effect).

## Threat model

Threats specific to the reroute mechanism, with mitigations:

| Threat | Mitigation |
| --- | --- |
| Attacker steals token from network | Token is controller-signed; unforgeable and tamper-evident without the controller signing key. Readable but useless to forge with. Edge channels are mTLS. Reuse still requires authenticating as the bound identity and passing current authz. |
| Attacker replays a previously-used token | Iteration freshness: a successful takeover advances `circuit.iteration`, so a captured token (at the old iteration) is rejected as stale. No expiry window or nonce-dedup set needed. |
| Attacker presents valid token to claim victim's circuit | Token bound to `identityId`; the new router verifies the token signature and matches `token.identityId` against the authenticated edge identity (it holds both); the controller re-checks defense-in-depth. |
| Attacker on victim's identity steals token from compromised SDK process | Out of scope — compromised identity is fully compromised. Same posture as today. |
| Original-dial authorization revoked between dial and revival (access policy, session, posture) | Two layers, closing revocation regardless of timing. (1) Revoked while live: the router's `cleanupXgressCircuit` emits `IngressFault{Reason: AccessLoss}`; controller tears down rather than entering Limbo (only `ChannelClosed` is Limbo-eligible; absent the field, old router, the conservative default is teardown). (2) Revoked while already in Limbo (no live router to emit AccessLoss): the takeover handler under the CAS guard runs the *same* authz check `CreateCircuitV3` does at dial time — identity + API session + service policy + posture/RDM state — against current state (X1 step 4a). The new router also fail-fasts via `checkAccess` (X1 step 0). Token possession alone is insufficient — current dial-time authorization is required to revive, with full session/posture parity. |
| Attacker floods controller with takeover attempts | Rate-limit per identity; the router's local signature verification + identity match + access fail-fast reject forged or unauthorized attempts before they reach the controller. |
| Owning controller is down during reroute | Phase A requires the owning controller reachable from the new router; takeover fails otherwise and the SDK falls back to a fresh dial. Cluster-wide takeover during owner outage is deferred to phase B3 and needs a separate ownership-transfer protocol that doesn't exist today. (See `[[feedback_circuit_ownership]]` — circuit data plane survives owning-controller loss, but reroute capability does not in Phase A.) |
| Old-path Unroute arriving after takeover damages new-path state | Phase A's single-controller-writer constraint (the owning controller is the only entity mutating a circuit's path) plus `unrouteRemovedPathNodes` (only sends Unroutes to old-path routers absent from the new path) plus Limbo rule D1 (no path mutations during Limbo) plus the `Rerouting` CAS guard (serializes path mutations). Phase A intentionally does NOT depend on per-IngressId or path-scoped unroute idempotency at the wire level — the existing circuit-scoped `ctrl_pb.Unroute` is sufficient because concurrent conflicting unroutes are prevented at the controller. |
| Compromised router lies about authenticated identity during takeover | Out of scope for Phase A. Routers are trusted infrastructure (controller-issued certs, administered alongside the controller); a compromised router can already tap data on its circuits via normal operation, and takeover doesn't materially expand that attack surface. SDK-signed takeover proof (identity-key signature over `{circuitId, iteration, timestamp}`, verified by the controller against its identity DB) is the obvious hardening if this threat needs to be addressed — see "Future hardening" below. |

### Trust boundary note

mTLS on the SDK↔router edge channel protects the channel against
off-path attackers, NOT against the router itself. In the takeover
flow the new router attests to the controller that the SDK
authenticated as `identityId` X; the controller takes that on
trust. A compromised router could lie about this attestation,
modify the takeover request before forwarding, or forward
genuine-but-stale takeover attempts. This is the same posture
OpenZiti already has for normal circuit operation: routers are
trusted infrastructure. Phase A doesn't claim to defend against
compromised routers; if that threat needs explicit defense, see
the future-hardening note below.

### Future hardening — SDK-signed takeover proof

If the compromised-router threat needs explicit defense rather
than relying on routers being trusted infrastructure, the SDK can
include a signature over `{circuitId, iteration, timestamp}` in
the `TakeoverCircuit` message, signed with its identity private
key. The controller verifies the signature against the identity's
public key from its identity database (which the controller
already maintains for TLS auth). Removes the router's role in
identity attestation entirely — even a compromised router can't
successfully forward a takeover for an identity whose private key
it doesn't have. Adds one asymmetric crypto operation per
takeover. Not a Phase A requirement.

## Sequencing notes

- **Token design is the load-bearing decision** for everything after
  phase A. Get the fields and semantics right once; the same token
  shape carries B1 (terminator-side reroute) by minting tokens with
  `side: Egress` and dispatching the controller's splice on the
  `side` claim, authorizes multi-path additions in B2
  (`AdditionalPath` flag in the takeover request), is the same token
  whose `ownerControllerId` field becomes a hint rather than a
  constraint in B3 (any controller may then act, not just the named
  owner), and authorizes path-attach in C (token grants use of paths
  in this scope). Token shape is stable across all phases — claim
  values evolve (`side`, multi-path scoping, owner role), the
  envelope doesn't.
- **Don't build long-lived paths first.** It's the right end state but
  the wire-protocol break and authorization-delegation work is bigger
  than A, B1, B2, and B3 combined. The cost of doing A as a tactical
  stop is small; the cost of doing C first is forfeiting reroute
  capability for the duration of the C effort.
- **B1, B2, B3, and B4 are independent of each other.** Each builds
  on A. Sequence is a question of which value lands first, not a
  dependency. B1 unblocks terminator-side survival, B2 unblocks
  performance and load distribution, B3 unblocks reroute during owner
  outages, B4 stops transient controller↔router connectivity loss
  from tearing down circuits whose data plane is fine.
- **Phase A should ship with no path-allocation changes.** A circuit
  still has one path at a time. The MultiPathAdapter has one path at
  a time. Token-driven reroute swaps that one path. Multi-path is
  phase B2's concern.
- **Per-path metrics in phase B2 unlock the p2p work too.** Same
  underlying need (path-tagged in-flight tracking) — coordinate with
  the M1 work in `doc/design/p2p.md` if both are in flight.

## Open questions

- **Migration during in-flight crypto rekey?** The secretstream
  rekey path isn't path-aware; reroute mid-rekey is probably fine
  because rekey is per-circuit not per-path, but worth verifying with
  an integration test.
- **Lost takeover success reply (accepted for Phase A).** If a
  takeover commits on the controller but the success reply is lost in
  transit, the SDK retries with its now-stale token, gets
  `TokenRejected` (iteration already advanced), and surfaces the conn
  closed. The committed-but-unused circuit is orphaned on the
  controller and reaped by idle/scanner cleanup; the SDK tears down
  its speculative registrations on close. No corruption, but a
  recoverable circuit is needlessly lost in this narrow window.
  Accepted for Phase A (outcome is a surfaced-closed conn + app
  re-dial, same as any unrecoverable reroute). Future tightening: on
  `TokenRejected`, the SDK queries the controller for the circuit's
  current iteration/state to discover a takeover it didn't know
  succeeded, and adopts it instead of closing — a reconciliation
  round trip deferred as not worth it for Phase A.
- **Self-dial on one router (pre-existing bug, low priority but
  worth fixing).** When the *same* SDK dials a service it hosts over
  the *same* edge channel, both the dialer-side and terminator-side
  `xgEdgeForwarder`s share one `edgeClientConn` and both
  `xgCircuits.Set(circuitId, self)` on that conn's circuit-id-keyed
  index — second write wins, orphaning the first. The fabric-side
  `forwarder` is unaffected (keyed by address), but the conn's
  inbound-from-SDK dispatch (`handleXgPayload` /
  `handleXgAcknowledgement` / `handleXgControl` / `handleXgClose`, all
  `xgCircuits.Get(circuitId)`) resolves both ends to the one surviving
  forwarder → mis-forwarding. This is independent of rerouting (a
  self-dial on one router already misbehaves today), which is why
  Phase A excludes same-SDK self-dials from reroutability rather than
  trying to recover them. The fix, if we take it: key `xgCircuits` by
  `(circuitId, originator)` or by `connId` instead of `circuitId`
  alone, so the two ends don't collide. Not high priority, but
  desirable — file as its own issue against the router edge listener.
- **Typed dial-rejection reasons (reuse the FaultReason/disposition
  model for the dial path).** Today `DialContextWithOptions`
  (`ziti.go`) handles any dial failure with one blanket move:
  invalidate the cached per-service state (ER list + session) and
  retry `dialService` exactly once. It works but is coarse — every
  rejection is treated identically because the router's `state_closed`
  reply carries only an opaque reason *string* (the SDK does
  `errors.Errorf("dial failed: %v", string(replyMsg.Body))`), so the
  SDK can't tell "access revoked" (retry is pointless — should refresh
  services and surface a clear authz error) from "stale SDK state"
  (refresh + retry is right) from "this ER is bad" (route around it on
  retry — note `getEdgeRouterConn` prefers already-*connected* routers
  by latency, so a connected-but-rejecting ER can be re-selected, and
  cache invalidation alone doesn't avoid it). Doing this well wants a
  **typed dial-rejection reason** on the `state_closed` reply (a header
  enum, same shape as the takeover `TakeoverResultCodeHeader` table
  above) plus the same retryable/fatal disposition split: fatal
  reasons (access revoked) skip the retry, surface cleanly, and trigger
  a service refresh; retryable reasons refresh + retry, excluding the
  ER that just rejected. This is the dial-time analogue of the
  `FaultReason` enum and the retryable/fatal mapping this design
  already defines for reroute/takeover — they should share one
  disposition model rather than grow a parallel one, which is why it's
  captured here rather than bolted on as a one-off string check. It's a
  cross-repo change (router stamps the reason, SDK classifies) and
  exceeds connect-v2 scope; sequence it with whichever of this design's
  phases lands the `FaultReason` wire work.

## Source of truth

This design targets the **connect-v2 as-built state** described in
`connectv2.md` (and the `connect-v2` branch). Some router internals
referenced here — `cleanupXgressCircuit`, the `IngressFault`/
`EgressFault` subjects, `xgEdgeForwarder`, the `xgCircuits` index —
only exist in their final form on that branch. If a source snapshot
ever disagrees with `connectv2.md` (e.g. still showing `route_circuit`,
`ConnectRequestIdHeader`, or router-pre-generated circuit ids, which
connect-v2 removed), trust `connectv2.md` + the connect-v2 branch; the
snapshot was taken from a divergent checkout.

## References

- `connectv2.md` — connect-v2 as built; defines the capability
  negotiation pattern and the `state_connected` reply structure that
  this design extends.
- `routing-v2.md` — future-work notes on byte-encoded IDs and
  address-folding; phase C of this design wants the same wire-protocol
  break, should be bundled if both happen.
- `ziti/doc/design/p2p.md` — M1 ("MultiPathAdapter alongside
  XgAdapter") is the structural foundation reused here. Phase B2's
  per-path metrics work overlaps with p2p M1's "path-tagged in-flight
  tracking."
- RFC 9000 §§8, 9, 21 — QUIC connection migration: token design, path
  validation, security considerations.
- draft-ietf-quic-multipath — explicit path identifiers, PATH_ABANDON,
  per-path congestion control; informs phase B2/C design.
