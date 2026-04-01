# Network Federation and Multi-Tenant Transit

## Status: Design Exploration

## What Problem Are We Solving?

We want independent Ziti networks to share routers. The motivating scenario is
multi-tenant transit: a hosting provider runs public transit routers, and tenant
networks (50-500 per host) use them to bridge their private infrastructure. The
transit routers just forward packets. No edge or SDK functionality.

The same mechanism generalizes to network federation (two independent networks sharing
routers for cross-network circuits) and hierarchical topologies (regional transit
networks connecting local ones).

### Terminology

- **Host network**: The network that owns routers and shares them.
- **Client network**: A network that uses shared routers from a host.
- **Network**: A model entity on both sides of a federation relationship.
  - On the **host** side: represents a client, linked to an authenticator and Network
    Router Policies.
  - On the **client** side: represents a host, stores certificates and endpoints for
    the host's federation API. Also serves as a foreign key on imported router entities.
- **Network cert** (under consideration): A certificate representing the network as a
  whole, for JWT signing and challenge-response. Provisioned once during cluster
  bootstrap and shared across controllers. Alternative: per-host certs via
  enrollment/CSR (see Step 1).
- **Network Router Policy**: Links Network entities to routers, controlling which routers
  a client network can see.
- **Federation API**: The API that Network entities authenticate against. Network logins
  can't access the general management API.
- **Network identifier**: A 16-bit ID assigned to each client network for circuit table
  segregation and controller dispatch. This must be negotiated, since the client can
  reject an ID that collides with one from another host.
- Two networks can be clients of each other for bidirectional sharing.

### Design Principles

1. **Federation**: Each network keeps its own controller, PKI, and policies. We share
   routers, not control planes.
2. **Transit only**: Shared routers forward packets. No edge functionality (SDK
   connections, service termination, hosted services). We may expand beyond this later.
