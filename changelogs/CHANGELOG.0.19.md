# Release 0.19.13

## What's New

* Fix bug in tunneler source transparency when using UDP
* Added guidance under /quickstart for quickly launching a simplified, local environment suitable for local dev testing and learning
* Removed xtv framework from fabric and moved edge terminator identity validation to control channel handler. The `terminator:` section may be removed from the controller configuration file.
* Listen for SIGINT for router shutdown
* Implement dial and listen identity options in go tunneler
* Edge REST API Deprecation Warnings
* Posture Check Process Multi

### Deprecation Warning Of Non-Prefixed Edge REST API

Upcoming changes will remove support for non-prefixed Edge REST API URLs. The correct API URL prefix has been `edge/v1`
for over a year and not using will become unsupported at a future date. Additionally, the Edge REST API will be splitting
into two separate APIs in the coming months:

  - `/edge/management/v1`
  - `/edge/client/v1`

These new prefixes are not currently live and will be released in a subsequent version.

### Posture Check Process Multi

A new posture check type has been introduced: `PROCESS_MULTI`. This posture check
is meant to replace the posture check type `PROCESS` and `PROCESS` should be considered
deprecated. `PROCESS_MULTI` covers all the uses that its predecessor provided with
additional semantic configuration options.

#### Process Multi Fields:

- semantic: Either `AllOf` or `OneOf`. Determines which processes specified in `processes` must pass
- processes: An array of objects representing a process. Similar to `PROCESS`'s fields but with the ability to specify multiple binary hashes
  -  osType - Any of the standard posture check OS types (Android, iOS, macOS, Linux, Windows, WindowsServer)
  -  path - The absolute file path the process is expected to run from
  -  hashes - An array of sha512 hashes that are valid (optional, none allows any)
  -  signerFingerprints - An array of sha1 signer fingerprints that are valid (optional, none allows any)

# Release 0.19.12

## What's New

* Revert dial error messages to what sdks are expecting. Add error codes so future sdks don't have to parse error text
* Add router events
* Allow filters with no predicate if sort or paging clauses are provided
   * Ex: instead of `true limit 5` you could have just `limit 5`. Or instead of `true sort by name` you could have `sort by name`
* Corrected host.v1 configuration type schema to prevent empty port range objects

## Router events

To enable:

```
events:
  jsonLogger:
    subscriptions:
      - type: fabric.routers
```

Example JSON output:

```
{
  "namespace": "fabric.routers",
  "event_type": "router-online",
  "timestamp": "2021-04-22T11:26:31.99299884-04:00",
  "router_id": "JAoyjafljO",
  "router_online": true
}
{
  "namespace": "fabric.routers",
  "event_type": "router-offline",
  "timestamp": "2021-04-22T11:26:41.335114358-04:00",
  "router_id": "JAoyjafljO",
  "router_online": false
}
```

# Release 0.19.11

## What's New

* Add workaround for bbolt bug which caused some data to get left behind when deleting identities, seen when turning off tunneler capability on edge routers
* Remove deprecated ziti-enroller command. Enrollement can be done using the ziti-tunnel, ziti-router and ziti commands
* Fix UDP intercept handling
* The host.v1 service configuration type has been changed as follows:
    * Rename `dialIntercepted*` properties to `forwardProtocol`, `forwardAddress`, `forwardPort` for better consistency with non-tunneler client applications.
    * Add `allowedProtocols`, `allowedAddresses`, and `allowedPortRanges` properties to whitelist destinations that are dialed via `forward*`. The `allowed*` properties are required for any corresponding `forward*` property that is `true`.
    * Add `allowedSourceAddresses`, which serves as a whitelist for source IPs/CIDRs and informs the hosting tunneler of the local routes to establish when hosting a service.
* Ziti Controller will now report service posture query policy types (Dial/Bind)
* Ziti Controller now supports enrollment extension for routers
* Ziti Router now support forcing enrollment extension via `run -e`
* Ziti Routers will now automatically extend their enrollment before their certificates expire
* `ziti edge enroll` with a UPDB JWT now confirms and properly sets the password supplied

  Caveats:
    * Any existing host.v1 configurations that use will become invalid.
    * ziti-tunnel and the converged router/tunneler creates local routes that are established for `allowedSourceAddresses`, but the routes are not consistently cleaned up when `ziti-tunnel` exits. This issue will be addressed in a future release.

# Release 0.19.10

## What's New

* Fixed issue where edge router renames didn't propagate to fabric
* Fixed issue where gateway couldn't dial after router rename
* Allowed parsing identities where values were not URIs
* Allow updating which configs are used by services

# Release 0.19.9

## What's New

