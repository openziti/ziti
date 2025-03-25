# Release 1.5.0

## What's New

* Bug fixes and features
* Change to router endpoints file default name
* Updated Cluster Defaults
* Updates to terminator costing
* Router metrics changes

## Router Endpoints File

The router endpoints file used to have a default name of `endpoints`. 
As it is a YAML file, the file now has a default name of `endpoints.yml`. 
This may affect existing setups running the beta HA code. If this is
a concern, the endpoints file path can be configured:

```
ctrl:
  endpointsFile: /path/to/endpoints.file
```

## Updated Cluster Defaults

The following defaults have changed.

```
cluster:
  # How many updates before creating a new snapshot. 
  #
  # New default: 500
  # Old default: 8192
  snapshotThreshold: 500 

  # How many old entries to keep around, so that a stream of
  # entries can be sent to sync peers, instead of sending an
  # entire snapshot
  #
  # New default: 500
  # Old default: 10240
  trailingLogs: 500
```

## Terminator Costing Changes

When a terminator is selected for a service dial, and that dial results in a failure, a failure cost
is added to that terminator. This will bias future dials towards other terminators, if they are available.

The failure cost can be reduced by successful dials. Failure cost is also reduced over time. In previous
releases this was a fixed credit of 5, every minute. This is now changing to an exponential 
amount, based on time since the last failure. 

If X is minutes since last failure, the credit is now: `min(10, 2 ^ (X/5))`.

## Router Metrics Changes

There are four new router metrics, focused on visibility into flow control.

* `xgress.blocked_by_local_window_rate` - meter which ticks whenever an xgress becomes blocked by the local window being full
* `xgress.blocked_by_remote_window_rate` - meter which ticks whenever an xgress becomes blocked by the remote receive buffer being full
* `xgress.blocked_time` - timer which tracks how long xgresses are in a blocked state. 
* `xgress_edge.long_data_queue_time` - timer that tracks times to process incoming data payloads to `xgress_edge`. 

The `xgress_edge.long_data_queue_time` will be controller by a router config file setting. It will default to disabled. The other metrics will always be enabled.

Router metrics has two new configuration setting:

```
metrics:
  # Number of usage events to be able to queue. Defaults to 256. If this queue backs up, it can
  # slow down dispatch of data from an SDK onto the fabric.
  eventQueueSize: 256

  # If set to true, enables the xgress_edge.long_data_queue_time metric. Defaults to false.
  enableDataDelayMetric: false
```

## Component Updates and Bug Fixes

