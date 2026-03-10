# Network Federation and Multi-Tenant Transit

## Status: Design Exploration

## Overview

This document describes a federation mechanism that allows Ziti networks to share routers.
The primary motivation is multi-tenant transit: public routers that forward traffic between
private networks belonging to different tenants. The same mechanism generalizes to network
federation — bridging independent Ziti networks through shared routing infrastructure.

### Use Cases

- **Multi-tenant transit**: A hosting provider runs shared public transit routers. Tenant
  networks (50-500 per host) use these routers to bridge their private networks. Transit
  routers only forward — no edge/SDK functionality.
- **Network federation**: Two independent Ziti networks share routers to enable cross-network
  circuits without merging their control planes.
- **Hierarchical topologies**: Regional transit networks connecting local networks.

### Terminology

- **Host network**: The network that owns routers and shares them with others.
- **Client network**: The network that uses shared routers from a host network.
- **Network**: A model entity used on both sides of a federation relationship.
  - On the **host** side: represents a client network, linked to an auth policy and to
    Network Router Policies. The credential the client uses to authenticate.
  - On the **client** side: represents a host network, stores the certificates and
    endpoints for accessing the host's federation API. Also serves as a foreign key
    on router entities that were imported from the host network.
- **Network Router Policy**: A policy that associates Network entities with routers,
  controlling which routers are visible to each client network.
- Two networks can be clients of each other if both want to share routers bidirectionally.

### Key Principles

1. **Federation, not merger**: Each network retains its own controller, PKI, and policies.
   Networks share routers, not control planes.
2. **Transit only (for now)**: Shared routers forward packets. No edge functionality (SDK
   connections, service termination, hosted services) on shared routers.
3. **Existing enrollment reused**: Routers join foreign networks via standard enrollment.
   No new trust bootstrapping protocol.
4. **Controller ignorance (routing)**: For routing purposes, each controller sees shared
   routers as normal routers — no special handling in path selection or circuit building.
   The controller may be aware that a router is federated, but that only matters for
   lifecycle operations (creation, deletion, re-sharing enforcement).
5. **No re-sharing**: Only a router's owning network can share it. Client networks cannot
   re-share routers they received from a host network to their own clients. Routers
   enforce this by only accepting enrollments delivered through their owner's controller.
6. **Purpose-built primitives**: Federation uses dedicated Network and Network Router Policy
   entities rather than overloading existing identity and edge router policy types.

### Architectural Note

An alternative approach would be to handle federation through a custom-built application
with a separate control interface to the router, keeping federation concerns entirely
outside the controller and router codebases. While this would be architecturally cleaner,
it would substantially increase install complexity — requiring additional components to
deploy and manage. If federation is to be a commonly used feature for OpenZiti users,
the integrated approach is preferable.

Additionally, while router sharing itself may not be a broadly used feature, service
sharing could potentially be built on top of it. Service-level federation would likely
see much wider adoption, which further argues for keeping the install footprint simple.

---

## Federation Bootstrap

### Step 1: Establish Federation Relationship

The **host network** (the network sharing routers) initiates:

1. Admin creates a **Network** entity on the host network, which:
   - Represents the client network in the host's model
   - Generates an enrollment JWT containing:
     - Hosting network's CA bundle
     - Hosting network's controller endpoints
     - Network ID and name

2. The JWT is provided to the **client network** out of band.

3. The client network completes the enrollment:
   - Creates a **Network** entity on its own controller representing the host network
   - Stores the host network's CA bundle and controller endpoints on this entity
   - Obtains a client certificate for the Network entity on the host network
   - Can now authenticate to the host network's federation API

4. On the host network: **Network Router Policies** associate routers with the Network
   entity, controlling which routers are visible to the client network. Only routers
   owned by the host network are eligible for inclusion in Network Router Policies.

5. On the host network: an **auth policy** is associated with the Network entity,
   governing how the client network authenticates to the federation API.

Both sides now have a Network entity. On the host, it represents the client and is
linked to auth policies and Network Router Policies. On the client, it represents the
host and stores the credentials and endpoints needed to access the host's federation
API. When the client later imports routers from the host (Step 2), those router
entities reference the client-side Network entity as a foreign key, making it clear
which host network each shared router came from.

Network logins are restricted to the federation API — they cannot access the general
management API.

### Step 2: Add Routers to Consuming Network

The client network uses its federation credentials to enable shared routers:

1. Consuming network authenticates to the host network's federation API using the
   Network entity's certificate.

2. The Network entity has **limited permissions** (federation API only):
   - List routers it has access to (filtered by Network Router Policies on the host network)
   - Request addition or removal of a router from its network

3. To **add a router**:
   - Consuming network generates a standard router enrollment token
   - Passes the enrollment token to the host network via the federation API
   - Hosting network validates that the router is owned by the host (not itself a
     shared router from another network) before proceeding
   - Hosting network delivers the token to the target router over the **control channel**
   - Router validates that the enrollment came from its owner controller before
     accepting it
   - Router completes enrollment with the client network (CSR signed by client
     network's enrollment CA, fingerprint registered)
   - Client network creates a router entity referencing the client-side Network entity
     as a foreign key (identifying the host network it came from)
   - The imported router entity retains the same router ID as on the host network, so
     that links between routers have consistent IDs regardless of which network's
     controller requested the dial

The client network controls what it trusts (it generates the enrollment). The host
network controls what it exposes (Network Router Policies on the Network entity).
The ownership constraint ensures routers can only be shared by the network that owns
them — a client network cannot re-share routers it received from a host. The foreign
key on imported router entities allows the client network to track provenance and
enforce the no-re-sharing constraint from its side as well.

### Step 3: Control Channel Establishment

After enrollment completes:

1. The router has a cert signed by the client network's CA
2. The router knows the client network's controller endpoints (from the enrollment JWT)
3. The router establishes a control channel to the client network's controller(s) using
   its new identity
4. The client network's controller sees a normal router connection — no special handling
5. The router registers route/unroute handlers that feed into the shared forwarder

The router now has **multiple identities** — its host network identity (for links) and
one per client network (for control channels). Adding a client network at runtime
means completing enrollment and spinning up a new control channel — no restart needed.

### Step 4: Link Establishment

Links use on-demand CA resolution. The link listener aggregates CA bundles from all
enrolled networks into its TLS config, so client certs are verified at the TLS level.
The identity framework already supports runtime CA bundle changes, so adding or
removing a network updates the listener's trust roots without restart.

**Dialer side** (tenant router dialing transit router):
1. Controller sends `UpdateLinkDest` with the transit router's info + owner network ID
2. Tenant router requests the CA bundle for the transit network from its controller
   (the controller has this from the federation enrollment in Step 1)
3. Tenant router dials the transit router, uses the transit CA to verify the server cert
4. Tenant router presents its own network cert as the client cert

**Listener side** (transit router accepting connection):
1. TLS handshake verifies the client cert against the aggregated CA bundle (which
   includes CAs from all enrolled networks)
2. Post-TLS: receives router ID + owner network ID from link headers
3. Calls `VerifyRouter` on the controller for that network to confirm identity
4. Only after all checks pass is the link registered in the forwarder

**Transit router dialing tenant router**: Same pattern in reverse — uses the tenant
network's CA (fetched from that tenant's controller) to verify the destination's server
cert. The tenant router's listener verifies the client cert at the TLS level (its
aggregated CA bundle includes the transit network's CA).

CAs are fetched on demand and cached. The listener's aggregated CA bundle is updated
whenever a network is added or removed.

---

## Multi-Tenant Transit Architecture

Federation enables multi-tenant transit. A single router process serves many tenant
networks by sharing its forwarder across multiple control channels.

### Why the Forwarder Already Supports This

The forwarder's data plane is decoupled from router identity:

- **Circuit table**: Keyed by `(networkId, circuitId)`. Each `forwardTable` stores
  the `(networkId, ctrlId)` that established the route.
- **Faulter**: Groups faults by `(networkId, ctrlId)` and sends each batch to the
  correct controller.
- **Scanner**: Groups idle circuits by `(networkId, ctrlId)` and sends confirmations
  to the correct controller.
- **Destination table**: Keyed by address strings (link IDs, xgress addresses). No
  identity reference.

The forwarding hot path:
1. `circuitTable.get(circuitId)` → forwardTable
2. `forwardTable.get(srcAddr)` → dstAddr
3. `destinationTable.get(dstAddr)` → Destination
4. `destination.SendPayload()`

No router identity is referenced anywhere in this path.

### Network-Prefixed Circuit Isolation

Each client network is assigned a 16-bit network identifier by the host controller
when the Network entity is created. This identifier is injected into payload headers
when traffic crosses between networks, allowing the circuit table to be keyed by
`(networkId, circuitId)` rather than `circuitId` alone.

