# Anycast Support for Router Links

## Motivation

There are situations where we may have a pool of routers on the public internet that are either 
being used to provide transit between routers in private networks or are providing SDK edge access
to routers in private networks. We want to support putting these kinds of routers behind anycast 
IP addresses. This allows us to expand these pools by placing additional routers in new geographic
locations. This allows the private routers to connect to them automatically without changing 
firewall allow-lists. The set of anycast IPs for a region stays stable even as the public 
routers behind them change.

## Anycast Assumptions

1. **Single destination per anycast address.** A private router only needs to reach *one* public
   router behind a given anycast address. It does not need to discover or connect to all of them.
   Load balancing is achieved by deploying public routers geographically close to private
   routers, so BGP attracts their connections to the nearest one. Multiple anycast addresses can
   be used to provide additional scale and reliability, each backed by its own pool of public
   routers. A private router would establish one link per anycast address.

2. **BGP route stability.** Anycast routing is topology-based. From a given source, the same
   anycast IP will consistently route to the same destination as long as BGP routes are stable.
   We accept that if routes change mid-connection (BGP flap, router withdrawal, etc.), the TCP
   connection will break. This is the standard anycast trade-off and is acceptable for our use
   case, the private router will reconnect, potentially to a different public router,
   and re-form the link.

3. **BGP shifts may not produce clean disconnects.** Depending on the anycast implementation,
   a BGP route change may not result in a TCP RST. If packets are silently redirected to a
   different public router that drops them (no matching TCP state), the private router won't
   get an immediate error. In this case, the link heartbeater will detect the dead link. Link
   heartbeats are sent every 10 seconds, and the link is closed after 60 seconds without a
   response (configurable via `CloseUnresponsiveTimeout`). In the common case where the new
   router's OS sends RSTs for the unknown TCP connection, teardown is near-immediate.

4. **No need to influence backend selection.** We don't need ECMP tricks, per-connection hashing,
   or anycast-layer cooperation to reach specific backends. We accept whatever router BGP gives
   us.

5. **Reconnect may land on a different router.** If a link drops and the private router redials,
   it might reach a different public router than before. This is fine and even desirable,
   it means the topology adapts to changes in the network or in the set of deployed public
   routers.

## Design

### Advertising Anycast Addresses

Public routers advertise their link listener as normal but with an anycast query parameter
in the advertise address:

```
tls:192.168.1.1:3022?anycast=true
```

This flows through the existing `PeerStateChange` mechanism. Multiple public routers may
advertise the same anycast address. No controller changes are needed, it just passes listener
info through as it does today.

### Multiple Listeners: Direct and Anycast

A public router will typically advertise two listeners: a direct address and an anycast
address. For example:

```
listeners:
  - binding: transport
    bind: tls:0.0.0.0:3022
    advertise: tls:pub-east-1.example.com:3022    # direct, reachable by peers
    groups:
      - public
  - binding: transport
    bind: tls:0.0.0.0:3023
    advertise: tls:192.168.1.1:3023?anycast=true  # anycast, reachable by private routers
    groups:
      - private
```

This is important for mesh formation. Without a direct address, public routers behind the
same anycast IP couldn't form links with each other, since dialing the anycast address might
reach themselves or a random peer.

Ideally, link groups would be used to keep these separated: the anycast listener in a group
targeted at private routers, and the direct listener in a group for public routers that can
actually reach it. This avoids private routers attempting the direct address they can't reach,
and avoids public routers going through anycast when they have a direct path.

**Dialer preference:** When a router advertises both a direct and an anycast listener in the
same group, the dialer should prefer the direct address. The anycast address is a fallback for
routers that can't reach the direct address. If the direct dial succeeds, the anycast listener
for that router is skipped. If the direct dial fails or isn't attempted, the anycast path is
used.

### Dialer Behavior

When the dialer processes `PeerStateChange` listeners and sees `?anycast=true`:

1. **Group by anycast address.** All public routers advertising the same anycast address
   are treated as one dial target rather than N separate destinations.
2. **Dial once.** The private router connects to the anycast address a single time.
3. **Discover the destination.** After the TLS handshake, the private router extracts the
   public router's ID from the presented certificate. It also reads a router ID hello
   header from the public router and cross-checks the two. A mismatch indicates
   misconfiguration or MITM.
4. **Register the link.** The link key and link state are created post-connect using the
   discovered router ID, then registered normally.
5. **Skip redundant dials.** Other public routers advertising the same anycast address are
   not dialed separately, the private router already has a link through that address.
6. **Warn on non-anycast duplicates.** If multiple routers advertise the same address *without*
   `?anycast=true`, log a warning. This is likely a misconfiguration (we've hit this before with
   routers accidentally sharing an advertise address). The anycast grouping logic and this
   duplicate detection are essentially the same check with different outcomes.

### Destination Verification

Today, only the listener side verifies the dialer's identity (via `verifyRouter()` on the
controller). The dialer does not verify that it reached the expected destination.

