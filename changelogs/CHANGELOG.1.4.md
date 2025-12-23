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
