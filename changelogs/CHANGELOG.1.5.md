# Release 1.5.4

## What's new

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v1.5.3 -> v1.5.4](https://github.com/openziti/ziti/compare/v1.5.3...v1.5.4)
    * [Issue #2947](https://github.com/openziti/ziti/issues/2947) - Panic on router started up if edge/tunnel bindings not configured
    * [Issue #2948](https://github.com/openziti/ziti/issues/2948) - Allow ER/T to run without edge listener

# Release 1.5.3

## What's New

* This release updates the Go version from 1.23 to 1.24

# Release 1.5.2

## What's New

* This release reverts a change refactoring some flow-control apis, as the change caused a panic

# Release 1.5.1

## What's New

* Bug fixes and features

## Component Updates and Bug Fixes

* github.com/openziti/sdk-golang: [v0.25.0 -> v0.25.1](https://github.com/openziti/sdk-golang/compare/v0.25.0...v0.25.1)
  * [Issue #699](https://github.com/openziti/sdk-golang/issues/699) - SDK UPDB enrollment

* github.com/openziti/ziti: [v1.5.0 -> v1.5.1](https://github.com/openziti/ziti/compare/v1.5.0...v1.5.1)
  * [Issue #2931](https://github.com/openziti/ziti/issues/2931) - help ext-jwt-signer auth by logging incoming jwt audience
  * [Issue #2934](https://github.com/openziti/ziti/issues/2934) - API Session Certs in HA not connect to ERs in all scenarios
  * [Issue #2926](https://github.com/openziti/ziti/issues/2926) - Implement minimal Xgress SDK


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
