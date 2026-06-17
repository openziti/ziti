# Load Balancer Support for Router Links: Summary

This is a high-level overview of the [Load Balancer Support for Router Links](lb-routers.md)
design. It assumes you know how Ziti links and routers work and you want the shape of the feature
without the implementation detail. The full doc has all the rationale, edge cases, and work items;
this one is for getting the gist.

## The Problem

You have a pool of public routers fronting a set of private routers, and you want to hand the
private routers a single stable address rather than a list of every public router. Add or remove
public routers and nobody has to update firewall allow-lists or router configs.

[Anycast](anycast.md) solves the same problem, but anycast means cooperating with BGP and dealing
with mid-connection redirects and geographic pools. A load balancer is the boring, familiar
alternative: customers already run them, and stock cloud LBs (AWS NLB, GCP TCP LB, HAProxy in TCP
mode) give you health checking and graceful drain for free.

The trade-off is cost. All link traffic flows through the LB, so you pay per-connection,
per-byte, or LCU charges. Some customers have told us they'll happily pay that for the operational
simplicity of one address.

## What It Assumes About the LB

The design leans on a few properties of the LB, all satisfied by an NLB-style product in TCP mode:

- **L4 TCP passthrough, never TLS termination.** Router identity verification depends on the
  dialer seeing the *router's* certificate, not the LB's. This rules out L7 load balancers.
- **Round-robin or least-connections backend selection.** The dialer relies on "keep dialing and
  you'll eventually reach every backend." Sticky sessions would defeat the design.
- **Reconnects may land on different backends.** That's fine and expected, same as anycast.

The existing 10s link heartbeat sits comfortably under every managed LB's idle timeout, so links
don't die on the idle interval.

## The Core Idea

A direct link dial knows its destination before it connects. An LB dial doesn't: the LB decides
which backend you reach. Everything in this design follows from that one fact.

Three pieces make it work:

1. **Multiple public routers advertise the same LB address.** A listener carries a "load balanced"
   flag, distributed to private routers through the existing `PeerStateChange` mechanism. Several
   routers advertising the same LB address is the signal to treat them as a *pool* rather than as
   independent destinations.

2. **Identity is verified after the handshake, not before.** Since the dialer can't know which
   backend it reached until it sees the certificate, it extracts the router ID from the presented
   cert and cross-checks it against router fingerprints it already holds locally. No backend can be
   accepted just because the LB routed you to it.

3. **The dialer deliberately fans out.** Because round-robin reaches every backend if you keep
   trying, the dialer dials the LB address repeatedly, building out a link to each backend in the
   pool until every one has its full underlay.

## How It Works

### Advertising

