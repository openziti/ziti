# Validation Test Summary

Fablab-based smoke/chaos tests under `zititest/models/`. Each test uses a shared set of chaos and validation
utilities to inject failures, then check that the system converges to a correct, consistent state.

## Shared Infrastructure

### Chaos Utilities (`zititest/zitilab/chaos/`)
- `chaos.go` - `SelectRandom` with `RandomOfTotal`, `RandomInRange`, `PercentageRange`, `Percentage`; `RestartSelected`, `StopSelected`, `ValidateUp`
- `partition.go` - iptables-based connection disruption between components

### Test Harness Helpers

Fablab tests wrap a small set of controller-side validation operations and inspect queries. Most of what the tests
verify is actually delegated to the controller; the test code is thin orchestration around these backend capabilities.

### Controller Validation Operations

The controller exposes a set of validation requests over the management channel. Test code triggers them via the
`ziti fabric validate <operation>` CLI or programmatically using the validation wrappers above. Handlers are
registered in `controller/handler_mgmt/bind.go`.

| CLI | Handler | What it checks |
|---|---|---|
| `validate circuits` | `validate_circuits.go` | Per-router circuit consistency: forwarder circuits, edge listener circuits, SDK circuits. Flags missing-in-controller / missing-in-forwarder / missing-in-edge / missing-in-SDK. |
| `validate terminators` | `validate_terminators.go` | SDK identity-connection vs. ER/T terminator hosting consistency. Optional auto-fix for invalid terminators. Supports terminator/identity filters. |
| `validate router-links` | `validate_router_links.go` | Per-router link state and health, compared against controller's view of the mesh. |
| `validate router-sdk-terminators` | `validate_router_sdk_terminators.go` | SDK terminator state as reported by each router. |
| `validate router-ert-terminators` | `validate_router_ert_terminators.go` | ER/T terminator state as reported by each router. |
| `validate router-data-model` | `validate_router_data_model.go` | Router data model vs. controller data model with detailed diff. Optional controller-side validation and auto-fix. |
| `validate identity-connection-statuses` | `validate_identity_connection_statuses.go` | Identity connection statuses on each router vs. controller view. Catches unreported events, state mismatches, and stale connections. |
| `validate controller-dialers` | `validate_controller_dialers.go` | Controller-to-controller dialer health when controller-dialer mode is enabled. |

CLI implementations live under `ziti/cmd/fabric/validate_*.go`; each takes an optional router regex filter.

### Inspect / Debug Framework

For investigating failures the controller supports rich runtime introspection. `InspectionsManager`
(`controller/network/inspect.go`) fans an inspect request out to matched routers and raft peers in parallel
with a 10-second timeout; results can be queried interactively with `ziti fabric inspect <app-regex> <key>` or
over `POST /inspections`. Router side handler: `router/handler_ctrl/inspect.go`. Shared response types:
`common/inspect/*.go`.

**Process health / runtime**
- `stackdump` - goroutine stack trace (controller or router)
- `metrics` - current Prometheus metrics registry
- `config` - fully rendered JSON configuration

**Cluster / controller topology**
- `cluster-config` - Raft cluster config
- `connected-peers` - raft member status
- `connected-routers` - connected router list with id, name, version, connect time, underlays
- `data-model-index` - controller data-model index
- `router-messaging` - pending peer updates and terminator validations
- `terminator-costs` - dynamic terminator cost details (circuit count, failure cost)
- `ctrl-dialer` - controller dialer config and per-router dial state

**Router state**
- `links` - router's view of its links (see `LinksInspectResult`)
- `sdk-terminators` / `ert-terminators` - terminator state by source
- `router-circuits` - forwarder circuit map
- `router-edge-circuits` - edge listener circuit map
- `router-sdk-circuits` - SDK circuit map
- `router-data-model` - full router data model
- `router-data-model-index` - router index/timeline
- `router-controllers` - router's view of its controller connections
- `identity-connection-statuses` - router's identity connection state

