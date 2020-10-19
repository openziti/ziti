# Release 0.16.5

## Breaking CLI Change
  * The `ziti edge controller create service` and `ziti edge controller update service` subcommands no longer support the `--encryption-optional` and `--encryption-required` flags. Instead, these subcommands now have the `--encryption=ON|OFF` flag.  If this flag is not specified, the default is `ON`.

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
  * fix [edge#349 Using 'edge_transport' binding on non-encrypted service leads to Dial failure](https://github.com/openziti/edge/issues/349)

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

Each section can include any number of event subscriptions and a single handler. The supported event types are:

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
There is a new Usage event type which has been derived from the metrics events. This allows handlers to see discrete usage events, rather than a collection of them.

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
Metrics events can now be filtered. Metrics events processed through the new event framework no longer sending metrics messages directly. Rather, a flattened (and more easily filterable) event type is provided. The new event type looks like:

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

Edge API endpoints have been added to configure posture checks. Posture
checks not currently enforced. However it is possible for integrations
to being developed against the API.

This section contains an overview of the new endpoints. See the OpenAPI
2.0 API definition (swagger.yml) for complete details.

### New Endpoints

- `GET /posture-checks` - retrieve a list of existing Posture Checks
- `POST /posture-checks` - create a new Posture Check
- `PUT/PATCH /posture-checks/<id>` - update an existing Posture Check
- `DELETE /posture-checks/<id>` - delete an existing Posture Check
- `GET /posture-check-types` - retrieve a list of existing Posture Check
  Types
- `GET /posture-check-types/<id>` - retrieve a an existing Posture Check
  Type
- `GET /identities/<id>/posture-data` - retrieve the Posture Data for a
  specific identity

### Modified Endpoints

- `GET/POST /service-policies` - now accepts/returns field
  `postureCheckRoles` and `postureCheckRolesDisplay`
- `PUT/PATCH /service-policies/<id>` - now accepts `postureCheckRoles`

# Release 0.16.4

## Breaking CLI Change
  * The `ziti edge enroll` and `ziti-tunnel enroll` subcommands no longer require a --jwt argument. Instead the JWT can be supplied as the first argument. So `ziti edge enroll --jwt /path/to/my.jwt` would become `ziti edge enroll /path/to/my.jwt`. For now the --jwt flag is still accepted as well.

## Deprecations
  * The `ziti-enroller` command is deprecated and will be removed in a future release. It will now display a DEPRECATION warning when it is run

## What's New
  * [ziti#192 CAs default to 10yr expiration](https://github.com/openziti/ziti/issues/192)
  * Allow specifying edge config file when using ziti-fabric-test loop2
  * Add grouping data to streaming metrics, so values can be associated with their source metric
  * New WSS underlay to support Edge Router connections from Browser-based webapps using the ziti-sdk-js
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

This config stanza enables validating edge terminators. If this stanza is not added, everything will continue to work, but different Edge identities will be allowed to bind to the same terminator identity, which is generally not a valid state.

# SDK API changes
The `Context.ListenWithOptions` method now takes a `ListenOptions` which are defined in the `ziti` package, instead of the `edge` package.

There's a new `Context.DialWithOptions` method which takes a `DialOptions` struct.

## Addressable Terminators and Terminator Validation
This release contains two new features related to terminators. The first allows you to connect to a subset of terminators for a service. The second allows developers to plugin validation for terminators with different validation logic per binding type.

### Addressable Terminators
Terminators define how network traffic for Ziti services exits the fabric and makes its way to the application providing/hosting the service. Each terminator for a service specifies the following:

 1. The router at which traffic terminates
 2. The binding, which specifies the Xgress component responsible for providing the connection to the hosting application
 3. The address, which the Xgress component can use to create or lookup the connection

 There are currently two kinds of termination. Router terminated services make outbound connections to the hosting applications. SDK hosted servers make inbound connections from the SDK to the router.

Now that Ziti supports multiple terminators we may want to be able to connect to a specific hosting application. This can be used to allow a service to cover many endpoints, each of which can be connected to individually. Some common use cases for this might be a peer-to-peer application, like a voip service, or a service like SSH covering multiple machines.

What we don't want in these cases is to have to create a new service for each voip client or each new machine that we want to ssh to.
We also want to make sure that if an application is making multiple connections (and thus multiple terminators) for redundancy or load balancing purposes, that we can address the application, rather than an individual terminator.

To this end, terminators now have two new fields:

1. Identity - defines the name by which a terminator can be addressed (along with other terminators using the same identity)
1. IdentitySecret - allows verifying that terminators using the same identity are from the same application.

Notes

1. Identity here may be related to the concept of the Edge Identity, but is not required to be.
2. How IdentitySecret is used to validate a terminator is up to the terminator valiator for the binding. The edge has a terminator validator which uses the client's certs to ensure that all terminators for a given terminator identity belong to the same edge identity.

The identity allows the service to be addressed with that identity.

This can now be used in the fabric by prefixing the service name with the identity, separated by the `@` symbol. For example, if a service `ssh` has a terminator with identity `web-server`, it could be dialed using `web-server@ssh`.

The Edge SDK also supports dialing and binding using identity addressing. The `Context` type now supports a new `DialWithOptions` method, which can be used to specify an identity.

    dialOptions := &ziti.DialOptions{
        Identity:       "555-555-5555",
        ConnectTimeout: 1 * time.Minute,
    }
    conn, err := app.context.DialWithOptions(app.service, dialOptions)

`Context` already has a ListenWithOptions, and the `ListenOptions` now support providing an Identity. Users may also set a flag to automatically set the Identity to the edge identity's name. If an identity is provided, the IdentitySecret will automatically be calculated and passed in as well.

    host.listener, err = host.context.ListenWithOptions(service.Name, &ziti.ListenOptions{
        Identity: "555-555-5555",
    })

or

    host.listener, err = host.context.ListenWithOptions(service.Name, &ziti.ListenOptions{
        BindUsingEdgeIdentity: true,
    })

For non-Edge SDK terminators the identity can be provided on the command line when creating the terminator.

#### Example
There is a simple application in the sdk-golang repository `example/call` which shows how identity addressing can be used to implement something like a VoIP service.

### xvt (eXtensible Termrinator Validation)
The fabric now supports pluggable validators for terminators. Validators implement the following interface:

    type Validator interface {
        Validate(tx *bbolt.Tx, terminator Terminator, create bool) error
    }

Validators can be registered with the controller on startup. For example, here is how the edge terminator validator is registered:

	xtv.RegisterValidator("edge", env.NewEdgeTerminatorValidator(c.AppEnv))

Validators can then be tied to a binding in the config file:

    terminator:
      validators:
        edge: edge

In the example above, terminators with the binding `edge` will use the validator which was registered under the name `edge`. The binding is the key and the validator name is the value.

# Release 0.16.2

* What's New
  * Smart routing fixes
      * [Persist Terminator Precedence](https://github.com/openziti/fabric/issues/112)
      * [Terminators and Fixed Link Cost, Incorrect Path Selection](https://github.com/openziti/fabric/issues/121)
      * [If link vanishes during reroute, controller can panic](https://github.com/openziti/fabric/issues/122)
      * [Scale latency metric when used for path costing](https://github.com/openziti/fabric/issues/123)
      * [Services not always getting cleared from cache when terminators are updated](https://github.com/openziti/fabric/issues/124)
    * [Fix service policy denormalization migration](https://github.com/openziti/edge/issues/291)
  * [sdk-golang#84](https://github.com/openziti/sdk-golang/issues/84) Fixes go routine leak that would slowly kill SDK application (i.e. ziti-probe)
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
  * Docker image for `ziti-tunnel` - Embellish examples and fix entrypoint script to wait for clean up of iptables rules on exit
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
  * [foundation#116](https://github.com/openziti/foundation/issues/116) - Add generic event framework and use it for metrics
  * [fabric#106](https://github.com/openziti/fabric/issues/106) - Event Streaming
  * [edge#229](https://github.com/openziti/edge/issues/229) - Stream Session Events

* Bug Fixes:
  * [#152](https://github.com/openziti/ziti/issues/152) - Fix ziti-router enroll exit code on failure
  * [#156](https://github.com/openziti/ziti/issues/156) - Fix display of policies with empty roles lists
  * [#169](https://github.com/openziti/ziti/issues/169) - Fix delete by ID in Ziti CLI
  * [edge#269](https://github.com/openziti/edge/issues/269) - Service Policy types in the bbolt should be 1 and 2, rather than 4 and 5
  * [edge#273](https://github.com/openziti/edge/issues/273) - Avoid tun "not pollable" read failures
  * [fabric#114](https://github.com/openziti/fabric/issues/114) - When egress connect fails, router does not return failure to controller
  * [fabric#117](https://github.com/openziti/fabric/issues/117) - Xgress setup has a race condition

* Backwards Compatibility
  * The `ziti edge snapshot-db` command is now `ziti edge db snapshot`
  * In order to fix [fabric#117](https://github.com/openziti/fabric/issues/117), the xgress protocol flow had to be updated. This means 0.16+ controllers and routers will not work with older controllers and routers

## End-To-End Encryption Enhancements
### E2E Encryption Router Termination
A new xgress module has been added specifically for handling Ziti Edge e2e to handle SDK to Router Termination scenarios.
Previously, only SDK-to-SDK end-to-end encryption was supported. When e2e encryption is desired
for a router terminated service, use the bind value `edge_transport` when defining
the terminator for the service. This value is now the default when using the CLI to create
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

Edge Services can now be set to require e2e encryption. All Edge
Services defined before this version will default to requiring e2e
encryption. Existing services will need to have their terminators
updated to use `edge_transport` or update the service to not
require e2e encryption.

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

When scaling Ziti Routers it was possible that numerous requests to
complete xgress routes or establish links between routers could block
the control plane of a router. This could cause timeouts of other
control messages and delay the establishment of new service routes and
links. This would be especially noticeable when starting multiple
routers at the same time or when a Ziti Controller was restarted with
multiple routers already connected.

To alleviate control channel congestion, worker queues and pools
have been added to xgress and link dial processing. New options are
exposed in the `forwarder` section of router configuration files to
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
Policy relationships are now stored in a denormalized fashion. This means that checking if an entity is tied to another entity via a policy is now a direct lookup, and much faster. This means that the Ziti controller should scale very well in cases where we have many identities, services and/or edge routers. Performance was tested against the APIs used by the SDKs.

See for more detail:

* [Denormalized Policies](https://github.com/openziti/edge/wiki/Denormalized-Policies)
* [Characterization (Pure Model Tests)](https://github.com/openziti/ziti/wiki/Characterization#pure-model-tests)

## Data Integrity Checking Framework
The bbolt datastore used by Ziti provides simple key/value storage with nesting. Ziti has implemented some basic relational constructs on top of bbolt, such as indexed values, foreign key indexes, many to many collections and reference counted collections (for policy denormalization). This release adds a data integrity checking framework which allows us to verify that constraint assumptions are valid, and allows fixing issues when they are found (if possible). This work was done in part to validate that the policy denormalization code is working correctly, and to provide a rememdy if issues are found.

There are two new REST APIs available

* GET `/database/check-data-integrity`
    * https://github.com/openziti/edge/blob/master/specs/swagger.yml#L2916
* POST `/database/fix-data-integrity`
    * https://github.com/openziti/edge/blob/master/specs/swagger.yml#L2930

These APIs can be used from the ziti CLI.

* `ziti edge db check-integrity` - to report on data integrity issues
* `ziti edge db check-integrity -f` - to report on data integrity issues and attempt to fix any that are found

## Events Framework
Ziti now has a shared events framework used across projects. Events are used internally and can be used by users to write components which plug into Ziti and react to events (or make them available externally).

Each project which exposes events will have a top-level events package where you can find registration hooks for all exposed events in the project

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

NOTE: The clientId on fabric session events is the edge session token for fabric sessions created from the edge

# Release 0.15.3

* What's New:
  * Add example docker compose for ziti-tunnel

# Release 0.15.2

* What's New:
  * [#140](https://github.com/openziti/ziti/issues/140) - Allow logging JSON request for Ziti CLI
  * [#148](https://github.com/openziti/ziti/issues/148) - Show isOnline in ziti edge list edge-routers
  * [#144](https://github.com/openziti/ziti/issues/144) - Allow ziti-fabric list to use queries. Default to `true limit none`

* Bug Fixes:
  * [#142](https://github.com/openziti/ziti/issues/142) - fix CLI ca create not defaulting identity roles
  * [#146](https://github.com/openziti/ziti/issues/146) - Export edge router JWT fails sometimes when there are more than 10 edge routers
  * [#147](https://github.com/openziti/ziti/issues/147) - Fix paging output when using 'limit none'
  * [edge#243](https://github.com/openziti/edge/issue/243) - Session creation only returns 10 edge routers
  * [edge#245](https://github.com/openziti/edge/issue/245) - fingerprint calculation changed from 0.14 to 0.15. Ensure 0.15 routers can work with 0.14 controllers
  * [edge#248](https://github.com/openziti/edge/issue/248) - Edge Router Hello can time out on slow networks with many links to establish
  * [foundation#103](https://github.com/openziti/foundation/issues/103) - Fix config file env injection for lists

# Release 0.15.1

* What's New:
No new functionality introduced.

* Bug fixes
  * [#129](https://github.com/openziti/ziti/issues/129) - minor issue with `ziti-tunnel enroll` outputting the success message at ERROR level
  * [#131](https://github.com/openziti/ziti/issues/131) - issues w/ creating identities, CAs and validating CAs
  * [#133](https://github.com/openziti/ziti/issues/133) - fix service lookup by name when creating service edge router policies
  * [edge#191](https://github.com/openziti/edge/issues/191) - updating self password via CLI would error with 404 not found
  * [edge#231](https://github.com/openziti/edge/issues/231) - identities missing enrollment expiresAt property
  * [edge#237](https://github.com/openziti/edge/issues/237) - Policy Advisor CLI is failing because common routers IsOnline value is missing
  * [edge#233](https://github.com/openziti/edge/issues/233) - REST API Errors should be application/json if possible
  * [edge#240](https://github.com/openziti/edge/issues/240) - listing specs results in a 404

# Release 0.15.0
Ziti 0.15.0 includes the following:

* The ability to invoke a database snapshot/backup
  * [Create fabric mgmt API to request database snapshot/backup be created](https://github.com/openziti/fabric/issues/99)
  * [Add snapshot db REST API](https://github.com/openziti/edge/issues/206)
* Removal of deprecated code/migrations
  * [Remove postgres store code including migrations](https://github.com/openziti/edge/issues/195)
  * Remove deprecated AppWan and Clusters - These have been replaced by service policies and service edge router policies respectively
* Edge Routers are now a subtype of Fabric Routers
  * see [Unverified Edge Routers Cannot Be Used For Terminators](https://github.com/openziti/edge/issues/144)
* Fabric services and routers now have names
  * see [Add name to service and router](https://github.com/openziti/fabric/issues/101)
* cosmetic changes to the ziti-enroller binary
* cosmetic changes to the ziti-tunnel binary when running the enroll subcommand
* Memory leak remediation in the `PayloadBuffer` subsystem. Corrects unbounded memory growth in `ziti-router`.
* Edge REST API Enhancements
  * [OpenApi 2.0/Swagger](https://github.com/openziti/edge/issues/108)
  * [Changes to support Fabric REST API](https://github.com/openziti/edge/issues/101)

## Removal of deprecated code
The code to migrate a Ziti instance from pre-0.9 releases has been removed. If you want to migrate from a pre-0.9 version you should first update to 0.14.12, then to new versions.

## Database Snapshots
Database snapshots can now be triggered in a variety of ways to cause the creation of a dabase backup/snapshot. This can be done from the ziti-fabric CLI, the ziti CLI and the REST API

    $ ziti-fabric snapshot-db
    $ ziti edge snapshot-db

The REST API is available by POSTing to `/edge/v1/database/snapshot`. This ability is only available to administrators.

The snapshot will be a copy of the database file created within a bolt transaction. The file name will have the data and time appended to it. Snapshotting can be done at most once per minute.

## Edge Routers/Fabric Router subtyping
Previously edge routers and fabric routers were closely related, but weren't actually the same entity. When an edge router was created, there was no corresponding fabric router until the edge router had been succesfully enrolled.

Now, edge routers are a type of fabric router. When an edge router is created, it will be visible as a fabric router with no fingerprint. This means that the corresponding router application won't be able to connect until enrollment is complete.

This simplifies some things, including allowing adding terminators to an edge router before enrollment is complete.

## Fabric Router and Service Names
Previously fabric routers and services only had ids, which were assumed to be something user friendly. Now they also have a name. If no name is provided, the id will be used for the name as well. This was done primarily so that we have consistency between the fabric and the edge. Now when viewing a service or router you can be sure to find the label in the same place.

### Edge REST API Enhancements

The v0.15.0 brings in new Edge REST API changes that are being made in
preparation for future enhancements. Please read these changes carefully
and adopt new patterns to avoid future incompatibility issues.

#### OpenApi 2.0/Swagger

The REST presentation of the Edge REST API is now fully generated from
the Open API 2.0/Swagger specification in `edge/spec`. The the generated
code is in `edge/rest_model`, `edge/rest_server`, and
`edge_rest_client`. The code is generated by installing `go-swagger`,
currently at version 0.24.0.

The generated code introduces a few changes that may impact clients:

*  `content-type` and `accept` headers are now meaningful and important
*  enrollment endpoints can return JSON if JSON is explicitly set in
`accept` headers
*  API input validation errors
*  various entity ref bugs
*  standardization of id properties

#### Content Type / Accept Headers

For `content-type` and `accept` headers, if `accept` is not being set,
clients usually send an `accept` of `*/*` - accepting anything. If so,
the Edge REST API will continue to return `content-type`s that are the
same as previous versions. However, non-JSON responses from enrollment
endpoints are now deprecated.

If a client is setting the `accept` header to anything other than
`application/json` for most endpoints, the API will return errors
stating the that the content types are not acceptable.

#### API Input Validation

API input validation is now handled by the Open API libraries and
go-swagger generated code. The error formats returned are largely the
same. However validations errors now all return the same outer
error and set the cause error properly. Prior to this change
errors were handled in an inconsistent manner.

#### Entity Ref Bugs

Various entity references were fixed where URLs were pointing to the
wrong or invalid API URLs.

#### Id Properties

Id properties are now fully typed: <type>Id, in API request/responses.

Entities affected:
* config
  * `type` to `configTypeid`
* identity service config
  * `service` to `serviceId`
  * `config` to `configId`

`IdentityTypeId` references were not updated as they are slated for
removal and are now deprecated. This includes `/identity-type` and
associated `/identity` properties for create/update/patch operations.

### Changes to support Fabric REST API

The following changes were done to support the future Fabric REST API

* Edge REST API base moved to `/edge/v1`
* `apiVersions` was introduced to `GET /versions`
* move away from UUID formats for ids to shortIds

#### Base Path

The Edge REST API now has a base path of `edge/v1`. The previous base
path, `/`, is now deprecated but remains active till a later release.
This move is to create room for the Fabric REST API to take over the
root path and allow other components to register APIs.

#### API Versions

For now the `GET /versions` functionality is handled by the Edge REST
API but will be subsumed by a future Fabric REST API.

The `GET /versions` now reports version information in a map structure
such that future REST APIs, as they are introduced, can register
supported versions. It is the goal of the Ziti REST APIs to support
multiple API versions for backwards comparability.

Example `GET /versions` response:

```
{
    "data": {
        "apiVersions": {
            "edge": {
                "v1": {
                    "path": "/edge/v1"
                }
            }
        },
        "buildDate": "2020-06-11 16:03:13",
        "revision": "95e78d4bc64b",
        "runtimeVersion": "go1.14.3",
        "version": "v0.15.0"
    },
    "meta": {}
}
```

Example of a theoretical future version with the Fabric REST API:

```
{
    "data": {
        "apiVersions": {
            "edge": {
                "v1": {
                    "path": "/edge/v1"
                }
            }
            "fabric": {
                "v1": {
                    "path": "/fabric/v1"
                }
            }
        },
        "buildDate": "2020-06-20 12:43:03",
        "revision": "1a27ed4bc64b",
        "runtimeVersion": "go1.14.3",
        "version": "v0.15.10"
    },
    "meta": {}
}
```

#### ShortIds

The Edge REST API has used UUID and its associated UUID text format for
all ids. In 0.15 and forward, `shortIds` will be used and their
associated format.

 * make ids more human human friendly (logs, visual comparison)
 * consolidate on ids that look similar between Fabric and Edge entities
 * maintain a high degree of uniqueness comparable to UUIDs

All Ziti REST APIs will specify their ids as `strings`. If clients treat
ids as opaque strings, then no comparability issues are expected. It is
highly highly suggested that all clients follow this pattern.

# Release 0.14.13
Ziti 0.14.13 includes the following:

* Ensure version information gets updated on non linux-amd64 builds

# Release 0.14.12
Ziti 0.14.12 includes the following:

* Fix the logging prefix to be github.com/openziti

# Release 0.14.11
Ziti 0.14.11 includes the following:

* [Ziti-Tunnel - Bind terminators are only created during startup](https://github.com/openziti/sdk-golang/issues/56)
* [Close on one side of connection doesn't propagate to other side](https://github.com/openziti/edge/issues/189)
* [Simplify sequencer close logic](https://github.com/openziti/foundation/issues/81)
* Misc Fixes
  * PEM decoding returns error when not able to decode
  * Ziti enrolment capabilities now supports `plain/text`, `application/x-pem-file`, and `application/json` response `accept` and `content-types`
* CLI Change
  * ziti-tunnel has learned a new subcommand `enroll`. Usage is identical to the existing `ziti-enroller`

# Release 0.14.10
Ziti 0.14.10 includes the following:

* Doc updates

# Release 0.14.9
Ziti 0.14.9 includes the following:

* [Move ziti edge controller commands to ziti edge](https://github.com/openziti/ziti/issues/108)
    * Note: for now `ziti edge` and `ziti edge controller` will both have edge controller related commands. `ziti edge controller` is deprecated and will be removed in a future release. Please update your scripts.

# Release 0.14.8
Ziti 0.14.8 includes the following:

* Doc updates

# Release 0.14.7
Ziti 0.14.7 includes the following:

* [Add CLI support for updating terminators](https://github.com/openziti/ziti/issues/106)
* [Add CLI support for managing identity service config overrides](https://github.com/openziti/ziti/issues/105)

NOTE: 0.14.6 was released with the same code as 0.14.5 due to CI re-running

# Release 0.14.5
## Theme
Ziti 0.14.5 includes the following:

### Features

  * Ziti Edge API
    * [CA Identity Name Format](https://github.com/openziti/edge/issues/147)
  * [Remove sourceType from metrics](https://github.com/openziti/foundation/issues/68)
  * Fix name of metric from `egress.tx.Msgrate` to `egress.tx.msgrate`

## Ziti Edge API
### CA Identity Name Format

A new field, `identityNameFormat`,has been added to all certificate authority elements (`GET /cas`) that is available for all CRUD operations.
This field is optional and defaults to `[caName] - [commonName]`. All existing CAs will also default to `[caName] - [commonName]`.

The field, `identityNameFormat`, may contain any text and optionally include the following strings that are
replaced with described values:

* `[caId]` - the id of the CA used for auto enrollment
* `[caName]` - the name of the CA used for auto enrollment
* `[commonName]` - the common name supplied by the enrolling cert
* `[identityName]` - the name supplied during enrollment (if any, defaults to `[identityId]` if the `name` field is blank during enrollment)
* `[identityId]` - id of the resulting identity

The default, `[caName] - [commonName]`, would result in the following for a CA named "myCa" with an enrolling certificate with the common name "laptop01":

```
myCa - laptop01
```

#### Identity Name Collisions

If an `identityNameFormat` results in a name collision during enrollment, an incrementing number will be appended to the resulting identity name. If this is not desired,
define an `identityNameFormat` that does not collide by using the above replacement strings and ensuring the resulting values (i.e. from`commonName`) are unique.

# Release 0.14.4
## Theme
Ziti 0.14.4 includes the following:

### Misc

  * Migration to github.com/openziti

# Release 0.14.3
## Theme
Ziti 0.14.3 includes the following:

### Fixes
  * [orphaned enrollments/authenticators post identity PUT](https://github.com/openziti/edge/issues/158)

## Orphaned Enrollments/Authenticators
When updating an identity via PUT it was possible to clear the authenticators and enrollments associated with the identity making
it impossible to authenticate as that identity or complete enrollment. This release removes failed enrollments, associates orphaned
authenticators with their target identities, addresses the root cause, and adds regression tests.

# Release 0.14.2
## Theme
Ziti 0.14.2 includes the following:

  * CLI enhancements
      * [can't create service policy with @ identity name](https://github.com/openziti/ziti/issues/93)
      * [Add CLI commands to allow updating policies and role attributes](https://github.com/openziti/ziti/issues/94)
      * [CLI: read config/config-type JSON from file](https://github.com/openziti/ziti/issues/90)
  * [Not found errors for assigned/related ids do not say which resource was not found](https://github.com/openziti/edge/issues/148)
  * Fixes to connection setup timing

## CLI Updates
### Names in Policy Roles
Polices can now be created from the CLI using @name. This was previously supported natively in the REST APIs, however it was stripped out for consistency. The CLI now supports this by looking up names and replacing them with IDS when they are entered. When policies are listed they will show names instead of IDs now as well.

```shell script
$ ziti edge controller create service-policy test-names Dial -i '#all' -s '@ssh'
Found services with id db9488ba-d0af-455b-9503-c6df88f228ff for name ssh
ba233791-8fde-44ba-9509-948275e3e3bb

$ ziti edge controller list service-policies 'name="test-names"'
id: ba233791-8fde-44ba-9509-948275e3e3bb    name: test-names    type: Dial    service roles: [@ssh]    identity roles: [#all]
results: 1-1 of 1 
```

### New Update Commands
There are now update commands which allow updating role attributes on edge-routers, identities and services and roles on all three policy types.

All the update commands also allow changing the entity and policy names.

```shell script
$ ziti edge controller update identity jsmith-laptop -a us-east,sales
$ ziti edge controller update service-policy sales-na -s o365,mattermost
```

### Breaking Change to CLI commands
The shorthands for some policy flags have changed

  * The shorthand for create edge-router-policy `--edge-router-roles` is now `-e`. It was `-r`
  * The shorthand for create service-edge-router-policy `--edge-router-roles` is now `-e`. It was `-r`
  * The shorthand for create service-policy `--service-roles` is now `-s`. It was `-r`

# Release 0.14.1
## Theme
Ziti 0.14.1 includes the following:

### Features
  * [Enable graceful shutdown of bound connections](https://github.com/openziti/edge/issues/149)

### Fixes
  * [Enrollments w/ 0 length bodies cause enrollment errors](https://github.com/openziti/edge/issues/150)
  * Fixed race condition in end-to-end encryption setup
  * Xt fixes
      * Fixed strategies missing session ended events
      * Fixed costed terminator sorting
      * Fixed race condition where terminators could be selected right after delete because they would have default cost.
      * Expanded space between precedence levels to ensure terminator static cost doesn't allow total costs to jump precedence boundary
      * Fixed type error in failure cost tracker
  * Logging cleanup - many log statements that were error or info have been dropped to debug
  * ziti-probe can now handle partial configs

## Graceful SDK Hosted Application Shutdown
The Golang SDK now returns an edge.Listener instead of a net.Listener from Listen

```go
type Listener interface {
	net.Listener
	UpdateCost(cost uint16) error
	UpdatePrecedence(precedence Precedence) error
	UpdateCostAndPrecedence(cost uint16, precedence Precedence) error
}
```

This allows clients to set their precedence to `failed` when shutting down. This will allow them to gracefully finishing any outstanding requests while ensuring that no new requests will be routed to this application. This should allow for applications to do round-robin upgrades without service interruption to clients. It also allows clients to influence costs on the fly based on knowledge available to the application.

Support is currently limited to the Golang SDK, support in other SDKs will be forthcoming as it is prioritized.

# Release 0.14.0
## Theme
Ziti 0.14.0 includes the following:

### Features
  * The first full implementation of high availability (HA) and horizontal scale (HS) services

### Fixes
  * [When using index scanner, wrong count is returned when using skip](https://github.com/openziti/foundation/issues/62)
  * fabric now includes migration to extract terminators from services
  * more errors which were returning 500 now return appropriate 404 or 400 field errors
  * terminators are now validated when routers connect, and invalid ones can be removed
  * a potential race condition in UDP connection last time has been fixed and UDP connection logging has been tidied
  * Terminator precedence may now be specified in the golang SDK in the listen options when binding a service

## HA/HS
Ziti 0.12 extracted terminators from services. Services could have multiple terminators but only the first one would get used. Service have a `terminatorStrategy` field which was previously unused. Now the terminatorStrategy will determine how Ziti picks from multiple terminators to enable either HA or HS behavior.

### Xt
The fabric now includes a new framework called Xt (eXtensible Terminators) which allows defining terminator strategies and defines how terminator strategies and external components integrate with smart routing. The general flow of terminator selection goes as follows:

  1. A client requests a new session for a service
  1. Smart routing finds all the active terminators for the session (active meaning the terminator's router is connected)
  1. Smart routing calculates a cost for each terminator then hands the service's terminator strategy a list of terminators and their costs ranked from lowest to highest
  1. The strategy returns the terminator that should be used
  1. A new session is created using that path.

Strategies will often work by adjusting terminator costs. The selection algorithm the simply returns the lowest cost option presented by smart routing.

#### Costs
There are a number of elements which feed the smart routing cost algorithm.

##### Route Cost
The cost of the route from the initiating route to the terminator router will be included in the terminator cost. This cost may be influenced by things such as link latencies and user determined link costs.

##### Static Cost
Each terminator has a static cost which can be set or updated when the terminator is created. SDK applications can set the terminator cost when they invoke the Listen operation.

#### Precedence
Each terminator has a precedence. There are three precedence levels: `required`, `default` and `failed`.

Smart routing will always rank terminators with higher precedence levels higher than terminators with lower precedence levers. So required terminators will always be first, default second and failed third. Precedence levels can be used to implement HA. The primary will be marked as required and the secondary as default. When the primary is determined to be down, either by some internal or external set of heuristics, it will be marked as Failed and new sessions will go to the secondary. When the primary recovers it can be bumped back up to Required.

##### Dynamic Cost
Each terminator also has a dynamic cost that will move a terminator up and down relative to its precedence. This cost can be driven by stratagies or by external components. A strategy might use number of active of open sessions or dial successes and failures to drive the cost.

##### Cost API
Costs can be set via the Costs API in Xt:

```go
package xt

type Costs interface {
	ClearCost(terminatorId string)
	GetCost(terminatorId string) uint32
	GetStats(terminatorId string) Stats
	GetPrecedence(terminatorId string) Precedence
	SetPrecedence(terminatorId string, precedence Precedence)
	SetPrecedenceCost(terminatorId string, weight uint16)
	UpdatePrecedenceCost(terminatorId string, updateF func(uint16) uint16)
	GetPrecedenceCost(terminatorId string) uint16
}
```

Each terminator has an associated precedence and dynamic cost. This can be reduced to a single cost. The cost algorithm ensures terminators at difference precedence levels do not overlap. So a terminator which is marked failed, with dynamic cost 0, will always have a higher calculated cost than a terminator with default precedence and maximum value for dynamic cost.

#### Strategies
Strategies must implement the following interface:

```go
package xt

type Strategy interface {
	Select(terminators []CostedTerminator) (Terminator, error)
	HandleTerminatorChange(event StrategyChangeEvent) error
	NotifyEvent(event TerminatorEvent)
}
```

The `Select` method will be called by smart routing to pick terminators for a session. The session can react to terminator changes, such when a terminator is added to or removed from a service. The service is also notified via `NotifyEvent` whenever a session dial succeeds or fails and when a session for the service is ended.

The fabric currently provides four strategy implementions.

##### `smartrouting`
This is the default strategy. It always uses the lowest cost terminator. It drives costs as follows:

  * Cost is proportional to number of open sessions
  * Dial failures drive the cost up
  * Dial successes drive the cost down, but only as much as they were previously driven up by failures

##### `weighted`
This strategy drives costs in the same way as the `smartrouting` strategy. However instead of always picking the lowest cost terminator it does a weighted random selection across all terminators of the highest precedence. If a terminator has double the cost of another terminator it should get picked approximately half as often.

##### `random`
This strategy does not change terminator weights. It does simple random selection across all terminators of the highest precedence.

##### `ha`
This strategy assumes that one terminator will have `required` precedence and there will be a secondary terminator with `default` precedence. If three consecutive dials to the highest ranked terminator fail in a row it will be marked as failed. This will allow the secondary to take over. If the primary recovers it can be marked as required again via the APIs.

### API Changes
The terminator endpoint now supports setting the static terminator cost and terminator precedence.

    * Endpoint: /terminators
        * Operations: PUT/POST/PATCH now take 
            * cost, type uint16, default 0
            * prededence, type string, default 'default', valid values: required, default, failed
        * Operation: GET now returns staticCost, dynamicCost


# Release 0.13.9
## Theme
Ziti 0.13.9 includes the following:

 * Adds paging information to cli commands

 Example

 ```shell script
$ ec list api-sessions "true sort by token skip 2 limit 3" 
id: 37dd1463-e4e7-40de-9a63-f75486430361    token: 0b392a2f-47f8-4561-af63-93807ce70d93    identity: Default Admin
id: 6fb5b488-debf-4212-9670-f250e31b3d4f    token: 15ae6b00-f123-458c-a121-5cf91983a2c2    identity: Default Admin
id: 8aa4a074-b2c7-4d55-9f56-17199ab6ac11    token: 1b9418d8-b9a7-4e39-a876-7a9588f5e7ed    identity: Default Admin
results: 3-5 of 23
```

# Release 0.13.8
## Theme
Ziti 0.13.8 includes the following:

 * Fixes Ziti Edge Router ignoring connect options for SDK listener


# Release 0.13.7
## Theme
Ziti 0.13.7 includes the following:

  * Improvements to sdk availability when hosting services
  * Various bug fixes to related to terminators and transit routers

## SDK Resilience
The golang sdk now has a new listen method on context, which takes listen options.

```go
type Context interface {
	...
	ListenWithOptions(serviceName string, options *edge.ListenOptions) (net.Listener, error)
    ...
}

type ListenOptions struct {
	Cost           uint16
	ConnectTimeout time.Duration
	MaxConnections int
}
```

The SDK now supports the following:

  * Configuring connect timeout
  * Allow establishing new session, if existing session goes away
  * Allow establishing new API session, existing API session goes away
  * If client doesn't have access to service, it should stop listening and return an error
  * If client can't establish or re-establish API session, it should stop listening and return error

If paired with a ziti controller/routers which support terminator strategies for HA/HS, the following features are also supported:

  * Handle listen to multiple edge routers.
  * Allow configuring max number of connections to edge routers

# Release 0.13.6
## Theme

  * Fixes the `-n` flag being ignored for `ziti-enroll`

# Release 0.13.5
## Theme

  * Adds ability to verify 3rd party CAs via the CLI in the Ziti Edge API

## Ziti CLI Verify CA Support

Previous to this version the CLI was only capable of creating, editing,
and deleting CAs. For a CA to be useful it must be verified. If not,
it cannot be used for enrollment or authentication. The verification
process requires HTTP requests and the creation of a signed verification
certificate. The Ziti CLI can now perform all or part of this process.


### Example: No Existing Verification Cert
This example is useful for situations where access to the CA's
private key is possible. This command will fetch the CA's verification
token from the Ziti Edge API, create a short lived (5 min) verification
certificate, and use it to verify the CA.

This example includes the `--password` flag which is optional. If the
`--password` flag is not included and the private key is encrypted
the user will be prompted for the password.

- `myCa` is the name or id of a CA that has already been created.
- `ca.cert.pem` the CA's public x509 PEM formatted certificate
- `ca.key.pem` the CA's private x509 PEM formatted key

```
$ ziti edge controller verify ca myCa --cacert ca.cert.pem --cakey ca.key.pem --password 1234
```

###  Example: Existing Verification Certificate
This example is useful for situations where access to the signing CA's
private key is not possible (off-site, coldstore, etc.). This example
assumes that the appropriate `openssl` commands have been run to
generate the verification script.

- `myCa` is the name or id of a CA that has already been created.
- `verificationCert.pem` is a PEM encoded x509 certificate that has the common name set to the verification token of `myCa`
```
$ ziti edge controller verify ca myCa --cert verificationCert.pem
```

### Command help:
```
$ ziti edge controller verify ca --help

Usage:
  ziti edge controller verify ca <name> ( --cert <pemCertFile> | --cacert
  <signingCaCert> --cakey <signingCaKey> [--password <caKeyPassword>]) [flags]

Flags:
  -a, --cacert string     The path to the CA cert that should be used togenerate and sign a verification cert
  -k, --cakey string      The path to the CA key that should be used to generate and sign a verification cert
  -c, --cert string       The path to a cert with the CN set as the verification token and signed by the target CA
  -h, --help              help for ca
  -j, --output-json       Output the full JSON response from the Ziti Edge Controller
  -p, --password string   The password for the CA key if necessary
```

# Release 0.13.4
## Theme
 * Updates `quickstart` scripts

# Release 0.13.3
## Theme
Ziti 0.13.3 includes the following:

  * Adds connect parameters for incoming channel2 connections (control, management, and SDK connections)
    * The options have internal defaults are needed only when connections

## Connection Parameters

A new set of options have been introduced for channel2 backed listeners. Channel2 is a library used to establish message based connections between a channel2 client and server.
Most importantly this is used for control and management connections in the `ziti-controller` and for the SDK connections accepted in `ziti-router`. Setting these values to
invalid values will result in errors during startup of the `ziti-controller` and `ziti-router`

  * `maxQueuedConnects` - set the maximum number of connect requests that are buffered and waiting to be acknowledged (1 to 5000, default 1000)
  * `maxOutstandingConnects` - the maximum number of connects that have  begun hello synchronization (1 to 1000, default 16)
  * `connectTimeoutMs` - the number of milliseconds to wait before a hello synchronization fails and closes the connection (30ms to 60000ms, default: 1000ms)


Example: `ziti-controller` configuration file:

```
# the endpoint that routers will connect to the controller over.
ctrl:
  listener:             tls:127.0.0.1:6262
  options:
    maxQueuedConnects:      50
    maxOutstandingConnects: 100
    connectTimeoutMs:       3000

# the endpoint that management tools connect to the controller over.
mgmt:
  listener:             tls:127.0.0.1:10000
  options:
    maxQueuedConnects:      50
    maxOutstandingConnects: 100
    connectTimeoutMs:       3000
```

Example: `ziti-router` configuration file:

```
listeners:
  - binding: edge
    address: tls:0.0.0.0:3022
    options:
      # (required) The public hostname and port combination that Ziti SDKs should connect on. Previously this was in the chanIngress section.
      advertise: 127.0.0.1:3022
      maxQueuedConnects:      50
      maxOutstandingConnects: 100
      connectTimeoutMs:       3000
```


# Release 0.13
## Theme
Ziti 0.13 includes the following:

  * Changes to make working with policies easier, including
      * New APIs to list existing role attributes used by edge routers, identities and services
      * New APIs to list entities related by polices (such as listing edge routers available to a service via service edge router policies)
      * Enhancements to the LIST APIs for edge routers, identities and services which allow one to filter by roles
      * A policy advisor API, which helps analyze policies and current system state to figure out if an identity should be able to use a service and if not, why not
  * CA Auto Enrollment now allows identities to inherit role attributes from the validating CA
      * New `identityRole` attributes added to CA entities
  * New APIs to list and manage Transit Routers
  * Transit Routers now support enrolment via `ziti-router enroll`
  * Embedded Swagger/OpenAPI 2.0 endpoint
  * A small set of APIs accepted id or name. These have been changed to accept only id
  * Fabric enhancements
      * New Xlink framework encapsulating the router capabilities for creating overlay mesh links.
      * Adjustable Xgress MTU size.
  * All Ziti Go projects are now being built with Go 1.14
      * See here for change to Go in 1.14 - https://golang.org/doc/go1.14

## Making Policies More User Friendly
### Listing Role Attributes in Use

There are three new endpoints for listing role attributes in use.

    * Endpoint: /edge-router-role-attributes
    * Endpoint: /identity-role-attributes
    * Endpoint: /service-role-attributes

All three support the same operations:

    * Supported operations
        * List: GET
            * Supports filtering
            * role attributes can be filtered/sorted using the symbol `id`
            * Ex:`?filter=id contains "north" limit 5`

The CLI supports these new operations as well.

    ziti edge controller list edge-router-role-attributes
    ziti edge controller list identity-role-attributes
    ziti edge controller list service-role-attributes

Example output:

    $ ec list service-role-attributes "true sort by id desc limit 5" -j
    {
        "meta": {
            "filterableFields": [
                "id"
            ],
            "pagination": {
                "limit": 5,
                "offset": 0,
                "totalCount": 10
            }
        },
        "data": [
            "two",
            "three",
            "support",
            "sales",
            "one"
        ]
    }

## Listing Entities Related by Policies
This adds operations to the `/services`, `/identities` and `/edge-routers` endpoints.

    * Endpoint: /edge-routers
    * New operations
       * Query related identities: GET /edge-routers/<edge-router-id>/identities?filter=<optional-filter>
       * Query related services: GET /edge-routers/<edge-router-id>/services?filter=<optional-filter>

    * Endpoint: /identities
    * New operations
       * Query related edge routers: GET /identities/<identity-id>/edge-routers?filter=<optional-filter>
       * Query related services: GET /identities/<identity-id>/services?filter=<optional-filter>

    * Endpoint: /services
    * New operations
       * Query related identities: GET /services/<service-id>/identities?filter=<optional-filter>
       * Query related edge routers: GET /services/<service-id>/edge-routers?filter=<optional-filter>

## Filtering Entity Lists by Roles
When building UIs it may be useful to list entities which have role attributes by role filters, to see what policy changes may look like.

     * Endpoint: /edge-routers
     * Query: GET /edge-routers now supports two new query paramters
         * roleFilter. May be specified more than one. Should be an id or role specifier (ex: @38683097-2412-4335-b056-ae8747314dd3 or #sales)
         * roleSemantic. Optional. Defaults to AllOf if not specified. Indicates which semantic to use when evaluating role matches
 
     * Endpoint: /identities
     * Query: GET /identities now supports two new query paramters
         * roleFilter. May be specified more than one. Should be an id or role specifier (ex: @38683097-2412-4335-b056-ae8747314dd3 or #sales)
         * roleSemantic. Optional. Defaults to AllOf if not specified. Indicates which semantic to use when evaluating role matches
 
     * Endpoint: /services
     * Query: GET /services now supports two new query paramters
         * roleFilter. May be specified more than one. Should be an id or role specifier (ex: @38683097-2412-4335-b056-ae8747314dd3 or #sales)
         * roleSemantic. Optional. Defaults to AllOf if not specified. Indicates which semantic to use when evaluating role matches

Note that a roleFilter should have one role specifier (like `@some-id` or `#sales`). If you wish to specify multiple, provide multiple role filters.

    /edge-routers?roleFilter=#sales&roleFilter=#us

These are also supported from the CLI when listing edge routers, identities and services using the --role-filters and --role-semantic flags.

Example:

    $ ec list services
    id: 2a724ae7-8b8f-4688-90df-34951bce6720    name: grpc-ping    terminator strategy:     role attributes: ["fortio","fortio-server"]
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: 9480e39d-0664-4482-b230-5da2c17b225b    name: iperf    terminator strategy:     role attributes: {}
    id: a949cf80-b11b-4cce-bbb7-d2e4767878a6    name: baz    terminator strategy:     role attributes: ["development","sales","support"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]
    
    $ ec list services --role-filters '#three'
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]

    $ ec list services --role-filters '#three','#two'
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]
    
    $ ec list services --role-filters '#three','#sales' --role-semantic AnyOf
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: a949cf80-b11b-4cce-bbb7-d2e4767878a6    name: baz    terminator strategy:     role attributes: ["development","sales","support"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]
    
    $ ec list services --role-filters '#three''#sales','@4e33859b-070d-42b1-8b40-4adf973f680c' --role-semantic AnyOf
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: a949cf80-b11b-4cce-bbb7-d2e4767878a6    name: baz    terminator strategy:     role attributes: ["development","sales","support"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]

## Policy Advisor
This adds a new operation to the /identities endpoint

    * Endpoint: /identities
    * New operations
       * Query related identities: GET /identities/<identity-id>/policy-advice/<service-id>

This will return the following information about the identity and service:

   * If the identity can dial the service
   * If the identity can bind the service
   * How many edge routers the identity has access to
   * How many edge routers the service can be accessed through
   * Which edge routers the identity and service have in common (if this is none, then the service can't be accessed by the identity)
   * Which of the common edge routers are on-line

Example result:

    {
        "meta": {},
        "data": {
            "identityId": "700347c8-ca3a-4438-9060-68f7255ee4f8",
            "identity": {
                "entity": "identities",
                "id": "700347c8-ca3a-4438-9060-68f7255ee4f8",
                "name": "ssh-host",
                "_links": {
                    "self": {
                        "href": "./identities/700347c8-ca3a-4438-9060-68f7255ee4f8"
                    }
                }
            },
            "serviceId": "8fa27a3e-ffb1-4bd1-befa-fcd38a6c26b3",
            "service": {
                "entity": "services",
                "id": "8fa27a3e-ffb1-4bd1-befa-fcd38a6c26b3",
                "name": "ssh",
                "_links": {
                    "self": {
                        "href": "./services/8fa27a3e-ffb1-4bd1-befa-fcd38a6c26b3"
                    }
                }
            },
            "isBindAllowed": true,
            "isDialAllowed": false,
            "identityRouterCount": 2,
            "serviceRouterCount": 2,
            "commonRouters": [
                {
                    "entity": "edge-routers",
                    "id": "43d220d8-860e-4d80-a25c-97322a7326b4",
                    "name": "us-west-1",
                    "_links": {
                        "self": {
                            "href": "./edge-routers/43d220d8-860e-4d80-a25c-97322a7326b4"
                        }
                    },
                    "isOnline": false
                },
                {
                    "entity": "edge-routers",
                    "id": "8c118857-c12e-430d-9109-c31f535933f6",
                    "name": "us-east-1",
                    "_links": {
                        "self": {
                            "href": "./edge-routers/8c118857-c12e-430d-9109-c31f535933f6"
                        }
                    },
                    "isOnline": true
                }
            ]
        }
    }

The CLI has also been updated with a new policy-advisor common.

Examples:

    # Inspect all identities for policy issues
    ziti edge controller policy-advisor identities

    # Inspect just the jsmith-laptop identity for policy issues with all services that the identity can access
    ziti edge controller policy-advisor identities jsmith-laptop

    # Inspect the jsmith-laptop identity for issues related to the ssh service
    ziti edge controller policy-advisor identities jsmith-laptop ssh

    # Inspect all services for policy issues
    ziti edge controller policy-advisor services

    # Inspect just the ssh service for policy issues for all identities the service can access
    ziti edge controller policy-advisor services ssh

    # Inspect the ssh service for issues related to the jsmith-laptop identity 
    ziti edge controller policy-advisor identities ssh jsmith-laptop

Some example output of the CLI:

    $ ec policy-advisor identities -q
    ERROR: mlapin-laptop (1) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    ERROR: mlapin-laptop (1) -> ssh (2) Common Routers: (0/0) Dial: Y Bind: N 
      - Identity and services have no edge routers in common. Adjust edge router policies and/or service edge router policies.
    
    ERROR: ndaniels-laptop (1) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    ERROR: ndaniels-laptop (1) -> ssh (2) Common Routers: (0/1) Dial: Y Bind: N 
      - Common edge routers are all off-line. Bring routers back on-line or adjust edge router policies and/or service edge router policies to increase common router pool.
    
    ERROR: Default Admin 
      - Identity does not have access to any services. Adjust service policies.
    
    ERROR: jsmith-laptop (2) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    OKAY : jsmith-laptop (2) -> ssh (2) Common Routers: (1/2) Dial: Y Bind: N 
    
    ERROR: ssh-host (2) -> ssh-backup (0) Common Routers: (0/0) Dial: N Bind: Y 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    OKAY : ssh-host (2) -> ssh (2) Common Routers: (1/2) Dial: N Bind: Y 
    
    ERROR: aortega-laptop 
      - Identity does not have access to any services. Adjust service policies.
    
    ERROR: djones-laptop (0) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Identity has no edge routers assigned. Adjust edge router policies.
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    ERROR: djones-laptop (0) -> ssh (2) Common Routers: (0/0) Dial: Y Bind: N 
      - Identity has no edge routers assigned. Adjust edge router policies.

    $ ec policy-advisor identities aortega-laptop ssh-backup -q
    Found identities with id 70567104-d4bd-45f1-8179-bd1e6ab8751f for name aortega-laptop
    Found services with id 46e94977-0efc-4e7d-b9ae-cc8df1c95fc1 for name ssh-backup
    ERROR: aortega-laptop (0) -> ssh-backup (0) Common Routers: (0/0) Dial: N Bind: N 
      - No access to service. Adjust service policies.
      - Identity has no edge routers assigned. Adjust edge router policies.
      - Service has no edge routers assigned. Adjust service edge router policies.

## CA Auto Enrollment Identity Attributes

Identities that are enrolled via a CA can now inherit a static list of identity role attributes. The normal create,
update, patch requests supported by the CA entities now allow the role attributes to be specified. CA identity role
attribute changes do no propagate to identities that have completed enrollment.

This feature allows a simple degree of automation for identities that are auto-provisioning through a third party CA.

   * `identityRoles` added to `/ca` endpoints for normal CRUD operations
   * `identityRoles` from a CA entity are point-in-time copies

## New APIs to list and manage Transit Routers

The endpoint`/transit-routers` has been added to create and manage Transit Routers. Transit Routers do not handle incoming Ziti
Edge SDK connections.

    * Endpoint: /transit-routers
    * Supported operations
        * Detail: GET /transit-routers/<transit-router-id>
        * List: GET /transit-routers/
        * Create: POST /transit-routers
        * Update All Fields: PUT /transit-routers/<transit-router-id>
        * Update Selective Fields: PATCH /transit-routers/<transit-router-id>
        * Delete: DELETE /transit-routers/<transit-router-id>
     * Properties
         * Transit Routers support the standard properties (id, createdAt, updatedAt, tags)
         * name - Type string - a friendly Edge name for the transit router
         * fingerprint - Type string - a hex string fingerprint of the transit router's public certificate (post enrollment)
         * isVerified - Type bool - true if the router has completed enrollment
         * isOnline - Type bool - true if the router is currently connected to the controller
         * enrollmentToken - Type string - the enrollment token that would be used during enrollment (nil post enrollment)
         * enrollmentJwt - Type string - an enrollment JWT suitable for use with "ziti-router enroll" (nil post enrollment)
         * enrollmentCreatedAt - Type date-time - the date and time the enrollment was created (nil post enrollment)
         * enrollmentExpiresAt - Type date-time - the date and time the enrollment expires at (matches JWT expiration time, nil post enrollment)

Example list output:
```json
{
    "meta": {
        "filterableFields": [
            "id",
            "createdAt",
            "updatedAt",
            "name"
        ],
        "pagination": {
            "limit": 10,
            "offset": 0,
            "totalCount": 2
        }
    },
    "data": [
        {
            "id": "002",
            "createdAt": "2020-03-30T00:55:38.1701084Z",
            "updatedAt": "2020-03-30T00:55:38.1701084Z",
            "_links": {
                "self": {
                    "href": "./transit-routers/002"
                }
            },
            "tags": {},
            "name": "",
            "fingerprint": "07e011481921b4734df82c52ae2b3113617cdd18",
            "isVerified": true,
            "isOnline": false,
            "enrollmentToken": null,
            "enrollmentJwt": null,
            "enrollmentCreatedAt": null,
            "enrollmentExpiresAt": null
        },
        {
            "id": "99f4109b-cd6d-40e3-9a62-bee24d7eccd6",
            "createdAt": "2020-03-30T17:48:17.2949059Z",
            "updatedAt": "2020-03-30T17:48:17.2949059Z",
            "_links": {
                "self": {
                    "href": "./transit-routers/99f4109b-cd6d-40e3-9a62-bee24d7eccd6"
                }
            },
            "tags": {},
            "name": "",
            "fingerprint": "25d1048f3c7bc4a5956ce7316e2ca70999c0e27d",
            "isVerified": true,
            "isOnline": false,
            "enrollmentToken": null,
            "enrollmentJwt": null,
            "enrollmentCreatedAt": null,
            "enrollmentExpiresAt": null
        }
    ]
}
```
## Transit Routers now support enrolment via `ziti-router enroll`

Transit Routers now enroll using the same command: `ziti-router enroll <config> -j <jwt>`. During the enrollment process,
the CSR properties used will be taken from `edge.csr`. If `edge.csr` does not exist `csr` will be utilized. If both are
missing an error will occur.

Example router configuration:

```yaml
v: 3

identity:
  cert:                 etc/ca/intermediate/certs/001-client.cert.pem
  server_cert:          etc/ca/intermediate/certs/001-server.cert.pem
  key:                  etc/ca/intermediate/private/001.key.pem
  ca:                   etc/ca/intermediate/certs/ca-chain.cert.pem

# Configure the forwarder options
#
forwarder:
  # How frequently does the forwarder probe the link latency. This will ultimately determine the resolution of the
  # responsiveness available to smart routing. This resolution comes at the expense of bandwidth utilization for the
  # probes, control plane utilization, and CPU utilization processing the results.
  #
  latencyProbeInterval: 1000

# Optional CSR section for transit router enrollment via `ziti-router enroll <config> -j <jwt>`
csr:
  country: US
  province: NC
  locality: Charlotte
  organization: NetFoundry
  organizationalUnit: Ziti
  sans:
    dns:
      - "localhost"
      - "test-network"
      - "test-network.localhost"
      - "ziti-dev-ingress01"
    email:
      - "admin@example.com"
    ip:
      - "127.0.0.1"
    uri:
      - "ziti://ziti-dev-router01/made/up/example"


#trace:
#  path:                 001.trace

#profile:
#  memory:
#    path:               001.memprof
#  cpu:
#    path:               001.cpuprof

ctrl:
  endpoint:             tls:127.0.0.1:6262

link:
  dialers:
    - binding:          transport

listeners:
  # basic ssh proxy
  - binding:            proxy
    address:            tcp:0.0.0.0:1122
    service:            ssh
    options:
      mtu:              768

  # for iperf_tcp (iperf3)
  - binding:            proxy
    address:            tcp:0.0.0.0:7001
    service:            iperf

  # for iperf_udp (iperf3)
  - binding:            proxy_udp
    address:            udp:0.0.0.0:7001
    service:            iperf_udp

  # example xgress_transport
  - binding:            transport
    address:            tls:0.0.0.0:7002
    options:
      retransmission:   true
      randomDrops:      true
      drop1InN:         5000

  # example xgress_udp
  - binding:            transport_udp
    address:            udp:0.0.0.0:7003
    options:
      retransmission:   true
      randomDrops:      true
      drop1InN:         5000

```

## Embedded Swagger/OpenAPI 2.0 endpoint

The endpoint`/specs` has been added to retrieve API specifications from the Ziti Controller. The specifications
are specific to the version of the controller deployed.

The main endpoint to retrieve the Swagger/Open API 2.0 specification is: `/specs/swagger/spec`

    * Endpoint: /specs
    * Supported operations
        * Detail: GET /specs/<spec-id>
        * Get Spec: GET /specs/<spec-id>/spec
        * List: GET /specs/
     * Properties
         * Transit Routers support the standard properties (id, createdAt, updatedAt, tags)
         * name - Type string - the and intent of the spec


## APIs now only accept ID, not ID or Name
  1. Some APIs related to configurations accepted config names or ids. These now only accept name.
  1. Policies would accept entity references with names as well as ids. So you could use `@ssh`, for example when referencing the ssh service. These now also only accept ID

In general allowing both values adds complexity to the server side code. Consuming code, such as user interfaces or the ziti cli, can do the name to id translation just as easily.

## Fabric Enhancements
### Xlink Framework

The new Xlink framework **requires** that the router configuration file is updated to `v: 3`.

The `link` section of the router configuration is now structured like this:

```
link:
  listeners:
    - binding:          transport
      bind:             tls:127.0.0.1:6002
      advertise:        tls:127.0.0.1:6002
      options:
        outQueueSize:   16
  dialers:
    - binding:          transport
```

The `link/listeners/bind` address replaces the old `link/listener` address, and the `link/listeners/advertise` address replaces the old `link/advertise` address.

**The router configuration MUST be updated to include `link/dialers` section with a `transport` binding (as in the above example) to include support for outbound link dialing.**

Subsequent releases will include support for multiple Xlink listeners and dialers. 0.13 supports only a single listener and dialer to be configured.

### Xgress MTU

The Xgress listeners and dialers now support an `mtu` option in their `options` stanza:

```
listeners:
  # basic ssh proxy
  - binding:            proxy
    address:            tcp:0.0.0.0:1122
    service:            ssh
    options:
      mtu:              768
      
dialers:
  - binding:            transport
    options:
      mtu:              512
```
This MTU controls the maximum size of the `Payload` packet sent across the data plane of the overlay.

# Release 0.12
## Theme
Ziti 0.12 includes the following:

 * Terminators have been extracted from services
     * Terminators define where a service terminates. Previously each service had exactly one terminator. Now services can have 0 to N terminators.
 * List APIs now support inline paging
 * Association APIs now support filtering, paging and querying
 * The bolt datastore creates a backup of the datastore file before attempting a schema/data migration
 * Fabric and edge code are now much more closely aligned at the persistence and model layers
 * Some deprecated endpoints are now being removed

## Terminators
See https://github.com/openziti/fabric/wiki/Pluggable-Service-Terminators for a discussion of what service terminators are, the motivation for extracting them from services and the design for how they will work.

This release includes the following:

  * Terminators extracted from service as separate entities
  * When SDK applications bind and unbind, the controller now dynamically adds/removes terminators as appropriate

This release does not yet include a terminator strategy API. Strategies can be specified per service, but if a service has multiple terminators the first one will be used. The terminator strategy API along with some implementations will be added in a follow-up release. This release also does not include strategy inputs on terminators as discussed in the above design document. If strategy inputs end up being useful, they may be added in the furure.

### Terminator related API changes

There is a new terminators endpoint:

    * Endpoint: /terminators
    * Supported operations
        * Detail: GET /terminators/<terminator-id>
        * List: GET /terminators/
        * Create: POST /terminators
        * Update All Fields: PUT /terminators/<terminator-id>
        * Update Selective Fields: PATCH /terminators/<terminator-id>
        * Delete: DELETE /terminators/<terminator-id>
     * Properties
         * Terminators support the standard properties (id, createdAt, updatedAt, tags)
         * service - type: uuid, must be a valid service id
         * router - type: uuid, must be a valid router id
         * binding - type: string. Optional, defaults to "transport". The xgress binding on the selected router to use
         * address - type: string. The address that will be dialed using the xgress component on the selected router

The service endpoint has changes as well:

    * Endpoint: /services
    * New operations
       * Query related endpoints: GET /services/<service-id>/terminators?filter=<optional-filter>
    * The following properties have been removed
       * egressRouter
       * endpointAddress
    * The following property has been added
       * terminatorStrategy - type: string, optional. The terminator strategy to use. Currently unused.

The fabric service definition has also changed (visible from ziti-fabric).

  * The following properties have been removed
       * `binding`
       * `egressRouter`
       * `endpointAddress`
  * The following property has been added
       * `terminatorStrategy`

The ziti and ziti-fabric CLIs have been updated with new terminator related functionality, so that terminators can be viewed, created and deleted from both.

## Filtering/Sorting/Paging Changes
List operations on entities previously allowed the following parameters:

  * `filter`
  * `sort`
  * `limit`
  * `offset`

These are all still supported, but now sort, limit and offset can also be included in the filter. If parameters are specified both in the filter and in an explicit query parameter, the filter takes precedence.

When listing entities from the ziti CLI, filters can be included as an optional argument.

For example:

    $ ziti edge controller list services
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: 9480e39d-0664-4482-b230-5da2c17b225b    name: iperf    terminator strategy:     role attributes: {}
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    id: dc0446f0-7eaa-465f-80b5-c88f0a6b59cc    name: grpc-ping    terminator strategy:     role attributes: ["fortio","fortio-server"]
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}

    $ ziti edge controller list services 'name contains "s"'
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    
    $ ziti edge controller list services 'name contains "s" sort by name'
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    
    $ ziti edge controller list services 'name contains "s" sort by name skip 1 limit 2'
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}

Association lists now also support filtering, sorting and paging. Association GET operations only support the filter parameter.


    $ ziti edge controller list service terminators ssh
    Found services with id cd1ae16e-5015-49ad-9864-3ca0f5814091 for name ssh
    id: 41f4fd01-0bd7-4987-93b3-3b2217b00a22    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:22
    id: a5213300-9c5f-4b0e-a790-1ed460964d7c    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:2022
    
    $ ziti edge controller list service terminators ssh "true sort by address"
    Found services with id cd1ae16e-5015-49ad-9864-3ca0f5814091 for name ssh
    id: a5213300-9c5f-4b0e-a790-1ed460964d7c    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:2022
    id: 41f4fd01-0bd7-4987-93b3-3b2217b00a22    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:22

    $ ziti edge controller list service terminators ssh "true sort by address desc"
    Found services with id cd1ae16e-5015-49ad-9864-3ca0f5814091 for name ssh
    id: 41f4fd01-0bd7-4987-93b3-3b2217b00a22    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:22
    id: a5213300-9c5f-4b0e-a790-1ed460964d7c    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:2022

## Bolt Datastore Migrations
The fabric now supports migrating schema/data from one version to another. The fabric and edge share a common framework for migration. The migration framework now also automatically backs up the bolt data file before migration data. The backup file will have the same name as the original bolt file but with a timestamp appended to it.

Example:

    Original file: /tmp/ziti-bolt.db
    Backup file:   /tmp/ziti-bolt.db-20200316-134725

The fabric and edge schemas do not yet get migrated in the same transaction. This will be addressed in a follow-up release.

## Fabric and Edge Alignment
The fabric and edge persistence and model layers are now using the same foundational plumbing. This will allow for a common API layer in a follow-up release.

As part of this consolidation effort, fabric entities now share the same set of common properties as edge entities, namely:

  * `id`
  * `createdAt`
  * `updatedAt`
  * `tags`

Previously the only common property was `id`.

## Deprecated Endpoints
The `/gateways` (replaced by `/edge-routers`) and `network-sessions` (replaced by `/sessions`) endpoints, which were previously deprecated, have now been removed.

## Miscellaneous

There is a new `ziti edge controller version` command which shows information about the version of the controller being connected to:

Example:

    $ ziti edge controller version
    Version     : v0.9.0
    GIT revision: ea556fc18740
    Build Date  : 2020-02-11 16:09:08
    Runtime     : go1.13

# Release 0.11
## Theme
Ziti 0.11 includes the following:

 * Ziti connections from Ziti SDK client to services hosted by SDK are encrypted end-to-end (no API changes)


## End-to-end Encryption

Client and Hosting SDK instances setup end-to-end channels using secure key exchange and [AEAD](https://en.wikipedia.org/wiki/Authenticated_encryption) streams.
Read more about on https://openziti.github.io (_coming soon_)

# Releaes 0.10
## Theme
Ziti 0.10 includes a single change:

 * Edge API input validation processing was changed to operate on the supplied JSON instead of target objects


## Edge API Validation

Before this version, the Edge API allowed some fields to be omitted from requests. This behavior was due to the fact
that the API was validating the object that resulted from the JSON. This would cause some fields that were not supplied
to default to an acceptable nil/null/empty value.

Some APIs call may now fail with validation errors expecting fields to be defined for POST (create) and PUT (update)
operations. PATCH (partial update) should not be affected.

# Release 0.9
## Theme
Ziti 0.9 includes the following

 * a generic service configuration facility, useful for configuring service centric edge configuration data
 * several changes to policy syntax and semantics
 * service edge router policies are now a separate entity, instead of just a field on service


## Service Configuration
Configurations are named JSON style objects that can be associated with services. Configurations have a type.
A service can have 0 or 1 configurations of each configuration type associated with it.

### Configuration types
There is a new endpoint for managing config types.

    * Endpoint: `/config-types`
    * Supported operations
        * Detail: GET `/config-types/<config-type-id>`
        * List: GET `/config-types/`
        * Create: POST `/config-types`
        * Update All Fields: PUT `/config-types/<config-type-id>`
        * Update Selective Fields: PATCH `/config-types/<config-type-id>`
        * Delete: DELETE `/config-types/<config-type-id>`
        * List associated configs GET `/config-types/<config-id>/configs`
     * Properties
         * Config types support the standard properties (id, createdAt, updatedAt, tags)
         * name - type: string, constraints: required, must be unique. If provided must be a valid JSON schema.
         * schema - type: object. Optional.

If a schema is set on a type, that schema will be used to validate config data on configurations of that type. Validation
will happen if a configuration is created or updated. If a config type schema changes, the system does not re-validate
configurations of that type.

It is generally assumed that if there are backwards incompatible changes being made to a schema that a new config type
will be created and interested applications can support multiple configuration types.

The ziti CLI supports the following operations on config types:

    * create config-type
    * list config-types
    * list config-type configs
    * delete config-type

### Configurations
There is a new endpoint for managing configurations

    * Endpoint: `/configs`
    * Supported operations
        * Detail: GET `/configs/<config-id>`
        * List: GET `/configs/`
        * Create: POST `/configs/`
        * Update All Fields: PUT `/configs/<config-id>`
        * Update Selective Fields: PATCH `/configs/<config-id>`
        * Delete: DELETE `/config-types/<config-id>`
     * Properties
         * Configs support the standard properties (id, createdAt, updatedAt, tags)
         * name - type: string, constraints: unique
         * type - type: string. May be a config type id or config type name
         * data - type: JSON object
             * Support values are strings, numbers, booleans and nested objects/maps

The ziti CLI supports the following operations on configs:

    * create config
    * update config
    * list configs
    * delete config

```shell script
$ ziti edge controller create config ssh ziti-tunneler-client.v1 '{ "hostname" : "ssh.mycompany.com", "port" : 22 }'
83a1e815-04bc-4c91-8d88-1de8c943545f

$ ziti edge controller list configs
id:   83a1e815-04bc-4c91-8d88-1de8c943545f
name: ssh
type: f2dd2df0-9c04-4b84-a91e-71437ac229f1
data: {
          "hostname": "ssh.mycompany.com",
          "port": 22
      }

$ ziti edge controller update config ssh -d '{ "hostname" : "ssh.mycompany.com", "port" : 2022 }'
Found configs with id 83a1e815-04bc-4c91-8d88-1de8c943545f for name ssh

$ ziti edge controller list configs
id:   83a1e815-04bc-4c91-8d88-1de8c943545f
name: ssh
type: f2dd2df0-9c04-4b84-a91e-71437ac229f1
data: {
          "hostname": "ssh.mycompany.com",
          "port": 2022
      }

$ ziti edge controller delete config ssh
Found configs with id 83a1e815-04bc-4c91-8d88-1de8c943545f for name ssh

$ ziti edge controller list configs
$ 
```

### Service Configuration
The DNS block, which included hostname and port, has been removed from service definitions. When creating or updating
services, you can submit a `configs` array, which may include config ids or names (or a mix of the two). Configs are
not required.

**NOTE**: Only one config of a given type may be associated with a service.

Configurations associated with a service may be listed as entities using:

    * List associated configs GET `/services/<config-id>/configs`

#### Retrieving service configuration
When authenticating, a user may now indicate which config types should be included when listing services.
The authentication POST may include a body. If the body has a content-type of application/json, it will
be parsed as a map. The controller will looking for a key at the top level of the map called `configTypes`,
which should be an array of config type ids or names (or mix of the two).

Example authentication POST body:
```json
{
    "configTypes" : ["ziti-tunneler-client.v1", "ziti-tunneler-client.v2"]
}
```
When retrieving services, the config data for for those configuration types that were requested will be embedded in
the service definition. For example, if the user has requested (by name) the config types "ziti-tunneler-client.v1" and
"ziti-tunneler-server.v1" and the `ssh` service has configurations of both of those kinds associated, a listing which
includes that service might look as follows:

```json
{
    "meta": {
        "filterableFields": [
            "id",
            "createdAt",
            "updatedAt",
            "name",
            "dnsHostname",
            "dnsPort"
        ],
        "pagination": {
            "limit": 10,
            "offset": 0,
            "totalCount": 1
        }
    },
    "data": [
        {
            "id": "2e79d56a-e37a-4f32-9769-f934976843d9",
            "createdAt": "2020-01-23T20:08:58.634275277Z",
            "updatedAt": "2020-01-23T20:08:58.634275277Z",
            "_links": {
                "edge-routers": {
                    "href": "./services/2e79d56a-e37a-4f32-9769-f934976843d9/edge-routers"
                },
                "self": {
                    "href": "./services/2e79d56a-e37a-4f32-9769-f934976843d9"
                },
                "service-policies": {
                    "href": "./services/2e79d56a-e37a-4f32-9769-f934976843d9/identities"
                }
            },
            "tags": {},
            "name": "ssh",
            "endpointAddress": "tcp:localhost:22",
            "egressRouter": "cf5d76cb-3fff-4dce-8376-60b2bfb505a6",
            "edgeRouterRoles": null,
            "roleAttributes": null,
            "permissions": [
                "Dial"
            ],
            "config": {
                "ziti-tunneler-client.v1": {
                    "hostname": "ssh.mycompany.com",
                    "port": 22
                },
                "ziti-tunneler-server.v1": {
                    "protocol" : "tcp",
                    "hostname": "ssh.mycompany.com",
                    "port": 22
                }
            }
        }
    ]
}
```

### Identity Service Configuration
Configuration for a service can also be specified for a given identity. If a configuration is specified for a service,
it will replace any configuration of that type on that service.

    * Endpoint /identities/<identityId/service-configs
    * Supported operations
        * GET returns the array of  
        * POST will add or update service configurations for the identity
            * If a configuration has the same type as another configuration on the same service, it will replace it
        * DELETE 
            * if given an array of service configs, will delete any matching entries
            * If given an empty body or empty array, all service configurations will be removed from the identity
    * Data Format all operations take or return an array of objects with service and config parameters
        * service may be a service name or id. If there are id and name collisions, id will take precedence
        * config may be a config name or id. If there are id and name collisions, id will take precedence
        * Ex: [{"service": "ssh", "config"  : "my-custom-ssh-config" }]

## Policy Changes
### Syntax Changes
   1. Roles are now prefixed with `#` instead of `@`
   1. Ids previously did not require a prefix. They now require an `@` prefix
   1. Entities could previously only be referenced by id. They can now also be referenced by name.
   1. Like ids, names must be prefixed with `@`. Entity references will first be check to see if they are a name. If no name is found then they are treated as ids.

### Entity Reference by Name
Previously, entities could be referenced in policies by id. They can now also be referenced by name, using the same
syntax. So a service named "ssh" can be referenced as `@ssh`. If the entity is renamed, the policy will be updated
with the updated name.

If a reference matches both a name and an ID, the ID will always take precedence.

### `Any Of` Semantics
Previously polices operated using 'all of' semantics. In other words, to match a policy, an entity had to have ALL OF
the role attributes specified by the policy or be listed explicitly by id.

Edge Router and Service policies now have a new field `semantics`, which may have values of `AnyOf` or `AllOf`. If no
value is provided, it will default to the original behavior of `AllOf`. If `AnyOf` is provided then an entity will match
if it matches any of the roles listed, or if it is listed explicitly by id or name.

**NOTE**
Because service edgeRouterRoles are not broken out into a separate policy entity, they do not support `AnyOf` semantics.

### `#All` limitations
Because having #all grouped with other roles or entity references doesn't make any sense, `#all` policies must now be
created with no other roles or entity references.

### Service Edge Router Policy
Previously services could be confgured with edge router roles, which limited which edge routers could be used to dial
or bind the service.

In 0.9 that is replaced with a new standalone type: service edge router policies. A service edge router policy has three attributes:

  * Name
  * Service Roles
  * Edge Router Roles

An service can be a member of multiple policies and will have access to the union of all edge routers linked to from those policies.

There is a new `/service-edge-router-policies` endpoint which can be used for creating/updating/deleting/querying service edge router policies. Service edge router policies PUT/POST/PATCH all take the following properties:

  * name
  * edgeRouterRoles
  * serviceRoles
  * tags

#### IMPORTANT NOTES
    1. Previously edge router roles on service could be left blank, and the service would be allowed access to all edge routers. Now, a service must be included in at least one service edge router policy or it cannot be dialed or bound.
    1. The set of edge routers an identity can use to dial/bind a service is the intersection of the edge routers that the identity has access to via edge router policies and the edge routers that the service has access to via service edge router policies 

#### CLI Updates
The CLI now has
    # create service-edge-router-policy
    # list service-edge-router-policies
    # list service-edge-router-policy services
    # list service-edge-router-policy edge-routers
    # list services service-edge-router-policies
    # list edge-router service-edge-router-policies
    # delete service-edge-router-policy

## Session Types
Previously when creating a session a flag named `hosting` was provided to indicate if this was a Dial or Bind session.
Now a field named `type` should be provided instead with `Dial` and `Bind` being accepted values. If no value is provided
it will default to `Dial`.

Ex:
```json
    {
        "serviceId" : "a5a0f6af-c833-4961-be0a-c7fb093bb11e",
        "type" : "Dial"
    }
```

Similarly, when sessions were listed, they had a `hosting` flag, which has been replaced by a `type` flag.

**NOTE**: Finally when sessions are transmitted between the controller and edge router, the format has also switched from using
a hosting flag to a type field. This means that controllers and edge routers will **not inter-operate** across the the 0.9
version boundary.


# Release 0.8
## Theme
 * Ziti 0.8.0 replaces appwans with role attribute based service policies
 * Ziti 0.8.0 consolidates dial and bind permissions into service policies

## Service Policy
In 0.7.0 and prior access to services was controlled by appwans.

  * Appwans had lists of identities and services
  * Identities and services could be associated with 0-n appwans
  * Services had explicit lists of identities that could bind the service
  * In order to dial a service, the identity had to be an admin or be in at least one appwan with that service
  * In order to bind a serivice, the identity had to be able to dial the service and be in the list of identities allowed to bind the service

Release 0.8.0 replaces this model with something new. It has the following goals:

  * Allow grouping identities and services dynamically using role attributes rather than hard-coded lists
  * Consolidate dial and bind permissions into the same model

The following concepts were introduced in 0.7 for edge router policies. They are now used for service policies as well.

  * Role attributes
     * Role attributes are just a set of strings associated to a model entity
     * The semantics of the role attributes are determined by the system administrator
     * Ex: an edge router might have the role attributes `["us-east", "new-york", "omnicorp"]`
     * These tags might indicate that this edge router is located on the east coast of the USA, specifically in New York and should be dedicated to use by a customer named OmniCorp
     * Currently role attributes are supported on edge routers and identities
  * Roles
     * Roles specify a set of entities
     * Roles may include role attributes as well as entity ids
     * A role will match all entities which either:
         * Have **_all_** role attributes in the role OR
         * Have an ID which is listed explicitly
     * Role attributes are prefixed with `@`. Role elements not prefixed with `@` are assumed to be ids
     * There is a special role attribute `@all` which will match all entities
     * A role may have only role attributes or only ids or may have both

### Role Example
  * Service with id 1 has role attributes `["sales", "New York City"]`
  * Service with id 2 has role attributes `["sales", "Albany"]`
  * Service with id 3 has role attributes `["support", "Los Angeles"]`
  * A service role of `["@sales", "@New York City", "3"]` would evaluate as follows
     * Service 1 would match because it has all listed role attributes
     * Service 2 would not match, because it doesn't have all listed role attributes
     * Service 3 would match because its ID is listed explicitly

### Model Changes
#### Session Names
  1. api sessions had two endpoints in 0.7, `/api-sessions` and `/sessions` which was deprecated. `/sessions` is now no longer valid for api sessions
  2. sessions used the `/network-sessions` endpoint. In this release, `/network-sessions` has been deprecated and `/sessions` should be used instead.
  3. `/current-session` is now `/current-api-session`

#### Session Format
  1. When creating a session, the returned JSON has the same base format as when listing sessions, so it now includes the service and api-session information. The only difference is that the session token is also returned from session create, but not when listing sessions.
  1. The gateways attribute of session has been renamed to edgeRouters.

#### Role Attributes
Services now have a roleAttributes field. Identities already had a roleAttributes field, for used with edge router policies.

#### Service Policies
0.8.0 introduces a new model construct, the Service Policy. This entity allows restricting which services identities are allowed to dial or bind. A service policy has four attributes:

  * Name
  * Policy Type ("Bind" or "Dial")
  * Identity Roles
  * Service Roles

An identity can be a member of multiple policies and will have access to the union of all services linked to from those policies.

There is a new `/service-policies` endpoint which can be used for creating/updating/deleting/querying service policies. Service policies PUT/POST/PATCH all take the following properties:

  * name
  * type
      * valid values are "Bind" and "Dial"
  * identityRoles
  * serviceRoles
  * tags

There are also new association endpoints allowing the listing of services and identities associated with service policies and vice-versa.

  * /service-policies/<id>/services
  * /service-policies/<id>/identities
  * /identities/<id>/service-policies
  * /services/<id>/service-policies

#### Service Access
  * An admin may dial or bind any service
  * A non-admin identity may dial any service it has access to via service policies of type "Dial"
  * A non-admin identity may bind any service it has access to via service policies of type "Bind"

When listing services, the controller used to provide a hostable flag with each service to indicate if the service could be bound in addition to being dialed. Now, the service will have a permissions block which will indicate if the service may be dialed, bound or both.

Ex:
```json
        {
            "meta": {},
            "data": {
                "id": "1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba",
                "createdAt": "2020-01-04T02:34:00.788444359Z",
                "updatedAt": "2020-01-04T02:34:00.788444359Z",
                "_links": {
                    "edge-routers": {
                        "href": "./services/1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba/edge-routers"
                    },
                    "self": {
                        "href": "./services/1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba"
                    },
                    "service-policies": {
                        "href": "./services/1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba/identities"
                    }
                },
                "tags": {},
                "name": "cac9593c-0494-4800-9f70-c258ff28a702",
                "dns": {
                    "hostname": "0bf71754-ed5b-4b2d-9adf-a542f1284275",
                    "port": 0
                },
                "endpointAddress": "4662d564-3fc3-4f10-b8cd-ee0e3629ad24",
                "egressRouter": "aedab92f-2ddf-445a-9194-73d428322a34",
                "edgeRouterRoles": null,
                "roleAttributes": [
                    "2c68789a-fe71-4d25-a483-43e54ee4fd98"
                ],
                "permissions": [
                    "Bind"
                ]
            }
        }
```

#### Appwan Removal and Migration
The `/app-wans` endpoint has been removed. The bbolt schema version has been bumped to 3. If starting a fresh controller no action will be taken. However, if coming from an existing 0.7 or earlier bbolt database, the following will be done:

  1. For each existing appwan, a service policy with type "Dial" will be created
  1. The new service policy will have the same name as the appwan it replaces
  1. The new service policy will have the same identities and services as the appwan it replaces
  1. Identities and services will be specified explicitly by ID rather as opposed to by creating new role attributes

NOTE: Service hosting identities will not be migrated into equivalent Bind service policies, as binds are not yet used in any production scenarios.

## Go SDK changes
Several types have been renamed to conform to standard nomenclature

  * Session is now ApiSession
  * NetworkSession is now Session
     * The SessionId field is now ApiSessionId
     * The Gateways field is now EdgeRouters
  * Gateway is now EdgeRouter
  * On the Service type the Hostable flag has been removed and replaced with a Permissions string array
      * It may be nil, empty or contain either or both of "Dial" and "Bind"
  * On the Context type
      * GetNetworkSession is now GetSession
      * GetNetworkHostSession is now GetBindSession

## ziti command line changes
  1. The `ziti edge controller create/delete gateway` commands have been removed. Use `ziti edge controller create/delete edge-router` instead.
  2. There are new `ziti edge controller create/delete service-policy` commands

## Ziti Proxy changes
ziti-proxy has been incorporated into the ziti-tunnel command. Where previously one would have run

```
ZITI_SDK_CONFIG=./config.json ziti-proxy run <proxied services>
```

now one should use

```
ziti-tunnel proxy -i ./config.json <proxied services>
```

# Release 0.7
## Theme
 * Ziti 0.7.0 replaces clusters with role attribute based policies
 * Ziti 0.7.0 takes steps towards consistent terminology for sessions

## Edge Router Policy
In 0.6.0 access to edge routers was controlled by clusters and services.

  * Every edge router was assigned to a cluster
  * Services belonged to 1 or more clusters
  * Dial/bind request would results would include a list of edge routers which were
      * in clusters linked to the dialed/bound service and
      * were online when the request was made

Release 0.7.0 replaces this model with something new. It has the following goals:

  * Allow grouping edge routers and other entities dynamically using role attributes rather than hard-coded lists
  * Allow restricting access to edge router by identity in addition to by service

It includes the following new concepts:

  * Role attributes
     * Role attributes are just a set of strings associated to a model entity
     * The semantics of the role attributes are determined by the system administrator
     * Ex: an edge router might have the role attributes `["us-east", "new-york", "omnicorp"]`
     * These tags might indicate that this edge router is located on the east coast of the USA, specifically in New York and should be dedicated to use by a customer named OmniCorp
     * Currently role attributes are supported on edge routers and identities
  * Roles
     * Roles specify a set of entities
     * Roles may include role attributes as well as entity ids
     * A role will match all entities which either:
         * Have **_all_** role attributes in the role OR
         * Have an ID which is listed explicitly
     * Role attributes are prefixed with `@`. Role elements not prefixed with `@` are assumed to be ids
     * There is a special role attribute `@all` which will match all entities
     * A role may have only role attributes or only ids or may have both

### Role Example
  * Edge router with id 1 has role attributes `["us-east", "New York City"]`
  * Edge router with id 2 has role attributes `["us-east", "Albany"]`
  * Edge router with id 3 has role attributes `["us-west", "Los Angeles"]`
  * An edge router role of `["@us-east", "@New York City", "3"]` would evaluate as follows
     * Edge router 1 would match because it has all listed role attributes
     * Edge router 2 would not match, because it doesn't have all listed role attributes
     * Edge router 3 would match because its ID is listed explicitly

### Model Changes
#### Role Attributes
Edge routers and identities now have roleAttributes fields. Edge routers no longer have an associated cluster.

#### Edge Router Policies
0.7.0 introduces a new model construct, the Edge Router Policy. This entity allows restricting which edge routers identities are allowed to use. An edge router policy has three attributes:

  * Name
  * Identity Roles
  * Edge Router Roles

An identity can be a member of multiple policies and will have access to the union of all edge routers linked to from those policies.

There is a new `/edge-router-policies` endpoint which can be used for creating/updating/deleting/querying edge router policies. Edge router policies PUT/POST/PATCH all take the following properties:

  * name
  * edgeRouterRoles
  * identityRoles
  * tags

#### Service Edge Router Roles
Services now have a new edgeRouterRoles field. If set, this specifies which edge routers may be used for a service. This replaces the old cluster functionality.

#### Edge Router Access
When a service is dialed or bound, which edge routers will be returned?

  * If the service edgeRouterRoles are NOT set, then it will be the set of edge routers to which the dialing/binding identity has access
  * If the service edgeRouterRoles ARE set, then it will be the intersection of the edge routers to which the service has access and the set of edge routers to which the identity has access

#### Cluster Removal and Migration
The `/clusters` endpoint has been removed. The bbolt schema version has been bumped to 2. If starting a fresh controller no action will be taken. However, if coming from an existing 0.6 or earlier bbolt database, the following will be done:

  1. An edge router policy will be created with `@all` for both identityRoles and edgeRouterRoles, allowing access to all edge routers from all identities. This will allow the current identities to continue using the system. Otherwise, no identities would be able to connect to any edge routers.
  2. Each edge router will get a role attribute of `cluster-<cluster name>` for the cluster it belonged to
  3. If a service belongs to 1 or more clusters it will get a role attribute corresponding to the first cluster. Any edge routers assigned to additional clusters will be added to edge router roles field by ID.
      1. Noe: If we were to add additional role clusters for the other clusts we'd get the intersection, not the union and would end up with access to 0 edge routers

## Session changes
Terminology related to sessions is being made consistent between the edge and fabric.

There are two types of sessions:

  1. Sessions between edge clients the edge controller, which allowed clients to manage controller state as well as dial and bind services
      1. These were referred to as sessions in the edge and have no fabric equivalent
  1. Sessions which establish routing and allow data flow to/from/within the edge and fabric
      1. These were referred to as network sessions in the edge and sessions in the fabric

Going forward, what was called a session in the edge will now be referred to as an API session. What was called a network session will be now just be called session in both the edge and fabric.

As a first step, in 0.7.0 API sessions will be available at both the `/sessions` and `/api-sessions` endpoints. Use of the `/sessions` endpoint is deprecated. In later releases the `/sessions` endpoint will be used for sessions instead of API sessions.

# Release 0.6
## Theme
Ziti 0.6.0 moves the back-end persistence model of Ziti Edge and Ziti Fabric into the same repository based on Bbolt (an in memory data store that is backed by a memory mapped file). The changes remove the requirement for PostgresSQL.

## UPDB Enrollment JWTs
Enrollments that are for UPDB (username password database) are now consistent with all other enrollment and use JWTs for processing. Prior to this a naked URL was provided.

### What This Breaks
Any UPDB enrollment processing that relied upon the URL for the enrollment.



Ziti 0.5.x UPDB enrolling entity

```
{
    "meta": {},
    "data": {
        "id": "612843ae-6ac8-48ac-a737-bfc2d28ab9ea",
        "createdAt": "2019-11-21T17:23:00.316631Z",
        "updatedAt": "2019-11-21T17:23:00.316631Z",
        "_links": {
            "self": {
                "href": "./identities/612843ae-6ac8-48ac-a737-bfc2d28ab9ea"
            }
        },
        "tags": {},
        "name": "updb--5badbdc5-e8dd-4877-82df-c06aea7f1197",
        "type": {
            "id": "577104f2-1e3a-4947-a927-7383baefbc9a",
            "name": "User"
        },
        "isDefaultAdmin": false,
        "isAdmin": false,
        "authenticators": {},
        "enrollment": {
            "updb": {
                "username": "asdf",
                "url": "https://demo.ziti.netfoundry.io:1080/enroll?method=updb&token=911e6562-0c83-11ea-a81a-000d3a1b4b17&username=asdf"
            }
        },
        "permissions": []
    }
}
```

Ziti 0.6.x UPDB enrolling entity (note the changes in the enrollment.updb object):

```
{
    "meta": {},
    "data": {
        "id": "39f11c10-0693-41ed-9bec-8011e2721562",
        "createdAt": "2019-11-21T17:28:18.2855234Z",
        "updatedAt": "2019-11-21T17:28:18.2855234Z",
        "_links": {
            "self": {
                "href": "./identities/39f11c10-0693-41ed-9bec-8011e2721562"
            }
        },
        "tags": {},
        "name": "updb--b55f5372-3993-40f5-b534-126e0dd2f1be",
        "type": {
            "entity": "identity-types",
            "id": "577104f2-1e3a-4947-a927-7383baefbc9a",
            "name": "User",
            "_links": {
                "self": {
                    "href": "./identity-types/577104f2-1e3a-4947-a927-7383baefbc9a"
                }
            }
        },
        "isDefaultAdmin": false,
        "isAdmin": false,
        "authenticators": {},
        "enrollment": {
            "updb": {
                "expiresAt": "2019-11-21T17:33:18.2855234Z",
                "issuedAt": "2019-11-21T17:28:18.2855234Z",
                "jwt": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbSI6InVwZGIiLCJleHAiOjE1NzQzNTc1OTgsImlzcyI6Imh0dHBzOi8vbG9jYWxob3N0OjEyODAiLCJqdGkiOiJiYzBlY2NlOC05ZGY0LTQzZDYtYTVhMC0wMjI1MzY2YmM4M2EiLCJzdWIiOiIzOWYxMWMxMC0wNjkzLTQxZWQtOWJlYy04MDExZTI3MjE1NjIifQ.PUcnACCdwqfWRGRzF8lG6xDTgHKAwKV6eTw8tHFuNBXaUNbqExBwUQEW0-cCHsV-nLEyhxyjhXmVCkIDgz-ukKfS0xStiDrJQbiq8m0auodkArmJSsYzElXkKdv37FHu0t-CGoXptdLyuo9eCnzzmci3ev18zMR5HjYMCQEclELV6OEICNr_0EwhAGJa1yX6ODYrLMZ3SdEd6fj-ZGX7j9owTs6iEsqCB_TORfnGGg6lEINE5GlYsyp7JUxolS6H4lPeN5h2mxk2_OkJY8GX3ydv75LsIZ-jjL3xC5XncCESrefgDabib1fudJ4038D0EzqTcOREPAqmjWhnDhTulQ",
                "token": "bc0ecce8-9df4-43d6-a5a0-0225366bc83a"
            }
        },
        "permissions": []
    }
}
```



### What To Do
Use the new JWT format to:

verify the signature of the JWT to match the iss URL's TSL presented certificates
construct the enrollment url from the JWTs properties in the following format:

```
<iss> + "/enroll?token=" + <jti>
```

## Multiple Invalid Value Error Handling
Errors where there is the potential to report about multiple invalid field values for a given field used to report as a separate error for each value. Now there will be one error, but the values field will hold the invalid values.

### Old Format
```
{
    "error": {
        "args": {
            "urlVars": {
                "id": "097018b6-108e-42b3-869b-deb9e1814594"
            }
        },
        "cause": {
            "errors": [
                {
                    "message": "entity not found for id [06ecf930-3a9f-4a6c-98b5-8f0be1bde9e2]",
                    "field": "ids[0]",
                    "value": "06ecf930-3a9f-4a6c-98b5-8f0be1bde9e2"
                }
            ]
        },
        "causeMessage": "There were multiple field errors: the value '06ecf930-3a9f-4a6c-9...' for 'ids[0]' is invalid: entity not found for id [06ecf930-3a9f-4a6c-98b5-8f0be1bde9e2]",
        "code": "INVALID_FIELD",
        "message": "The field contains an invalid value",
        "requestId": "48ea4bce-f233-410e-a062-5dbceee20223"
    },
    "meta": {
        "apiEnrolmentVersion": "0.0.1",
        "apiVersion": "0.0.1"
    }
}
```

### New Format
```
{
    "error": {
        "args": {
            "urlVars": {
                "id": "5b15c442-5590-4c58-8bc7-0da788e0cfcf"
            }
        },
        "cause": {
            "message": "clusters(s) not found",
            "field": "clusters",
            "value": [
                "68f8739f-cf52-4d51-9553-dfe7cf9c6a03"
            ]
        },
        "causeMessage": "the value '[68f8739f-cf52-4d51-9553-dfe7cf9c6a03]' for 'clusters' is invalid: clusters(s) not found",
        "code": "INVALID_FIELD",
        "message": "The field contains an invalid value",
        "requestId": "ab6553e8-e9b1-408c-9fed-11cc627cfd84"
    },
    "meta": {
        "apiEnrolmentVersion": "0.0.1",
        "apiVersion": "0.0.1"
    }
}
```