* Converged Tunneler/Router (Beta 2)
    * intercept.v1 support (also in ziti-tunnel)
    * support for setting per-service hosting cost/precedence on identity
* Identities and edge routers now support appData, which is tag data consumable by sdks
* Edge Router Policies now expose the `isSystem` flag for system managed policies
* ziti-tunnel no longer supports tun mode. It has been superseded by tproxy mode

## Fixes

* Fix deadlock in ziti-router which would stop new connections from being established after an api session is removed
* Fix id extraction for data plane link latency metrics
* Fix id extraction for ctrl plane link latency metrics
* ziti-tunnel wasn't asking for host.v1/host.v2 configs

## Per-service Cost/Precedence

Previously support was adding for setting the default cost and precedence that an identity would use when hosting services. Now, in addition to setting default values, costs and precedences can be set per-service. These are exposed as maps keyed by service. There is one map for costs and another for precedences. There is also CLI support for setting these values.

Example creating an identity:

```
ziti edge create identity service test2 --default-hosting-cost 10 --default-hosting-precedence failed --service-costs loop=20,echo=30 --service-precedences loop=default,echo=required

```

When viewing the identity, these values can be seen:

```
    "id": "-qJRZFqV8t",
    "tags": {},
    "updatedAt": "2021-04-05T16:54:42.763Z",
    "authenticators": {},
    "defaultHostingCost": 10,
    "defaultHostingPrecedence": "failed",
    "envInfo": {},
    "hasApiSession": false,
    "hasEdgeRouterConnection": false,
    "isAdmin": false,
    "isDefaultAdmin": false,
    "isMfaEnabled": false,
    "name": "test2",
    "roleAttributes": null,
    "sdkInfo": {},
    "serviceHostingCosts": {
        "vH3QndzRYt": 20,
        "wAnuyO3PmI": 30
    },
    "serviceHostingPrecedences": {
        "vH3QndzRYt": "default",
        "wAnuyO3PmI": "required"
    },

```

Note that this mechanism replaces setting cost and precedence via the host.v1/host.v2 config types `listenOptions`. Those values will be ignored, and may in future be removed from the schemas.

## Identity/Edge Router app data

We have an existing tags mechanism, which can be used by system administrators to annotate ziti entities in whatever is useful to them. Tags are an administrator function and are not meant to be visible to SDKs and SDK applications. If an administrator wants to provide custom data for services to the SDK they can use config types and configs for that purpose. Up until now however, there hasn't been a means to annotate identities and edge routers, which are the other two entities visible to SDKs, with data that the SDKs can consume.
0.19.9 introduces `appData` on identities and edge-routers. `appData` has the same structure as `tags`, but is intended to allow administrators to push custom data on identities and edge routers to SDKs. An example use is for tunnelers. The `sourceIp` can contain template information, which can refer back to the `appData` for the tunneler's identity.

The CLI supports setting appData on identities and edge routers.

```
ziti edge create identity service myIdentity --tags office=Regional5,device=Laptop --app-data ip=1.1.1.1,QoS=voip

```

# Release 0.19.8

## What's New

* Transwarp beta_1
* Converged Tunneler/Router (Beta)
    * Updates to the host.v1 config type
    * New host.v2 config type
    * New health events

## Fixes

* Service Policies with no Posture Checks properly grant access even if an identity has another Service Policies with posture checks
* Removing entities with no referencing entities via attributes no longer panics
* API Sessions and Current Api Session now share the same modeling logic
* API Sessions now have lastActivityAt and cachedLastActivityAt
* Enrolling and unenrolling in MFA sets MFA posture data

## Transwarp beta_1

See the Transwarp beta_1 guide [here](doc/transwarp_b1/transwarp_b1.md).

## Converged Tunneler/Router (Beta)

ziti-router can now run with the tunneler embedded. It has the same capabilities as ziti-tunnel. As ziti-tunnel gains new features, the combined ziti-router/tunnel should maintain feature parity.

### Beta Status

This is a beta release. It should be relatively feature complete and bug-free for hosting services but intercept support is still nascent. Some features are likely to be changed based on user feedback.

### Os Compatibility

Like ziti-tunnel, `tproxy` mode will only work on linux. `proxy` and `host` modes should work on all operating systems.

The converged tunneler/router does not support running in tun mode. This mode may be deprecated for ziti-tunnel as well at some point in the future if no advantages over tproxy mode are evident.

### Supported configurations

The following configurations are supported:

* `ziti-tunneler-client.v1`
* `ziti-tunneler-server.v1`
* `host.v1` (updated)
* `host.v2` (new)

Support for `intercept.v1` will be added in a follow-up release.

### Router Identities