* github.com/openziti/channel/v3: [v3.0.37 -> v3.0.39](https://github.com/openziti/channel/compare/v3.0.37...v3.0.39)
* github.com/openziti/edge-api: [v0.26.41 -> v0.26.42](https://github.com/openziti/edge-api/compare/v0.26.41...v0.26.42)
* github.com/openziti/foundation/v2: [v2.0.58 -> v2.0.59](https://github.com/openziti/foundation/compare/v2.0.58...v2.0.59)
* github.com/openziti/metrics: [v1.2.69 -> v1.3.0](https://github.com/openziti/metrics/compare/v1.2.69...v1.3.0)
    * [Issue #49](https://github.com/openziti/metrics/issues/49) - Make usage registry event queue size configurable
    * [Issue #50](https://github.com/openziti/metrics/issues/50) - Do metrics message construction in msg sender goroutine rather than usage/interval event goroutine

* github.com/openziti/runzmd: [v1.0.65 -> v1.0.67](https://github.com/openziti/runzmd/compare/v1.0.65...v1.0.67)
* github.com/openziti/secretstream: [v0.1.31 -> v0.1.32](https://github.com/openziti/secretstream/compare/v0.1.31...v0.1.32)
* github.com/openziti/storage: [v0.4.5 -> v0.4.7](https://github.com/openziti/storage/compare/v0.4.5...v0.4.7)
* github.com/openziti/transport/v2: [v2.0.165 -> v2.0.167](https://github.com/openziti/transport/compare/v2.0.165...v2.0.167)
* github.com/openziti/ziti: [v1.4.3 -> v1.5.0](https://github.com/openziti/ziti/compare/v1.4.3...v1.5.0)
    * [Issue #2910](https://github.com/openziti/ziti/issues/2910) - Add additional metrics for visibility into flow control backpressure
    * [Issue #2608](https://github.com/openziti/ziti/issues/2608) - Verify latest Prometheus server
    * [Issue #2899](https://github.com/openziti/ziti/issues/2899) - Allow configuring size of router metrics event queue size
    * [Issue #2896](https://github.com/openziti/ziti/issues/2896) - `ziti router run --extend` does not function
    * [Issue #2796](https://github.com/openziti/ziti/issues/2796) - Generated API client enrollment operations fail
    * [Issue #2889](https://github.com/openziti/ziti/issues/2889) - Ensure identity online/offline statuses work correctly for ER/Ts
    * [Issue #2891](https://github.com/openziti/ziti/issues/2891) - Restore can panic if using import from db
    * [Issue #2835](https://github.com/openziti/ziti/issues/2835) - Add mechanism for selecting CLI layout
    * [Issue #2836](https://github.com/openziti/ziti/issues/2836) - Add run subcommand
    * [Issue #2837](https://github.com/openziti/ziti/issues/2837) - Add enroll subcommand
    * [Issue #2851](https://github.com/openziti/ziti/issues/2851) - Change terminator failure cost crediting to be exponential based on time since last failure
    * [Issue #2854](https://github.com/openziti/ziti/issues/2854) - Fix controller online status
    * [Issue #2829](https://github.com/openziti/ziti/issues/2829) - Update Raft Configuration Defaults
    * [Issue #2849](https://github.com/openziti/ziti/issues/2849) - Router endpoints file should have .yml extension by default
    * [Issue #2875](https://github.com/openziti/ziti/issues/2875) - add --authenticate to `verify ext-jwt-signer oidc`
    * [Issue #2873](https://github.com/openziti/ziti/issues/2873) - updates to `verify ext-jwt-signer oidc`

# Release 1.4.3

## What's New

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v1.4.2 -> v1.4.3](https://github.com/openziti/ziti/compare/v1.4.2...v1.4.3)
  * [Issue #2865](https://github.com/openziti/ziti/issues/2865) - Connections w/ API Session Certs Fail To Dial

# Release 1.4.2

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v1.4.1 -> v1.4.2](https://github.com/openziti/ziti/compare/v1.4.1...v1.4.2)
    * [Issue #2860](https://github.com/openziti/ziti/issues/2860) - router healtcheck with invalid address logs error but still doesn't listen
    
# Release 1.4.1

## What's New

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v1.4.1 -> v1.5.0](https://github.com/openziti/ziti/compare/v1.4.1...v1.5.0)
    * [Issue #2854](https://github.com/openziti/ziti/issues/2854) - Fix controller online status
    * [Issue #2829](https://github.com/openziti/ziti/issues/2829) - Update Raft Configuration Defaults
    * [Issue #2849](https://github.com/openziti/ziti/issues/2849) - Router endpoints file should have .yml extension by default

# Release 1.4.0

## What's New

* Changes to backup/restore and standalone to HA migrations
* Use `cluster` consistently for cluster operations
* Event Doc and Consistency
* ziti ops verify changes
    * Moved `ziti ops verify-network` to `ziti ops verify network`
    * Moved `ziti ops verify traffic` to `ziti ops verify traffic`
    * Added `ziti ops verify ext-jwt-signer oidc` to help users with configuring OIDC external auth
    * Added `ziti ops verify ext-jwt-signer oidc` to help users with configuring OIDC external auth 
* Router Controller Endpoint Changes
* Bug fixes

## Config Changes

**NOTE:** For HA configuration, the `raft:` config stanza is now named `cluster:`.

Example:

```yaml
cluster:
  dataDir: ./data
```

## Event Doc and Consistency

The event types are now exhaustively documented as part of the [OpenZiti Reference Documentation](https://openziti.io/docs/reference/events).

During documentation, some inconsistencies were found the following changes were made.

### Namespace Cleanup

Namespaces have been cleaned up, with the following changes:

* edge.apiSessions -> apiSession
* fabric.circuits -> circuit
* edge.entityCount -> entityCount
* fabric.links -> link
* fabric.routers -> router
* services -> service
* edge.sessions -> session
* fabric.terminators -> terminator
* fabric.usage -> usage

Note that service events used `services` in the config file, but `service.events` in the event itself.
The old namespaces still work. If the old event type is used in the config file, the old namespace will be in the events as well

### Timestamp field

The following event types now have a timestamp field:

* service
* usage

This timestamp is the time the event was generated.

### Event Source ID

All event types now have a new field: `event_src_id`. This field is the id of the controller
which emitted the event. This may be useful in HA environments, during event processing.

## Cluster Operations Naming

The CLI tools under `ziti fabric raft` are now found at `ziti ops cluster`.

The Raft APIs available in the fabric management API are now namespaced under Cluster instead.

## Backup/Restore/HA Migrations

What restoring from a DB snapshot has in common with migrating from a standalone setup to
a RAFT enabled one, is that the controller is changing in a way that the router might not
notice.

Now that routers have a simplified data model, they need know if the controller database
has gone backwards. In the case of a migration to an HA setup, they need to know that
the data model index has changed, likely resetting back to close to zero.

To facilitate this, the database now has a timeline identifier. This is shared among
controllers and is sent to routers along with the data state. When the controller
restores to a snapshot of previous state, or when the the controller moves to a
raft/HA setup, the timeline identifier will change.

When the router requests data model changes, it will send along the current timeline
identifier. If the controller sees that the timeline identifier is different, it knows
to send down the full data state.

### Implementation Notes

In general this is all handled behind the scenes. The current data model index and
timeline identifier can be inspected on controllers and routers using:

```
ziti fabric inspect router-data-model-index
```

**Example**

```
$ ziti fabric inspect router-data-model-index
Results: (3)
ctrl1.router-data-model-index
index: 25
timeline: MMt19ldHR

vEcsw2kJ7Q.router-data-model-index
index: 25
timeline: MMt19ldHR

ctrl2.router-data-model-index
index: 25
timeline: MMt19ldHR
```

Whenever we create a database snapshot now, the snapshot will contain a flag indicating
that the timeline identifier needs to be changed. When a standalone controller starts
up, if that flag is set, the controller changes the timeline identifier and resets the flag.

When an HA cluster is initialized using an existing controller database it also changes the
timeline id.

### HA DB Restore

There's a new command to restore an HA cluster to an older DB snapshot.

```
ziti agent controller restore-from-db </path/to/database.file>
```

Note that when a controller is already up and running and receives a snapshot to apply, it
will move the database into place and then shutdown, expecting to be restarted. This is
because there is caching in various places and restartingi makes sure that everything is
coherent with the changes database.

## Router Controller Endpoint Updates

### Endpoints File Config

The config setting for controller the endpoints file location has changed.

It was:

```
ctrl:
  dataDir: /path/to/dir
```

The endpoints file would live in that directory but always be called endpoints.

This is replaced by a more flexible `endpointsFile`.

```
ctrl:
  endpointsFile: /path/to/endpoints.file
```

The default location is unchanged, which is a file named `endpoints` in the same
directory as the router config file.

### Enrollment

The router enrollment will now contain the set of known controllers at the time
the router as enrollled. This also works for standalone controllers, as long as
the `advertiseAddress` settings is set.

Example

```
ctrl:
  listener: tls:0.0.0.0:6262
  options:
    advertiseAddress: tls:ctrl1.ziti.example.com
```

This means that the controller no longer needs to be set manually in the config
file, enrollment should handle initializing the value appropriately.

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.23 -> v1.0.26](https://github.com/openziti/agent/compare/v1.0.23...v1.0.26)
* github.com/openziti/channel/v3: [v3.0.26 -> v3.0.37](https://github.com/openziti/channel/compare/v3.0.26...v3.0.37)
    * [Issue #168](https://github.com/openziti/channel/issues/168) - Add DisconnectHandler to reconnecting channel

* github.com/openziti/edge-api: [v0.26.38 -> v0.26.41](https://github.com/openziti/edge-api/compare/v0.26.38...v0.26.41)
* github.com/openziti/foundation/v2: [v2.0.56 -> v2.0.58](https://github.com/openziti/foundation/compare/v2.0.56...v2.0.58)
* github.com/openziti/identity: [v1.0.94 -> v1.0.100](https://github.com/openziti/identity/compare/v1.0.94...v1.0.100)
* github.com/openziti/metrics: [v1.2.65 -> v1.2.69](https://github.com/openziti/metrics/compare/v1.2.65...v1.2.69)
* github.com/openziti/runzmd: [v1.0.59 -> v1.0.65](https://github.com/openziti/runzmd/compare/v1.0.59...v1.0.65)
* github.com/openziti/sdk-golang: [v0.23.44 -> v0.24.1](https://github.com/openziti/sdk-golang/compare/v0.23.44...v0.24.1)
    * [Issue #673](https://github.com/openziti/sdk-golang/issues/673) - Add license check to GH workflow
    * [Issue #663](https://github.com/openziti/sdk-golang/issues/663) - Add API to allow controlling proxying connections to controllers and routers.
    * [Issue #659](https://github.com/openziti/sdk-golang/issues/659) - E2E encryption can encounter ordering issues with high-volume concurrent writes

* github.com/openziti/secretstream: [v0.1.28 -> v0.1.31](https://github.com/openziti/secretstream/compare/v0.1.28...v0.1.31)
* github.com/openziti/storage: [v0.3.15 -> v0.4.5](https://github.com/openziti/storage/compare/v0.3.15...v0.4.5)
    * [Issue #94](https://github.com/openziti/storage/issues/94) - Snapshots aren't working correctly

* github.com/openziti/transport/v2: [v2.0.159 -> v2.0.165](https://github.com/openziti/transport/compare/v2.0.159...v2.0.165)
* github.com/openziti/xweb/v2: [v2.1.3 -> v2.2.1](https://github.com/openziti/xweb/compare/v2.1.3...v2.2.1)
    * [Issue #18](https://github.com/openziti/xweb/issues/18) - verify advertised host/ip has a certificate defined in the identity block

* github.com/openziti/ziti: [v1.3.3 -> v1.4.0](https://github.com/openziti/ziti/compare/v1.3.3...v1.4.0)
    * [Issue #2807](https://github.com/openziti/ziti/issues/2807) - Cache ER/T terminator ids in the router for faster restarts
    * [Issue #2288](https://github.com/openziti/ziti/issues/2288) - Edge router/tunneler hosting Chaos Test
    * [Issue #2821](https://github.com/openziti/ziti/issues/2821) - Add --human-readable and --max-depth options to ziti ops db du
    * [Issue #2742](https://github.com/openziti/ziti/issues/2742) - Add event when non-member peer connects and doesn't join
    * [Issue #2738](https://github.com/openziti/ziti/issues/2738) - Cluster operations should return 503 not 500 if there's no leader
    * [Issue #2712](https://github.com/openziti/ziti/issues/2712) - /version is missing OIDC API
    * [Issue #2785](https://github.com/openziti/ziti/issues/2785) - Peer data model state not always updated
    * [Issue #2737](https://github.com/openziti/ziti/issues/2737) - OIDC issue mismatch with alt server certs
    * [Issue #2774](https://github.com/openziti/ziti/issues/2774) - API Session Certificate SPIFFE IDs fail validation in Routers
    * [Issue #2672](https://github.com/openziti/ziti/issues/2672) - [Bug] Posture check PUT method doesn't update nested structures but works fine with PATCH
    * [Issue #2668](https://github.com/openziti/ziti/issues/2668) - [Feature Request] Filterable field for posture check type
    * [Issue #2681](https://github.com/openziti/ziti/issues/2681) - Support specifying which token to use on external jwt signers
    * [Issue #2756](https://github.com/openziti/ziti/issues/2756) - Remove ziti agent cluster init-from-db command
    * [Issue #2723](https://github.com/openziti/ziti/pull/2723) - attempts to probe advertise address on startup to ensure the SANS is correct
    * [Issue #2722](https://github.com/openziti/ziti/issues/2722) - router: check advertised address on startup
    * [Issue #2745](https://github.com/openziti/ziti/issues/2745) - Remove cluster initialMembers config
    * [Issue #2746](https://github.com/openziti/ziti/issues/2746) - Move agent controller commands to agent cluster
    * [Issue #2743](https://github.com/openziti/ziti/issues/2743) - Agent and rest cluster command names should match
    * [Issue #2731](https://github.com/openziti/ziti/issues/2731) - Rename raft controller config section to cluster
    * [Issue #2724](https://github.com/openziti/ziti/issues/2724) - Allow configuring endpoints file full path instead of directory
    * [Issue #2728](https://github.com/openziti/ziti/issues/2728) - Write initial router endpoints file based on ctrls in JWT
    * [Issue #2108](https://github.com/openziti/ziti/issues/2108) - Add `ctrls` property to non-ha router enrollment
    * [Issue #2729](https://github.com/openziti/ziti/issues/2729) - Enrollment doesn't contain controller which created the enrollment
    * [Issue #2549](https://github.com/openziti/ziti/issues/2549) - Handle Index Non HA to HA Transitions During Upgrades
    * [Issue #2649](https://github.com/openziti/ziti/issues/2649) - Make restoring an HA cluster from a DB backup easier
    * [Issue #2707](https://github.com/openziti/ziti/issues/2707) - Ensure database restores work with RDM enabled routers
    * [Issue #2593](https://github.com/openziti/ziti/issues/2593) - Update event documentation with missing event types
    * [Issue #2720](https://github.com/openziti/ziti/issues/2720) - new verify oidc command on prints usage
    * [Issue #2546](https://github.com/openziti/ziti/issues/2546) - Use consistent terminology for HA
    * [Issue #2713](https://github.com/openziti/ziti/issues/2713) - Routers with no edge components shouldn't subscribe to RDM updates

# Release 1.3.3

## What's New

* Bug Fixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v1.3.2 -> v1.3.3](https://github.com/openziti/ziti/compare/v1.3.2...v1.3.3)
    * [Issue #2694](https://github.com/openziti/ziti/issues/2694) - Router should use router data model if it has more than one controller configured, regardless of controller configuration


# Release 1.3.2

## What's New

* Bug Fixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v1.3.1 -> v1.3.2](https://github.com/openziti/ziti/compare/v1.3.1...v1.3.2)
    * [Issue #2692](https://github.com/openziti/ziti/issues/2692) - Routers get empty ctrl list on upgrade
    * [Issue #2689](https://github.com/openziti/ziti/issues/2689) - OIDC authentication with form data requires "id" in form data, authReqeustID in query string is ignored

# Release 1.3.1

## What's New

* Bug Fixes

## Component Updates and Bug Fixes


* github.com/openziti/ziti: [v1.3.0 -> v1.3.1](https://github.com/openziti/ziti/compare/v1.3.0...v1.3.1)
    * [Issue #2682](https://github.com/openziti/ziti/issues/2682) - HA Controller panics when bootstrapping by setting the db variable in the configuration
    * [Issue #2683](https://github.com/openziti/ziti/issues/2683) - Controller fails to save peer configuration on a fresh install
    * [Issue #2684](https://github.com/openziti/ziti/issues/2684) - Controller emits duplicate cluster events on startup

# Release 1.3.0

## What's New

* Router Data Model enabled by default
* Bug fixes
* Controller Health Check HA Update (from @nenkoru)

## Router Data Model

As part of the controller HA work, a stripped down version of the data model can now be distributed to the routers, 
allowing routers to make some authorization/authentication decisions. This code has existed for some time, but
after testing and validation, is now enabled by default. 

It can still be disabled at the controller level using new configuration. Note that the router data model is required
for HA functionality, so if the controller is running in HA mode, it cannot be disabled. 

```yaml
routerDataModel:
  # Controls whether routers are told to enable functionality dependent on the router data model
  # Defaults to true
  enabled: true 

  # How many model changes to buffer so that routers can be updated iteratively. If a router requests
  # data that's no longer available, it will receive the full data model
  logSize: 10000
```

## HA Changes

Routers no longer require the `ha: enabled` flag be set in the configuration. Routers should work correctly
whether connecting to HA or non-HA controllers. 

NOTE: If the controller a router is connected changes modes, specifically if the controller goes from
      supporting the router data model to not, or vice-versa, the router will shutdown so that it can
      restart with the correct mode.

## Controller Health Check HA Update

This feature was contributed by @nenkoru.

The controller health check can now optionally return information about raft and leadership when the `/controller/raft` path is provided.

```
$ curl -k https://localhost:1280/health-checks/controller/raft
{
    "data": {
        "checks": [
            {
                "healthy": true,
                "id": "bolt.read",
                "lastCheckDuration": "0s",
                "lastCheckTime": "2025-01-14T19:42:13Z"
            }
        ],
        "healthy": true
    },
    "meta": {},
    "raft": {
        "isLeader": true,
        "isRaftEnabled": true
    }
}
```

Note the `raft` section, which indicates if raft is enabled and if the queried controller is currently the leader. If the 
`controller/raft` path isn't present in the request, the result should be unchanged from previous releases. 

When querying the controller/raft health, if raft is enabled but the controller is not the leader, the check will
return an HTTP status of 429.

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.20 -> v1.0.23](https://github.com/openziti/agent/compare/v1.0.20...v1.0.23)
* github.com/openziti/channel/v3: [v3.0.16 -> v3.0.26](https://github.com/openziti/channel/compare/v3.0.16...v3.0.26)
* github.com/openziti/edge-api: [v0.26.35 -> v0.26.38](https://github.com/openziti/edge-api/compare/v0.26.35...v0.26.38)
    * [Issue #138](https://github.com/openziti/edge-api/issues/138) - management api deletes were generally not mapping 404 properly

* github.com/openziti/foundation/v2: [v2.0.52 -> v2.0.56](https://github.com/openziti/foundation/compare/v2.0.52...v2.0.56)
* github.com/openziti/identity: [v1.0.90 -> v1.0.94](https://github.com/openziti/identity/compare/v1.0.90...v1.0.94)
* github.com/openziti/metrics: [v1.2.61 -> v1.2.65](https://github.com/openziti/metrics/compare/v1.2.61...v1.2.65)
* github.com/openziti/runzmd: [v1.0.55 -> v1.0.59](https://github.com/openziti/runzmd/compare/v1.0.55...v1.0.59)
* github.com/openziti/secretstream: [v0.1.26 -> v0.1.28](https://github.com/openziti/secretstream/compare/v0.1.26...v0.1.28)
* github.com/openziti/storage: [v0.3.8 -> v0.3.15](https://github.com/openziti/storage/compare/v0.3.8...v0.3.15)
    * [Issue #91](https://github.com/openziti/storage/issues/91) - Support dashes in identifier segments after the first dot

* github.com/openziti/transport/v2: [v2.0.153 -> v2.0.159](https://github.com/openziti/transport/compare/v2.0.153...v2.0.159)
* github.com/openziti/ziti: [v1.2.2 -> v1.3.0](https://github.com/openziti/ziti/compare/v1.2.2...v1.3.0)
    * [Issue #2674](https://github.com/openziti/ziti/issues/2674) - 404 not found on well-known OIDC configuration with default ports/localhost
    * [Issue #2669](https://github.com/openziti/ziti/issues/2669) - Router api session tracker leaks memory.
    * [Issue #2659](https://github.com/openziti/ziti/issues/2659) - OIDC Login Panics On Unsupported Media Type
    * [Issue #2582](https://github.com/openziti/ziti/issues/2582) - An endpoint to determine whether a node is a raft leader
    * [Issue #2619](https://github.com/openziti/ziti/issues/2619) - Add source id to all events
    * [Issue #2644](https://github.com/openziti/ziti/issues/2644) - enhance mismapped external identity logging
    * [Issue #2636](https://github.com/openziti/ziti/issues/2636) - Enable HA smoketest
    * [Issue #2586](https://github.com/openziti/ziti/issues/2586) - Ziti Controller in HA mode doesn't update binding address in a bolt database after config changed
    * [Issue #2639](https://github.com/openziti/ziti/issues/2639) - Change cluster events namespace from fabric.cluster to cluster
    * [Issue #2184](https://github.com/openziti/ziti/issues/2184) - Add Event(s) For Controller Leader Connection State
    * [Issue #2548](https://github.com/openziti/ziti/issues/2548) - Generate a log message if the cluster is without a leader for some configurable period of time
    * [Issue #2624](https://github.com/openziti/ziti/issues/2624) - Remove uri/params from connect events
    * [Issue #2596](https://github.com/openziti/ziti/issues/2596) - Add DisableRouterDataModel config flag to controller
    * [Issue #2599](https://github.com/openziti/ziti/issues/2599) - Routers should only stream model data from one controller
    * [Issue #2232](https://github.com/openziti/ziti/issues/2232) - Standardized REST API Error For Mutation on Non-Consensus Controller
    * [Issue #2566](https://github.com/openziti/ziti/issues/2566) - Remove HA config flag from router
    * [Issue #2550](https://github.com/openziti/ziti/issues/2550) - Router Data Model Chaos Test
    * [Issue #2625](https://github.com/openziti/ziti/issues/2625) - edge sessions for an ERT may not be cleaned up when the ER/T is deleted 
    * [Issue #2591](https://github.com/openziti/ziti/issues/2591) - Split Edge APIs can cause `ziti edge login` to fail

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
{
  "namespace": "connect",
  "src_type": "router",
  "src_id": "2L7NeVuGBU",
  "src_addr": "127.0.0.1:42702",
  "dst_id": "ctrl_client",
  "dst_addr": "127.0.0.1:6262",
  "timestamp": "2024-10-02T12:17:40.529865849-04:00"
}
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

{
  "namespace": "sdk",
  "event_type" : "sdk-status-unknown",
  "identity_id": "ji2Rt8KJ4",
  "timestamp": "2024-10-02T12:17:40.501821249-04:00"
}

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

# Release 1.1.15

## What's New

* Bug fixes, enhancements and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/channel/v3: [v3.0.3 -> v3.0.5](https://github.com/openziti/channel/compare/v3.0.3...v3.0.5)
    * [Issue #146](https://github.com/openziti/channel/issues/146) - Transport options aren't being set in dialer
    * [Issue #144](https://github.com/openziti/channel/issues/144) - Add ReadAdapter utility

* github.com/openziti/edge-api: [v0.26.31 -> v0.26.32](https://github.com/openziti/edge-api/compare/v0.26.31...v0.26.32)
* github.com/openziti/sdk-golang: [v0.23.42 -> v0.23.43](https://github.com/openziti/sdk-golang/compare/v0.23.42...v0.23.43)
    * [Issue #629](https://github.com/openziti/sdk-golang/issues/629) - JWT session refresh interprets expiration date incorrectly

* github.com/openziti/secretstream: [v0.1.24 -> v0.1.25](https://github.com/openziti/secretstream/compare/v0.1.24...v0.1.25)
* github.com/openziti/ziti: [v1.1.14 -> v1.1.15](https://github.com/openziti/ziti/compare/v1.1.14...v1.1.15)
    * [Issue #2460](https://github.com/openziti/ziti/issues/2460) - Panic on JWT token refresh

# Release 1.1.14

## What's New

* Bug fixes, enhancements and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.30 -> v0.26.31](https://github.com/openziti/edge-api/compare/v0.26.30...v0.26.31)
* github.com/openziti/jwks: [v1.0.5 -> v1.0.6](https://github.com/openziti/jwks/compare/v1.0.5...v1.0.6)
* github.com/openziti/ziti: [v1.1.13 -> v1.1.14](https://github.com/openziti/ziti/compare/v1.1.13...v1.1.14)
    * [Issue #2119](https://github.com/openziti/ziti/issues/2119) - Add authentication events
    * [Issue #2424](https://github.com/openziti/ziti/issues/2424) - Enabling any health check causes WARNING to be logged
    * [Issue #2454](https://github.com/openziti/ziti/issues/2454) - Fix release archive
    * [Issue #1479](https://github.com/openziti/ziti/issues/1479) - ziti edge list ... show paginated output but no suggestions on how to go to next page
    * [Issue #1420](https://github.com/openziti/ziti/issues/1420) - ziti-cli comma+space causes unhelpful error
    * [Issue #2207](https://github.com/openziti/ziti/issues/2207) - ziti edge login --token -- gets "username and password fields are required"
    * [Issue #2444](https://github.com/openziti/ziti/issues/2444) - Change default semantic for policies created from the CLI from AllOf to AnyOf

* github.com/openziti/xweb/v2: [v2.1.2 -> v2.1.3](https://github.com/openziti/xweb/compare/v2.1.2...v2.1.3)
  * [Issue #2454](https://github.com/openziti/ziti/issues/2454) - Fix release archive
  * [Issue #2429](https://github.com/openziti/ziti/issues/2429) - Controller configurations without default Edge API binding panics 
* github.com/openziti/ziti: [v1.1.12 -> v1.1.13](https://github.com/openziti/ziti/compare/v1.1.12...v1.1.13)
  * [Issue #2427](https://github.com/openziti/ziti/issues/2427) - Add low overhead xgress protocol for DTLS links
  * [Issue #2422](https://github.com/openziti/ziti/issues/2422) - Busy first hop links should backpressure to xgress senders
  * support using "\*" in host.v1/host.v2 allowedAddresses


# Release 1.1.13

This release will not be promoted, as a test binary was unintentionally released in the release archives.


# Release 1.1.12

## What's New

* Bug fixes, enhancements and continuing progress on controller HA
* Data corruption Fix

## Data Corruption Fix

Previous to version 1.1.12, the controller would not handle changes to the policy type of a service policy.
Specifically if the type was changed from Bind -> Dial, or Dial -> Bind, a set of denormalized data would
be left behind, leaving the permissions with the old policy type. 

Example:

1. Identity A has Bind access to service B via Bind service policy C. 
2. The policy type of service policy C is changed from Bind to Dial.
3. The service list would now likely show that Identity A has Dial and Bind access to service B, instead of
  just Dial access.

### Mitigation/Fixing Bad Data

If you encounter this problem, the easiest and safest way to solve the problem is to to delete and recreate
the affected service policy.

If changing policy types is something you do on a regular basis, and can't upgrade to a version with the fix,
you can work around the issue by deleting and recreating policies, instead of updating them. 

If you're not sure if you have ever changed a policy type, there is a database integrity check tool which can
 be run which looks for data integrity errors. It is run against a running system. 

Start the check using:

```
ziti fabric db start-check-integrity
```

This kicks off the operation in the background. The status of the check can be seen using:

```
ziti fabric db check-integrity-status 
```

By default this is a read-only operation. If the read-only run reports errors, it can be run 
with the `-f` flag, which will have it try to fix errors. The data integrity errors caused
by this bug should all be fixable by the integrity checker.

```
ziti fabric db start-check-integrity -f
```

**WARNINGS**: 
* Always make a database snapshot before running the integrity checker: `ziti db fabric snapshot <optional path`
* The integrity checker can be very resource intensive, depending on the size of your data model. 
  It is recommended that you run the integrity checker when the system is otherwise not busy.

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.17 -> v1.0.18](https://github.com/openziti/agent/compare/v1.0.17...v1.0.18)
* github.com/openziti/channel/v3: [v2.0.143 -> v3.0.3](https://github.com/openziti/channel/compare/v2.0.143...v3.0.3)
    * [Issue #138](https://github.com/openziti/channel/issues/138) - Allow custom message serialization. Add support for a 'raw' message type.
    * [Issue #82](https://github.com/openziti/channel/issues/82) - Remove transport.Configuration from UnderlayFactory.Create

* github.com/openziti/edge-api: [v0.26.29 -> v0.26.30](https://github.com/openziti/edge-api/compare/v0.26.29...v0.26.30)
* github.com/openziti/foundation/v2: [v2.0.48 -> v2.0.49](https://github.com/openziti/foundation/compare/v2.0.48...v2.0.49)
* github.com/openziti/identity: [v1.0.84 -> v1.0.85](https://github.com/openziti/identity/compare/v1.0.84...v1.0.85)
* github.com/openziti/jwks: [v1.0.4 -> v1.0.5](https://github.com/openziti/jwks/compare/v1.0.4...v1.0.5)
    * [Issue #9](https://github.com/openziti/jwks/issues/9) - Using NewKey w/ RSA key results in nil pointer exception

* github.com/openziti/metrics: [v1.2.57 -> v1.2.58](https://github.com/openziti/metrics/compare/v1.2.57...v1.2.58)
* github.com/openziti/runzmd: [v1.0.50 -> v1.0.51](https://github.com/openziti/runzmd/compare/v1.0.50...v1.0.51)
* github.com/openziti/sdk-golang: [v0.23.40 -> v0.23.42](https://github.com/openziti/sdk-golang/compare/v0.23.40...v0.23.42)
    * [Issue #625](https://github.com/openziti/sdk-golang/issues/625) - traffic optimization: implement support for receiving multi-part edge payloads

* github.com/openziti/secretstream: [v0.1.21 -> v0.1.24](https://github.com/openziti/secretstream/compare/v0.1.21...v0.1.24)
* github.com/openziti/storage: [v0.3.0 -> v0.3.2](https://github.com/openziti/storage/compare/v0.3.0...v0.3.2)
* github.com/openziti/transport/v2: [v2.0.143 -> v2.0.146](https://github.com/openziti/transport/compare/v2.0.143...v2.0.146)
    * [Issue #92](https://github.com/openziti/transport/issues/92) - Implement simple traffic shaper

* github.com/openziti/xweb/v2: [v2.1.1 -> v2.1.2](https://github.com/openziti/xweb/compare/v2.1.1...v2.1.2)
* github.com/openziti-incubator/cf: v0.0.3 (new)
* github.com/openziti/dilithium: [v0.3.3 -> v0.3.5](https://github.com/openziti/dilithium/compare/v0.3.3...v0.3.5)
* github.com/openziti/ziti: [v1.1.11 -> v1.1.12](https://github.com/openziti/ziti/compare/v1.1.11...v1.1.12)
    * [Issue #2413](https://github.com/openziti/ziti/issues/2413) - Add db anonymization utility
    * [Issue #2415](https://github.com/openziti/ziti/issues/2415) - Fix policy denormalization when service policy type is changed
    * [Issue #2406](https://github.com/openziti/ziti/issues/2406) - ziti agent controller snapshot-db exit code is always successful
    * [Issue #2405](https://github.com/openziti/ziti/issues/2405) - Investigate Older SDKs Not Enrolling Not Connecting in HA
    * [Issue #2403](https://github.com/openziti/ziti/issues/2403) - Fix terminator costing concurrency issue
    * [Issue #2397](https://github.com/openziti/ziti/issues/2397) - JWKS endpoints w/ new keys do not get refreshed
    * [Issue #2390](https://github.com/openziti/ziti/issues/2390) - Update to github.com/openziti/channel/v3
    * [Issue #2388](https://github.com/openziti/ziti/issues/2388) - Remove use of ziti fabric add-identity commands in 004-controller-pki.md

# Release 1.1.11

# What's New

* This release updates to Go v1.23
* Updates to the latest version of golangci-lint, to allow it to work with the new version of Go
* Linter fixes to address issues caught by updated linter

# Release 1.1.10

## What's New

* Bug fixes, enhancements and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/cobra-to-md: v1.0.1 (new)
* github.com/openziti/edge-api: [v0.26.25 -> v0.26.29](https://github.com/openziti/edge-api/compare/v0.26.25...v0.26.29)
* github.com/openziti/jwks: [v1.0.3 -> v1.0.4](https://github.com/openziti/jwks/compare/v1.0.3...v1.0.4)
* github.com/openziti/ziti: [v1.1.9 -> v1.1.10](https://github.com/openziti/ziti/compare/v1.1.9...v1.1.10)
    * [Issue #2374](https://github.com/openziti/ziti/issues/2374) - Edge Routers Do Not Accept JWTs for `openziti`/`native` audiences
    * [Issue #2353](https://github.com/openziti/ziti/issues/2353) - Update go-jose to avoid CVEs
    * [Issue #2333](https://github.com/openziti/ziti/issues/2333) - Give zit agent controller snapshot-db same capabilities as ziti fabric db snapshot
    * [Issue #2343](https://github.com/openziti/ziti/issues/2343) - Transferring leadership to another controller fails sometimes

# Release 1.1.9

## What's New

* Bug fixes, enhancements and continuing progress on controller HA
* Includes a performance update ([Issue #2340](https://github.com/openziti/ziti/issues/2340)) which should improve
  connection ramp times. Previously circuits would start with a relatively low window size and ramp slowly. Circuits
  will now start with a large initial window size and scale back if they can't keep up
* Added `ziti ops verify-network`. A command to aid when configuring the overlay network, this command will check config
  files for obvious mistakes
* Added `ziti ops verify-traffic`. Another command to aid when configuring the overlay network, this command will ensure
  the overlay network is able to pass traffic

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.16 -> v1.0.17](https://github.com/openziti/agent/compare/v1.0.16...v1.0.17)
* github.com/openziti/channel/v2: [v2.0.136 -> v2.0.143](https://github.com/openziti/channel/compare/v2.0.136...v2.0.143)
    * [Issue #136](https://github.com/openziti/channel/issues/136) - Fix timeout on classic dialer 
    * [Issue #134](https://github.com/openziti/channel/issues/134) - Support the dtls transport

* github.com/openziti/edge-api: [v0.26.23 -> v0.26.25](https://github.com/openziti/edge-api/compare/v0.26.23...v0.26.25)
* github.com/openziti/foundation/v2: [v2.0.47 -> v2.0.48](https://github.com/openziti/foundation/compare/v2.0.47...v2.0.48)
* github.com/openziti/identity: [v1.0.81 -> v1.0.84](https://github.com/openziti/identity/compare/v1.0.81...v1.0.84)
* github.com/openziti/metrics: [v1.2.56 -> v1.2.57](https://github.com/openziti/metrics/compare/v1.2.56...v1.2.57)
* github.com/openziti/runzmd: [v1.0.49 -> v1.0.50](https://github.com/openziti/runzmd/compare/v1.0.49...v1.0.50)
* github.com/openziti/sdk-golang: [v0.23.39 -> v0.23.40](https://github.com/openziti/sdk-golang/compare/v0.23.39...v0.23.40)
    * [Issue #601](https://github.com/openziti/sdk-golang/issues/601) - Only send config types on service list if controller version supports it

* github.com/openziti/transport/v2: [v2.0.138 -> v2.0.143](https://github.com/openziti/transport/compare/v2.0.138...v2.0.143)
    * [Issue #85](https://github.com/openziti/transport/issues/85) - Update to latest dtls library

* github.com/openziti/ziti: [v1.1.8 -> v1.1.9](https://github.com/openziti/ziti/compare/v1.1.8...v1.1.9)
    * [Issue #2343](https://github.com/openziti/ziti/issues/2343) - Transferring leadership to another controller fails sometimes
    * [Issue #2340](https://github.com/openziti/ziti/issues/2340) - Update xgress defaults
    * [Issue #2336](https://github.com/openziti/ziti/issues/2336) - Re-enable optional xgress payload mtu, with message framing
    * [Issue #2091](https://github.com/openziti/ziti/issues/2091) - Add `scope` and `cliend_id` configuration to ext jwt signers
    * [Issue #2318](https://github.com/openziti/ziti/issues/2318) - Unable to update appData on existing edge routers using PATCH
    * [Issue #2281](https://github.com/openziti/ziti/issues/2281) - Session Certificates Should Return a Chain
    * [Issue #2285](https://github.com/openziti/ziti/issues/2285) - routers sometimes report link metrics for closed links 
    * [Issue #2282](https://github.com/openziti/ziti/issues/2282) - Investigate OIDC TOTP Redirect w/ application/json
    * [Issue #2279](https://github.com/openziti/ziti/issues/2279) - Ensure xweb initialized before RAFT
    * [Issue #2277](https://github.com/openziti/ziti/issues/2277) - docker controller and router deployments - generate a config by default
    * [Issue #2154](https://github.com/openziti/ziti/issues/2154) - HA MFA Enrollment returns 500
    * [Issue #2159](https://github.com/openziti/ziti/issues/2159) - API Session in HA return 400

# Release 1.1.8

## What's New

* Bug fixes, enhancements and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.20 -> v0.26.23](https://github.com/openziti/edge-api/compare/v0.26.20...v0.26.23)
    * [Issue #120](https://github.com/openziti/edge-api/issues/120) - Add API for retrieving services referencing a config
    * [Issue #121](https://github.com/openziti/edge-api/issues/121) - Add API for retrieving the set of attribute roles used by posture checks

* github.com/openziti/sdk-golang: [v0.23.38 -> v0.23.39](https://github.com/openziti/sdk-golang/compare/v0.23.38...v0.23.39)
    * [Issue #596](https://github.com/openziti/sdk-golang/issues/596) - SDK should submit selected config types to auth and service list APIs
    * [Issue #593](https://github.com/openziti/sdk-golang/issues/593) - SDK Golang OIDC Get API Session Returns Wrong Value

* github.com/openziti/storage: [v0.2.47 -> v0.3.0](https://github.com/openziti/storage/compare/v0.2.47...v0.3.0)
    * [Issue #80](https://github.com/openziti/storage/issues/80) - Set indexes aren't cleaned up when referenced entities are deleted, only when they change
    * [Issue #78](https://github.com/openziti/storage/issues/78) - Allow searching for things without case sensitivity

* github.com/openziti/ziti: [v1.1.7 -> v1.1.8](https://github.com/openziti/ziti/compare/v1.1.7...v1.1.8)
    * [Issue #2121](https://github.com/openziti/ziti/issues/2121) - Use router data model for edge router tunnel
    * [Issue #2245](https://github.com/openziti/ziti/issues/2245) - Add ability to retrieve a list of services that reference a config
    * [Issue #2089](https://github.com/openziti/ziti/issues/2089) - Enhance Management API to list Posture Check Roles
    * [Issue #2209](https://github.com/openziti/ziti/issues/2209) - `/edge/v1/external-jwt-signers` needs to be open
    * [Issue #2010](https://github.com/openziti/ziti/issues/2010) - Add config information to router data model
    * [Issue #1990](https://github.com/openziti/ziti/issues/1990) - Implement subscriber model for identity/service events in router
    * [Issue #2240](https://github.com/openziti/ziti/issues/2240) - Secondary ext-jwt Auth Policy check incorrectly requires primary ext-jwt auth to be enabled


# Release 1.1.7

## What's New

* Release actions fixes
* Fix for a flaky acceptance test

# Release 1.1.6

## What's New

* Trust Domain Configuration
* Controller HA Beta 2

## Trust Domain Configuration

OpenZiti controllers from this release forward will now require a `trust domain` to be configured. 
High Availability (HA) controllers already have this requirement. HA Controllers configure their trust domain via SPIFFE 
ids that are embedded in x509 certificates.

For feature parity, non-HA controllers will now have this same requirement. However, as re-issuing certificates is not
always easily done. To help with the transition, non-HA controllers will have the ability to have their trust domain
sourced from the controller configuration file through the root configuration value `trustDomain`. The configuration
field which takes a string that must be URI hostname compatible (see: https://github.com/spiffe/spiffe/blob/main/standards/SPIFFE-ID.md).
If this value is not defined, a trust domain will be generated from the root CA certificate of the controller. 

For networks that will be deployed after this change, it is highly suggested that a SPIFFE id is added to certificates.
The `ziti pki create ...` tooling supports the `--spiffe-id` option to help handle this scenario.

### Generated Trust Domain Log Messages

The following log messages are examples of warnings produced when a controller is using a generated trust domain:

```
WARNING this environment is using a default generated trust domain [spiffe://d561decf63d229d66b07de627dbbde9e93228925], 
  it is recommended that a trust domain is specified in configuration via URI SANs or the 'trustDomain' field

WARNING this environment is using a default generated trust domain [spiffe://d561decf63d229d66b07de627dbbde9e93228925], 
  it is recommended that if network components have enrolled that the generated trust domain be added to the 
  configuration field 'additionalTrustDomains'
```

### Trust domain resolution:

- Non-HA controllers
  - Prefers SPIFFE ids in x509 certificate URI SANs, looking at the leaf up the signing chain
  - Regresses to `trustDomain` in the controller configuration file if not found
  - Regress to generating a trust domain from the server certificates root CA, if the above do not resolve

- HA Controllers
  - Requires x509 SPIFFE ids in x509 certificate URI SANs

### Additional Trust Domains

When moving between trust domains (i.e. from the default generated to a new named one), the controller supports having
other trust domains. The trust domains do not replace certificate chain validation, which is still checked and enforced.

Additional trust domains are configured in the controller configuration file under the root field 
`additionalTrustDomains`. This field is an array of hostname safe strings.

The most common use case for this is field is if a network has issued certificates using the generated trust domain and 
now wants to transition to a explicitly defined one.

## Controller HA Beta 2

This release can be run in HA mode. The code is still beta, as we're still finding and fixing bugs. Several bugs
have been fixed since Beta 1 and c-based SDKs and tunnelers now work in HA mode. The smoketest can now be run
with HA controllers and clients.

* Latest ZET release supporting HA control: https://github.com/openziti/ziti-tunnel-sdk-c/releases/tag/v2.0.0-alpha9
* Windows, Mac and Mobile clients are in the process of being updated

For more information:

* HA overview/getting started/migration: [HA Documentation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
* Open Issues: [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1)

## Component Updates and Bug Fixes

* github.com/openziti/storage: [v0.2.45 -> v0.2.46](https://github.com/openziti/storage/compare/v0.2.45...v0.2.46)
    * [Issue #76](https://github.com/openziti/storage/issues/76) - Add support for non-boltz symbols to the the boltz stores

* github.com/openziti/ziti: [v1.1.5 -> v1.1.6](https://github.com/openziti/ziti/compare/v1.1.5...v1.1.6)
    * [Issue #2171](https://github.com/openziti/ziti/issues/2171) - Routers should consider control channels unresponsive if they are not connected
    * [Issue #2219](https://github.com/openziti/ziti/issues/2219) - Add inspection for router connections
    * [Issue #2195](https://github.com/openziti/ziti/issues/2195) - cached data model file set to
    * [Issue #2222](https://github.com/openziti/ziti/issues/2222) - Add way to get read-only status from cluster nodes
    * [Issue #2191](https://github.com/openziti/ziti/issues/2191) - Change raft list cluster members element name from values to data to match rest of REST api
    * [Issue #785](https://github.com/openziti/ziti/issues/785) - ziti edge update service-policy to empty/no posture checks fails
    * [Issue #2205](https://github.com/openziti/ziti/issues/2205) - Merge fabric and edge model code
    * [Issue #2165](https://github.com/openziti/ziti/issues/2165) - Add network id

# Release 1.1.5

## What's New

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.133 -> v2.0.136](https://github.com/openziti/channel/compare/v2.0.133...v2.0.136)
    * [Issue #132](https://github.com/openziti/channel/issues/132) - reconnecting dialer doesn't take local binding into account when reconnecting

* github.com/openziti/identity: [v1.0.80 -> v1.0.81](https://github.com/openziti/identity/compare/v1.0.80...v1.0.81)
* github.com/openziti/transport/v2: [v2.0.136 -> v2.0.138](https://github.com/openziti/transport/compare/v2.0.136...v2.0.138)
    * [Issue #83](https://github.com/openziti/transport/issues/83) - tls.Dial should use proxy configuration if provided

* github.com/openziti/xweb/v2: [v2.1.0 -> v2.1.1](https://github.com/openziti/xweb/compare/v2.1.0...v2.1.1)
* github.com/openziti/ziti: [v1.1.4 -> v1.1.5](https://github.com/openziti/ziti/compare/v1.1.4...v1.1.5)
    * [Issue #2173](https://github.com/openziti/ziti/issues/2173) - panic on HA peer connect
    * [Issue #2171](https://github.com/openziti/ziti/issues/2171) - Routers should consider control channels unresponsive if they are not connected
    * [Issue #2086](https://github.com/openziti/ziti/issues/2086) - Enable File Watching for Router/Controller Identities
    * [Issue #2087](https://github.com/openziti/ziti/issues/2087) - Ext JWT not setting provider value in auth query

# Release 1.1.4

## What's New

* Controller HA Beta 1
* Bug fixes

## Controller HA Beta 1

This release can be run in HA mode. The code is still beta, as we're still finding and fixing bugs. Several bugs 
have been fixed since Alpha 3 and c-based SDKs and tunnelers now work in HA mode. The smoketest can now be run
with HA controllers and clients.

* Initial ZET release support HA control: https://github.com/openziti/ziti-tunnel-sdk-c/releases/tag/v2.0.0-alpha1
* Windows, Mac and Mobile clients are in the process of being updated

For more information:

* HA overview/getting started/migration: [HA Documentation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
* Open Issues: [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1)

## Component Updates and Bug Fixes 

* github.com/openziti/channel/v2: [v2.0.130 -> v2.0.133](https://github.com/openziti/channel/compare/v2.0.130...v2.0.133)
* github.com/openziti/edge-api: [v0.26.19 -> v0.26.20](https://github.com/openziti/edge-api/compare/v0.26.19...v0.26.20)
    * [Issue #113](https://github.com/openziti/edge-api/issues/113) - RecoveryCodesEnvelope is wrong

* github.com/openziti/foundation/v2: [v2.0.45 -> v2.0.47](https://github.com/openziti/foundation/compare/v2.0.45...v2.0.47)
    * [Issue #407](https://github.com/openziti/foundation/issues/407) - Remove Branch from build info

* github.com/openziti/identity: [v1.0.77 -> v1.0.80](https://github.com/openziti/identity/compare/v1.0.77...v1.0.80)
* github.com/openziti/metrics: [v1.2.54 -> v1.2.56](https://github.com/openziti/metrics/compare/v1.2.54...v1.2.56)
* github.com/openziti/runzmd: [v1.0.47 -> v1.0.49](https://github.com/openziti/runzmd/compare/v1.0.47...v1.0.49)
* github.com/openziti/sdk-golang: [v0.23.37 -> v0.23.38](https://github.com/openziti/sdk-golang/compare/v0.23.37...v0.23.38)
    * [Issue #573](https://github.com/openziti/sdk-golang/issues/573) - api session refresh spins in a tight loop if there is no current api session
    * [Issue #562](https://github.com/openziti/sdk-golang/issues/562) - Support sticky dials

* github.com/openziti/secretstream: [v0.1.20 -> v0.1.21](https://github.com/openziti/secretstream/compare/v0.1.20...v0.1.21)
* github.com/openziti/storage: [v0.2.41 -> v0.2.45](https://github.com/openziti/storage/compare/v0.2.41...v0.2.45)
    * [Issue #73](https://github.com/openziti/storage/issues/73) - db integrity checker doesn't take nullable flag into account when checking unique indices
    * [Issue #71](https://github.com/openziti/storage/issues/71) - Add AddFkIndexCascadeDelete

* github.com/openziti/transport/v2: [v2.0.133 -> v2.0.136](https://github.com/openziti/transport/compare/v2.0.133...v2.0.136)
* github.com/openziti/ziti: [v1.1.3 -> v1.1.4](https://github.com/openziti/ziti/compare/v1.1.3...v1.1.4)
    * [Issue #2084](https://github.com/openziti/ziti/issues/2084) - Bug: Router enrollment is missing its server chain
    * [Issue #2124](https://github.com/openziti/ziti/issues/2124) - api session certs should be deleted when related api sessions are deleted

# Release 1.1.3

## What's New

* Sticky Terminator Selection
* Linux and Docker deployments log formats no longer default to the simplified format option and now use logging library
  defaults: `json` for non-interactive, `text` for interactive.

NOTE: This release is the first since 1.0.0 to be marked promoted from pre-release. Be sure to check the release notes
      for the rest of the post-1.0.0 releases to get the full set of changes.

## Stick Terminator Strategy

This release introduces a new terminator selection strategy `sticky`. On every dial it will return a token to the 
dialer, which represents the terminator used in the dial. This token maybe passed in on subsequent dials. If no token
is passed in, the strategy will work the same as the `smartrouting` strategy. If a token is passed in, and the 
terminator is still valid, the same terminator will be used for the dial. A terminator will be consideder valid if
it still exists and there are no terminators with a higher precedence. 

This is currently only supported in the Go SDK.

### Go SDK Example

```
ziti edge create service test --terminator-strategy sticky
```

```
	conn := clientContext.Dial("test")
	token := conn.Conn.GetStickinessToken()
	_ = conn.Close()

	dialOptions := &ziti.DialOptions{
		ConnectTimeout:  time.Second,
		StickinessToken: token,
	}
	conn = clientContext.DialWithOptions("test", dialOptions))
	nextToken := conn.Conn.GetStickinessToken()
	_ = conn.Close()
```

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.128 -> v2.0.130](https://github.com/openziti/channel/compare/v2.0.128...v2.0.130)
* github.com/openziti/edge-api: [v0.26.18 -> v0.26.19](https://github.com/openziti/edge-api/compare/v0.26.18...v0.26.19)
* github.com/openziti/foundation/v2: [v2.0.42 -> v2.0.45](https://github.com/openziti/foundation/compare/v2.0.42...v2.0.45)
* github.com/openziti/identity: [v1.0.75 -> v1.0.77](https://github.com/openziti/identity/compare/v1.0.75...v1.0.77)
* github.com/openziti/metrics: [v1.2.51 -> v1.2.54](https://github.com/openziti/metrics/compare/v1.2.51...v1.2.54)
* github.com/openziti/runzmd: [v1.0.43 -> v1.0.47](https://github.com/openziti/runzmd/compare/v1.0.43...v1.0.47)
* github.com/openziti/sdk-golang: [v0.23.35 -> v0.23.37](https://github.com/openziti/sdk-golang/compare/v0.23.35...v0.23.37)
    * [Issue #562](https://github.com/openziti/sdk-golang/issues/562) - Support sticky dials

* github.com/openziti/secretstream: [v0.1.19 -> v0.1.20](https://github.com/openziti/secretstream/compare/v0.1.19...v0.1.20)
* github.com/openziti/storage: [v0.2.37 -> v0.2.41](https://github.com/openziti/storage/compare/v0.2.37...v0.2.41)
* github.com/openziti/transport/v2: [v2.0.131 -> v2.0.133](https://github.com/openziti/transport/compare/v2.0.131...v2.0.133)
* github.com/openziti/ziti: [v1.1.2 -> v1.1.3](https://github.com/openziti/ziti/compare/v1.1.2...v1.1.3)
    * [Issue #2064](https://github.com/openziti/ziti/issues/2064) - Fix panic on link close
    * [Issue #2062](https://github.com/openziti/ziti/issues/2062) - Link connection retry delays should contain some randomization 
    * [Issue #2055](https://github.com/openziti/ziti/issues/2055) - Controller panics on 'ziti agent cluster list'
    * [Issue #2019](https://github.com/openziti/ziti/issues/2019) - Support mechanism for sticky dials

# Release 1.1.2

## What's New

* Bug fixes and minor enhancements

## Component Updates and Bug Fixes
* github.com/openziti/sdk-golang: [v0.23.32 -> v0.23.35](https://github.com/openziti/sdk-golang/compare/v0.23.32...v0.23.35)
* github.com/openziti/ziti: [v1.1.1 -> v1.1.2](https://github.com/openziti/ziti/compare/v1.1.1...v1.1.2)
  * [Issue #2032](https://github.com/openziti/ziti/issues/2032) - Auto CA Enrollment Fails w/ 400 Bad Request
  * [Issue #2026](https://github.com/openziti/ziti/issues/2026) - Root Version Endpoint Handling 404s
  * [Issue #2002](https://github.com/openziti/ziti/issues/2002) - JWKS endpoints may not refresh on new KID
  * [Issue #2007](https://github.com/openziti/ziti/issues/2007) - Identities for edge routers with tunneling enabled sometimes show hasEdgeRouterConnection=false even though everything is OK
  * [Issue #1983](https://github.com/openziti/ziti/issues/1983) - delete of non-existent entity causes panic when run on follower controller


# Release 1.1.1

## What's New

* HA Alpha-3
* Bug fixes and minor enhancements
* [The all-in-one quickstart compose project](./quickstart/docker/all-in-one/README.md) now uses the same environment variable to configure the controller's address as the ziti command line tool

## HA Alpha 3

This release can be run in HA mode. The code is still alpha, as we're still finding and fixing bugs. 

For more information:

* HA overview/getting started/migration: [HA Documementation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
* Open Issues: [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1) 

## New Contributors

Thanks to new contributors

* @Vrashabh-Sontakke

## Component Updates and Bug Fixes
* github.com/openziti/edge-api: [v0.26.17 -> v0.26.18](https://github.com/openziti/edge-api/compare/v0.26.17...v0.26.18)
* github.com/openziti/sdk-golang: [v0.23.27 -> v0.23.32](https://github.com/openziti/sdk-golang/compare/v0.23.27...v0.23.32)
    * [Issue #554](https://github.com/openziti/sdk-golang/issues/554) - Passing in config types on service list breaks on older controller

* github.com/openziti/storage: [v0.2.36 -> v0.2.37](https://github.com/openziti/storage/compare/v0.2.36...v0.2.37)
    * [Issue #64](https://github.com/openziti/storage/issues/64) - Add support for transaction complete listeners

* github.com/openziti/ziti: [v1.1.0 -> v1.1.1](https://github.com/openziti/ziti/compare/v1.1.0...v1.1.1)
    * [Issue #1973](https://github.com/openziti/ziti/issues/1973) - Raft should not initialize if db is misconfigured
    * [Issue #1971](https://github.com/openziti/ziti/issues/1971) - BUG: OIDC authentication does not convert config type names to ids
    * [Issue #1966](https://github.com/openziti/ziti/issues/1966) - Handle multi-entity updates in router data model
    * [Issue #1772](https://github.com/openziti/ziti/issues/1772) - provide a better error when the user is not logged in
    * [Issue #1964](https://github.com/openziti/ziti/issues/1964) - Add API Session Token Update Messaging
    * [Issue #1960](https://github.com/openziti/ziti/issues/1960) - JWT Session exchange isn't working
    * [Issue #1962](https://github.com/openziti/ziti/issues/1962) - permissions enum doesn't contain "Invalid"

# Release 1.1.0

## What's New

* HA Alpha2
* Deployments Alpha
    * Linux packages provide systemd services for controller and router. Both depend on existing package `openziti` which provides the `ziti` command line tool.
        * `openziti-controller` provides `ziti-controller.service`
        * `openziti-router` provides `ziti-router.service`
    * Container images for controller and router now share the bootstrapping logic with the packages, so they
      support the same configuration options.

## HA Alpha2

This release can be run in HA mode. The code is still alpha, so there are still some bugs and missing features,
however basic functionality work with the exceptions noted. See the [HA Documementation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
for instructions on setting up an HA cluster.

### Known Issues

* JWT Session exchange isn't working with Go SDK clients
    * This means Go clients will need to be restarted once their sessions expire
* Service/service policy changes might not be reflected in routers
    * Changes to policy may not yet properly sync to the routers, causing unexpected behavior with ER/Ts running in HA mode

More information can be found on the [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1)

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.16 -> v0.26.17](https://github.com/openziti/edge-api/compare/v0.26.16...v0.26.17)
    * [Issue #107](https://github.com/openziti/edge-api/issues/107) - Add configTypes param to service list

* github.com/openziti/sdk-golang: [v0.23.19 -> v0.23.27](https://github.com/openziti/sdk-golang/compare/v0.23.19...v0.23.27)
    * [Issue #545](https://github.com/openziti/sdk-golang/issues/545) - Set config types on query when listing services
    * [Issue #541](https://github.com/openziti/sdk-golang/issues/541) - Token exchange in Go SDK not working
    * [Issue #540](https://github.com/openziti/sdk-golang/issues/540) - Switch to EdgeRouter.SupportedProtocols from deprecated URLs map

* github.com/openziti/ziti: [v1.0.0 -> v1.1.0](https://github.com/openziti/ziti/compare/v1.0.0...v1.1.0)
    * [Issue #1952](https://github.com/openziti/ziti/issues/1952) - Remove support for fabric only identities in CLI
    * [Issue #1950](https://github.com/openziti/ziti/issues/1950) - Add policy type to service policy router events
    * [Issue #1951](https://github.com/openziti/ziti/issues/1951) - Add more attributes to route data model Identity
    * [Issue #1942](https://github.com/openziti/ziti/issues/1942) - Rework ER/T intercept code to be sessionless or use JWT sessions
    * [Issue #1936](https://github.com/openziti/ziti/issues/1936) - SDK Hosted HA sessions are getting removed when they shouldn't be
    * [Issue #1934](https://github.com/openziti/ziti/issues/1934) - Don't publish binary builds to artifactory
    * [Issue #1931](https://github.com/openziti/ziti/issues/1931) - "invalid kid: <kid>" randomly occurs in HA mode

# Release 1.0.0

## About 1.0

What does marking OpenZiti as 1.0 mean?

### Backwards Compatibility
We've guaranteed API stability for SDK clients for years and worked hard to ensure that routers 
and controllers would be backwards and forward compatible. However, we have had a variety of 
management API changes and CLI changes. For post 1.0 releases we expect to make additions to the 
APIs and CLI, but won't remove anything until it's been first marked as deprecated and then only
with a major version bump. 

### Stability and Scale
Recent releases have seen additional testing using chaos testing techniques. These tests involve
setting up relatively large scale environments, knocking out various components and then verifying
that the network is able to return to a stable state. These test are run for hours to try and 
eliminate race conditions and distributed state machine problems. 

OpenZiti is also being used as underlying infrastrcture for the zrok public service. Use of this 
network has grown quickly and proven that it's possible to build ziti native apps that can scale
up.

## Backward Incompatible Changes to pre-1.0 releases

Administrators no longer have access to dial/bind all services by default. See below for details.

## What's New

* Administrators no longer have access to dial/bind all services by default.
* TLS Handshakes can now be rate limited in the controller
* TLS Handshake timeouts can now be set on the controller when using ALPN
* Bugfixes

## DEFAULT Bind/Dial SERVICE PERMISSIONS FOR Admin IDENTITIES HAVE CHANGED

Admin identities were able to Dial and Bind all services regardless of the effective service policies
prior to this release. This could lead to a confusing situation where a tunneler that was assuming an Admin
identity would put itself into an infinite connect-loop when a service's host.v1 address overlapped with
any addresses in its intercept configuration.

Please create service policies to grant Bind or Dial permissions to Admin identities as needed.

## TLS Handshake

A TLS handhshake rate limiter can be enabled. This is useful in cases where there's a flood of TLS requests and the
controller can't handle them all. It can get into a state where it can't respond to TLS handshakes quickly enough,
so the clients time out. They then retry, adding to the the load. The controller ends up wasting time doing work 
that isn't use. 

This uses the same rate limiting as the auth rate limiter. 

Additionally the server side handshake timeout can now be configured.

Configuration:

```
tls: 
  handshakeTimeout: 15s

  rateLimiter:
    # if disabled, no tls handshake rate limiting with be enforced
    enabled: true
    # the smallest window size for tls handshakes
    minSize: 5
    # the largest allowed window size for tls handshakes
    maxSize: 5000
    # after how long to consider a handshake abandoned if neither success nor failure was reported
    timeout: 30s
```

New metrics:

* `tls_handshake_limiter.in_process` - number of TLS handshakes in progress
* `tls_handshake_limiter.window_size` - number of TLS handhshakes allowed concurrently
* `tls_handshake_limiter.work_timer` - timer tracking how long TLS handshakes are taking


## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.122 -> v2.0.128](https://github.com/openziti/channel/compare/v2.0.122...v2.0.128)
* github.com/openziti/edge-api: [v0.26.14 -> v0.26.16](https://github.com/openziti/edge-api/compare/v0.26.14...v0.26.16)
* github.com/openziti/foundation/v2: [v2.0.40 -> v2.0.42](https://github.com/openziti/foundation/compare/v2.0.40...v2.0.42)
* github.com/openziti/identity: [v1.0.73 -> v1.0.75](https://github.com/openziti/identity/compare/v1.0.73...v1.0.75)
* github.com/openziti/metrics: [v1.2.48 -> v1.2.51](https://github.com/openziti/metrics/compare/v1.2.48...v1.2.51)
* github.com/openziti/runzmd: [v1.0.41 -> v1.0.43](https://github.com/openziti/runzmd/compare/v1.0.41...v1.0.43)
* github.com/openziti/sdk-golang: [v0.23.15 -> v0.23.19](https://github.com/openziti/sdk-golang/compare/v0.23.15...v0.23.19)
* github.com/openziti/secretstream: [v0.1.18 -> v0.1.19](https://github.com/openziti/secretstream/compare/v0.1.18...v0.1.19)
* github.com/openziti/storage: [v0.2.33 -> v0.2.36](https://github.com/openziti/storage/compare/v0.2.33...v0.2.36)
* github.com/openziti/transport/v2: [v2.0.125 -> v2.0.131](https://github.com/openziti/transport/compare/v2.0.125...v2.0.131)
    * [Issue #79](https://github.com/openziti/transport/issues/79) - Add adaptive rate limiting to shared tls listener

* github.com/openziti/ziti: [v0.34.2 -> v1.0.0](https://github.com/openziti/ziti/compare/v0.34.2...v1.0.0)
    * [Issue #1923](https://github.com/openziti/ziti/issues/1923) - Add release validation test suite
    * [Issue #1904](https://github.com/openziti/ziti/issues/1904) - Add TLS handshake rate limiter
    * [Issue #1921](https://github.com/openziti/ziti/issues/1921) - Tidy CLI
    * [Issue #1916](https://github.com/openziti/ziti/issues/1916) - SDK dials fails with 'token is malformed' error
    * [Issue #1911](https://github.com/openziti/ziti/issues/1911) - Fix panic on first HA controller startup
    * [Issue #1914](https://github.com/openziti/ziti/issues/1914) - Fix panic in PeerConnected
    * [Issue #1781](https://github.com/openziti/ziti/issues/1781) - Admin identities have bind and dial permissions to services
