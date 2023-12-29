# Release 0.22.11

## What's New

* Feature: API Session Events

## API Session Events

API Session events can now be configured by adding `edge.apiSessions` under event subscriptions. The events may be of
type `created` and `deleted`. The event type can be filtered by adding an `include:` block, similar to edge sessions.

The JSON output looks like:

```
{
  "namespace": "edge.apiSessions",
  "event_type": "created",
  "id": "ckvr2r4fs0001oigd6si4akc8",
  "timestamp": "2021-11-08T14:45:45.785561479-05:00",
  "token": "77cffde5-f68e-4ef0-bbb5-731db36145f5",
  "identity_id": "76BB.shC0",
  "ip_address": "127.0.0.1"
}
```

# Release 0.22.10

# What's New

* Bug fix: address client certificate changes altered by library changes
* Bug fix: fixes a panic on session read in some situations
* Enhancement: Certificate Authentication Extension provides the ability to extend certificate expiration dates in the
  Edge Client and Management APIs

## Certificate Authentication Extension

The Edge Client and Management APIs have had the following endpoint added:

- `POST /current-identity/authenticators/{id}/extend`

It is documented as:

```
Allows an identity to extend its certificate's expiration date by
using its current and valid client certificate to submit a CSR. This CSR may
be passed in using a new private key, thus allowing private key rotation.

After completion any new connections must be made with certificates returned from a 200 OK
response. The previous client certificate is rendered invalid for use with the controller even if it
has not expired.

This request must be made using the existing, valid, client certificate.
```

An example input is:

```
{
    "clientCertCsr": "...<csr>..."
}
```

Output responses include:

- `200 OK` w/ empty object payloads: `{}`
- `401 UNAUTHORIZED` w/ standard error messaging
- `400 BAD REQUEST` w/ standard error messaging for field errors or CSR processing errors

# Release 0.22.9

# What's New

* Build: This release adds an arm64 build and improved docker build process

# Release 0.22.8

# What's New

* Bug fix: Workaround bbolt bug where cursor next sometimes skip when current is deleted. Use skip instead of next.
  Fixes orphan session issue.
* Bug fix: If read fails on reconnecting channel, close peer before trying to reconnect
* Bug fix: Don't log every UDP datagram at info level in tunneler
* Change: Build with -trimpath to aid in plugin compatibility

# Release 0.22.7

# What's New

* Bug fix: Router automatic certificate enrollments will no longer require a restart of the router
* Enhancement: foundation Identity implementations now support reloading of tls.Config certificates for CAs
* Enhancement: foundation Identity library brought more in-line with golang idioms
* Experimental: integration with PARSEC key service
* Bug fix: Fix controller panic when router/tunnel tries to host invalid service

## PARSEC integration (experimental)

Ziti can now use keys backed by PARSEC service for identity. see https://parallaxsecond.github.io/parsec-book/index.html

example usage during enrollment (assuming `my-identity-key` exists in PARSEC service):

```
$ ziti-tunnel enroll -j my-identity.jwt --key parsec:my-identity-key
```

# Release 0.22.6

# What's New

* Enhancement: Add terminator_id and version to service events. If a service event relates to a terminator, the
  terminator_id will now be included. Service events now also have a version field, which is set to 2.
* Enhancement: Don't let identity/service/edge router role attributes start with a hashtag or at-symbol to prevent
  confusion.
* Bug fix: Timeout remaining for onWake/onUnlock will properly report as non-zero after MFA submission
* Enhancement: traceroute support
* Enhancement: add initial support for UDP links

## Traceroute

The Ziti cli and Ziti Golang SDK now support traceroute style operations. In order for this to work the SDK and routers
must be at version 0.22.6 or greater. This is currently only supported in the Golang SDK.

The SDK can perform a traceroute as follows:

```
conn, err := ctx.Dial(o.Args[0])
result, err := conn.TraceRoute(hop, time.Second*5)
```

The result structure looks like:

```
type TraceRouteResult struct {
    Hops    uint32
    Time    time.Duration
    HopType string
    HopId   string
}
```

Increasing numbers of hops can be requested until the hops returned is greater than zero, indicating that additional
hops weren't available. This functionality is available in the Ziti CLI.