When an edge router is marked as being tunneler enabled, a matching identity will be created, of type Router, as well as an edge router policy. The edge router policy ensures that the identity always
has access to the edge router. The identity allows the router to be included in service policies, to configure which services will intercepted/hosted.

1. The identity will have the same id and name as the edge router
1. If an identity with the same name as the router already exists, the router create/update will fail
1. When the router name is changed, the identity name will be updated as well.
1. The identity name and type cannot be changed directly. The type may not be changed at all and the name may only be changed by changing the name of the router.
1. The identity may not be deleted except by deleting the router or disabling tunneler support for the identity.
1. When the router is deleted, the accompanying identity and edge router policy will also be deleted
1. If tunneler support is disabled in the router, the accompanying identity and edge router policy will also be deleted.
1. The edge router policy will have the same id as the router and have a name of the form `edge-router-<edge-router-id>-system`, where `<edge-router-id>` is replaced by the id of the edge router.
1. The edge router policy is considered a `system` entity, and cannot be updated and cannot deleted except by the system when the associated router is deleted.

### Tunneler Prerequisites

In order for a router instance to host a tunneler, it must meet the following criteria:

1. It must be represented in the model by an edge router
2. The edge router field `isTunnelerEnabled` must be set to true
3. Edge functionality in the router must be enabled, which means the `edge:` config section must be present. NOTE: The edge listener does **not** need to be enabled.
4. The tunnel listener must be enabled.

### Making an Edge Router Tunneler enabled

The ziti CLI can be used to enable/disable tunneler support on edge routers. When creating an edge router, the `-t` flag can be passed in to enable running the tunneler.

```shell
ziti edge create edge-router myEdgeRouter --tunneler-enabled
```

or

```shell
ziti edge create edge-router myEdgeRouter -t
```

An existing edge router can be marked as tunneler enabled as follows:

```shell
ziti edge update edge-router myEdgeRouter --tunneler-enabled
```

or

```shell
ziti edge update edge-router myEdgeRouter -t
```

An existing edge router can be marked as not supporting the tunneler as follows:

```shell
ziti edge update edge-router myEdgeRouter -t=false
```

or

```shell
ziti edge update edge-router myEdgeRouter --tunneler-enabled=false
```

### Tunnel listener configuration

```yaml

listeners:
  - binding: tunnel
    options:
      mode: tproxy # mode to run in. Valid values [tproxy, host, proxy]. Default: tproxy
      svcPollRate: 15s # How often to poll for service changes. Default: 15s
      resolver: udp://127.0.0.1:53 # DNS resolve. Default: udp://127.0.0.1:53 for tproxy, blank for others
      dnsSvcIpRange: 100.64.0.1/10 # cidr to use when assigning IPs to unresolvable intercept hostnames (default "100.64.0.1/10")
      services: # services to intercept in proxy mode. Default: none
        - echo:1977
      lanIf: tun1 # if specified, INPUT rules for intercepted service addresses are assigned to this interface. Defaults to unspecified.
```

### Updates to health checks/health check events

Health checks now have a new trigger option `change`. This will trigger the action whenever the health state changes from pass to fail or fail to pass.

There's now a new action, `send event`, which will send a health event to the controller. This will be reported via the recently introduced service events.

The `change` option is designed to be sued with `send event` as a way to send events only when the state changes, rather than sending every single pass/fail.

An example configuration:

```json

{
  "address": "localhost",
  "port": 8171,
  "protocol": "tcp",
  "portChecks": [
    {
      "address": "localhost:8171",
      "interval": "5s",
      "timeout": "100ms",
      "actions": [
        {
          "trigger": "change",
          "action": "send event"
        }
      ]
    }
  ]
}
```

What the service events look like:

```json
{
  "namespace": "service.events",
  "event_type": "service.health_check.failed",
  "service_id": "vH3QndzRYt",
  "count": 2,
  "interval_start_utc": 1616703360,
  "interval_length": 60
}
{
  "namespace": "service.events",
  "event_type": "service.health_check.failed",
  "service_id": "vH3QndzRYt",
  "count": 1,
  "interval_start_utc": 1616703720,
  "interval_length": 60
}
{
  "namespace": "service.events",
  "event_type": "service.health_check.passed",
  "service_id": "vH3QndzRYt",
  "count": 2,
  "interval_start_utc": 1616703720,
  "interval_length": 60
}
```

### Changes to host.v1

The host.v1 config type now support health checks. The configuration is the same for `ziti-tunneler-server-v1`.

See here: https://github.com/openziti/edge/blob/v0.19.54/tunnel/entities/host.v1.json for the full schema

### host.v2

There is a new host configuration, `host.v2`, which should supersede `host.v1`. The primary difference from `host.v1` is that it supports multiple terminators. This means that when a service is hosted by a router, it can connect to multiple service instances if the service is horizontally scaled, or in a primary/failover setup.

