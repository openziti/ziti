# Release 0.21.0

## Semantic now Required for policies (BREAKING CHANGE)
Previouxly semantic was optional when creating or updating policies (POST or PUT), defaulting to `AllOf` when not specified. It is now required.

## What's New

* Bug fix: Using PUT for policies without including the semantic would cause them to be evaluated using the AllOf semantic
* Bug fix: Additional concurrency fix in posture data
* Feature: Ziti CLI now supports a comprehensive set of `ca` and `cas` options
* Feature: `ziti ps` now supports `set-channel-log-level` and `clear-channel-log-level` operations
* Change: Previouxly semantic was optional when creating or updating policies (POST or PUT), defaulting to `AllOf` when not specified. It is now required.


# Release 0.20.14

## What's New

* Bug fix: Posture timeouts (i.e. MFA timeouts) would not apply to the first session of an API session
* Bug fix: Fix panic during API Session deletion
* Bug fix: DNS entries in embedded DNS server in go tunneler apps were not being cleaned up
* Feature: Ziti CLI now supports attribute updates on MFA posture checks
* Feature: Posture queries now support `timeout` and `timeoutRemaining`

# Release 0.20.13

## What's New

* Bug fix: [edge#712](https://github.com/openziti/edge/issues/712) 
  * NF-INTERCEPT chain was getting deleted when any intercept was stopped, not when all intercepts were stopped
  * IP address could get re-used across DNS entries. Added DNS cache flush on startup to avoid this
  * IP address cleanup was broken as all services would see last assigned IP
* Bug fix: Introduce delay when closing xgress peer after receiving unroute if end of session not yet received
* Feature: Can now search relevant entities by role attributes
  * Services, edge routers and identities can be search by role attribute. Ex: `ziti edge list services 'anyOf(roleAttributes) = "one"'`
  * Polices can be searched by roles. Ex: `ziti edge list service-policies 'anyOf(identityRoles) = "#all"'`

# Release 0.20.12

## What's New

* Bug fix: [edge#641](https://github.com/openziti/edge/issues/641)Management and Client API nested resources now support `limit` and `offset` outside of `filter` as query params
* Feature: MFA Timeout Options

## MFA Timeout Options

The MFA posture check now supports three options:

* `timeoutSeconds` - the number of seconds before an MFA TOTP will need to be provided before the posture check begins to fail (optional)
* `promptOnWake` - reduces the current timeout to 5m (if not less than already) when an endpoint reports a "wake" event (optional)
* `promptOnUnlock` - reduces the current timeout to 5m (if not less than already) when an endpoint reports an "unlock" event (optional)
* `ignoreLegacyEndpoints` - forces all other options to be ignored for legacy clients that do not support event state (optional)

Event states, `promptOnWake` and `promptOnUnlock` are only supported in Ziti C SDK v0.20.0 and later. Individual ZDE/ZME clients
may take time to update. If older endpoint are used with the new MFA options `ignoreLegacyEndpoints` allows administrators to decide
how those clients should be treated. If `ignoreLegacyEndpoints` is `true`, they will not be subject to timeout or wake events.

# Release 0.20.11

* Bug fix: CLI Admin create/update/delete for UPDB authenticators now function properly
* Maintenance: better logging [sdk-golang#161](https://github.com/openziti/sdk-golang/pull/161) and [edge#700](https://github.com/openziti/edge/pull/700)
* Bug fix: [sdk-golang#162](https://github.com/openziti/sdk-golang/pull/162) fix race condition on close of ziti connections

# Release 0.20.10

## What's New

* Bug fix: patch for process multi would clear information
* Bug fix: [ziti#420](https://github.com/openziti/ziti/issues/420) fix ziti-tunnel failover with multiple interfaces when once becomes unavailable
* Bug fix: [edge#670](https://github.com/openziti/edge/issues/670) fix ziti-tunnel issue where address were left assigned to loopback after clean shutdown
* Bug fix: race condition in edge session sync could cause router panic. Regression since 0.20.9
* Bug fix: terminator updates and deletes from the combined router/tunneler weren't working
* Feature: Router health checks
* Feature: Controller health check

## Router Health Checks

Routers can now enable an HTTP health check endpoint. The health check is configured in the router config file with the new `healthChecks` section. 

```
healthChecks:
    ctrlPingCheck:
        # How often to ping the controller over the control channel. Defaults to 30 seconds
        interval: 30s
        # When to timeout the ping. Defaults to 15 seconds
        timeout: 15s
        # How long to wait before pinging the controller. Defaults to 15 seconds
        initialDelay: 15s
```

The health check endpoint is configured via XWeb, same as in the controller. As section like the following can be added to the router config to enable the endpoint.

```
web:
  - name: health-check
    bindPoints:
      - interface: 127.0.0.1:8081
        address: 127.0.0.1:8081
    apis:
      - binding: health-checks
```

The health check output will look like this:

```
$ curl -k https://localhost:8081/health-checks
{
    "data": {
        "checks": [
            {
                "healthy": true,
                "id": "controllerPing",
                "lastCheckDuration": "767.381µs",
                "lastCheckTime": "2021-06-21T16:22:36-04:00"
            }
        ],
        "healthy": true
    },
    "meta": {}
}

```

The endpoint will return a 200 if the health checks are passing and 503 if they are not.

# Controller Health Check
Routers can now enable an HTTP health check endpoint. The health check is configured in the router config file with the new `healthChecks` section. 

```
healthChecks:
    boltCheck:
        # How often to check the bolt db. Defaults to 30 seconds
        interval: 30s
        # When to timeout the bolt db check. Defaults to 15 seconds
        timeout: 15s
        # How long to wait before starting bolt db checks. Defaults to 15 seconds
        initialDelay: 15s
```

The health check endpoint is configured via XWeb. In order to enable the health check endpoint, add it **first** to the list of apis.

```
    apis:
      # binding - required
      # Specifies an API to bind to this webListener. Built-in APIs are
      #   - edge-management
      #   - edge-client
      #   - fabric-management
      - binding: health-checks
        options: { }
      - binding: edge-management
        # options - variable optional/required
        # This section is used to define values that are specified by the API they are associated with.
        # These settings are per API. The example below is for the `edge-api` and contains both optional values and
        # required values.
        options: { }
      - binding: edge-client
        options: { }

```

The health check output will look like this:

```
$ curl -k https://localhost:1280/health-checks
{
    "data": {
        "checks": [
            {
                "healthy": true,
                "id": "bolt.read",
                "lastCheckDuration": "27.46µs",
                "lastCheckTime": "2021-06-21T17:32:31-04:00"
            }
        ],
        "healthy": true
    },
    "meta": {}
}

```

# Release 0.20.9

## What's New

* Bug fix: router session sync would fail if it took longer than a second
* Bug fix: API sessions created during session sync could get thrown out when session sync was finalized
* Bug fix: Update of identity defaultHostingCost and defaultHostingPrecedence didn't work
* Improvement: List identities is faster as it no longer always iterates through all api-sessions
* Improvement: API Session enforcer now batches deletes of session for better performance

# Release 0.20.8

## What's New

* 0.20.7 was missing the most up-to-date version of the openziti/edge library dependency

# Release 0.20.7

## What's New

* Xlink now supports to a boolean `split` option to enable/disable separated payload and ack channels.
* Router identity now propagated through the link establishment plumbing. Will facilitate router-directed `transport.Configuration` profiles in a future release.
* Bug fix: tunneler identity appData wasn't propagated to tunneler/router
* Bug fix: API session updates were only being sent to one router (regression since 0.20.4)
* Bug fix: API session enforcer wasn't being started (regression since 0.20.0)
* Bug fix: Setting per identity service costs/precedences didn't work with PATCH

### Split Xlink Payload/Ack Channels

Split payload and ack channels are enabled by default, preserving the behavior of previous releases. To disable split channels, merge the following stanza into your router configuration:

```
link:
  dialers:
    - binding:              transport
      split:                false
```

# Release 0.20.6

## What's New

* Bug fix: Revert defensive Edge Router disconnect protection in Edge

# Release 0.20.5

## What's New

* Bug fix: Fix panic on double chan close that can occur when edge routers disconnect/reconnect in rapid succession
* Bug fix: Fix defaults for enrollment durations when not specified (would default near 0 values)

# Release 0.20.4

## What's New

* Bug fix: Fix a deadlock that can occur if Edge Routers disconnect during session synchronization or update processes
* Bug fix: Fix URL for CAS create in Ziti CLI

# Release 0.20.3

## What's New

* Bug fix: Update of identity appData wasn't working
* Bug fix: Terminator updates failed if cost wasn't specified
* Bug fix: Control channel handler routines were exiting on error instead of just closing peer and continuing

# Release 0.20.2

## What's New

* ziti-router will now emit a stackdump before exiting when it receives a SIGQUIT
* ziti ps stack now takes a --stack-timeout and will quit after the specified timeout if the stack dump hasn't completed yet
* ziti now supports posture check types of process multi
* Fixes a bug in Ziti Management API where posture checks of type process multi were missing their base entity information (createdAt, updatedAt, etc.)

# Release 0.20.1

## What's New

* Fixes a bug in the GO sdk which could cause panic by return nil connection and nil error
* [ziti#170](https://github.com/openziti/ziti/issues/170) Fixes the service poll refresh default for ziti-tunnel host mode
* Fixes a deadlock in control channel reconnect logic triggerable when network path to controller is unreliable

# Release 0.20.0

## What's New

* Fix bug in router/tunneler where only first 10 services would get picked up for intercepting/hosting
* Fix bug in router/tunneler where we'd process services multiple times on service add/remove/update
* Historical Changelog Split
* Edge Management REST API Transit Router Deprecation
* Edge REST API Split & Configuration Changes

### Historical Changelog Split

Changelogs for previous minor versions are now split into their own files
under `/changelogs`.

### Edge Management REST API Transit Router Deprecation

The endpoint `/transit-routers` is now `/routers`. Use of the former name
is considered deprecated. This endpoint only affects the new Edge Management API.

### Edge REST API Split

The Edge REST API has now been split into two APIs: The Edge Client API and the Edge Management API.
There are now two Open API 2.0 specifications present in the `edge` repository under `/specs/client.yml`
and `/specs/management.yml`. These two files are generated (see the scripts in `/scripts/`) from decomposed
YAML source files present in `/specs/source`.

The APIs are now hosted on separate URL paths:

- Client API: `/edge/client/v1`
- Management API: `/edge/management/v1`

Legacy path support is present for the Client API only. The Management API does not support legacy
URL paths. The Client API Legacy paths that are supported are as follows:

- No Prefix: `/*`
- Edge Prefix: `/edge/v1/*`

This support is only expected to last until all Ziti SDKs move to using the new prefixed paths and versions
that do not reach the end of their lifecycle. After that time, support will be removed. It is highly  
suggested that URL path prefixes be updated or dynamically looked up via the `/version` endpoint (see below)

#### Client and Management API Capabilities

The Client API represents only functionality required by and endpoint to
connected to and use services. This API services Ziti SDKs.

The Management API represents all administrative configuration capabilities.
The Management API is meant to be used by the Ziti Admin Console (ZAC) or
other administrative integrations.

*Client API Endpoints*
  - `/edge/client/v1/`
  - `/edge/client/v1/.well-known/est/cacerts`
  - `/edge/client/v1/authenticate`
  - `/edge/client/v1/authenticate/mfa`
  - `/edge/client/v1/current-api-session`
  - `/edge/client/v1/current-api-session/certificates`
  - `/edge/client/v1/current-api-session/certificates/{id}`
  - `/edge/client/v1/current-api-session/service-updates`
  - `/edge/client/v1/current-identity`
  - `/edge/client/v1/current-identity/authenticators`
  - `/edge/client/v1/current-identity/authenticators/{id}`
  - `/edge/client/v1/current-identity/edge-routers`
  - `/edge/client/v1/current-identity/mfa`
  - `/edge/client/v1/current-identity/mfa/qr-code`
  - `/edge/client/v1/current-identity/mfa/verify`
  - `/edge/client/v1/current-identity/mfa/recovery-codes`
  - `/edge/client/v1/enroll`
  - `/edge/client/v1/enroll/ca`
  - `/edge/client/v1/enroll/ott`
  - `/edge/client/v1/enroll/ottca`
  - `/edge/client/v1/enroll/updb`
  - `/edge/client/v1/enroll/erott`
  - `/edge/client/v1/enroll/extend/router`
  - `/edge/client/v1/posture-response`
  - `/edge/client/v1/posture-response-bulk`
  - `/edge/client/v1/protocols`
  - `/edge/client/v1/services`
  - `/edge/client/v1/services/{id}`
  - `/edge/client/v1/services/{id}/terminators`
  - `/edge/client/v1/sessions`
  - `/edge/client/v1/sessions/{id}`
  - `/edge/client/v1/specs`
  - `/edge/client/v1/specs/{id}`
  - `/edge/client/v1/specs/{id}/spec`
  - `/edge/client/v1/version`

*Management API Endpoints*
  - `/edge/management/v1/`
  - `/edge/management/v1/api-sessions`
  - `/edge/management/v1/api-sessions/{id}`
  - `/edge/management/v1/authenticate`
  - `/edge/management/v1/authenticate/mfa`
  - `/edge/management/v1/authenticators`
  - `/edge/management/v1/authenticators/{id}`
  - `/edge/management/v1/cas`
  - `/edge/management/v1/cas/{id}`
  - `/edge/management/v1/cas/{id}/jwt`
  - `/edge/management/v1/cas/{id}/verify`
  - `/edge/management/v1/config-types`
  - `/edge/management/v1/config-types/{id}`
  - `/edge/management/v1/config-types/{id}/configs`
  - `/edge/management/v1/configs`
  - `/edge/management/v1/configs/{id}`
  - `/edge/management/v1/current-api-session`
  - `/edge/management/v1/current-identity`
  - `/edge/management/v1/current-identity/authenticators`
  - `/edge/management/v1/current-identity/authenticators/{id}`
  - `/edge/management/v1/current-identity/mfa`
  - `/edge/management/v1/current-identity/mfa/qr-code`
  - `/edge/management/v1/current-identity/mfa/verify`
  - `/edge/management/v1/current-identity/mfa/recovery-codes`
  - `/edge/management/v1/database/snapshot`
  - `/edge/management/v1/database/check-data-integrity`
  - `/edge/management/v1/database/fix-data-integrity`
  - `/edge/management/v1/database/data-integrity-results`
  - `/edge/management/v1/edge-router-role-attributes`
  - `/edge/management/v1/edge-routers`
  - `/edge/management/v1/edge-routers/{id}`
  - `/edge/management/v1/edge-routers/{id}/edge-router-policies`
  - `/edge/management/v1/edge-routers/{id}/identities`
  - `/edge/management/v1/edge-routers/{id}/service-edge-router-policies`
  - `/edge/management/v1/edge-routers/{id}/services`
  - `/edge/management/v1/edge-router-policies`
  - `/edge/management/v1/edge-router-policies/{id}`
  - `/edge/management/v1/edge-router-policies/{id}/edge-routers`
  - `/edge/management/v1/edge-router-policies/{id}/identities`
  - `/edge/management/v1/enrollments`
  - `/edge/management/v1/enrollments/{id}`
  - `/edge/management/v1/identities`
  - `/edge/management/v1/identities/{id}`
  - `/edge/management/v1/identities/{id}/edge-router-policies`
  - `/edge/management/v1/identities/{id}/service-configs`
  - `/edge/management/v1/identities/{id}/service-policies`
  - `/edge/management/v1/identities/{id}/edge-routers`
  - `/edge/management/v1/identities/{id}/services`
  - `/edge/management/v1/identities/{id}/policy-advice/{serviceId}`
  - `/edge/management/v1/identities/{id}/posture-data`
  - `/edge/management/v1/identities/{id}/failed-service-requests`
  - `/edge/management/v1/identities/{id}/mfa`
  - `/edge/management/v1/identity-role-attributes`
  - `/edge/management/v1/identity-types`
  - `/edge/management/v1/identity-types/{id}`
  - `/edge/management/v1/posture-checks`
  - `/edge/management/v1/posture-checks/{id}`
  - `/edge/management/v1/posture-check-types`
  - `/edge/management/v1/posture-check-types/{id}`
  - `/edge/management/v1/service-edge-router-policies`
  - `/edge/management/v1/service-edge-router-policies/{id}`
  - `/edge/management/v1/service-edge-router-policies/{id}/edge-routers`
  - `/edge/management/v1/service-edge-router-policies/{id}/services`
  - `/edge/management/v1/service-role-attributes`
  - `/edge/management/v1/service-policies`
  - `/edge/management/v1/service-policies/{id}`
  - `/edge/management/v1/service-policies/{id}/identities`
  - `/edge/management/v1/service-policies/{id}/services`
  - `/edge/management/v1/service-policies/{id}/posture-checks`
  - `/edge/management/v1/services`
  - `/edge/management/v1/services/{id}`
  - `/edge/management/v1/services/{id}/configs`
  - `/edge/management/v1/services/{id}/service-edge-router-policies`
  - `/edge/management/v1/services/{id}/service-policies`
  - `/edge/management/v1/services/{id}/identities`
  - `/edge/management/v1/services/{id}/edge-routers`
  - `/edge/management/v1/services/{id}/terminators`
  - `/edge/management/v1/sessions`
  - `/edge/management/v1/sessions/{id}`
  - `/edge/management/v1/sessions/{id}/route-path`
  - `/edge/management/v1/specs`
  - `/edge/management/v1/specs/{id}`
  - `/edge/management/v1/specs/{id}/spec`
  - `/edge/management/v1/summary`
  - `/edge/management/v1/terminators`
  - `/edge/management/v1/terminators/{id}`
  - `/edge/management/v1/routers`
  - `/edge/management/v1/transit-routers`
  - `/edge/management/v1/routers/{id}`
  - `/edge/management/v1/transit-routers/{id}`
  - `/edge/management/v1/version`


#### XWeb Support & Configuration Changes

The underlying framework used to host the Edge REST API has been moved into a new library
that can be found in the `fabric` repository under the module name `xweb`. XWeb allows arbitrary
APIs and website capabilities to be hosted on one or more http servers bound to any number of
network interfaces and ports.

The main result of this is that the Edge Client and Management APIs can be hosted on separate ports or
even on separate network interfaces if desired. This allows for configurations where the Edge Management
API is not accessible outside of localhost or is only presented to network interfaces that are inwardly facing.

The introduction of XWeb has necessitated changes to the controller configuration. For a full documented example
see the file `/etc/ctrl.with.edge.yml` in this repository.

##### Controller Configuration: Edge Section

The Ziti Controller configuration `edge` YAML section remains as a shared location for cross-API settings. It however,
does not include HTTP settings which are now configured in the `web` section.

Additionally, all duration configuration values must be specified in `<integer><unit>` durations. For example

- "5m" for five minutes
- "100s" for one hundred seconds

```
# By having an 'edge' section defined, the ziti-controller will attempt to parse the edge configuration. Removing this
# section, commenting out, or altering the name of the section will cause the edge to not run.
edge:
  # This section represents the configuration of the Edge API that is served over HTTPS
  api:
    #(optional, default 90s) Alters how frequently heartbeat and last activity values are persisted
    # activityUpdateInterval: 90s
    #(optional, default 250) The number of API Sessions updated for last activity per transaction
    # activityUpdateBatchSize: 250
    # sessionTimeout - optional, default 10m
    # The number of minutes before an Edge API session will timeout. Timeouts are reset by
    # API requests and connections that are maintained to Edge Routers
    sessionTimeout: 30m
    # address - required
    # The default address (host:port) to use for enrollment for the Client API. This value must match one of the addresses
    # defined in this webListener's bindPoints.
    address: 127.0.0.1:1280
  # enrollment - required
  # A section containing settings pertaining to enrollment.
  enrollment:
    # signingCert - required
    # A Ziti Identity configuration section that specifically makes use of the cert and key fields to define
    # a signing certificate from the PKI that the Ziti environment is using to sign certificates. The signingCert.cert
    # will be added to the /.well-known CA store that is used to bootstrap trust with the Ziti Controller.
    signingCert:
      cert: ${ZITI_SOURCE}/ziti/etc/ca/intermediate/certs/intermediate.cert.pem
      key: ${ZITI_SOURCE}/ziti/etc/ca/intermediate/private/intermediate.key.decrypted.pem
    # edgeIdentity - optional
    # A section for identity enrollment specific settings
    edgeIdentity:
      # durationMinutes - optional, default 5m
      # The length of time that a Ziti Edge Identity enrollment should remain valid. After
      # this duration, the enrollment will expire and not longer be usable.
      duration: 5m
    # edgeRouter - Optional
    # A section for edge router enrollment specific settings.
    edgeRouter:
      # durationMinutes - optional, default 5m
      # The length of time that a Ziti Edge Router enrollment should remain valid. After
      # this duration, the enrollment will expire and not longer be usable.
      duration: 5m

```

##### Controller Configuration: Web Section
The `web` section now allows Ziti APIs to be configured on various network interfaces and
ports according to deployment requirements. The `web` section is an array of configuration
that defines `WebListener`s. Each `WebListener` has its own HTTP configuration, `BindPoint`s,
identity override, and `API`s which are referenced by `binding` name.

Each `WebListener` maps to at least one HTTP server that will be bound on at least one `BindPoint`
(network interface/port combination and external address) and will host one or more `API`s defined
in the `api` section. `API`s are configured by `binding` name. The following `binding` names
are currently supported:

- Edge Client API: `edge-client`
- Edge Management API: `edge-management`

An example `web` section that places both the Edge Client and Management APIs on the same
`BindPoint`s would be:

```
# web 
# Defines webListeners that will be hosted by the controller. Each webListener can host many APIs and be bound to many
# bind points.
web:
  # name - required
  # Provides a name for this listener, used for logging output. Not required to be unique, but is highly suggested.
  - name: all-apis-localhost
    # bindPoints - required
    # One or more bind points are required. A bind point specifies an interface (interface:port string) that defines
    # where on the host machine the webListener will listen and the address (host:port) that should be used to
    # publicly address the webListener(i.e. my-domain.com, localhost, 127.0.0.1). This public address may be used for
    # incoming address resolution as well as used in responses in the API.
    bindPoints:
      #interface - required
      # A host:port string on which network interface to listen on. 0.0.0.0 will listen on all interfaces
      - interface: 127.0.0.1:1280
        # address - required
        # The public address that external incoming requests will be able to resolve. Used in request processing and
        # response content that requires full host:port/path addresses.
        address: 127.0.0.1:1280
    # identity - optional
    # Allows the webListener to have a specific identity instead of defaulting to the root `identity` section.
    #    identity:
    #      cert:                 ${ZITI_SOURCE}/ziti/etc/ca/intermediate/certs/ctrl-client.cert.pem
    #      server_cert:          ${ZITI_SOURCE}/ziti/etc/ca/intermediate/certs/ctrl-server.cert.pem
    #      key:                  ${ZITI_SOURCE}/ziti/etc/ca/intermediate/private/ctrl.key.pem
    #      ca:                   ${ZITI_SOURCE}/ziti/etc/ca/intermediate/certs/ca-chain.cert.pem
    # options - optional
    # Allows the specification of webListener level options - mainly dealing with HTTP/TLS settings. These options are
    # used for all http servers started by the current webListener.
    options:
      # idleTimeoutMs - optional, default 5000ms
      # The maximum amount of idle time in milliseconds allowed for pipelined HTTP requests. Setting this too high
      # can cause resources on the host to be consumed as clients remain connected and idle. Lowering this value
      # will cause clients to reconnect on subsequent HTTPs requests.
      idleTimeout: 5000ms  #http timeouts, new
      # readTimeoutMs - optional, default 5000ms
      # The maximum amount of time in milliseconds http servers will wait to read the first incoming requests. A higher
      # value risks consuming resources on the host with clients that are acting bad faith or suffering from high latency
      # or packet loss. A lower value can risk losing connections to high latency/packet loss clients.
      readTimeout: 5000ms
      # writeTimeoutMs - optional, default 10000ms
      # The total maximum time in milliseconds that the http server will wait for a single requests to be received and
      # responded too. A higher value can allow long running requests to consume resources on the host. A lower value
      # can risk ending requests before the server has a chance to respond.
      writeTimeout: 100000ms
      # minTLSVersion - optional, default TSL1.2
      # The minimum version of TSL to support
      minTLSVersion: TLS1.2
      # maxTLSVersion - optional, default TSL1.3
      # The maximum version of TSL to support
      maxTLSVersion: TLS1.3
    # apis - required
    # Allows one or more APIs to be bound to this webListener
    apis:
      # binding - required
      # Specifies an API to bind to this webListener. Built-in APIs are
      #   - edge-management
      #   - edge-client
      #   - fabric-management
      - binding: edge-management
        # options - variable optional/required
        # This section is used to define values that are specified by the API they are associated with.
        # These settings are per API. The example below is for the `edge-api` and contains both optional values and
        # required values.
        options: { }
      - binding: edge-client
        options: { }
  - name: test-remove-me
    bindPoints:
      - interface: 127.0.0.1:1281
        address: 127.0.0.1:1281
    options: { }
    apis:
      - binding: edge-management
        options: { }
      - binding: edge-client
        options: { }
```

All optional values are defaulted. The smallest configuration possible that places the Edge Client
and Managements APIs on the same `BindPoint` would be:

```
web:
  - name: client-management-localhost
    bindPoints:
      - interface: 127.0.0.1:1280
        address: 127.0.0.1:1280
    options: { }
    apis:
      - binding: edge-management
        options: { }
      - binding: edge-client
        options: { }
```

The following examples places the Management API on localhost and the Client API on all available interface
and advertised as `client.api.ziti.dev:1280`:

```
web:
  - name: client-all-interfaces
    bindPoints:
      - interface: 0.0.0.0:1280
        address: client.api.ziti.dev:1280
    options: { }
    apis:
      - binding: edge-client
        options: { }
  - name: management-local-only
    bindPoints:
      - interface: 127.0.0.1:1234
        address: 127.0.0.1:1234
    options: { }
    apis:
      - binding: edge-management
        options: { }
```

#### Version Endpoint Updates

All Edge APIs support the `/version` endpoint and report all the APIs supported by the controller.
Each API now has a `binding` (string name) which is a global handle for that API's capabilities. See
the current list below

- Client API: `edge-client`, `edge`
- Management API: `edge-management`

Note: `edge` is an alias of `edge-client` for the `/version` endpoint only. It is considered deprecated.

These `bind names` can be used to parse the information returned by the `/version` endpoint to obtain the
most correct URL path for each API and version present. At a future date, other APIs with new `binding`s
(e.g. 'fabric-management` or 'fabric') or new versions may be added to this endpoint.

Versions prior to 0.20 of the Edge Controller reported the following:

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
        "buildDate": "2020-08-11 19:48:57",
        "revision": "e4ae43213a8d",
        "runtimeVersion": "go1.14.7",
        "version": "v0.16.0"
    },
    "meta": {}
}
```
Note: `/edge/v1` is deprecated

Version 0.20 and later report:

```
{
    "data": {
        "apiVersions": {
            "edge": {
                "v1": {
                    "apiBaseUrls": [
                        "https://127.0.0.1:1280/edge/client/v1",
                        "https://127.0.0.1:1281/edge/client/v1"
                    ],
                    "path": "/edge/client/v1"
                }
            },
            "edge-client": {
                "v1": {
                    "apiBaseUrls": [
                        "https://127.0.0.1:1280/edge/client/v1",
                        "https://127.0.0.1:1281/edge/client/v1"
                    ],
                    "path": "/edge/client/v1"
                }
            },
            "edge-management": {
                "v1": {
                    "apiBaseUrls": [
                        "https://127.0.0.1:1280/edge/management/v1",
                        "https://127.0.0.1:1281/edge/management/v1"
                    ],
                    "path": "/edge/management/v1"
                }
            }
        },
        "buildDate": "2020-01-01 01:01:01",
        "revision": "local",
        "runtimeVersion": "go1.16.2",
        "version": "v0.0.0"
    },
    "meta": {}
}.

```
