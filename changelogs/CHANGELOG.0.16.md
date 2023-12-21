# Release 0.16.5

## Breaking CLI Change

* The `ziti edge controller create service` and `ziti edge controller update service` subcommands no
  longer support the `--encryption-optional` and `--encryption-required` flags. Instead, these
  subcommands now have the `--encryption=ON|OFF` flag. If this flag is not specified, the default
  is `ON`.

## What's New

* fix [edge#338 configurable IP range for DNS services](https://github.com/openziti/edge/issues/338)
  The IP range for hostname-configured services defaults to 100.64/10, and can be changed with the
  `-d` command-line option.
* fix [edge#336 Startup Race Condition](https://github.com/openziti/edge/issues/336)
* fix api-session and session deletes from ziti CLI
* update ziti-fabric-test loop3
* allow specifying policy semantic from CLI when creating policies
* new eventing features
* Posture Check Configuration API
*

fix [edge#349 Using 'edge_transport' binding on non-encrypted service leads to Dial failure](https://github.com/openziti/edge/issues/349)

## Event Changes

### Event Configuration

Handlers can now be configured for events via the config file. Here is an example configuration:

    events:
        jsonLogger:
            subscriptions:
                - type: metrics
                  sourceFilter: .*
                  metricFilter: .*egress.*m1_rate*
                - type: fabric.sessions
                  include:
                    - created
                - type: edge.sessions
                  include:
                    - created
                - type: fabric.usage
            handler:
                type: file
                format: json
                path: /tmp/ziti-events.log

Each section can include any number of event subscriptions and a single handler. The supported event
types are:

* metrics
* fabric.sessions
* fabric.usage
* edge.sessions

### Event Handlers

There are two new handlers which can be used to output events.

#### File Handler

Sends events to disk. Supported options:

* type: file
* format: json|plain
* path: Path to the target file
* bufferSize: size of event queue. When this fills up, processes submitting events will block
* maxsizemb: max size of the file before it gets rolled. default is 10MB
* maxbackups: max number of rolling files to keep. default is 0 (keeps all)

#### Stdout Handler

Sends events to stdout. Supported options:

* type: stdout
* format: json|plain
* bufferSize: size of event queue. When this fills up, processes submitting events will block

### Usage Events

There is a new Usage event type which has been derived from the metrics events. This allows handlers
to see discrete usage events, rather than a collection of them.

The Usage event looks like:

    type UsageEvent struct {
        Namespace        string `json:"namespace"`
        EventType        string `json:"event_type"`
        SourceId         string `json:"source_id"`
        SessionId        string `json:"session_id"`
        Usage            uint64 `json:"usage"`
        IntervalStartUTC int64  `json:"interval_start_utc"`
        IntervalLength   uint64 `json:"interval_length"`
    }

### Metrics Events

Metrics events can now be filtered. Metrics events processed through the new event framework no
longer sending metrics messages directly. Rather, a flattened (and more easily filterable) event
type is provided. The new event type looks like:

    type MetricsEvent struct {
        Namespace    string
        SourceId     string
        Timestamp    *timestamp.Timestamp
        Tags         map[string]string
        IntMetrics   map[string]int64
        FloatMetrics map[string]float64
        MetricGroup  map[string]string
    }

## Posture Check Configuration API

Edge API endpoints have been added to configure posture checks. Posture checks not currently
enforced. However it is possible for integrations to being developed against the API.

This section contains an overview of the new endpoints. See the OpenAPI 2.0 API definition (
swagger.yml) for complete details.

### New Endpoints

- `GET /posture-checks` - retrieve a list of existing Posture Checks
- `POST /posture-checks` - create a new Posture Check
- `PUT/PATCH /posture-checks/<id>` - update an existing Posture Check
- `DELETE /posture-checks/<id>` - delete an existing Posture Check
- `GET /posture-check-types` - retrieve a list of existing Posture Check Types
- `GET /posture-check-types/<id>` - retrieve a an existing Posture Check Type
- `GET /identities/<id>/posture-data` - retrieve the Posture Data for a specific identity

### Modified Endpoints

- `GET/POST /service-policies` - now accepts/returns field
  `postureCheckRoles` and `postureCheckRolesDisplay`
- `PUT/PATCH /service-policies/<id>` - now accepts `postureCheckRoles`

# Release 0.16.4

## Breaking CLI Change

* The `ziti edge enroll` and `ziti-tunnel enroll` subcommands no longer require a --jwt argument.
  Instead the JWT can be supplied as the first argument. So `ziti edge enroll --jwt /path/to/my.jwt`
  would become `ziti edge enroll /path/to/my.jwt`. For now the --jwt flag is still accepted as well.

## Deprecations

* The `ziti-enroller` command is deprecated and will be removed in a future release. It will now
  display a DEPRECATION warning when it is run

## What's New

* [ziti#192 CAs default to 10yr expiration](https://github.com/openziti/ziti/issues/192)
* Allow specifying edge config file when using ziti-fabric-test loop2
* Add grouping data to streaming metrics, so values can be associated with their source metric
* New WSS underlay to support Edge Router connections from Browser-based webapps using the
  ziti-sdk-js
* [ziti#151 enroll subcommand w/out args should print help](https://github.com/openziti/ziti/issues/192)
* Fix processing of `--configTypes all` in `ziti edge list services`
* Addressable Terminators and the eXtensible Terminator Validation framework
* GO Edge SDK now respects Service.EncryptionRequired setting
* [fabric#133 Add Version Information To Hellos](https://github.com/openziti/fabric/issues/133)
* [edge#326 Nested Transaction Deadlock](https://github.com/openziti/edge/issues/326)

# Config File Changes

The following should be added to controller configuration files when using the Edge components:

    terminator:
      validators:
        edge: edge

This config stanza enables validating edge terminators. If this stanza is not added, everything will
continue to work, but different Edge identities will be allowed to bind to the same terminator
identity, which is generally not a valid state.

# SDK API changes

The `Context.ListenWithOptions` method now takes a `ListenOptions` which are defined in the `ziti`
package, instead of the `edge` package.

There's a new `Context.DialWithOptions` method which takes a `DialOptions` struct.

## Addressable Terminators and Terminator Validation

This release contains two new features related to terminators. The first allows you to connect to a
subset of terminators for a service. The second allows developers to plugin validation for
terminators with different validation logic per binding type.

### Addressable Terminators

Terminators define how network traffic for Ziti services exits the fabric and makes its way to the
application providing/hosting the service. Each terminator for a service specifies the following:

1. The router at which traffic terminates
2. The binding, which specifies the Xgress component responsible for providing the connection to the
   hosting application
3. The address, which the Xgress component can use to create or lookup the connection

There are currently two kinds of termination. Router terminated services make outbound connections
to the hosting applications. SDK hosted servers make inbound connections from the SDK to the router.

Now that Ziti supports multiple terminators we may want to be able to connect to a specific hosting
application. This can be used to allow a service to cover many endpoints, each of which can be
connected to individually. Some common use cases for this might be a peer-to-peer application, like
a voip service, or a service like SSH covering multiple machines.

What we don't want in these cases is to have to create a new service for each voip client or each
new machine that we want to ssh to. We also want to make sure that if an application is making
multiple connections (and thus multiple terminators) for redundancy or load balancing purposes, that
we can address the application, rather than an individual terminator.

To this end, terminators now have two new fields:

1. Identity - defines the name by which a terminator can be addressed (along with other terminators
   using the same identity)
1. IdentitySecret - allows verifying that terminators using the same identity are from the same
   application.

Notes

1. Identity here may be related to the concept of the Edge Identity, but is not required to be.
2. How IdentitySecret is used to validate a terminator is up to the terminator validator for the
   binding. The edge has a terminator validator which uses the client's certs to ensure that all
   terminators for a given terminator identity belong to the same edge identity.

The identity allows the service to be addressed with that identity.

This can now be used in the fabric by prefixing the service name with the identity, separated by
the `@` symbol. For example, if a service `ssh` has a terminator with identity `web-server`, it
could be dialed using `web-server@ssh`.

The Edge SDK also supports dialing and binding using identity addressing. The `Context` type now
supports a new `DialWithOptions` method, which can be used to specify an identity.

    dialOptions := &ziti.DialOptions{
        Identity:       "555-555-5555",
        ConnectTimeout: 1 * time.Minute,
    }
    conn, err := app.context.DialWithOptions(app.service, dialOptions)

`Context` already has a ListenWithOptions, and the `ListenOptions` now support providing an
Identity. Users may also set a flag to automatically set the Identity to the edge identity's name.
If an identity is provided, the IdentitySecret will automatically be calculated and passed in as
well.

    host.listener, err = host.context.ListenWithOptions(service.Name, &ziti.ListenOptions{
        Identity: "555-555-5555",
    })

or

    host.listener, err = host.context.ListenWithOptions(service.Name, &ziti.ListenOptions{
        BindUsingEdgeIdentity: true,
    })

For non-Edge SDK terminators the identity can be provided on the command line when creating the
terminator.

#### Example

There is a simple application in the sdk-golang repository `example/call` which shows how identity
addressing can be used to implement something like a VoIP service.

### xvt (eXtensible Termrinator Validation)

The fabric now supports pluggable validators for terminators. Validators implement the following
interface:

    type Validator interface {
        Validate(tx *bbolt.Tx, terminator Terminator, create bool) error
    }

Validators can be registered with the controller on startup. For example, here is how the edge
terminator validator is registered:

	xtv.RegisterValidator("edge", env.NewEdgeTerminatorValidator(c.AppEnv))

Validators can then be tied to a binding in the config file:

    terminator:
      validators:
        edge: edge

In the example above, terminators with the binding `edge` will use the validator which was
registered under the name `edge`. The binding is the key and the validator name is the value.

# Release 0.16.2

* What's New
    * Smart routing fixes
        * [Persist Terminator Precedence](https://github.com/openziti/fabric/issues/112)
        * [Terminators and Fixed Link Cost, Incorrect Path Selection](https://github.com/openziti/fabric/issues/121)
        * [If link vanishes during reroute, controller can panic](https://github.com/openziti/fabric/issues/122)
        * [Scale latency metric when used for path costing](https://github.com/openziti/fabric/issues/123)
        * [Services not always getting cleared from cache when terminators are updated](https://github.com/openziti/fabric/issues/124)
        * [Fix service policy denormalization migration](https://github.com/openziti/edge/issues/291)
    * [sdk-golang#84](https://github.com/openziti/sdk-golang/issues/84) Fixes go routine leak that
      would slowly kill SDK application (i.e. ziti-probe)
    * REST API doc via ReDoc available at `https://<host>:<port>/docs`

# Release 0.16.1

* What's New
    * Metrics Refactoring
        * [Support timers in metrics events](https://github.com/openziti/foundation/issues/121)
        * [Convert json file reporter to generic reporter supporting multiple formatters, including json and plain](https://github.com/openziti/foundation/issues/122)
    * Session Performance Fixes
        * [Supported unindexed FK constraints in bbolt](https://github.com/openziti/foundation/issues/119)
        * [Improve API Session and Session creation performance](https://github.com/openziti/edge/issues/281)
    * [Make enrollment available from the ziti CLI](https://github.com/openziti/ziti/issues/182)
    * Docker image for `ziti-tunnel` - Embellish examples and fix entrypoint script to wait for
      clean up of iptables rules on exit
    * Various Internal Stability & Scale Fixes
        * Edge Controller:
            * Use bbolt batch operations where possible (heartbeat updates, sdk/env info)
            * Stream API Sessions & Session during Edge Router Sync
            * Removal of `Panic()` calls
        * Edge Router:
            * Support heartbeat interval configuration, default raised from 5s to 60s
    * Ziti-Probe
        * Attempts to retain and reuse API Sessions
        * Attempts to reconnect on disconnection, API Session removal, session removal
        * Improve reconnection strategy
        * Adds `version` command to `ziti-probe`
    * Go SDK
        * Removal of `Fatal()` call
        * Add ability to detect invalid sessions

# Release 0.16.0

## Overview:

**Important Note:** This release contains backwards incompatible changes. See below for details.

* End-To-End Encryption Enhancements
    * [e2e Service Configuration & Router Termination](https://github.com/openziti/edge/issues/173)
* Router Scaling Issues
    * [Add worker pools for link and xgress dials](https://github.com/openziti/fabric/issues/109)
* Model Performance Improvements
    * [Denormalize policy links for performance](https://github.com/openziti/edge/issues/256)
* Datastore Integrity Checker
    * [foundation#107](https://github.com/openziti/foundation/issues/107)
    * [edge#258](https://github.com/openziti/edge/issues/258)
    * [#163](https://github.com/openziti/ziti/issues/163)
* Events Framework
    * [foundation#116](https://github.com/openziti/foundation/issues/116) - Add generic event
      framework and use it for metrics
    * [fabric#106](https://github.com/openziti/fabric/issues/106) - Event Streaming
    * [edge#229](https://github.com/openziti/edge/issues/229) - Stream Session Events

* Bug Fixes:
    * [#152](https://github.com/openziti/ziti/issues/152) - Fix ziti-router enroll exit code on
      failure
    * [#156](https://github.com/openziti/ziti/issues/156) - Fix display of policies with empty roles
      lists
    * [#169](https://github.com/openziti/ziti/issues/169) - Fix delete by ID in Ziti CLI
    * [edge#269](https://github.com/openziti/edge/issues/269) - Service Policy types in the bbolt
      should be 1 and 2, rather than 4 and 5
    * [edge#273](https://github.com/openziti/edge/issues/273) - Avoid tun "not pollable" read
      failures
    * [fabric#114](https://github.com/openziti/fabric/issues/114) - When egress connect fails,
      router does not return failure to controller
    * [fabric#117](https://github.com/openziti/fabric/issues/117) - Xgress setup has a race
      condition

* Backwards Compatibility
    * The `ziti edge snapshot-db` command is now `ziti edge db snapshot`
    * In order to fix [fabric#117](https://github.com/openziti/fabric/issues/117), the xgress
      protocol flow had to be updated. This means 0.16+ controllers and routers will not work with
      older controllers and routers

## End-To-End Encryption Enhancements

### E2E Encryption Router Termination

A new xgress module has been added specifically for handling Ziti Edge e2e to handle SDK to Router
Termination scenarios. Previously, only SDK-to-SDK end-to-end encryption was supported. When e2e
encryption is desired for a router terminated service, use the bind value `edge_transport` when
defining the terminator for the service. This value is now the default when using the CLI to create
a terminator. If the `binding` value is omitted when using the REST API directly, it will default
to `transport` - which does not support e2e encryption.

##### CLI Example (explicit binding):

```
ziti edge create terminator mytcpservice 002 tcp:my-tcp-service.com:12345 --binding edge_transport
```

##### Edge Rest API Example:

```
POST /terminators
{
    "service": "ZbX9",
    "router": "002",
    "binding": "edge_transport",
    "address": "tcp:my-tcp-service.com:12345"
}
```

### End-to-End Encryption Service Configuration

Edge Services can now be set to require e2e encryption. All Edge Services defined before this
version will default to requiring e2e encryption. Existing services will need to have their
terminators updated to use `edge_transport` or update the service to not require e2e encryption.

##### Create Service Example (encryption required)

```
POST /services
{
    "name": "my-service",
    "encryptionRequired": true
}
```

##### Patch Service Example (encryption required)

Can also be set via PUT.

```
PATCH /services/<id>
{
    "encryptionRequired": true
}
```

#### Create Service CLI (encryption required)

End-to-end encryption defaults to required, no flag needed.

```
ziti edge create service myservice
```

#### Create Service CLI (encryption optional)

```
ziti edge create service myservice -o
```

#### Update Service CLI (set encryption required)

```
ziti edge update service myservice -e
```

#### Update Service CLI (set encryption optional)

```
ziti edge update service myservice -o
```

## Router Scaling Issues

When scaling Ziti Routers it was possible that numerous requests to complete xgress routes or
establish links between routers could block the control plane of a router. This could cause timeouts
of other control messages and delay the establishment of new service routes and links. This would be
especially noticeable when starting multiple routers at the same time or when a Ziti Controller was
restarted with multiple routers already connected.

To alleviate control channel congestion, worker queues and pools have been added to xgress and link
dial processing. New options are exposed in the `forwarder` section of router configuration files to
control the queue and worker pool.

The new settings are:

* `xgressDialQueueLength`
* `xgressDialWorkerCount`
* `linkDialQueueLength`
* `linkDialWorkerCount`

...and are explained in the following example.

##### Example Router Configuration Section:

```
forwarder:
  # How frequently does the forwarder probe the link latency. This will ultimately determine the resolution of the
  # responsiveness available to smart routing. This resolution comes at the expense of bandwidth utilization for the
  # probes, control plane utilization, and CPU utilization processing the results.
  #
  latencyProbeInterval: 1000
  # How many xgress dials can be queued for processing by the xgress dial workers. An xgress dial occurs
  # for services that have a terminator egress specified with an xgress binding (e.g. transport)
  # (minimum 1, max 10000, default 1000)
  xgressDialQueueLength: 1000
  # The number of xgress dial workers used to process the xgress dial queue.
  # (minimum 1, max 10000, default 10)
  xgressDialWorkerCount: 10
  # How many link dials can be queued for processing by the link dial workers. An link dial occurs
  # when a router is notified of a new router by the controller.
  # (minimum 1, max 10000, default 1000)
  linkDialQueueLength: 1000
  # The number of link dial workers used to process the link dial queue.
  # (minimum 1, max 10000, default 10)
  linkDialWorkerCount: 10
```

## Model Performance Improvements

Policy relationships are now stored in a denormalized fashion. This means that checking if an entity
is tied to another entity via a policy is now a direct lookup, and much faster. This means that the
Ziti controller should scale very well in cases where we have many identities, services and/or edge
routers. Performance was tested against the APIs used by the SDKs.

See for more detail:

* [Denormalized Policies](https://github.com/openziti/edge/wiki/Denormalized-Policies)
* [Characterization (Pure Model Tests)](https://github.com/openziti/ziti/wiki/Characterization#pure-model-tests)

## Data Integrity Checking Framework

The bbolt datastore used by Ziti provides simple key/value storage with nesting. Ziti has
implemented some basic relational constructs on top of bbolt, such as indexed values, foreign key
indexes, many to many collections and reference counted collections (for policy denormalization).
This release adds a data integrity checking framework which allows us to verify that constraint
assumptions are valid, and allows fixing issues when they are found (if possible). This work was
done in part to validate that the policy denormalization code is working correctly, and to provide a
rememdy if issues are found.

There are two new REST APIs available

* GET `/database/check-data-integrity`
    * https://github.com/openziti/edge/blob/master/specs/swagger.yml#L2916
* POST `/database/fix-data-integrity`
    * https://github.com/openziti/edge/blob/master/specs/swagger.yml#L2930

These APIs can be used from the ziti CLI.

* `ziti edge db check-integrity` - to report on data integrity issues
* `ziti edge db check-integrity -f` - to report on data integrity issues and attempt to fix any that
  are found

## Events Framework

Ziti now has a shared events framework used across projects. Events are used internally and can be
used by users to write components which plug into Ziti and react to events (or make them available
externally).

Each project which exposes events will have a top-level events package where you can find
registration hooks for all exposed events in the project

### Current Event Types

* foundation
    * metrics events
* fabric
    * fabric session events (session created, session deleted, session path changed)
    * trace events
* edge
    * **NEW** edge session events (session created, session deleted)
        * the session event created includes sessionId, session token, API sessionId and identity id
        * the session deleted event includes sessionId and session token

NOTE: The clientId on fabric session events is the edge session token for fabric sessions created
from the edge