As part of this work, we want to add destination verification for *all* link dials, not just
anycast. After connecting, the dialer should extract the listener's router ID from the TLS
certificate (and cross-check against a hello header) and confirm it matches the expected
destination. For non-anycast dials, a mismatch is an error. For anycast dials, the discovered
ID is used to register the link.

### Fingerprint Distribution

Currently `verifyRouter()` requires a synchronous RPC to the controller during link setup. We
can eliminate this round-trip by distributing router certificate fingerprints to all routers
ahead of time. Two options:

1. **PeerStateChange messages.** Add fingerprints to the `PeerStateChange` data alongside
   the existing router ID, version, and listeners. Simple, but means fingerprints are sent
   with every peer state update.
2. **Router data model.** Store fingerprints on the router data model and let them propagate
   through the normal data model sync. Less chatty, since fingerprints only change on router
   re-enrollment or cert rotation, not on every connect/disconnect.

Either way, the verifying side (dialer or listener) compares the presented TLS certificate's
fingerprint against the known fingerprints for that router ID. No controller round-trip needed.

For anycast this is especially useful: the dialer has fingerprints for all routers in the
anycast group, connects, checks the presented cert against the full set, and immediately knows
both which router it reached and that it's legitimate.

This could eventually replace the `verifyRouter()` RPC for non-anycast dials as well, making
all link establishment faster.

### Listener Behavior

The listener needs one small change: include its own router ID in the hello headers sent back to
the dialer. The router ID is already available in the TLS certificate. The header provides a fast
cross-check.

### Link Key Construction

Today the link key is `dialerBinding->protocol:destRouterId->listenerBinding` and is computed
before dialing (from `PeerStateChange` data). For anycast links:

- The key is computed *after* connecting, once the destination router ID is known from the cert.
- The key format stays the same -- an anycast link looks like any other link once established.

### Reconnect

If the link drops:

1. The private router re-dials the same anycast address.
2. It may reach a different public router than before.
3. The old link (to router B) is gone. The new link (to router C) is registered with router C's
   ID.
4. The controller receives the updated `RouterLinks` and adjusts its routing topology.

### What Doesn't Change

- **Controller** - minimal changes. If fingerprints go through the router data model, the
  controller needs to store and sync them. Otherwise it broadcasts `PeerStateChange` as today
  and accepts `RouterLinks` reports as today.
- **Link registry dedup** - works as-is, keys are just computed later for anycast links.
- **Listener config** - public routers configure listeners normally, just set the advertise
  address with `?anycast=true`.
- **Link protocol/format** - an established anycast link is indistinguishable from a normal link.

## Open Questions

**linkState lifecycle for anycast groups.** Today there's one `linkState` per destination router,
and it drives the dial/retry logic. With anycast grouping, multiple routers share one address and
we only dial once. The state model needs to change: probably one `linkState` per anycast address
rather than per destination. When the link drops, the anycast-level state owns the retry timer.
When reconnect reaches a different router, the old per-router state is cleaned up and new state
is created for the discovered router. Details to be resolved during implementation.

**PeerStateChange removal and anycast groups.** When a public router disconnects, the
controller sends `PeerState_Removed` for it. If that router was one of several behind an anycast
address, the anycast address is still valid. If the private router's current link happened to be
to that router, the link is already dead, but the reconnect should target the anycast address,
not the removed router specifically. Need to make sure removal of one router's `linkDest` doesn't
tear down the anycast group. Details to be resolved during implementation.

**Anycast dial reaches a router that already has a direct link.** A private router could end up
dialing an anycast address and discovering it reached router B, to which it already has a direct
link. The link keys should match (same dest router ID, protocol, and bindings), so normal dedup
should handle this. Need to verify that the listener binding from the anycast listener matches
the direct listener, otherwise we'd get two links to the same router.

**Address parsing for query parameters.** The channel address format (`tls:192.168.1.1:3022`)
is not a standard URI and may not support query parameters today. If the existing parser can't
handle `?anycast=true`, the anycast flag may need to be a separate field on the `Listener`
protobuf rather than embedded in the address string.

## Work Items

- [ ] Listener sends router ID in hello headers
- [ ] Dialer extracts listener router ID from TLS cert after connect
- [ ] Dialer cross-checks cert router ID against hello header router ID
- [ ] Dialer verifies destination router ID on all dials (error on mismatch for non-anycast)
- [ ] Distribute router fingerprints (via router data model or PeerStateChange)
- [ ] Local fingerprint verification to replace `verifyRouter()` RPC
- [ ] Parse `?anycast=true` from listener advertise addresses
- [ ] Anycast grouping in link state: group routers by shared anycast address
- [ ] Post-connect link key/state creation for anycast dials
- [ ] Dialer prefers direct listeners over anycast for the same router
- [ ] Skip redundant dials for routers behind an already-connected anycast address
- [ ] Reconnect logic, re-dial anycast address, accept different destination
- [ ] Warn when multiple routers share an address without `?anycast=true`
- [ ] Tests for anycast dial flow
- [ ] Tests for reconnect-to-different-router scenario
