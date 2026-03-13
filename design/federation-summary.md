# Network Federation — Summary

For full details, open items, and decision log, see [federation.md](federation.md).

## What Are We Trying to Do?

We want independent Ziti networks to share routers without merging their control planes.
The main use case is multi-tenant transit: a hosting provider runs shared transit
routers, and 50–500 tenant networks use them to bridge their private infrastructure.
The same mechanism supports network federation (two networks sharing routers for
cross-network circuits) and hierarchical topologies.

**Key principles:**

- Each network keeps its own controller, PKI, and policies.
- Shared routers are transit-only — no edge/SDK functionality. We may expand later.
- For routing, controllers see shared routers as normal routers. They may know a router
  is federated, but that only matters for lifecycle operations (creation, deletion,
  re-sharing enforcement).
- Only a router's owning network can share it. No re-sharing.
- We use purpose-built Network and Network Router Policy entities — not overloaded
  identity or edge router policy types.

**Why not a separate appliance?** We considered it. Architecturally cleaner, but it
introduces a lot of operational overhead (trust relationships, separate PKI, DNS, where
does the CLI live?). The integrated approach avoids all of that. And if service-level
federation gets built on top of router sharing — which is likely — keeping the install
simple matters even more.

---

## Router Changes for Multi-Network Support

The forwarder's data plane is already decoupled from router identity. The forwarding
hot path (circuit table → forward table → destination → send) never references a router
identity, and the faulter and scanner already group work by controller. Most of the
forwarder works as-is.

The main addition is the 16-bit network identifier, needed to disambiguate both circuit
IDs and controller IDs across networks. The real work is in how the router manages
identities, control channels, and links across multiple networks.

### Multiple Control Planes

A federated router has one control channel per network. Each has its own identity
(cert + key from enrollment) and its own route/unroute handlers, all feeding into the
shared forwarder.

```mermaid
graph TB
    subgraph Router["Federated Router"]
        subgraph Owner["Owner Network (host)"]
            OI["Identity<br/>(original cert + key)"]
            OCC["Control channel"]
        end

        subgraph Client1["Client Network 1"]
            C1I["Identity<br/>(cert from enrollment)"]
            C1CC["Control channel"]
        end

        subgraph Client2["Client Network 2"]
            C2I["Identity<br/>(cert from enrollment)"]
            C2CC["Control channel"]
        end

        subgraph Shared["Shared Infrastructure"]
            FW["Forwarder<br/>(circuit table keyed by networkId+circuitId)"]
            LL["Link listener<br/>(single port, aggregated CA bundle)"]
            LD["Link dialer<br/>(per-destination CA)"]
            FA["Faulter<br/>(groups by networkId+ctrlId)"]
            SC["Scanner<br/>(groups by networkId+ctrlId)"]
        end
    end

    OCC --> CA["Controller A"]
    C1CC --> CB["Controller B"]
    C2CC --> CC["Controller C"]
```

A `MultiNetworkControllers` wrapper dispatches across all per-network controller sets.
`GetChannel(networkId, ctrlId)` resolves any controller regardless of network.

### Network-Prefixed Circuit Isolation

Each client network gets a 16-bit network identifier from the host controller. It's
injected into payload headers, making the circuit table key `(networkId, circuitId)`
instead of just `circuitId`.

This gives us a structural guarantee of cross-tenant isolation — even a duplicate
circuit ID can't cross the network boundary.

The identifier is also required for controller dispatch. Controller IDs aren't unique
across independent networks — two clients could have controllers with the same ID. The
faulter, scanner, and channel lookups all use `(networkId, ctrlId)` as the composite
key.

16 bits = 65,536 client networks per host. Well above the 50–500 target. Also gives us
cheap per-network identification for metrics and future rate limiting.

### Owner Awareness

The router tracks its owner — the network it was originally provisioned into.
Federation enrollment tokens are only accepted from the owner's control channel. This
prevents re-sharing.

### Link Listener — Aggregated CA Bundle

One port. The TLS config includes CAs from every enrolled network. Client certs are
verified during the TLS handshake — no post-TLS validation needed.

When a network is added or removed, the identity framework updates the trust roots at
runtime. After TLS, the router reads link headers to find the remote router's network
and calls `VerifyRouter` on that controller before registering the link.

### Link Dialer — Per-Destination CA

`UpdateLinkDest` includes the destination's owner network ID. The dialing router
fetches and caches that network's CA bundle, then uses it to verify the server cert.

### Scoped Link Reporting

Each controller only learns about links relevant to its network. The router caches
known router IDs from `UpdateLinkDest` messages and reports each link only to networks
whose cache contains the remote router.

This handles a nice edge case: if a link already exists and a new network later enrolls
both endpoints, it learns about the link without re-establishing it.

### Runtime Add/Remove