3. **Enrollment patterns reused**: Routers join foreign networks through flows that
   follow existing enrollment patterns, but with extensions. Enrollment tokens need
   additional fields (network identifier, trust domain), routers need per-network
   config storage, and the controller-side flow is purpose-built. A new enrollment
   token type is likely warranted (see #6).
4. **Controller ignorance (for routing)**: Controllers see shared routers as normal
   routers for path selection and circuit building. A controller may know a router is
   federated, but that only matters for lifecycle operations: creation, deletion,
   re-sharing enforcement.
5. **No re-sharing**: Only a router's owning network can share it. Routers enforce this
   by only accepting enrollments from their owner's controller.
6. **Purpose-built primitives**: We use dedicated Network and Network Router Policy
   entities rather than overloading existing identity and edge router policy types. A
   new enrollment token type for shared routers avoids adding edge cases to the existing
   enrollment process.

### Why Not a Separate Appliance?

We considered handling federation through a standalone application with its own control
interface to routers. Architecturally cleaner, but it introduces a lot of operational
overhead:

- Trust relationships between host network and appliance, each client and appliance
- Its own mechanism for managing who can federate with whom
- Separate certificate and key management. What root CA does the appliance use?
- Potentially its own hostname/DNS registration and host machine
- Potentially a new ALPN protocol
- Where does the CLI live? Separate binary? Embedded in the existing one?

The integrated approach avoids all of this, gives us a clear network-to-network model,
and puts the CLI in its natural home.

Worth noting: router sharing itself may not be a broadly used feature, but service
sharing could be built on top of it. Service-level federation would likely see much
wider adoption, which further argues for keeping the install footprint simple.

---

## Federation Bootstrap

### Step 1: Establish the Federation Relationship

We establish federation through a bidirectional JWT exchange. Each JWT carries the
network's CA bundle, controller endpoints, and network ID, giving the other side
everything it needs to verify and contact us.

**Client generates a federation JWT:**

1. Client admin requests a federation JWT from their controller. It contains the client
   network's CA bundle, controller endpoints, and network ID. No Network entity is
   created yet; that happens when the host JWT comes back.

2. The client JWT goes to the host admin out of band.

**Host imports the client JWT:**

3. Host admin imports the client JWT. The controller:
   - Creates a **Network** entity representing the client
   - Ingests the client's CA bundle and endpoints
   - Creates an **authenticator** for the Network entity
   - Generates the host's own federation JWT (with host CA bundle, endpoints, network ID)

4. The host JWT goes back to the client admin out of band.

5. On the host: **Network Router Policies** link the Network entity to specific
   routers. Only owned routers are eligible.

**Client imports the host JWT:**

6. Client admin imports the host JWT. The controller:
   - Creates a **Network** entity representing the host
   - Ingests the host's CA bundle and endpoints
   - Contacts the host network
   - Both sides validate each other via challenge-response
   - Client submits a CSR; host issues a certificate and returns it with its CA bundle
   - Client can now authenticate to the host's federation API

**JWT signing, two options we're considering:**

- **Network cert**: One certificate per network, provisioned during cluster bootstrap
  (migration for existing installs). Shared across all controllers. Simplifies
  multi-controller clusters: one identity regardless of which controller handles the
  request. But it introduces a key distribution problem and needs a cert rolling
  strategy.

- **Per-host cert via enrollment/CSR**: Each federation relationship gets its own cert,
  issued during enrollment. Avoids shared-key distribution, but each network has a
  different identity per peer. Multi-controller clusters need each controller to
  present the cert or proxy to the one that has it.

After this step, both sides have a Network entity. The host side links it to an
authenticator and Network Router Policies. The client side stores credentials and
endpoints. Imported routers (Step 2) reference the client-side entity as a foreign key,
so we always know which host a shared router came from.

Network logins are restricted to the federation API.

### Step 2: Add Routers to the Client Network

The client authenticates to the host's federation API and requests routers.

1. Client authenticates using the certificate obtained during enrollment.

2. Client networks have limited permissions (federation API only):
   - List accessible routers (filtered by Network Router Policies)
   - Request addition or removal of a router

3. To **add a router**:
   - Client generates a federation router enrollment token (a new token type, distinct
     from standard enrollment to avoid adding edge cases)
   - Passes the token to the host via the federation API
   - Host validates the router is owned (not itself shared from another network)
   - Host delivers the token to the router over the **control channel**, along with the
     client network's root CA (trust is already established between host and client,
     so the router doesn't need to bootstrap trust independently)
   - Router validates the enrollment came from its owner controller
   - Router completes enrollment (CSR signed by the client CA delivered with the token)
   - Client creates a router entity with a foreign key to the client-side Network entity
   - The imported router keeps the same router ID as on the host, so links have
     consistent IDs regardless of which controller requested the dial

The client controls what it trusts (it generates the enrollment token). The host
controls what it exposes (Network Router Policies). The ownership constraint ensures
routers can only be shared by the network that owns them. The foreign key on imported
routers lets the client track provenance and enforce the no-re-sharing constraint from
its side too.

**Auto-sync mode**: The manual flow above is useful for controlled environments, but
most deployments will want hands-off operation after the initial federation setup. In
auto-sync mode, the client subscribes to the host's router list and automatically adds
routers as they appear in the Network Router Policy, and removes them when they
disappear. No user interaction after Step 1.

To make auto-synced routers immediately useful, the client-side Network entity includes
an **attribute template** that maps host-side router attributes to client-side
attributes. When a router is auto-imported, the template is applied to assign role
attributes so the router can match existing policies from the start. For example, a
template might map the host attribute `region=us-east` to a client attribute
`transit-us-east`, or assign a fixed set of attributes to all routers from a given
host. This avoids the need to manually tag each imported router before it participates
in services.

### Step 3: Control Channel Establishment

Once a router completes enrollment with a client network:

1. It has a cert signed by the client's CA
2. It knows the client's controller endpoints (from the enrollment token)
3. It establishes a control channel to the client controller using its new identity
4. The client controller sees a normal router connection, no special handling
5. Route/unroute handlers feed into the shared forwarder

The router now has multiple identities: its host network identity (for links) and one
per client network (for control channels). Adding a new client at runtime means
completing enrollment and spinning up a control channel. No restart.

### Step 4: Link Establishment

The link listener aggregates CA bundles from all enrolled networks into its TLS config.
Client certs get verified at the TLS level during the handshake. The identity framework
supports runtime CA bundle changes, so adding or removing a network updates the trust
roots without restart.

**Dialer side** (tenant router dialing transit router):
1. Controller sends `UpdateLinkDest` with the transit router's info + owner network ID
2. Tenant router fetches the transit network's CA bundle from its controller
3. Tenant router dials, verifies the transit server cert against the transit CA
4. Tenant router presents its own network cert as the client cert

**Listener side** (transit router accepting connection):
1. TLS verifies the client cert against the aggregated CA bundle
2. Post-TLS: receives router ID + owner network ID from link headers
3. Calls `VerifyRouter` on that network's controller to confirm identity
4. Only after all checks pass is the link registered in the forwarder

**Transit dialing tenant**: Same pattern in reverse. The tenant router's listener
verifies the client cert at the TLS level (its aggregated bundle includes the transit
network's CA).

CAs are fetched on demand and cached. The bundle updates whenever a network is added
or removed.

---

## Multi-Tenant Transit Architecture

Federation enables multi-tenant transit. One router process serves many tenant networks
by sharing its forwarder across multiple control channels.

### The Forwarder Already Mostly Supports This

The forwarder's data plane doesn't care about router identity:

- **Circuit table**: Keyed by `(networkId, circuitId)`. Each `forwardTable` stores
  `(networkId, ctrlId)` for the controller that established the route.
- **Faulter**: Groups faults by `(networkId, ctrlId)`, sends each batch to the right
  controller.
- **Scanner**: Groups idle circuits by `(networkId, ctrlId)`, sends confirmations to the
  right controller.
- **Destination table**: Keyed by address strings (link IDs, xgress addresses). No
  identity reference.

The forwarding hot path:
1. `circuitTable.get(networkId, circuitId)` -> forwardTable
2. `forwardTable.get(srcAddr)` -> dstAddr
3. `destinationTable.get(dstAddr)` -> Destination
4. `destination.SendPayload()`

No router identity anywhere in that path.

### Network-Prefixed Circuit Isolation

Each client network gets a 16-bit network identifier, assigned by the host controller
when the Network entity is created. This identifier is injected into payload headers,
making the circuit table key `(networkId, circuitId)` instead of `circuitId` alone.

This gives us a structural guarantee of cross-tenant isolation. Circuit IDs are UUIDs,
so collision is astronomically unlikely, but the network prefix eliminates the
possibility entirely. Even a forged or duplicate circuit ID can't cross the network
boundary.

The network identifier is also required for controller dispatch. Controller IDs aren't
guaranteed unique across independent networks; two client networks could have
controllers with the same ID. The faulter, scanner, and
`MultiNetworkControllers.GetChannel()` all use `(networkId, ctrlId)` as the composite
key.

It also gives us:
- Cheap network identification in the forwarding hot path
- A natural hook for per-network rate limiting and QoS
- Per-network metrics at the packet level

16 bits supports up to 65,536 client networks per host, well above the 50-500 target.

### Network ID in the Forwarding Path

Payloads travel between routers over links using one of two serialization paths:

1. **Raw path**: A compact binary format where flags, circuit ID, sequence, headers, and
   data are packed into a byte slice. Transit routers pass these through without
   deserializing. The link's datagram underlay uses `MarshalV2WithRaw`, which returns the
   raw body bytes directly; the entire channel message envelope (headers map, content
   type) is discarded.

2. **Standard path**: A full `channel.Message` with individually keyed headers. Used when
   the raw representation isn't available (e.g., at the originating xgress before the
   first forward, or when headers need modification).

The raw path is the common case for transit forwarding. Any mechanism for carrying the
network ID must work within it, since channel-level message headers won't survive the wire.

#### Extending the Raw Format

The raw payload's second byte (the sizes field) uses bits 0-3 for circuit ID length. Bits
4-7 are unused. We define:

```
NetworkIdFlagMask byte = 0b00010000  // bit 4 of sizes byte
```

When set, 2 bytes of big-endian uint16 network ID follow immediately after the sizes byte,
before RTT and circuit ID:

```
[flags:1][sizes:1][networkId:2][rtt:0-2][circuitId:0-15][seq:varint][headers:var][data:var][heartbeat:0-8]
```

On the receive side (`UnmarshallPacketPayload`), after parsing the sizes byte, check bit 4.
If set, read 2 bytes of network ID before continuing to the remaining fields.

#### Two Sources of Network ID

A transit router has two sources for determining which network a payload belongs to:

1. **Payload bytes**: an explicit network ID in the raw format. Takes precedence when
   present.
2. **Link context**: each link is established for a specific network. When a single-network
   tenant router sends a payload with no network ID, the transit router's link handler
   knows which network the link belongs to and uses that for the circuit table lookup.

This means single-network routers are completely unaware of federation. They never inject,
strip, or read network IDs. Only transit routers, where circuits from multiple networks
coexist, deal with the complexity.

#### Injection and Stripping

The forwarder is the single decision point. After resolving the destination for a payload,
it compares the circuit's network ID (from the `forwardTable`) with the destination link's
owner network ID:

- **Dest owner != circuit network -> inject.** The next hop can't derive the network from
  link context alone (the link belongs to a different network than the circuit). Splice 2
  bytes into the raw payload: `[flags][sizes | flag][networkId:2][original[2:]]`.

- **Dest owner = circuit network -> strip (if present).** The next hop's link context is
  sufficient. Remove the 2 bytes: `[flags][sizes &^ flag][original[4:]]`.

- **No network ID and none needed -> pass through.** Nothing to do.

- **Destination is an xgress -> pass through.** Xgress is local; no network ID on the wire.

Both inject and strip are a memcpy of the payload body, much cheaper than falling through
to full message serialization. For typical payloads the cost is negligible.

#### Worked Example

**Single-hop transit** (R1 -> TR -> R2, all circuits for tenant A):

1. R1 creates raw bytes at its xgress. No network ID; R1 is single-network.
2. R1's forwarder sends to TR. R1 doesn't know about federation; sends clean raw bytes.
3. TR receives on a link that belongs to network A. No network ID in payload, so it falls
   back to link context. Circuit table lookup uses `(A, circuitId)`.
4. TR's forwarder resolves dest = R2 link. Circuit network = A, R2 owner = A. Same network,
   no injection. Sends clean raw bytes.
5. R2 receives. No network ID, lookup `(0, circuitId)`. Works.

**Multi-hop transit** (R1 -> TR1 -> TR2 -> R2):

1-2. Same as above. R1 sends clean raw bytes to TR1.
3. TR1 receives on network A's link, lookup `(A, circuitId)`.
4. TR1's forwarder resolves dest = TR2 link. Circuit network = A, TR2 owner = Host.
   A != Host, so **inject** A's network ID into raw bytes.
5. TR2 receives. Reads network ID = A from raw bytes, lookup `(A, circuitId)`.
6. TR2's forwarder resolves dest = R2 link. Circuit network = A, R2 owner = A.
   Same, so **strip** network ID from raw bytes.
7. R2 receives. No network ID, lookup `(0, circuitId)`. Works.

#### Composite Circuit Table Key

The circuit table uses a composite string key: the 2-byte big-endian network ID prepended
to the circuit ID string. For non-federated routers, network ID is always 0, so the key is
`"\x00\x00" + circuitId`, consistent with the existing single-network behavior. The
`forwardTable` gains a `networkId uint16` field, set from the route message during
`Route()`.

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

Today, routers report all links to all controllers. A federated router needs to scope
this to avoid leaking cross-network topology.

The router builds a per-network cache of known router IDs from `UpdateLinkDest`
messages. This cache tells us what each network's controller knows about, and it serves
two purposes:

1. **Link establishment**: Which destinations are in scope for each network.
2. **Link reporting**: Which networks to notify about each link.

This handles a nice edge case: if a link already exists (established for Network A) and
Network B later enrolls both endpoints, Network B learns about the existing link
without re-establishing it.

Examples:
- Tenant A router -> Transit Router: A's cache has the tenant router, reported to A
- Transit Router 1 -> Transit Router 2: A enrolled both, reported to A
- Same link NOT reported to Tenant B unless B's cache also has TR2

### What Doesn't Need to Change

- `Forwarder` core (`forwardPayload`, `ForwardAcknowledgement`, `ForwardControl`):
  the circuit table key becomes `(networkId, circuitId)` but the lookup pattern is
  the same. The forwarder gains inject/strip logic around `SendPayload`, but the
  forwarding decision itself is unchanged.
- `Faulter.run()`: groups by `(networkId, ctrlId)`
- `Scanner.scan()`: groups by `(networkId, ctrlId)`
- Route/Unroute message handling logic (gains networkId from the route message)
- Link channel handlers: the payload handler reads network ID from the raw bytes when
  present, falling back to the link's network context. The handler itself doesn't change
  structurally; it just passes the resolved network ID to the forwarder.
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
| D3 | Controller stays ignorant (for routing) | Each controller sees a normal router for path selection; federation awareness only for lifecycle |
| D4 | Runtime tenant add/remove | Restart disrupts all tenants; unacceptable at scale |
| D5 | Per-network usage metrics from start | Needed for limits and billing |
| D6 | Best-effort fairness initially | QoS and adaptive rate limiting in follow-up releases |
| D7 | Shared fate acceptable | Transit routers aren't single points of failure; circuits reroute |
| D8 | Symmetric Network entity | Same entity type on both sides: host-side for policy/auth, client-side for credentials/router provenance |
| D9 | Network Router Policies | Dedicated policy type; clearer than overloading edge router policies |
| D10 | Network login restricted to federation API | Prevents network credentials from reaching the general management API |
| D11 | No re-sharing of routers | Only owned routers eligible for sharing; routers enforce owner-only enrollment |
| D12 | Enrollment delivery via ctrl channel | Router has no other mechanism for receiving controller commands |
| D13 | Aggregated CA bundle on link listener | TLS config includes CAs from all enrolled networks; verifies client certs at TLS level |
| D14 | On-demand CA resolution | Routers fetch CAs from controllers when needed, cache results |
| D15 | Scoped link reporting | Report links only to controllers whose cache contains both endpoints |
| D16 | Integrated federation over separate application | Separate appliance is architecturally cleaner but substantially increases install complexity |
| D17 | 16-bit network identifier for circuit isolation | Structural cross-tenant isolation; eliminates reliance on UUID uniqueness |
| D18 | Network ID in raw payload format | Extends the sizes byte's unused upper nibble; avoids falling through to standard serialization on the hot path |
| D19 | Link context as implicit network ID | Single-network routers never touch network IDs; transit routers derive the network from the link when the payload doesn't carry one explicitly |
| D20 | Strip on send at the forwarder | The forwarder already knows the circuit's network and the destination's owner; co-locates inject and strip logic in one place |
| D21 | Composite string key for circuit table | 2-byte network ID prepended to circuit ID; works with the existing concurrent map; degenerates to `\x00\x00` + circuitId for non-federated routers |

---

## Open Items

### Protocol Changes

- [ ] **`UpdateLinkDest` owner network ID**: Include the owner network ID for the
  destination router so the dialer knows which CA to fetch.

- [ ] **Link header network ID**: Add `LinkHeaderNetworkId` (or similar) so the listener
  knows which controller to use for `VerifyRouter`.

- [ ] **CA bundle request message**: New control channel message type. Router requests
  CA bundle for a given network ID, controller responds with CA PEM.

- [ ] **Federation enrollment delivery message**: New control channel message type.
  Controller delivers a federation enrollment token (+ client root CA) to a router.

- [ ] **Network ID in raw payload format**: Extend the sizes byte (bit 4 =
  `NetworkIdFlagMask`) to indicate a 2-byte network ID follows. Update
  `UnmarshallPacketPayload` to parse it. Update the chunked write path in `Xgress.Write`
  to support setting it (for the standard marshalling path, add `NetworkId` to the
  `Payload` struct and include it as a channel header for non-raw messages).

- [ ] **Forwarder inject/strip logic**: After resolving the destination in
  `forwardPayload`, compare `forwardTable.networkId` with the destination link's owner
  network. Inject or strip the network ID in the raw bytes as needed. Needs the link
  destination to expose its owner network ID.

- [x] **Link network context**: Each link tracks which network it was established for.
  The payload handler uses this as the implicit network ID when the payload doesn't carry
  one. Populated from `UpdateLinkDest` or link establishment metadata. *(Prototype:
  `NetworkId()` added to `Xlink` interface, field on impl/splitImpl. All existing links
  default to 0. Actual population from UpdateLinkDest deferred.)*

### Controller Changes

- [ ] **Network entity (host side)**: Represents a client network. Linked to an
  authenticator and Network Router Policies. Supports enrollment (generates JWT with CA
  bundle + endpoints + network ID) and cert-based auth for the federation API. Assigned
  a 16-bit network identifier for circuit table segregation.

- [ ] **Network entity (client side)**: Represents a host network. Stores host CA bundle,
  endpoints, and the client certificate from enrollment. Foreign key on imported router
  entities for provenance tracking and no-re-sharing enforcement.

- [ ] **Network Router Policies**: New policy type linking Network entities to routers.
  Only owned routers are eligible.

- [ ] **Authenticator for Network entities**: Created on the host side during JWT import,
  governing client network authentication.

- [ ] **Network enrollment flow**: Bidirectional JWT exchange. Client generates JWT, host
  imports and generates host JWT, client imports host JWT and completes enrollment via
  challenge-response + CSR. Both sides create Network entities during their import step.

- [ ] **Federation API endpoints**: Authenticated by Network certificates. Client networks
  can list routers (filtered by Network Router Policies) and request add/remove.
  Restricted from the general management API.

- [ ] **Ownership validation on router sharing**: Controller verifies a router is owned
  (not shared from another network) before delivering enrollment to it.

- [ ] **Router import by enrollment**: Client controller registers the router by
  fingerprint with a foreign key to the client-side Network entity. Imported router
  keeps the same ID as on the host. Marked as imported (not owned) for no-re-sharing
  enforcement.

- [ ] **Auto-sync mode**: Client subscribes to the host's router list and automatically
  enrolls/removes routers as they appear/disappear in the Network Router Policy. Should
  be the default for most deployments. Need to define the subscription mechanism (polling
  vs push), how to handle enrollment failures/retries, and how removals propagate.

- [ ] **Attribute template for auto-imported routers**: The client-side Network entity
  includes a template that maps host-side router attributes to client-side role
  attributes. Applied during auto-import so routers match existing policies immediately.
  Need to define the template syntax: direct mapping, fixed attributes, or both.

### Router Changes

- [ ] **Owner awareness**: Router tracks its owner controller. Only accepts federation
  enrollment tokens from the owner's control channel.

- [x] **Multi-identity management**: Per-network certs/keys, control channels, and CA
  bundles. Runtime enrollment completion (new identity -> new control channel). Persisted
  across restarts. *(Prototype: `federation` package handles per-network identity storage,
  CSR enrollment, control channel setup. Persisted in `<config-dir>/networks/<id>/`.
  Reconnects on startup.)*

- [x] **MultiNetworkControllers wrapper**: Wraps N per-network `NetworkControllers`.
  `GetChannel(networkId, ctrlId)` dispatches across all networks. Per-network `ForEach`
  for scoped link notifications. *(Prototype: `forwarder.MultiNetworkControllers` created.
  Not yet wired into faulter/scanner dispatch; they still use `env.NetworkControllers`
  directly.)*

- [ ] **Per-destination CA selection for link dialing**: Link dialer uses the CA from
  `UpdateLinkDest` to verify the destination's server cert (currently uses the router's
  own identity CA).