```
$ ziti edge traceroute simple -c ./simple-client.json 
 1               xgress/edge    1ms 
 2     forwarder[n4yChTL3Jy]     0s 
 3     forwarder[Yv7BPW0kGR]     0s 
 4               xgress/edge    1ms 
 5                sdk/golang     0s 

plorenz@carrot:~/work/nf$ ziti edge traceroute simple -c ./simple-client.json 
 1               xgress/edge     0s 
 2     forwarder[n4yChTL3Jy]     0s 
 3     forwarder[Yv7BPW0kGR]    1ms 
 4     xgress/edge_transport     0s 
```

# Release 0.22.5

## What's New

* Update from Go 1.16 to Go 1.17

# Release 0.22.4

## What's New

* Bug fix: Ziti CLI creating a CA now has the missing `--identity-name-format` / `-f` option
* Bug fix: Edge router/tunneler wasn't getting per-service precedence/cost defined on identity
* Cleanup: The HA terminator strategy has been removed. The implementation was incomplete on its own. Use health checks
  instead of active/passive setups

# Release 0.22.3

## What's New

* Bug fix: Fix panic in listener close if the socket hadn't been initialized yet
* Bug fix: Fix panic in posture bulk create if mfa wasn't set
* Bug fix: Fix panic in circuit creation on race condition when circuits are add/removed concurrently

# Release 0.22.2

## What's New

* Bug fix: Upgrading a controller from 0.22.0 or earlier to 0.22.2 will no longer leave old sessions w/o identityId
  properties. Workaround for previous versions is to use `ziti-controller delete-sessions`
* Bug fix: If a router/tunneler loses connectivity with the controller long enough for the api session to time out, the
  router will now restablish any terminators for hosted services
* Enhancement: Add some short aliases for the CLI
    * edge-router -> er
    * service-policy -> sp
    * edge-router-policy -> erp
    * service-edge-router-policy -> serp
* Feature: Add GetServiceTerminators to Golang SDK ziti.Context
* Feature: Add GetSourceIdentifier to Golang SDK edge.ServiceConn

# Release 0.22.1

## What's New

* Bug fix: Fabric v0.16.93 fixes `xgress.GetCircuit` to provide a `ctrl not ready` error response when requests arrive
  before the router is fully online.
* Bug fix: Ziti CLI will no longer truncate paths on logins with explicit URLs
* Bug fix: Ziti CLI will now correctly check the proper lengths of sha512 hashes in hex format
* Bug fix: MFA Posture Check timeout will no longer be half their set value
* Bug fix: MFA Posture Checks w/ a timeout configured to 0 will be treated as having no timeout (-1) instead of always
  being timed out
* Bug fix: MFA Posture Checks will no longer cause an usually high frequency of session updates
* Bug fix: MFA Posture Checks during subsequent MFA submissions will no longer 401
* Bug fix: Listing sessions via `GET /sessions` will no longer report an error in certain data states
* Feature: Posture responses now report services affected with timeout/state changes
* Feature: Ziti CLI `unwrap` command for identity json files will now default the output file names
* Feature: Ziti CLI improvements
    * New interactive tutorial covering creating your first service. Run using: `ziti edge tutorial first-service`
    * You can now delete multiple entities at once, by providing multiple ids. Ex: `ziti edge delete services one two`
      or `ziti edge delete service one two` will both work.
    * You can now delete multiple entities at once, by providing a filter.
      Ex: `ziti edge delete services where 'name contains "foo"`
    * Create and delete output now has additional context.
* Feature: Terminators can now be filtered by service and router name:
  Ex: `ziti edge list terminators 'service.name = "echo"'`
* Feature: New event type `edge.entityCounts`

## Entity Count Events

The Ziti Controller can now generate events with a summary of how many of each entity type are currently in the data
store. It can be configured with an interval for how often the event will be generated. The default interval is five
minutes.

```
events:
  jsonLogger:
    subscriptions:
      - type: edge.entityCounts
        interval: 5m
```

Here is an example of the JSON output of the event:

```
{
  "namespace": "edge.entityCounts",
  "timestamp": "2021-08-19T13:39:54.056181406-04:00",
  "counts": {
    "apiSessionCertificates": 0,
    "apiSessions": 9,
    "authenticators": 4,
    "cas": 0,
    "configTypes": 5,
    "configs": 2,
    "edgeRouterPolicies": 4,
    "enrollments": 0,
    "eventLogs": 0,
    "geoRegions": 17,
    "identities": 6,
    "identityTypes": 4,
    "mfas": 0,
    "postureCheckTypes": 5,
    "postureChecks": 0,
    "routers": 2,
    "serviceEdgeRouterPolicies": 2,
    "servicePolicies": 5,
    "services": 3,
    "sessions": 0
  },
  "error": ""
}
```