Adding a client network: complete enrollment, spin up a control channel. No restart.
The CA bundle updates automatically. Removing: tear down the control channel, drain
circuits, remove the CA.

---

## Provisioning with the OpenZiti Controller

The controller gets two new model entities — **Network** (represents a federation peer)
and **Network Router Policy** (links Networks to routers) — plus a **federation API**
restricted to Network logins.

### Phase 1: Establish Federation

We establish the relationship through a bidirectional JWT exchange. Each JWT carries
the network's CA bundle, controller endpoints, and network ID.

```mermaid
sequenceDiagram
    participant CA as Client Admin
    participant CC as Client Controller
    participant HA as Host Admin
    participant HC as Host Controller

    CA->>CC: 1. Request federation JWT
    CC-->>CA: Client JWT
    Note right of CC: Contains: client CA bundle,<br/>controller endpoints,<br/>network ID

    CA-->>HA: 2. Client JWT (out of band)

    HA->>HC: 3. Import client JWT
    HC->>HC: Create Network entity,<br/>ingest client CA + endpoints,<br/>create authenticator
    HC-->>HA: Host JWT
    Note right of HC: Contains: host CA bundle,<br/>controller endpoints,<br/>network ID

    HA-->>CA: 4. Host JWT (out of band)

    Note over HC: 5. On host: Network Router Policies<br/>link Network to routers (owned only)

    CA->>CC: 6. Import host JWT
    CC->>CC: Create Network entity,<br/>ingest host CA + endpoints
    CC->>HC: Challenge-response + CSR
    HC-->>CC: Certificate + CA bundle
    Note over CC: Client can now authenticate<br/>to host federation API
```

Both sides end up with a Network entity. The host side links it to an authenticator and
Network Router Policies. The client side stores credentials and endpoints.

**JWT signing and authentication** are still under consideration — either a network-wide
cert (shared across controllers) or per-host certs via enrollment/CSR. See the detailed
design doc for tradeoffs.

### Phase 2: Share Routers

The client authenticates to the host's federation API and requests routers.

```mermaid
sequenceDiagram
    participant CC as Client Controller
    participant HC as Host Controller
    participant R as Router R1

    CC->>HC: 1. Authenticate (federation API only)
    CC->>HC: 2. List accessible routers
    HC-->>CC: Router list (filtered by Network Router Policies)

    CC->>HC: 3. Request add router R1<br/>(includes enrollment token)
    HC->>HC: 4. Validate R1 is owned<br/>(not shared from another network)
    HC->>R: 5. Deliver enrollment token + client root CA<br/>via control channel
    R->>R: 6. Validate token came from owner
    R->>R: 7. Complete enrollment<br/>(CSR signed by client CA)
    R->>CC: 8. Connect with new identity<br/>(same router ID as on host)

    CC->>CC: 9. Create router entity:<br/>• same router ID as host<br/>• foreign key → Network entity<br/>• marked as imported (not owned)

    Note over CC,R: Router now in client's model.<br/>Cannot be re-shared. Control channel<br/>established. Route/unroute handlers<br/>feed shared forwarder.
```

**Auto-sync mode**: Most deployments won't want to add routers one at a time. In
auto-sync mode, the client subscribes to the host's router list and automatically
enrolls routers as they appear in the Network Router Policy (and removes them when they
disappear). An **attribute template** on the client-side Network entity maps host-side
router attributes to client-side attributes, so imported routers match existing policies
from the start — no manual tagging needed.

### Phase 3: Link Establishment

Once routers are enrolled in multiple networks, links form. Each controller drives link
formation independently.

```mermaid
sequenceDiagram
    participant TC as Tenant Controller
    participant TR as Transit Router
    participant TenR as Tenant Router

    Note over TC: Controller includes transit<br/>router in path planning<br/>(sees it as a normal router)

    TC->>TenR: 1. UpdateLinkDest<br/>(transit router info + owner network ID)
    TenR->>TC: 2. Fetch transit network's CA bundle

    TenR->>TR: 3. TLS dial<br/>(verify transit server cert against transit CA)
    TR->>TR: 4. Verify tenant client cert<br/>against aggregated CA bundle
    TR->>TC: 5. VerifyRouter<br/>(confirm tenant router identity)
    TR->>TR: 6. Register link in shared forwarder

    TR->>TC: 7. Report link to tenant controller<br/>(scoped: only reported to networks<br/>whose cache contains both endpoints)
```

### No-Re-Sharing Enforcement

Three levels:

1. **Host controller**: Network Router Policies only allow owned routers. Ownership is
   validated before delivering enrollment tokens.
2. **Router**: Only accepts federation enrollment tokens from its owner controller.
3. **Client controller**: Imported routers have a foreign key to the Network entity,
   marking them as imported. Ineligible for the client's own Network Router Policies.