**Targeted detail**
- `sdk-context:<identity-id>` - SDK context for one identity
- `circuit:<circuit-id>` - detail for one circuit
- `circuitandstacks:<circuit-id>` - circuit detail with goroutine stacks at the forwarder

### Agent CLI (IPC)

For low-level debugging of a running process, `ziti agent` (`ziti/cmd/agentcli/`) talks to the process over its
local agent socket. Generic: `ps`, `list`, `stack`, `gc`, `memstats`, `stats`, `pprof-cpu`, `pprof-heap`, `trace`,
`dump-heap`, `set-log-level`, `set-channel-log-level`.

Router-specific: `router dump-api-sessions`, `router dump-routes`, `router dump-links`, `router forget-link`,
`router disconnect`, `router reconnect`, `router decommission`.

Controller-specific: `controller snapshot-db`.

Cluster: `cluster add` / `remove` / `list` / `transfer-leadership` / `init` / `restore-from-db`.

Relevant management channel debug request types used behind the agent CLI:
`RouterDebugDumpForwarderTablesRequestType`, `RouterDebugDumpLinksRequestType`, `RouterQuiesceRequestType`,
`RouterDequiesceRequestType`, `RouterDecommissionRequestType`.

## 1. Circuit Test (`circuit-test`)
**Regions**: 2 (us-east-1, us-west-2), 3 controllers (HA)
**Scale**: 2 edge routers/region + 1 metrics router, 2 client routers, 2 host routers. Scaled loop4 SDK and ERT clients/hosts.
**Instance types**: t3.micro controllers, c5.xlarge routers.

**What it tests**: End-to-end circuit creation, data flow, and consistency. Runs loop4 sim scenarios (throughput,
gentle-throughput, latency, slow) across SDK and ERT clients/hosts. After chaos, validates circuit state is
consistent across controllers, router forwarder + link state, edge state, and SDK state. Supports
backwards-compatibility modes (default, client-backwards-compat, host-backwards-compat) to catch version drift.

**Chaos injection**: random 0-3 controllers + 10-75% routers restarted (7 scenarios on 3-bit counter).

**Validation**: `chaos.ValidateUp`, `validations.ValidateCircuits` (limit=none), custom `SimMetricsValidator`
(circuit-test/sim_metrics.go), `edge.ControllerAvailable`.

**Iteration**: transitionLifecycleRouters -> enableMetrics -> sowChaos -> runSimScenario -> validateSimMetrics -> validateCircuits

## 2. Links Test (`links-test`)
**Regions**: 4 (us-east-1, us-west-2, eu-west-2, eu-central-1), 3 HA controllers (one each in us-east-1/us-west-2/eu-west-2)
**Scale**: 5 hosts/region x 20 routers/host = ~100 routers, expecting ~79,800 links. Mixed v1.5.4 and current versions for compat testing.
**Instance types**: c5.large controllers, c5.xlarge routers.

**What it tests**: Full mesh link establishment at scale. After chaos, validates all expected links are present
and that each router's link state matches the controller's view. Detects duplicate links, missing links, and
per-router state drift.

**Chaos injection**: all controllers restart candidates + 10-75% routers restarted.

**Validation**: `chaos.SelectRandom` (percentage-based), `chaos.RestartSelected`, custom `validateLinks` using
management channel to each router.

**Iteration**: sowChaos -> validateUp -> validateLinks

## 3. Router Data Model Test (`router-data-model-test`)
**Regions**: 3 (us-east-1, eu-west-2, ap-southeast-2), 3 HA controllers.
**Scale**: 2 routers/region, 8/6/6 hosts per region (scaled). 100 services, 100 identities, 100 service policies,
configs, config types, posture checks. 2000+ configuration entities.
**Instance types**: c5.xlarge controllers and routers.

**What it tests**: Router data models converge to match controller state under combined chaos AND massive data
model churn. 7 scenarios cycle through create/delete/modify of services, configs, config types, identities,
service policies, posture checks. Validates dependent entity ordering (configs before config types, etc).

**Chaos injection**: random controllers restart + 10-75% routers restart + concurrent CRUD churn from test driver.