- [ ] **Aggregated CA bundle on link listener**: TLS config includes CAs from all enrolled
  networks. Updated at runtime via the identity framework when networks are added or
  removed. After TLS, `VerifyRouter` is still called before registering the link.

- [ ] **Scoped link reporting**: Per-network cache of known router IDs from
  `UpdateLinkDest`. Report links only to networks whose cache contains the remote
  router. When a new network enrolls, check existing links against its cache.

- [ ] **State manager conditional enablement**: `state.Manager.Enabled()` returns
  hardcoded `true`. Make conditional; skip when no edge config is present.

- [x] **Composite circuit table key**: Circuit table keyed by 2-byte network ID prepended
  to circuit ID string. `forwardTable` gains a `networkId uint16` field set from the route
  message. Non-federated routers use network ID 0. *(Prototype: `CircuitKey()`,
  `CircuitIdFromKey()` in `forwarder/tables.go`. All forwarder methods, `env.Forwarder`
  interface, and callers updated to pass `networkId`.)*

- [ ] **Forwarder network ID handling**: `forwardPayload` (and `ForwardAcknowledgement`,
  `ForwardControl`) resolves network ID from the payload's raw bytes or the source link's
  network context. Injects or strips the network ID in the raw bytes before sending based
  on whether the destination link's owner matches the circuit's network. *(Prototype:
  networkId parameter threaded through all forwarding methods. Inject/strip of raw bytes
  and link-context fallback not yet implemented.)*