This provides a structural guarantee of cross-tenant isolation in the forwarding plane.
While circuit IDs are UUIDs and collision is astronomically unlikely, the network
prefix eliminates the possibility entirely — even a forged or duplicate circuit ID
cannot cross the network boundary.

The network identifier is also required for controller dispatch. Controller IDs
(`ctrlId`) are not guaranteed to be unique across independent networks — two different
client networks could have controllers with the same ID. The faulter, scanner, and
`MultiNetworkControllers.GetChannel()` all use `(networkId, ctrlId)` as the composite
key to ensure correct dispatch.

The 16-bit identifier also provides:
- **Cheap network identification** in the forwarding hot path without an extra lookup
- **Natural hook for per-network rate limiting** and QoS (future work)
- **Per-network metrics** at the packet level

16 bits supports up to 65,536 client networks per host, well above the 50–500 target.

### Component Layout

```
Federated Transit Router
├── Shared
│   ├── Identity (1 cert + 1 CA from host network)
│   ├── Link listener (1 port, aggregated CA bundle from all enrolled networks)
│   ├── Link dialer (per-destination CA from UpdateLinkDest)
│   ├── Link registry
│   ├── Forwarder (circuitTable keyed by networkId+circuitId, destinationTable)
│   ├── Faulter (routes by networkId+ctrlId)
│   ├── Scanner (routes by networkId+ctrlId)
│   ├── Metrics (per-network namespacing)
│   ├── CA cache (network ID → CA bundle)
│   └── Goroutine pools, health checks, debug
│
├── MultiNetworkControllers
│   ├── Wraps N per-network NetworkControllers
│   ├── GetChannel(networkId, ctrlId) dispatches across all networks
│   └── Per-network ForEach for scoped link notifications
│
└── Per-Network (from enrollment)
    ├── Identity (cert + key from enrollment with that network)
    ├── NetworkControllers (control channels to that network)
    └── Control channel bind handler (routes/unroutes → shared forwarder)
```

### Scoped Link Reporting

Routers currently report all links to all controllers. A federated router must scope
link reports to avoid leaking cross-network topology.

**Mechanism**: The router builds a per-network cache of known router IDs from
`UpdateLinkDest` messages. Each network's cache represents what that network's
controller knows about.

This cache serves two purposes:

1. **Link establishment**: The router knows which destinations are in scope for each
   network and can dial accordingly.

2. **Link reporting**: For any link, the router checks which networks know about the
   remote router (from the cache) and reports the link to those networks.

This handles the case where a link already exists (established for Network A) and
Network B later enrolls both endpoints — Network B learns about the existing link
without it needing to be re-established.

Examples:
- Tenant A router → Transit Router: Tenant A's cache has the tenant router → reported to A
- Transit Router 1 → Transit Router 2: Tenant A enrolled both, so both are in A's cache
  → reported to A
- Same link NOT reported to Tenant B unless B's cache also contains TR2

### What Does NOT Need to Change

- `Forwarder` core (`forwardPayload`, `ForwardAcknowledgement`, `ForwardControl`) —
  circuit table key changes from `circuitId` to `(networkId, circuitId)` but the
  lookup pattern is the same
- `Faulter.run()` — groups by `(networkId, ctrlId)`
- `Scanner.scan()` — groups by `(networkId, ctrlId)`
- Route/Unroute message handling logic
- Link channel handlers (payload, ack, control, close)
- Controller circuit building / path selection (sees a normal router)

### Multi-Tenant Transit vs Multi-Process

| Dimension | Federated Transit Router | N Separate Processes |
|-----------|-------------------------|---------------------|
| Memory per tenant | ~1-2MB (ctrl channel) | ~30-50MB (Go runtime) |
| Code changes | Moderate | None |
| Fault isolation | Shared fate (circuits reroute) | Full isolation |
| Security isolation | Shared memory (Go safe) | Process-level |
| Port usage | 1 shared port | N ports |
| Ops complexity | 1 process | N processes, N configs |
| Independent upgrades | No | Yes |
| Resource fairness | Best-effort initially | OS-level (cgroups) |
| Federation path | Built-in | N/A |

