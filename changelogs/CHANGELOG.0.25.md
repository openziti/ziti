# Release 0.25.13

## What's New
- Edge
  - Bug fixes
- Fabric
  - N/A
- Ziti CLI
  - N/A
- SDK Golang
  - N/A

## Edge
### Bug Fixes

* [https://github.com/openziti/edge/issues/1055](Fix for an edge router panic)

# Release 0.25.12

## What's New

No functional changes, build process changes only

# Release 0.25.11

## What's New
- Edge
  - Management API: Breaking Changes
  - Management API: New Endpoints
  - Management API: JWKS Support
  - Bug fixes
- Fabric
  - Bug fixes
  - Metrics API
- Ziti CLI
  - N/A
- SDK Golang
  - N/A

## Edge
### Management API Breaking Changes

The following Edge Management REST API Endpoints have breaking changes:

- `POST /ext-jwt-signers`
  - `kid` is required if `certPem` is specified
  - `jwtEndpoint` or `certPem` is required
  - `issuer` is now required
  - `audience` is now required
- `PUT /ext-jwt-signers` - `kid` is required if `certPem` is specified, `issuer` is required, `audience` is required
  - `kid` is required if `certPem` is specified
  - `jwtEndpoint` or `certPem` is required
  - `issuer` is now required
  - `audience` is now required
- `PATCH /ext-jwt-signers` - `kid` is required if `certPem` is specified, `issuer` is required, `audience` is required
  - `kid` is required if `certPem` is set and `kid` was not previously set
  - `jwtEndpoint` or `certPem` must be defined or previously set of the other is  `null`
  - `issuer` may not be set to `null` or `""`
  - `audience` may not be set to `null` or `""`

The above changes will render existing `ext-jwt-signers` as always failing authentication is `issuer` and `audience`
were not previously set.

### Management API: New Endpoints

The following new endpoints have been added:

- `GET /identities/:id/enrollments` - returns a pre-filtered list of enrollments for the identity specified by `:id`

### Management API: JWKS Support

JWKS (JSON Web Key Sets) is defined in [rfc7517](https://www.rfc-editor.org/rfc/rfc7517) and defines the format
and methods that public and private keys may be published via JSON. JWKS support enables Ziti to obtain
public signing keys from identity providers as needed. This enables identity providers to rotate signing keys without
breaking SSO integrations.

To facilitate this, `ext-jwt-signers` now support `jwksEndpoint` which is a URL that resolves to a service that returns 
a JWKS JSON payload. When specified, the `certPem` and `kid` files are no longer required. Additionally, when a JWT `iss` 
fields matches an existing `extj-jwt-signers`'s `issuer` field and the `kid` is currently unknown, the `jwksEndpoint` 
will be interrogated for new signing keys. The `jwksEndpoint` will only be interrogated at most once every five seconds.

### Bug Fixes

* https://github.com/openziti/edge/issues/1027
* https://github.com/openziti/edge/issues/1025
* https://github.com/openziti/edge/issues/1035
* https://github.com/openziti/edge/issues/1045
* https://github.com/openziti/edge/issues/1049

## Fabric
### Bug Fixes

* https://github.com/openziti/fabric/issues/406
* https://github.com/openziti/ziti/issues/565 - Moved terminator information to its own field.

### Metrics API

The following new endpoint has been added:
- `GET /metrics` - returns metrics for the controller and all routers in the Prometheus text exposition format.  See [https://openziti.github.io/ziti/metrics/prometheus.html] for more information and instructions to set it up.


# Release 0.25.10

## What's New
- Edge
  - N/A
- Fabric
  - N/A
- Ziti CLI
  - CLI support for enrollments/authenticators/re-enrollment
  - Fix prox-c download
  - ziti-fabric cleanup
  - Add public attributes and service policies allowing public access to routers in docker-compose quickstart
  - Add file overwrite checks for the "Local ziti quickstart" script
- SDK Golang
  - N/A

## Ziti CLI

### CLI support for enrollments/authenticators/re-enrollment

The CLI has been augmented to support the following commands:

- `ziti edge list authenticators` - to generically list existing authenticators
- `ziti edge list enrollments` - to generically list existing enrollments
- `ziti edge delete enrollment <id>` - to generically delete existing enrollments
- `ziti edge delete authenticator <id>` - to generically delete existing authenticator
- `ziti edge create enrollment ott ...` - to create a new one-time-token enrollment for an existing identity
- `ziti edge create enrollment ottca ...` - to create a new one-time-token enrollment for an existing identity for a 3rd party CA issued certificate
- `ziti edge create enrollment updb ...` - to create a new updb (username/password) enrollment for an existing identity

These commands, specifically the enrollment related ones, can be used to re-enroll existing identities. See the 0.25.9 changeFor all arguments and options, please see their CLI related `-h`.

Also note that the `ziti edge delete authenticator updb` command has been supplanted by `ziti edge delete authenticator <authenticator id>`

### Fix prox-c download

The prox-c releases on GitHub now include the architecture in the download URL. 
`ziti install ziti-prox-c` has been updated to take this into account.

### ziti-fabric cleanup

Ziti CLI install/upgrade/remove commands related to `ziti-fabric` have been
removed since `ziti-fabric` was deprecated and is not being published anymore.

# Release 0.25.9

## What's New
- Edge
  - Create Identity Enrollments / Allow Identity Re-Enrollment
- Fabric
  - Bug fixes
- Ziti CLI
  - N/A
- SDK Golang
  - N/A

## Edge

### Create Identity Enrollments / Allow Identity Re-Enrollment

The ability to create identity enrollments, allows new enrollment JWTs to be generated throughout any identity's
lifetime. This allows Ziti to support scenarios where re-enrolling an identity is more convenient than recreating it.

The most common scenario is new device transitions. Previously, the only way to deal with this scenario was to remove
the identity and recreate it. Depending on how the role attributes and policies were configured this may be a trivial or
demanding task. The more policies utilizing direct identity reference, instead of attribute selectors, the
more difficult it is to recreate that identity. Additional, re-enrolling an identity retains MFA TOTP enrollment,
recovery codes, and authentication policy assignments/configuration.

#### New Endpoints
- `POST /enrollments` - Create enrollments associated to an identity

#### POST /enrollments Properties

- `method` - required - one of `ott`, `ottca`, or `updb` to specify the type of enrollment (this affects other field requirements)
- `expiresAt` - required - the date and time the enrollment will expire
- `identityId` - required - the identity the enrollment is tied to
- `caId` - `ottca` required, others ignored - the verifying 3rd party CA id for the `ottca` enrollment
- `username` - `updb` required, others ignored - the default username granted to an identity during `updb` enrollment

#### Creating Identity Enrollments

Identity enrollments only allow one outstanding enrollment for each type of enrollment supported. For example attempting 
to create multiple `ott` (one-time-token) enrollments will return a `409 Conflict` error. Deleting existing enrollments will
resolve the issue.

As noted in the properties' section above, some properties are utilized for different `method` types. Please be aware
that while setting these values through the API will not be rejected, they are not utilized.

Please note that it is possible for an identity to have multiple authentication types. Authentication policies should
be used to restrict the type of authenticators that are valid, even if enrolment has been completed.


## Fabric

### Bug Fixes

* https://github.com/openziti/fabric/issues/404
    * Goroutine pool metrics for xgress and link dials not working

# Release 0.25.8

## Maintenance
Improved MacOS compatibility with cert handling and ioKit.

## Fabric

### Bug Fixes

* https://github.com/openziti/fabric/pull/403

# Release 0.25.7

## Fabric

### Xgress and Link Dial Defaults Updated
The default size of the xgress dialer pool has been updated to 128 from 10.
The default size of the link dialer pool has been updated to 32 from 10.

### Dial Timeout Propagation
Currently each section of the dial logic has its own timeouts. It can easily happen that
an early stage timeout expires while a later one doesn't, causing work to be done whose
results will be ignored. A first pass has been completed at threading timeouts/deadline
through the dial logic, spanning controller and routers, so that we use approximately
the same timeout throughout the dial process.

### Link Initial Latency
Previously links would start with a latency of 0, which they would keep until the latency
was reported from the routers. Now, latency will be initialized to a default of 65 seconds,
which it will stay at until an actual latency is reported. If a new link is the only 
available one for a given path, this won't prevent the link from being used. However, if
there are other paths available, this will bias the network to the existing paths until
it can see what the actual link latency is. Latency should generally be reported
within a minute or so. 

This value can be adjusted in the controller config, under the `network` section.

```
network:
  initialLinkLatency: 65s
```

### Link Verification changes
In previous releases when a router received a link dial from another router, it would verify
that the link was known to the controller and the dialing router was valid. Router validity
was checked by making sure the fingerprints of the certs used to establish the link matched
the fingerprints on record for the router.

From this release forwards we will only verify that the router requesting the link is valid
and won't check that the link is valid. This is because the router has more control over the
links now, and in future, may take over more of link management. As long as we're getting
link dials from a valid router, we don't care if they were controller initiated or router 
initiated. For now they are all controller initiated, but this also covers the case where
the controller times out a link, but the router still manages to initiate it. Now the router
can report the link back to the controller and it will be used.

### Add Goroutine Pool Metrics
We use goroutine pools which are fed by queues in several places, to ensure that we have 
guardrails on the number of concurrent activities. There are now metrics emitted for these
pools.

The pool types on the controller are:

* pool.listener.ctrl
* pool.listener.mgmt

The pool types on router are:

* pool.listener.link
* pool.link.dialer
* pool.route.handler
* pool.listener.xgress_edge (if edge is enabled)

Each pool has metrics for

* Current worker count
* Current queue size
* Current active works
* Work timer, which includes count of work performed, meter for work rate and histogram for work execution time


An example of the metric names for pool.listener.link:

```
pool.listener.link.busy_workers
pool.listener.link.queue_size
pool.listener.link.work_timer.count
pool.listener.link.work_timer.m15_rate
pool.listener.link.work_timer.m1_rate
pool.listener.link.work_timer.m5_rate
pool.listener.link.work_timer.max
pool.listener.link.work_timer.mean
pool.listener.link.work_timer.mean_rate
pool.listener.link.work_timer.min
pool.listener.link.work_timer.p50
pool.listener.link.work_timer.p75
pool.listener.link.work_timer.p95
pool.listener.link.work_timer.p99
pool.listener.link.work_timer.p999
pool.listener.link.work_timer.p9999
pool.listener.link.work_timer.std_dev
pool.listener.link.work_timer.variance
```

### Add Link Count and Cost to Circuit Events
Link Count will now be available on all circuit events. Circuit cost will be available on
circuit created events. The circuit cost is the full circuit cost (router costs + link costs
+ terminator costs). 

Example:
```
{
  "namespace": "fabric.circuits",
  "event_type": "created",
  "circuit_id": "XpSWLZB1P",
  "timestamp": "2022-05-11T13:00:06.976266668-04:00",
  "client_id": "cl31tuu93000iaugd57qv6hjc",
  "service_id": "dstSybunfM",
  "creation_timespan": 969933,
  "path": "[r/h-DqbP927]->[l/1qp6LIzSlWkQM1jSSTJG1j]->[r/Ce1f5dDCey]",
  "link_count": 1,
  "path_cost": 890
}
```

### Remove ziti-fabric CLI command
The previously deprecated ziti-fabric command will no longer be published as part of Ziti releases. 
All of ziti-fabric's functionality is available in the `ziti` CLI under `ziti fabric`.

### Add link delete
If a link gets in a bad state (see bug below for how this could happen), you can now use 
`ziti fabric delete link <link id>`. This will remove the link from the controller as well
as send link faults to associated routers. If the link is not known to the controller, a 
link fault will be sent to all connected routers.

## Miscellaneous

The `ziti-probe` tool will no longer be built and published as part of Ziti releases.

### Bug Fixes

* https://github.com/openziti/fabric/issues/393
* https://github.com/openziti/fabric/issues/395
* https://github.com/openziti/channel/issues/50
* `ziti fabric list circuits` was showing the router id instead of the link id in the circuit path

# Release 0.25.6

* Moving from Go 1.17 to 1.18
* Bug fix: Fixes an issue in quickstart "Host it anywhere" where an EXTERNAL_DNS was not added to the PKI causing failures when attempting to use a router from outside the hosted environment.

# Release 0.25.5

* Bug fix: Fixes an issue where dial could fail if the terminator router didn't response to routing last
* Enhancement: Updated Control Channel to use new heartbeat logging mirroring Links in Release `0.25.0`
* Enhancement: Added Circuit Creation Timespan which denotes how long the fabric took to construct a requested circuit.
```json
{
    "namespace": "namespace",
    "event_type": "event_type",
    "circuit_id": "circuit_id",
    "timestamp": "2022-04-07T14:00:52.0500632-05:00",
    "client_id": "client_id",
    "service_id": "service_id",
    "creation_timespan": 5000000, //Timespan in nanoseconds
    "path": "path"
}
```

* Bug fix: Fixes an issue where Edge administrator checks would not take default admin flag into account
* Bug fix: Fix an issue with docker-compose quickstart not properly loading env vars
* Enhancement: Add support for Apple M1 using the ziti quickstart CLI script
* Enhancement: Use an env file for docker-compose quickstart for easier version changes and other duplicated field values
* Enhancement: Allow for version override using the ziti quickstart CLI script
* Change: Renamed `pushDevBuild.sh` to `buildLocalDev.sh`, the script used for building a local dev version of the docker quickstart image
* Bug fix: Fixes an issues where `isAdmin` would always default to false on updates (put/patch)
* Bug fix: Identity property `externalId` was not properly rendering on `GET` and not handled consistently on `PUT` and `PATCH`
* Enhancement: External JWT Signer Issuer & Audience Validation
* Enhancement: Add ability to define local interface binding for link and controller dial
* Bug fix: Edge Management REST API Doc shows Edge Client REST API Doc
* Enhancement: `ziti db explore <ctrl.db>` command has been added to explore offline database files
* Enhancement: The mgmt API is now available via websocket. The stream commands are now available on `ziti fabric`
* Enhancement: Most list commands have been updated with tabular output
* Enhancement: `ziti edge show` is now available with subcommands `config` and `config-type`
    * `ziti edge list configs` no longer shows the associated json. It can be viewed using `ziti edge show config <config name or id>`
* Enhancement: `ziti edge update config-type` is now available
* Enhancement: `ziti edge create|update identity` now supports `--external-id`
* Bug fix: Fixes an issue where the router config would use hostname instead of the DNS name
* Bug fix: When establishing links, a link could be closed while being registered, leading the controller and router to get out of sync
* Enhancement: Add min router cost. Helps to minimize unnecessary hops.
    * Defaults to 10, configurable in the controller config with the minRouterCost value under `network:`
* Enhancement: Can now see xgress instance and link send buffer pointer values in circuit inspections. This allows correlating to stackdumps
* Enhancement: Can now see xgress related goroutines by using `ziti fabric inspect '.*' circuitAndStacks:<circuitId>`
* Enhancement: If a router connects to the controller but is already connected, the new connection now takes precedence
    * There is a configurable churn limit, which limits how often this can happen. 
    * The default is 1 minute and is settable via `routerConnectChurnLimit` under `network`
* Enhancement: Flow control changes
    * Duplicate acks won't shrink window. Duplicate acks imply retransmits and the retransmits already affect the window size
    * Drop min round trip time scaling to 1.5 as will get scaled up as needed by duplicate ack detection
    * Drop round trip time addition to 0 from 100ms and rely purely on scaling
    * Avoid potential stall by always allowing at least one payload into sender side, even when receiver is full.
        * This way if receiver signal to sender is lost, we'll still having something trying to send
* Enhancement: When router reconnects to controller, re-establish any embedded tunneler hosting on that router to ensure router and controller are in sync


## External JWT Signer Issuer & Audience Validation

External JWT Signers (endpoint `/external-jwt-signers`) now support `issuer` and `audience` optional string fields.
These fields may be set to `null` on `POST`/`PUT`/`PATCH` or omitted; which will result in no validation of incoming
JWT's `aud` and `iss` fields. If `issuer` is defined, JWT `iss` fields will be validated. If `audience` is defined, JWT
`aud` fields will be validated. If a JWT contains multiple audience values as an array of strings and will be validated,
validation will check if the External JWT Signer's `audience` value is present as one of the values.

## Add ability to define local interface binding for link and controller dial

The network interface used to dial the controller and router links can be provided in the router configuration file.  The interface can be provided as either a name or an IP address.  

```yaml
ctrl:
  endpoint:             tls:127.0.0.1:6262
  bind:                 wlp5s0

link:
  dialers:
    - binding:          transport
      bind:            192.168.1.11
```

# Release 0.25.4

**NOTE**: Link management is undergoing some restructuring to support better link costing and multiple interfaces. The link types introduced in 0.25 should not be used. A more complete replacement is coming soon.

* Enhancement: Add additional logging information to tunnel edge routers. Now adds the local address to the router/link chain.
* Enhancement: Add additional metrics for terminator errors. 
    - `service.dial.terminator.timeout`: Raised when the terminator times out when connecting with it's configured endpoint
    - `service.dial.terminator.connection_refused`: Raised when the terminator cannot connect to it's configured endpoint
    - `service.dial.terminator.invalid`: Raised when the edge router is unable to get or access the terminator
    - `service.dial.terminator.misconfigured`: Raised when the fabric is unable to find or create the terminator
* Enhancement: Authentication Policies
* Enhancement: JWT Primary/Secondary Authentication
* Enhancement: Required TOTP (fka MFA) Enrollment
* Bug fix: Fix router panic which can happen on link bind
* Bug fix: Fix router panic which can happen if the router shuts down before it's fully up an running
* Enhancement: Avoid router warning like `destination exists for [p57a]` by not sending egress in route, since egress will always already be established
* Enhancement: Change default dial retries to 3 from 2
* Enhancement: Add circuit inspect. `ziti fabric inspect .* circuit:<circuit-id>` will now return information about the circuit from the routers. This will include routing information as well as flow control data from the initiator and terminator.
* Change: Support for link types removed

## Authentication Policies

Authentication policies are configuration that allows administrators to enforce authentication requirements. A single
authentication policy is assigned to each identity in the system. This assignment is controlled on the `Identity`
entities within the Ziti Edge Management API. If an authentication policy is not specified, a system default policy is
applied that. The default policy represents the behavior of Ziti v0.25.3 and earlier and may be updated to the network's
requirements.

### Assignment

The `Identity` entity now supports a new field `authPolicyId`. In the REST Edge API this field is optional during create
and existing calls to `POST /identities` will succeed. Every identity must have exactly one authentication policy
assigned to it. If one is not assigned, the default authentication policy will be used (`authPolicyId` == `default`)

Example w/o `authPolicyId`:

`POST /edge/v1/management/identities`
```json

{
    "name": "zde",
    "type": "User",
    "isAdmin": false,
    "enrollment": {
        "ott": "true"
    },
    "roleAttributes": [
        "dial"
    ]
}
```

Example w/ `authPolicyId`:

`POST /edge/v1/management/identities`
```json
{
    "name": "zde",
    "type": "User",
    "isAdmin": false,
    "enrollment": {
        "ott": "true"
    },
    "roleAttributes": [
        "dial"
    ],
    "authPolicyId": "xyak1."
}
```

### Default Authentication Policy

Ziti contains a single default authentication policy that is marked as a "system" definition. It cannot be deleted,
but it can be updated. This authentication policy has a well known id of `default`. It can be viewed according to the
following example:

`GET /edge/v1/management/auth-policies/default`
```json
{
  "data": {
    "_links": {
      "self": {
        "href": "./auth-policies/default"
      }
    },
    "createdAt": "2022-03-30T17:54:55.785Z",
    "id": "default",
    "tags": {},
    "updatedAt": "2022-03-30T17:54:55.785Z",
    "name": "Default",
    "primary": {
      "cert": {
        "allowExpiredCerts": true,
        "allowed": true
      },
      "extJwt": {
        "allowed": true,
        "allowedSigners": null
      },
      "updb": {
        "allowed": true,
        "lockoutDurationMinutes": 0,
        "maxAttempts": 0,
        "minPasswordLength": 5,
        "requireMixedCase": false,
        "requireNumberChar": false,
        "requireSpecialChar": false
      }
    },
    "secondary": {
      "requireExtJwtSigner": null,
      "requireTotp": false
    }
  },
  "meta": {}
}
```

### AuthPolicy Endpoints

The following endpoints were added to support CRUD operations:

- List `GET /edge/v1/management/auth-policies`
- Create `POST /edge/v1/management/auth-policies`
- Detail `GET /edge/v1/management/auth-policies/{id}`
- Replace `PUT /edge/v1/management/auth-policies/{id}`
- Patch `PATCH /edge/v1/management/auth-policies/{id}`
- Delete `Delete /edge/v1/management/auth-policies/{id}`

And have the following properties:

- `name`: a unique name for the policy
- `primary.cert.allowed` - allow certificate based authentication
- `primary.cert.allowExpiredCerts` - allows clients with expired certificates to authenticate
- `primary.extJwt.allowed` - allow external JWT authentication
- `primary.extJwt.allowedSigners` - a specific set of external jwt signers that are allowed, if not set all enabled signers are allowed
- `primary.updb.allowed` - allow username/password authentication
- `primary.updb.lockoutDurationMinutes` - the number of minutes to lock an identity after exceeding `maxAttempts`, 0 = indefinite
- `primary.updb.minPasswordLength` - the minimum lengths passwords must be, currently a placeholder
- `primary.updb.requireMixedCase` - requires passwords to include mixed cases, currently a placeholder
- `primary.updb.requireNumberChar` - requires passwords to include at least 1 number, currently a placeholder
- `primary.updb.requireSpecialChar` - requires passwords to include at least 1 special character, currently a placeholder
- `secondary.requireExtJwtSigner` - requires an additional JWT bearer token be provided on all API requests, null is disabled
- `secondary.requireTotp` - requires TOTP (fka MFA enrollment) enrollment to be completed and in use
Example Create:

```json
{
    "name": "Original Name 1",
    "primary": {
        "cert": {
            "allowExpiredCerts": true,
            "allowed": true
        },
        "extJwt": {
            "allowed": true,
            "allowedSigners": [
                "2BurseGARW"
            ]
        },
        "updb": {
            "allowed": true,
            "lockoutDurationMinutes": 0,
            "maxAttempts": 5,
            "minPasswordLength": 5,
            "requireMixedCase": true,
            "requireNumberChar": true,
            "requireSpecialChar": true
        }
    },
    "secondary": {
        "requireExtJwtSigner": null,
        "requireTotp": false
    },
    "tags": {
        "originalTag1Name": "originalTag1Value"
    }
}
```

## JWT Primary/Secondary Authentication

A new primary authentication mechanism is available in addition to `cert` and `password` (UPDB). The internal
method name is `ext-jwt` and it allows authentication by providing a bearer token by a known external JWT signer.
A new entity `External JWT Singer` has been introduced and is defined in subsequent sections.

Successful primary authentication requires:

1) The target identity must have an authentication policy that allows primary external JWT signer authentication
2) The JWT provided must include a `kid` that matches the `kid` defined on an external JWT signer
3) The JWT provided must include a `sub` (or configured claim) that matches the identity's `id` or `externalId` (see below)
4) The JWT provided must be properly signed by the signer defined by `kid`
5) The JWT provided must be unexpired
6) The encoded JWT must be provided during the initial authentication in the `Authorization` header with the prefix `Bearer ` and subsequent API calls

