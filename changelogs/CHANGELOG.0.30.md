# Release 0.30.5

## What's New

* Initial proxy support in host.v1/host.v2

## Proxy Support in host.v1/host.v2

`host.v1` and `host.v2` configurations may now specify a proxy to use. 
Currently only HTTP Connect proxies which don't require authentication are supported.

**Example using `host.v1`**

```
{
    "address": "192.168.2.50",
    "port": 1234,
    "protocol": "tcp",
    "proxy": {
        "address": "192.168.1.110:3128",
        "type": "http"
    }
}
```


## Component Updates and Bug Fixes
* github.com/openziti/channel/v2: [v2.0.99 -> v2.0.101](https://github.com/openziti/channel/compare/v2.0.99...v2.0.101)
* github.com/openziti/edge-api: [v0.25.37 -> v0.25.38](https://github.com/openziti/edge-api/compare/v0.25.37...v0.25.38)
* github.com/openziti/foundation/v2: [v2.0.32 -> v2.0.33](https://github.com/openziti/foundation/compare/v2.0.32...v2.0.33)
* github.com/openziti/identity: [v1.0.63 -> v1.0.64](https://github.com/openziti/identity/compare/v1.0.63...v1.0.64)
* github.com/openziti/metrics: [v1.2.35 -> v1.2.36](https://github.com/openziti/metrics/compare/v1.2.35...v1.2.36)
* github.com/openziti/runzmd: [v1.0.32 -> v1.0.33](https://github.com/openziti/runzmd/compare/v1.0.32...v1.0.33)
* github.com/openziti/sdk-golang: [v0.20.116 -> v0.20.122](https://github.com/openziti/sdk-golang/compare/v0.20.116...v0.20.122)
    * [Issue #436](https://github.com/openziti/sdk-golang/issues/436) - HTTP calls should respect environment proxy settings

* github.com/openziti/storage: [v0.2.18 -> v0.2.20](https://github.com/openziti/storage/compare/v0.2.18...v0.2.20)
    * [Issue #52](https://github.com/openziti/storage/issues/52) - Grammar should expect single valid query followed by EOF

* github.com/openziti/transport/v2: [v2.0.107 -> v2.0.109](https://github.com/openziti/transport/compare/v2.0.107...v2.0.109)
* github.com/openziti/ziti: [v0.30.4 -> v0.30.5](https://github.com/openziti/ziti/compare/v0.30.4...v0.30.5)
    * [Issue #1336](https://github.com/openziti/ziti/issues/1336) - `ziti edge quickstart` did
      not create the usual edge router/service edge router policy.
    * [Issue #1397](https://github.com/openziti/ziti/issues/1397) - HTTP Proxy support for host.v1/host.v2 config types
    * [Issue #1423](https://github.com/openziti/ziti/issues/1423) - Controller crashes when edge router reconnects (Client Hello)
    * [Issue #1414](https://github.com/openziti/ziti/issues/1414) - Race condition in xgress_edge_tunnel tunneller at start but not seen in pre-compiled binary
    * [Issue #1406](https://github.com/openziti/ziti/issues/1406) - Entity change event dispatcher isn't shutting down properly when controller shuts down
    * [Issue #1382](https://github.com/openziti/ziti/issues/1382) - service failure costs are not shrinking over time

# Release 0.30.4

## What's New

* `ziti edge quickstart`](https://github.com/openziti/ziti/issues/1298). You can now
  download the `ziti` CLI and have a functioning network with just one command. The
  network it creates is ephemeral and is intended to be torn down when the process exits.
  It is intended for quick evaluation and testing of an overlay network. It supports the
  following flags:

  ```
      --already-initialized     Specifies the PKI does not need to be created and the db does not need to be initialized. Recommended to be combined with --home. If --home is not specified the environment will be destroyed on shutdown! default: false
      --ctrl-address string     Sets the advertised address for the control plane and API
      --ctrl-port int16         Sets the port to use for the control plane and API
  -h, --help                    help for quickstart
      --home string             Sets the directory the environment should be installed into. Defaults to a temporary directory. If specified, the environment will not be removed on exit.
  -p, --password string         Password to use for authenticating to the Ziti Edge Controller. default: admin
      --router-address string   Sets the advertised address for the integrated router
      --router-port int16       Sets the port to use for the integrated router
  -u, --username string         Username to use when creating the Ziti Edge Controller. default: admin
  ```

  Example Usage:
  ```
  ziti edge quickstart \
    --ctrl-address potato \
    --ctrl-port 12345 \
    --router-address avocado \
    --router-port 23456 \
    --home $HOME/.ziti/pet-ziti \
    --already-initialized \
    --username someOtherUsername \
    --password someOtherPassword
  ```

* Bugfixes

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v0.30.3 -> v0.30.4](https://github.com/openziti/ziti/compare/v0.30.3...v0.30.4)
  * Fixed an issue causing router configs to be rewritten when docker compose was brought up with existing configs

# Release 0.30.3

## What's New

* Bugfixes

## Component Updates and Bug Fixes

* github.com/openziti/edge: [v0.24.401 -> v0.24.404](https://github.com/openziti/edge/compare/v0.24.401...v0.24.404)
* github.com/openziti/fabric: [v0.24.20 -> v0.24.23](https://github.com/openziti/fabric/compare/v0.24.20...v0.24.23)
  * [Issue #786](https://github.com/openziti/fabric/issues/786) - entityChangeEventDispatcher.flushLoop doesn't shutdown when controller shuts down
  * [Issue #785](https://github.com/openziti/fabric/issues/785) - Allow link groups to be single string value
  * [Issue #783](https://github.com/openziti/fabric/issues/783) - Raft cluster connections not updated for ALPN

* github.com/openziti/ziti: [v0.30.2 -> v0.30.3](https://github.com/openziti/ziti/compare/v0.30.2...v0.30.3)

# Release 0.30.2

## What's New

* Identity type consolidation
* HTTP Connect Proxy support for control channel and links

## Identity Type Consolidation

Prior to this release there were four identity types:

* User
* Service
* Device
* Router

Of these four types, only Router has any functional purpose. Given that, the other three have been merged into
a single `Default` identity type. Since Router identities can only be created by the system, it's no longer
necessary to specify the identity type when creating identities.

The identity type may still be provided, but a deprecation warning will be emitted.

**Backwards Compatibility**

Existing non-Router identities will be migrated to the `Default` identity type. If an identity type other
than `Default` is provided when creating an identity, it will be coerced to the `Default` type. Existing
code may have issues with the new identity type being returned.


## HTTP Connect Proxy support

Routers may now specify a proxy configuration which will be used when establishing connections to controllers
and data links to other routers. At this point only HTTP Connect Proxies with no authentication required are
supported.

Example router config:

```yaml
proxy:
  type: http
  address: localhost:3128

```

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.91 -> v2.0.95](https://github.com/openziti/channel/compare/v2.0.91...v2.0.95)
* github.com/openziti/edge: [v0.24.381 -> v0.24.401](https://github.com/openziti/edge/compare/v0.24.381...v0.24.401)
  * [Issue #1597](https://github.com/openziti/edge/issues/1597) - ER/T cached API sessions aren't remembered when pulled from cache
  * [Issue #1428](https://github.com/openziti/edge/issues/1428) - Make identity type optional. Consolidate User/Service/Device to 'Default'.
  * [Issue #1584](https://github.com/openziti/edge/issues/1584) - AuthPolicyDetail is incompatible with API response

* github.com/openziti/edge-api: [v0.25.31 -> v0.25.33](https://github.com/openziti/edge-api/compare/v0.25.31...v0.25.33)
* github.com/openziti/fabric: [v0.24.2 -> v0.24.20](https://github.com/openziti/fabric/compare/v0.24.2...v0.24.20)
  * [Issue #775](https://github.com/openziti/fabric/issues/775) - Add support for proxies for control channel and links

* github.com/openziti/foundation/v2: [v2.0.29 -> v2.0.30](https://github.com/openziti/foundation/compare/v2.0.29...v2.0.30)
* github.com/openziti/identity: [v1.0.60 -> v1.0.61](https://github.com/openziti/identity/compare/v1.0.60...v1.0.61)
* github.com/openziti/runzmd: [v1.0.29 -> v1.0.30](https://github.com/openziti/runzmd/compare/v1.0.29...v1.0.30)
* github.com/openziti/sdk-golang: [v0.20.90 -> v0.20.101](https://github.com/openziti/sdk-golang/compare/v0.20.90...v0.20.101)
* github.com/openziti/storage: [v0.2.12 -> v0.2.14](https://github.com/openziti/storage/compare/v0.2.12...v0.2.14)
* github.com/openziti/transport/v2: [v2.0.99 -> v2.0.103](https://github.com/openziti/transport/compare/v2.0.99...v2.0.103)
  * [Issue #54](https://github.com/openziti/transport/issues/54) - Support HTTP Connect proxying for TLS connections

* github.com/openziti/metrics: [v1.2.31 -> v1.2.33](https://github.com/openziti/metrics/compare/v1.2.31...v1.2.33)
* github.com/openziti/secretstream: [v0.1.10 -> v0.1.11](https://github.com/openziti/secretstream/compare/v0.1.10...v0.1.11)
* github.com/openziti/ziti: [v0.30.1 -> v0.30.2](https://github.com/openziti/ziti/compare/v0.30.1...v0.30.2)
  * [Issue #1266](https://github.com/openziti/ziti/issues/1266) - Outdated README.md: Some links return "Page Not Found"


# Release 0.30.1

## What's New

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v0.30.0 -> v0.30.1](https://github.com/openziti/ziti/compare/v0.30.0...v0.30.1)
  * [Issue #1225](https://github.com/openziti/ziti/issues/1225) - Updated ZITI_ROUTER_ADVERTISED_HOST to use the more common naming convention of ZITI_ROUTER_ADVERTISED_ADDRESS
  * [Issue #1233](https://github.com/openziti/ziti/issues/1233) - Added `lsof` to the list of prerequisites to be checked during quickstart

# Release 0.30.0

## What's New

* Link management is now delegated to routers
* Controller and routers can operate with a single listening port

## Link Management Updates

Previously, the controller would do its best to determine where links needed to be established.
It would send messages to the routers, telling them which addresses to dial on other routes.
The routers would in turn let the controller know if link establishment was successful or
if the router already had a link to the given endpoint.

With this release, the controller will only let routers know which routers exist, whether they
are currently connected to the controller, and what link listeners they are advertising. The
routers will now decide which links to make and let the controllers know as links are created
and broken.

### Link Groups

Both dialers and listeners can now specify a set of groups. If no groups are specified, the
dialer or listener will be placed in the `default` group. Dialers will only attempt to dial
listeners who have at least one group in common with them.


### Failed Links

Previously when a link failed, the controller would show it in the link list as failed for a time
before removing it. Now failed links are removed immediately. There are existing link events for
link creation and link failure which can be used for forensics.

### Duplicate Links

There is a new link status `Duplicate` used when a router receives a link request and determines
that it's a duplicate of an existing link. This happens when two routers both have listeners
and dialers. They will often dial each other at the same time, resulting in a duplicate link.

### Compatibility

If you use a 0.30+ controller with older routers, the controller will still do link calculation
and send dial messages, as long as the `enableLegacyLinkMgmt` setting is set to true.

If you use a pre 0.30.0 controller with newer routers, the new routers will still accept the
dial messages.

### New Configuration

#### Controller

The controller has three new options:

```
network:
    routerMessaging:
        queueSize: 100
        maxWorkers: 100
    enableLegacyLinkMgmt: true
```

When a router connects or disconnects from the controller, we send two sets of updates.

1. If a router has connected we send it the the state of the other routers
1. We send all the other routers the updated state of the connecting/disconnecting router

These messages are sent using a worker pool. The size of the queue feeding the worker pool is controlled with
`routerMessaging.queueSize`. The max size of the worker pool is controlled used the `routerMessaging.maxWorkers`
option.

* queueSize
  * Min value: 0
  * Max value: 1,000,000
  * Default: 100
* maxWorkers
  * Min value: 1
  * Max value: 10,000
  * Default: 100

If you have routers older than 0.30.0, the controller will calculate which links to dial. This can be disabled
by setting `enableLegacyLinkMgmt` to false. This setting currently defaults to true, but will default to false
in a future release. In a subsequent release this functionality will be removed all together.

#### Router

The router has new configuration options for link dialing.

```
link:
   dialers:
       - binding: transport
         groups: 
             - public
             - vpc1234
         healthyDialBackoff:
             retryBackoffFactor: 1.5
             minRetryInterval: 5s
             maxRetryInterval: 5m
         unhealthyDialBackoff:
             retryBackoffFactor: 10
             minRetryInterval: 1m
             maxRetryInterval: 1h
    listeners:
        - binding: transport
          groups: vpc1234
```

**Groups**

See above for a description of link groups work.

Default value: `default`

**Dial Back-off**

Dialers can be configured with custom back-off behavior. Each dialer has a back-off policy for dialing
healthy routers (those that are connected to a controller) and a separate policy for unhealthy routers.

The back-off policies have the following attributes:

* minRetryInterval - duration specifying the minimum time between dial attempts
  * Min value: 10ms
  * Max value: 24h
  * Default: 5s for healthy, 1m for unhealthy
  * Format: Golang Durations, see: https://pkg.go.dev/maze.io/x/duration#ParseDuration
* maxRetryInterval - duration specifying the maximum time between dial attempts
  * Min value: 10ms
  * Max value: 24h
  * Default: 5m for healthy, 1h for unhealthy
  * Format: Golang duration, see: https://pkg.go.dev/maze.io/x/duration#ParseDuration
* retryBackoffFactor - factor by which to increase the retry interval between failed dial attempts
  * Min value: 1
  * Max value: 100
  * Default: 1.5 for healthy, 100 for unhealthy


## Single Port Support / ALPN
Ziti Controller and Routers can operate with a single open port. In order to implement this feature we use
ALPN ([Application Layer Protocol Negotiation](https://en.wikipedia.org/wiki/Application-Layer_Protocol_Negotiation))
TLS extension. It allows TLS client to request and TLS server to select appropriate application protocol handler during
TLS handshake.

### Protocol Details

The following protocol identifiers are defined:

| id        | purpose |
| --------- | ------- |
| ziti-ctrl | Control plane connections             |
| ziti-link | Fabric link  connections              |
| ziti-edge | Client SDK connection to Edge Routers |

Standard HTTP protocol identifiers (`h2`, `http/1.1`) are used for Controller REST API and Websocket listeners.

### Backward Compatibility

This feature is designed to be backward compatible with SDK clients: older client will still be able to connect without
requesting `ziti-edge` protocol.

**Breaking**

Older routers won't be able to establish control channel or fabric links with updated network.
However, newer Edge Routers should be able to join older network in some circumstances -- only outbound links from new Routers would work.

## Component Updates and Bug Fixes
* github.com/openziti/agent: [v1.0.14 -> v1.0.15](https://github.com/openziti/agent/compare/v1.0.14...v1.0.15)
* github.com/openziti/channel/v2: [v2.0.84 -> v2.0.91](https://github.com/openziti/channel/compare/v2.0.84...v2.0.91)
  * [Issue #108](https://github.com/openziti/channel/issues/108) - Reconnecting underlay not returning headers from hello message

* github.com/openziti/edge: [v0.24.364 -> v0.24.381](https://github.com/openziti/edge/compare/v0.24.364...v0.24.381)
  * [Issue #1548](https://github.com/openziti/edge/issues/1548) - Panic in edge@v0.24.326/controller/sync_strats/sync_instant.go:194

* github.com/openziti/edge-api: [v0.25.30 -> v0.25.31](https://github.com/openziti/edge-api/compare/v0.25.30...v0.25.31)
* github.com/openziti/fabric: [v0.23.45 -> v0.24.2](https://github.com/openziti/fabric/compare/v0.23.45...v0.24.2)
  * [Issue #766](https://github.com/openziti/fabric/issues/766) - Lookup of terminators with same instance id isn't filtering by instance id
  * [Issue #692](https://github.com/openziti/fabric/issues/692) - Add ability to control link formation between devices more granularly
  * [Issue #749](https://github.com/openziti/fabric/issues/749) - Move link control to router
  * [Issue #343](https://github.com/openziti/fabric/issues/343) - Link state Failed on startup

* github.com/openziti/foundation/v2: [v2.0.28 -> v2.0.29](https://github.com/openziti/foundation/compare/v2.0.28...v2.0.29)
* github.com/openziti/identity: [v1.0.59 -> v1.0.60](https://github.com/openziti/identity/compare/v1.0.59...v1.0.60)
* github.com/openziti/runzmd: [v1.0.28 -> v1.0.29](https://github.com/openziti/runzmd/compare/v1.0.28...v1.0.29)
* github.com/openziti/sdk-golang: [v0.20.78 -> v0.20.90](https://github.com/openziti/sdk-golang/compare/v0.20.78...v0.20.90)
* github.com/openziti/storage: [v0.2.11 -> v0.2.12](https://github.com/openziti/storage/compare/v0.2.11...v0.2.12)
* github.com/openziti/transport/v2: [v2.0.93 -> v2.0.99](https://github.com/openziti/transport/compare/v2.0.93...v2.0.99)
* github.com/openziti/xweb/v2: [v2.0.2 -> v2.1.0](https://github.com/openziti/xweb/compare/v2.0.2...v2.1.0)
* github.com/openziti/ziti-db-explorer: [v1.1.1 -> v1.1.3](https://github.com/openziti/ziti-db-explorer/compare/v1.1.1...v1.1.3)
  * [Issue #4](https://github.com/openziti/ziti-db-explorer/issues/4) - db explore timeout error is uninformative

* github.com/openziti/metrics: [v1.2.30 -> v1.2.31](https://github.com/openziti/metrics/compare/v1.2.30...v1.2.31)
* github.com/openziti/ziti: [v0.29.0 -> v0.30.0](https://github.com/openziti/ziti/compare/v0.29.0...v0.30.0)
  * [Issue #1199](https://github.com/openziti/ziti/issues/1199) - ziti edge list enrollments - CLI gets 404
  * [Issue #1135](https://github.com/openziti/ziti/issues/1135) - Edge Router: Support multiple protocols on the same listener port
  * [Issue #65](https://github.com/openziti/ziti/issues/65) - Add ECDSA support to PKI subcmd
  * [Issue #1212](https://github.com/openziti/ziti/issues/1212) - getZiti fails on Mac OS
  * [Issue #1220](https://github.com/openziti/ziti/issues/1220) - Fixed getZiti function not respecting user input for custom path
  * [Issue #1219](https://github.com/openziti/ziti/issues/1219) - Added check for IPs provided as a DNS SANs entry, IPs will be ignored and not added as a DNS entry in the expressInstall PKI or router config generation.