---

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | 50-500 tenants per host | Justifies shared-process approach over multi-process |
| D2 | Transit only, no edge | Keeps edge subsystem (sessions, policies, tunneling) out of scope |
| D3 | Controller stays ignorant | Each controller sees a normal router, no multi-tenancy awareness |
| D4 | Runtime tenant add/remove | Restart disrupts all tenants — unacceptable at scale |
| D5 | Per-network usage metrics from start | Needed for limits and billing |
| D6 | Best-effort fairness initially | QoS and adaptive rate limiting in follow-up releases |
| D7 | Shared fate acceptable | Transit routers aren't single points of failure; circuits reroute |
| D8 | Symmetric Network entity | Same entity type used on both sides: host-side for policy/auth, client-side for credentials/router provenance |
| D9 | Network Router Policies | Dedicated policy type for linking networks to routers; clearer than overloading edge router policies |
| D10 | Network login restricted to federation API | Prevents network credentials from accessing the general management API |
| D11 | No re-sharing of routers | Only owned routers eligible for sharing; routers enforce owner-only enrollment |
| D12 | Enrollment delivery via ctrl channel | Router has no other mechanism for receiving controller commands |
| D13 | Aggregated CA bundle on link listener | Listener TLS config includes CAs from all enrolled networks; verifies client certs at TLS level, not post-TLS |
| D14 | On-demand CA resolution | Routers fetch CAs from controllers when needed, cache results |
| D15 | Scoped link reporting | Report links only to the controller that sent `UpdateLinkDest` |
| D16 | Integrated federation over separate application | Custom app with separate control interface would be architecturally cleaner but substantially increases install complexity; service sharing built on top would be widely used |
| D17 | 16-bit network identifier for circuit isolation | Structural guarantee of cross-tenant isolation; eliminates reliance on UUID uniqueness for tenant separation |

---

## Open Items

### Protocol Changes

- [ ] **`UpdateLinkDest` owner network ID**: Include the owner network ID for the
  destination router so the dialer knows which CA to fetch and which network the
  destination belongs to.

- [ ] **Link header network ID**: Add `LinkHeaderNetworkId` (or similar) so the listener
  can determine which controller to use for `VerifyRouter`.

- [ ] **CA bundle request message**: New control channel message type — router requests
  CA bundle for a given network ID, controller responds with CA PEM.

- [ ] **Federation enrollment delivery message**: New control channel message type —
  controller delivers a federation enrollment JWT to a router.

- [ ] **Network ID in payload headers**: Payload headers include the 16-bit network
  identifier so that circuit table lookups use `(networkId, circuitId)` as the key.

### Controller Changes

- [ ] **Network entity (host side)**: Represents a client network. Linked to an auth
  policy and to Network Router Policies. Supports enrollment (generates JWT with CA
  bundle + endpoints + network ID) and cert-based authentication for federation API
  access. Assigned a 16-bit network identifier used for circuit table segregation.

- [ ] **Network entity (client side)**: Represents a host network. Stores the host's CA
  bundle, controller endpoints, and the client certificate obtained during enrollment.
  Serves as a foreign key on router entities imported from the host, enabling the
  client to track which routers came from which host and enforce the no-re-sharing
  constraint.

- [ ] **Network Router Policies**: New policy type associating Network entities with
  routers. Only routers owned by the host network are eligible — shared routers
  from other networks must be excluded.

- [ ] **Auth policy for Network entities**: Auth policy associated with each Network
  entity on the host side, governing client network authentication.

- [ ] **Network enrollment flow**: Admin creates a Network entity on the host →
  generates enrollment JWT. Client network completes enrollment: creates its own
  Network entity representing the host, stores host CA and endpoints, receives a
  certificate for federation API access.

- [ ] **Federation API endpoints**: Authenticated by Network certificates. Consuming
  network can list accessible routers (filtered by Network Router Policies) and
  request add/remove. Network logins must be restricted to these endpoints — they
  cannot access the general management API.

- [ ] **Ownership validation on router sharing**: When a client network requests to add
  a router, the controller must verify the router is owned (not itself shared from
  another network) before delivering the enrollment to it.

- [ ] **Router import by enrollment**: When a router completes federation enrollment
  with a client network, the client network's controller registers the router
  by fingerprint with a foreign key reference to the client-side Network entity.
  The imported router must use the same router ID as on the host network, so that
  links have consistent router IDs regardless of which controller requested the dial.
  This marks the router as imported (not owned), which is used to enforce the
  no-re-sharing constraint. Standard router connection handling from that point.