- [ ] **Per-network metrics**: Bytes forwarded, active circuits, active links, tagged by
  network identifier.

### Design Questions

- [ ] **Network identifier negotiation**: The host assigns the 16-bit ID, but the client
  needs to reject collisions with IDs from other hosts. When does negotiation happen:
  during enrollment, during router sharing, or both? What if no non-colliding ID is
  available?

- [ ] **Network cert vs per-host cert**: Network cert simplifies multi-controller clusters
  (one identity everywhere) but requires shared private key distribution and a cert
  rolling strategy. Per-host cert via CSR avoids key distribution but creates per-peer
  identities and complicates multi-controller setups. See Step 1.

- [ ] **Router removal flow**: Can be triggered from either side.
  - **Host**: Admin or policy change -> unprovision message to router. Unilateral.
  - **Client**: Requests removal via federation API -> host forwards unprovision.
  - **Teardown**: Close control channel, drain circuits, clean up identity.
  - **Tracking**: Host must track which clients have enabled which routers.

- [ ] **CA cache invalidation**: When a network rotates its CA, how do routers learn?
  TTL-based expiry? Controller push? Link failure triggers re-fetch?

- [ ] **Multi-hop transit**: If Tenant A enrolls TR1 and TR2, A's controller needs the
  link between them to build multi-hop paths. Scoped link reporting should handle this
  (A sent `UpdateLinkDest` for TR2 to TR1), but it should be validated end-to-end.