A new secondary factor authentication mechanism is available in addition to TOTP (fka MFA). Both TOTP and `ext-jwt`
secondary authentication factors can be enabled at the same time for a "nFA" setup.

Successful secondary authentication requires all the same JWT token validation items, but as a secondary
factor, not providing a valid JWT bearer token on API requests will drop the request's access to 
"partially authenticated" - which has reduced access. Access can be restored by providing a valid JWT bearer token.
Additionally, to turn on the functionality, an authentication policy that has the `requireExtJwtSigner` field must be
set to a valid external JWT signer and assigned to the target identity(ies).

### External JWT Signers 

External JWT Signers can be managed on the following new REST Edge Management API endpoints:

- List `GET /edge/v1/management/external-jwt-signers`
- Create `POST /edge/v1/management/external-jwt-signers`
- Detail `GET /edge/v1/management/external-jwt-signers/{id}`
- Replace `PUT /edge/v1/management/external-jwt-signers/{id}`
- Patch `PATCH /edge/v1/management/external-jwt-signers/{id}`
- Delete `Delete /edge/v1/management/external-jwt-signers/{id}`

And support the following properties:

- `name` - a unique name for the signer
- `certPem` - a unique PEM x509 certificate for the signer
- `enabled` - whether the signer is currently enabled or disabled
- `externalAuthUrl` - the URL clients should use to obtain a JWT
- `claimsProperty` - the property to alternatively use for the target identity's `id` or `externalId`
- `useExternalId` - whether to match the `claimsProperty` to `id` (false) or `externalId` (true)
- `kid` - a unique `kid` value that will be present in a valid JWT's `kid` header

