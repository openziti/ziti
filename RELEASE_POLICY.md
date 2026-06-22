# OpenZiti Release Policy

## Problem Statement

Customers operating OpenZiti networks — whether self-hosted or through NetFoundry — face significant friction around upgrades. The root causes fall into two categories:

**Upgrade friction**
- Customers with downstream users (customers-of-customers) cannot absorb upgrades on short notice. Coordinating upgrades across their own customers introduces outages and support burden they are unwilling to accept.
- Network upgrades introduce connectivity interruptions. Until hitless upgrades are available, any upgrade carries downtime risk, making customers resistant to frequent version changes.
- There is currently no mechanism to enforce or incentivize version upgrades, leaving networks on old versions indefinitely.

**Version clarity**
- Clients and SDKs are released on a different cadence than Controllers and Routers. This creates combinations of components that have never been tested together and have no stated support status.
- There is no documented version compatibility matrix, so neither customers nor support teams have a clear answer to "is my version combination supported?"
- Without defined support windows, customers have no basis for planning upgrades, and there is no language available to enforce end-of-life transitions contractually.

---

## How LTS Addresses This

Long Term Stable (LTS) versioning establishes a predictable, well-defined support lifecycle that directly addresses the above pain points:

- **Predictable upgrade windows** — customers can plan upgrades around annual LTS releases rather than reacting to continuous releases.
- **Defined support guarantees** — two active LTS versions at any time means any customer is always within a two-year support window, giving them time to plan migrations.
- **Reduced testing surface** — LTS anchors compatibility testing to a small set of known-good version combinations rather than every possible permutation.
- **Contract language** — LTS provides the vocabulary and structure needed to define support SLAs, enforce upgrade timelines, and price extended support.
- **Client/SDK compatibility guarantee** — all current client and SDK releases are guaranteed to function with supported LTS versions, resolving the version matrix problem for the majority of deployments.

---

## Definitions

These terms are used consistently across both the OpenZiti and NetFoundry LTS strategies.