A public router typically advertises two listeners: a direct address (so the public routers can
mesh with each other) and an LB address (for the private routers to reach them through the LB).
[Link groups](lb-routers.md#multiple-listeners-direct-and-lb) are the recommended way to keep these
audiences separate, so a private router only ever sees the LB path and a public router only sees
the direct path.

The LB flag travels as a field on the listener message, not embedded in the address. The advertise
address stays a clean `tls:host:port`. This keeps the address usable as an identity key for
grouping and dedup, and avoids changing the shared transport address parser.

LB listeners only go to routers that can actually use them. A two-way capability handshake gates
this: the controller advertises that it understands LB listeners, the router advertises that it can
dial them, and an LB listener is only forwarded when both sides agree. An LB-unaware router never
sees one, so it can never accidentally dial an LB address as a plain direct link.

### Dialing and Verification

When the dialer sees LB listeners, it groups all routers sharing the LB address into one pool, then
dials repeatedly. After each TLS handshake it:

- reads the backend's router ID from the presented certificate,
- checks that the cert's fingerprint matches the known fingerprint for that router ID,
- confirms the router is actually a member of this LB pool,
- and only then computes the link key and attaches the connection to that backend's link.

A connection to a new backend starts a new link. A connection to a backend that already has a link
joins its underlay. A connection to a backend that's already fully provisioned is closed. The
dialer keeps going until every advertised backend has its complete underlay shape (for example
2 payload + 1 ack connections), then goes idle.

LB dials **fail closed**: if a fingerprint isn't available or the reached router isn't a pool
member, the connection is dropped, never accepted unverified. This is safe because LB is new by
construction, there's no legacy peer to accommodate.

This dialer-side destination verification is genuinely new, and it's valuable on its own. The
design calls it [Phase 1](lb-routers.md#phase-1-fingerprint-based-router-verification-both-directions):
move both link verification directions (the listener checking the dialer, and now the dialer
checking the destination) to local fingerprint checks against the router data model, removing a
controller round-trip from every link setup. LB and anycast both build on top of it.

### Keeping a Link on One Backend

Ziti's routing model assumes a link connects to one specific router. But a link's underlay is
several connections, and behind an LB each connection could land on a different backend. The design
treats "an underlay spanning two backends" as something that must be actively prevented, not just
avoided.

The risk is real because the multi-underlay channel normally grows its own connections when it's
below its desired count, reusing its dial endpoint. Behind an LB, a self-initiated grow-dial would
round-robin to the wrong backend. So **in LB mode the channel's self-grow is turned off**, and a
single group-level dialer owns all dialing. Every connection is identity-checked before it's
allowed to join a link, so a link's underlay can only ever attach connections verified to be the
same backend.

The trick that makes grouping work: the dial is blind only through TCP and TLS, not through the
channel hello. By the time the hello is composed, the TLS handshake is done and the dialer knows
exactly which backend it reached. So it stamps the link's identity (a per-instance grouping ID, the
link ID, the iteration, the underlay type) into the hello *after* verifying the backend. This
relies on a `channel/v5` hook that lets the dialer compute hello headers from the peer certificate
post-handshake.

Because identity is *transported* in the hello rather than derived, an LB link's identity ends up
being ordinary link identity, just set later in the flow. Both ends agree on the link ID and
iteration, so faulting and the controller's stale-fault handling work exactly as they do for direct
links. Earlier revisions of this design tried to derive these identifiers; once you realize the
hello is post-TLS, all that machinery drops away.

### Fan-out, Backoff, and Failure Handling

A single per-destination dial loop with exponential backoff doesn't fit a pool, where one dial can
land on any backend and "failure" stops being a single condition. So the link registry grows a
pool-aware model alongside its existing per-destination dialing. State lives at two levels:

- a **group** per LB address, owning the dial cadence, backoff, and the set of advertised backends,
- and **per-backend bookkeeping** that tracks each backend's link and underlay quota but never
  dials on its own. When a backend's link drops, it just marks itself unsatisfied and re-arms the
  group dialer.

Every dial gets classified into one of a handful of outcomes, and the classification matters
because the wrong bucket either penalizes the whole pool for one bad backend or lets a broken
backend spin invisibly. The key distinctions:

- A failure *before* TCP connects means the **LB itself is unreachable**, and only this drives
  pool-wide backoff.
- A TCP connection that can't be verified (TLS failure, not a router) is **one bad backend**, not a
  pool outage, so it gets its own counter and an alert rather than penalizing the pool.
- A verified router that isn't a pool member, or whose fingerprint doesn't match, is a
  **verification failure**: closed and alerted.
- Landing on an already-satisfied backend is **redundant**, not a failure, but it isn't free (it's
  a billable LB connection), so a consecutive-redundant counter guards against spinning forever
  when the LB never routes you to the one backend you still need.

There's a subtlety worth knowing about: because verification happens mid-dial on a worker thread
but all state lives on the registry's single loop, the design uses a short, time-bounded
**reservation** handshake. The dial reserves a slot on the loop before sending its hello, and
commits only after the link is fully formed and the final cross-check passes. Reservations are
leased so an abandoned one self-heals. This keeps concurrent dials from overfilling a backend's
quota or landing inconsistent identities.

### Direct vs LB Collision

A router could be reachable both directly and through the LB. These keep separate link keys (an LB
link uses a reserved `loadbalanced` binding), so the registry won't dedup them automatically. The
rule that resolves it: **an established direct link to a backend satisfies that backend's LB
requirement**. Direct is preferred, LB is the fallback. If a direct link establishes, any redundant
LB link is closed; if the direct link later drops, LB fan-out re-arms. Link groups are meant to
keep this from arising in the first place; the rule is the backstop.

## What Doesn't Change

- **The controller** needs targeted changes (the listener flag, the capability, per-recipient
  filtering) but no new link-formation or circuit logic.
- **An established LB link looks identical to a direct link on the wire.** Forwarding,
  heartbeating, and teardown are unchanged.
- **Link registry dedup** works as-is; LB links compute the same key format, just later in the flow.

## Status and Dependencies

This builds on several pieces, some already landed and some in flight:

- the router-to-router link channel migrating to `channel/v5`, plus its hello-header hook,
- the `LinkHeaderLinkId` header (already landed),
- RDM router fingerprint distribution (from the controller-managed-router-config work),
- and the runtime listener-republish mechanism for advertising LB listeners post-handshake.

Phase 1 (local fingerprint verification in both directions) is worth doing on its own and is shared
with anycast. The LB-specific fan-out, reservation model, and collision handling sit on top.

For the full rationale, the seven dial-outcome classifications, the capability-handshake sequencing,
the registry visibility seams, and the complete work-item list, see
[lb-routers.md](lb-routers.md).