Example Create:

`POST /edge/v1/management/external-jwt-signers`
```json
{
    "certPem": "-----BEGIN CERTIFICATE-----\nMIIBizC ...",
    "enabled": true,
    "kid": "c7e2081d-b8f0-44b1-80fa-d73872692fd6",
    "name": "Test JWT Signer Pre-Patch Kid",
    "externalAuthUrl" : "https://my-jwt-provide/auth",
    "claimsProperty": "email",
    "useExternalId": "true"
}
```

The above example creates a new signer that is enabled and that will instruct clients that they can attempt to obtain
a JWT from `https://my-jwt-provide/auth`. The JWT that is returned from `https://my-jwt-provide/auth` should have a
`kid` header of `c7e2081d-b8f0-44b1-80fa-d73872692fd6` and the `email` claim will be matched against Ziti identity's
`externalId` field.

### Identity ExternalId

Ziti identity's have a new optional field named `externalId`. All existing identities will have this value defaulted
to `null`. This value is unique if set and is currently only used for external JWT signer authentication. Ziti treats 
the value as a case-sensitive opaque string.

It has standard CRUD access on the `edge/v1/management/identities` endpoints for `POST`, `PUT`, `PATCH`, and `GET`.

## Required TOTP (fka MFA) Enrollment

With authentication policies, it is now possible to enforce MFA enrollment at authentication. Prior to this release,
it was only possible to restrict access to service(s) via posture checks. The authentication policy value
`secondary.requireTotp` being set to true will now force identities into a "partially authenticated" state unless
TOTP MFA is completed.

