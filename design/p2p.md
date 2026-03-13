# P2P Direct Connections for OpenZiti

## Status: Early Design

## Goal

Enable OpenZiti SDK endpoints to establish direct peer-to-peer UDP connections
when possible, bypassing the router data plane for lower latency and reduced
infrastructure cost, while retaining the full OpenZiti circuit as a fallback.

---

## Existing Building Blocks

### 1. DTLS Transport (`transport/dtls/`)
- Secure UDP transport using `pion/dtls/v3`
- Already integrated into the transport address registry
- Handles certificate-based mutual authentication
- Supports configurable buffer sizes and bandwidth shaping

### 2. SDK Xgress (`sdk-golang/xgress/`)
- Retransmit with RTT-based adaptive timeouts
- Window-based flow control (4KB-4MB adaptive window)
- Sequence-numbered payloads with ACK/NAK
- Chunking support for MTU compliance
- `LinkSendBuffer` / `LinkReceiveBuffer` for TX/RX management

### 3. SDK-Router Connectivity
- SDKs already maintain persistent channel connections to edge routers
- Routers are on the public internet (or at least reachable)
- Routers already act as signaling/control plane intermediaries
- Natural candidates for STUN-like address discovery

### 4. Fallback Data Path
- Standard OpenZiti circuits through the router mesh
- Already proven, reliable, always available
- Can serve as relay (analogous to TURN) when direct path fails

### 5. Virtual UDP Multiplexing (`transport/udpconn/`)
- Single UDP socket can handle multiple virtual connections
- Connection-per-remote-address model with expiration policies
- Already used by the UDP and DTLS transports

---

## Architecture Overview

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
  │ Channel  │    (fallback / parallel)         │ Channel  │
  └──────────┘                                  └──────────┘
        │                                            │
        └──── control plane (routers/ctrl) ──────────┘
```

The xgress layer dispatches to one or both data paths. The direct DTLS
path is preferred when available; the circuit path is always maintained
as fallback.

---

## Design Areas

### 1. Orchestration

**Problem:** How do two SDKs discover that they both support P2P, agree on
parameters, and coordinate the UDP hole punch?

**Proposed approach: Circuit-first, then upgrade**

1. Client SDK dials a service normally, including a `P2P-Capable` header
2. The circuit is established through the router mesh as usual
3. The hosting SDK sees the `P2P-Capable` header in the dial and responds
   with its own `P2P-Capable` acknowledgement
4. Both sides now know the other supports P2P. They begin the direct
   connection setup over the existing circuit as a signaling channel

**Advantages:**
- No changes to the dial path's success/failure semantics
- Application gets a working connection immediately
- P2P upgrade is a transparent optimization
- Graceful degradation: if P2P setup fails, nothing changes

**Open questions:**
- [ ] What message type carries the P2P capability advertisement?
  Options: header on existing dial/dial-success, new content type, or
  xgress control message
- [ ] Should P2P preference be per-service policy (controller-level), per-SDK
  config, or both? Likely both: controller policy gates whether P2P is
  allowed for a service, SDK config controls whether it wants to use it
- [ ] Timeout for the P2P upgrade attempt before giving up and staying on
  the circuit path
- [ ] Should the controller be involved in authorizing/auditing P2P
  connections, or is it purely SDK-to-SDK via the circuit?

### 2. STUN / Address Discovery

**Problem:** Each SDK needs to learn its public-facing UDP address:port so the
peer can send packets to it for hole punching.

**Proposed approach: Router-as-STUN-server**

Routers that are on the public internet can offer a STUN-like service:
- SDK sends a UDP probe to the router
- Router reflects back the observed source IP:port
- SDK now knows its mapped address

**Router selection:**
- Routers advertise STUN capability (new flag/role in the control plane)
- SDK picks a STUN-capable router, ideally the one it's already connected
  to or one topologically close
- Multiple STUN servers can be queried to detect symmetric NAT (different
  mapped ports per destination = symmetric NAT = hole punch unlikely)

**Open questions:**
- [ ] Use standard STUN protocol (RFC 5389) or a simpler custom protocol
  over the existing channel? Standard STUN has the advantage of being
  well-understood and testable with existing tools. Could use `pion/stun`
  since `pion/dtls` is already a dependency
- [ ] How does the SDK open the UDP socket that will later be used for the
  P2P connection? It needs to be the *same* socket used for STUN probing
  and then for the actual DTLS connection (so the NAT mapping is preserved)
- [ ] Do we need ICE-style candidate gathering (host candidates, server
  reflexive, relay)? Or is a simpler model sufficient for v1?
- [ ] Symmetric NAT detection: if both sides are behind symmetric NAT, skip
  the P2P attempt entirely and stay on the circuit

### 3. UDP Hole Punch Procedure

**Proposed flow** (after both sides have mapped addresses via STUN):

1. SDK-A sends its mapped address to SDK-B via the circuit (signaling)
2. SDK-B sends its mapped address to SDK-A via the circuit
3. Both sides begin sending UDP packets to each other's mapped address
4. NAT mappings allow the return traffic through
5. Once bidirectional connectivity is confirmed, perform DTLS handshake
6. Xgress layer begins sending data over the direct DTLS connection

**Open questions:**
- [ ] Who initiates the DTLS handshake (client vs server role)? The dial
  initiator is the natural choice for DTLS client
- [ ] How many probe packets, what interval, what timeout before declaring
  hole punch failed?
- [ ] Do we need a keepalive to maintain the NAT mapping once established?
  DTLS heartbeats might serve this purpose
- [ ] How do we handle the DTLS identity/certificates? SDKs already have
  identities with certs. Mutual TLS auth over DTLS gives us
  authentication of the P2P channel for free
- [ ] Port prediction strategies for moderate/symmetric NATs?

### 4. Decoupling Xgress from Router Connection

**Problem:** Today, `edgeConn` is tightly bound to a single `RouterConn` via
`ConnMux`. The `XgAdapter` bridges xgress to the edge protocol, but everything
flows through one router channel. For P2P we need xgress to dispatch across
multiple underlying transports.

**Current path:**
```
App → edgeConn.Write() → XgAdapter.writeAdapter → Xgress
    → XgAdapter.ForwardPayload() → MsgChannel → RouterConn → Router
