# Load Balancer Support for Router Links

## Motivation

Similar to the [anycast design](anycast.md), there are situations where we have a pool of public
routers providing transit or SDK edge access for routers in private networks, and we'd like to
front that pool with a single stable address. A load balancer is the most familiar way to do
this, customers already know how to operate one, and it sidesteps the complications of anycast
routing (BGP cooperation, mid-connection redirects, geographic anycast pools).

The operational pitch is the same as anycast: customers get a single IP or DNS name they hand
out to private routers, and they can scale the backend router pool up and down without anyone
updating firewall allow-lists. Unlike anycast, the LB approach works with stock cloud
infrastructure (AWS NLB, GCP TCP LB, HAProxy in TCP mode) and gives clean backend health checking
and graceful drain for free.

The trade-off is cost. All link traffic flows through the LB, which means per-connection,
per-byte, or LCU charges depending on the provider. For high-throughput router pools this can be
significant. Some customers have explicitly told us they're willing to accept this cost for the
operational simplicity of a stable single address.

## LB Assumptions

1. **L4 TCP passthrough only.** The LB must not terminate TLS. Router identity verification
   depends on the dialer seeing the router's certificate, not the LB's. This rules out L7 load
   balancers (AWS ALB, GCP HTTPS LB) and means customers must use NLB-style products configured
   for TCP passthrough.

2. **Round-robin (or similar) backend selection.** We assume the LB distributes new connections
   across backends in a way that, given enough dials, will reach every backend. Most LBs default
   to round-robin or least-connections, both of which satisfy this. Sticky-session configurations
   would defeat the design and should not be used.

3. **Backend removal is handled gracefully, often promptly.** When an LB removes a backend
   (health check failure, scale-down), what happens to existing connections depends on the LB
   and its configuration: some send a prompt TCP RST, some drain the backend while letting
   established connections finish, and some simply stop sending new connections and let the old
   ones hit the idle timeout. In the best case teardown is immediate and diagnostics are clean;
   in the worst case the existing 10s link heartbeat detects the dead link and tears it down.
   Either way the private router reconnects through the LB. Operators who want fast failover
   should prefer an LB and configuration that closes drained connections rather than waiting for
   idle timeout.

4. **LB has its own idle timeout.** Most managed LBs idle-close connections after some period
   (AWS NLB defaults to 350s, GCP TCP LB to 600s, ALB to 60s). The existing 10s link heartbeat
   keeps connections alive well under any of these, but it's worth being aware: a configuration
   error that disables heartbeats would manifest as links dying at exactly the LB's idle interval.

5. **Reconnects may land on different backends.** Same as anycast, this is fine and desirable.
   Round-robin makes it more predictable, the next dial likely reaches the next backend in
   rotation.

## Design

The design centers on three ideas. First, multiple public routers can advertise the same LB
address as their link listener, and the controller distributes those advertisements to private
routers through the existing `PeerStateChange` mechanism. Second, because the dialer can't know
in advance which backend a given dial will reach, identity verification happens *after* the TLS
handshake, by extracting the router ID from the presented certificate and cross-checking it
against fingerprints the dialer already has. Third, because round-robin lets us reach every
backend if we keep trying, the dialer can deliberately fan out multiple dials through the LB
address to build out a link's underlay or to establish links to multiple backends.

This is similar in spirit to anycast-fronted router pools (see [anycast.md](anycast.md) for that
design); the verification machinery is largely the same, but the dialing strategy and failure
semantics differ enough to warrant a distinct flag and dialer path.

### Advertising LB Addresses

A public router marks a link listener as load-balanced with a dedicated **`Listener` flag**, not
by embedding anything in the advertise address. The advertise address stays a clean
`tls:host:port`; a boolean field on the `Listener` message (carried in `PeerStateChange`)
indicates it's an LB listener. In the router's local config this can be expressed as a
`loadBalanced: true` field on the listener (or, if the `?lb=true` query-parameter syntax is
preferred for ergonomics, the router parses it out of its own config locally and sets the field;
the flag never travels in the wire address). See Address Representation below for why the flag is
a field rather than part of the address.

The listener flows through the existing `PeerStateChange` mechanism, which is how the controller
tells each router about the listeners advertised by its peers. Multiple public routers
advertising the same LB address is expected and is what tells the dialer to treat them as a pool
rather than as independent destinations.

Unlike a plain listener relay, LB listeners require the controller's cooperation, because an
LB-unaware router that received one would dial it as an ordinary direct link (no verification, no
fan-out, landing on a random backend). A two-way capability handshake gates this (see Capability
Handshake below): the controller only forwards LB listeners to routers that can dial them, and a
router only advertises an LB listener to a controller that can filter them.

The LB flag is distinct from the anycast flag used for anycast-fronted pools because the dialer
behavior differs (see below), even though the listener-side and verification mechanics are
similar.

### Multiple Listeners: Direct and LB

A public router typically advertises two listeners: a direct address and an LB address. For
example:

```
listeners:
  - binding: transport
    bind: tls:0.0.0.0:3022
    advertise: tls:pub-east-1.example.com:3022   # direct, reachable by peers
    groups:
      - public
  - binding: transport
    bind: tls:0.0.0.0:3023
    advertise: tls:lb.example.com:3023          # LB, reachable by private routers
    loadBalanced: true                          # marks this as an LB listener (sets the Listener flag)
    groups:
      - private
```

The direct address matters for mesh formation among the public routers themselves. If the only
advertised address were the LB, public routers trying to peer with each other would dial the LB
and round-robin would send them to themselves or to an arbitrary peer, neither of which is what
they want. The direct address lets them reach a specific peer.

Link groups are the recommended way to keep these separated: put the LB listener in a group
targeted at private routers, and the direct listener in a group for public routers that can
actually reach it. This avoids private routers wasting connect attempts on a direct address
they can't reach through the firewall, and avoids public routers going through the LB when
they have a direct path. Admins who want to avoid dial failures and the noise they generate
should lean on link groups rather than relying on dialer fallback. The upcoming
controller-managed router configuration will make defining and assigning these groups
considerably easier than today's per-router config.

**Dialer preference:** When a router sees both a direct and an LB path to the same backend B, the
dialer prefers direct, with LB as the fallback. The precise rule (so an unreachable direct address
can't starve B, and a reachable one doesn't cause steady-state double-dialing):

- **Only an *established* direct link to B suppresses LB fan-out for B.** Merely advertising a direct
  listener, or having a *pending*, *dialing*, or *backoff-failed* direct `linkState`, does **not**
  suppress LB. So if B's direct address is unreachable, LB stays eligible and B still gets linked.
- **LB fallback arms after one direct-first attempt.** B's direct dial gets the first attempt; if it
  establishes, LB never fans out to B (or closes a redundant LB link per the collision rules below);
  if that first attempt fails, LB fallback is armed for B. This keeps direct preferred, bounds
  fallback latency to a single direct attempt, and avoids racing both paths from the start.
- If a direct link to B later establishes, the LB link is closed (direct supersedes LB); if the
  direct link later drops, LB re-arms. See Direct-link collision.

This collision is meant to be **rare**: the primary control is **link groups** -- put the LB listener
in a group targeted at routers that must use it and the direct listener in a group for routers that
can actually reach it, so a given router sees *one* path to B, not both. The dialer-preference rule
above is the backstop for the cases where both are nonetheless visible (overlapping groups,
misconfiguration), not the intended steady state. Operators should lean on link groups to keep
direct and LB audiences disjoint.

### Address Representation

The LB flag is a field on the `Listener` message, and the advertise address stays a clean
`tls:host:port`. We deliberately do *not* embed the flag in the address (e.g.
`tls:host:port?lb=true`) for two reasons:

1. **The transport address parser rejects it.** `transport.ParseAddress` splits on host/port and
   runs `strconv.ParseUint` on the port; for `tls:host:3023?lb=true` the port becomes
   `3023?lb=true` and parsing fails. Supporting query parameters would mean changing the shared
   transport library (used by every component and the SDK) and stripping the parameter before
   every parse, which is broad and easy to get wrong.
2. **The address is used as an identity.** LB grouping keys on the LB address, and the existing
   "two routers accidentally share an advertise address" check compares addresses. Embedding a
   parameter would force every such site to decide whether the parameter is part of the address
   identity and strip it consistently. A separate field keeps the address a clean key.

A `Listener` field also propagates cleanly: the controller stores and rebroadcasts router listener
messages as-is (`SetLinkListeners` / `router.Listeners`), so an LB-capable controller relays the
field without special handling. We do *not* rely on an un-rebuilt controller transparently
relaying the field, because LB advertisement is capability-gated, an old controller never receives
an LB listener in the first place (see the capability handshake).

### Capability Handshake