- [ ] **Federation revocation**: How does the host revoke a client's access? Delete the
  Network entity? What happens to routers already enrolled?

- [ ] **Service policies for federation**: Network Router Policies control router sharing.
  Could service policies control which services are available across the federation
  boundary? What would that look like?

### Future Work

- [ ] QoS: traffic prioritization across networks/tenants
- [ ] Adaptive rate limiting: per-network fairness under system stress
- [ ] Dynamic scaling: auto-provision transit routers based on load
- [ ] Service-level federation: cross-network service access via service policies

---

## Prototype Progress

Router-side prototype focused on multi-network forwarding and runtime enrollment via IPC.
Uses the existing controller enrollment endpoint; federation-specific endpoints deferred.

### Completed

- [x] **Composite circuit table key**: `CircuitKey(networkId, circuitId)` prepends 2-byte
  network ID. `forwardTable` carries `networkId`. All forwarder methods, the `env.Forwarder`
  interface, and every caller updated. Host network uses networkId 0.
  (`forwarder/tables.go`, `forwarder/forwarder.go`, `env/env.go`, plus ripple through
  handler_ctrl, handler_link, handler_xgress, xgress_edge, agent, tests)

- [x] **Faulter/scanner composite grouping**: faulter groups faults by `(networkId, ctrlId)`.
  Scanner groups idle circuits by `(networkId, ctrlId)` and strips the composite key prefix
  before sending circuit IDs to the controller.
  (`forwarder/faulter.go`, `forwarder/scanner.go`)