Due to this, it is now possible to enroll in TOTP MFA while "partially authenticated". It is not possible to manipulate
an existing completed enrollment.

## Circuit Inspection
Here is an example of the kind of information you can get with the new circuit inspection factility

```
$ ziti fabric inspect .* circuit:GrtfcCjzD -j | jq
{
  "errors": null,
  "success": true,
  "values": [
    {
      "appId": "aKYdwbTf7l",
      "name": "circuit:GrtfcCjzD",
      "value": {
        "Destinations": {
          "1LKMInhzapHdurbaABaa50": {
            "dest": "CX1kmb0fAl",
            "id": "1LKMInhzapHdurbaABaa50",
            "protocol": "tls",
            "split": true,
            "type": "link"
          },
          "wPBx": {
            "addr": "wPBx",
            "originator": "Initiator",
            "recvBuffer": {
              "lastSizeSent": 21,
              "size": 0
            },
            "sendBuffer": {
              "accumulator": 47,
              "acquiredSafely": true,
              "blockedByLocalWindow": false,
              "blockedByRemoteWindow": false,
              "closeWhenEmpty": false,
              "closed": false,
              "duplicateAcks": 0,
              "linkRecvBufferSize": 23,
              "linkSendBufferSize": 0,
              "retransmits": 0,
              "retxScale": 2,
              "retxThreshold": 100,
              "successfulAcks": 3,
              "timeSinceLastRetx": "1m17.563s",
              "windowsSize": 16384
            },
            "timeSinceLastLinkRx": "1m11.451s",
            "type": "xgress"
          }
        },
        "Forwards": {
          "1LKMInhzapHdurbaABaa50": "wPBx",
          "wPBx": "1LKMInhzapHdurbaABaa50"
        }
      }
    },
    {
      "appId": "CX1kmb0fAl",
      "name": "circuit:GrtfcCjzD",
      "value": {
        "Destinations": {
          "1LKMInhzapHdurbaABaa50": {
            "dest": "aKYdwbTf7l",
            "id": "1LKMInhzapHdurbaABaa50",
            "protocol": "tls",
            "split": true,
            "type": "link"
          },
          "MZ9x": {
            "addr": "MZ9x",
            "originator": "Terminator",
            "recvBuffer": {
              "lastSizeSent": 23,
              "size": 0
            },
            "sendBuffer": {
              "accumulator": 45,
              "acquiredSafely": true,
              "blockedByLocalWindow": false,
              "blockedByRemoteWindow": false,
              "closeWhenEmpty": false,
              "closed": false,
              "duplicateAcks": 0,
              "linkRecvBufferSize": 21,
              "linkSendBufferSize": 0,
              "retransmits": 0,
              "retxScale": 2,
              "retxThreshold": 102,
              "successfulAcks": 2,
              "timeSinceLastRetx": "457983h26m1.336s",
              "windowsSize": 16384
            },
            "timeSinceLastLinkRx": "1m16.555s",
            "type": "xgress"
          }
        },
        "Forwards": {
          "1LKMInhzapHdurbaABaa50": "MZ9x",
          "MZ9x": "1LKMInhzapHdurbaABaa50"
        }
      }
    }
  ]
}
```