# Release 0.22.0

## What's New

* Refactor: Fabric Sessions renamed to Circuits (breaking change)
* Feature: Links will now wait for a timeout for retrying
* Bug fix: Sessions created on the controller when circuit creation fails are now cleaned up
* Feature: Enhanced `ziti` CLI login functionality (has breaking changes to CLI options)
* Feature: new `ziti edge list summary` command, which shows database entity counts
* Bug fix: ziti-fabric didn't always report an error to the OS when it had an error
* Refactor: All protobuf packages have been prefixed with `ziti.` to help prevent namespace clashes. Should not be a
  breaking change.
* Feature: Selective debug logging by identity for path selection and circuit establishment
    * `ziti edge trace identity <identity id>` will turn on debug logging for selecting paths and establishing circuits
    * Addition context for these operations including circuitId, sessionid and apiSessionId should now be in log
      messages regardless of whether tracing is enabled
    * Tracing is enabled for a given duration, which defaults to 10 minutes

## Breaking Changes

Fabric sessions renamed to circuits. External integrators may be impacted by changes to events. See below for details.

### Ziti CLI

Commands under `ziti edge` now reserve the `-i` flag for specifying client identity. Any command line argument which
previously had a `-i` short version now only has a long version.

For consistency, policy roles parameters must all be specified in long form

This includes the following flags:

* ziti edge create edge-router-policy --identity-roles --edge-router-roles
* ziti edge update edge-router-policy --identity-roles --edge-router-roles
* ziti edge create service-policy --identity-roles --service-roles
* ziti edge update service-policy --identity-roles --service-roles
* ziti edge create service-edge-router-policy --service-roles --edge-router-roles
* ziti edge update service-edge-router-policy --service-roles --edge-router-roles
* ziti edge create posture-check mfa --ignore-legacy
* ziti edge update posture-check mfa --ignore-legacy
* ziti edge update authenticator updb --identity
* ziti edge update ca --identity-atributes (now -a)

The `ziti edge` commands now store session credentials in a new location and new format. Existing sessions will be
ignored.

The `ziti edge controller` command was previously deprecated and has now been removed. All commands that were previously
available under `ziti edge controller` are available under `ziti edge`.

## Fabric Sessions renamed to Circuits

Previously we had three separate entities named session: fabric sessions, edge sessions and edge API sessions. In order
to reduce confusion, fabric sessions have been renamed to circuits. This has the following impacts:

* ziti-fabric CLI
    * `list sessions` renamed to `list circuits`
    * `remove session` renamed to `remove circuit`
    * `stream sessions` renamed to `stream circuits`
* Config properties
    * In the controller config, under `networks`, `createSessionRetries` is now `createCircuitRetries`
    * In the router config, under xgress dialer/listener options, `getSessionTimeout` is now `getCircuitTimeout`
    * In the router config, under xgress dialer/listener options, `sessionStartTimeout` is now `circuitStartTimeout`
    * In the router, under `forwarder`, `idleSessionTimeout` is now `idleCircuitTimeout`

In the context of the fabric there was an existing construct call `Circuit` which has now been renamed to `Path`. This
may be visible in a few `ziti-fabric` CLI outputs

### Event changes

Previously the fabric had session events. It now has circuit events instead. These events have the `fabric.circuits`
namespace. The `circuitUpdated` event type is now the `pathUpdated` event.

```
type CircuitEvent struct {
	Namespace string    `json:"namespace"`
	EventType string    `json:"event_type"`
	CircuitId string    `json:"circuit_id"`
	Timestamp time.Time `json:"timestamp"`
	ClientId  string    `json:"client_id"`
	ServiceId string    `json:"service_id"`
	Path      string    `json:"circuit"`
}
```

Additionally the Usage events now have `circuit_id` instead of `session_id`. The usage events also have a new `version`
field, which is set to 2.

# Pending Link Timeout