- [x] **MultiNetworkControllers**: wraps host `NetworkControllers` (networkId 0) plus
  per-client-network controllers. `GetChannel(networkId, ctrlId)` dispatches to the right
  network. `ForEach` iterates all. `AddNetwork`/`RemoveNetwork` for runtime changes.
  (`forwarder/multi_network_controllers.go`)

- [x] **Link network tagging**: `NetworkId() uint16` on the `Xlink` interface. Field and
  method on `impl` and `splitImpl`. All existing links default to networkId 0.
  (`xlink/xlink.go`, `xlink_transport/xlink.go`, `xlink_transport/xlink_split.go`)

- [x] **Per-network identity storage**: `NetworkIdentity` struct with on-disk persistence
  in `<config-dir>/networks/<networkId>/` (cert.pem, key.pem, ca.pem, endpoints.yml).
  Load/save/scan functions for startup reconnection.
  (`federation/network_identity.go`)

- [x] **Federation enrollment**: CSR-based enrollment against a client controller's existing
  enrollment endpoint. Parses JWT, fetches CAs, generates key, creates CSRs, stores identity
  on disk. Extracts controller endpoints from JWT claims.
  (`federation/enroll.go`)

- [x] **Transit bind handler**: registers only route, unroute, and peer state change
  handlers on client network control channels. No edge, terminator, inspect, or xrctrl
  handlers. Passes the client network's `networkId` to route/unroute handlers.
  (`handler_ctrl/transit_bind.go`)