### Router Changes

- [ ] **Owner awareness**: Routers must know which controller is their owner (the network
  they were originally provisioned into). Routers must only accept federation enrollment
  tokens delivered through their owner's control channel, rejecting enrollments from
  non-owner controllers. This prevents client networks from re-sharing routers.

- [ ] **Multi-identity management**: Store certs/keys per enrolled network. Map network
  IDs to identities, control channels, and CA bundles. Handle enrollment completion at
  runtime (new identity → new control channel). Persist across restarts.

- [ ] **MultiNetworkControllers wrapper**: Wraps N per-network `NetworkControllers`.
  `GetChannel(networkId, ctrlId)` dispatches across all networks. Per-network `ForEach` for scoped
  link notifications.

- [ ] **Per-destination CA selection for link dialing**: Link dialer uses the CA from
  `UpdateLinkDest` to verify the destination's server cert. Currently uses the router's
  own identity CA.

- [ ] **Aggregated CA bundle on link listener**: Listener's TLS config includes CAs from
  all enrolled networks. When a network is added or removed, update the listener's
  trust roots at runtime via the identity framework. After TLS verifies the client
  cert, the router still calls `VerifyRouter` on the appropriate controller (identified
  from link headers) before registering the link.

- [ ] **Scoped link reporting**: Build a per-network cache of known router IDs from
  `UpdateLinkDest` messages. Use this cache to determine which networks to report each
  link to (report to networks whose cache contains the remote router). Also use this
  cache to drive link establishment (dial destinations in scope for each network).
  When a new network enrolls, check existing links against its cache and report any
  that are already established.

- [ ] **State manager conditional enablement**: `state.Manager.Enabled()` currently returns
  hardcoded `true`. Make conditional — skip when no edge config is present.

- [ ] **Network-prefixed circuit table**: Circuit table keyed by `(networkId, circuitId)`
  instead of `circuitId` alone. The 16-bit network identifier is carried in payload
  headers and injected by the route handler when the circuit is established. Provides
  structural cross-tenant isolation and a natural key for per-network metrics and
  future rate limiting.

- [ ] **Per-network metrics**: Usage metrics (bytes forwarded, active circuits, active links)
  tagged by network. Network identifier on each circuit provides direct aggregation.

### Design Questions Still To Explore

- [ ] **Network enrollment authentication details**: The host-side Network entity uses
  cert-based auth (certificate obtained during enrollment). Details: Does the enrollment
  JWT include a one-time token that the client exchanges for a CSR-signed cert? Or does
  the client present a self-signed cert during enrollment that gets registered? How does
  the client-side Network entity get populated — does the enrollment response include
  the host's CA bundle and endpoints, or are those extracted from the JWT itself?

- [ ] **Router removal flow**: Can be triggered from either side:
  - **Host network**: Admin or policy change triggers removal. Host controller sends an
    unprovision message to the router for that client network. Unilateral — host can
    always revoke access.
  - **Client network**: Requests removal via the federation API. Host controller forwards
    the unprovision to the router.
  - **Router teardown**: Close control channel to the client network, drain circuits for
    that network, clean up the client network identity.
  - **Host controller tracking**: Must track which client networks have enabled which
    routers, so it can trigger unprovision when policies change or federation is revoked.

- [ ] **CA cache invalidation**: When a network rotates its CA, how do routers learn the
  new CA? TTL-based cache expiry? Controller push? Link failure triggers re-fetch?

- [ ] **Multi-hop transit**: If Tenant A enrolls two transit routers (TR1, TR2), and there's
  a link between them, Tenant A's controller needs to know about that link to build
  multi-hop paths. This works with scoped link reporting (Tenant A's controller sent
  `UpdateLinkDest` for TR2 to TR1), but should be validated end-to-end.

- [ ] **Federation revocation**: How does the host network revoke a client network's
  access? Delete the Network entity? What happens to routers already enrolled with
  the client network?

- [ ] **Service policies for federation**: Network Router Policies control which routers
  are shared. Could service policies later control which services are available across
  the federation boundary? What would that look like?

### Future Enhancements

- [ ] **QoS**: Traffic prioritization across networks/tenants
- [ ] **Adaptive rate limiting**: Per-network fairness under system stress
- [ ] **Dynamic scaling**: Auto-provision transit routers based on load
- [ ] **Service-level federation**: Extend beyond transit to allow cross-network service
  access controlled by service policies