**Validation**: `chaos.SelectRandom`, `chaos.RestartSelected`, custom `ValidateRouterDataModel` using management
channel, parallel task execution with retry policies from `zititest/zitilab/models/retry.go`.

**Iteration**: sowChaos -> validateUp -> validate

## 4. SDK Hosting Test (`sdk-hosting-test`)
**Regions**: 3, 3 HA controllers with preferred leaders.
**Scale**: 2 routers/region, 5 hosts/region, 100 tunnelers/host, 5 services/tunneler, 2 terminators/service. Expected terminators: 3 x 5 x 100 x 5 x 2 = 15,000. 2,000 services total.
**Instance types**: c5.2xlarge controllers (TLS handshake load), c5.xlarge routers/hosts.

**What it tests**: SDK-hosted terminator convergence at massive scale. After chaos, validates expected
terminator count is reached and router SDK terminator state is consistent with the controller.

**Chaos injection**: 7 scenarios combining 0-3 controller restarts + 10-75% router restarts + 10-75% host stops
(hosts are stopped, not restarted).

**Validation**: `chaos.SelectRandom`, `chaos.RestartSelected` (controllers/routers), `chaos.StopSelected` (hosts),
`validations.ValidateTerminators` with `ValidateSdkTerminators`.

**Iteration**: sowChaos -> validateUp -> validate

## 5. ERT Hosting Test (`ert-hosting-test`)
**Regions**: 4 (us-east-1, us-west-2, eu-west-2, eu-central-1), 3 HA controllers.
**Scale**: 5 hosts/region x 20 routers/host (all tagged as tunnelers), 400 services with smart-routing terminator strategy.
**Instance types**: c5.large controllers, c5.xlarge routers.

**What it tests**: Edge-router-tunneler hosted terminator convergence. Similar to SDK hosting test but
terminators are hosted by the routers themselves acting as tunnelers. Validates terminator count and router
ERT terminator state consistency.

**Chaos injection**: 3 scenarios (2-bit counter) - controllers, 10-75% routers, or both. Simpler than
sdk-hosting-test because there are no separate hosts.

**Validation**: `chaos.SelectRandom`, `chaos.RestartSelected`, `validations.ValidateTerminators` with
`ValidateErtTerminators`.

**Iteration**: sowChaos -> validateUp -> validate

## 6. SDK Status Test (`sdk-status-test`)
**Regions**: 3 (us-east-1, us-west-2, ap-southeast-2), 3 HA controllers (2/1/0 distribution).
**Scale**: 8 hosts in us-east, 6 in others, 10 tunnelers per host = ~200 tunnelers.
**Instance types**: c5.xlarge across the board.

**What it tests**: Accuracy of identity connection status reporting. Unique pattern: chaos stops (rather than
restarts) components, validates status reporting, waits 2 minutes, validates again, then restarts everything
and validates a third time. Tests both down-detection and recovery.

**Chaos injection**: 1-2 controllers stopped + 10-75% routers stopped + 10-75% hosts stopped.

**Validation**: `chaos.SelectRandom` (range-based), `chaos.StopSelected`, custom `validateSdkStatus` via
management channel, `edge.ControllerAvailable`.

**Iteration**: sowChaos -> validate -> sleep2Min -> validate -> ensureAllStarted -> validateUp -> validate

## 7. Private Controller Test (`private-ctrl-test`)
**Regions**: 2 (us-east-1 public, us-west-2 private subnet), 3 controllers (1 public, 2 private HA).
**Scale**: 2 main routers (1/region) + 15 scaled lifecycle routers (10 east, 5 west). Mixed public/private security groups.
**Instance types**: t3.medium.

**What it tests**: Private controller cluster architecture with controllers in a private subnet. Exercises
multi-region Raft consensus with mixed public/private controllers, lifecycle router state transitions
(Absent/Active/Quiescing), controller-to-controller communication under network disruption, and circuit
validation from each controller's perspective. Loop client/host SDKs generate throughput (5000 txRequests,
1ms pacing) and latency (1 txRequest, 400 iterations) workloads.