**Feature**
New capability, behavior, or configuration option that expands what the software can do. Features are not backported to LTS releases except under the narrow conditions described in the [Feature Backport Exception](#feature-backport-exception) section below.

**Bug Fix**
A correction to unintended behavior that does not introduce new functionality. A bug fix restores the software to its documented or reasonably expected behavior. It must not change any existing API, protocol behavior, or configuration contract.

**Critical Bug Fix**
A bug fix that meets one or more of the following criteria: it affects all or a significant majority of deployments, it causes measurable operational impact (elevated error rates, unexpected restarts, degraded throughput, failed enrollments), or it blocks a required security or compliance workflow. A critical bug fix is still bound by the same constraints as a standard bug fix — it must not alter any existing API, protocol behavior, or configuration contract.

**Security Fix**
A patch that addresses a known vulnerability — CVE-assigned or internally identified — that could expose users to unauthorized access, data exposure, privilege escalation, denial of service, or credential compromise. Security fixes are applied to all active LTS versions regardless of phase and are prioritized above all other work. See [SECURITY.md](./SECURITY.md) for vulnerability reporting and disclosure handling.

**Critical Production Defect**
A defect that causes one or more of the following in a production environment: loss of network connectivity for authenticated identities, inability to authenticate or enroll, data corruption or loss, unrecoverable process crashes, or a regression directly introduced by a prior patch release. **Scope**: the defect must affect production deployments at scale, not isolated edge cases or single-tenant configurations. Performance degradation, UI issues, edge-case behavior, and feature limitations do not qualify.

**No Support**
For OpenZiti (OSS): no new patches, security fixes, bug fixes, or active issue triage. GitHub Issues may remain open but will not be actively worked. Community discussion is welcome but carries no response guarantee.

---

## OpenZiti LTS Strategy

OpenZiti is an open-source project with community consumers. LTS here establishes community expectations and testing guarantees, but does not enforce upgrade paths.

### Support Lifecycle

| Phase | Duration | Scope | Notes |
|---|---|---|---|
| Latest Development | Rolling | Features + Security + Bug Fixes | Continuous releases; no LTS guarantees |
| Active LTS (N) | 12 months | Security + Critical Bug Fixes | Dependency updates accept compatible (minor and patch) updates of external dependencies and point/bugfix updates of internal dependencies, gated by the smoketest and full validation suite. Breaking (major) updates and feature backports remain excluded |
| Maintenance LTS (N-1) | Months 13–24 | Security + Critical Production Defects Only | Dependency updates restricted to security fixes and required vulnerability SLA remediation |
| End of Life | Month 25+ | No Support | Deprecation announced in release notes; archive only. See [No Support](#definitions) definition above. |

#### Dependency Update Stance (Active LTS)

Security vulnerabilities in transitive dependencies are common enough that holding an Active LTS to security-only dependency updates leaves it carrying known issues between cuts. Active LTS therefore accepts compatible (minor and patch) updates of external dependencies and point/bugfix updates of internal OpenZiti dependencies on each monthly cut. The churn this introduces is mitigated by gating every cut on the smoketest and the full validation suite: a dependency update ships only if the validated build passes. Breaking (major) dependency updates and feature backports are out of scope and follow the [Feature Backport Exception](#feature-backport-exception) rules where applicable. Maintenance LTS (N-1) remains restricted to security and required vulnerability-SLA updates only.

### Patch Release Cadence

- **Active LTS (N)**: monthly patch releases, rolling up accumulated bug and security fixes.
- **Maintenance LTS (N-1)**: patch releases as needed, driven by critical bugs and security fixes. No fixed cadence.
- **Latest Development**: continuous releases on the normal feature cadence; not bound by the LTS patch schedule.

Security fixes may ship outside the regular cadence when warranted by severity.

### Naming

LTS versions are defined at the `major.minor` level and cut approximately every 12 months.

- **Current LTS (N)** — the most recently designated LTS `major.minor`
- **Previous LTS (N-1)** — the prior LTS `major.minor`, in maintenance

**Example:** Assuming `2.0` is the current LTS and `2.1` is in active development:

| Label | Version |
|---|---|
| Latest Development | 2.1 |
| Active LTS (N) | 2.0 |
| Maintenance LTS (N-1) | 1.6 |
| EOL / No Support | 1.5 and below |

### Upgrade Paths

Sequential upgrade paths (N-1 → N) are guaranteed to work and are validated as part of each new LTS minor cut. Skipping generations (e.g., jumping directly from N-2 to N) is not tested and carries no compatibility guarantee.

The following upgrade sequences are validated:
- **Maintenance LTS (N-1) → Active LTS (N)**: tested as part of the Active LTS (N) cut.
- **Active LTS (N) → next Active LTS**: tested when the new LTS is designated.

Customers are expected to complete the N-1 → N migration before the N-1 End of Life date to remain on a supported and tested upgrade path.

### Feature Backport Exception

Feature backports to Active LTS are strictly exceptional and are **not** a standard part of LTS delivery. A backport may be considered only when all of the following are true:

1. A documented bug exists that cannot be resolved without introducing the feature.
2. The feature has been explicitly reviewed and approved by the LTS maintainers.
3. The feature introduces no breaking changes to any existing API, protocol behavior, or configuration contract.

Feature backports are **never** applied to Maintenance LTS (N-1) releases. All backport decisions must be tracked and documented in the release notes of the resulting patch.

### Clients/SDKs

OpenZiti provides the following SDKs and client applications:

**SDKs**
- Go (`sdk-golang`)
- Java (`sdk-jvm`)
- Swift (`sdk-swift`)
- Python (`sdk-python`)
- Node.js (`sdk-nodejs`)
- .NET/C# (`sdk-csharp`)
- C (`sdk-c`)

All SDKs must be tested against all active LTS versions and the current latest release before a new SDK version is published. This ensures that SDK consumers are not broken regardless of which supported Controller/Router version they are running against.

**Clients**
- Windows Desktop Edge
- macOS
- Android
- iOS
- Linux

All Clients must be tested against all active LTS versions and the current latest release before a new client version is published. This ensures that client users are not broken regardless of which supported Controller/Router version they are running against.

### Testing

#### Controller/Router Interoperability

Every new release is tested for controller/router interoperability against the current Active LTS (N) and Maintenance LTS (N-1) before shipping. A minimum of four combinations are validated: the new version acting as controller against each LTS version's routers, and the new version acting as router against each LTS version's controllers.

Interoperability is guaranteed between any new release and the current Active LTS (N). Interoperability with the Maintenance LTS (N-1) is best effort. This applies to both LTS releases and dev releases that pass the interop test gate before shipping.

Combinations spanning more than two LTS generations are not tested and carry no compatibility guarantee.

#### Release Testing Levels

| Release Type | Required Testing |
|---|---|
| New LTS minor cut | Smoketest + full validation suite |
| Active LTS (N) point release | Smoketest + full validation suite |
| Maintenance LTS (N-1) point release | Smoketest |
| Latest development release | Smoketest |

#### SDK and Tunneler Compatibility

**Guarantee**: The latest released version of every supported SDK and tunneler is guaranteed to function with any active LTS Controller/Router version (N or N-1). Additionally, the SDK and tunneler versions that were current at the time an LTS was minted remain guaranteed to function with all subsequent patch releases of that LTS — so a deployment that does not update its SDK or tunneler will not be broken by a Controller/Router patch within the same LTS generation.

The guarantee does **not** cover: older SDK or tunneler versions used against a newer LTS generation (e.g., an SDK version predating LTS N is not guaranteed to work with LTS N+1 unless explicitly tested).

- **Go SDK (`sdk-golang`) and C SDK (`sdk-c`)**: Tested against every release via the standard smoketest. Compatibility is guaranteed for every release, not just LTS cuts.
- **All other SDKs** (Java, Swift, Python, Node.js, .NET/C#): Tested against new LTS minor cuts. Compatibility is guaranteed for active LTS versions.
- **Tunnelers/clients** (Desktop Edge, mobile apps): The latest released version is tested against new LTS minor cuts. Compatibility is guaranteed for active LTS versions.

Community-contributed test coverage is accepted but not required for LTS designation.

---

