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