# Release 0.25.3

* Enhancement: Add cost and precedence to host.v1 and host.v2 config types. This allows router-embedded tunnelers the ability to handle HA failover scenarios.
* Bug fix: Router link listener type was only inferred from the adverise address, not the bind address

# Release 0.25.2

## Deprecations
The Ziti Edge management REST `/database` and `/terminators` endpoints are being deprecated. They belong in the 
fabric management API, but there was no fabric REST api at the time when they were added. Now that they are 
available under fabric, they will be removed from the edge APIs in a future release, v0.26 or later.

## What's New

* Enhancement: Only translate router ids -> names in `ziti edge traceroute` when requested to with flag
* Enhancement: Add the /database rest API from edge to fabric, where they below
    * `ziti fabric db` now as the same commands as `ziti edge db`
* Enhancement: Add `ziti agent` command for sending IPC commands. Contains copy of what was under `ziti ps`.
* Enhancement: Add `ziti agent controller snapshot-db <name or pid>` IPC command


# Release 0.25.1

* Bug fix: Fix panic caused by race condition at router start up
    * Regression since 0.25.0

# Release 0.25.0

## Breaking Changes
Routers with version 0.25.0 or greater must be used with a controller that is also v0.25 or greater. 
Controllers will continue to work with older routers. Router of this version should also continue to interoperate with older routers.