**Chaos injection**: 7 scenarios - random controller restarts + 10-100% of test routers restarted + lifecycle
router subset + **connection disruption on non-restarted components** (iptables blocks on ports 6262, 6263).

**Validation**: custom `validateClusterConnectivity` (checks Raft peers via `checkConnectedPeers`, router
connections via `checkConnectedRouters`), `validations.ValidateControllerDialers`, `validations.ValidateTerminators`,
`validations.ValidateCircuits` per-controller with router filtering, `RunSimScenarios`.

**Iteration**: transitionLifecycleRouters -> sowChaos -> validateUp -> validateClusterConnectivity ->
validateControllerDialers -> validateTerminators -> runSimScenario -> validateCircuits

## 8. OIDC Auth Test (`oidc-auth-test`)
**Regions**: 3 (us-east-1, eu-west-2, ap-southeast-2), 3 HA controllers with `InitOidc` setup.
**Scale**: 2 ERT routers + 2 client-facing routers + 1 SDK-hosting router per region. Go SDK direct clients:
2 hosts/region x 100 = 600. C-SDK proxy clients: 18 hosts/region x 100 = 5,400. Total **~6,000 clients**.
SDK hosts: 5 Go + 10 ZET/C-SDK per region, each hosting 5 services. Services: svc-go (45 terminators),
svc-zet (90 terminators), svc-ert (6 terminators) = ~140 total. 6,000 expected OIDC sessions.
**Instance types**: c5.2xlarge controllers, c5.xlarge routers/hosts/clients.

**What it tests**: OIDC authentication and session management across a heterogeneous client fleet
(Go SDK, C-SDK/ZET, ERT). Validates login event collection via event forwarder, traffic success per client,
terminator counts for mixed service types, and client identity tracking across restarts. Driven by the current
oidc-auth-test branch.

**Chaos injection**: 7 scenarios - controller restarts + 10-100% routers **stopped** (clean OIDC session
transitions) + full restart of all clients (go-client and prox-c).

**Validation**: `chaos.SelectRandom`, `chaos.RestartSelected` (controllers), `chaos.StopSelected` (routers),
custom `buildClientIdentityRegistry` and `restartAllClients`, `validations.ValidateServiceTerminators`,
`validations.OidcEventCollector`, `validations.TrafficResultsCollector`, `component_event_forwarder`,
`component_oidc_test_client`, debug server for troubleshooting.

**Iteration**: transitionLifecycleRouters -> sowChaos -> validateUp -> restartAllClients ->
validateServiceTerminators -> validateCircuits -> validateOidcEvents -> validateTraffic

---

# Gap Analysis

## Currently Well-Covered
- Link mesh convergence at scale (links-test)
- Terminator convergence: SDK + ERT + mixed (sdk-hosting-test, ert-hosting-test, oidc-auth-test)
- Router data model sync under churn (router-data-model-test)
- Circuit state consistency across controllers (circuit-test, private-ctrl-test)
- Identity status reporting with down/recovery detection (sdk-status-test)
- Controller restart resilience, including mixed public/private (private-ctrl-test)
- OIDC auth and session lifecycle (oidc-auth-test)
- Inter-controller connectivity under iptables disruption (private-ctrl-test via `chaos/partition.go`)

## High Priority - Testing Infrastructure

### TODO: Metrics Anomaly Detection Framework
**Why this is high priority**: Today the tests assert on *functional* state (terminator counts, link presence,
circuit consistency) but have no systematic way to catch *performance* or *resource* regressions. A leak, a
goroutine explosion, a latency cliff, or a CPU pathology can pass the functional checks and remain invisible
until production. This is a cross-cutting gap that affects every test above.

**What to build**:
- Baseline collection: record per-scenario metric distributions (memory, goroutine count, FD count, CPU,
  channel send latencies, raft commit latencies, session throughput, forwarder queue depths) on known-good runs.
- Anomaly detection: compare live runs against baselines using sliding windows; flag percentile deviations,
  unbounded growth trends (linear memory growth over iterations), and distribution shape changes.