LB listeners can't be handed to a router that doesn't understand them: an LB-unaware router would
treat one as an ordinary listener and dial the LB address as a plain direct link, with none of
the verification or fan-out machinery, landing on a random backend. So both ends of the
router-controller relationship gate on a capability. The two directions use **different existing
transports** (this asymmetry is easy to get wrong, so it's called out explicitly):

- **`ControllerLoadBalancedLinks`** (controller -> router): a new bit in the **`common/capabilities`
  bitmask** the controller advertises in its hello `CapabilitiesHeader` (alongside
  `ControllerCreateTerminatorV2`, etc.); routers already read this mask. The controller understands
  LB listeners and performs the per-recipient filtering below. A router advertises its LB listener
  to a controller *only if* that controller advertises this capability; otherwise it withholds it
  (and logs). Evaluated **per controller connection**, since a router may be connected to a mix of
  upgraded and older controllers.
- **`RouterLoadBalancedLinks`** (router -> controller): a new value in the **`ctrl_pb.RouterCapability`
  enum** (joining `LinkManagement`), sent in the router's `RouterMetadata` message via the
  `RouterMetadataHeader` -- **not** a `common/capabilities` bit (that bitmask is controller-sent).
  The controller records it at connect and includes an LB listener in the `PeerStateChange` it
  sends a router *only if* that router advertises this capability; otherwise it filters the LB
  listener out for that recipient.

The asymmetry mirrors how these capabilities already flow: `common/capabilities` carries
controller-advertised capabilities, while a router advertises its own through `RouterMetadata`.
Putting `RouterLoadBalancedLinks` in the controller bitmask would compile but never be read on the
router-advertised path, silently disabling the leak-prevention filter.

**Advertisement sequence (why the hello can't carry the LB listener).** A router sends its
listeners in its *initial connect hello*, but only learns a controller's capabilities from the
controller's hello *response*, so at hello-send time it doesn't yet know whether that controller
can filter LB listeners. Putting an LB listener in the hello would therefore risk handing it to an
old controller before the router knows to withhold it. So:

1. **The connect hello never carries LB listeners.** The router excludes LB listeners from the
   initial listener snapshot in the hello (it can't be filtered per-controller there, and the
   capability isn't known yet). If it withheld at least one LB listener, it also sets an
   **LB-listeners-pending** flag in the hello so the controller knows a post-handshake snapshot is
   coming (used for coalescing, below).
2. **LB listeners are advertised post-handshake via `UpdateLinkListeners`.** Once the router has
   seen a controller advertise `ControllerLoadBalancedLinks`, it publishes its listeners to that
   controller using the runtime listener-republish mechanism (`UpdateLinkListeners`, from the
   controller-managed-router-config work; see Work Items), with LB listeners included **only** for
   capable controllers.

This relies on two things from the controller-managed-router-config work, which already implements
the republish mechanism (`Router.publishLinkListeners` / the controller's `UpdateLinkListeners`
handler, which does a full `SetLinkListeners` replace and re-distributes to peers): (a) that
mechanism must exist (it does), and (b) `publishLinkListeners` must filter the published set **per
recipient controller** so LB listeners go only to capable controllers, rather than sending one set
to all. The hello-exclusion in step 1 is the only other delta. Note that the hello still carries
the router's **non-LB** listeners as it does today, only LB listeners are held back, so a router is
never spuriously seen as having no listeners and its direct links are never disrupted.

**Coalescing the connect snapshot (so reconnects don't flap LB links).** The hello and the
follow-up `UpdateLinkListeners` are each a full `SetLinkListeners` replace, so naively the
controller would redistribute the hello snapshot (LB listener absent) and then the
`UpdateLinkListeners` snapshot (LB listener present). On a control-channel *reconnect* where the
router already has established LB links, that intermediate LB-absent snapshot would make peers
orphan-reap those links (`ApplyListenerChanges` closes links whose key no longer aligns), then
re-form them, a flap on every reconnect. To avoid it, the controller **holds the initial
post-connect peer notification** until the `UpdateLinkListeners` arrives or a configurable
interval elapses, then redistributes the combined set once. Two refinements keep this cheap:

1. **Notify on the earlier of `UpdateLinkListeners` received or the interval elapsing.** The
   interval is a safety upper-bound, not a fixed delay, since `UpdateLinkListeners` is normally
   sent within sub-second of connect, the coalesced notification almost always fires promptly.
2. **Arm the hold only when an LB listener was actually withheld.** When a router excludes an LB
   listener from its hello (because it has one to advertise post-capability), it sets an explicit
   **LB-listeners-pending** flag in the hello. The controller arms the hold only when that flag is
   present (and it is itself LB-capable, an old controller doesn't coalesce at all). A router with
   no LB listener, including a dialer-only LB participant that advertises `RouterLoadBalancedLinks`
   but exposes no LB listener, doesn't set the flag and notifies immediately. So the hold keys on
   the precise condition (a withheld listener is coming), not on the dial capability, and needs no
   "always send a snapshot" coordination. The flag is just a hint; an old controller ignores it
   safely, and it carries no listener data, so there's nothing to leak.

The interval is tunable with a small default (~5s). The tradeoff, only when armed, is that a
(re)connecting router's peers learn its listeners up to the interval later; that is bounded and
benign, since peers dial on their own retry cadence. This is a controller-side coalescing of the
connect hello with the immediate `UpdateLinkListeners`; it does not change the hello's
reset-then-set semantics, so the shared non-LB path is untouched. (Genuine LB-listener removal --
a later `UpdateLinkListeners` that drops it -- still reaps normally; only the connect-time
window is coalesced.)

Per-controller gating is then sufficient. A backend withholds its LB listener from any controller
that can't filter it, so an old controller never receives an LB listener to leak to LB-unaware
routers; and a capable controller forwards LB listeners only to routers that advertise
`RouterLoadBalancedLinks`. So in every version combination, including *first connection*, an
LB-unaware router never sees an LB listener.

> *Note:* an earlier revision of this design required *all* connected controllers to be LB-capable,
> because the router's link-key computation would otherwise drop into a legacy mode that forced both
> binding components to `default` and collapsed the reserved `loadbalanced` namespace. That legacy
> link-key mode has since been removed from the router (ziti #3923), so link keys are always
> binding-preserving regardless of controller version, and the simpler per-controller gating is
> sufficient.

**Capability loss is benign for existing links.** If a router that already has LB links later
connects to an older, non-LB-capable controller, it just applies the per-controller rule going
forward (withholds its LB listener from that controller). Existing LB links need **not** be torn
down: they are ordinary established links, indistinguishable from direct links at the controller,
and with legacy link-key mode removed their keys are unaffected by controller version. No
teardown, quiesce, or special transition handling is required.

**Known limitation: divergent reports during a controller upgrade.** Per-controller withholding
means that, while controllers in an HA set have *mixed* capabilities (only during an upgrade), a
backend's LB listener is known to the capable controllers but not the old ones. A peer connected
to both then gets conflicting `PeerStateChange` reports for that backend, the capable controller
includes the LB listener, the old one omits it. Since a peer's per-destination state is keyed by
router ID (not by controller), this is last-writer-wins, so the LB link to that backend can
**flap** (form / tear / re-form) as the two controllers' reports interleave. This is a deliberate
non-goal to engineer around, because: the divergence is *inherent* to the safety requirement (the
backend must withhold its LB listener from the old controller, or the old controller would leak it
unfiltered); every time the link forms it is a *valid* link, so there is no correctness violation,
only transient churn and a few extra LB connections; and it is confined to the upgrade window,
once all controllers are upgraded the reports converge and it self-heals. The operational guidance
is simply to complete controller upgrades promptly. If this churn ever proves painful, a future
hardening could have peers treat an LB listener as present if *any* connected controller reports
it (a per-controller listener union), but we are not building that now.

### Dialer Behavior

When the dialer sees a listener with the LB flag set in `PeerStateChange`:

1. **Group by LB address.** All public routers advertising the same LB address are treated as
   a single pool rather than N independent destinations.
2. **Dial repeatedly, verifying each connection.** After each TLS handshake, the dialer
   extracts the listener's router ID from the certificate, cross-checks against a router ID
   hello header, and verifies the certificate fingerprint against the known set for the pool.
   See Destination Verification below for the rationale; a mismatch in any of these checks is
   an error.
3. **Register the link post-connect.** The verified router ID is used to compute the link key
   and look up or create link state for that backend. A connection to a new backend starts a
   new link; a connection to a known backend joins its underlay (payload or ack channel as
   needed, e.g. to satisfy a 2 payload + 1 ack underlay shape).
4. **Stop when every known backend has a satisfied underlay.** Because the dialer already
   knows the full set of routers advertising this LB address (via `PeerStateChange`), it has
   a concrete target: one link per backend, each with its required payload and ack
   connections. Connections that land on a backend whose link is already fully provisioned are
   closed (see Underlay Across Backends). The case where a backend is advertised but the LB
   never routes a connection to it (out of the LB's rotation, stale advertisement) is handled
   by the consecutive-redundant guard in Fan-out, Termination, and Backoff below, so the dialer
   doesn't dial forever waiting for it.
5. **Warn on non-LB duplicates.** If multiple routers advertise the same address *without* the
   LB flag set (and without the anycast flag), log a warning, this is likely a misconfiguration.

#### Underlay Across Backends

A link's underlay is made of multiple connections (e.g. 2 payload + 1 ack). Behind an LB, each
connection dialed to the LB address may land on a different backend, so the connections for one
logical link could in principle terminate at different routers. **We don't support that, and it
must be actively prevented.** Ziti's routing model assumes a link connects to one specific
destination router; changing that would require large changes well beyond the scope of this
work. If a future routing rework makes "link-to-a-pool" viable, we can revisit.

**Invariant:** no logical link's underlay may span backends. Every underlay connection is
verified (the checks in Destination Verification) and attached only to the link whose verified
router ID matches.

The risk is concrete because the channel layer grows a multi-underlay link's connections *on
its own*. A multi-underlay channel is configured with per-type constraints (desired/min
connection counts) and a dial policy; when it's below desired, it autonomously dials more
underlay connections, reusing the channel's fixed dial endpoint and its own group credentials.
For a direct dial that's correct (same address = same router). Behind an LB it's exactly wrong:
a grow-dial for backend B's link reuses the LB address, round-robins to backend C, and arrives
carrying B's group credentials with no router-identity check at the join point. Left alone,
this silently produces a link whose underlay spans B and C.

**So in LB mode, the channel's autonomous self-grow is turned off and a single group-level LB
dialer owns all dialing.** In `channel/v5` terms this maps cleanly onto the `DialPolicy`
interface: a multi-underlay channel with a `nil` dial policy still enforces its min/desired
constraints for *validity* (it closes if it drops below the minimum) but never dials underlays
itself. LB links are created with no self-grow dial policy; the registry's LB-group dial logic
(the "group dialer," a part of the link registry's run loop rather than a separate component,
see Fan-out, Termination, and Backoff) is the sole driver. For each connection it dials to the LB
and verifies, it then:

- **Attaches** the connection to the matching backend's channel via the channel's
  `AcceptUnderlay` entry point, if that backend's link still wants an underlay of that type.
- **Starts a new link** if the verified backend has no link yet.
- **Closes** the connection if that backend's underlay is already satisfied (the redundant case
  in the backoff model), rather than holding it open or forcing it into a mismatched group.

This keeps the per-link underlay shape predictable, guarantees every underlay connection is
identity-checked before it joins a link, and avoids idle TCP connections the LB would still
charge for. Which dial policy a link uses is a per-link choice: ordinary direct links keep the
default self-growing policy, LB links get the nil-policy-plus-group-dialer wiring, so the
behavior is tweakable rather than global.

**How a connection joins the right backend's link.** Turning off self-grow answers "who dials,"
but not "how does a connection that landed on backend C get grouped into C's link rather than
some other backend's." The key realization is that an LB dial is blind only *through TCP+TLS, not
through the hello*: once the TLS handshake completes, the dialer has the backend's certificate and
therefore its router ID, *before* the channel hello is sent. So the dialer composes a hello that
already names the backend it reached.

A multi-underlay channel groups its connections by a `ConnectionId`, and the listener keys on it:
the listener keeps a `ConnectionId -> channel` map and either joins the existing channel for that
id or, if there's none, creates one. For an LB dial, the dialer sets a **per-instance
`ConnectionId`** in the hello: a fresh token minted when a new link instance is formed, stored in
the per-backend state, reused for that instance's grow-underlays, and **rotated on every re-form**
(see Link Identity). So every underlay the dialer forms for a given instance of backend B's link
carries that instance's `ConnectionId`; the next instance (after a drop and re-form) carries a new
one. They group per-instance, on their respective backends, and can never merge.

Per-instance rather than a deterministic per-backend value is what keeps re-forms clean. The
listener's `MultiListener` holds a `ConnectionId -> channel` entry until the channel's close
callback fires. A deterministic per-backend `ConnectionId` (the same value across re-forms) would
let a fast re-form or a restart arrive while the listener's old channel is **still registered**
under that id, and be accepted as another underlay on the stale, dead instance. Minting a fresh
`ConnectionId` per instance means the re-form's underlays land on a new entry, never the lingering
old one. (Direct multi-underlay links avoid the same hazard the same way: a fresh grouping token
per channel instance.) The `ConnectionId` is set by the dialer and the listener simply groups by
what it receives, so it need not be derivable by both ends; a fresh minted token is enough.

**The `ConnectionId` alone isn't enough; the channel needs its grouped-underlay metadata too.**
`MultiListener.AcceptUnderlay` and `channelImpl.AcceptUnderlay` key off three more headers, so the
hook must stamp all of them or grouping silently fails:

- **`IsGroupedHeader=true`** -- without it, `MultiListener` routes the underlay to its *ungrouped
  fallback* (a plain single channel), not the multi-underlay path. LB dials always set it.
- **`IsFirstGroupConnection`** -- set **true only on the first underlay of a new instance**. When no
  channel yet exists for the `ConnectionId`, `MultiListener` *closes* an underlay that doesn't have
  it; once the channel exists, grow-underlays join by `ConnectionId` and must leave it unset. The
  reserve already distinguishes new-instance from grow, so it carries this verdict.
- **`GroupSecretHeader`** -- a **per-instance secret**. The first underlay establishes it on the
  channel; `channelImpl.AcceptUnderlay` then **rejects any grow-underlay whose secret doesn't
  match** ("incorrect group secret"). So every grow-underlay must carry the same secret as its
  instance's first underlay. It is minted with the instance, stored in the per-backend state next to
  the `ConnectionId`, reused for the instance's grows, and **rotated with the `ConnectionId` on
  re-form**. This is the channel's own grow-authentication, per channel instance; it is *not* the
  pool-shared secret an earlier design used (that guarded cross-pool injection and was dropped) --
  this one is required mechanically by the multi-underlay API regardless of LB.

Because LB turns off the channel's self-grow (`DialPolicy`), the *group dialer* dials every
underlay, so it (not the channel) is responsible for carrying the matching secret on each grow. And
it only issues a **grow** reservation once the instance's **first underlay has committed** -- so the
listener's channel exists before grow-underlays arrive (matching the create-then-join order
`MultiListener` enforces, avoiding the "no channel yet, not first -> closed" churn).

The no-spanning invariant then holds **by construction, with no pool-shared identifier**: each
instance's `ConnectionId` is distinct, so a connection only ever joins the channel for the backend
instance it actually reached. There's no "what credentials do I send before I know the backend"
problem, because by hello-compose time the dialer *does* know the backend.

This relies on a `channel/v5` hook that lets the dialer compute hello headers from the peer
certificate after the TLS handshake and before the hello is sent (see Dependencies). With it:

- **Listener-side grouping needs no change.** It's the ordinary `ConnectionId`-keyed
  create-or-join the multi-underlay listener already does (`xlink_transport/listener.go` builds a
  `MultiListener`), and the link registry sits above it, deduping established links by link key.
  Because the `ConnectionId` is per-instance rather than pool-shared, the shared-secret injection
  concern an earlier design had to guard against (binding the group to the dialer's certificate)
  **doesn't arise** -- there's no pool-shared id for a different pool member to collide with, so
  ordinary channel identity handling applies.
- **The group dialer still owns all dialing.** For each connection it dials and verifies, it lets
  the channel form or grow backend B's current instance under that instance's `ConnectionId`,
  starts a new instance (with a freshly minted `ConnectionId`) if B has none, or closes the
  connection if B is already satisfied (the redundant case). The only
  listener change for LB remains honoring the LB flag to skip the dialed-router-ID check (see LB
  Dial Contract).

So an LB link's grouping, once the dialer knows its backend, is just ordinary multi-underlay
grouping with a per-instance `ConnectionId`. The earlier "uniform pool-shared `ConnectionId` +
mandatory cert-owner-binding" machinery is unnecessary and is dropped.

**Dependency on the `channel/v5` link channel.** Four `channel/v5` pieces underpin the above: the
`DialPolicy` injection point (to turn off self-grow), `AcceptUnderlay` (to attach a verified
connection to a backend's channel), the **hello-header hook** that lets the dialer set hello
*headers* from the peer certificate post-TLS (the LB per-instance `ConnectionId`, link ID via
`LinkHeaderLinkId`, iteration, and underlay type) **and that feeds those provider headers into the
dialer's own underlay initialization, not just the outbound hello** -- so a provider-set
`ConnectionId` and underlay type drive the dialer's local grouping and type, matching what the
listener sees (without this, the dialer would group its own underlay by a stale value while the
listener groups by the provider's, splitting the link), and the **`LinkHeaderLinkId`** header itself
(already landed via openziti/ziti #3938) so the link ID can travel in a header rather than the
channel-identity token (the hook is headers-only, so this is how the LB link ID is carried; see
Link Identity). The router-to-router link channel is being migrated to `channel/v5`; this LB work
depends on that migration and on the hello-header hook (openziti/channel #258, PR #259) landing.
On `channel/v4` none of these injection points exist cleanly, so we build on v5 rather than
retrofit v4.

A fifth dependency is the v5 channel's **underlay-constraint config** on the link. A `channel/v5`
multi-underlay channel enforces per-type `UnderlayConstraint{Min, Desired}` (and a
`minTotalUnderlays`), closing itself if a minimum is unmet (`channelImpl.countsShowValidState`).
The general multi-underlay policy is **one required (fallback) underlay type** that all
communication can fall back to, so a channel is functional with a single underlay of that type and
every other type is optional (`Min = 0`). LB relies on this shape: the group dialer dials the
**required type first** so the listener's channel is valid from its first underlay (see The target
is per-backend, per-underlay-type), then grows the optional types. So the LB link's v5 constraint
config must follow the one-required-type policy, and the group dialer must know which type is
required; this is owned by the `xlink_transport` v5 link config the migration sets, and LB
constrains it to that shape.

### Destination Verification

Two verifications run in **opposite directions**, and it's important to keep them separate:

- **Listener verifies the dialer** (the connection initiator). This exists today: the listener
  calls `verifyRouter()`, a synchronous controller RPC, to confirm the dialing router's
  certificate belongs to a known router.
- **Dialer verifies the destination** (the router it actually reached). This does *not* exist
  today, the dialer assumes the address it dialed corresponds to the router it found in
  `PeerStateChange`. That assumption is fine for a direct dial but breaks for an LB dial, where the
  dialer can't know which backend it reached until it inspects what came back.

Both move to local, fingerprint-based verification, and that is **Phase 1** of this effort, worth
doing on its own even if everything else is deferred: it removes a controller round-trip from
every link setup and adds destination verification to every link. LB (and anycast) build on Phase
1's dialer-side destination verification, they don't replace it.

#### Phase 1: fingerprint-based router verification (both directions)

Both directions match a presented certificate against the RDM-distributed router fingerprint set
(see Fingerprint Distribution):

- **Listener side, replacing the `verifyRouter()` RPC.** The listener matches the *dialer's*
  presented cert fingerprint against the local RDM fingerprint for the dialer's claimed router ID,
  same authentication as the RPC, no controller round-trip. This is a *local* decision gated only
  on fingerprint availability, no peer capability is involved. While fingerprint distribution is
  still rolling out and a fingerprint isn't yet available, the listener falls back to the existing
  `verifyRouter()` RPC; the RPC is retired only once distribution is guaranteed.
- **Dialer side, new.** After the handshake the dialer verifies the destination it reached. The
  checks fall into two groups by **when the data they need is available** -- the cert is in hand
  post-TLS, but the listener's hello headers (including its `CapabilitiesHeader`) don't arrive until
  the hello *response*. Note that *data availability* and *enforcement timing* are not the same
  thing: the cert-derived checks below can be **computed** pre-hello, but **when their verdict is
  enforced differs by dial type** (LB enforces pre-hello, direct defers to the response), which is
  the crux of the LB-vs-direct split.

  **Cert-derived, computable pre-hello** (post-TLS, from the cert alone):
  1. **Cert ID extracted** -- the listener's router ID is read from its presented TLS certificate.
  2. **Fingerprint matches that ID** -- the presented cert's fingerprint matches the RDM
     fingerprint for that router ID, proving it's a legitimate controller-known router.
  3. **Destination match** -- the verified router ID is an acceptable destination (differs by dial
     type, below).

  For an **LB dial these three *are* the reserve admission decision, enforced pre-hello**: the
  hello-header hook runs here, post-TLS/pre-hello, and `reserveLbUnderlay` consumes exactly this set
  (fingerprint + membership) plus the deficit accounting (see Fan-out, Termination, and Backoff). A
  failure aborts the connection **before any hello is sent** -- LB fails closed (missing fingerprint
  or non-member → no underlay forms). LB needs **no capability gate** (it's new-by-construction, so
  every backend supports verification), so nothing about the LB decision waits on the listener's
  hello.

  For a **direct dial the enforcement is deferred to the hello response**, even though the cert data
  is available pre-hello. The direct-dial destination check is gated on the listener advertising
  `RouterLinkDestVerification` (below), which arrives in the listener's hello-response
  `CapabilitiesHeader` -- *after* the dialer has sent its hello. So a direct dial computes the
  cert/fingerprint pre-hello if convenient but makes the **enforce-or-fall-back decision on the
  response**: capability present (and a fingerprint available) → verify strictly, a mismatch is an
  error; capability absent → **no destination check, today's behavior**. A direct dial must **not**
  enforce pre-hello, or it would reject legacy peers the fallback is meant to accommodate. (There's
  no reserve on the direct path; this is the ordinary post-connect link setup.)

  **Post-response cross-check** (after the hello round-trip):
  4. **Header matches cert** -- the listener's router identity in the hello response matches the
     cert ID verified above. The identity in the response is the channel **`IdHeader`** (the
     listener sets `response IdHeader = its router identity`, `classic_listener.go`; the dialer
     already reads it as the peer's `Channel.Id()`), *not* `LinkHeaderRouterId` -- that header
     carries the *dialer's* own router id in the *request*, a different field in the other
     direction. The cross-check compares the response `IdHeader` against the cert-derived ID. It
     can't be checked at reserve time because it doesn't exist until the hello response. Since the cert checks already passed, this is a cheap consistency check (it
     catches a listener whose self-reported ID disagrees with its cert, and covers the cert-less
     fallback path; a mismatch signals misconfiguration or MITM). On an LB dial a mismatch is a
     **verification-failure outcome that rolls back the pre-hello reservation** (restores the
     reserved deficit so fan-out re-counts the backend) and closes the underlay -- so commit never
     happens for a mismatched underlay (see The pre-hello reservation: commit is the last step,
     gated on this check). On a direct dial it faults/closes the just-formed underlay. See Listener
     Behavior.

  The dialer-side fallback, when it *can't* verify (peer too old to send the header, or no
  fingerprint yet), is **no destination check at all, exactly today's behavior** -- *not* the
  listener-side `verifyRouter()` RPC, which is the other direction and is unaffected. Because
  dialer-side destination verification is brand new, making it opportunistic is never a regression.
  (This fallback applies to direct dials; LB fails closed, as above.)

#### Destination match: direct vs LB (Phase 2 for LB)

The destination match is check 3 above, and it's where direct and LB diverge -- both in *what*
counts as a match and in *when* the verdict is enforced:

- **Direct dial.** The verified router ID must equal the single expected destination. **Enforced on
  the hello response**, gated on the peer advertising the capability below; if it doesn't (or no
  fingerprint yet), no destination check (the opportunistic fallback above). The cert/fingerprint
  may be computed pre-hello, but the enforce-or-fall-back decision waits for the response, since the
  capability isn't known until then.
- **LB dial.** The verified router ID must be a current member of the set advertising this LB
  address in `PeerStateChange` (the dialer already has this set from grouping). A verified router
  that isn't a pool member means misconfiguration (wrong backend, stale advertisement, routing
  error), the connection is **closed and an alert raised**. And LB dials **fail closed**: if the
  fingerprint isn't available the connection is closed and retried, never accepted unverified. This
  is safe because LB is new-by-construction (it already requires the `channel/v5` link channel, the
  router-ID header, and RDM fingerprints), so there's no legacy peer to accommodate.

#### Capability gating for the dialer-side check on direct dials

The dialer-side destination check on *direct* dials is gated by a link capability,
`RouterLinkDestVerification`, advertised (consistent with the `common/capabilities` bitmask in the
channel hello via `CapabilitiesHeader`) by a router that sends its router-ID header and
participates in destination verification. For a direct dial: if the peer advertised it and a
fingerprint is available, verify strictly (a mismatch is an error); otherwise do no destination
check (today's behavior). A MITM gains nothing by stripping the capability, the fallback is
exactly current behavior. Once a minimum fleet version is enforced the check can be made mandatory
for direct dials; that's the end state, not assumed now. (The listener-side `verifyRouter()`
replacement is *not* gated on this capability, it's the local fingerprint-availability decision
above.)

### Fingerprint Distribution

Both verification directions (Phase 1) need the same thing: a local view of every router's
certificate fingerprint, so a presented cert can be checked without a controller round-trip.
Today the only such check is `verifyRouter()`, a synchronous controller RPC.

This piece is largely in flight. As part of the upcoming controller-managed router configuration
work, router certificate fingerprints are being distributed through the router data model (RDM)
and sync'd to every router. That code isn't merged yet (it's a prerequisite, see Work Items), but
the fingerprint-distribution piece is done. Once it lands, every router has an up-to-date local
view of all router fingerprints and can verify both the dialers connecting to it (replacing
`verifyRouter()`) and the destinations it dials (the new check), with no controller round-trip.
The fingerprint store needs no special LB-pool grouping, it's just the set of all router
fingerprints, keyed by router ID.

For an **LB dial** specifically: the dialer takes the fingerprint from the presented certificate,
looks it up by the cert's router ID in the local set, and knows it reached a legitimate
controller-known router. But a fingerprint match alone is *not* sufficient to accept the link: it
proves "this is a real mesh router," not "this is a router I expected behind this LB address."
Acceptance additionally requires the pool-membership check (Destination match, above), the matched
router ID must be advertising this LB address in `PeerStateChange`. The hello-header cross-check
ensures the cert was actually presented by the router that owns it.

### Listener Behavior

The listener's router identity is **authoritatively its leaf certificate** (the CN is the router
ID), which the channel already exposes, `channel/v5` even uses it as the channel's owner identity
when no `IdHeader` overrides it. So the dialer takes the listener's router ID from the presented
cert (this is the "extract cert ID" step in Destination Verification); no new authoritative
identity field is introduced.

The one listener change is to also send the router ID in a **hello header as a cross-check**, not
as a second authoritative source. It lets the dialer compare header-vs-cert cheaply (a mismatch
signals misconfiguration or tampering) without parsing the cert first, and it is the identity
source for any transport that presents no certificate to pull the ID from. For LB links, which are
always TLS (L4 passthrough, the cert is end-to-end), the cert is always present and the header is
purely a cross-check.

A listener serving an LB-advertised listener must also honor the LB dial flag: skip the
dialed-router-ID check when the `LinkHeaderLoadBalanced` header is set, rather than rejecting
the connection for not carrying its own router ID (see LB Dial Contract).

Finally, two listener-side rules keep the link key unambiguous (see Link Key Construction):

- **At most one LB listener per router.** A router may expose a single LB listener. Configuring
  more than one is a misconfiguration and is rejected at config validation. This guarantees
  there is exactly one listener the `loadbalanced` binding applies to, so the listener can
  compute the inbound LB link key with that binding with no ambiguity.
- **The LB listener advertises the reserved `loadbalanced` binding.** Whatever local interface
  the operator binds, the listener publishes `loadbalanced` as its listener binding in the
  `PeerStateChange` advertisement, and uses the same value when computing the inbound link key.
  The dialer then sees a single, consistent binding for the LB listener and needs no special
  key handling.

Otherwise the bind, advertise, and TLS handshake behavior all stay the same as for a direct
listener.

### Link Key Construction

Today, link keys have the form `dialerBinding->protocol:destRouterId->listenerBinding` and are
computed *before* dialing, using the destination router ID already known from the
`PeerStateChange` data. The key uniquely identifies a link and is what link registry dedup
relies on.

For LB dials the format stays the same, but two things change: the timing, and the listener
binding component.

**Timing.** The destination router ID isn't known until after the TLS handshake and post-connect
verification, so the key is computed *after* connecting rather than before. Once the key is in
hand, the link is registered through the normal registry path.

**Listener binding component is a reserved constant.** For a direct link the key's listener
binding is the destination's advertised local binding. For an LB link we don't want that value
to vary across the backends behind one address, otherwise backends advertising the same LB
address under different bindings would produce inconsistent keys. So an LB listener advertises a
reserved binding value, `loadbalanced`, regardless of the local interface it actually binds. The
operator can still choose any interface; only the *advertised* binding is normalized. This is
enforced as a listener-side rule (see Listener Behavior): a router may expose at most one LB
listener, and it is always advertised with the `loadbalanced` binding. Because there is exactly
one such listener per router, both sides compute the key with the same value, the dialer from the
advertised binding and the listener for its single LB listener, with no per-connection
binding logic.

A consequence is that LB links live in their own key namespace, distinct from direct links: an
LB link to router B keys as `dialerBinding->protocol:B->loadbalanced`, while a direct link to B
keys with B's real binding. The two do not dedup against each other. That is intentional, they
genuinely traverse different paths, and the rare case where we reach a router both ways is
handled in the fan-out target logic rather than by key collision (see Direct-link collision).

This relies on the router's link keys being binding-preserving. An older router had a legacy
link-key mode that forced both binding components to `default` (which would have erased
`loadbalanced` and collapsed the LB and direct namespaces), but that mode has been removed (ziti
#3923), so `GetLinkKey` now always preserves the bindings and the reserved `loadbalanced` value is
always honored.

Within LB links, dedup still falls out naturally: two dials through the LB that land on the same
backend compute the same key once verified (same dest router ID, same `loadbalanced` binding),
and the registry collapses them just as it would two concurrent direct dials to one destination.

### Link Identity: ConnectionId vs Key vs ID

A link involves three identities. For an LB link the dialer sets all three once it knows the
backend (post-TLS, before the hello), and from there they behave like a direct link's:

- **`ConnectionId`** -- the multi-underlay grouping key. For LB it is **per-instance**: a fresh
  token minted when a new link instance forms, stored in the per-backend state, reused for that
  instance's grow-underlays, and rotated on every re-form, then set in the hello so each instance's
  underlays group separately and a re-form never lands on the listener's stale channel (see Underlay
  Across Backends). A transport-grouping token, not a link identity -- and deliberately distinct
  from the link ID: the link ID is *stable* across re-forms (for controller continuity) while the
  `ConnectionId` *rotates* per instance (for listener-side grouping freshness).
- **Link key** -- the deterministic dedup string `dialerBinding->protocol:destRouterId->loadbalanced`,
  computed post-verification (see Link Key Construction).
- **Link ID** -- the link identifier the controller uses for `RouterLinks` and faults. For LB it is
  **minted by the dialer** and carried in the hello, exactly as a direct link mints and conveys its
  link ID, only set post-TLS rather than pre-dial. It is **stable for the backend's retained
  per-backend link state across re-forms** (a re-form reuses the ID with `iteration + 1`); a fresh
  `uuid.New()` is minted only for a *new* per-backend state (a backend first seen, or re-seen after
  member removal) or a process restart. (Contrast the `ConnectionId`, which *rotates* per instance;
  the link ID deliberately does not.)

**This is the whole simplification.** A direct dial knows its destination before connecting, so it
stamps the link ID and iteration into the dial headers up front. An LB dial is blind *only through
TLS*: by the time it composes the channel hello it has the backend's certificate, so it stamps the
same things into the hello then. The `channel/v5` hello-header hook (see Dependencies) is what lets
it set hello headers from the peer cert post-TLS. So an LB link's identity is *ordinary* link
identity, transported in the post-TLS hello. Specifically:

- **Both ends agree, so bilateral faulting works.** The dialer puts `linkId` and `iteration` in
  the hello; the listener reads them rather than deriving anything. Both sides hold the same
  `(linkId, iteration)`, so either side can fault at the current iteration and it's honored, and
  the controller's iteration-supersede (`fault.Iteration < link.Iteration -> ignore`) drops stale
  faults from either end. This is exactly how direct links behave today; LB inherits it. (This was
  the property that an earlier derive-the-ID approach struggled to keep, because the dialer and a
  half-open listener could disagree on the iteration; transporting it removes the disagreement.)

- **`iteration` is per-link, like a direct link:** the link ID is stable across reconnects, the
  iteration increments on each re-form, and both are transported. A re-form reuses the link ID
  with a higher iteration so the controller supersedes the old instance; a process restart mints
  *fresh* link IDs (again, like direct links), so faults for pre-restart IDs simply don't match
  anything after restart. No group epoch, no group iteration, no derivation, none of that
  apparatus is needed once identity is transported rather than derived.

- **Link ID rides in `LinkHeaderLinkId`, not the channel-identity token.** A *direct* link today
  carries its link ID by cloning the dialer identity's *token* to the link ID
  (`ShallowCloneWithNewToken`), so `Channel.Id()` comes out as the link ID. An LB dial can't do
  that, the link ID isn't known until post-TLS, and the hello-header hook can set *headers* but not
  the identity token. Instead the LB dialer carries the link ID in the dedicated
  **`LinkHeaderLinkId`** header (already landed via openziti/ziti #3938 -- emitted on dial and
  preferred on read, see Header allocation below), set post-TLS via the hook, along with the
  iteration in `LinkHeaderIteration`, the
  per-instance grouping key in `ConnectionIdHeader`, the dialer's router id in `LinkHeaderRouterId`,
  and the grouped-underlay metadata the multi-underlay listener requires -- `IsGroupedHeader=true`,
  `IsFirstGroupConnection` (first underlay of a new instance only), and the per-instance
  `GroupSecretHeader` (see Underlay Across Backends). The dialer's hello *token* stays its real
  router identity (no override needed, and nothing to compute pre-dial).
- **Listener reads `LinkHeaderLinkId` if present, else `Channel.Id()`.** The listener uses the link
  ID from `LinkHeaderLinkId` when the header is present, falling back to `Channel.Id()` (the token)
  otherwise. This is safe in a mixed fleet (an old dialer omits the header -> token fallback,
  today's behavior) and is exactly the *listener half* of the v5 link-id-header migration; if that
  migration lands first, the listener already does it. LB-capable routers are by definition new
  enough to emit and read the header, so an LB link's ID always comes from `LinkHeaderLinkId`, and
  `Channel.Id()` for an LB link is just the dialer's router id (the post-flip convention). The only
  other LB-*specific* header is the `LinkHeaderLoadBalanced` flag (skip the dialed-router-ID check);
  the grouped-underlay headers (`IsGroupedHeader`, `IsFirstGroupConnection`, `GroupSecretHeader`)
  aren't LB-specific -- they're the channel's standard multi-underlay headers, but where a normal
  multi-underlay dial sets them via the channel's self-grow `DialPolicy`, LB (which turns self-grow
  off) stamps them through the hook instead. Direct links are entirely unchanged.

The earlier rounds' derived `DeriveLbLinkId` (namespace UUID, length-prefixed encoding), the group
epoch, the group iteration counter, and the mandatory listener-side cert-owner-binding were all
workarounds for the assumption that the dialer was blind *through link formation* and therefore
couldn't transport identity or pick a per-instance `ConnectionId`. The hello being post-TLS makes
that assumption false, so all of it is dropped. The underlay-grouping injection concern those
guards addressed also dissolves: with a **per-instance `ConnectionId`** there is no pool-shared id
or secret for another pool member to collide with, so ordinary channel identity handling applies.

**Header allocation.** All of these headers ride the **channel hello's single header map**, which
the link layer *shares* with the channel library's own headers. The channel owns the low range and
the grouped-underlay dial path overlays `channel.TypeHeader = 7`, `IdHeader = 8`,
`IsGroupedHeader = 9`, `GroupSecretHeader = 10`, `IsFirstGroupConnection = 11`, and
`ConnectionIdHeader = 0` onto the same map. So a link header that reused a channel key would
silently overwrite grouping/type metadata and form a wrong or rejected link. This is already solved
upstream: openziti/ziti #3938 added `LinkHeaderLinkId = 100` with the rule (documented in
`router/xlink_transport/factory.go`) that **new link headers start at ≥100 to stay clear of the
channel library's key range** (the pre-existing link headers `LinkHeaderConnId = 0` … `LinkDialedRouterId = 6`
predate the overlay and are managed against the specific channel keys actually used). LB inherits
this: it builds on the #3938 link-id header rather than introducing it, and any **new** LB link
header (e.g. `LinkHeaderLoadBalanced`) is allocated at **≥100** per the same rule. The LB
capabilities (C1) are *not* link-hello headers, so they don't enter this keyspace.

### LB Dial Contract

For direct dials, the dialer knows the destination before connecting: it picks the underlay
mode from the destination's version, stamps the destination router ID into the dial headers,
and builds the link object (id, key, router ID) up front. None of that information is available
for an LB dial, where the backend isn't known until after the handshake. LB dials therefore
follow a distinct contract:

1. **Underlay mode: always multi-underlay.** A direct dial selects between the multi-underlay,
   split, and single-channel implementations based on the destination router's advertised
   version. An LB dial can't, since the version isn't known pre-connect and backends may
   differ, so the dialer uses multi-underlay unconditionally for LB dials. Multi-underlay itself is
   the historical 1.6.6+ baseline, but that version alone is **not** sufficient to be an LB backend:
   LB additionally requires the `channel/v5` link channel, the hello-header hook, `LinkHeaderLinkId`,
   and the LB capabilities (see Dependencies and the Prerequisite work items). So "supports
   multi-underlay" is necessary but not the bar; an LB backend must be a current LB-capable/channel-v5
   router build. This is a deployment requirement, not a runtime negotiation: an operator fronting
   routers with an LB is expected to run a build that supports it. This is also what lets us build
   the link post-dial (see point 3).

2. **Dial mode flagged with a dedicated header.** Direct dials send the expected destination
   router ID in the `LinkDialedRouterId` hello header, and the listener rejects the connection
   if it isn't the addressed router. An LB dial has no single expected router ID, so the
   authoritative signal is a dedicated boolean hello header, `LinkHeaderLoadBalanced: true`. A
   listener serving an LB-advertised listener sees this flag and skips the dialed-router-ID
   check; correctness instead rests on the dialer's post-handshake verification (see Destination
   Verification). A boolean is trivial to read with the existing typed-header helpers and the
   decision never depends on parsing an ID field.

   The LB dial still populates `LinkDialedRouterId`, but with the LB address it dialed rather
   than a real router ID. This is purely diagnostic: paired with the boolean flag, it gives
   listener-side debug logs a clear record that the connection arrived via an LB and through
   which address, which helps track down misconfigured advertisements. The listener does *not*
   key any decision off this value, the boolean flag drives the skip.

3. **Identity is set in the hello, post-TLS.** Today a link's id/key/ConnectionId are built before
   the channel is dialed. That works for a direct dial (the destination is known up front) and is
   also forced by the split dialer, which opens two channels and needs a shared link object to
   bind them. An LB dial can't pre-build identity (the backend is unknown until the handshake), but
   it doesn't need to: the channel hello is sent *after* the TLS handshake, so via the
   `channel/v5` hello-header hook (see Dependencies) the dialer composes the hello *after* it has
   verified the backend, stamping the per-instance `ConnectionId`, the link ID, and the iteration
   into it. So the LB dial flow is: dial the LB, complete the handshake, verify the backend, then
   build/look-up the link and send the hello carrying its identity. (Multi-underlay only, per point
   1; the split and single paths keep their pre-connect construction and aren't used for LB dials.)

### Reconnect

If a link drops, the dialer re-dials the LB address. Because the dialer can't target a
specific backend, re-establishing a link can take several dials: round-robin (or
least-connections) will steer the new connection to whichever backend the LB decides, which
may or may not be one the dialer currently wants more underlay to. Connections that land on a
fully-provisioned backend are closed (see Underlay Across Backends), and the dialer tries
again (subject to the fan-out and backoff model above). This produces some churn at reconnect
time, more noticeable on small pools where the odds of hitting an already-saturated backend are
higher; see Reconnect Churn below.

The new link is registered with the discovered router's ID, the controller receives updated
`RouterLinks`, and routing adjusts. The replacement may land on a different backend than the
dropped one, that's fine and is the whole point of the pool design, *as long as that backend is
currently unsatisfied*. A reconnect dial is accepted only for a backend that still needs a link
or underlay under the membership/quota target; if it lands on an already-satisfied backend it is
a redundant close and fan-out continues, exactly as in the steady-state model (see Fan-out,
Termination, and Backoff). Reconnect is not a special case, it's the same fan-out loop re-armed
by the drop.

### Fan-out, Termination, and Backoff

Existing link retry uses per-destination exponential backoff (see `linkState.dialFailed`): one
state per destination router, one expected dial, exponential backoff on failure. That model
doesn't fit a pool, where a single dial to the LB address can land on any backend and "failure"
is no longer a single condition. A pool-aware model is layered on top.

**Where this lives.** "Group dialer" in this document is shorthand for the link registry's
LB-group dial logic, *not* a separate component or goroutine. The registry already owns all
dialing from its single `run()` loop (it processes `destinations` and drains the
`linkStateQueue` on its tickers, dispatching the actual network dials to the existing dial pool).
LB groups slot into that same model: an `lbGroups` map alongside `destinations`, evaluated in the
same loop, with group re-dials riding the same queue. So no new goroutine and no per-group timer,
overall load stays flat. The split is on this axis:

- **The run loop owns state.** All LB group state mutation (membership, per-backend satisfaction,
  counters, backoff, re-arm) happens in the registry loop, in-memory and non-blocking, just like
  the existing per-destination state. The LB logic is factored into its own type/file but is
  *called from* the loop, so the non-LB path isn't muddied.
- **The dial pool owns I/O.** The TLS handshake and post-connect verification run on the dial
  pool, never in the loop. A completed dial's outcome (the classification below) returns to the
  loop as an **event on the registry's existing events channel** (the same pattern as
  `dialFailed`/`markNewLinksNotified` today), so state stays single-threaded.
- **Two concurrency limits compose.** `lb.maxConcurrentDials` is a *per-group* cap the group
  logic enforces on its own in-flight dials; the dial pool is the *global* bound. Both apply, so a
  busy group can't monopolize the pool and the pool can't let a group exceed its cap.

**The pre-hello reservation.** There's a wrinkle in "I/O on the pool, state on the loop": the
hello-header hook runs on the dial-pool goroutine *mid-dial* (post-TLS, pre-hello), and the
decisions it must make there, mint-or-reuse the link ID, pick the iteration, pick the per-instance
`ConnectionId`, pick the underlay type from the backend's deficit, decide redundant-or-close, are
all group state. The hook must not read or mutate that state off-loop, and it must not defer the
decision until after the listener has already accepted the underlay (that's how concurrent dials
silently overfill a quota or land inconsistent identities). So the hook makes a **synchronous
hand-off to the loop**:

1. **Reserve (synchronous, mid-dial).** Post-TLS, the hook extracts the backend router id from the
   cert and calls `reserveLbUnderlay(backendId, certFingerprint)`, an event on the registry loop.
   The loop, holding group state, makes the whole admission decision and returns one of: **accept**
   plus the reserved `{linkId (minted or reused), iteration, per-instance ConnectionId, per-instance
   groupSecret, isFirst, underlay type}`, marking an **in-flight reservation** for that (backend,
   type) and decrementing the deficit; or **close** (redundant, no remaining deficit counting
   in-flight), **verification failure** (fingerprint mismatch), or **missing fingerprint**. The
   `ConnectionId` and `groupSecret` follow the instance: a grow-underlay onto an existing instance
   gets that instance's stored pair (and `isFirst=false`), while a new instance (first underlay, or
   a re-form) gets a **freshly minted** pair (rotated from the prior instance's) and `isFirst=true`,
   so the listener never groups a re-form onto a stale channel and grow-underlays authenticate into
   the right instance. The reserved **underlay type also follows `isFirst`**: a new instance
   reserves the channel's **required (fallback) type** so the created channel is valid from its
   first underlay, while a grow reserves the **largest outstanding per-type deficit** among the
   remaining optional types (see The target is per-backend, per-underlay-type). The hook stamps
   `IsGroupedHeader=true`, `IsFirstGroupConnection=isFirst`, and `GroupSecretHeader=groupSecret`
   alongside the identity headers (see Underlay Across Backends). The loop issues a **grow**
   reservation only after the instance's first underlay has committed, so the listener's channel
   exists (and is already valid) before grows arrive. The hook then
   either composes the hello with the reserved values, or aborts the connection per the verdict.
   Folding membership +
   fingerprint + deficit into this one loop event keeps it the single decision point (the
   listener-router-id *header* cross-check is separate and post-response, see Destination
   Verification).

   The hand-off is **time-bounded so it can never pin a dial-pool worker**. The hook enqueues the
   reserve event and waits for the verdict under a single deadline (`lb.reserveTimeout`, default
   ~200ms) that spans *both* the enqueue onto the registry's **bounded** events channel *and* the
   wait for the reply -- the enqueue matters because `queueEvent` blocks on a full channel, so a
   response-only timeout would miss a loop that's backlogged before the hook even starts waiting. If
   either phase can't complete in time, the hook **aborts the dial fail-closed** (closes the
   connection, frees the dial-pool worker); the group's normal re-arm/backoff retries later. The
   reserve handler is O(in-memory) once the loop reaches it, so the deadline only ever trips under
   loop backlog, never on the happy path. (`IsKnownLinkId` is a *partial* precedent: it bounds only
   the reply wait, while its `queueEvent` can still block on a full channel. The reserve needs the
   stronger shape -- a `queueEventWithDeadline` whose deadline covers the enqueue too -- precisely
   because it runs mid-dial holding a dial-pool worker, where `IsKnownLinkId` does not.)
2. **Commit / rollback (async, post-dial).** **Commit is the *last* step, not hello-success.** The
   reserve runs pre-hello, but the post-response router-ID header cross-check (Destination
   Verification check 4) and the local attach/register only happen *after* the hello round-trip, so
   commit must wait for **all** of them: a completion event **commits** the reservation (in-flight
   -> established) only once the header cross-check has passed *and* the underlay has attached and
   registered. If any step fails -- the dial or hello errors, the **header cross-check mismatches**
   (a verification-failure), or attach/register fails -- a **rollback** event releases the
   reservation (restores the deficit so fan-out re-counts the backend) and the underlay is closed.
   Committing on hello-success *before* the cross-check would be wrong: a later mismatch would tear
   down an already-committed underlay, leaving the deficit decremented and the backend
   phantom-satisfied until the lease reaper happened to notice. This is the same async-outcome path
   described above.

   **Commit revalidates against group changes during the async window.** Because the dial/hello runs
   async between reserve and commit, the group state can move underneath the reservation: the
   backend can leave membership (`PeerState_Removed`), or get **satisfied by a direct link** that
   established meanwhile. So commit re-checks, on the loop, that the reservation is **still valid**:
   it still exists (not reaped or rolled back), the backend is **still a member**, it is **not
   satisfied-by-direct**, and it **still has the reserved deficit**. If any check fails, commit
   becomes a **rollback** -- close the underlay, restore the deficit -- rather than registering. This
   matters because registry dedup will *not* catch a stale LB commit: a direct and an LB link to the
   same backend have different keys (`...->default` vs `...->loadbalanced`), so a blind commit would
   register an LB link parallel to a direct one, or an orphan link to a router no longer in the pool.
3. **Leased reservations self-heal.** The grant reply rides a buffered channel, so a hook that has
   already hit its deadline and walked away can leave the loop holding a granted reservation that no
   commit or rollback will ever clear -- a phantom deficit decrement that would wedge the backend
   below target forever. To prevent that, every in-flight reservation is **leased**: the loop stamps
   it with a deadline when it grants, and the existing periodic tick (the `queueCheckTicker`, 5s)
   **reaps any reservation past its lease that never committed**, restoring the deficit. A normal
   commit/rollback clears the lease early; the reaper is the backstop that also reclaims a
   reservation whose rollback event was lost. So a timed-out (or otherwise abandoned) reserve always
   self-heals, and the loop's serialized accounting can't drift.

Because reserve is serialized on the loop and counts in-flight reservations, two concurrent dials
landing on the same (backend, type) can't both succeed, the first reserves the slot, the second
gets close-redundant. And there's no deadlock: the loop never blocks waiting on the dial pool (it
dispatches dials and moves on), so it's always free to service a reserve while the dial-pool
goroutine briefly blocks for the reply; the reserve work is O(in-memory). The dial-pool goroutine's
block is itself bounded by `lb.reserveTimeout`, so even a backlogged or stalled loop can't pin a
dial-pool worker indefinitely, the worker aborts fail-closed and frees up.

State lives at two levels (the lifecycle details are in Implementation Notes below):

- **Group level (per LB address):** owns the dial cadence, the target set of advertised
  backends and their underlay-satisfaction status, the connection-failure backoff, and a
  consecutive-redundant-dial counter.
- **Per-backend level:** each discovered backend's link and underlay quota. This level is
  **bookkeeping only and never independently dials.** Unlike a direct destination's `linkState`
  (which is itself a dial loop, with `nextDial`/`dialFailed`/backoff that enqueues it on the dial
  queue), an LB per-backend state has no address of its own to dial, the only dial target is the
  LB address, owned by the group. So an LB per-backend state is never enqueued on the dial queue
  and never runs `dialFailed`/backoff. When a backend's link drops, it transitions that backend to
  *unsatisfied* and **re-arms the group dialer**; the group then re-fans-out per its own cadence.
  A drop **retains the backend's identity bookkeeping** -- the **link ID** and the **next
  iteration** -- and clears only the *live instance* bookkeeping (established underlay references,
  the current instance's `ConnectionId`, any in-flight reservations). That's what lets the re-form
  reuse the same link ID with `iteration + 1` (so the controller supersedes the old instance,
  per Link Identity) rather than minting an unrelated new ID. The retained link ID + iteration are
  discarded only when the backend **leaves the group** (`PeerState_Removed` / the LB address no
  longer advertised), not on a transient drop. This state is in-memory only, so a process restart
  loses it and the backend gets a *fresh* link ID on re-acquisition -- exactly the intended
  "restart mints fresh IDs, pre-restart faults don't match" behavior, with no persistence needed.
  All retry pacing for the pool flows through the single group loop, so a stray per-backend dial
  can't bypass the connection-failure backoff, the redundant-guard, or the healthy/unhealthy
  selection. (An LB per-backend state may reuse the `linkState` struct for its bookkeeping fields,
  but its dial-loop machinery is inert; direct destinations keep their per-destination dial loop
  unchanged.)

**The target is per-backend, per-underlay-type.** The dialer knows the full set of routers
advertising this LB address from `PeerStateChange`. The goal is one link per advertised backend,
each with its full underlay shape, and that shape is **per type** (e.g. 2 payload + 1 ack). So a
backend isn't "satisfied" until *each* underlay type meets its desired count; tracking only a
total would let the dialer pile up payload connections while an ack deficit lingers.

This interacts with blind dialing: the underlay type is normally chosen before the connect, but an
LB dial doesn't know the backend yet, so it can't know which type that backend needs. The
hello-header hook solves this the same way it solves identity: the dialer chooses the underlay
**type post-TLS**, once it has verified the backend, and stamps it into the hello. So a connection
that lands on backend B is assigned a type B needs, rather than committing to a type blindly. If
the verified backend needs *no* more underlays of *any* type, it's the redundant case (closed); a
"verified but this backend doesn't need this type" outcome can't occur, because the type is only
chosen after the backend is known.

**First-underlay type differs from grow-underlay type.** A multi-underlay channel has **one
required underlay type** -- a fallback type that all communication can use -- so a channel is
functional with just that one underlay; every other type is optional (the channel's per-type `Min`
is 0 for them). The channel enforces this: `channelImpl` closes itself if the required type's
minimum isn't met (see Dependencies). So **the first underlay of a new instance must be the
required type**, not merely the largest deficit. Picking, say, an `ack` underlay first (legal under
an ack-heavy shape if we went by deficit alone) would create a channel with the required type
unmet, which the listener's channel then closes, showing up as churn or a phantom-unreachable
backend. Because one required underlay makes the channel valid, choosing it first means the channel
is valid **from its first underlay** -- there's no window where a half-formed channel sits invalid
waiting for a min-set to assemble. The reserve enforces this split: for a **new instance**
(`isFirst`) it reserves the **required type**; for a **grow** onto an existing, already-valid
instance it reserves the **largest outstanding per-type deficit** among the remaining (optional)
types. So fan-out fills toward the full shape (e.g. `2 payload + 1 ack`) starting from a valid
single-payload channel, never starving a type and never creating an invalid one.

The dialer keeps initiating LB dials while any advertised backend has an unmet per-type deficit,
up to `lb.maxConcurrentDials` outstanding dials at a time (default 2), and stops once every
advertised backend's full per-type shape is satisfied. It re-arms when a backend link drops, an
underlay degrades (any type falls below desired), or `PeerStateChange` adds a backend.

**Group-activation gate (fingerprints first).** Before fanning out, the group dialer checks the
RDM for fingerprints of its member backends (it knows the membership from `PeerStateChange`). It
**starts fan-out only once at least one member has a fingerprint**, and otherwise parks (re-armed
when the RDM delivers fingerprints), so it never dials into a total fingerprint void and spins on
fail-closed verifications. "At least one" is the right threshold because the dial is blind, you
can't wait for a *specific* backend, you just need *some* verifiable target to make progress; the
rest fill in via the missing-fingerprint retry above as their fingerprints arrive. This per-group,
targeted check is preferred over gating on a global "RDM fully synced" signal: a group activates
as soon as its first member is verifiable, even while the broader RDM is still catching up.

**Classifying each dial outcome.** Every dial is exactly one of seven outcomes. They differ along
a few axes: did a TCP connection establish, did it yield a verified router ID, (for a verified
in-pool router) do we have its fingerprint and does it still need underlay, and did the underlay
actually *form* once reserved. Getting the classification right matters because the wrong bucket
either penalizes the whole pool for one bad backend or lets a broken backend spin invisibly.

- **Progress** -- connected, verified, in-pool, and the backend still needed underlay; the
  connection was attached. Resets the consecutive-redundant counter and the bad-connection counter.
  (It also clears the pool-wide connection-failure backoff, but so does *any* TCP-established
  outcome -- see "The connection-failure backoff clears on any TCP-established outcome" below; it's
  not special to progress.)
- **Redundant** -- connected, verified, in-pool, but the backend's underlay is already satisfied;
  the connection is closed (see Underlay Across Backends). This is *not* a failure and never feeds
  any backoff, but it isn't free either (it's a billable LB connection), so it **increments the
  consecutive-redundant counter** that drives the missing-backend slow-poll below.
- **Connection failure** -- a failure *before a TCP connection is established*: DNS failure,
  connection refused, or connect timeout with no TCP session. No backend answered at all, so the
  LB itself is the problem, and this is the **only** outcome that drives **group-level (pool-wide)
  exponential backoff**, reusing the existing healthy/unhealthy backoff config (min/max interval,
  factor); which policy applies is governed by pool composition, see Healthy vs unhealthy pacing
  below. The TCP-establish boundary is the dividing line: once TCP connects, a failure is
  *connected-but-unverified* (or a later outcome), not this. Because it's the only signal of an
  LB-wide outage, it is **cleared by any TCP-established outcome** (see below), not just by progress.
- **Connected-but-unverified** -- a TCP connection established but **no verified router ID could be
  obtained, so the reserve never granted**: TLS handshake failure, or TLS succeeded but the peer
  presents no usable router certificate (the LB routed us to a non-router TLS service, so the hook
  can't even extract a backend router ID to reserve against). The defining trait is that we never
  got far enough to identify the backend; a failure that happens *after* a reserve grant is the
  separate *post-reserve formation failure* below, not this. This almost always means *one bad
  backend*, not an LB outage, so it must **not** drive the pool-wide connection-failure backoff,
  that would penalize the whole healthy pool. Instead it has its **own bad-connection counter and
  escalating backoff**, separate from both the pool backoff and the redundant counter, and it
  **raises an alert** because it signals a broken or misconfigured backend an operator needs to fix.
  Diagnostics are best-effort: because there's no verified router ID, and because behind an L4
  passthrough LB the dialer's socket peer is the LB's address rather than the backend's, we usually
  can't name the offending backend. We log what we have, the LB address dialed, the failure stage
  (TLS vs not-a-router-cert), and any partial peer info such as an untrusted certificate's subject,
  so the condition is visible and an operator can narrow it down rather than chase a silent failure.
- **Verification failure** -- connected and the handshake produced a *verified* router ID, but
  that router fails a Destination Verification check: not in the pool, fingerprint mismatch, the
  **post-response router-ID header cross-check mismatches**, or **the listener rejects the
  `LinkHeaderLoadBalanced` flag** (a non-LB or misconfigured listener refusing the LB role -- it
  isn't a valid LB destination). The connection is closed and an alert raised, and if a reservation
  was taken (the post-reserve cases: header mismatch and LB-flag rejection) it is **rolled back**,
  restoring the deficit. Unlike connected-but-unverified, we have a router ID to name in the alert.
  It counts toward the redundant counter for throttling purposes (a flood signals an LB pointed at a
  real-but-wrong router), and it **resets the bad-connection counter** -- it got far enough to
  produce a cert-derived router ID, which disproves the "can't even get an identity" hypothesis that
  counter tracks.
- **Missing fingerprint** -- connected and the handshake produced a **cert-derived router ID that
  matches a pool member**, but the dialer has *no* RDM fingerprint on record for it yet, so it
  **can't complete verification** (the identity is extracted and in-pool, not yet *verified* -- that
  distinction matters on a security-sensitive path). This is distinct from a *mismatch* (a bad/rogue
  backend, the verification-failure case): it's "verification not available yet," typically
  transient sync skew (LB membership arrives via `PeerStateChange`, fingerprints via the RDM, on
  separate channels). The connection is **closed
  and retried under a bounded group delay** (fail-closed), and it feeds **neither** the
  bad-connection counter **nor** the pool-wide backoff, it isn't a bad connection. Alerting is
  plausibility-gated: **silent** while it could be transient, but **alert** if a member's
  fingerprint stays missing past a threshold (an in-pool backend whose fingerprint never arrives is
  anomalous, the controller should have it). The group-activation gate below makes this rare in
  the first place.
- **Post-reserve formation failure** -- the dial got *past* reserve (a verified, in-pool backend ID
  and a granted reservation), but the underlay never reached commit for a reason that *isn't* a
  destination-validity failure: a hello Tx/Rx error, a post-hello setup problem, or attach/register
  failing. (Destination-validity failures after reserve -- header-cross-check mismatch, LB-flag
  rejection -- are *verification-failure* above, not this.) It **always rolls back the reservation**
  (restore the deficit, re-arm the backend) and is **attributed to the verified backend ID** -- we
  know exactly who, unlike connected-but-unverified. Because the backend is reachable and verified,
  it drives **neither** the pool-wide connection-failure backoff (not an LB outage) **nor** the
  connected-but-unverified bad-connection counter (that tracks "couldn't get a verified ID"; here we
  have one). Instead it re-arms the backend under a **bounded per-backend delay** (like missing-
  fingerprint), and alerting is **plausibility-gated**: silent while it looks transient, but alert if
  a backend keeps reserving-then-failing-to-form past a threshold (a backend that verifies but can
  never complete a hello is anomalous). This keeps a single flaky backend from either spinning hot
  or being mistaken for a pool outage.

**Staging the outcome.** Classifying a dial requires knowing *where* it failed, but a single LB
dial bundles DNS, TCP connect, TLS handshake, and channel hello into one operation that surfaces
one error. There are **two boundaries** the implementation must expose -- the **TCP-established
boundary** and the **reserve-granted boundary** (by staging the dial: TCP, then TLS, then the
hook's reserve, then the hello, or by classifying the error against these stages) -- and apply:
a failure with no TCP session is *connection failure* (pool-wide backoff); a failure after TCP but
*before a reserve grant* (TLS handshake failure, or a peer with no usable router cert) is
*connected-but-unverified* (the LB reached a backend it couldn't identify, that backend's problem,
not a pool outage); a failure *after a reserve grant* is either *verification-failure* (a
destination-validity failure: header mismatch or LB-flag rejection) or *post-reserve formation
failure* (everything else), both of which roll back the reservation. Treating TLS failures as
connected-but-unverified means a misconfigured LB that wrongly terminates TLS shows up as a rapid
run of connected-but-unverified across the pool, which trips the group-level bad-connection throttle
and alerts, rather than being mislabeled a pool outage.

**The connection-failure backoff clears on any TCP-established outcome.** The pool-wide
connection-failure backoff exists only to pace dialing while the *LB itself is unreachable*, and a
TCP session is the proof it's reachable again. So **every outcome except connection-failure clears
it** -- progress, redundant, connected-but-unverified, verification-failure, missing-fingerprint, and
post-reserve formation failure all crossed the TCP-established boundary, so any of them ends the
outage. Resetting only on *progress* (an earlier reading) would leave a recovered pool throttled by a
stale outage counter whenever no dial happens to land as progress -- e.g. every backend is already
satisfied (all redundant), or all backends are failing verification while the LB is perfectly
reachable. The outcome-specific counters (consecutive-redundant, bad-connection, missing-fingerprint
and per-backend delays) layer on top and are orthogonal: they pace *which backend gets dialed and how
often*, while the connection-failure backoff answers only *can we reach the LB at all*.

**One dial, one outcome (reserve aborts are non-retryable).** Each of these seven outcomes is the
result the registry loop counts and paces against, so a single LB dial must surface as **exactly
one** outcome to the loop, no more. There's a subtlety in the classic dialer that would break this:
on a failed hello it **retries internally** (it opens a fresh connection and re-sends the hello)
to negotiate a protocol-version downgrade. A reserve abort travels out of the hook as an error from
the `HelloHeaderProvider`, so a naive dialer would treat it as a retryable hello failure and open a
*second* LB connection, running a *second* reserve, before the loop ever sees the first, doubling a
billable connection and desyncing the outcome counters and backoff from the actual TCP attempts. So
**a provider/reserve abort is a non-retryable error**: the dialer returns it immediately rather than
re-dialing, and *only* genuine protocol-negotiation errors retry internally. Every reserve verdict
(redundant-close, verification-failure, missing-fingerprint, reserve-timeout) thus maps to one dial
and one loop outcome. (Implemented in channel PR #259 via a `NonRetryableError` wrapper that
`CreateWithHeaders` checks before retrying; a provider error is always wrapped non-retryable.)

**The bad-connection counter is group-level, and resets on any verified ID.** Connected-but-
unverified has no backend identity to attribute to (and behind an L4 LB the socket peer is the LB),
so the counter is per-LB-address (like the redundant counter): consecutive connected-but-unverified
outcomes drive an escalating backoff on the *group* dial rate plus an alert. It is reset by **any**
dial that yields a verified router ID -- progress, redundant, or even verification-failure -- since
all of those prove the TLS/hello path works and the LB can reach a good-enough backend; only
connected-but-unverified increments it. This bad-connection backoff is one more input to the group
dial cadence, alongside the connection-failure backoff, the redundant slow-poll, and the
healthy/unhealthy pacing; as elsewhere, the **longest applicable delay governs**.

**What "alert" means.** Wherever this document says a condition "raises an alert" (verification
failure, connected-but-unverified backend), the minimum is a structured WARN-level log carrying
the diagnostics named for that case (LB address, failure stage, router ID where one is available)
*plus* a metric counter, labeled by category so verification-failure and connected-but-unverified
are distinguishable in monitoring. Beyond that minimum, the router already has an alert mechanism
(`common/alert`), and these conditions should also emit through it. To avoid alert spam, **an
alert is emitted only the first time for a given source**: keyed by **`(LB address, router ID)`**
for verification failures, or by the LB address for connected-but-unverified failures (where no
router ID is available). The verification-failure key includes the LB address because the *same*
real router can be a valid member of one LB pool and a wrong backend for another -- keying on router
ID alone would suppress a distinct pool's misconfiguration after the first alert. Repeat occurrences
bump the metric but don't re-alert; the per-source alert state is cleared when the condition clears
for that source, so a fresh occurrence after recovery alerts again.

**The missing-backend case.** The dangerous scenario is when most backends are satisfied but
the LB keeps routing to them and never to the one still-unsatisfied backend (it's out of the
LB's rotation, a stale advertisement, etc.). Each such dial is a redundant close: neutral for
backoff, so without a guard the dialer would spin forever, paying per connection, while the
missing backend stays invisible. The consecutive-redundant counter is that guard. Once it
crosses `lb.redundantDialThreshold` (default: 2x the advertised backend count, floored at 4)
while a backend is still unsatisfied, the group concludes the LB isn't routing it to the
missing backend right now and drops from active fan-out to a **slow capped poll** at
`lb.pollInterval` (default 30s). It also logs a warning and emits a metric naming the
unreachable advertised backend, so the condition is visible rather than silently masked. The
counter and the slow-poll state reset on any progress, a backend link drop, or a fresh
`PeerStateChange` for the address (any of which means the situation may have changed and a
fresh round of fan-out is worthwhile).

All of `lb.maxConcurrentDials`, `lb.redundantDialThreshold`, `lb.pollInterval`, and
`lb.reserveTimeout` (the bounded reserve hand-off, default ~200ms; see The pre-hello reservation)
are tunable; the defaults above are starting points. The connection-failure backoff inherits the
existing healthy/unhealthy backoff configuration rather than introducing new knobs.

**Healthy vs unhealthy pacing.** A direct dest already chooses between a healthy and an unhealthy
backoff config based on whether the controller currently marks it healthy. The LB group reuses
the same two configs, selected by *pool composition* rather than a single destination:

- While **any pool member is healthy and still needs a link or more underlay**, the group uses
  the **healthy** backoff. This is the normal fan-out regime.
- Once **only unhealthy members remain needing links** (every healthy member is satisfied, or
  there are no healthy members), the group switches to the **unhealthy** backoff and keeps trying
  at that slower cadence, exactly as a direct unhealthy dest does. Unhealthy is a soft signal, the
  router may still accept links, so we keep trying, just slowly.
- When **every member that we'd dial for is satisfied**, the group is idle (no dialing).

An unhealthy member is therefore *not* dropped from the fan-out target, it's still pursued, just
at the unhealthy cadence when it's all that's left. A connection that lands on an unhealthy member
forms a link normally (it's progress). And an established link to a member that goes unhealthy is
left intact, only `PeerState_Removed` tears links down.

This composition-based backoff selection is a distinct dimension from the redundant-dial slow-poll
above. The backoff config paces retries after dial *failures*; the slow-poll throttles the case
where dials *succeed* but keep landing on already-satisfied backends while a healthy member goes
unreached (successful-redundant dials don't trip the failure backoff, which is why both exist).
Whichever produces the longer delay governs at a given moment.

### Reconnect Churn

The choice of LB algorithm affects how much redundant-dial churn reconnect produces.
Round-robin spreads new connections evenly, fine when building out the pool from scratch but
wasteful at reconnect when most backends already have full underlays (more redundant closes
before hitting the one that just dropped). A fewest-connections algorithm biases new
connections toward the backend that just lost its link (its count just dropped), which is
exactly where the dialer wants to land and which keeps the consecutive-redundant counter low.
Recommending fewest-connections in the operator docs is probably the right default.

### What Doesn't Change

- **Controller** - the controller *does* need targeted LB changes (this is not a zero-touch
  feature): a new `Listener` flag it relays, a `ControllerLoadBalancedLinks` capability it
  advertises, and per-recipient filtering that withholds LB listeners from routers lacking
  `RouterLoadBalancedLinks` (see Capability Handshake). It does *not* need new link-formation
  logic, circuit handling, or fingerprint plumbing, fingerprint distribution rides on the
  controller-managed router config work already in flight.
- **Link registry dedup** - works as-is for the dedup *mechanism*: LB-backed links compute the
  same key *format* as direct links, just later in the dial flow and with the reserved
  `loadbalanced` listener-binding component (see Link Key Construction), so the registry handles
  them through the normal path.
- **Listener config** - public routers configure listeners normally, just mark the LB listener
  with the `loadBalanced` field (see Advertising LB Addresses).
- **Link protocol/format** - once established, an LB-backed link looks identical to a direct
  link on the wire. Forwarding, heartbeating, and teardown all behave the same.

## Operational Notes

- **PROXY protocol / source IP visibility.** L4 LBs typically present the LB's IP as the
  TCP-level source rather than the original client. If source IP matters at the listener
  (logging, allowlists, metrics), the LB needs PROXY protocol v2 enabled and the router needs
  to parse it. The current design doesn't require this, but it's worth being aware of as a
  follow-up option.

- **Cost shape.** Charges typically scale with new connections, active connections, and bytes.
  The "keep dialing to fan out underlay" behavior will produce more new connections than a
  direct-dial design, modeling this against expected link counts and rotation rates is worth
  doing before rollout.

- **Health check coupling.** The LB's own health checks decide when to drain a backend. These
  are independent of ziti's link heartbeat, but generally agree, an unresponsive router will
  fail both. A misconfigured LB health check (wrong port, wrong protocol) could mask real
  failures or drain healthy routers, worth documenting recommended health check config.

## Implementation Notes

These aren't open design questions, they're known wrinkles to handle as the work lands.

**Two-level state lifecycle.** Today there's one `linkDest` per destination router, keyed by a
router ID known before dialing, and one `linkState` per advertised listener under it; each
`linkState` drives its own dial/retry loop against the advertised address (see
`linkRegistryImpl.destinations` and `linkDestUpdate.ApplyListenerChanges`). An LB pool doesn't
fit that shape: many routers advertise one address, the per-backend link key can't be computed
until after connect (see Link Key Construction), and we want one dial driver for the pool rather
than one per router. So LB state lives at two levels, both owned by the link registry and
processed in its existing `run()` loop (an `lbGroups` map alongside `destinations`, no new
goroutine, see Fan-out, Termination, and Backoff for the loop/pool split):

- **LB group**, keyed by `(dialerBinding, protocol, LB-address)`. (The listener binding is
  deliberately *not* part of the key, there's no point exposing multiple listener bindings
  through one LB, so a single group per address/protocol/dialer-binding is sufficient.) The
  group holds the membership set of advertised backend router IDs, the group dial state
  (cadence, connection-failure backoff, consecutive-redundant counter, slow-poll flag from
  Fan-out, Termination, and Backoff), and the map of per-backend link states.
- **Per-backend link state**, created *post-connect* and keyed by the discovered router ID once
  verification identifies the backend. It is **bookkeeping only and never independently dials**
  (see Fan-out, Termination, and Backoff): no `nextDial`/`dialFailed`/queue enqueue. When a
  backend link drops, the state marks that backend unsatisfied and re-arms the group dialer; it
  does not run its own retry and does not disturb the group or other backends. A drop retains the
  backend's link ID + next iteration (clearing only the live instance bookkeeping) so the re-form
  reuses the ID; the state is fully discarded only on member removal (see Fan-out, Termination, and
  Backoff). It may reuse the `linkState` struct's bookkeeping fields, with the dial-loop machinery
  left inert.

When a router advertises a *direct* (non-LB) listener, nothing changes, it follows the existing
per-destination `linkState` path. The fork is in `ApplyListenerChanges`: an LB-flagged listener
does **not** spawn a per-destination dialing `linkState`, instead it adds the backend to the
matching `lbGroup`'s membership; the normal per-destination dial path skips LB-flagged listeners.
So routers advertising the same LB address converge into one group rather than N independent dial
loops. This branch is the main seam in the otherwise-unchanged non-LB code; the bulk of the LB
logic lives in its own type invoked from the loop.

The lifecycle, event by event:

| Event | Group-level action | Per-backend action |
|-------|--------------------|--------------------|
| `UpdateLinkDest` / Healthy: router R advertises LB address A | ensure the group for A exists; add R to membership (or mark healthy); re-arm fan-out and use the healthy backoff if R is unsatisfied | none yet (no link until a dial reaches R) |
| `UpdateLinkDest` / Unhealthy: member R goes unhealthy | mark R unhealthy; keep R a fan-out target; if R is now the only unsatisfied member, pace dialing with the unhealthy backoff | keep any established link to R intact |
| `UpdateLinkDest` / Healthy: R advertises a direct listener | unchanged | existing per-destination path |
| group below target (a member lacks a satisfied underlay) | group dialer dials A, up to `lb.maxConcurrentDials` | on verified connect: attach / new link / redundant-close (see Underlay Across Backends) |
| dial reaches a verified, not-yet-linked backend R | reset redundant counter and connection-failure backoff | create R's link state; register the link |
| dial reaches a verified, already-satisfied backend R | increment consecutive-redundant counter; maybe enter slow-poll | close the connection |
| dial connection failure (LB unreachable) | group-level exponential backoff | none |
| backend R's link drops | mark R unsatisfied and re-arm fan-out for R; leave membership and other backends untouched | clear R's *live instance* bookkeeping (underlay refs, `ConnectionId`, in-flight reservations); **retain R's link ID + next iteration** for the re-form; **no independent per-backend dial** |
| a *direct* link to member R becomes established | mark R satisfied-by-direct; drop R from the fan-out target and close any existing LB link to R (direct supersedes LB) | close R's LB link if present, **and invalidate any in-flight LB reservation for R** so a dial mid-flight rolls back at commit instead of registering a parallel link |
| the direct link to member R is removed | mark R unsatisfied again and re-arm fan-out for R | none (re-acquired via the LB on next fan-out) |
| `PeerState_Removed` for R | drop R from membership; if R was the last member, tear the group down; otherwise keep the group and stop counting R toward the target | close R's link if present; **invalidate any in-flight LB reservation for R** (a dial mid-flight rolls back at commit); **discard its state including the retained link ID + iteration** (a later re-join starts a fresh link ID) |
| group reaper tick | reap the group iff membership is empty | (no backends remain) |

**`PeerState_Removed` and group teardown.** Removing one backend must not tear down the group
while other members remain, that's the key correctness rule above. The group is reaped only
when membership goes empty. There's no time-based safety net (unlike the 48-hour unhealthy
reaping for dead single routers), because membership is controller-authoritative: once no
router advertises the address, the group has no valid backends, and any connection the LB might
still return would fail the pool-membership check and be dropped anyway. So an empty group has
nothing worth keeping alive.

**Registry visibility.** Moving LB bookkeeping into `lbGroups` must not make LB links invisible to
the registry's reporting, sync, and inspection paths. The registry has two iteration bases, and
LB integrates with each differently:

- **Established-link maps** (`self.linkMap` by key, `self.linkByIdMap` by id) drive `Iter()`,
  `GetLink`, `Inspect`, `syncRequiredLinkStates`, and the fast path of `IsKnownLinkId`. An
  established LB link registers through the **normal accept path** into these maps, exactly like any
  other link, so all of these paths include LB links with no LB-specific code. This is a requirement
  on the LB accept path, not a change to the visibility paths.
- **`IsKnownLinkId`'s fall-through scan.** When a link ID isn't in `linkByIdMap`, `IsKnownLinkId`
  falls through to `scanForLinkIdEvent`, a loop-side scan that today walks only `destinations` ->
  `linkState.linkId` to recognize IDs that are *assigned but not yet established*. LB has two such
  windows that live in `lbGroups`, not `destinations`: an **in-flight** reserved ID (minted by the
  reserve before the underlay establishes) and a **retained-on-drop** ID (held across a drop for the
  re-form, see C4). So `scanForLinkIdEvent` must **also iterate `lbGroups`' per-backend states**.
  Without it `IsKnownLinkId` returns false in those windows, and the only caller, the link-metrics
  GC, prematurely disposes the LB link's metrics (a metrics gap, then recreation, not a traffic or
  fault bug, hence minor). Symmetric to the `notifyControllersOfLinks` seam below.
- **Per-destination state** (`self.destinations` -> `dest.linkMap` -> `linkState`) drives
  `notifyControllersOfLinks` (which builds *both* `RouterLinks` and fault reports from
  `state.ctrlsNotified`/`linkFaults`/`status`). LB per-backend states live in `lbGroups`, not
  `destinations`, so `notifyControllersOfLinks` must **also iterate `lbGroups`' per-backend
  states** (they carry the same `ctrlsNotified`/`linkFaults`/`status` fields) to report LB links
  and faults. This is the one genuine seam; without it LB links would carry traffic while being
  silently absent from `RouterLinks` and fault reporting.
- **Lifecycle status/fault writes (`updateLinkStatusForLink`).** This is the same seam on the
  *write* side, and it's the one easiest to miss. `applyLink` (registration) and `LinkClosed` both
  funnel into `updateLinkStatusForLink`, which today resolves state via
  `registry.destinations[link.DestinationId()]` and **returns early** (with a misleading
  "destination not present" warning) when the dest isn't there. For an LB link the backend's state
  is in `lbGroups`, so this path no-ops: the link sits in `linkMap` while group satisfaction,
  `ctrlsNotified`, re-arm, and pending faults silently never update. So **every shared link-state
  lookup/update helper must resolve the link in `destinations` *or* `lbGroups`** -- not just the
  reporting scans, but registration, the established/closed/failed status writes, and the
  duplicate-replacement fault path. For an LB-resolved state the transitions apply with LB
  semantics: **established** is owned by the reserve **commit** (the shared established-status write
  *defers* for LB, so the state isn't double-marked); **failed/closed** records the pending fault,
  marks the backend unsatisfied, and **re-arms the group dialer** rather than setting `nextDial` on a
  self-dialing `linkState`; duplicate-replacement faults are recorded on the LB per-backend state.
  Replace the `destinations`-only early-return with the both-maps lookup so LB links can't register
  in `linkMap` while their group state goes stale.

`evaluateDestinations` stays LB-free, the LB-group logic owns LB lifecycle and reaping (see C4),
so the per-destination eval/reap path is not muddied with LB states. The choice to keep LB
per-backend states in `lbGroups` (rather than in `destinations[backend].linkMap`) is deliberate:
it keeps LB out of the dial/eval/reap path. But the seam is **not reporting-only** -- the shared
status/fault *write* helpers above must be `lbGroups`-aware too, or LB links go stale silently;
that's the lookup rule, applied consistently across reporting *and* lifecycle writes.

**Inspect must also surface LB *group* state.** Established LB links show up in `Inspect` via the
established-link maps above, but the LB-specific bookkeeping, group membership, which backends are
unsatisfied, and the group dial state (slow-poll flag, backoff, redundant/bad-connection counters),
lives only in `lbGroups` and would otherwise be invisible to operators diagnosing a pool. `Inspect`
should add an LB-group section reporting, per LB address: the membership set, each backend's
satisfaction/underlay status, and the group's current dial/backoff state. This is what makes a
misbehaving pool (e.g. a backend the LB never routes to, sitting in slow-poll) diagnosable.

**Direct-link collision.** An LB link and a direct link can both target the same router B.
Because an LB link keys with the reserved `loadbalanced` binding while a direct link keys with
B's real binding (see Link Key Construction), the two have *different* keys and the registry
will not collapse them. That is deliberate, but it means the collision has to be prevented in the
fan-out logic rather than relied on to dedup away. The single rule that covers every ordering:

> **A direct link to an LB-group member satisfies that member's LB link requirement.**

Because the link registry owns both the direct `destinations`/links and the `lbGroups` (see
Two-level state lifecycle), it can apply this rule locally as links come and go. The three cases:

- **Direct-first** (direct link to B already exists, then an LB dial reaches B). B is already
  satisfied, so the group never fans out an LB link to B; if an LB connection nonetheless lands
  on B, it's a redundant close (and B stays excluded from the fan-out target). No second link.
- **LB-first** (an LB link to B exists, then a direct dial to B succeeds). When the direct link
  to B becomes established, the registry marks B satisfied-by-direct and **closes the now-redundant
  LB link to B**, dropping B from the LB fan-out target. Direct supersedes LB, consistent with the
  dialer-preference principle (the LB path is a fallback). This avoids leaving two parallel links
  to B. **The in-flight case is covered too:** if an LB dial to B is mid-flight (reserved but not
  yet committed) when the direct link establishes, closing the established LB link isn't enough --
  there's no LB link yet, only a reservation. So the direct-establish handler also **invalidates
  B's in-flight LB reservation**, and commit **revalidates** (member, not-satisfied-by-direct,
  deficit still reserved) before registering, so the mid-flight dial rolls back at commit instead of
  registering a parallel link (see The pre-hello reservation). Without both, the LB commit could
  land *after* the direct-establish handler ran and find nothing to supersede.
- **Direct drops** (the direct link to B goes away while B is still an LB member). B becomes
  unsatisfied again, so the LB group re-arms and reaches B via the LB on the next fan-out.

To avoid churn from a flapping direct link, the registry acts on the *established* and *removed*
transitions it already tracks for the satisfied-by-direct decision; a brief window where both
*established* links to B exist during the handover is harmless and self-correcting. The one
in-progress case it must act on is the in-flight LB *reservation* (above): a direct establish or a
member removal invalidates it so the pending commit rolls back, since an uncommitted reservation
isn't an established link the transition logic would otherwise see. Suppression keys on an
**established** direct link, never on a mere advertisement or a pending/failed direct attempt: only
an established direct link to B satisfies B's LB target, and LB fallback arms after one failed
direct-first attempt (see Dialer preference), so an unreachable direct address can't starve B. The
**primary** control is link groups (keep direct and LB audiences disjoint so a router sees one path
to B); this rule is the backstop for overlapping-group/ordering edge cases. All three orderings,
plus the two mid-flight races (member-removed and direct-established between reserve and commit),
should be covered by focused tests (see Work Items).

## Settled Decisions

**The LB flag is a `Listener` field, not part of the address.** See Address Representation for the
full rationale (the transport address parser rejects query parameters, and the address is used as
an identity for grouping/dedup). The advertise address stays a clean `tls:host:port`; the
`loadBalanced: true` config field sets a boolean on the `Listener` message. If the
`?lb=true` query-parameter *config* syntax is wanted for ergonomics, the router parses it locally
and sets the field; it never travels in the wire address.

**Distinct LB flag rather than a unified pool flag.** We considered a single shared flag with a
`mode=lb|anycast` parameter, since LB and anycast pools share the same identity-verification
machinery. We're going with a distinct LB flag instead. The dialer behavior differs substantially
between the two (LB fans out and round-robins; anycast dials once), so a distinct flag lets each
path be reasoned about and code-branched cleanly. This is settled to keep it from churning through
the codebase mid-implementation.

## Work Items

A number of these overlap directly with anycast and only need to be done once.

- [ ] **Prerequisite:** migrate the router-to-router link channel to `channel/v5` (the `DialPolicy` self-grow control, `AcceptUnderlay` attach point, and the hello-header hook this design relies on are v5 APIs; see Underlay Across Backends)
- [ ] **Prerequisite:** the `channel/v5` hello-header hook that lets the dialer compute hello *headers* from the peer certificate after the TLS handshake and before the hello is sent (openziti/channel #258, PR #259, headers-only). The hook must merge the provider's headers into the **effective header set used for *both* the outbound hello *and* the dialer's own underlay initialization** (`underlay.init`), so a provider-set `ConnectionId` *and* underlay `type` drive the dialer's local grouping/type, not just what the listener receives; otherwise the two ends group the link's underlays by different values. (Implemented in PR #259: `sendHello` reads `connectionId` and `type` from the merged `request.Headers`, not the pre-provider `headers` arg; covered by a test asserting a provider-set `ConnectionId` and `type` reach the local underlay.) A `HelloHeaderProvider` error must be **non-retryable** so a reserve abort doesn't trigger the dialer's internal hello-retry and silently open a second LB connection; only protocol-negotiation errors retry internally (implemented in PR #259 via a `NonRetryableError` wrapper that `CreateWithHeaders` checks; tested that a provider error aborts with the provider invoked exactly once). LB uses the hook post-verification to stamp the per-instance `ConnectionId`, the link ID (`LinkHeaderLinkId`), the iteration, the underlay type, and the grouped-underlay headers (`IsGroupedHeader`, `IsFirstGroupConnection`, per-instance `GroupSecretHeader`) into the hello
- [ ] **Prerequisite:** the `LinkHeaderLinkId` header (already landed via openziti/ziti #3938, `LinkHeaderLinkId = 100`) so the link ID can ride in a header rather than the channel-identity token; the listener reads the link ID from `LinkHeaderLinkId` if present, else falls back to `Channel.Id()`. LB builds on #3938 (emit-on-dial + prefer-on-read are already there); LB always emits it (the hook can't set the token). The `anycast-support`/LB branch must include #3938
- [ ] **Prerequisite:** RDM router fingerprint distribution must have landed before LB is enabled (the LB verification path requires it). Group-activation gate: the group dialer checks the RDM for its member backends' fingerprints (membership known from `PeerStateChange`) and starts fan-out only once *at least one* member has a fingerprint, otherwise it parks and re-arms on RDM fingerprint delivery (avoids spinning on fail-closed dials before the fingerprint view is usable). Preferred over gating on a global "RDM fully synced" signal
- [ ] **Prerequisite:** the `UpdateLinkListeners` runtime listener-republish mechanism (`Router.publishLinkListeners` + the controller's `UpdateLinkListeners` handler) from the controller-managed-router-config work, LB uses it to advertise LB listeners post-handshake (the connect hello can't carry them; see Capability Handshake)
- [ ] Listener sends router ID in hello headers *(shared with anycast)*
- [ ] Dialer extracts listener router ID from TLS cert after connect *(shared with anycast)*
- [ ] Dialer cross-checks cert router ID against hello header router ID *(shared with anycast)*
- [ ] **Phase 1 (do first; valuable on its own, LB/anycast build on it):** fingerprint-based router verification, both directions, against the RDM-distributed fingerprint set:
  - [ ] **Listener side** -- match the dialer's presented cert fingerprint against the local RDM fingerprint for its claimed router ID, replacing the `verifyRouter()` RPC. Gated only on local fingerprint availability (no peer capability); fall back to the `verifyRouter()` RPC while a fingerprint isn't available; retire the RPC once distribution is guaranteed
  - [ ] **Dialer side (new)** -- the cert-derived checks (cert ID extracted, fingerprint matches the RDM fingerprint for that ID, destination match) are *computable* post-TLS/pre-hello, plus a **post-response cross-check** (the listener's router-ID hello header matches the verified cert ID). **Enforcement timing differs by dial type:** for **LB** the cert-derived set *is* the `reserveLbUnderlay` admission decision, enforced **pre-hello** in the hook (aborts before the hello, fail-closed, no capability gate); for **direct** the enforce-or-fall-back decision is **deferred to the hello response** because the gating `RouterLinkDestVerification` capability only arrives in the listener's response `CapabilitiesHeader` (capability present + fingerprint available → verify strictly; absent → no destination check, today's behavior; never enforce pre-hello on the direct path). Fallback is *not* the listener-side `verifyRouter()` RPC
  - [ ] Add a `RouterLinkDestVerification` link capability (consistent with `common/capabilities` / `CapabilitiesHeader`), advertised by a router that sends the router-ID header and participates in destination verification; gates the dialer-side check on direct dials (not the listener-side replacement)
- [ ] **Phase 2 (LB):** dialer-side destination match is pool-membership (verified ID must advertise this LB address in `PeerStateChange`); LB dials are always strict + fail-closed (close + alert on a non-member or missing fingerprint) *(shared with anycast: anycast uses the same Phase 1 dialer-side check with its own destination-match rule)*
- [ ] Add a `loadBalanced` boolean field to the `Listener` protobuf and the router listener config; the advertise address stays a clean `tls:host:port`. Optionally accept `?lb=true` config sugar parsed locally into the field. Do *not* embed the flag in the wire address
- [ ] Add the two LB capabilities on their **asymmetric** existing transports: `ControllerLoadBalancedLinks` (controller->router) as a new bit in the `common/capabilities` bitmask (controller hello `CapabilitiesHeader`); `RouterLoadBalancedLinks` (router->controller) as a new `ctrl_pb.RouterCapability` enum value sent in `RouterMetadata` via `RouterMetadataHeader` (**not** a `common/capabilities` bit). Wire the controller's parse/record path for the new `RouterCapability` and the router's read of the controller bitmask. Test: a router lacking `RouterLoadBalancedLinks` in its metadata never receives an LB listener in `PeerStateChange`; a router withholds its LB listener from a controller lacking `ControllerLoadBalancedLinks`
- [ ] Controller filters LB listeners out of `PeerStateChange` for routers lacking `RouterLoadBalancedLinks`
- [ ] Router advertises LB listeners per the advertisement sequence: exclude LB listeners from the initial connect hello snapshot (the hello still carries non-LB listeners), and after observing `ControllerLoadBalancedLinks` publish them via `UpdateLinkListeners` to capable controllers only. Requires extending `publishLinkListeners` to filter the published set per recipient controller (LB listeners only to capable ones) instead of sending one set to all. Per-controller gating is sufficient (legacy link-key mode is gone, #3923, so keys are always binding-preserving); no all-or-nothing gate or capability-loss teardown, existing LB links survive connecting to an older controller. Tests: (a) first connection to a mix of LB-capable and old controllers, the old controller never receives the LB listener (hello-excluded and not sent via `UpdateLinkListeners`); (b) existing LB links survive connecting to an older controller and keys stay binding-preserving
- [ ] Controller coalesces the connect snapshot: hold the initial post-connect peer notification until the router's `UpdateLinkListeners` arrives or a configurable interval (~5s default) elapses, then redistribute once, so a control-channel reconnect doesn't flap established LB links via an intermediate LB-absent snapshot. Arm the hold only when the hello sets the **LB-listeners-pending** flag (set by a router that withheld an LB listener) and the controller is LB-capable; routers that withheld nothing (incl. dialer-only LB participants) and old controllers notify immediately. Notify on the earlier of update-received or interval-elapsed. Test: a reconnecting router with established LB links produces a single combined peer notification, no orphan-reap/re-form flap; a dialer-only LB router notifies immediately
- [ ] Document the upgrade-window limitation: while controllers have mixed LB capability, peers connected to both get divergent `PeerStateChange` reports for an LB backend and the LB link may flap; it's a valid link whenever formed and self-heals once all controllers are upgraded, so it's documented (upgrade promptly), not coded around
- [ ] Enforce at most one LB listener per router at config validation; advertise that listener with the reserved `loadbalanced` binding (regardless of local interface) and use the same value when the listener computes the inbound LB link key, so dialer and listener keys match
- [ ] Two-level LB state owned by the link registry and processed in its existing `run()` loop (an `lbGroups` map alongside `destinations`, no new goroutine; group re-dials ride the existing `linkStateQueue`; network dials + verification go on the existing dial pool; dial outcomes return to the loop as events). An LB group keyed by `(dialerBinding, protocol, LB-address)` holds membership + group dial state; per-backend link states are bookkeeping created post-connect. `ApplyListenerChanges` forks: LB-flagged listeners join the group instead of spawning a per-destination `linkState`; direct listeners keep the existing path. Factor the LB logic into its own type called from the loop
- [ ] Pre-hello reservation protocol: a synchronous `reserveLbUnderlay(backendId, certFingerprint)` registry-loop event the hook calls post-TLS, returns accept (+ minted/reused link ID, iteration, per-instance `ConnectionId`, underlay type, marking an in-flight reservation that decrements the per-type deficit) or close / verification-failure / missing-fingerprint; plus async commit (underlay established) / rollback (dial or hello failed, release the reservation). Keeps all group-state mutation on the loop and serializes deficit accounting so concurrent dials to one (backend, type) can't overfill. Test: two concurrent dials to the same backend/type, one reserves, the other gets close-redundant
- [ ] Bound the reserve hand-off so it can't pin a dial-pool worker: the hook enqueues and waits under a single `lb.reserveTimeout` deadline (default ~200ms) spanning *both* the enqueue onto the bounded `events` channel (`queueEvent` blocks when full) *and* the verdict wait; on either timeout, abort the dial fail-closed (close conn, free the worker), group re-arm/backoff retries later. Add a `queueEventWithDeadline` variant for the enqueue. Test: a stalled/backlogged registry loop makes an LB reserve time out and abort fail-closed rather than blocking the dial pool
- [ ] Lease in-flight reservations so an abandoned reserve self-heals: stamp each reservation with a deadline on grant; commit/rollback clears it; the existing `queueCheckTicker` (5s) reaps any reservation past its lease that never committed and restores the deficit (covers both a timed-out hook that walked away from the buffered reply and a lost rollback). Test: a granted reservation with no commit/rollback is reclaimed by the reaper and the backend's deficit is restored
- [ ] One dial, one outcome: a reserve abort must surface as exactly one LB dial outcome to the loop, never trigger the classic dialer's internal hello-retry (which would open a second connection + run a second reserve). Relies on the channel-side `NonRetryableError` contract (hook prerequisite above); the LB path must propagate the abort as a provider error, not a retryable hello failure. Test: a reserve abort (redundant/verification-failure/missing-fingerprint/timeout) produces one TCP connection and one loop outcome, no silent re-dial
- [ ] Commit is the last step, gated on post-response verification: an LB reservation commits (in-flight -> established) only after the post-response router-ID header cross-check passes *and* the underlay attaches/registers; a header-cross-check mismatch (or attach/register failure) is a rollback (close underlay, restore the reserved deficit, re-arm), never a commit. Don't commit on hello-success before the cross-check. Test: a post-response header mismatch rolls back the reservation, the backend's deficit is restored and fan-out re-arms (no phantom-satisfied backend, no reliance on the lease reaper for this path)
- [ ] LB group lifecycle: add/remove members on `UpdateLinkDest`/`PeerState_Removed`, keep the group alive while any member remains, reap only on empty membership (no time-based net), and clean up a dropped backend link without disturbing the group
- [ ] LB per-backend state is bookkeeping only and never independently dials: not enqueued on the dial queue, no `dialFailed`/backoff; a backend drop marks it unsatisfied and re-arms the registry's LB-group logic, which is the sole dial driver for the pool (direct destinations keep their per-destination dial loop). On a link drop, **retain the backend's link ID + next iteration** and clear only the live instance bookkeeping (underlay refs, `ConnectionId`, in-flight reservations), so the re-form reuses the link ID with `iteration + 1`; discard the retained identity only on member removal (`PeerState_Removed` / address no longer advertised). State is in-memory only, so a restart yields a fresh link ID. Test: a backend link drop then re-form reuses the link ID with a higher iteration; member removal then re-join yields a fresh link ID
- [ ] Registry visibility for LB links: established LB links register through the normal accept path into `self.linkMap`/`linkByIdMap` (so `Iter`/`GetLink`/`Inspect`/`syncRequiredLinkStates` and the fast path of `IsKnownLinkId` include them with no LB-specific code); `notifyControllersOfLinks` additionally iterates `lbGroups`' per-backend states to emit `RouterLinks` and faults; and `scanForLinkIdEvent` (the `IsKnownLinkId` fall-through) additionally iterates `lbGroups` so in-flight/retained-on-drop LB link IDs aren't reported unknown (else the link-metrics GC prematurely disposes their metrics). `evaluateDestinations` stays LB-free. Test: an in-flight (reserved-not-established) and a retained-on-drop LB link ID are both recognized by `IsKnownLinkId`
- [ ] Make the shared link-state lifecycle helpers `lbGroups`-aware, not just the reporting scans: `updateLinkStatusForLink` (and the `addLinkFaultForReplacedLink` path) resolve the link in `destinations` **or** `lbGroups` instead of returning early when the dest isn't a direct destination. LB-resolved transitions use LB semantics: established is owned by the reserve commit (shared established-status write defers for LB, no double-mark); failed/closed records the pending fault, marks the backend unsatisfied, and re-arms the group dialer (not `nextDial`); replace the `destinations`-only early-return + "destination not present" warning with the both-maps lookup. Tests: an LB link establishing updates group satisfaction + `ctrlsNotified` + triggers notify; an LB link closing records a fault, re-arms fan-out, and reports the fault to controllers; no spurious "destination not present" warning for LB links
- [ ] Enforce both concurrency limits: `lb.maxConcurrentDials` is a per-group in-flight cap enforced by the group logic; the existing dial pool remains the global bound. Route LB dial outcomes back to the run loop as events so group state stays single-threaded
- [ ] LB group health pacing: select the healthy vs unhealthy backoff config by pool composition (healthy while any healthy member needs fanout, unhealthy once only unhealthy members remain needing links, idle when all satisfied); keep unhealthy members as fan-out targets and leave their established links intact (only `PeerState_Removed` tears links down)
- [ ] Refactor the multi-underlay dial path to construct the link object *after* the initial dial and verification, rather than before (the current pre-connect construction is a holdover from the split dialer; multi-underlay establishes from a single dial and doesn't need it)
- [ ] Add the `LinkHeaderLoadBalanced` dial flag: dialer sets it on LB dials and populates `LinkDialedRouterId` with the dialed LB address (diagnostic only). On an **LB-configured listener only**, skip the `LinkDialedRouterId` match when the flag is set and proceed to normal dialer identity verification; a non-LB listener must **reject** the flag (the `ConnectionHandler` runs the check before bind handling, so don't loosen it globally). **Allocate `LinkHeaderLoadBalanced` (and any new LB link header) at a key ≥100** per the `factory.go` rule from #3938, so it can't collide with the channel library's grouping/type headers overlaid on the same hello map (see Header allocation)
- [ ] Post-connect link key/state creation for LB dials
- [ ] LB link identity carried in the post-TLS hello (via the hello-header hook), not derived: the dialer mints the link ID (`uuid.New()`) for a new instance, reuses it for a re-form, and stamps it in **`LinkHeaderLinkId`** plus a per-link `iteration` in `LinkHeaderIteration`, the per-backend `ConnectionIdHeader`, and `LinkHeaderRouterId` (dialer's real router id; the hello token stays the dialer's router identity, no override). The listener reads the link ID from `LinkHeaderLinkId` if present, else `Channel.Id()`. Both ends agree, so bilateral faulting works and the controller's iteration-supersede handles stale faults. Per-link, like a direct link: stable ID across reconnects, iteration++ on re-form, fresh minted IDs on restart. No `DeriveLbLinkId`, no namespace/encoding, no group epoch, no group iteration. Direct links unchanged. Tests: two backends behind one pool get distinct IDs; a re-form reuses the ID with a higher iteration; a restart yields fresh IDs so pre-restart faults don't match; either end can fault a current-iteration link
- [ ] Fan-out dialing with per-backend, per-underlay-type deficits: keep dialing the LB until every known backend has its full per-type underlay shape (e.g. 2 payload + 1 ack), not just a nonzero total. Choose the underlay **type post-TLS** (via the hello hook) and stamp it into the hello; a backend with no deficit of any type is the redundant case. **First-underlay vs grow:** a new instance (`isFirst`) reserves the channel's **required (fallback) type** so the created channel is valid from its first underlay; a grow onto an already-valid instance reserves the largest outstanding per-type deficit among the optional types. Never pick an optional type (e.g. `ack`) first, or the listener's channel closes for an unmet required-type minimum. Tests: a 2 payload + 1 ack shape is satisfied across backends without starving the ack type; an **ack-heavy** shape still dials the required type first and the channel doesn't close on creation
- [ ] Disable channel self-grow for LB links (run the multi-underlay channel with a nil `channel/v5` `DialPolicy` so it enforces min/desired constraints for validity but never dials underlays itself); make the policy a per-link choice so direct links keep the default self-growing policy
- [ ] Group-level LB dialer drives all underlay dialing and attaches each verified connection to the matching backend's channel via `AcceptUnderlay`, starts a new link for a new backend, or closes a redundant connection
- [ ] LB connection grouping: the dialer sets a **per-instance `ConnectionId`** in the post-TLS hello (a fresh token minted when a link instance forms, stored in the per-backend state, reused for that instance's grow-underlays, rotated on re-form), so the listener's ordinary `ConnectionId`-keyed create-or-join groups each instance's underlays separately and a re-form never lands on a stale listener channel. Per-instance (not a deterministic per-backend value) is what avoids the `MultiListener` stale-channel collision on fast re-form/restart; it stays distinct from the link ID (stable across re-forms). No pool-shared `ConnectionId`/secret and no listener-side cert-owner-binding change (the per-instance id removes the injection vector). Test: concurrent LB dials landing on different backends form separate links; two dialer bindings to one backend don't collapse; an immediate re-form gets a new `ConnectionId` and doesn't attach to the old channel
- [ ] Stamp the full grouped-underlay header set the multi-underlay listener requires (not just `ConnectionId`): `IsGroupedHeader=true` (else `MultiListener` routes to the ungrouped fallback), `IsFirstGroupConnection=true` on the first underlay of a new instance only (else `MultiListener` closes an underlay with no existing channel; grow-underlays leave it unset), and a per-instance `GroupSecretHeader` minted with the instance, stored next to the `ConnectionId`, reused for grows, and rotated on re-form (`channelImpl.AcceptUnderlay` rejects a grow whose secret doesn't match). The reserve response carries `isFirst` and `groupSecret`; the group dialer issues a grow reservation only after the instance's first underlay commits. Tests: first underlay creates the channel; a grow-underlay with the matching secret joins; a grow with a wrong/absent secret is rejected; an underlay without `IsGroupedHeader` hits the ungrouped fallback
- [ ] Close excess connections when the targeted backend's per-type underlay quota is already met
- [ ] Pool-aware fan-out and backoff: classify each dial outcome (the seven of the classification section: progress / redundant / connection-failure (TCP/DNS) / connected-but-unverified / verification-failure / missing-fingerprint / post-reserve-formation-failure); pool-wide exponential backoff **only** on TCP/DNS connection failure, and **cleared by any TCP-established outcome** (not just progress), since TCP reachability ends the outage; consecutive-redundant-dial guard that drops to a slow capped poll and warns on an unreachable advertised backend; per-backend link failures don't back off the whole pool. New tunables `lb.maxConcurrentDials` (default 2), `lb.redundantDialThreshold` (default 2x backend count, floor 4), `lb.pollInterval` (default 30s). Test: a pool in connection-failure backoff is released by a single redundant (or any TCP-established) outcome, not only by progress
- [ ] Staged dial-outcome classification: surface the TCP-established boundary (stage the dial or classify the transport error) so a no-TCP failure is connection-failure (pool-wide backoff) while any post-TCP failure, including TLS, is connected-but-unverified
- [ ] Connected-but-unverified handling: a post-TCP failure that yields **no verified router ID and so never reaches reserve** (TLS failure, or TLS succeeded but the peer presents no usable router cert) uses a group-level bad-connection counter + escalating backoff (not the pool-wide backoff, not the redundant counter), raises an alert, and logs best-effort diagnostics (LB address, failure stage, partial peer info). The counter resets on any verified-ID outcome (progress / redundant / verification-failure / missing-fingerprint / post-reserve-formation-failure); only connected-but-unverified increments it. Composes with the other dial-rate inputs by longest-delay-wins. Test: repeated connected-but-unverified throttles the group (not the pool) and alerts; a single verified-ID dial resets it
- [ ] Post-reserve formation failure handling: a dial that passed reserve (verified, in-pool, reservation taken) but failed to form before commit for a non-destination-validity reason (hello Tx/Rx error, post-hello setup, attach/register failure) is its own outcome: roll back the reservation (restore deficit, re-arm), attribute to the verified backend ID, drive **neither** the pool-wide backoff **nor** the bad-connection counter, re-arm under a bounded per-backend delay, and alert only if a backend reserves-then-fails-to-form past a threshold. A listener rejecting `LinkHeaderLoadBalanced` is *verification-failure* instead (destination-validity), also rolling back the reservation. Tests: a non-LB listener rejecting the LB flag rolls back + alerts as verification-failure; an attach/register failure after reserve rolls back, re-arms, and doesn't trip the pool backoff or bad-connection counter
- [ ] Missing-fingerprint handling: a verified, in-pool router with no RDM fingerprint yet is a distinct outcome from a mismatch, close + bounded-delay retry (not the bad-connection counter, not pool backoff); alert only if a member's fingerprint stays missing past a threshold (silent while plausibly transient sync skew). Test: missing-then-arriving fingerprint forms the link without alerting; a persistently-missing in-pool fingerprint alerts
- [ ] Alerting: emit verification-failure and connected-but-unverified alerts as a structured WARN log + a category-labeled metric (minimum), and also through the existing `common/alert` mechanism. Dedup per source (`(LB address, router ID)` for verification failures so the same router being wrong for one pool doesn't suppress a distinct pool's alert; LB address for connected-but-unverified), emitting only on the first occurrence and re-arming once the condition clears for that source
- [ ] Dialer prefers direct listeners over LB for the same backend, with the precise suppression/fallback rule: **only an established direct link** to B suppresses LB fan-out for B (not a mere advertisement, nor a pending/dialing/backoff-failed direct `linkState`); LB fallback arms for B after **one failed direct-first attempt**, so an unreachable direct address doesn't starve B and a reachable one doesn't steady-state double-dial. Link groups are the primary control (keep audiences disjoint); this is the backstop. Tests: unreachable direct address -> B linked via LB after one direct attempt; a pending/retrying direct dial does not block LB fallback; an established direct link suppresses/closes LB
- [ ] Apply the rule "a direct link to an LB-group member satisfies its LB requirement" in the registry across all orderings: direct-first (don't fan out LB to it / redundant-close), LB-first (close the redundant LB link when a direct link establishes), and direct-drop (re-arm LB fan-out); act on established/removed transitions to avoid flap churn
- [ ] Reserve/commit race handling for group changes during the async dial window: commit revalidates (reservation still exists, backend still a member, not satisfied-by-direct, deficit still reserved) and rolls back (close underlay, restore deficit) instead of registering if any check fails; the direct-establish and `PeerState_Removed` handlers invalidate any in-flight LB reservation for that backend so a mid-flight dial rolls back at commit. Covers the case registry dedup can't (direct vs LB keys differ). Tests: (a) a member removed between reserve and commit rolls back, no orphan LB link; (b) a direct link established between reserve and commit rolls back the LB dial, no parallel link
- [ ] Reconnect logic: re-dial LB address, accept whichever backend the LB returns
- [ ] Warn when multiple routers share an address without the LB flag (or the anycast flag) set
- [ ] Document recommended LB configuration (L4 TCP passthrough, fewest-connections, idle timeout, health checks)
- [ ] Tests for the LB dial happy path: dial the LB, reach an in-pool backend, all verification checks pass, link forms; reaching a not-yet-linked backend starts a new link; reaching a satisfied backend is a redundant close
- [ ] Add an LB-group section to `Inspect` reporting, per LB address: membership set, each backend's satisfaction/underlay status, and the group's dial/backoff state (slow-poll flag, backoff, redundant/bad-connection counters), so a misbehaving pool is diagnosable (group state lives only in `lbGroups`, not the established-link maps)
- [ ] Tests for LB link visibility: an established LB link appears in `RouterLinks`, emits multi-underlay `LinkStateUpdate`, reports faults/closure to the controller, shows in `Inspect` output, is recognized by `IsKnownLinkId`, and LB group state (membership + per-backend + backoff) appears in `Inspect`
- [ ] Tests for LB verification failures (the security boundary, named negative cases):
  - [ ] verified router ID not in the pool membership -> close + alert
  - [ ] fingerprint missing for the reached router on an LB dial -> fail-closed (close + retry), not accepted
  - [ ] fingerprint mismatch (cert fingerprint != RDM fingerprint for that ID) -> close + alert
  - [ ] hello-header router ID != certificate router ID -> close (misconfig/MITM)
  - [ ] connected-but-unverified (TLS handshake failure / peer is not a link listener) -> bad-connection counter + alert, and *no* pool-wide backoff
- [ ] Tests for verification capability gating: a direct dial to a peer not advertising `RouterLinkDestVerification` (or with no local fingerprint) does **no dialer-side destination check** (today's behavior), not rejection; the listener still authenticates the dialer (locally if its fingerprint is available, else via the `verifyRouter()` RPC); an LB dial is always strict + fail-closed
- [ ] Tests for fan-out to multiple backends
- [ ] Tests for reconnect-to-different-backend scenario
- [ ] Tests for excess-connection close on quota-met
- [ ] Tests for direct-vs-LB collision, both orderings: (a) direct-first, the group does not fan out an LB link to a router already reachable directly and an LB connection that lands on it is redundant-closed; (b) LB-first, when a direct link to a backend that already has an LB link establishes, the LB link is closed and not left as a parallel link; and (c) when that direct link later drops, LB fan-out re-arms for the backend