- [x] **Client network connection**: `ConnectClientNetwork` creates a `ClientDialEnv`
  (host config with swapped identity), dials the client controller, and registers with
  `MultiNetworkControllers`. Handles the circular dependency between the transit bind
  handler and `NetworkControllers` by deferred bind handler assignment.
  (`federation/connect.go`, `federation/dial_env.go`)

- [x] **Agent operation**: `RouterFederationAddNetworkRequestType` (10125) with
  `FederationNetworkId` header. Handler reads JWT from disk, enrolls, connects.
  Rejects if network already enrolled. (`router/agent.go`, `mgmt_pb/mgmt.proto`)

- [x] **Startup reconnection**: `reconnectFederatedNetworks()` loads all persisted
  network identities and re-establishes control channels after the host control plane
  is up. (`router/router.go`)

- [x] **CLI command**: `ziti agent router federation-add-network <jwt-path> --network-id <id>`.
  (`agentcli/agent_router_federation_add_network.go`)

### Remaining

- [ ] **Wire MultiNetworkControllers into faulter/scanner**: the `multiCtrls` field exists
  on Router but faulter and scanner still use the host `NetworkControllers` directly. They
  need to dispatch via `multiCtrls.GetChannel(networkId, ctrlId)` so faults and idle circuit
  confirmations reach client network controllers.

- [ ] **Populate link networkId from UpdateLinkDest**: links currently default to 0. When
  a link is established for a client network (via UpdateLinkDest with owner network info),
  the link's `networkId_` should be set so the payload handler can use it for implicit
  network ID resolution.

- [ ] **Payload handler link-context fallback**: when a payload arrives on a link with no
  explicit network ID in the raw bytes, the handler should use `link.NetworkId()` for the
  circuit table lookup instead of defaulting to 0.

- [ ] **Raw payload network ID inject/strip**: extend the raw format's sizes byte (bit 4)
  for multi-hop transit. The forwarder compares `forwardTable.networkId` with the
  destination link's owner and splices 2 bytes into/out of the raw payload as needed.

- [ ] **End-to-end test**: two independent controllers, one shared router. Verify circuits
  from both networks traverse the shared router without interference.

- [ ] **Channel headers for ClientDialEnv**: `GetChannelHeaders()` currently returns empty
  headers. Should include router version and link listener addresses so the client controller
  can build link paths.