```

**Target path:**
```
App → edgeConn.Write() → Xgress
    → MultiPathDispatcher
        ├→ DirectPath (DTLS conn to peer)
        └→ CircuitPath (MsgChannel → RouterConn → Router)
```

**Proposed approach:**

Introduce a `DataPlaneAdapter` that can manage multiple underlying paths:

- **`PathSelector`** — decides which path to use for each payload
  - Prefer direct path when available and healthy
  - Fall back to circuit path on direct path failure
  - Optionally use both for redundancy during transition
- **`DirectPath`** — wraps the DTLS connection to the peer
  - Implements xgress `ForwardPayload` / `ForwardAcknowledgement`
  - Monitors health (RTT, loss rate)
- **`CircuitPath`** — the existing `XgAdapter.ForwardPayload` path
  - Always available as fallback

**This also enables future SDK-level rerouting:** if a circuit is rerouted to
a different path through the mesh, the xgress layer just gets a new
`CircuitPath` without disrupting the application connection.

**Open questions:**
- [ ] Where does the multi-path abstraction live? In `XgAdapter`, or a new
  layer between `edgeConn` and `Xgress`?
- [ ] How do ACKs work across paths? If a payload is sent on the direct path,
  the ACK might come back on either path. The xgress layer already handles
  ACKs by sequence number, so this may just work
- [ ] Sequence number continuity: when switching from circuit to direct path,
  sequence numbers must be continuous. Since xgress owns sequencing, this
  should be natural
- [ ] How does the receive side work? The xgress `LinkReceiveBuffer`
  already handles out-of-order delivery, so packets arriving from
  different paths should be reorderable
- [ ] What does `Close()` look like? Need to tear down both paths cleanly

### 5. Dual-Path Operation

**Problem:** When both paths are active, how do we manage them?

**Modes:**
1. **Prefer direct, circuit as standby** — all data flows over direct path.
   Circuit is kept alive with minimal keepalive. If direct path fails,
   immediately switch to circuit
2. **Prefer direct, circuit as hot fallback** — data on direct path, but
   circuit is actively monitored. Retransmissions go over circuit path.
   Provides fastest recovery
3. **Active-active** — both paths carry data, receiver deduplicates by
   sequence number. Maximum reliability, double bandwidth cost
4. **Measurement-based** — start on circuit, measure direct path quality,
   switch when direct path proves better. Conservative but safe

**Recommendation:** Start with mode 2 (direct preferred, circuit hot fallback)
for the initial implementation. It gives the latency/cost benefit of direct
while providing fast recovery.

**Open questions:**
- [ ] How to detect direct path degradation? RTT increase, packet loss
  rate, consecutive retransmit threshold?
- [ ] Hysteresis for path switching: avoid flapping between paths
- [ ] How to handle the transition period when hole punch first succeeds?
  Drain in-flight circuit packets before switching? Or accept brief
  reordering (xgress handles it)?
- [ ] Metrics and observability: need to expose which path is active, per-path
  RTT, loss rates

---

## Implementation Phases

### Phase 1: Foundation
- Add STUN capability to routers (reflect mapped address)
- Add P2P capability headers to the SDK dial/bind protocol
- Prototype SDK-to-SDK DTLS connection using hole punching

### Phase 2: Multi-Path Xgress
- Refactor `XgAdapter` / `DataPlaneAdapter` to support multiple paths
- Implement `PathSelector` with direct-preferred strategy
- Wire up direct DTLS path alongside circuit path

### Phase 3: Integration
- End-to-end P2P dial with automatic upgrade
- Fallback handling when hole punch fails
- Keepalive and NAT mapping maintenance
- Circuit-based signaling for P2P setup messages

### Phase 4: Polish
- Controller policy for P2P eligibility per service
- Symmetric NAT detection and early bail-out
- Metrics, logging, management events
- Performance testing and tuning

---

## Analysis & Recommendations

### Circuit-First is the Right Call

The circuit-first-then-upgrade approach is strongly favored. It preserves
existing dial semantics exactly — the application gets a working connection
immediately with zero additional latency. P2P becomes a transparent optimization
that can fail silently. This also means no new failure modes in the critical
dial path.

### Socket Reuse is the Key STUN Constraint

The most subtle requirement in the STUN/hole-punch flow is **socket reuse**.
The UDP socket used for STUN probing must be the exact same socket used for
the subsequent DTLS handshake and data transfer. If the SDK opens a new socket
for DTLS, the NAT mapping from the STUN probe won't apply and the hole punch
will fail. The `udpconn` package's virtual connection multiplexing is a natural
fit here — a single `net.UDPConn` can handle the STUN probe, the hole-punch
probes, and then the DTLS session, all sharing the same local port.

### pion/stun is the Natural STUN Library

Since `pion/dtls` is already a dependency, adding `pion/stun` keeps the
dependency tree clean. Standard STUN (RFC 5389) also means the router's STUN
service is testable with off-the-shelf tools, which helps debugging. A custom
protocol would save a dependency but lose interoperability and tooling.

### Early Symmetric NAT Detection Saves Time

Symmetric NAT detection should be the first step after STUN discovery, before
any signaling is exchanged. Query two different STUN-capable routers from the
same socket — if the mapped ports differ, the NAT is symmetric and hole
punching will almost certainly fail. Bail out immediately rather than burning
through the hole-punch timeout. This is cheap (two extra UDP round trips) and
avoids a multi-second timeout in the common corporate-NAT case.

### Xgress Decoupling is the Hardest But Most Valuable Piece

The `DataPlaneAdapter` interface (`ForwardPayload`, `ForwardAcknowledgement`,
`RetransmitPayload`) is already the right seam for multi-path. The xgress
layer doesn't care where payloads go — it hands them to the adapter and
receives ACKs keyed by sequence number. A multi-path adapter just needs to:

1. Choose which underlying path to forward each payload on
2. Accept ACKs from any path (sequence numbers are path-independent)
3. Optionally route retransmissions to a different path than the original

This means ACKs should "just work" across paths without special handling,
since `LinkSendBuffer` matches ACKs by sequence number regardless of origin.

The receive side is similarly clean: `LinkReceiveBuffer` already does
out-of-order reassembly via its B-tree, so packets arriving interleaved from
both paths will be correctly ordered and delivered.

This refactoring also directly unblocks SDK-level rerouting — a circuit
reroute just swaps the `CircuitPath` underneath the multi-path adapter without
touching the `Xgress` instance or the application connection.

### Recommended Dual-Path Mode: Direct + Circuit Hot Fallback

Mode 2 (direct preferred, circuit hot fallback) is the best starting point:

- Primary data flows over the direct DTLS path for lowest latency
- The circuit path stays active — retransmissions can be sent over the circuit,
  giving near-instant recovery from direct-path packet loss without waiting
  for the full RTT-based retransmit timeout
- If the direct path fails entirely, all traffic shifts to circuit seamlessly
- xgress sequence numbers ensure no duplicates or gaps regardless of path

This avoids the bandwidth cost of active-active while providing much faster
recovery than standby mode.

### Mobile / Network Change Consideration

P2P connections are inherently fragile across network changes (WiFi → cellular,
IP change, etc.). The circuit fallback is critical here — it provides
continuity while a new P2P connection is re-established on the new network.
This could eventually be automatic: detect direct path failure → fall back to
circuit → re-probe STUN → re-punch → upgrade again. The multi-path xgress
architecture makes this a natural extension.

---

## Related Future Work

- **SDK-level rerouting:** The multi-path xgress work directly enables
  rerouting at the SDK level. If a circuit is rerouted, the SDK can
  receive a new circuit path without breaking the application connection.
- **Multi-path routing:** Could extend beyond two paths to use multiple
  circuit paths simultaneously for bandwidth aggregation.
- **Mobile handoff:** P2P connections will break on network change (new IP).
  The fallback circuit provides continuity. Could re-establish P2P on the
  new network automatically.
