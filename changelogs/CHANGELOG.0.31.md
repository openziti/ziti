# Release 0.31.4

## What's New

* Bug fix for a data flow stall which is especially likely to happen on circuits with single router paths

## Thanks

* @marvkis - for providing high quality debug data which made tracking down a couple of flow control stall issues much easier

## Component Updates and Bug Fixes

* github.com/openziti/metrics: [v1.2.40 -> v1.2.41](https://github.com/openziti/metrics/compare/v1.2.40...v1.2.41)
* github.com/openziti/sdk-golang: [v0.21.2 -> v0.22.0](https://github.com/openziti/sdk-golang/compare/v0.21.2...v0.22.0)
    * [Issue #468](https://github.com/openziti/sdk-golang/issues/468) - SDK does an unnecessary number of session refreshes

* github.com/openziti/storage: [v0.2.26 -> v0.2.27](https://github.com/openziti/storage/compare/v0.2.26...v0.2.27)
* github.com/openziti/ziti: [v0.31.3 -> v0.31.4](https://github.com/openziti/ziti/compare/v0.31.3...v0.31.4)
    * [Issue #1645](https://github.com/openziti/ziti/issues/1645) - Once routers share a link id, we can't use the link id to decide which duplicate link to discard
    * [Issue #1642](https://github.com/openziti/ziti/issues/1642) - Revert posture check optimization
    * [Issue #1586](https://github.com/openziti/ziti/issues/1586) - If ack is received before payload is processed by link send buffer, a stall can result


# Release 0.31.3

## What's New

* Services Max Idle Time
* Add/Remove Peer and Transfer Leadership via REST

## Service Max Idle Time

A max idle time can now be configured on services. The default value of 0 indicates that no maximum will 
be enforced. A circuit is considered idle when no traffic is flowing across through the initiating or
terminating router. 

```
ziti edge create service test-service --max-idle-time 5m
```

Note that the idle time calculation is done on the router, so if max idle time on a service is less 
than the configured scan interval on the router, it make take longer than expected for idle circuits
to be removed.

## Raft Cluster Management via REST

The controller now allows some Raft cluster management operations to be performed via REST. 

NOTE: If your cluster is not bootstrapped yet, the REST API won't be available. These will only work on a bootstrapped cluster! 

The following operations are now supported:

* Add member
* Remove member
* Transfer leadership

```
ziti fabric raft add-member tls:localhost:6363
ziti fabric raft add-member tls:localhost:6464
ziti fabric raft transfer-leadership 
ziti fabric raft transfer-leadership ctrl3
ziti fabric raft remove-member ctrl2
ziti fabric raft remove-member ctrl3
```

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.1 -> v0.26.6](https://github.com/openziti/edge-api/compare/v0.26.1...v0.26.6)
* github.com/openziti/sdk-golang: [v0.20.139 -> v0.21.2](https://github.com/openziti/sdk-golang/compare/v0.20.139...v0.21.2)
    * [Issue #465](https://github.com/openziti/sdk-golang/issues/465) - Allow listen options to specify how many listeners need to be established before returning
    * [Issue #462](https://github.com/openziti/sdk-golang/issues/462) - Allow refreshing a single service

* github.com/openziti/ziti: [v0.31.2 -> v0.31.3](https://github.com/openziti/ziti/compare/v0.31.2...v0.31.3)
    * [Issue #1583](https://github.com/openziti/ziti/issues/1583) - xgress: Potential data stall due when processing acks after checking window size 
    * [Issue #1578](https://github.com/openziti/ziti/issues/1578) - Send BindSuccess notifications to SDK if supported
    * [Issue #1544](https://github.com/openziti/ziti/issues/1544) - Support transfer raft leadership via REST
    * [Issue #1543](https://github.com/openziti/ziti/issues/1543) - Support add/remove raft peer via REST
    * [Issue #1496](https://github.com/openziti/ziti/issues/1496) - Configurable Timer needed to close idle circuits
    * [Issue #1402](https://github.com/openziti/ziti/issues/1402) - Allow router to decomission itself

# Release 0.31.2

## What's New

* Go version updated from 1.20 to 1.21

# Release 0.31.1

## What's New

* SDK Hosting Improvements
* Terminator validation utility
* Circuit/Link query support

## SDK Hosting Improvements

In previous versions of OpenZiti, if many SDK clients were attempting to establish hosting, the controller could get overwhelmed. 
In this release, routers will use the rate limiter pool introduced in 0.27.6 when creating terminators on behalf of sdk clients
hosting applications. Additionally, routers now have the ability to verify terminator state with the sdk, if the sdk supports it.
In general, hosting large numbers of services using the sdk should now be less suceptible to thundering herd issues.

## Manual Terminator Validation

There is a new CLI command available to validate terminator state. This is primarily a developer tool to validate that terminator 
setup logic is correct. However it may also be used to diagnose and resolve issues with production systems, should the need arise.

```
ziti fabric validate terminators
```

## Circuit/Link Query Support

Previously listing circuit and links always showed the full list. This is because these types are in memory only and are not stored
in the bbolt datastore. There's now basic support for querying in-memory types and circuits and links can now be filtered/paged/sorted
 the same as other entity types.

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.105 -> v2.0.111](https://github.com/openziti/channel/compare/v2.0.105...v2.0.111)
    * [Issue #118](https://github.com/openziti/channel/issues/118) - Allowing checking if reconnecting impl is currently connected

* github.com/openziti/edge-api: [v0.26.0 -> v0.26.1](https://github.com/openziti/edge-api/compare/v0.26.0...v0.26.1)
* github.com/openziti/foundation/v2: [v2.0.33 -> v2.0.35](https://github.com/openziti/foundation/compare/v2.0.33...v2.0.35)
* github.com/openziti/identity: [v1.0.66 -> v1.0.68](https://github.com/openziti/identity/compare/v1.0.66...v1.0.68)
* github.com/openziti/metrics: [v1.2.37 -> v1.2.40](https://github.com/openziti/metrics/compare/v1.2.37...v1.2.40)
* github.com/openziti/runzmd: [v1.0.33 -> v1.0.36](https://github.com/openziti/runzmd/compare/v1.0.33...v1.0.36)
* github.com/openziti/sdk-golang: [v0.20.129 -> v0.20.139](https://github.com/openziti/sdk-golang/compare/v0.20.129...v0.20.139)
    * [Issue #457](https://github.com/openziti/sdk-golang/issues/457) - Add  inspect support
    * [Issue #450](https://github.com/openziti/sdk-golang/issues/450) - Support idempotent terminator creation

* github.com/openziti/secretstream: [v0.1.13 -> v0.1.14](https://github.com/openziti/secretstream/compare/v0.1.13...v0.1.14)
* github.com/openziti/storage: [v0.2.23 -> v0.2.26](https://github.com/openziti/storage/compare/v0.2.23...v0.2.26)
    * [Issue #57](https://github.com/openziti/storage/issues/57) - Support querying collections of in memory objects

* github.com/openziti/transport/v2: [v2.0.113 -> v2.0.119](https://github.com/openziti/transport/compare/v2.0.113...v2.0.119)
* github.com/openziti/ziti: [v0.31.0 -> v0.31.1](https://github.com/openziti/ziti/compare/v0.31.0...v0.31.1)
    * [Issue #1555](https://github.com/openziti/ziti/issues/1555) - Consolidate fabric/edge persistence code
    * [Issue #1547](https://github.com/openziti/ziti/issues/1547) - Support filtering, sorting and paging circuits and links
    * [Issue #1446](https://github.com/openziti/ziti/issues/1446) - Allow for idempotent sdk based terminators 
    * [Issue #1540](https://github.com/openziti/ziti/issues/1540) - Transit router create fails in HA environment
    * [Issue #1523](https://github.com/openziti/ziti/issues/1523) - Bootstrap members not working
    * [Issue #1525](https://github.com/openziti/ziti/issues/1525) - Improve cluster list output
    * [Issue #1519](https://github.com/openziti/ziti/issues/1519) - Simplify link ack handling
    * [Issue #1513](https://github.com/openziti/ziti/issues/1513) - DNS service failure should not cause a router restart
    * [Issue #1494](https://github.com/openziti/ziti/issues/1494) - Panic if applying raft log returns nil result


# Release 0.31.0

## What's New

* Rate limited for model changes

## Rate Limiter for Model Changes

To prevent the controller from being overwhelmed by a flood of changes, a rate limiter
can be enabled in the configuration file. A maximum number of queued changes can also
be configured. The rate limited is disabled by default for now. If not specified the
default number of queued changes is 100.

When the rate limit is hit, an error will be returned. If the request came in from 
the REST API, the response will use HTTP status code 429 (too many requests). 

The OpenAPI specs have been updated, so if you're using a generated client to make
REST calls, it's recommended that you regenerate your client.


```
commandRateLimiter:
    enabled:   true
    maxQueued: 100
```

If the rate limiter is enabled, the following metrics will be produced:

* `command.limiter.queued_count` - gauge of the current number of queued operations
* `command.limiter.work_timer` - timer for operations. Includes the following:
    * A histogram of how long operations take to complete 
    * A meter showing that rate at which operations are executed
    * A count of how many operations have been executed

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.15 -> v1.0.16](https://github.com/openziti/agent/compare/v1.0.15...v1.0.16)
* github.com/openziti/channel/v2: [v2.0.101 -> v2.0.105](https://github.com/openziti/channel/compare/v2.0.101...v2.0.105)
* github.com/openziti/edge-api: [v0.25.38 -> v0.26.0](https://github.com/openziti/edge-api/compare/v0.25.38...v0.26.0)
    * [Issue #49](https://github.com/openziti/edge-api/issues/49) - Add 429 responses to allow indicating that the server is too busy

* github.com/openziti/identity: [v1.0.64 -> v1.0.66](https://github.com/openziti/identity/compare/v1.0.64...v1.0.66)
* github.com/openziti/metrics: [v1.2.36 -> v1.2.37](https://github.com/openziti/metrics/compare/v1.2.36...v1.2.37)
* github.com/openziti/sdk-golang: [v0.20.122 -> v0.20.129](https://github.com/openziti/sdk-golang/compare/v0.20.122...v0.20.129)
    * [Issue #443](https://github.com/openziti/sdk-golang/issues/443) - Don't send close in response to a close on a listener

* github.com/openziti/secretstream: [v0.1.12 -> v0.1.13](https://github.com/openziti/secretstream/compare/v0.1.12...v0.1.13)
* github.com/openziti/storage: [v0.2.20 -> v0.2.23](https://github.com/openziti/storage/compare/v0.2.20...v0.2.23)
* github.com/openziti/transport/v2: [v2.0.109 -> v2.0.113](https://github.com/openziti/transport/compare/v2.0.109...v2.0.113)
* github.com/openziti/ziti: [v0.30.5 -> v0.31.0](https://github.com/openziti/ziti/compare/v0.30.5...v0.31.0)
    * [Issue #1471](https://github.com/openziti/ziti/issues/1471) - Router links not resilient to controller crash
    * [Issue #1468](https://github.com/openziti/ziti/issues/1468) - Quickstart quietly fails if password is < 5 characters long
    * [Issue #1445](https://github.com/openziti/ziti/issues/1445) - Add controller update guardrail
    * [Issue #1442](https://github.com/openziti/ziti/issues/1442) - Network watchdog not shutting down when controller shuts down
    * [Issue #1465](https://github.com/openziti/ziti/issues/1465) - Upgrade functions `getZiti` and `performMigration` were only functional on Mac OS, now they are functional for Linux and Mac OSs.
    * [Issue #1217](https://github.com/openziti/ziti/issues/1217) - Quickstart was improperly handling special characters in `ZITI_PWD`. Special characters are now supported for `ZITI_PWD` in quickstart functions.