NOTE: You may be used to seeing two links between routers, if they both have link listeners. Starting with v0.25 expect to see only
a single link between routers, unless you use the new link types feature.

## What's New

* Bug fix: Fixed an issue with the ziti CLI quickstart routine which also affected router and controller config generation leaving many config fields blank or incorrect.
    * Note: This fix was previously reported to have been fixed in 0.24.13 but the fix was actually applied to this release.
* Enhancement: Router Link Refactor
    * Support for multiple link types
    * Existing link notifications
    * Link heartbeats/latency have changed
    * Inspect and ps support for links
    * Router version dissemination
    * Distributed control preparation
* Enhancement: `ziti fabric list routers` now includes the link listener types and advertise addresses

## Router Link Refactor

### Multiple Link Types 
Routers can now configure multiple link listeners. Listeners now support an option 'type' attribute. If no type is provided, the link type will be derived from the address. For example, given the following configuration:

```
link:
  dialers:
    - binding:          transport
  listeners:
    - binding:          transport
      bind:             tls:127.0.0.1:7878
      advertise:        tls:127.0.0.1:7878

    - binding:          transport
      bind:             tls:127.0.0.1:5876
      advertise:        tls:127.0.0.1:5876
      type: cellular
```

The first listener will have a type of `tls` and the second listener will have a type of `cellular`. 