Previously whenever a router connected we'd look for new links possibilities and create new links between routers where
any were missing. If lots of routers connected at the same time, we might create duplicate links because the links
hadn't been reported as established yet. Now we'll checking for links in Pending state, and if they haven't hit a
configurable timeout, we won't create another link.

The new config property is `pendingLinkTimeoutSeconds` in the controller config file under `network`, and defaults to 10
seconds.

## Enhanced CLI Login Functionality

### Server Trust

#### Untrusted Servers

If you don't provide a certificates file when logging in, the server's well known certificates will now be pulled from
the server and you will be prompted if you want to use them. If certs for the host have previously been retrieved they
will be used. Certs stored locally will be checked against the certs on the server when logging in. If a difference is
found, the user will be notified and asked if they want to update the local certificate cache.

If you provide certificates during login, the server's certificates will not be checked or downloaded. Locally cached
certificates for that host will not be used.

#### Trusted Servers

If working with a server which is using certs that your OS already recognizes, nothing will change. No cert needs to be
provided and the server's well known certs will not be downloaded.

### Identities

The Ziti CLI now supports multiple identities. An identity can be specified using `--cli-identity` or `-i`.

Example commands:

```
$ ziti edge login -i dev localhost:1280
Enter username: admin
Enter password: 
Token: 76ff81b4-b528-4e2c-ad73-dcb0a39b6489
Saving identity 'dev' to ~/.config/ziti/ziti-cli.json

$ ziti edge -i dev list services
id: -JucPW0kGR    name: ssh    encryption required: true    terminator strategy: smartrouting    role attributes: ["ssh"]
results: 1-1 of 1
```

If no identity is specified, a default will be used. The default identity is `default`.

#### Switching Default Identity

The default identity can be changed with the `ziti edge use` command.

The above example could also be accomplished as follows:

```
$ ziti edge use dev
Setting identity 'dev' as default in ~/.config/ziti/ziti-cli.json

$ ziti edge login localhost:1280
Enter username: admin
Enter password: 
Token: e325d91c-a452-4454-a733-cfad88bfa356
Saving identity 'dev' to ~/.config/ziti/ziti-cli.json

$ ziti edge list services
id: -JucPW0kGR    name: ssh    encryption required: true    terminator strategy: smartrouting    role attributes: ["ssh"]
results: 1-1 of 1

$ ziti edge use default
Setting identity 'default' as default in ~/.config/ziti/ziti-cli.json
```

`ziti edge use` without an argument will list logins you have made.

```
$ ziti edge use
id:      default | current:  true | read-only:  true | urL: https://localhost:1280/edge/management/v1
id:        cust1 | current: false | read-only: false | urL: https://customer1.com:443/edge/management/v1
```

#### Logout

You can now also clear locally stored credentials using `ziti edge logout`

```
$ ziti edge -i cust1 logout  
Removing identity 'cust1' from ~/.config/ziti/ziti-cli.json
```

#### Read-Only Mode

When logging in one can mark the identity as read-only. This is a client side enforced flag which will attempt to make
sure only read operations are performed by this session.

```
$ ziti edge login --read-only localhost:1280
Enter username: admin
Enter password: 
Token: 966192c6-fb7f-481e-8230-dcef157770ef
Saving identity 'default' to ~/.config/ziti/ziti-cli.json

$ ziti edge list services
id: -JucPW0kGR    name: ssh    encryption required: true    terminator strategy: smartrouting    role attributes: ["ssh"]
results: 1-1 of 1

$ ziti edge create service test
error: this login is marked read-only, only GET operations are allowed
```

NOTE: This is not guaranteed to prevent database changes. It is meant to help prevent accidental changes, if the wrong
profile is accidentally used. Caution should always be exercised when working with sensitive data!

#### Login via Token

If you already have an API session token, you can use that to create a client identity using the new `--token` flag.
When using `--token` the saved identity will be marked as read-only unless `--read-only=false` is specified. This is
because if you only have a token and not full credentials, it's more likely that you're inspecting a system to which you
have limited privileges.

```
$ ziti edge login localhost:1280 --token c9f37575-f660-409b-b731-5a256d74a931
NOTE: When using --token the saved identity will be marked as read-only unless --read-only=false is provided
Saving identity 'default' to ~/.config/ziti/ziti-cli.json
```

Using this option will still check the server certificates to see if they need to be downloaded and/or compare them with
locally cached certificates.