- Hooks into existing tests: leverage the per-iteration structure so every existing test can opt into
  anomaly checks without changing its chaos logic.
- Report artifacts: alongside pass/fail, emit a summary of which metrics drifted, at what iteration, and
  by how much. Integrates with the validation utilities in `zititest/zitilab/runlevel/5_operation/`.
- Consider time-series export (Prometheus/Grafana) alongside the existing metricbeat integration.

## Recommended New Test Areas

### 1. HA Leader Election & Failover (Chaos)
Current tests restart random controllers but don't target the Raft leader specifically. A dedicated test should:
- Identify and kill the leader repeatedly.
- Validate client operations (session creation, service access) continue with minimal disruption.
- Measure failover time.
- Test split-brain scenarios by partitioning network between controller nodes (extends the iptables
  approach from `chaos/partition.go` already used in private-ctrl-test).

### 2. Circuit Failover & Reroute (Reliability)
The circuit test validates consistency but doesn't focus on mid-circuit disruption. A test should:
- Establish long-running data flows through specific routers.
- Kill routers mid-flow and validate smart reroute kicks in.
- Measure data loss and reconnection time.
- Test circuit rerouting when the only path involves a multi-hop alternative.

**NOTE:** This testing will be more valuable once we can reroute from the edge, otherwise we can only test that rerouting handles intermediary routers, not initiating or terminating routers.

### 3. Session Scale & Churn (Scale/Performance)
No current test exercises massive concurrent API sessions. A test should:
- Create thousands of concurrent client API sessions (oidc-auth-test proves ~6K clients is feasible).
- Continuously churn sessions (create/delete) while maintaining active circuits.
- Measure session creation latency under load.
- Validate session cleanup doesn't leak resources (ties in with metrics anomaly detection).

### 4. Policy Evaluation at Scale (Performance)
The router data model test creates 100 of each policy type but doesn't stress the policy evaluation engine.
A test should:
- Create thousands of overlapping service policies, edge router policies, and posture checks.
- Measure time-to-evaluate for dial/bind decisions.
- Test policy changes propagating to active sessions (e.g., revoking access mid-session).

### 5. Enrollment Storm (Scale/Reliability)
No test exercises mass enrollment. A test should:
- Enroll hundreds of routers and identities concurrently.
- Validate PKI operations don't become a bottleneck.
- Test certificate rotation under load.

### 6. Link Flapping (Chaos)
The links test validates static convergence. A link flap test should:
- Use iptables/tc to simulate intermittent link failures between specific routers (extends
  `chaos/partition.go`).
- Validate the system doesn't oscillate (link up/down thrashing).
- Test that circuits are stable when underlying links briefly drop and recover.
- Measure impact on active data flows.

### 7. Event/Metrics Pipeline Backpressure (Reliability)
oidc-auth-test exercises the event forwarder but not under stress. A dedicated test should:
- Configure multiple event subscribers (AMQP, file, etc.).
- Generate massive event volumes via circuit/session churn.
- Validate events aren't dropped and the system doesn't deadlock.
- Test slow subscriber behavior (does it back-pressure the control plane?).

### 8. Database Performance Under Scale (Performance)
The BBolt database is single-writer. A test should:
- Load the database with realistic production-scale data (10K+ services, 50K+ identities).
- Measure API response times for list/query operations.
- Test database migration performance.
- Validate backup/restore operations under load.

### 9. Network Partition Tolerance (Chaos)
private-ctrl-test uses iptables for inter-controller disruption, but broader partitions aren't tested:
- Partition routers from controllers while maintaining router-to-router links.
- Partition between HA controller nodes in non-private topologies.
- Validate behavior when a router can reach some controllers but not others.
- Test reconnection behavior after partition heals.

### 10. Long-Running Stability (Reliability/Soak)
The validation loop runs for 2 hours. A dedicated soak test should:
- Run for 24-48 hours with continuous traffic.
- Monitor memory growth (goroutine/connection leaks) - pairs with metrics anomaly detection.
- Track latency percentiles over time for degradation.
- Exercise certificate expiration and rotation during the soak.