Routers will now try to maintain one link of each type available on the target router.

When using `ziti fabric list links` the link type will now be shown.

### Existing link notifications
As the controller doesn't persist links, when the controller restarts or loses connection it loses all information about router links. Routers can now notify the controller about existing links when they reconnect. If they receive a link dial request for a link that they already have (based on the target router and link type), they can now report back the existing link. This should prevent the number of links to remain relatively constant.

### Link Heartbeats

Because we are now limiting the number of links it is even more vital to ensure that links are healthy, and to respond quickly when links become unresponsive. To that end links now use heartbeats. As data flows across the link, heartbeat headers will be added periodically. Heartbeat responses will be added to return messages. If the link is currently quiet, explicit heartbeat messages will be sent. Heartbeats will also be used to measure latency. If heartbeats are unreturned for a certain amount of time, the link will be considered bad and torn down, so a new one can be established.

The link.latency metric now is calculated starting when the message is about to be sent. It may have a few extra milliseconds time, as the response waits briefly to see if there's an existing message that the response can piggyback on.

Previously link.latency include both queue and network time. Now that it only has network time, there's a new metrics, `link.queue_time` which tracks how long it takes messages to get from send requested to just before send.

### Inspect and ps support for links

`ziti fabric inspect .* links` can now be used to see what links each router knows about. This can be useful to determine if/how the controller and routers may have gotten out of sync.

Router can also be interrogated directly for their links via IPC, using `ziti ps`. 

```
$ ziti ps router dump-links 275061
id: 4sYO18tZ1Fz4HByXuIp1Dq dest: o.oVU2Qm. type: tls
id: 19V7yhjBpHAc2prTDiTihQ dest: hBjIP2wmxj type: tls
```

### Router version dissemination

Routers now get the version of the router they are dialing a link to, and pass their own version to that router as part of the dial. This allows routers to only enable specific features if both sides of the link support it.

### Distributed Control preparation

Giving the routers have more control over the links prepares us for a time when routers may be connected to multiple controllers. Routers will be able to notify controllers of existing links and will be prepared to resolve duplicate link dial requests from multiple sources.
