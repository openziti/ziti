# Release 1.2.2

## What's New

* Bug fixes and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/secretstream: [v0.1.25 -> v0.1.26](https://github.com/openziti/secretstream/compare/v0.1.25...v0.1.26)
* github.com/openziti/storage: [v0.3.6 -> v0.3.8](https://github.com/openziti/storage/compare/v0.3.6...v0.3.8)
    * [Issue #87](https://github.com/openziti/storage/issues/87) - negative URL filter returns incorrect results

* github.com/openziti/ziti: [v1.2.1 -> v1.2.2](https://github.com/openziti/ziti/compare/v1.2.1...v1.2.2)
    * [Issue #2559](https://github.com/openziti/ziti/issues/2559) - expired JWTs are allowed to enroll
    * [Issue #2543](https://github.com/openziti/ziti/issues/2543) - Support adding adding an uninitialized node to a cluster (rather than the reverse)


# Release 1.2.1

## What's New

* Bug fixes and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.19 -> v1.0.20](https://github.com/openziti/agent/compare/v1.0.19...v1.0.20)
* github.com/openziti/channel/v3: [v3.0.10 -> v3.0.16](https://github.com/openziti/channel/compare/v3.0.10...v3.0.16)
* github.com/openziti/foundation/v2: [v2.0.50 -> v2.0.52](https://github.com/openziti/foundation/compare/v2.0.50...v2.0.52)
* github.com/openziti/identity: [v1.0.88 -> v1.0.90](https://github.com/openziti/identity/compare/v1.0.88...v1.0.90)
* github.com/openziti/metrics: [v1.2.59 -> v1.2.61](https://github.com/openziti/metrics/compare/v1.2.59...v1.2.61)
* github.com/openziti/runzmd: [v1.0.53 -> v1.0.55](https://github.com/openziti/runzmd/compare/v1.0.53...v1.0.55)
* github.com/openziti/storage: [v0.3.2 -> v0.3.6](https://github.com/openziti/storage/compare/v0.3.2...v0.3.6)
* github.com/openziti/transport/v2: [v2.0.150 -> v2.0.153](https://github.com/openziti/transport/compare/v2.0.150...v2.0.153)
* github.com/openziti/ziti: [v1.2.0 -> v1.2.1](https://github.com/openziti/ziti/compare/v1.2.0...v1.2.1)
    * [Issue #2543](https://github.com/openziti/ziti/issues/2543) - Support adding adding an uninitialized node to a cluster (rather than the reverse)
    * [Issue #2541](https://github.com/openziti/ziti/issues/2541) - Add cluster id, to prevent merging disparate clusters
    * [Issue #2532](https://github.com/openziti/ziti/issues/2532) - When adding an existing HA cluster member, remove/add if suffrage has changed
    * [Issue #2217](https://github.com/openziti/ziti/issues/2217) - Controller list is empty until peers connect
    * [Issue #2533](https://github.com/openziti/ziti/issues/2533) - Handle concurrent raft connections
    * [Issue #2534](https://github.com/openziti/ziti/issues/2534) - Ziti ID with leading hyphen causes command-line parameter ambiguity
    * [Issue #2528](https://github.com/openziti/ziti/issues/2528) - Updated router costs are not use when evaluating current path cost in the context of smart rerouting

# Release 1.2.0

## What's New

* New Router Metrics
* Changes to identity connect status
* HA Bootstrap Changes
* Connect Events
* SDK Events
* Bug fixes and other HA work

## New Router Metrics

The following new metrics are available for edge routers:

1. edge.connect.failures - meter tracking failed connect attempts from sdks
   This tracks failures to not having a valid token. Other failures which 
   happen earlier in the connection process may not be tracked here.
2. edge.connect.successes - meter tracking successful connect attempts from sdks
3. edge.disconnects - meter tracking disconnects of previously successfully connected sdks
4. edge.connections - gauge tracking count of currently connected sdks

## Identity Connect Status

Ziti tracks whether an identity is currently connected to an edge router. 
This is the `hasEdgeRouterConnection` field on Identity. 

Identity connection status used to be driven off of heartbeats from the edge router.
This feature doesn't work correctly when running with controller HA. 

To address this, while also providing more operation insight, connect events were added
(see below for more details on the events themselves).

The controller can be configured to use status from heartbeats, connect events or both.
If both are used as source, then if either reports the identity as connected, then it 
will show as connected. This is intended for when you have a mix of routers and they
don't all yet supported connect events.

The controller now also aims to be more precise about identity state. There is a new
field on Identity: `edgeRouterConnectionStatus`. This field can have one of three
values:

* offline
* online
* unknown

If the identity is reported as connected to any ER, it will be marked as `online`. 
If the identity has been reported as connected, but the reporting ER is now
offline, the identity may still be connected to the ER. While in this state
it will be marked as 'unknown'. After a configurable interval, it will be marked
as offline.

New controller config options:

```
identityStatusConfig:
  # valid values ['heartbeats', 'connect-events', 'hybrid']
  # defaults to 'hybrid' for now
  source: connect-events 

  # determines how often we scan for disconnected routers
  # defaults to 1 minute
  scanInterval: 1m

  # determines how long an identity will stay in unknown status before it's marked as offline
  # defaults to 5m
  unknownTimeout: 5m
```
  
## HA Bootstrapping Changes

Previously bootstrapping the RAFT cluster and initializing the controller with a 
default administrator were separate operations.
Now, the raft cluster will be bootstrapped whenever the controller is initialized. 

The controller can be initialized as follows:

1. Using `ziti agent controller init`
2. Using `ziti agent controller init-from-db`
3. Specifying a `db:` entry in the config file. This is equivalent to using `ziti agent controller init-from-db`.

Additionally:

1. `minClusterSize` has been removed. The cluster will always be initialized with a size of 1.
2. `bootstrapMembers` has been renamed to `initialMembers`. If `initialMembers` are specified,
   the bootstrapping controller will attempt to add them after bootstrap has been complete. If
   they are invalid they will be ignored. If they can't be reached (because they're not running
   yet), the controller will continue to retry until they are reached, or it is restarted.


## Connect Events

These are events generated when a successful connection is made to a controller, from any of:

1. Identity, using the REST API
2. Router
3. Controller (peer in an HA cluster)

They are also generated when an SDK connects to a router. 

**Controller Configuration**

```yml
events:
  jsonLogger:
    subscriptions:
      - type: connect
    handler:
      type: file
      format: json
      path: /tmp/ziti-events.log
```

**Router Configuration**
```yml
connectEvents:
  # defaults to true. 
  # If set to false, minimal information about which identities are connected will still be 
  # sent to the controller, so the `edgeRouterConnectionStatus` field can be populated, 
  # but connect events will not be generated.
  enabled: true

  # The interval at which connect information will be batched up and sent to the controller. 
  # Shorter intervals will improve data resolution on the controller. Longer intervals could
  # more efficient.
  batchInterval: 3s

  # The router will also periodically sent the full state to the controller, to ensure that 
  # it's in sync. It will do this automatically if the router gets disconnected from the 
  # controller, or if the router is unable to send a connect events messages to the controller.
  # This controls how often the full state will be sent under ordinairy conditions
  fullSyncInterval: 5m

  # If enabled is set to true, the router will collect connect events and send them out
  # at the configured batch interval. If there are a huge number of connecting identities
  # or if the router is disconnected from the controller for a time, it may be unable to
  # send events. In order to prevent queued events from exhausting memory, a maximum 
  # queue size is configured. 
  # Default value 100,000
  maxQueuedEvents: 100000
  
```

**Example Events**

```json
{
  "namespace": "connect",
  "src_type": "identity",
  "src_id": "ji2Rt8KJ4",
  "src_addr": "127.0.0.1:59336",
  "dst_id": "ctrl_client",
  "dst_addr": "localhost:1280/edge/management/v1/edge-routers/2L7NeVuGBU",
  "timestamp": "2024-10-02T12:17:39.501821249-04:00"
}
```

```json
{
  "namespace": "connect",
  "src_type": "router",
  "src_id": "2L7NeVuGBU",
  "src_addr": "127.0.0.1:42702",
  "dst_id": "ctrl_client",
  "dst_addr": "127.0.0.1:6262",
  "timestamp": "2024-10-02T12:17:40.529865849-04:00"
}
```

```json
{
  "namespace": "connect",
  "src_type": "peer",
  "src_id": "ctrl2",
  "src_addr": "127.0.0.1:40056",
  "dst_id": "ctrl1",
  "dst_addr": "127.0.0.1:6262",
  "timestamp": "2024-10-02T12:37:04.490859197-04:00"
}
```

## SDK Events

Building off of the connect events, there are events generated when an identity/sdk comes online or goes offline.

```yml
events:
  jsonLogger:
    subscriptions:
      - type: sdk
    handler:
      type: file
      format: json
      path: /tmp/ziti-events.log
```

```json
{
  "namespace": "sdk",
  "event_type" : "sdk-online",
  "identity_id": "ji2Rt8KJ4",
  "timestamp": "2024-10-02T12:17:39.501821249-04:00"
}
```

```json
{
  "namespace": "sdk",
  "event_type" : "sdk-status-unknown",
  "identity_id": "ji2Rt8KJ4",
  "timestamp": "2024-10-02T12:17:40.501821249-04:00"
}
```

```json
{
  "namespace": "sdk",
  "event_type" : "sdk-offline",
  "identity_id": "ji2Rt8KJ4",
  "timestamp": "2024-10-02T12:17:41.501821249-04:00"
}
```

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.18 -> v1.0.19](https://github.com/openziti/agent/compare/v1.0.18...v1.0.19)
* github.com/openziti/channel/v3: [v3.0.5 -> v3.0.10](https://github.com/openziti/channel/compare/v3.0.5...v3.0.10)
* github.com/openziti/edge-api: [v0.26.32 -> v0.26.35](https://github.com/openziti/edge-api/compare/v0.26.32...v0.26.35)
* github.com/openziti/foundation/v2: [v2.0.49 -> v2.0.50](https://github.com/openziti/foundation/compare/v2.0.49...v2.0.50)
* github.com/openziti/identity: [v1.0.85 -> v1.0.88](https://github.com/openziti/identity/compare/v1.0.85...v1.0.88)

* github.com/openziti/metrics: [v1.2.58 -> v1.2.59](https://github.com/openziti/metrics/compare/v1.2.58...v1.2.59)
* github.com/openziti/runzmd: [v1.0.51 -> v1.0.53](https://github.com/openziti/runzmd/compare/v1.0.51...v1.0.53)
* github.com/openziti/sdk-golang: [v0.23.43 -> v0.23.44](https://github.com/openziti/sdk-golang/compare/v0.23.43...v0.23.44)
* github.com/openziti/transport/v2: [v2.0.146 -> v2.0.150](https://github.com/openziti/transport/compare/v2.0.146...v2.0.150)
* github.com/openziti/ziti: [v1.1.15 -> v1.2.0](https://github.com/openziti/ziti/compare/v1.1.15...v1.2.0)
    * [Issue #2212](https://github.com/openziti/ziti/issues/2212) - Rework distributed control bootstrap mechanism
    * [Issue #1835](https://github.com/openziti/ziti/issues/1835) - Add access log for rest and router connections
    * [Issue #2234](https://github.com/openziti/ziti/issues/2234) - Emit an event when hasEdgeRouterConnection state changes for an Identity
    * [Issue #2506](https://github.com/openziti/ziti/issues/2506) - Identity service config overrides referential integrity issues
    * [Issue #2491](https://github.com/openziti/ziti/issues/2491) - fix router CSR loading
    * [Issue #2478](https://github.com/openziti/ziti/issues/2478) - JWT signer secondary auth: not enough information to continue
    * [Issue #2482](https://github.com/openziti/ziti/issues/2482) - router run command - improperly binds 127.0.0.1:53/udp when tunnel mode is not tproxy
    * [Issue #2474](https://github.com/openziti/ziti/issues/2474) - Enable Ext JWT Enrollment/Generic Trust Bootstrapping
    * [Issue #2471](https://github.com/openziti/ziti/issues/2471) - Service Access for Legacy SDKs in  HA does not work
    * [Issue #2468](https://github.com/openziti/ziti/issues/2468) - enrollment signing cert is not properly identified
