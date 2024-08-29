# Release 1.1.9

## What's New

* Bug fixes, enhancements and continuing progress on controller HA
* Includes a performance update ([Issue #2340](https://github.com/openziti/ziti/issues/2340)) which should improve
  connection ramp times. Previously circuits would start with a relatively low window size and ramp slowly. Circuits
  will now start with a large initial window size and scale back if they can't keep up
* Added `ziti ops verify-network`. A command to aid when configuring the overlay network, this command will check config
  files for obvious mistakes
* Added `ziti ops verify-traffic`. Another command to aid when configuring the overlay network, this command will ensure
  the overlay network is able to pass traffic

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.16 -> v1.0.17](https://github.com/openziti/agent/compare/v1.0.16...v1.0.17)
* github.com/openziti/channel/v2: [v2.0.136 -> v2.0.143](https://github.com/openziti/channel/compare/v2.0.136...v2.0.143)
    * [Issue #136](https://github.com/openziti/channel/issues/136) - Fix timeout on classic dialer 
    * [Issue #134](https://github.com/openziti/channel/issues/134) - Support the dtls transport

* github.com/openziti/edge-api: [v0.26.23 -> v0.26.25](https://github.com/openziti/edge-api/compare/v0.26.23...v0.26.25)
* github.com/openziti/foundation/v2: [v2.0.47 -> v2.0.48](https://github.com/openziti/foundation/compare/v2.0.47...v2.0.48)
* github.com/openziti/identity: [v1.0.81 -> v1.0.84](https://github.com/openziti/identity/compare/v1.0.81...v1.0.84)
* github.com/openziti/metrics: [v1.2.56 -> v1.2.57](https://github.com/openziti/metrics/compare/v1.2.56...v1.2.57)
* github.com/openziti/runzmd: [v1.0.49 -> v1.0.50](https://github.com/openziti/runzmd/compare/v1.0.49...v1.0.50)
* github.com/openziti/sdk-golang: [v0.23.39 -> v0.23.40](https://github.com/openziti/sdk-golang/compare/v0.23.39...v0.23.40)
    * [Issue #601](https://github.com/openziti/sdk-golang/issues/601) - Only send config types on service list if controller version supports it

* github.com/openziti/transport/v2: [v2.0.138 -> v2.0.143](https://github.com/openziti/transport/compare/v2.0.138...v2.0.143)
    * [Issue #85](https://github.com/openziti/transport/issues/85) - Update to latest dtls library

* github.com/openziti/ziti: [v1.1.8 -> v1.1.9](https://github.com/openziti/ziti/compare/v1.1.8...v1.1.9)
    * [Issue #2343](https://github.com/openziti/ziti/issues/2343) - Transferring leadership to another controller fails sometimes
    * [Issue #2340](https://github.com/openziti/ziti/issues/2340) - Update xgress defaults
    * [Issue #2336](https://github.com/openziti/ziti/issues/2336) - Re-enable optional xgress payload mtu, with message framing
    * [Issue #2091](https://github.com/openziti/ziti/issues/2091) - Add `scope` and `cliend_id` configuration to ext jwt signers
    * [Issue #2318](https://github.com/openziti/ziti/issues/2318) - Unable to update appData on existing edge routers using PATCH
    * [Issue #2281](https://github.com/openziti/ziti/issues/2281) - Session Certificates Should Return a Chain
    * [Issue #2285](https://github.com/openziti/ziti/issues/2285) - routers sometimes report link metrics for closed links 
    * [Issue #2282](https://github.com/openziti/ziti/issues/2282) - Investigate OIDC TOTP Redirect w/ application/json
    * [Issue #2279](https://github.com/openziti/ziti/issues/2279) - Ensure xweb initialized before RAFT
    * [Issue #2277](https://github.com/openziti/ziti/issues/2277) - docker controller and router deployments - generate a config by default
    * [Issue #2154](https://github.com/openziti/ziti/issues/2154) - HA MFA Enrollment returns 500
    * [Issue #2159](https://github.com/openziti/ziti/issues/2159) - API Session in HA return 400

# Release 1.1.8

## What's New

* Bug fixes, enhancements and continuing progress on controller HA

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.20 -> v0.26.23](https://github.com/openziti/edge-api/compare/v0.26.20...v0.26.23)
    * [Issue #120](https://github.com/openziti/edge-api/issues/120) - Add API for retrieving services referencing a config
    * [Issue #121](https://github.com/openziti/edge-api/issues/121) - Add API for retrieving the set of attribute roles used by posture checks

* github.com/openziti/sdk-golang: [v0.23.38 -> v0.23.39](https://github.com/openziti/sdk-golang/compare/v0.23.38...v0.23.39)
    * [Issue #596](https://github.com/openziti/sdk-golang/issues/596) - SDK should submit selected config types to auth and service list APIs
    * [Issue #593](https://github.com/openziti/sdk-golang/issues/593) - SDK Golang OIDC Get API Session Returns Wrong Value

* github.com/openziti/storage: [v0.2.47 -> v0.3.0](https://github.com/openziti/storage/compare/v0.2.47...v0.3.0)
    * [Issue #80](https://github.com/openziti/storage/issues/80) - Set indexes aren't cleaned up when referenced entities are deleted, only when they change
    * [Issue #78](https://github.com/openziti/storage/issues/78) - Allow searching for things without case sensitivity

* github.com/openziti/ziti: [v1.1.7 -> v1.1.8](https://github.com/openziti/ziti/compare/v1.1.7...v1.1.8)
    * [Issue #2121](https://github.com/openziti/ziti/issues/2121) - Use router data model for edge router tunnel
    * [Issue #2245](https://github.com/openziti/ziti/issues/2245) - Add ability to retrieve a list of services that reference a config
    * [Issue #2089](https://github.com/openziti/ziti/issues/2089) - Enhance Management API to list Posture Check Roles
    * [Issue #2209](https://github.com/openziti/ziti/issues/2209) - `/edge/v1/external-jwt-signers` needs to be open
    * [Issue #2010](https://github.com/openziti/ziti/issues/2010) - Add config information to router data model
    * [Issue #1990](https://github.com/openziti/ziti/issues/1990) - Implement subscriber model for identity/service events in router
    * [Issue #2240](https://github.com/openziti/ziti/issues/2240) - Secondary ext-jwt Auth Policy check incorrectly requires primary ext-jwt auth to be enabled


# Release 1.1.7

## What's New

* Release actions fixes
* Fix for a flaky acceptance test

# Release 1.1.6

## What's New

* Trust Domain Configuration
* Controller HA Beta 2

## Trust Domain Configuration

OpenZiti controllers from this release forward will now require a `trust domain` to be configured. 
High Availability (HA) controllers already have this requirement. HA Controllers configure their trust domain via SPIFFE 
ids that are embedded in x509 certificates.

For feature parity, non-HA controllers will now have this same requirement. However, as re-issuing certificates is not
always easily done. To help with the transition, non-HA controllers will have the ability to have their trust domain
sourced from the controller configuration file through the root configuration value `trustDomain`. The configuration
field which takes a string that must be URI hostname compatible (see: https://github.com/spiffe/spiffe/blob/main/standards/SPIFFE-ID.md).
If this value is not defined, a trust domain will be generated from the root CA certificate of the controller. 

For networks that will be deployed after this change, it is highly suggested that a SPIFFE id is added to certificates.
The `ziti pki create ...` tooling supports the `--spiffe-id` option to help handle this scenario.

### Generated Trust Domain Log Messages

The following log messages are examples of warnings produced when a controller is using a generated trust domain:

```
WARNING this environment is using a default generated trust domain [spiffe://d561decf63d229d66b07de627dbbde9e93228925], 
  it is recommended that a trust domain is specified in configuration via URI SANs or the 'trustDomain' field

WARNING this environment is using a default generated trust domain [spiffe://d561decf63d229d66b07de627dbbde9e93228925], 
  it is recommended that if network components have enrolled that the generated trust domain be added to the 
  configuration field 'additionalTrustDomains'
```

### Trust domain resolution:

- Non-HA controllers
  - Prefers SPIFFE ids in x509 certificate URI SANs, looking at the leaf up the signing chain
  - Regresses to `trustDomain` in the controller configuration file if not found
  - Regress to generating a trust domain from the server certificates root CA, if the above do not resolve

- HA Controllers
  - Requires x509 SPIFFE ids in x509 certificate URI SANs

### Additional Trust Domains

When moving between trust domains (i.e. from the default generated to a new named one), the controller supports having
other trust domains. The trust domains do not replace certificate chain validation, which is still checked and enforced.

Additional trust domains are configured in the controller configuration file under the root field 
`additionalTrustDomains`. This field is an array of hostname safe strings.

The most common use case for this is field is if a network has issued certificates using the generated trust domain and 
now wants to transition to a explicitly defined one.

## Controller HA Beta 2

This release can be run in HA mode. The code is still beta, as we're still finding and fixing bugs. Several bugs
have been fixed since Beta 1 and c-based SDKs and tunnelers now work in HA mode. The smoketest can now be run
with HA controllers and clients.

* Latest ZET release supporting HA control: https://github.com/openziti/ziti-tunnel-sdk-c/releases/tag/v2.0.0-alpha9
* Windows, Mac and Mobile clients are in the process of being updated

For more information:

* HA overview/getting started/migration: [HA Documentation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
* Open Issues: [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1)

## Component Updates and Bug Fixes

* github.com/openziti/storage: [v0.2.45 -> v0.2.46](https://github.com/openziti/storage/compare/v0.2.45...v0.2.46)
    * [Issue #76](https://github.com/openziti/storage/issues/76) - Add support for non-boltz symbols to the the boltz stores

* github.com/openziti/ziti: [v1.1.5 -> v1.1.6](https://github.com/openziti/ziti/compare/v1.1.5...v1.1.6)
    * [Issue #2171](https://github.com/openziti/ziti/issues/2171) - Routers should consider control channels unresponsive if they are not connected
    * [Issue #2219](https://github.com/openziti/ziti/issues/2219) - Add inspection for router connections
    * [Issue #2195](https://github.com/openziti/ziti/issues/2195) - cached data model file set to
    * [Issue #2222](https://github.com/openziti/ziti/issues/2222) - Add way to get read-only status from cluster nodes
    * [Issue #2191](https://github.com/openziti/ziti/issues/2191) - Change raft list cluster members element name from values to data to match rest of REST api
    * [Issue #785](https://github.com/openziti/ziti/issues/785) - ziti edge update service-policy to empty/no posture checks fails
    * [Issue #2205](https://github.com/openziti/ziti/issues/2205) - Merge fabric and edge model code
    * [Issue #2165](https://github.com/openziti/ziti/issues/2165) - Add network id

# Release 1.1.5

## What's New

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.133 -> v2.0.136](https://github.com/openziti/channel/compare/v2.0.133...v2.0.136)
    * [Issue #132](https://github.com/openziti/channel/issues/132) - reconnecting dialer doesn't take local binding into account when reconnecting

* github.com/openziti/identity: [v1.0.80 -> v1.0.81](https://github.com/openziti/identity/compare/v1.0.80...v1.0.81)
* github.com/openziti/transport/v2: [v2.0.136 -> v2.0.138](https://github.com/openziti/transport/compare/v2.0.136...v2.0.138)
    * [Issue #83](https://github.com/openziti/transport/issues/83) - tls.Dial should use proxy configuration if provided

* github.com/openziti/xweb/v2: [v2.1.0 -> v2.1.1](https://github.com/openziti/xweb/compare/v2.1.0...v2.1.1)
* github.com/openziti/ziti: [v1.1.4 -> v1.1.5](https://github.com/openziti/ziti/compare/v1.1.4...v1.1.5)
    * [Issue #2173](https://github.com/openziti/ziti/issues/2173) - panic on HA peer connect
    * [Issue #2171](https://github.com/openziti/ziti/issues/2171) - Routers should consider control channels unresponsive if they are not connected
    * [Issue #2086](https://github.com/openziti/ziti/issues/2086) - Enable File Watching for Router/Controller Identities
    * [Issue #2087](https://github.com/openziti/ziti/issues/2087) - Ext JWT not setting provider value in auth query

# Release 1.1.4

## What's New

* Controller HA Beta 1
* Bug fixes

## Controller HA Beta 1

This release can be run in HA mode. The code is still beta, as we're still finding and fixing bugs. Several bugs 
have been fixed since Alpha 3 and c-based SDKs and tunnelers now work in HA mode. The smoketest can now be run
with HA controllers and clients.

* Initial ZET release support HA control: https://github.com/openziti/ziti-tunnel-sdk-c/releases/tag/v2.0.0-alpha1
* Windows, Mac and Mobile clients are in the process of being updated

For more information:

* HA overview/getting started/migration: [HA Documentation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
* Open Issues: [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1)

## Component Updates and Bug Fixes 

* github.com/openziti/channel/v2: [v2.0.130 -> v2.0.133](https://github.com/openziti/channel/compare/v2.0.130...v2.0.133)
* github.com/openziti/edge-api: [v0.26.19 -> v0.26.20](https://github.com/openziti/edge-api/compare/v0.26.19...v0.26.20)
    * [Issue #113](https://github.com/openziti/edge-api/issues/113) - RecoveryCodesEnvelope is wrong

* github.com/openziti/foundation/v2: [v2.0.45 -> v2.0.47](https://github.com/openziti/foundation/compare/v2.0.45...v2.0.47)
    * [Issue #407](https://github.com/openziti/foundation/issues/407) - Remove Branch from build info

* github.com/openziti/identity: [v1.0.77 -> v1.0.80](https://github.com/openziti/identity/compare/v1.0.77...v1.0.80)
* github.com/openziti/metrics: [v1.2.54 -> v1.2.56](https://github.com/openziti/metrics/compare/v1.2.54...v1.2.56)
* github.com/openziti/runzmd: [v1.0.47 -> v1.0.49](https://github.com/openziti/runzmd/compare/v1.0.47...v1.0.49)
* github.com/openziti/sdk-golang: [v0.23.37 -> v0.23.38](https://github.com/openziti/sdk-golang/compare/v0.23.37...v0.23.38)
    * [Issue #573](https://github.com/openziti/sdk-golang/issues/573) - api session refresh spins in a tight loop if there is no current api session
    * [Issue #562](https://github.com/openziti/sdk-golang/issues/562) - Support sticky dials

* github.com/openziti/secretstream: [v0.1.20 -> v0.1.21](https://github.com/openziti/secretstream/compare/v0.1.20...v0.1.21)
* github.com/openziti/storage: [v0.2.41 -> v0.2.45](https://github.com/openziti/storage/compare/v0.2.41...v0.2.45)
    * [Issue #73](https://github.com/openziti/storage/issues/73) - db integrity checker doesn't take nullable flag into account when checking unique indices
    * [Issue #71](https://github.com/openziti/storage/issues/71) - Add AddFkIndexCascadeDelete

* github.com/openziti/transport/v2: [v2.0.133 -> v2.0.136](https://github.com/openziti/transport/compare/v2.0.133...v2.0.136)
* github.com/openziti/ziti: [v1.1.3 -> v1.1.4](https://github.com/openziti/ziti/compare/v1.1.3...v1.1.4)
    * [Issue #2084](https://github.com/openziti/ziti/issues/2084) - Bug: Router enrollment is missing its server chain
    * [Issue #2124](https://github.com/openziti/ziti/issues/2124) - api session certs should be deleted when related api sessions are deleted

# Release 1.1.3

## What's New

* Sticky Terminator Selection
* Linux and Docker deployments log formats no longer default to the simplified format option and now use logging library
  defaults: `json` for non-interactive, `text` for interactive.

NOTE: This release is the first since 1.0.0 to be marked promoted from pre-release. Be sure to check the release notes
      for the rest of the post-1.0.0 releases to get the full set of changes.

## Stick Terminator Strategy

This release introduces a new terminator selection strategy `sticky`. On every dial it will return a token to the 
dialer, which represents the terminator used in the dial. This token maybe passed in on subsequent dials. If no token
is passed in, the strategy will work the same as the `smartrouting` strategy. If a token is passed in, and the 
terminator is still valid, the same terminator will be used for the dial. A terminator will be consideder valid if
it still exists and there are no terminators with a higher precedence. 

This is currently only supported in the Go SDK.

### Go SDK Example

```
ziti edge create service test --terminator-strategy sticky
```

```
	conn := clientContext.Dial("test")
	token := conn.Conn.GetStickinessToken()
	_ = conn.Close()

	dialOptions := &ziti.DialOptions{
		ConnectTimeout:  time.Second,
		StickinessToken: token,
	}
	conn = clientContext.DialWithOptions("test", dialOptions))
	nextToken := conn.Conn.GetStickinessToken()
	_ = conn.Close()
```

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.128 -> v2.0.130](https://github.com/openziti/channel/compare/v2.0.128...v2.0.130)
* github.com/openziti/edge-api: [v0.26.18 -> v0.26.19](https://github.com/openziti/edge-api/compare/v0.26.18...v0.26.19)
* github.com/openziti/foundation/v2: [v2.0.42 -> v2.0.45](https://github.com/openziti/foundation/compare/v2.0.42...v2.0.45)
* github.com/openziti/identity: [v1.0.75 -> v1.0.77](https://github.com/openziti/identity/compare/v1.0.75...v1.0.77)
* github.com/openziti/metrics: [v1.2.51 -> v1.2.54](https://github.com/openziti/metrics/compare/v1.2.51...v1.2.54)
* github.com/openziti/runzmd: [v1.0.43 -> v1.0.47](https://github.com/openziti/runzmd/compare/v1.0.43...v1.0.47)
* github.com/openziti/sdk-golang: [v0.23.35 -> v0.23.37](https://github.com/openziti/sdk-golang/compare/v0.23.35...v0.23.37)
    * [Issue #562](https://github.com/openziti/sdk-golang/issues/562) - Support sticky dials

* github.com/openziti/secretstream: [v0.1.19 -> v0.1.20](https://github.com/openziti/secretstream/compare/v0.1.19...v0.1.20)
* github.com/openziti/storage: [v0.2.37 -> v0.2.41](https://github.com/openziti/storage/compare/v0.2.37...v0.2.41)
* github.com/openziti/transport/v2: [v2.0.131 -> v2.0.133](https://github.com/openziti/transport/compare/v2.0.131...v2.0.133)
* github.com/openziti/ziti: [v1.1.2 -> v1.1.3](https://github.com/openziti/ziti/compare/v1.1.2...v1.1.3)
    * [Issue #2064](https://github.com/openziti/ziti/issues/2064) - Fix panic on link close
    * [Issue #2062](https://github.com/openziti/ziti/issues/2062) - Link connection retry delays should contain some randomization 
    * [Issue #2055](https://github.com/openziti/ziti/issues/2055) - Controller panics on 'ziti agent cluster list'
    * [Issue #2019](https://github.com/openziti/ziti/issues/2019) - Support mechanism for sticky dials

# Release 1.1.2

## What's New

* Bug fixes and minor enhancements

## Component Updates and Bug Fixes
* github.com/openziti/sdk-golang: [v0.23.32 -> v0.23.35](https://github.com/openziti/sdk-golang/compare/v0.23.32...v0.23.35)
* github.com/openziti/ziti: [v1.1.1 -> v1.1.2](https://github.com/openziti/ziti/compare/v1.1.1...v1.1.2)
  * [Issue #2032](https://github.com/openziti/ziti/issues/2032) - Auto CA Enrollment Fails w/ 400 Bad Request
  * [Issue #2026](https://github.com/openziti/ziti/issues/2026) - Root Version Endpoint Handling 404s
  * [Issue #2002](https://github.com/openziti/ziti/issues/2002) - JWKS endpoints may not refresh on new KID
  * [Issue #2007](https://github.com/openziti/ziti/issues/2007) - Identities for edge routers with tunneling enabled sometimes show hasEdgeRouterConnection=false even though everything is OK
  * [Issue #1983](https://github.com/openziti/ziti/issues/1983) - delete of non-existent entity causes panic when run on follower controller


# Release 1.1.1

## What's New

* HA Alpha-3
* Bug fixes and minor enhancements
* [The all-in-one quickstart compose project](./quickstart/docker/all-in-one/README.md) now uses the same environment variable to configure the controller's address as the ziti command line tool

## HA Alpha 3

This release can be run in HA mode. The code is still alpha, as we're still finding and fixing bugs. 

For more information:

* HA overview/getting started/migration: [HA Documementation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
* Open Issues: [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1) 

## New Contributors

Thanks to new contributors

* @Vrashabh-Sontakke

## Component Updates and Bug Fixes
* github.com/openziti/edge-api: [v0.26.17 -> v0.26.18](https://github.com/openziti/edge-api/compare/v0.26.17...v0.26.18)
* github.com/openziti/sdk-golang: [v0.23.27 -> v0.23.32](https://github.com/openziti/sdk-golang/compare/v0.23.27...v0.23.32)
    * [Issue #554](https://github.com/openziti/sdk-golang/issues/554) - Passing in config types on service list breaks on older controller

* github.com/openziti/storage: [v0.2.36 -> v0.2.37](https://github.com/openziti/storage/compare/v0.2.36...v0.2.37)
    * [Issue #64](https://github.com/openziti/storage/issues/64) - Add support for transaction complete listeners

* github.com/openziti/ziti: [v1.1.0 -> v1.1.1](https://github.com/openziti/ziti/compare/v1.1.0...v1.1.1)
    * [Issue #1973](https://github.com/openziti/ziti/issues/1973) - Raft should not initialize if db is misconfigured
    * [Issue #1971](https://github.com/openziti/ziti/issues/1971) - BUG: OIDC authentication does not convert config type names to ids
    * [Issue #1966](https://github.com/openziti/ziti/issues/1966) - Handle multi-entity updates in router data model
    * [Issue #1772](https://github.com/openziti/ziti/issues/1772) - provide a better error when the user is not logged in
    * [Issue #1964](https://github.com/openziti/ziti/issues/1964) - Add API Session Token Update Messaging
    * [Issue #1960](https://github.com/openziti/ziti/issues/1960) - JWT Session exchange isn't working
    * [Issue #1962](https://github.com/openziti/ziti/issues/1962) - permissions enum doesn't contain "Invalid"

# Release 1.1.0

## What's New

* HA Alpha2
* Deployments Alpha
    * Linux packages provide systemd services for controller and router. Both depend on existing package `openziti` which provides the `ziti` command line tool.
        * `openziti-controller` provides `ziti-controller.service`
        * `openziti-router` provides `ziti-router.service`
    * Container images for controller and router now share the bootstrapping logic with the packages, so they
      support the same configuration options.

## HA Alpha2

This release can be run in HA mode. The code is still alpha, so there are still some bugs and missing features,
however basic functionality work with the exceptions noted. See the [HA Documementation](https://github.com/openziti/ziti/tree/release-next/doc/ha)
for instructions on setting up an HA cluster.

### Known Issues

* JWT Session exchange isn't working with Go SDK clients
    * This means Go clients will need to be restarted once their sessions expire
* Service/service policy changes might not be reflected in routers
    * Changes to policy may not yet properly sync to the routers, causing unexpected behavior with ER/Ts running in HA mode

More information can be found on the [HA Project Board](https://github.com/orgs/openziti/projects/9/views/1)

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.16 -> v0.26.17](https://github.com/openziti/edge-api/compare/v0.26.16...v0.26.17)
    * [Issue #107](https://github.com/openziti/edge-api/issues/107) - Add configTypes param to service list

* github.com/openziti/sdk-golang: [v0.23.19 -> v0.23.27](https://github.com/openziti/sdk-golang/compare/v0.23.19...v0.23.27)
    * [Issue #545](https://github.com/openziti/sdk-golang/issues/545) - Set config types on query when listing services
    * [Issue #541](https://github.com/openziti/sdk-golang/issues/541) - Token exchange in Go SDK not working
    * [Issue #540](https://github.com/openziti/sdk-golang/issues/540) - Switch to EdgeRouter.SupportedProtocols from deprecated URLs map

* github.com/openziti/ziti: [v1.0.0 -> v1.1.0](https://github.com/openziti/ziti/compare/v1.0.0...v1.1.0)
    * [Issue #1952](https://github.com/openziti/ziti/issues/1952) - Remove support for fabric only identities in CLI
    * [Issue #1950](https://github.com/openziti/ziti/issues/1950) - Add policy type to service policy router events
    * [Issue #1951](https://github.com/openziti/ziti/issues/1951) - Add more attributes to route data model Identity
    * [Issue #1942](https://github.com/openziti/ziti/issues/1942) - Rework ER/T intercept code to be sessionless or use JWT sessions
    * [Issue #1936](https://github.com/openziti/ziti/issues/1936) - SDK Hosted HA sessions are getting removed when they shouldn't be
    * [Issue #1934](https://github.com/openziti/ziti/issues/1934) - Don't publish binary builds to artifactory
    * [Issue #1931](https://github.com/openziti/ziti/issues/1931) - "invalid kid: <kid>" randomly occurs in HA mode

# Release 1.0.0

## About 1.0

What does marking OpenZiti as 1.0 mean?

### Backwards Compatibility
We've guaranteed API stability for SDK clients for years and worked hard to ensure that routers 
and controllers would be backwards and forward compatible. However, we have had a variety of 
management API changes and CLI changes. For post 1.0 releases we expect to make additions to the 
APIs and CLI, but won't remove anything until it's been first marked as deprecated and then only
with a major version bump. 

### Stability and Scale
Recent releases have seen additional testing using chaos testing techniques. These tests involve
setting up relatively large scale environments, knocking out various components and then verifying
that the network is able to return to a stable state. These test are run for hours to try and 
eliminate race conditions and distributed state machine problems. 

OpenZiti is also being used as underlying infrastrcture for the zrok public service. Use of this 
network has grown quickly and proven that it's possible to build ziti native apps that can scale
up.

## Backward Incompatible Changes to pre-1.0 releases

Administrators no longer have access to dial/bind all services by default. See below for details.

## What's New

* Administrators no longer have access to dial/bind all services by default.
* TLS Handshakes can now be rate limited in the controller
* TLS Handshake timeouts can now be set on the controller when using ALPN
* Bugfixes

## DEFAULT Bind/Dial SERVICE PERMISSIONS FOR Admin IDENTITIES HAVE CHANGED

Admin identities were able to Dial and Bind all services regardless of the effective service policies
prior to this release. This could lead to a confusing situation where a tunneler that was assuming an Admin
identity would put itself into an infinite connect-loop when a service's host.v1 address overlapped with
any addresses in its intercept configuration.

Please create service policies to grant Bind or Dial permissions to Admin identities as needed.

## TLS Handshake

A TLS handhshake rate limiter can be enabled. This is useful in cases where there's a flood of TLS requests and the
controller can't handle them all. It can get into a state where it can't respond to TLS handshakes quickly enough,
so the clients time out. They then retry, adding to the the load. The controller ends up wasting time doing work 
that isn't use. 

This uses the same rate limiting as the auth rate limiter. 

Additionally the server side handshake timeout can now be configured.

Configuration:

```
tls: 
  handshakeTimeout: 15s

  rateLimiter:
    # if disabled, no tls handshake rate limiting with be enforced
    enabled: true
    # the smallest window size for tls handshakes
    minSize: 5
    # the largest allowed window size for tls handshakes
    maxSize: 5000
    # after how long to consider a handshake abandoned if neither success nor failure was reported
    timeout: 30s
```

New metrics:

* `tls_handshake_limiter.in_process` - number of TLS handshakes in progress
* `tls_handshake_limiter.window_size` - number of TLS handhshakes allowed concurrently
* `tls_handshake_limiter.work_timer` - timer tracking how long TLS handshakes are taking


## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.122 -> v2.0.128](https://github.com/openziti/channel/compare/v2.0.122...v2.0.128)
* github.com/openziti/edge-api: [v0.26.14 -> v0.26.16](https://github.com/openziti/edge-api/compare/v0.26.14...v0.26.16)
* github.com/openziti/foundation/v2: [v2.0.40 -> v2.0.42](https://github.com/openziti/foundation/compare/v2.0.40...v2.0.42)
* github.com/openziti/identity: [v1.0.73 -> v1.0.75](https://github.com/openziti/identity/compare/v1.0.73...v1.0.75)
* github.com/openziti/metrics: [v1.2.48 -> v1.2.51](https://github.com/openziti/metrics/compare/v1.2.48...v1.2.51)
* github.com/openziti/runzmd: [v1.0.41 -> v1.0.43](https://github.com/openziti/runzmd/compare/v1.0.41...v1.0.43)
* github.com/openziti/sdk-golang: [v0.23.15 -> v0.23.19](https://github.com/openziti/sdk-golang/compare/v0.23.15...v0.23.19)
* github.com/openziti/secretstream: [v0.1.18 -> v0.1.19](https://github.com/openziti/secretstream/compare/v0.1.18...v0.1.19)
* github.com/openziti/storage: [v0.2.33 -> v0.2.36](https://github.com/openziti/storage/compare/v0.2.33...v0.2.36)
* github.com/openziti/transport/v2: [v2.0.125 -> v2.0.131](https://github.com/openziti/transport/compare/v2.0.125...v2.0.131)
    * [Issue #79](https://github.com/openziti/transport/issues/79) - Add adaptive rate limiting to shared tls listener

* github.com/openziti/ziti: [v0.34.2 -> v1.0.0](https://github.com/openziti/ziti/compare/v0.34.2...v1.0.0)
    * [Issue #1923](https://github.com/openziti/ziti/issues/1923) - Add release validation test suite
    * [Issue #1904](https://github.com/openziti/ziti/issues/1904) - Add TLS handshake rate limiter
    * [Issue #1921](https://github.com/openziti/ziti/issues/1921) - Tidy CLI
    * [Issue #1916](https://github.com/openziti/ziti/issues/1916) - SDK dials fails with 'token is malformed' error
    * [Issue #1911](https://github.com/openziti/ziti/issues/1911) - Fix panic on first HA controller startup
    * [Issue #1914](https://github.com/openziti/ziti/issues/1914) - Fix panic in PeerConnected
    * [Issue #1781](https://github.com/openziti/ziti/issues/1781) - Admin identities have bind and dial permissions to services
