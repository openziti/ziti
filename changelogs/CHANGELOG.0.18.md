# Release 0.18.10

# What's New

* Close terminating xgress instances if no start is received with a configurable timeout
    * Timeout is set in the router config under listener/dialer options: `sessionStartTimeout`
      Default value: `3m`
* Don't add a second shutdown timer if one is already set
* Allow list/updating router forwarding tables if --debug-ops is passed
    * new command `ziti ps route <optional target> <session> <src-address> <dest-address>`
    * new command `ziti ps dump-routes <optional target>`
* If an xgress session fails in retransmit, sends fault notification to controller so controller can
  fix path or remove session, depending on session state

# Release 0.18.9

# What's New

* Fix PATCH OS Posture Checks clearing data
* Fix ziti-tunnel panic when removing
  services. [edge#517](https://github.com/openziti/edge/issues/517)
* ziti-tunnel tproxy now supports `--lanIf` option to automatically add rules to accept incoming
  connections for service connections. [edge#519](https://github.com/openziti/edge/issues/519)
* Fix orphaned ottca enrollments after `DELETE /cas/<id>'
* Add build info output when starting router/controller
* Remove histograms from xgress dataflow path as they were causing bottlenecks

# Release 0.18.8

## What's New

* Websocket binding in Edge Router to support connections from BrowZer-based apps that use
  the `ziti-sdk-js`. Edge Routers support various configurations including a single `tls` binding, a
  single `ws` binding, or having both `tls` and `ws` bindings simultaneously. If both binding types
  are used, they must be specified with different ports.
* Edge Router list for current identity
* Fix terminator PATCH, don't update/clear peer data unless requested
* Fix concurrency related crash in router
* Fix resource leak in fabric: forwarder tables weren't always updated after unroute
* Fix issue that prevented ziti-tunnel from cleaning up on shutdown in some cases.
  [edge#506](https://github.com/openziti/edge/issues/506)

## Websocket Binding

```
#   Example Edge Router config snippet (note new `ws` address type):

listeners:
  - binding: edge
    address: ws:0.0.0.0:3021
    options:
      advertise: curt-edge-ws-router:3021
  - binding: edge
    address: tls:0.0.0.0:3022
    options:
      advertise: curt-edge-ws-router:3022
```

* Fix router memory leak: purge session from forwardTable during unroutTimeout

## Edge Router List For Current Identity

A new endpoint has been added which will display the list of Edge Routers an authenticated session
has access to via any policy. The records will indicate whether the router is online, its hostname,
and its supported protocols. This endpoint will not return Edge Routers that have not completed
enrollment. Edge Routers that are offline will not have hostname and supported protocol information.

Endpoint: `GET /current-identity/edge-routers`

Example Output:

```
{
    "data": [
        {
            "createdAt": "2021-01-27T20:13:18.599Z",
            "id": "LolSlAQMq",
            "tags": {},
            "updatedAt": "2021-01-27T20:13:19.762Z",
            "hostname": "",
            "isOnline": false,
            "name": "er1",
            "supportedProtocols": {}
        },
        {
            "createdAt": "2021-01-27T20:13:19.308Z",
            "id": "oVzRl6kCq",
            "tags": {},
            "updatedAt": "2021-01-27T20:13:19.901Z",
            "hostname": "127.0.0.1:5002",
            "isOnline": true,
            "name": "er2",
            "supportedProtocols": {
                "tls": "tls://127.0.0.1:5002",
                "wss": "wss://127.0.0.1:5002"
            }
        }
    ],
    "meta": {
        "filterableFields": [
            "id",
            "createdAt",
            "updatedAt",
            "name",
        ],
        "pagination": {
            "limit": 10,
            "offset": 0,
            "totalCount": 2
        }
    }
}
```

# Release 0.18.7

## What's New

* Update ziti-tunnel service polling
    * Now uses new api (when available) to skip refreshing services if no services have been changed
    * Polling rate is passed through to sdk, so actual poll rate is now controlled
* Don't panic on router startup, just show error message and exit
* Fix resource leak: go-routine on terminators using the edge_transport binding
* Fix resource leak: retransmission timers for xgress instances not being shutdown when xgress
  closed
* Control channel metrics now available
* Fix potential deadlock on xgress close
* When closing due to fault notification, wait for data coming from remote to stop, not data from
  both local and remote, since local writes may never stop, due to retransmission attempts
* Add new config option to router `xgressCloseCheckInterval`, which dictates for how long data flow
  from the remote should have stopped before closing xgress after receiving fault
* `limit none` for Edge API Rest requests is now properly limited to 500 elements on list endpoints
* The HTTP header `server` is now populated on all responses with `ziti-controller/vX.Y.Z`

## Control Channel Metrics

**Note:** This feature is only available if both controller and router are on 0.18.7 or higher.

The control channels between the controller and routers now generate metrics, including:

* `ctrl.<router id>.latency`
* `ctrl.<router id>.tx.bytesrate`
* `ctrl.<router id>.tx.msgrate`
* `ctrl.<router id>.tx.msgsize`
* `ctrl.<router id>.rx.bytesrate`
* `ctrl.<router id>.rx.msgrate`
* `ctrl.<router id>.rx.msgsize`

There is a new controller config file setting:

* `ctrlChanLatencyIntervalSeconds` which controls how often the latency probe is sent. Default
  value: 10

# Release 0.18.6

## What's New

* Fix `ziti install terraform-provider-edgecontroller`

# Release 0.18.5

## What's New

* Update go-openapi libraries
* xgress_edge refactor, should fix 'failed to dipsatch to fabric' errors
* Update `ziti use` command to work with main branch
* MFA Support
* Fix deadlock on session close in router when connection is at capacity
* Fix issue where end of session didn't get sent in some scenarios

## MFA Support

Endpoint MFA is available that is based on RFC 4226 (HOTP: An HMAC-Based One-Time Password
Algorithm) and RFC 6238 (TOTP: Time-Based One-Time Password Algorithm). These standards are
compatible with standard "Authenticator" apps such as Google Authenticator and Authy. MFA is
suggested to be used in situations where human operators are involved and additional security is
desired.

### Restricting Service Access

Services can now have a Posture Check of type MFA that can be created and associated with a Service
Policy. Service Policies that are associated with an MFA Posture Check will restrict access to
services if a client has not enrolled in MFA and passed an MFA check on each login.

MFA Posture Checks support only the basic Posture Check fields:

- name - a name for the posture check
- typeId - should be "MFA"
- tags - any tags desired for this object
- roleAttributes - role attributes used to select this object from Service Policies

Example:

```
POST /posture-checks
{
    "name": "Any MFA",
    "typeId": "MFA",
    "roleAttributes": ["mfa"]
}
```

### Admin MFA Management

Admins of the Ziti Edge API can remove MFA from any user. However, they cannot enroll on behalf of
the client. The client will have to initiate MFA enrollment via their client.

Endpoints:

- `DELETE /identities/<id>/mfa` - remove MFA from an identity
- `GET /identities` - has a new field `isMfaEnabled` that is true/false based on the identity's MFA
  enrollment
- `GET /identities/<id>/posture-data` - now includes a `sessionPostureData` field which is a map of
  sessionId -> session posture data (including MFA status)

Example Posture Data:

```
{
  "mac": ["03092ac3bc69", "2b6df1dc52d9"],
  "domain": "mycorp.com",
  "os": {
    ...
  },
  processes: [
    ...
  ],
  sessionPostureData: {
     "xV1442s": {
        "mfa": {
          "passedMfa": true
        }
     }
  }
}

```

### Client MFA Enrollment

Clients must individually enroll in MFA as the enrollment process includes exchanging a symmetric
key. During MFA enrollment the related MFA endpoints will return different data and HTTP status
codes based upon the state of MFA enrollment (enrollment not started, enrollment started, enrolled).

The general MFA enrollment flow is:

1. Authenticate as the identity via `POST /authenticate`
2. Start MFA enrollment via `POST /current-identity/mfa`
3. Retrieve the MFA provisioning URL or QR code
    - `GET /current-identity/mfa`
    - `GET /current-identity/mfa/qr-code`
4. Use the provisioning URL or QR code with an authentication app such as Google Authenticator,
   Authy, etc.
5. Use a current code from the authenticator to `POST /current-identity/mfa/verify` with the code in
   the `code` field `{"code": "someCode"}`

#### MFA Endpoints Overview:

This section is an overview for the endpoints. Each endpoint may return errors depending on in input
and MFA status.

- `GET /current-identity/mfa` - returns the current state of MFA enrollment or 404 Not Found
- `POST /current-identity/mfa` - initiates MFA enrollment or 409 Conflict
- `DELETE /current-identity/mfa` - remove MFA enrollment, requires a valid TOTP or recovery code
- `GET /current-identity/mfa/recovery-codes` - returns the current recovery codes, requires a valid
  TOTP Code
- `POST /current-identity/mfa/recovery-codes` - regenerates recovery codes, requires a valid TOTP
  code
- `POST /current-identity/mfa/verify` - allows MFA enrollment to be completed, requires a valid TOTP
  code
- `GET /current-identity/mfa/qr-code` - returns a QR code for use with QR code scanner, MFA
  enrollment must be started
- `POST /authenticate/mfa` - allows MFA authentication checks to be completed, requires a valid TOTP
  or recovery code

MFA Enrollment Not Started:

- `GET /current-identity/mfa` - returns HTTP status 404
- `POST /current-identity/mfa` - start MFA enrollment, 200 Ok
- `DELETE /current-identity/mfa` - returns 404 Not Found
- `GET /current-identity/mfa/recovery-codes` - returns 404 Not Found
- `POST /current-identity/mfa` - returns 404 Not Found
- `POST /current-identity/mfa/verify` - returns 404 Not Found
- `GET /current-identity/mfa/qr-code` - returns 404 Not Found

MFA Enrollment Started:

- `GET /current-identity/mfa` - returns the current MFA enrollment and recovery codes
- `POST /current-identity/mfa` - returns 409 Conflict
- `DELETE /current-identity/mfa` - aborts the current enrollment, a blank `code` may be supplied
- `GET /current-identity/mfa/recovery-codes` - returns 404 Not Found
- `POST /current-identity/mfa` - returns HTTP status 409 Conflict
- `POST /current-identity/mfa/verify` - validates the supplied `code`
- `GET /current-identity/mfa/qr-code` - returns a QR code for use with QR code scanner in PNG format

MFA Completed:

- `GET /current-identity/mfa` - returns the current MFA enrollment, but not recovery codes
- `POST /current-identity/mfa` - returns 409 Conflict
- `DELETE /current-identity/mfa` - removes MFA, a valid TOTP or recovery code must be supplied
- `GET /current-identity/mfa/recovery-codes` - shows the current recovery codes, a valid TOTP code
  must be supplied
- `POST /current-identity/mfa` - returns HTTP status 409 Conflict
- `POST /current-identity/mfa/verify` - returns HTTP status 409 Conflict
- `GET /current-identity/mfa/qr-code` - returns 404 Not Found

### Client MFA Recovery Codes

Client MFA recovery codes are generated during enrollment and can be regenerated at any time with a
valid TOTP code. Twenty codes are generated and are one time use only. Generating new codes replaces
all existing recovery codes.

To view:

```
GET /current-identity/mfa/recovery-codes
{
  "code": "123456"
}
```

To Generate new codes:

```
POST /current-identity/mfa/recovery-codes
{
  "code": "123456"
}
```

### Authentication

During API Session authentication a new `authQuery` field is returned. This field will indicate if
there are any outstanding authentication Posture Queries that need to be fulfilled before
authentication is considered complete.

When MFA authentication is required a field will now appear as an
`authQuery` with the following format:

```
{
  ...
  "token": "c68a187a-f4af-490c-a9dd-a09076511419",
  "authQueries": [
    ...,
    {
      "typeId": "MFA",
      "provider": "ZITI",
      "httpMethod": "POST",
      "httpUrl": "./authenticate/mfa",
      "minLength": 4,
      "maxLength": 6,
      "format": "alphaNumeric"
    },
    ...
  ]       
}
```

# Release 0.18.4

## What's New

* New ziti CLI command `ziti ps set-log-level`, allows you to set the application wide log level at
  runtime
* Allow invalid event types in controller config event subscriptions. Instead of failing to start,
  the controller will emit a warning. This allows us to use uniform configs across controllers which
  may not all support the same event types.
* Edge routers now have configurable timeouts when looking up API sessions and sessions

## Edge Router: Configurable Session Lookup Times

An Edge SDK client will create and api session and session with the controller first, then attempt
to use those sessions at an edge router. The controller will push session information to routers as
quickly as it can, but clients may still connect to the edge router before the client can. We
previously would wait up to 5 seconds for session to arrive before declaring a session invalid, but
would not wait for api-sessions.

We can now wait for both api-sessions and sessions. Both timeouts are configurable. They are
configured in the router config file under listeners options.

* `lookupApiSessionTimeout`
    * How long to wait before timing out when looking up api-sessions after client connect. Default
      5 seconds.
* `lookupSessionTimeout`
    * How long to wait before timing out when looking up sessions after client dial/bind. Default 5
      seconds.

Example router config file stanza:

```
listeners:
  - binding: edge
    address: tls:0.0.0.0:6342
    options: 
      advertise: 127.0.0.1:6432
      lookupApiSessionTimeout: 5s
      lookupSessionTimeout: 5s
```

# Release 0.18.3

## What's New

* Ziti executables that use JSON logging now emit timestamps that include fractional seconds.
  Timestamps remain in the RFC3339 format.
* Authentication mechanisms now allow `appId` and `appVersion` in `sdkInfo`
* Ziti executables that use JSON logging now emit timestamps that include fractional seconds.
  Timestamps remain in the RFC3339 format.
* Improved query performance by caching antlr lexers and parsers. Testing showed 2x-10x performance
  improvement
* Improve service list time by using indexes get related posture data
* Improved service polling
* Improved service policy enforcement - instead of polling this is now event based, which should
  result in slower cpu utilization on the controller
* Fixed a bug in service policy PATCH which would trigger when the policy type wasn't sent
* Support agent utilitiles (`ziti ps`) in ziti-tunnel
* Cleanup ack handler goroutines when links shut down
* Remove the following fabric metrics timers, as they degraded performance while being of low value
    * xgress.ack.handle_time
    * xgress.payload.handle_time
    * xgress.ack_write_time
    * xgress.payload_buffer_time
    * xgress.payload_relay_time
* The check-data-integrity operation may now only run a single instance at a time
    * To start the check, `ziti edge db start-check-integrity`
    * To check the status of a run `ziti edge db check-integrity-status`
* The build date in version info spelling has been fixed from builDate to buildDate
* A new metric has been added for timing service list requests `services.list`
* A bug was fixed in the tunneler which may have lead to leaked connections
* Ziti Edge API configurable HTTP Timeouts
* Add `ziti log-format` or `ziti lf` for short, for formatting JSON log output as something more
  human readable
* [fabric#151](https://github.com/openziti/fabric/issues/151) Add two timeout settings to the
  controller to configure how long route and dial should wait before timeout
    * terminationTimeoutSeconds - how long the router has to dial the service
    * routeTimeoutSeconds - how long a router has to respond to a route create/update message
* [fabric#158](https://github.com/openziti/fabric/issues/158) Add a session creation timeout to the
  router. This controls how long the router will wait for fabric sessions to be created. This
  includes creating the router and dialing the end service, so the timeout should be at least as
  long as the controller `terminationTimeoutSeconds`and `routeTimeoutSeconds` added together
    * `getSessionTimeout` is specified in the router config under `listeners: options:`

## Improved Service Polling

There's a new REST endpoint /current-api-session/service-updates, which will return the last time
services were changed. If there have been no service updates since the api session was established,
the api session create date/time will be returned. This endpoint can be polled to see if services
need to be refreshed. This will save network and cpu utilization on the client and controller.

## Ziti Edge API configurable HTTP Timeouts

The controller configuration file now supports a `httpTimeouts` section under
`edge.api`. The section and all of its fields are optional and default to the values of previous
versions.

For production environments these values should be tuned for the networks intended userbase. The
quality and latency of the underlay between the networks endpoints/routers and controller should be
taken into account.

```
edge:
  ...
  api:
    ...
    httpTimeouts:
      # (optional, default 5s) readTimeoutMs is the maximum duration for reading the entire request, including the body.
      readTimeoutMs: 5000
      # (optional, default 0) readHeaderTimeoutMs is the amount of time allowed to read request headers.
      # The connection's read deadline is reset after reading the headers. If readHeaderTimeoutMs is zero, the value of
      # readTimeoutMs is used. If both are zero, there is no timeout.
      readHeaderTimeoutMs: 0
      # (optional, default 10000) writeTimeoutMs is the maximum duration before timing out writes of the response.
      writeTimeoutMs: 100000
      # (optional, default 5000) idleTimeoutMs is the maximum amount of time to wait for the next request when keepalives are enabled
      idleTimeoutMs: 5000
```

# Release 0.18.2

## What's New

* Default hosting precedence and cost can now be configured for identities
* Health checks can now be configured for the go based tunneler (ziti-tunnel) using server configs
* [ziti#177](https://github.com/openziti/ziti/issues/177) ziti-tunnel has a new `host` mode, if you
  are only hosting services
* Changes to terminators (add/updated/delete/router online/router offline) will now generate events
  that can be emitted
* fabric and edge session events now contain a timestamp

## Setting precedence and cost for tunneler hosted services

When the tunneler hosts services there was previously no way to specify the precedence and cost
associated with those services.
See [Ziti XT documentation](https://openziti.github.io/ziti/services/overview.html?tabs=create-service-ui#xt)
for an overview of how precedence and cost relate to HA and load balancing.

There are now two new fields on identity:

* defaultHostingPrecedence - value values are `default`, `required` and `failed`. Defaults
  to `default`.
* defaultHostingCost - valid values are between 0 and 65535. Defaults to 0.

When hosting a service via the tunneler, the terminator for the SDK hosted service will be created
with the precedence and cost of the identity used by the tunneler.

**NOTE:** This means all services hosted by an identity will have the same precedence and cost.
We'll likely add support for service specific overrides in the future if/when use cases arise which
call for it. In the meantime, a work-around is to use multiple identities if you need different
values for different services.

### CLI Support

The ziti CLI supports setting the default hosting precedence and cost when creating identities

### SDK API Change

The GO SDK has a new API method `GetCurrentIdentity() (*edge.CurrentIdentity, error)` which lets SDK
users retrieve the currently logged in identity, including the default hosting precedence and cost.
This could be used by other SDK applications which may want to use the fields for the same reason
when hosting services.

## Tunneler Health Checks

The go tunneler now supports health checks. Support for health checks may be added to other
tunnelers (such as ziti-edge-tunnel) in the future, but that is not guaranteed.

Health checks can be configured in the service configuration using the `ziti-tunneler-server.v1`
config type. Support in the `host.v1` config type will be added when support for that config type is
added to the go tunneler.

### Check Types

The tunneler supports two types of health check.

#### Port Checks

Port checks look to see if a host/port can be dialed. This is simple check which just ensures that
something is listening on a give host/port.

Port checks have the following properties:

* interval - how often the check is performanced
* timeout - how long to wait before declaring the check failed
* address - the address to dial. Should be of the form <host or ip>:<port>. Example: localhost:5432
* actions - an array of actions to perform based on health check results. Actions will be discussed
  in more detail below

#### HTTP Checks

Http checks a specific URL. They support the following properties:

* interval - how often the check is performanced
* timeout - how long to wait before declaring the check failed
* url - the url to connect to
* method - the HTTP method to use. Maybe one of `GET`, `POST`, `PUT` or `PATCH`. Defaults to `GET`
* body - the body of the HTTP request. Defaults to an empty string
* expectStatus - the HTTP status to expect in the response. Defaults to 200
* expectBody - an optional string to look for in the response body.
* actions - an array of actions to perform based on health check results. Actions will be discussed
  in more detail below

### Health Check Actions

Each health check may specify actions to execute when a health check runs.

Each action may specify:

* trigger - valid values `pass` or `fail`. Specifies if the action should run when the check is
  passing or failing
* consecutiveEvents - specifies if the action should only run after N consecutive passes or fails
* duration - specifies if the action should only run after the check has been passing or failing for
  some period of time
* action - specifies what to do when the action is run. valid values are:
    * `mark healthy` - the terminator precedence will be set to the default hosting precedence of
      the hosting identity
    * `mark unhealthy` - the terminator precedence will be set to `failed`
    * `increase cost N` - the terminator cost will be increased by N. This will only happen while
      the terminator precedence is not failed. Once the terminator has failed we don't keep
      increasing cost, otherwise it will likely reach max cost and take a long time to recover after
      it goes back to healthy.
    * `decrease cost N` - the terminator cost will be decrease by N to a minimuim. The terminator
      cost will not go below the hosting identity's default hosting cost

#### Examples

The following config defines a TCP service which can be reach at port 8171 on `localhost`. It has a
port check defined which runs every 5 seconds, with a timeout of 500 milliseconds. The following
actions are defined on the health check:

1. The terminator will be marked failed after the health check has failed 10 times in a row.
1. The terminator cost will be increased by 100 each time the health check fails while the
   terminator is not in failed state
1. The terminator will be returned to a non-failed state if the health check is healthy for 10
   seconds
1. Every time the health check passes the cost will be reduced by 25, until it hits the baseline
   cost defined by the hosting identity

```
{
    "protocol" : "tcp",
    "hostname" : "localhost",
    "port" : 8171,
    "portChecks" : [
        {
            "interval" : "5s",
            "timeout" : "500ms",
            "address" : "localhost:8171",
            "actions": [
                {
                    "action": "mark unhealthy",
                    "consecutiveEvents": 10,
                    "trigger": "fail"
                },
                {
                    "action": "increase cost 100",
                    "trigger": "fail"
                },
                {
                    "action": "mark healthy",
                    "duration": "10s",
                    "trigger": "pass"
                },
                {
                    "action": "decrease cost 25",
                    "trigger": "pass"
                }
            ]
        }
    ]
}

```

## ziti-tunnel host command

The ziti-tunnel can now be run in a mode where it will only host services and will not intercept any
services.

Ex: `ziti-tunnel host -i /path/to/identity.json`

## Schema Reference

For reference, here is the full, updated `ziti-tunneler-server.v1` schema:

```
{
    "$id": "http://edge.openziti.org/schemas/ziti-tunneler-server.v1.config.json",
    "additionalProperties": false,
    "definitions": {
        "action": {
            "additionalProperties": false,
            "properties": {
                "action": {
                    "pattern": "(mark (un)?healthy|increase cost [0-9]+|decrease cost [0-9]+)",
                    "type": "string"
                },
                "consecutiveEvents": {
                    "maximum": 65535,
                    "minimum": 0,
                    "type": "integer"
                },
                "duration": {
                    "$ref": "#/definitions/duration"
                },
                "trigger": {
                    "enum": [
                        "fail",
                        "pass"
                    ],
                    "type": "string"
                }
            },
            "required": [
                "trigger",
                "action"
            ],
            "type": "object"
        },
        "actionList": {
            "items": {
                "$ref": "#/definitions/action"
            },
            "maxItems": 20,
            "minItems": 1,
            "type": "array"
        },
        "duration": {
            "pattern": "[0-9]+(h|m|s|ms)",
            "type": "string"
        },
        "httpCheck": {
            "additionalProperties": false,
            "properties": {
                "actions": {
                    "$ref": "#/definitions/actionList"
                },
                "body": {
                    "type": "string"
                },
                "expectInBody": {
                    "type": "string"
                },
                "expectStatus": {
                    "maximum": 599,
                    "minimum": 100,
                    "type": "integer"
                },
                "interval": {
                    "$ref": "#/definitions/duration"
                },
                "method": {
                    "$ref": "#/definitions/method"
                },
                "timeout": {
                    "$ref": "#/definitions/duration"
                },
                "url": {
                    "type": "string"
                }
            },
            "required": [
                "interval",
                "timeout",
                "url"
            ],
            "type": "object"
        },
        "httpCheckList": {
            "items": {
                "$ref": "#/definitions/httpCheck"
            },
            "type": "array"
        },
        "method": {
            "enum": [
                "GET",
                "POST",
                "PUT",
                "PATCH"
            ],
            "type": "string"
        },
        "portCheck": {
            "additionalProperties": false,
            "properties": {
                "actions": {
                    "$ref": "#/definitions/actionList"
                },
                "address": {
                    "type": "string"
                },
                "interval": {
                    "$ref": "#/definitions/duration"
                },
                "timeout": {
                    "$ref": "#/definitions/duration"
                }
            },
            "required": [
                "interval",
                "timeout",
                "address"
            ],
            "type": "object"
        },
        "portCheckList": {
            "items": {
                "$ref": "#/definitions/portCheck"
            },
            "type": "array"
        }
    },
    "properties": {
        "hostname": {
            "type": "string"
        },
        "httpChecks": {
            "$ref": "#/definitions/httpCheckList"
        },
        "port": {
            "maximum": 65535,
            "minimum": 0,
            "type": "integer"
        },
        "portChecks": {
            "$ref": "#/definitions/portCheckList"
        },
        "protocol": {
            "enum": [
                "tcp",
                "udp"
            ],
            "type": [
                "string",
                "null"
            ]
        }
    },
    "required": [
        "hostname",
        "port"
    ],
    "type": "object"
}
```

## Terminator Events

Terminator events are now generated and can be found the fabric events/ package along with other
fabric events. They can also be emitted in json or plain text to a file or stdout, same as other
events. Events are generated when:

* A terminator is created
* A terminator is updated (generally precedence or static cost change)
* A terminator is deleted
* A router goes offline. Every terminator on that router will have an event generated
* A router goes online. Every terminator on that router will have an event generated

A terminator event will have the following properties:

* namespace - will always be `fabric.terminators`
* event_type - one of: created, updated, deleted, router-online, router-offline
* timestamp - when the event was generated
* service_id - id of the service that the terminator belongs to
* terminator_id - id of the terminator
* router_id - id of the router the terminator is on
* router_onlne - boolean flag indicating if the router is online
* precedence - the router precedence
* static_cost - the static cost of the terminator (managed externally, by admin or sdk)
* dynamic_cost - the dynamic cost of the terminator (managed by the terminator strategy for the
  service)
* total_terminators - the number of terminators currently existing on the service
* usable_default_terminators - the number of terminators on the service that have precedence default
* usable_required_terminators - the number of terminators on the service that have precedence
  required and are on an online router

Example: To register for json events

```
events:
  jsonLogger:
    subscriptions:
       - type: fabric.terminators
```

Example JSON output:

```
{
  "namespace": "fabric.terminators",
  "event_type": "updated",
  "timestamp": "2021-01-08T16:26:08.0005535-05:00",
  "service_id": "49Gc41SuL",
  "terminator_id": "y8qR",
  "router_id": "T-8CFqqtB",
  "router_online": true,
  "precedence": "required",
  "static_cost": 1100,
  "dynamic_cost": 0,
  "total_terminators": 1,
  "usable_default_terminators": 1,
  "usable_required_terminators": 0
}
```

# Release 0.18.1

* Improve log output for invalid API Session Tokens used to connect to Edge Routers
* Logs default to no color output
* API Session Certificate Support Added

### Logs default to no color output

Logs generated by Ziti components written in Go (Controller, Router, SDK) will no longer output ANSI
color control characters by default. Color logs can be enabled by setting in the environment
variable `PFXLOG_USE_COLOR` to any truthy value: 1, t, T, TRUE, true, True, 0, f, F, FALSE, false,
False.

### API Session Certificate Support Added

All authentication mechanisms can now bootstrap key pairs via an authenticated session using API
Session Certificates. These key pairs involve authenticating, preparing an X509 Certificate Signing
Request (CSR), and then submitting the CSR for processing. The output is an ephemeral certificate
tied to that session that can be used to connect to Edge Routers for session dial/binds.

New Endpoints:

- current-api-session/certificates
    - GET - lists current API Session Certificates
    - POST - create a new API Session Certificate (accepts a JSON payload with a `csr` field)
- current-api-session/certificates/<id>
    - GET - retrieves a specific API Session Certificate
    - DELETE - removes a specific API Session Certificate
    -

API Session Certificates have a 12hr life span. New certificates can be created before previous ones
expire and be used for reconnection.

# Release 0.18.0

## What's New

* [ziti#253](https://github.com/openziti/ziti/issues/253) `ziti-tunnel enroll` should set non-zero
  exit status if an error occur
* Rewrite of Xgress with the following goals
    * Fix deadlocks at high throughput
    * Fix stalls when some endpoints are slower than others
    * Improve windowing/retransmission by pulling forward some concepts from Michael Quigley's
      transwarp work
    * Split xgress links into two separate connections, one for data and one for acks
* Allow hosting applications to mark incoming connections as failed. Update go tunneler so when a
  dial fails for hosted services, the failure gets propagated back to controller
* Streamline edge hosting protocol by allowing router to assign connection ids
* Edge REST query failures should now result in 4xx errors instead of 500 internal server errors
* Fixed bug where listing terminators via `ziti edge` would fail when terminators referenced pure
  fabric services

## Xgress Rewrite

### Overview

This rewrite fixed several deadlocks observed at high throughput. It also tries to ensure that slow
clients attached to a router can't block traffic/processing for faster clients. It does this by
dropping data for a client if the client isn't handling incoming traffic quickly enough. Dropped
payloads will be retransmitted. The new xgress implementation uses similar windowing and
retransmission strategies to the upcoming transwarp work.

### Backwards Compatibility

0.18+ routers will probably work with older router versions, but probably not well. 0.18+ xgress
instances expect to get round trip times and receive buffer sizes on ack messages. If they don't get
them then retransmission will likely be either too aggressive or not aggressive enough.

Mixing 0.18+ routers with older router versions is not recommended without doing more testing first.

### Xgress Options Changes

**Added**

* txQueueSize - Number of payloads that can be queued for processing per client. Default value: 1
* txPortalStartSize - Initial size of send window. Default value: 16Kb
* txPortalMinSize - Smallest allowed send window size. Default value: 16Kb
* txPortalMaxSize - Largest allowed send window size. Default value: 4MB
* txPortalIncreaseThresh - Number of successful acks after which to increase send portal size:
  Default value: 224
* txPortalIncreaseScale - Send portal will be increased by amount of data sent since last
  retransmission. This controls how much to scale that amount by. Default value: 1.0
* txPortalRetxThresh - Number of retransmits after which to scale the send window. Default value: 64
* txPortalRetxScale - Amount by which to scale the send window after the retransmission threshold is
  hit. Default value: 0.75
* txPortalDupAckThresh - Number of duplicates acks after which to scale the send window. Default
  value: 64
* txPortalDupAckScale - Amount by which to scale the send window after the duplicate ack threshold
  is hit. Default value: 0.9
* rxBufferSize - Receive buffer size. Default value: 4MB
* retxStartMs - Time after which, if no ack has been received, a payload should be queued for
  retransmission. Default value: 200ms
* retxScale - Amount by which to scale the retranmission timeout, which is calculated from the round
  trip time. Default value: 2.0
* retxAddMs - Amount to add to the retransmission timeout after it has been scaled. Default value: 0
* maxCloseWaitMs - Maximum amount of time to wait for queued payloads to be
  acknowledged/retransmitted after an xgress session has been closed. If queued payloads are all
  acknowledged before this timeout is hit, the xgress session will be closed sooner. Default value:
  30s

**REMOVED:** The retransmission option is no longer available. Retransmission can't be toggled off
anymore as that would lead to lossy connections.

### Xgress Metrics Changes

New metrics were introduced as part of the rewrite.

**NOTE:** Some of these metrics were introduced to try and find places where tuning was required.
They may not be interesting or useful in the long term and may be removed in a future release.

The new metrics include:

**New Meters**

* xgress.dropped_payloads
    * The count and rates payloads being dropped
* xgress.retransmissions
    * The count and rates payloads being retransmitted
* xgress.retransmission_failures
    * The count and rates payloads being retransmitted where the send fails
* xgress.rx.acks
    * The count and rates of acks being received
* xgress.tx.acks
    * The count and rates of acks being sent
* xgress.ack_failures
    * The count and rates of acks being sent where the send fails
* xgress.ack_duplicates
    * The count and rates of duplicate acks received

**New Histograms**

* xgress.rtt
    * Round trip time statistics aggregated across all xgress instances
* xgress.tx_window_size
    * Local window size statistics aggregated across all xgress instances
* xgress.tx_buffer_size
    * Local send buffer size statistics aggregated across all xgress instances
* xgress.local.rx_buffer_bytes_size
    * Receive buffer size statistics in bytes aggregated across all xgress instances
* xgress.local.rx_buffer_msgs_size
    * Receive buffer size statistics in number of messages aggregated across all xgress instances
* xgress.remote.rx_buffer_size
    * Receive buffer size from remote systems statistics aggregated across all xgress instances
* xgress.tx_buffer_size
    * Receive buffer size from remote systems statistics aggregated across all xgress instances

**New Timers**

* xgress.tx_write_time
    * Times how long it takes to write xgress payloads from xgress to the endpoint
* xgress.tx_write_time
    * Times how long it takes to write acks to the link
* xgress.payload_buffer_time
    * Times how long it takes to process xgress payloads coming off the link (mostly getting them
      into the receive buffer)
* xgress.payload_relay_time
    * Times how long it takes to get xgress payloads out of the receive buffer and queued to be sent

**New Gauges**

* xgress.blocked_by_local_window
    * Count of how many xgress instances are blocked because the local transmit buffer size equals or
      exceeds the window size
* xgress.blocked_by_local_window
    * Count of how many xgress instances are blocked because the remote receive buffer size equals
      or exceeds the window size
* xgress.tx_unacked_payloads
    * Count of payloads in the transmit buffer
* xgress.tx_unacked_payload_bytes
    * Size in bytes of the transmit buffer

### Split Links

The fabric will now create two channels for each link, one for data and the other for acks. When
establishing links the dialing side will attach headers indicating the channel type and a shared
link ID. If the receiving side doesn't support split links then it will treat both channels as
regular links and send both data and acks over both.

If an older router dials a router expecting split links it won't have the link type and will be
treated as a regular, non-split link.

## Allow SDK Hosting Applications to propagate Dial Failures

The service terminator strategies use dial failures to adjust terminator weights and/or mark
terminators as failed. Previously SDK applications didn't have a way to mark a dial as failed. If
the SDK was hosting an application, this was generally not a problem. If the application could be
reached, it wouldn't want to mark an incoming connection as failed. However, the tunneler is just
proxying connections. It wants to be able to reach out to another application when the service is
dialed and proxy data. If the dial fails, it wants to be able to notify the controller that the
application wasn't reachable. The golang SDK now has the capability.

There is a new API on `edge.Listener`.

```
	AcceptEdge() (Conn, error)
```

The `Conn` returned here is an `edge.Conn` (which extends `net.Conn`). `edge.Conn` has two new APIs.

```
	CompleteAcceptSuccess() error
	CompleteAcceptFailed(err error)
```

If `ListenWithOptions` is called with the `ManualStart: true` in the provided options, the
connection won't be established until `CompleteAcceptSuccess` is called. Writing or reading the
connection before call that method will have undefined results.

The ziti-tunnel has been updated to use this API, and so should now work correctly with the various
terminator strategies.

### Edge Hosting Dial Protocol Enhancement

When establishing a new virtual connection to hosted SDK application the router had to execute the
following steps:

1. Send a Dial message to the sdk application
1. Receive the dial response, which included the sdk generaetd connection id.
1. Create the router side virtual connection with the new id and register it
1. Create the xgress instance tied to the new connection
1. Now that the xgress is created, send a message to the sdk application letting it now that it can
   start sending traffic

If the connection id could be established on the router, we could simplify things as follows

1. Create the router side virtual connection with the new id and register it
1. Create the xgress instance tied to the new connection
1. Send the dial mesasge to the sdk with the connection id
1. Receive the response and return the result to the controller

We didn't do this previously because the sdk controls ids for outbound connection. To enable this we
have split the 32 bit id range in half. The top half is now reserved for hosted connection ids. This
behavior is controlled by the SDK, which requests it when it binds using a boolean flag. The new
flag is:

```
    RouterProvidedConnId = 1012
```

If the bind result from the router has the same flag set to true, then the sdk will expect Dial
messages from the router to have a connection id provided in the header keyed with the same `1012`.

This means that this feature should be both backwards and forward compatible.