Each terminator definition is the same as a `host.v1` configuration, which one exception: `listOptions.connectTimeoutSeconds` is now `listenOptions.connectTimeout` and is specified as a duration (`5s`, `2500ms`, etc).

Example:

```json
{
  "terminators": [
    {
      "address": "localhost",
      "port": 8171,
      "protocol": "tcp",
      "listenOptions": {
        "cost": 50,
        "precedence": "required"
      },
      "portChecks": [
        {
          "address": "localhost:8171",
          "interval": "5s",
          "timeout": "100ms",
          "actions": [
            {
              "trigger": "change",
              "action": "send event"
            },
            {
              "trigger": "fail",
              "action": "increase cost 25"
            },
            {
              "trigger": "pass",
              "action": "decrease cost 10"
            }
          ]
        }
      ]
    },
    {
      "address": "localhost",
      "port": 8172,
      "protocol": "tcp",
      "listenOptions": {
        "cost": 51,
        "precedence": "required"
      },
      "portChecks": [
        {
          "address": "localhost:8172",
          "interval": "5s",
          "timeout": "100ms",
          "actions": [
            {
              "trigger": "change",
              "action": "send event"
            },
            {
              "trigger": "fail",
              "action": "increase cost 25"
            },
            {
              "trigger": "pass",
              "action": "decrease cost 10"
            }
          ]
        }
      ]
    }
  ]
}

```

# Release 0.19.7

## What's New

* Update to Golang 1.16
* Idle route garbage collection: orphaned routing table entries will be garbage collected. New
  infrastructure for session confirmation facilitating additional types of garbage collection
* Configurable Xgress dial "dwell time"
* Database tracing support
* Immediately close router ctrl channel connection if fingerprint validation fails
* Immediately close router ctrl channel if no version info is provided
* Ziti Controller Remove All Ziti Controller Remove API Sessions and Edge Sessions API Sessions and
  Edge Sessions
* Fixed posture check error responses to include only failed checks
* Heartbeat Collection And Batching
* Add Service Request Failures for Posture Checks
* Remove database migration code for versions older than 0.17.1

### Idle Route Garbage Collection

The following router configuration stanza controls idle route garbage collection:

```
forwarder:
  #
  # After how many milliseconds of inactivity is a forwarding table entry considered idle?
  #
  idleSessionTimeout: 60000
  #
  # How frequently will we confirm idle sessions with the controller?
  #
  idleTxInterval: 60000
```

### Xgress Dial Dwell Time

The following router configuration stanza controls Xgress dial "dwell time". You probably don't want
to use this unless you're debugging a timing-related issue in the overlay:

```
forwarder:
  #
  # (Debugging) Xgress dial "dwell time". When dialing, the Xgress framework will wait this number of milliseconds
  # before responding in the affirmative to the controller.
  #
  xgressDialDwellTime: 0
```

### Database Tracing

Enable database tracing using the `dbTrace` controller configuration directive:

```
dbTrace: true
```

This will result in log output that describes the entrance into and exit from transactional
functions operating against the underlying database:

```
[   0.003]    INFO fabric/controller/db.traceUpdateEnter: Enter Update (tx:18) [github.com/openziti/fabric/controller/db.createRoots]
[   0.003]    INFO fabric/controller/db.traceUpdateExit: Exit Update (tx:18) [github.com/openziti/fabric/controller/db.createRoots]
[   0.006]    INFO fabric/controller/db.traceUpdateEnter: Enter Update (tx:19) [github.com/openziti/foundation/storage/boltz.(*migrationManager).Migrate.func1]
[   0.006]    INFO foundation/storage/boltz.(*migrationManager).Migrate.func1: fabric datastore is up to date at version 4
[   0.006]    INFO fabric/controller/db.traceUpdateExit: Exit Update (tx:19) [github.com/openziti/foundation/storage/boltz.(*migrationManager).Migrate.func1]
```

### Ziti Controller Remove All Ziti Controller Remove API Sessions and Edge Sessions API Sessions and Edge Sessions

With the Ziti Controller shutdown, it is now possible to clear out all API Sessions and Edge
Sessions that were persisted prior to the controller being stopped. All connecting identities will
need to re-authenticate when the controller is restarted.

This command is useful in situations where the number of sessions is large and the database is being
copied and/or used for debugging.

Example Command:
`ziti-controller delete-sessions`

Example Output:

```
[   0.017]    INFO ziti/ziti-controller/subcmd.deleteSessions: {go-version=[go1.16] revision=[local] build-date=[2020-01-01 01:01:01] os=[windows] arch=[amd64] version=[v0.0.0]} removing API Sessions and Edge Sessions from ziti-controller
[   9.469]    INFO ziti/ziti-controller/subcmd.deleteSessions.func2: existing api Sessions: 2785
[  18.274]    INFO ziti/ziti-controller/subcmd.deleteSessions.func2: edge sessions bucket does not exist, skipping, count is: 4035
[  47.104]    INFO ziti/ziti-controller/subcmd.deleteSessions.func3: done removing api Sessions
[  55.866]    INFO ziti/ziti-controller/subcmd.deleteSessions.func3: done removing api session indexes
[  58.325]    INFO ziti/ziti-controller/subcmd.deleteSessions.func3: done removing edge session indexes
```

### Heartbeat Collection And Batching

In previous versions heartbeats from REST API usage and discrete Edge Router connection would all
cause writes for the same API Session as they were encountered. In situations where one or more REST
API requests were issues and/or one or more Edge Router connections were held by a ZitI Application,
multiple simultaneous heartbeats could occur for no apparent benefit and consume disk write I/O.

Heartbeats are now aggregated over a window of time in a cache and written to disk on an interval.
The write interval defaults to 90s and the batch size (for write transactions) to 250. Additionally,
all heartbeats are flush to disk when the controller is properly shut down.

These settings can be defined in the `edge.api` section for the Ziti Controller configuration.

Example:

```
edge:
  api:
    ...
    #(optional, default 90s) Alters how frequently heartbeat and last activity values are persisted
    activityUpdateInterval: 90s
    #(optional, default 250) The number of API Sessions updated for last activity per transaction
    activityUpdateBatchSize: 250
    ...
```

### Add Service Request Failures for Posture Checks

When a Ziti Identity (endpoint) requests a service that is provided via a Service Policy with
Posture Checks associated with it, failure to meet the Posture Check requirements results in an
error message with a code of `INVALID_POSTURE` and data elaborating on what Service Policies ids and
Posture Check ids failed. Additionally, now the controller will log the most recent requests and
provide detailed error output for administrators.

The output will show every Service Policy that was checked for access and every Posture Check that
failed. The failed Posture Checks include the Posture Data that was available for the identity and
the requirements defined in the Posture Check at the time of the Edge Session request.

New Endpoint: `GET /identities/{id}/failed-service-requests`

Example Output:

```
{
    "data": [
        {
            "apiSessionId": "ckmgcom680002cc61hggmcy1n",
            "policyFailures": [
                {
                    "policyId": "eqggCQlBm",
                    "policyName": "alldial",
                    "checks": [
                        {
                            "actualValue": {
                                "hash": "123",
                                "isRunning": true,
                                "signerFingerprints": [
                                    "834f29a60152ce36eb54af37ca5f8ec029eccf01",
                                    "123248b9e8b0dd41938018a871a13dd92bed4456"
                                ]
                            },
                            "expectedValue": {
                                "hashes": [
                                    "3af35956a71c2afefbfe356f86c9139725eeecb15f0de7d98557d4d696c434f51fbc2fa5f7543aef4f5f1afb83caa4a43619973bae52e1f4f92ec10c39b039e8"
                                ],
                                "osType": "Windows",
                                "path": "C:\\Program Files\\TestApp\\test.exe",
                                "signerFingerprint": "834f29a60152ce36eb54af37ca5f8ec029eccf01"
                            },
                            "postureCheckId": "UF9aOqlD3",
                            "postureCheckName": "processCheck",
                            "postureCheckType": "PROCESS"
                        },
                        {
                            "actualValue": {
                                "type": "Windows",
                                "version": "6.0.18364"
                            },
                            "expectedValue": [
                                {
                                    "type": "Windows",
                                    "versions": [
                                        ">=10.0.18364 <=10.0.19041"
                                    ]
                                }
                            ],
                            "postureCheckId": "fK0aOQFD3",
                            "postureCheckName": "osCheck",
                            "postureCheckType": "OS"
                        },
                        {
                            "actualValue": "wrong.com",
                            "expectedValue": [
                                "right.com"
                            ],
                            "postureCheckId": "i2wgOQlBm",
                            "postureCheckName": "domainCheck",
                            "postureCheckType": "DOMAIN"
                        }
                    ]
                }
            ],
            "serviceId": "ll-aOqFDm",
            "serviceName": "test-service",
            "sessionType": "Dial",
            "when": "2021-03-19T09:41:10.117-04:00"
        }
    ],
    "meta": {}
}
```

# Release 0.19.6

## What's New

* Service Event Counters
* New log message which shows local (router side) address, including port, when router dials are
  successful. This allows correlating server side access logs with ziti logs

## Service Event Counters

There are new events emitted for service, with statistics for how many dials are successful, failed,
timed out, or fail for non-dial related reasons. Stats are aggregated per-minute, similar to usage
statistics. They can be enabled in the controller config as follows:

```yaml
events:
  jsonLogger:
    subscriptions:
      - type: services
    handler:
      type: file
      format: json
      path: /tmp/ziti-events.log
```

Example of the events JSON output:

```json
{
  "namespace": "service.events",
  "event_type": "service.dial.fail",
  "service_id": "HSfgHzIom",
  "count": 8,
  "interval_start_utc": 1615400160,
  "interval_length": 60
}
{
  "namespace": "service.events",
  "event_type": "service.dial.success",
  "service_id": "HSfgHzIom",
  "count": 29,
  "interval_start_utc": 1615400160,
  "interval_length": 60
}
```

Event types:

* `service.dial.success`
* `service.dial.fail`
* `service.dial.timeout`
* `service.dial.error_other`

# Release 0.19.5

## What's New

* [fabric#206](https://github.com/openziti/fabric/issues/206) Fix controller deadlock which can
  happen when links go down
* Use AtomicBitSet for xgress flags. Minimize memory use and contention
* edge router status wasn't getting set online on connect
* ziti-tunnel proxy wasn't working for services without a client config
* Add queue for metrics messages. Add config setting for metrics report interval and message queue
  size
    * metrics.reportInterval - how often to report metrics to controller. Default: `15s`
    * metrics.messageQueueSize - how many metrics message to allow to queue before blocking.
      Default: 10

Example stanza from router config file:

```yaml
metrics:
  reportInterval: 15s
  messageQueueSize: 10
```

# Release 0.19.4

## What's New

* Link latency probe timeout parameter in router configuration.
* Support configurable timeout on Xgress dial operations. Router terminated services can now specify
  a short timeout for dial operations.
* Fix 0.19 regression: updating terminator cost/precedence from the sdk was broken
* Fix 0.19 regression: fabric session client id was incorrectly set to edge session token instead of
  id
* Fix MFA secret length, lowered from 80 bytes to 80 bits
* Ensure that negative lengths in message headers are properly handled
* Fix panic when updating session activity for removed session
* Fix panic when shared router state is used when a router disconnects or reconnects
* Additional garbage collection for parallel route algorithm, removes successful routes created
  during failed attempts, after final successful attempt.

## Link Latency Probe Timeout

Control the link latency probe timeout parameter with the following router configuration syntax:

```
forwarder:
  #
  # After how many milliseconds does the link latency probe timeout?
  # (default 10000)
  #
  latencyProbeTimeout: 10000
```

## Xgress Dial Timeout

Specify the dial timeout for Xgress dialers using the following syntax:

```
dialers:
  - binding:            transport
    options:
      connectTimeout:   2s
```

You will need to specify Xgress options for each dialer binding that you want to use with your
configuration.

# Release 0.19.3

## What's New

* Metric events formatting has changed

## Metric Events Changes

Each now gets its own event. Here are two example events:

```
{
  "metric": "xgress.tx_write_time",
  "metrics": {
    "xgress.tx_write_time.count": 0,
    "xgress.tx_write_time.m1_rate": 0,
    "xgress.tx_write_time.mean": 0,
    "xgress.tx_write_time.p99": 0
  },
  "namespace": "metrics",
  "source_event_id": "62c31ab9-e0ed-48f5-9907-2d2e8c76f393",
  "source_id": "pTF3hzUQI",
  "timestamp": "2021-02-23T19:33:39.017329033Z"
}

{
  "metric": "link.rx.msgsize",
  "metrics": {
    "link.rx.msgsize.count": 3,
    "link.rx.msgsize.mean": 0,
    "link.rx.msgsize.p99": 0
  },
  "namespace": "metrics",
  "source_entity_id": "8VEJ",
  "source_event_id": "62c31ab9-e0ed-48f5-9907-2d2e8c76f393",
  "source_id": "pTF3hzUQI",
  "timestamp": "2021-02-23T19:33:39.017329033Z"
}
```

Changes of note:

1. The metric name is now listed
1. There's a new `source_event_id` which can be used to link together all the metrics that were
   reported at a given time
1. The timestamp format has been changed to match the other event times. Format is: RFC3339Nano
1. Metrics which formerlly had an id in them, such as link and control channel metrics now have the
   id extracted. The id is stored in the `source_entity_id` field.

# Release 0.19.2

## Bug fixes

* Fix edge router synchronization from stopping after workers exit
* Session validation intervals in the edge router were calculated incorrected
* Notes on the configuration values related to session validation were missing from the 0.19 release
  notes. They have been added in the section called `Edge Session Validation`

# Release 0.19.1

## Bug fixes

* Fix v0.18.x - v0.19.x API Session id incompatibility, all API Session and Sessions are deleted
  during this upgrade
* Fix Edge Router double connect leading to panics during Edge Router REST API rendering

## What's New

* Ziti CLI now has 'Let's Encrypt' PKI support to facilitate TLS connections to Controller from
  BrowZer-based apps that use the `ziti-sdk-js`.

    * New command to Register a Let's Encrypt account, then create and install a certificate

      Usage:

      `ziti pki le create -d domain -p path-to-where-data-is-saved [flags]`

      Flags:

          -a, --acmeserver string                             ACME CA hostname (default "https://acme-v02.api.letsencrypt.org/directory")
          -d, --domain string                                 Domain for which Cert is being generated (e.g. me.example.com)
          -e, --email string                                  Email used for registration and recovery contact (default "openziti@netfoundry.io")
          -h, --help                                          help for create
          -k, --keytype EC256|EC384|RSA2048|RSA4096|RSA8192   Key type to use for private keys (default RSA4096)
          -p, --path string                                   Directory to use for storing the data
          -o, --port string                                   Port to listen on for HTTP based ACME challenges (default "80")
          -s, --staging                                       Enable creation of 'staging' Certs (instead of production Certs)

    * New command to Display Let's Encrypt certificates and accounts information

      Usage:

      `ziti pki le list -p path-to-where-data-is-saved [flags]`

      Flags:

          -a, --accounts      Display Account info
          -h, --help          help for list
          -n, --names         Display Names info
          -p, --path string   Directory where data is stored

    * New command to Renew a Let's Encrypt certificate

      Usage:

      `ziti pki le renew -d domain -p path-to-where-data-is-saved [flags]`

      Flags:

          -a, --acmeserver string                             ACME CA hostname (default "https://acme-v02.api.letsencrypt.org/directory")
              --days int                                      The number of days left on a certificate to renew it (default 14)
          -d, --domain string                                 Domain for which Cert is being generated (e.g. me.example.com)
          -e, --email string                                  Email used for registration and recovery contact (default "openziti@netfoundry.io")
          -h, --help                                          help for renew
          -k, --keytype EC256|EC384|RSA2048|RSA4096|RSA8192   Key type to use for private keys (default RSA4096)
          -p, --path string                                   Directory where data is stored
          -r, --reuse-key                                     Used to indicate you want to reuse your current private key for the renewed certificate (default true)
          -s, --staging                                       Enable creation of 'staging' Certs (instead of production Certs)

    * New command to Revoke a Let's Encrypt certificate

      Usage:

      `ziti pki le revoke -d domain -p path-to-where-data-is-saved [flags]`

      Flags:

          -a, --acmeserver string   ACME CA hostname (default "https://acme-v02.api.letsencrypt.org/directory")
          -d, --domain string       Domain for which Cert is being generated (e.g. me.example.com)
          -e, --email string        Email used for registration and recovery contact (default "openziti@netfoundry.io")
          -h, --help                help for revoke
          -p, --path string         Directory where data is stored
          -s, --staging             Enable creation of 'staging' Certs (instead of production Certs)

# Release 0.19.0

## Breaking Changes

* Edge session validation is now handled at the controller, not the edge router
* Routing across the overlay is now handled in parallel, rather than serially. This changes the
  syntax and semantics of a couple of control plane messages between the controller and the
  connected routers. See the section below on `Parallel Routing` for additional details.
* API Session synchronization improvements and pluggability

## Bug fixes

* ziti ps now supports `router-disconnect` and `router-reconnect`, which disconnects/reconnects the
  router from the controller. This allows easier testing of various failure states. Requires that
  --debug-ops is passed to `ziti-router` on startup.
* Golang SDK hosted service listeners are now properly closed when they receive close notifications
* Golang SDK now recovers if the session is gone
* Golang SDK now stops some go-routines that were previously left running after the SDK context was
  closed
* Fix session leak caused by using half close when tunneling UDP connections
* Fix connection leak caused by not closing the UDP connection when it's activity timer expires

## API Changes

* Fabric Xctrl instances are now notified when the control channel reconnects
* Fabric Xctrl instances may now provide message decoders for the trace infrastructure so that
  custom messages will be properly displayed in trace logs

## Edge Session Validation

Before 0.19, edge sessions (note: network sessions, not API sessions) would be sent to edge routers
after they were created. When the edge router received a dial or bind request it would verify that
the session was valid, then request the controller to create a fabric session.

This approach has two downsides.

1. There is a race condition where the edge router may receive a dial/bind request before it has
   received the session from the controller. It thus has to wait awhile before declaring the session
   invalid.
1. Sessions need to be managed across multiple edge routers, since we don't know where the client
   will connect. This adds a lot of control channel traffic.

Since the edge router makes a request to the controller anyway, we can pass the session token and
fingerprints up to the controller and do the verification there. This allows us to minimize the
amount of state the edge router needs to keep synchronized with the controller and removes the race
condition.

When an edge controller loses connection to the controller, it needs to verify that its sessions are
still valid, in case it missed any session deletion notifications. There are three new settings
which control this behavior.

* `sessionValidateChunkSize` - how many sessions to validate in each request to the controller.
  Default value: 1000
* `sessionValidateMinInterval` - minimum time to wait between chuenks of sessions. Default
  value: `250ms`. Format: duration - examples: `10s`, `1m`, `500ms`
* `sessionValidateMaxInterval` - maximum time to wait between chuenks of sessions. Default
  value: `1500ms`. Format: duration - examples: `10s`, `1m`, `500ms`

Intervals between sending session validation chunks is random, between min and max intervals. This
is done to prevent the routers from flooding the controller after a controller restart.

## Parallel Routing

Prior to 0.19, the Ziti controller would send a `Route` message to the terminating router first, to
establish terminator endpoint connectivity. If the destination endpoint was unreachable, the entire
session setup would be abandoned. If the terminator responded successfully, the controller would
then proceed to work through the chain of routers sending `Route` messages and creating the
appropriate forwarding table entries. This all happened sequentially.

In 0.19 route setup for session creation now happens in parallel. The controller sends `Route`
commands to all of the routers in the chain (including the terminating router), and waits for
responses and/or times out those responses. If all of the participating routers respond
affirmatively within the timeout period, the entire session creation succeeds. If any participating
router responds negatively, or the timeout period occurs, the session creation attempt fails,
updating configured termination weights. Session creation will retry up to a configured number of
attempts. Each attempt will perform a fresh path selection to ensure that failed terminators can be
excluded from subsequent attempts.

### Configuration of Parallel Routing

The `terminationTimeoutSeconds` timeout parameter has been removed and will be ignored.
The `routeTimeoutSeconds` controls the timeout for each route attempt.

```
#network:
  #
  # routeTimeoutSeconds controls the number of seconds the controller will wait for a route attempt to succeed.
  #
  #routeTimeoutSeconds:  10
```

You'll want to ensure that your participating routers' `getSessionTimeout` in the Xgress options is
configured to a suitably large enough value to support the configured number of routing attempts, at
the configured routing attempt timeout. In the router configuration, the `getSessionTimeout` value
is configured for your Xgress listeners like this:

```
listeners:
  # basic ssh proxy
  - binding:            	proxy
    address:            	tcp:0.0.0.0:1122
    service:            	ssh
    options:
      getSessionTimeout:	120s
```

The new parallel routing implementation also supports a configurable number of session creation
attempts. Prior to 0.19, the number of attempts was hard-coded at 3. In 0.19, the number of retries
is controlled by the `createSessionRetries` parameter, which defaults to 3.

```
network:
  #
  # createSessionRetries controls the number of retries that will be attempted to create a circuit (and terminate it)
  # for new sessions.
  #
  createSessionRetries: 5
```

## API Session Synchronization

Prior to 0.19 API Sessions were only capable of being synchronized with connecting/reconnecting edge
routers in a single manner. In 0.19 and forward improvements allow for multiple strategies to be
defined within the same code base. Future releases will be able to introduce configurable and
negotiable strategies.

The default strategy from prior releases, now named 'instant', has been improved to fix issues that
could arise during edge router reconnects where API Sessions would become invalid on the
reconnecting edge router. In addition, the instant strategy now allows for invalid synchronization
detection, resync requests, enhanced logging, and synchronization statuses for edge routers.

### Edge Router Synchronization Status

The `GET /edge-routers` list and `GET /edge-routers/<id>` detail responses now include
a `syncStatus`
field. This value is updated during the lifetime of the edge router's connection to the controller
and will provide insight on its status.

The possible `syncStatus` values are as follows:

- "SYNC_NEW" - connection accepted but no strategy actions have been taken
- "SYNC_QUEUED" - connection handed to a strategy and waiting for processing
- "SYNC_HELLO_TIMEOUT" - sync failed due to a hello timeout, requeued for hello
- "SYNC_HELLO" - controller edge hello being sent
- "SYNC_HELLO_WAIT" - hello received from router and queued for processing
- "SYNC_RESYNC_WAIT" - router requested a resync and queued for processing
- "SYNC_IN_PROGRESS" - synchronization processing
- "SYNC_DONE" - synchronization completed, router is now in maintenance updates
- "SYNC_UNKNOWN" - state is unknown, edge router misbehaved, error state
- "SYNC_DISCONNECTED" - strategy was disconnected before finishing, error state
