# Release 0.17.5

## What's New

* Builds have been moved from travis.org to Github Actions
* IDs generated for entities in the Edge no longer use underscores and instead use periods to avoid
  issues when used as a common name in CSRs
* [edge#424](https://github.com/openziti/edge/issues/424) Authenticated, non-admin, clients can
  query service terminators
* [sdk-golang#112](https://github.com/openziti/sdk-golang/issues/112) Process checks for Windows are
  case-insensitive
* The CLI agent now runs over unix sockets and is enabled by default. See doc/ops-agent.md for
  details in the ziti repository.
* [ziti#245](https://github.com/openziti/ziti/issues/245) Make timeout used by CLI's internal REST
  client configurable via cmd line arg

  All `ziti edge controller` subcommands now support the `--timeout=n` flag which controls the
  internal REST-client timeout used when communicating with the Controller. The timeout resolution
  is in seconds. If this flag is not specified, the default is `5`. Prior to this release, the the
  REST-client timeout was always `2`. You now have the opportunity to increase the timeout if
  necessary (e.g. if large amounts of data are being queried).

  All `ziti edge controller` subcommands now support the `--verbose` flag which will cause internal
  REST-client to emit debugging information concerning HTTP headers, status, raw json response data,
  and more. You now have the opportunity to see much more information, which could be valuable
  during trouble-shooting.

# Release 0.17.4

## Breaking Changes

* Process posture checks now accept process posture responses with `signerFingerprints` instead of a
  single
  `singerFingerprint`. This renders older versions of the C-SDK (<=0.17.15) and Go-SDK (<=0.14.8)
  unable to properly respond to process posture checks. Prior to this binaries with digital
  signatures not in leaf-leading order would fail process posture checks.
* OS posture checks implementations in Ziti SDKs and Ziti Apps must now pass x.y.z semver compliant
  versions in the `version` field of posture responses. Failure to do so results failing posture
  checks. For operating systems that do not have all three x.y.z values, zeros should be used to
  supply the missing values.

## What's New

* [edge/#392](https://github.com/openziti/edge/issues/392) Pass AppData message headers
* [edge/#394](https://github.com/openziti/edge/issues/394) Posture Checks evaluate to false for
  existing sessions that lose all checks
* [edge/#396](https://github.com/openziti/edge/issues/396) Process checks can incorrectly pass
* [edge/#403](https://github.com/openziti/edge/issues/403) Support multiple executable signers
  signatures per process
* [edge/#401](https://github.com/openziti/edge/issues/401) Improve OS checks to support X.Y.Z semver
  comparisons
* [edge/#406](https://github.com/openziti/edge/issues/406) Adds `WindowsServer` in addition
  to `Windows` for server vs desktop os checks

### Improve OS checks to support X.Y.Z semver comparisons

OS Posture checks now support matching on x.y.z semver compliant formatting as well a flexible
syntax to specify `Ranges` of versions. A `Range` is a set of conditions which specify which
versions satisfy the range. A singular version can be matched on by simply supplying the version
without any operators (e.g. `1.0.0` ).

A condition is composed of an operator and a version. The supported operators are:

- `<1.0.0` Less than `1.0.0`
- `<=1.0.0` Less than or equal to `1.0.0`
- `>1.0.0` Greater than `1.0.0`
- `>=1.0.0` Greater than or equal to `1.0.0`
- `1.0.0`, `=1.0.0`, `==1.0.0` Equal to `1.0.0`
- `!1.0.0`, `!=1.0.0` Not equal to `1.0.0`. Excludes version `1.0.0`.

Note that spaces between the operator and the version will be gracefully tolerated.

A `Range` can link multiple `Ranges` separated by space:

Ranges can be linked by logical AND:

- `>1.0.0 <2.0.0` would match between both ranges, so `1.1.1` and `1.8.7` but not `1.0.0` or `2.0.0`
- `>1.0.0 <3.0.0 !2.0.3` would match every version between `1.0.0` and `3.0.0` except `2.0.3`

Ranges can also be linked by logical OR:

- `<2.0.0 || >=3.0.0` would match `1.x.x` and `3.x.x` but not `2.x.x`

AND has a higher precedence than OR. It's not possible to use brackets.

Ranges can be combined by both AND and OR

- `>1.0.0 <2.0.0 || >3.0.0 !4.2.1` would match `1.2.3`, `1.9.9`, `3.1.1`, but not `4.2.1`, `2.1.1`

The Ziti Edge API still accepts arrays of versions, as such the arrays are OR'ed between. In
addition, the Ziti CLI supports providing version declarations:

```
ziti edge create posture-check os $postureCheckOsName -o "windows:>=10.0.19041" -a "pc1"
```

# Release 0.17.3

## Breaking Changes

* None

## What's New

* [edge] Addressed an issue where session type was not taken into account for posture checks

# Release 0.17.2

## Breaking Changes

* Configuring posture checks with current and previous Ziti App endpoint software will cause them to
  not be able to connect to services. Updated Ziti Apps will be released in future versions.
* The 'golang' SDK application included and updated Ziti SDK that supports posture checks

## What's New

* Posture Check CLI Commands
* Posture Check Enforcement
* [edge/#382](https://github.com/openziti/edge/issues/382) Add configuration types that support
  VoIP, SCTP use cases

## Posture Check CLI Commands

The following commands have been added to the `ziti` CLI:

* `ziti edge list posture-checks <flags>`
* `ziti edge delete posture-check <idOrName> <flags>`
* `ziti edge update posture-check <type> <flags>`
* `ziti edge create posture-check <type> <reqValues> <flags>`

See the `-h` usage for more information on each command.

## Posture Check Enforcement

This release includes the logic necessary to accept posture responses and notify clients of posture
queries necessary to connect to services. Posture data can be submitted via `POST /posture-response`
and can be viewed via `GET /identities/<id>/posture-data`.

As noted above, configuring posture checks will cause all current Ziti App (any endpoint7 software
using a Ziti SDK) to fail the checks as they currently do not submit posture response data. The
ability for Ziti Apps to supply this information will be included in a subsequent release.

### New configuration types for tunneled services

The `intercept.v1` configuration type can be used when defining services that:

* intercept CIDRs and/or multiple addresses
* intercept multiple ports and/or port ranges
* use identity dial
* require source IP spoofing at the hosting tunneler

The `host.v1` configuration type enables configuration of hosted services with bind-by-identity and
protocol/address/port pass-through of services that use `intercept.v1` to intercept multiple
addresses.

# Release 0.17.0

## Breaking CLI Change

* The `ziti edge enroll` subcommand now supports the `--keyAlg=RSA|EC` flag which controls the
  algorithm used to generate the private key of the identity. If this flag is not specified, the
  default is `RSA`. Prior to this release, the the algorithm used to generate the private key of the
  identity was forced to `EC`. You now have a choice (although RSA usage should be used if you
  desire utilization of the future Ziti Browser support).

## What's New

* [TCP half-close](#tcp-half-close-support-in-ziti-tunnel) [edge#368 Implement half-close support](https://github.com/openziti/edge/issues/368)

### TCP half close support in ziti tunnel

This release implements a more graceful termination of TCP connections proxied over Ziti network.
One side of TCP connection can sent TCP FIN to its peer while continuing to receive data from
connection. This avoids loss of data that could still be in flight in the network.
