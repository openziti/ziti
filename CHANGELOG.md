# Release 0.27.0

## What's New

* Ziti CLI
    * The CLI has been cleaned up and unused, unusable and underused components have been removed or hidden
    * Add create/delete transit-router CLI commands
    * [Issue-706](https://github.com/openziti/ziti/issues/706) - Add port check to quickstart

## Ziti CLI

* The update command has been removed. It was non-functional, so this should not affect anyone 
* The adhoc, ping and playbook commands have been removed. These were ansible and vagrant commands that were not widely used.
* Make the art command hidden, doesn't need to be removed, leave it as an easter egg
* Move ziti ps command under ziti agent. Remove all ziti ps subcommands, as they already exist as ziti agent subcommands
* Add `ziti controller` and `ziti router` commands
    * They should work exactly the same as `ziti-controller` and `ziti router` 
    * The standalone binaries for `ziti-controller` and `ziti-router` are deprecated and will be removed in a future release
* Add hidden `ziti tunnel` command
    * Should work exactly the same as `ziti-tunnel`
    * Is hidden as `ziti-edge-tunnel` is the preferred tunnelling application
    * The standalone binary `ziti-tunnel` is deprecated and will be removed in a future release
* The db, log-format and unwrap commands have been moved under a new ops command
* ziti executable download management has been deprecated
    * The init and uninstall commands have been removed
    * The install, upgrade, use and version commands have been hidden and will be hidden once tests using them are updated or replaced
* The demo and tutorial commands have been moved under the new learn subcommand
* `ziti edge enroll` now has a verbose option for additional debugging
* The `ziti edge` CLI now support create/delete transit-router. This allows transit/fabric routers to be provisioned using an enrollment process, rather than requiring certs to be created externally. Note that this requires that the fabric router config file has a `csr` section.

# Release 0.26.11

## What's New

This is mainly a bugfix release.

- Ziti CLI
  - Bug Fixes (See Component Updates and Bug Fixes below)
  - Added CLI flags for setting router tunneler capability

## Ziti CLI

### Added CLI flags for setting router tunneler capability
Ziti CLI `ziti create config router edge` now has two new flags; `--tunnelerMode` and `--lanInterface`
#### --tunnelerMode
The `--tunnelerMode` flag enables tunneling and sets the tunneler mode. Currently, there are `none`, `host` and `tproxy`
modes. The default tunneler mode is `host` mode, choosing `none` will disable tunnel capabilities for the router.

Examples:
```shell
ziti create config router edge --routerName myRouter --tunnelerMode tproxy

ziti create config router edge --routerName myRouter --tunnelerMode none
```
#### --lanInterface
If using the `tproxy` tunneler mode, there is an optional `lanIf` section in the config to identify an interface to use.

Example:
```shell
ziti create config router edge --routerName myRouter --tunnelerMode tproxy --lanInterface tun0
```

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.4 -> v1.0.5](https://github.com/openziti/agent/compare/v1.0.4...v1.0.5)
* github.com/openziti/channel/v2: [v2.0.9 -> v2.0.12](https://github.com/openziti/channel/compare/v2.0.9...v2.0.12)
* github.com/openziti/edge: [v0.24.12 -> v0.24.36](https://github.com/openziti/edge/compare/v0.24.12...v0.24.36)
    * [Issue #1217](https://github.com/openziti/edge/issues/1217) - Ziti Edge lists the edge router to be offline after recovering from an internet fluctuation

* github.com/openziti/fabric: [v0.21.9 -> v0.21.17](https://github.com/openziti/fabric/compare/v0.21.9...v0.21.17)
* github.com/openziti/foundation/v2: [v2.0.6 -> v2.0.7](https://github.com/openziti/foundation/compare/v2.0.6...v2.0.7)
* github.com/openziti/identity: [v1.0.18 -> v1.0.20](https://github.com/openziti/identity/compare/v1.0.18...v1.0.20)
* github.com/openziti/runzmd: v1.0.3 (new)
* github.com/openziti/sdk-golang: [v0.16.135 -> v0.16.146](https://github.com/openziti/sdk-golang/compare/v0.16.135...v0.16.146)
    * [Issue #328](https://github.com/openziti/sdk-golang/issues/328) - enrollment has no 'verbose' option for debugging
    * [Issue #314](https://github.com/openziti/sdk-golang/issues/314) - Incorrect documentation for grpc-example
    * [Issue #317](https://github.com/openziti/sdk-golang/issues/317) - No documenation for call example
    * [Issue #311](https://github.com/openziti/sdk-golang/issues/311) - Chat Client and Server needs documentation

* github.com/openziti/storage: [v0.1.25 -> v0.1.26](https://github.com/openziti/storage/compare/v0.1.25...v0.1.26)
* github.com/openziti/transport/v2: [v2.0.36 -> v2.0.38](https://github.com/openziti/transport/compare/v2.0.36...v2.0.38)
* github.com/openziti/metrics: [v1.1.4 -> v1.1.5](https://github.com/openziti/metrics/compare/v1.1.4...v1.1.5)
* github.com/openziti/ziti: [v0.26.10 -> v0.26.11](https://github.com/openziti/ziti/compare/v0.26.10...v0.26.11)
    * [Issue 868](https://github.com/openziti/ziti/issues/868): `ZITI_EDGE_ROUTER_IP_OVERRIDE` does not override the edge router advertise hostname
    * [Issue 882](https://github.com/openziti/ziti/issues/882): `ZITI_EDGE_ROUTER_RAWNAME` not stored in quickstart .env file

# Release 0.26.10

## What's New
This release has a single fix for a panic in edge routers with embedded tunnelers hosting services.
The only other changes are build updates.

## Ziti Component Updates and Bug Fixes
* github.com/openziti/agent: [v1.0.3 -> v1.0.4](https://github.com/openziti/agent/compare/v1.0.3...v1.0.4)
* github.com/openziti/channel/v2: [v2.0.5 -> v2.0.9](https://github.com/openziti/channel/compare/v2.0.5...v2.0.9)
* github.com/openziti/edge: [v0.24.7 -> v0.24.12](https://github.com/openziti/edge/compare/v0.24.7...v0.24.12)
    * [Issue #1209](https://github.com/openziti/edge/issues/1209) - edge router with embedded tunneler panics when intercepting services 

* github.com/openziti/fabric: [v0.21.3 -> v0.21.9](https://github.com/openziti/fabric/compare/v0.21.3...v0.21.9)
* github.com/openziti/foundation/v2: [v2.0.5 -> v2.0.6](https://github.com/openziti/foundation/compare/v2.0.5...v2.0.6)
* github.com/openziti/identity: [v1.0.16 -> v1.0.18](https://github.com/openziti/identity/compare/v1.0.16...v1.0.18)
* github.com/openziti/sdk-golang: [v0.16.129 -> v0.16.135](https://github.com/openziti/sdk-golang/compare/v0.16.129...v0.16.135)
* github.com/openziti/storage: [v0.1.23 -> v0.1.25](https://github.com/openziti/storage/compare/v0.1.23...v0.1.25)
* github.com/openziti/transport/v2: [v2.0.33 -> v2.0.36](https://github.com/openziti/transport/compare/v2.0.33...v2.0.36)
* github.com/openziti/metrics: [v1.1.2 -> v1.1.4](https://github.com/openziti/metrics/compare/v1.1.2...v1.1.4)
* github.com/openziti/ziti: [v0.26.9 -> v0.26.10](https://github.com/openziti/ziti/compare/v0.26.9...v0.26.10)

# Release 0.26.9

## What's New

- Edge
  - Bug Fixes
- Fabric
  - Bug Fixes
- Ziti CLI
  - Allow dynamic modification of enrollment durations
  - Bug Fixes
- SDK Golang
  - Bug Fixes
- Identity

## Ziti CLI
### Allow dynamic modification of enrollment durations
#### Identity Enrollment Duration
Setting the environment variable `ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION` to some value **in minutes** will override the default identity enrollment duration configuration
when creating new controller configurations. If left unset, the default value is used. Using this method applies to controller config generation through the CLI as
well as quickstart deployments.

Example:
```shell
# Set identity enrollment to 60 minutes, controller configs created afterward will use this value
export ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION=60
```

An additional argument `--identityEnrollmentDuration` has been added to the CLI controller config generation. If the argument is provided, the value of the argument will take
precedence, followed by the value of the environment variable (noted above), and if neither are used, the default value is used. Note that the argument takes a time unit
(m for minutes, h for hour, etc.)

Example:
```shell
# Create a controller config with an identity enrollment duration of 60 minutes
ziti create config controller --identityEnrollmentDuration 60m
# OR
ziti create config controller --identityEnrollmentDuration 1h
```
#### Router Enrollment Duration
Setting the environment variable `ZITI_EDGE_ROUTER_ENROLLMENT_DURATION` to some value **in minutes** will override the default router enrollment duration configuration
when creating new controller configurations. If left unset, the default value is used. Using this method applies to controller config generation through the CLI as
well as quickstart deployments.

Example:
```shell
# Set router enrollment to 60 minutes, controller configs created afterward will use this value
export ZITI_EDGE_ROUTER_ENROLLMENT_DURATION=60
```

An additional argument `--routerEnrollmentDuration` has been added to the CLI controller config generation. If the argument is provided, the value of the argument will take
precedence, followed by the value of the environment variable (noted above), and if neither are used, the default value is used. Note that the argument takes a time unit
(m for minutes, h for hour, etc.)

Example:
```shell
# Create a controller config with a router enrollment duration of 60 minutes
ziti create config controller --routerEnrollmentDuration 60m
# OR
ziti create config controller --routerEnrollmentDuration 1h
```

### Ziti Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v1.0.3 -> v2.0.4](https://github.com/openziti/channel/compare/v1.0.3...v2.0.4)
* github.com/openziti/edge: [v0.23.0 -> v0.24.3](https://github.com/openziti/edge/compare/v0.23.0...v0.24.3)
    * [Issue #1189](https://github.com/openziti/edge/issues/1189) - router embedded tunneler can create redundant api session if initial requests come in a flood
    * [Issue #1186](https://github.com/openziti/edge/issues/1186) - Panic when creating sdk hosted terminator

* github.com/openziti/fabric: [v0.20.0 -> v0.21.2](https://github.com/openziti/fabric/compare/v0.20.0...v0.21.2)
    * [Issue #469](https://github.com/openziti/fabric/issues/469) - Initial support for multiple control channels in routers

* github.com/openziti/foundation/v2: [v2.0.4 -> v2.0.5](https://github.com/openziti/foundation/compare/v2.0.4...v2.0.5)
* github.com/openziti/identity: [v1.0.12 -> v1.0.16](https://github.com/openziti/identity/compare/v1.0.12...v1.0.16)
* github.com/openziti/sdk-golang: [v0.16.121 -> v0.16.128](https://github.com/openziti/sdk-golang/compare/v0.16.121...v0.16.128)
* github.com/openziti/storage: [v0.1.21 -> v0.1.23](https://github.com/openziti/storage/compare/v0.1.21...v0.1.23)
    * [Issue #23](https://github.com/openziti/storage/issues/23) - fix panic: IterateLink on ref counted link collection should never return a nil cursor 

* github.com/openziti/transport/v2: [v2.0.29 -> v2.0.33](https://github.com/openziti/transport/compare/v2.0.29...v2.0.33)
* github.com/openziti/jwks: [v1.0.1 -> v1.0.2](https://github.com/openziti/jwks/compare/v1.0.1...v1.0.2)
* github.com/openziti/metrics: [v1.1.0 -> v1.1.2](https://github.com/openziti/metrics/compare/v1.1.0...v1.1.2)
* github.com/openziti/x509-claims: [v1.0.2 -> v1.0.3](https://github.com/openziti/x509-claims/compare/v1.0.2...v1.0.3)
* github.com/openziti/ziti: [0.26.8 -> 0.26.9](https://github.com/openziti/ziti/compare/0.26.8...0.26.9)
    * [Issue #845](https://github.com/openziti/ziti/issues/845) - Setting ZITI_EDGE_ROUTER_IP_OVERRIDE now adds the IP to the CSR SANs of the router config

# Release 0.26.8

## What's New

- General
  - Allow filtering model entities by tag
- Fabric
  - Usage v3 metrics
- Edge
  - Bug Fixes
- Ziti CLI
  - `ziti edge create|update ca` now supports `externalIdClaim`
  - Improved List CAs
- Identity
  - Automatic File Reloads

## General
Model entities can now be filtered by tags. This works via the fabric and edge REST APIs and can be 
used from the `ziti` CLI.

Example:

```
$ ziti edge update service demo --tags location=PA 
$ ziti edge update service echo --tags location=NY 
$ ziti edge ls services 'limit 4'
╭────────────────────────┬──────────────┬────────────┬─────────────────────┬────────────╮
│ ID                     │ NAME         │ ENCRYPTION │ TERMINATOR STRATEGY │ ATTRIBUTES │
│                        │              │  REQUIRED  │                     │            │
├────────────────────────┼──────────────┼────────────┼─────────────────────┼────────────┤
│ 1WztJ.YuMY             │ demo         │ true       │ smartrouting        │            │
│ 68kYZOS54kAbU4hEhKHgHT │ echo         │ true       │ smartrouting        │ echo       │
│ EjaiJkYuMY             │ project.mgmt │ true       │ smartrouting        │            │
│ F0JVJkY40Y             │ mattermost   │ true       │ smartrouting        │            │
╰────────────────────────┴──────────────┴────────────┴─────────────────────┴────────────╯
results: 1-4 of 13

$ ziti edge ls services 'tags.location != null'
╭────────────────────────┬──────┬────────────┬─────────────────────┬────────────╮
│ ID                     │ NAME │ ENCRYPTION │ TERMINATOR STRATEGY │ ATTRIBUTES │
│                        │      │  REQUIRED  │                     │            │
├────────────────────────┼──────┼────────────┼─────────────────────┼────────────┤
│ 1WztJ.YuMY             │ demo │ true       │ smartrouting        │            │
│ 68kYZOS54kAbU4hEhKHgHT │ echo │ true       │ smartrouting        │ echo       │
╰────────────────────────┴──────┴────────────┴─────────────────────┴────────────╯
results: 1-2 of 2

$ ziti edge ls services 'tags.location = "NY"'
╭────────────────────────┬──────┬────────────┬─────────────────────┬────────────╮
│ ID                     │ NAME │ ENCRYPTION │ TERMINATOR STRATEGY │ ATTRIBUTES │
│                        │      │  REQUIRED  │                     │            │
├────────────────────────┼──────┼────────────┼─────────────────────┼────────────┤
│ 68kYZOS54kAbU4hEhKHgHT │ echo │ true       │ smartrouting        │ echo       │
╰────────────────────────┴──────┴────────────┴─────────────────────┴────────────╯
results: 1-1 of 1
```

## Fabric
### Usage v3

This a new version of usage events available. The existing v2 version events can still be used. The version
is selected in the events configuration.

Here is a config showing how to get both sets of events:

```
events:
  jsonLogger:
    subscriptions:
      - type: fabric.usage
        version: 2
      - type: fabric.usage
        versin: 3
```
If no version is provided for usage, then v2 events will still be outputted by default.

### Event Consolidation

V3 events consolidate multiple usage metrics together to minimize the number of events.

Example:

```
{
  "namespace": "fabric.usage",
  "version": 3,
  "source_id": "cjc.1kYu0",
  "circuit_id": "CwbENl.lW",
  "usage": {
    "egress.rx": 47,
    "egress.tx": 47
  },
  "interval_start_utc": 1663342500,
  "interval_length": 60,
  "tags": {
    "clientId": "XtYOStBYgd",
    "hostId": "f3ltEI8Iok",
    "serviceId": "fclVFecdgakAoHyBvtIGy"
  }
}
```

Ingress and egress usage for a given circuit will consolidated into a single event per router. Fabric usage 
will also be consolided into a single, separate event.

### Event tagging

Usage events for ingress and egress usage will be annotated with edge information for both v2 and v3.

In the example above the event has tags for `clientId`, `hostId` and `serviceId`.

* `clientId` - The id of the edge identity using the service
* `hostId` - The id of the edge identity hosting the service (will be blank if not applicable, such as for router hosted)
* `serviceId` - The id of the service being used

## Edge
### Bug Fixes

* [Issue 1176](https://github.com/openziti/edge/issues/1176): Patching CA `externalIdClaim` Does Not Work

## Ziti CLI

### `ziti edge create|update ca` now support `externalIdClaim

Identities now have a field named `externalId` that can be used with 3rd Party CAs in addition to the existing
External JWT Signer support. 3rd Party CAs now support the following optional fields:

- `externalIdClaim.index` - if multiple externalId claims are located, the index will be used to select one, default 0
- `externalIdClaim.location` - extracts values from one of the following locations on a x509 certificate: `SAN_URI`, `SAN_EMAIL`, `COMMON_NAME`
- `externalIdClaim.matcher` - matches values in one of the following ways  `PREFIX`, `SUFFIX`, `SCHEME` in conjunction with `matcherCriteria` or select all values via `ALL`
- `externalIdClaim.matcherCriteria` - `matcher` values of `PREFIX`, `SUFFIX`, and `SCHEME` will use `matcherCriteria` as a matching value
- `externalIdClaim.parser`: - supports parsing values from all matched externalIds via `SPLIT` or `NONE`
- `externalIdClaim.parserCriteria` - for a `parser` value of `SPLIT`, `parserCriteria` will be used to split values

When defined the `externalIdClaim` configuration will be used to locate any `externalId`s present in the client
supplied x509 certificate. If an `externalId` is located, it will be used to associate the authentication request
with an identity. If found, authentication is considered successful if not the authentication request fails. If the 
client certificate does not contain an `externalId` then identities will be searched for that have a certificate
authenticator that matches the supplied client certificate. Should that fail, the authentication request fails.

This functionality can be used to support SPIFFE provisioned identities. For any specific SPIFFE ID, assign it to an
identity's `externalId` and then use the following `externalIdClaim` configurations.

#### CA Create/Update REST API
```json
{
  ...
  "externalIdClaim": {
    "location": "SAN_URI",
    "index": 0,
    "matcher": "SCHEME",
    "matcherCriteria": "spiffe",
    "parser": "NONE",
    "parserCriteria": ""
  }
}
```
#### Ziti CLI

```
ziti edge create ca myCa ca.pem -l SAN_URI -m SCHEME -x spiffe -p "NONE"
```

```
ziti edge update ca myCa -l SAN_URI -m SCHEME -x spiffe -p "NONE"
```

### Improved List CAs Output

The output for listing CAs in non-JSON format has been improved.

Example:

```text
╭────────────────────────┬─────────┬────────┬────────────┬─────────────┬─────────────────────────────────────────────────────────────────╮
│ ID                     │ NAME    │ FLAGS  │ TOKEN      │ FINGERPRINT │ CONFIGURATION                                                   │
├────────────────────────┼─────────┼────────┼────────────┼─────────────┼─────────────────┬──────────────────────┬────────────────────────┤
│ 1tu6CbXT18Dd9rybjCW5eX │ 2       │ [AOE]  │ KaPxRiKbk  │ -           │ AutoCA          │ Identity Name Format │ [caName]-[commonName]  │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Identity Roles       │ a,b,c                  │
│                        │         │        │            │             ├─────────────────┼──────────────────────┼────────────────────────┤
│                        │         │        │            │             │ ExternalIdClaim │ Index                │ 2                      │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Location             │ SAN_URI                │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Matcher              │ ALL                    │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Matcher Criteria     │                        │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Parser               │ NONE                   │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Parser Criteria      │                        │
├────────────────────────┼─────────┼────────┼────────────┼─────────────┼─────────────────┼──────────────────────┼────────────────────────┤
│ 7AGp9vUttJHKA1JWujNtpR │ test-ca │ [VAOE] │ -          │ 315e...ba   │ AutoCA          │ Identity Name Format │ [caName]-[commonName]  │
│                        │         │        │            │             │                 ├──────────────────────┼────────────────────────┤
│                        │         │        │            │             │                 │ Identity Roles       │  three, two,one        │
╰────────────────────────┴─────────┴────────┴────────────┴─────────────┴─────────────────┴──────────────────────┴────────────────────────╯
```

## Ziti Library Updates

* github.com/openziti/channel: [v1.0.2 -> v1.0.3](https://github.com/openziti/channel/compare/v1.0.2...v1.0.3)
* github.com/openziti/edge: [v0.22.91 -> v0.23.0](https://github.com/openziti/edge/compare/v0.22.91...v0.23.0)
    * [Issue #1173](https://github.com/openziti/edge/issues/1173) - Add session_type and service_id to edge session events
    * [Issue #1176](https://github.com/openziti/edge/issues/1176) - Patching CA ExternalIdClaim Does Not Work
    * [Issue #1174](https://github.com/openziti/edge/issues/1174) - Fix patching tags on services and transit routers
    * [Issue #1154](https://github.com/openziti/edge/issues/1154) - Additional filters for service list endpoint

* github.com/openziti/fabric: [v0.19.67 -> v0.20.0](https://github.com/openziti/fabric/compare/v0.19.67...v0.20.0)
    * [Issue #496](https://github.com/openziti/fabric/issues/496) - Reduce utiliztion messages by combining directionality
    * [Issue #499](https://github.com/openziti/fabric/issues/499) - Fix tag patching tags on service and router

* github.com/openziti/identity: [v1.0.11 -> v1.0.12](https://github.com/openziti/identity/compare/v1.0.11...v1.0.12)
* github.com/openziti/metrics: [v1.0.7 -> v1.1.0](https://github.com/openziti/metrics/compare/v1.0.7...v1.1.0)
    * [Issue #15](https://github.com/openziti/metrics/issues/15) - Support tags and multiple values on usage

* github.com/openziti/sdk-golang: [v0.16.119 -> v0.16.121](https://github.com/openziti/sdk-golang/compare/v0.16.119...v0.16.121)
* github.com/openziti/storage: [v0.1.20 -> v0.1.21](https://github.com/openziti/storage/compare/v0.1.20...v0.1.21)
    * [Issue #21](https://github.com/openziti/storage/issues/21) - Support querying tags by default

* github.com/openziti/transport/v2: [v2.0.28 -> v2.0.29](https://github.com/openziti/transport/compare/v2.0.28...v2.0.29)
* github.com/openziti/ziti: [0.26.7 -> 0.26.8](https://github.com/openziti/ziti/compare/0.26.7...0.26.8)
    * [Issue #835](https://github.com/openziti/ziti/issues/835) - Ensure model entity tags can be updated via CLI where appropriate

# Release 0.26.7

## What's New

The only change in this release is updating from Golang 1.18 to 1.19

# Release 0.26.6

## What's New

- Edge
  - N/A
- Fabric
  - Don't allow slow or blocked links to impede other links
  - Add destination address to circuit events
- Ziti CLI
  - Bug Fixes
- SDK Golang
  - N/A
- Identity

## Fabric
### Address slow/blocked links

Previously if a router had multiple links and one of them was slow or blocked, it could prevent other traffic from moving. Now, if a link is unable to keep up with incoming traffic, payloads will be dropped. The end-to-end flow control and retransmission logic will handle re-sending the packet. 

Links have a 64 message queue for incoming messages. Up to 64 messages are taken off the queue, sorted in priority order and then sent. Once the sorted list of messages has been sent, the next set of messages are dequeue, sorted and sent. If the queue fills while the current set of sorted messges is being sent, message will now be dropped instead of waiting for queue space to open up.

There is now a new per-link `link.dropped_msgs` metric to track how often links are dropping messages.

### Destination Address added to Circuit Events

When available, the remote address of the terminating side of a circuit is now available in the circuit event.

Example:

```
{
  "namespace": "fabric.circuits",
  "version": 2,
  "event_type": "created",
  "circuit_id": "kh7myU.bX",
  "timestamp": "2022-09-12T19:08:20.461576428-04:00",
  "client_id": "cl7zdm0d0000fbygdlzh268uq",
  "service_id": "6SIomYCjH5Jio52szEtX7W",
  "terminator_id": "7IIb1nU5yTfJVbaD8Tjuf3",
  "instance_id": "",
  "creation_timespan": 949916,
  "path": {
    "nodes": [
      "B3V.1kN40Y"
    ],
    "links": null,
    "ingress_id": "26D7",
    "egress_id": "wjo7",
    "terminator_local_addr": "127.0.0.1:44822",
    "terminator_remote_addr": "127.0.0.1:1234"
  },
  "link_count": 0,
  "path_cost": 262140
}
```

## Ziti CLI
### Bug Fixes

* [Issue 823](https://github.com/openziti/ziti/issues/843): Fixed quickstart bug with architecture detection not supporting `aarch64`

## Identity

Identity is a low-level library within Ziti and affects all Ziti components.

### Bug Fixes

* Fixed an issue where `alt_server_certs` were not always loaded and used for presenting TLS configurations

## Ziti Library Updates

* github.com/openziti/agent: [v1.0.1 -> v1.0.3](https://github.com/openziti/agent/compare/v1.0.1...v1.0.3)
* github.com/openziti/channel: [v0.18.58 -> v1.0.2](https://github.com/openziti/channel/compare/v0.18.58...v1.0.2)
    * [Issue #68](https://github.com/openziti/channel/issues/68) - Allow send with no wait, if queue is full
    * [Issue #69](https://github.com/openziti/channel/issues/69) - Respect OutQueueSize option

* github.com/openziti/edge: [v0.22.54 -> v0.22.91](https://github.com/openziti/edge/compare/v0.22.54...v0.22.91)
    * [Issue #1167](https://github.com/openziti/edge/issues/1167) - Send remote addr of dialed connection for xgress_edge_tunnel and xgress_edge_transport
    * [Issue #1169](https://github.com/openziti/edge/issues/1169) - Update of service policy with patch fails if service policy type is not provided
    * [Issue #1163](https://github.com/openziti/edge/issues/1163) - Support flushing dns cache with resolvectl
    * [Issue #1164](https://github.com/openziti/edge/issues/1164) - Fix panic in xgress conn LogContext()

* github.com/openziti/fabric: [v0.19.34 -> v0.19.67](https://github.com/openziti/fabric/compare/v0.19.34...v0.19.67)
    * [Issue #484](https://github.com/openziti/fabric/issues/484) - Don't let slow/stalled links block other links
    * [Issue #459](https://github.com/openziti/fabric/issues/459) - Add destination IP to fabric.circuits created message
    * [Issue #492](https://github.com/openziti/fabric/issues/492) - Add HostId to terminator events
    * [Issue #485](https://github.com/openziti/fabric/issues/485) - Metrics events timestamp format changed 

* github.com/openziti/foundation/v2: [v2.0.2 -> v2.0.4](https://github.com/openziti/foundation/compare/v2.0.2...v2.0.4)
* github.com/openziti/identity: [v1.0.5 -> v1.0.11](https://github.com/openziti/identity/compare/v1.0.5...v1.0.11)
* github.com/openziti/metrics: [v1.0.3 -> v1.0.7](https://github.com/openziti/metrics/compare/v1.0.3...v1.0.7)
* github.com/openziti/sdk-golang: [v0.16.104 -> v0.16.119](https://github.com/openziti/sdk-golang/compare/v0.16.104...v0.16.119)
* github.com/openziti/storage: [v0.1.16 -> v0.1.20](https://github.com/openziti/storage/compare/v0.1.16...v0.1.20)
* github.com/openziti/transport/v2: [v2.0.20 -> v2.0.28](https://github.com/openziti/transport/compare/v2.0.20...v2.0.28)
* github.com/openziti/ziti: [0.26.5 -> 0.26.6](https://github.com/openziti/ziti/compare/0.26.5...0.26.6)

# Release 0.26.5

## What's New

This build has no functional changes, but does have changes to the build workflow,
because github is deprecating certain action runners. See
https://github.blog/changelog/2022-08-09-github-actions-the-ubuntu-18-04-actions-runner-image-is-being-deprecated-and-will-be-removed-by-12-1-22/
and
https://github.blog/changelog/2022-07-20-github-actions-the-macos-10-15-actions-runner-image-is-being-deprecated-and-will-be-removed-by-8-30-22/
for details 

* MacOS builds are now done on the macos-11 github builder
* Linux builds are now done on the ubuntu-20.04 builder

This changes the oldest supported operating system versions for ziti-controller and ziti-router to those 
listed above, due to dependencies on system shared libraries that may not be available on older operating 
system versions.

If this change negatively impacts you, please let us on [Discourse](https://openziti.discourse.group).

# Release 0.26.4

## What's New

- Edge
  - N/A
- Fabric
  - Bug Fixes
- Ziti CLI
  - `ziti fabric inspect` can now emit results to individual files using the `-f` flag 
- SDK Golang
  - N/A

## Fabric
### Bug Fixes

* [Issue 463](https://github.com/openziti/fabric/issues/463): fix for panic when dial service with instanceId and service has terminators but non for requested instanceId

# Release 0.26.3

## What's New

- Edge
  - N/A
- Fabric
  - Link Events
  - Circuit Event Path Changes
  - Allow attributing usage to hosting identities
  - Capture IP/Port of edge routers creating api sessions
  - Report high link latency when heartbeats time out
  - Bug Fixes
- Ziti CLI
  - N/A
- SDK Golang
  - N/A
- Transport
  - WS/WSS no longer require client certificate

## Fabric
### Link Events

Link events can now be configured in the controller events configuration.

```
events:
  jsonLogger:
    subscriptions:
      - type: fabric.links
    handler:
      type: file
      format: json
      path: /var/log/ziti-events.log
```

#### Link Event Types

* `dialed` : Generated when the controller sends a link dial message to a router
* `connected` : Generated when a router sends a link connected message to the controller
* `fault` : Generated when a router sends a link fault to the controller
* `routerLinkNew` : Generated when a router sends a router link message to the controler and the link is new to the controller
* `routerLinkKnown` : Generated when a router sends a router link message to the controller and the link is known
* `routerLinkDisconnectedDest` : Generated when a router sends a route link message to the controller and the router on the other side of the link is not currently connected.


#### Link Dialed Event Example
```
{
  "namespace": "fabric.links",
  "event_type": "dialed",
  "timestamp": "2022-07-15T18:10:19.752766075-04:00",
  "link_id": "47kGIApCXI29VQoCA1xXWI",
  "src_router_id": "niY.XmLArx",
  "dst_router_id": "YPpTEd8JP",
  "protocol": "tls",
  "dial_address": "tls:127.0.0.1:4024",
  "cost": 1
}
```

#### Link Connected Example
```
{
  "namespace": "fabric.links",
  "event_type": "connected",
  "timestamp": "2022-07-15T18:10:19.973626185-04:00",
  "link_id": "47kGIApCXI29VQoCA1xXWI",
  "src_router_id": "niY.XmLArx",
  "dst_router_id": "YPpTEd8JP",
  "protocol": "tls",
  "dial_address": "tls:127.0.0.1:4024",
  "cost": 1,
  "connections": [
    {
      "id": "ack",
      "local_addr": "tcp:127.0.0.1:49138",
      "remote_addr": "tcp:127.0.0.1:4024"
    },
    {
      "id": "payload",
      "local_addr": "tcp:127.0.0.1:49136",
      "remote_addr": "tcp:127.0.0.1:4024"
    }
  ]
}
```

#### Link Fault Example
````
{
  "namespace": "fabric.links",
  "event_type": "fault",
  "timestamp": "2022-07-15T18:10:19.973867809-04:00",
  "link_id": "6slUYCqOB85YTfdiD8I5pl",
  "src_router_id": "YPpTEd8JP",
  "dst_router_id": "niY.XmLArx",
  "protocol": "tls",
  "dial_address": "tls:127.0.0.1:4023",
  "cost": 1
}
```

#### Router Link Known Example
````
{
  "namespace": "fabric.links",
  "event_type": "routerLinkKnown",
  "timestamp": "2022-07-15T18:10:19.974177638-04:00",
  "link_id": "47kGIApCXI29VQoCA1xXWI",
  "src_router_id": "niY.XmLArx",
  "dst_router_id": "YPpTEd8JP",
  "protocol": "tls",
  "dial_address": "tls:127.0.0.1:4024",
  "cost": 1
}
```

### Circuit Event Path Changes

* Circuit event paths are now structured, rather than being a string
* The path structure contains a string list of routers in the path, ordered from initiator to terminator
* The path structure contains a string list of links in the path, ordered from initiator to terminator
* The path structure also contains the initiator and terminator xgress instance ids
* `terminator_local_addr` has been moved inside the nested path structure
* There is also a new version field, which is set to 2.

Old circuit event:
```
{
  "namespace": "fabric.circuits",
  "event_type": "created",
  "circuit_id": "Y4aVR-QfM",
  "timestamp": "2022-07-19T12:39:21.500700972-04:00",
  "client_id": "cl5sehx8k000d0agdrqyh9aa4",
  "service_id": "bnNbAbsiYM",
  "instance_id": "",
  "creation_timespan": 812887,
  "path": "[r/niY.XmLArx]",
  "terminator_local_address": "",
  "link_count": 0,
  "path_cost": 262140,
  "failure_cause": null
}
```

New circuit event:
```
{
  "namespace": "fabric.circuits",
  "version": 2,
  "event_type": "created",
  "circuit_id": "Llm58Bn-J",
  "timestamp": "2022-07-19T12:41:31.043070164-04:00",
  "client_id": "cl5sekp6z000dk0gdej54ipgx",
  "service_id": "bnNbAbsiYM",
  "terminator_id": "6CNJIXdRQ6mctdzHXEx8nW",
  "instance_id": "",
  "creation_timespan": 781618,
  "path": {
    "nodes": [
      "niY.XmLArx"
    ],
    "links": null,
    "ingress_id": "v9yv",
    "egress_id": "2mOq",
    "terminator_local_addr": ""
  },
  "link_count": 0,
  "path_cost": 262140
}
```

### Allow attributing usage to hosting endpoints
Terminator now has a Host ID, similar to the session Client ID. This can be used by higher levels to associate an id 
with the terminator. The edge sets this field to the hosting session id. 
Circuits now also track which terminator they are using, with a new terminatorId field. 
These two changes together allow usage to be attributed to hosting entities as well
as dialing entities.

### Capture IP/Port of edge routers creatign api sessions
When an edge router creates an API session, the ip:port of the edge router control channel will be captured.

### Report high link latency when heartbeats time out
Previously when latency probes/heatbeats timed out, we wouldn't update the link latency. 
Now, link latency will be set to 88888888888ns (or ~88seconds). This will help keep
these links from being used. The use of this marker value will also let timeouts be 
identitied.

### Bug Fixes

* [Circuits on single router which is deleted are ophaned](https://github.com/openziti/fabric/issues/452)
* [API Session Certs not updated on ERs](https://github.com/openziti/edge/issues/1096)

# Release 0.26.2

## What's New
- Transport
  - WS/WSS Identity Support 
- Identity
  - Alternate Server Certificate Support
- Edge
  - N/A
- Fabric
  - N/A
- Ziti CLI
  - Improvements to `ziti edge list posture-check` output
- SDK Golang
  - N/A

## Transport
### WS/WSS Identity Support

The binding `ws` and `wss` in the transport library now use identity for server certificates. Prior to this release
`ws` and `wss` would load the `server_cert` and `key` field from files only. Both now support an optional field named
`identity`. If not specified, the root `identity` field will be used. If specified it will be used for the specified
`ws` or `wss` binding. Since this field is processed by the [identity library](https://github.com/openziti/identity)
it supports all the private key and certificate sources that the identity framework supports (file, pem, hsm, etc.).
Additionally it also enables SNI support for `ws` and `wss` listeners.

```yaml
transport:
  ws:
    writeTimeout:      10
    readTimeout:       5
    idleTimeout:       5
    pongTimeout:       60
    pingInterval:      54
    handshakeTimeout:  10
    readBufferSize:    4096
    writeBufferSize:   4096
    enableCompression: false
    identity:
      server_cert:          ./certs/er1.server.cert.pem
      server_key:                  ./certs/key.pem
```

Example: Relying on in the root `server_cert` and `alt_server_cert` field
```yaml
v: 3

identity:
  cert:                 ./certs/er1.client.cert.pem
  server_cert:          ./certs/er1.server.cert.pem
  key:                  ./certs/er1.key.pem
  ca:                   ./certs/er1.ca-chain.cert.pem
  alt_server_certs:
    - server_cert: ./certs/er1.alt.server.cert.pem
      server_key:  ./certs/er1.alt.server.cert.pem
...

transport:
  ws:
    writeTimeout:      10
    readTimeout:       5
    idleTimeout:       5
    pongTimeout:       60
    pingInterval:      54
    handshakeTimeout:  10
    readBufferSize:    4096
    writeBufferSize:   4096
    enableCompression: false
```

## Identity
### Alternate Server Certificate Support

The [identity library](https://github.com/openziti/identity) has been updated to support a new field: `alt_server_certs`
. This field is an array of objects with `server_cert` and `server_key` fields. `alt_server_certs` is not touched by
higher level Ziti automations to renew certificates and is intended for manual or externally automated use. It allows
additional server certificates to be used for the controller and routers with separate private keys. It is useful in
scenarios where routers or controllers are exposed using certificates signed by public CAs (i.e. Let's Encrypt).

The `server_cert` and `server_key` work the same as the root identity properties of the same name. In any single
`server_cert` source that provides a chain, it assumed that all leaf-certificates are based on the private key in
`server_key`. If `server_key` is not defined, the default root `server_key` will be used. The identity library will use
the certificate chains and private key pairs specified in `alt_server_certs` when generating a TLS configuration via
`ServerTLSConfig()`. All identity sources are viable: `pem`, `file`, etc.

Go Identity Config Struct Definition:
```go
type Config struct {
	Key            string       `json:"key" yaml:"key" mapstructure:"key"`
	Cert           string       `json:"cert" yaml:"cert" mapstructure:"cert"`
	ServerCert     string       `json:"server_cert,omitempty" yaml:"server_cert,omitempty" mapstructure:"server_cert,omitempty"`
	ServerKey      string       `json:"server_key,omitempty" yaml:"server_key,omitempty" mapstructure:"server_key,omitempty"`
	AltServerCerts []ServerPair `json:"alt_server_certs,omitempty" yaml:"alt_server_certs,omitempty" mapstructure:"alt_server_certs,omitempty"`
	CA             string       `json:"ca,omitempty" yaml:"ca,omitempty" mapstructure:"ca"`
}
```

JSON Example:

```json
{
  "cert": "./ziti/etc/ca/intermediate/certs/ctrl-client.cert.pem",
  "key": "./ziti/etc/ca/intermediate/private/ctrl.key.pem",
  "server_cert": "./ziti/etc/ca/intermediate/certs/ctrl-server.cert.pem",
  "server_key": "./ziti/etc/ca/intermediate/certs/ctrl-server.key.pem",
  "ca": "./ziti/etc/ca/intermediate/certs/ca-chain.cert.pem",
  "alt_server_certs": [
    {
      "server_cert": "./ziti/etc/ca/intermediate/certs/alt01-ctrl-server.cert.pem",
      "server_key": "./ziti/etc/ca/intermediate/certs/alt01-ctrl-server.key.pem"
    },
    {
      "server_cert": "pem:-----BEGIN CERTIFICATE-----\nIIGBjCCA+6gAwIBAgICEAAwDQYJKoZIhvcNAQELBQAwgZcxCzAJBgNVBAYTAlVT...",
      "server_key": "pem:-----BEGIN CERTIFICATE-----\nMIIEuDCCAqCgAwIBAgICEAAwDQYJKoZIhvcNAQELBQAwgYsxCzAJBgNVBAYTAlVT..."
    }
  ]
}
```

YAML Example:

```yaml
cert: "./ziti/etc/ca/intermediate/certs/ctrl-client.cert.pem"
key: "./ziti/etc/ca/intermediate/private/ctrl.key.pem"
server_cert: "./ziti/etc/ca/intermediate/certs/ctrl-server.cert.pem"
server_key: "./ziti/etc/ca/intermediate/certs/ctrl-server.key.pem"
ca: "./ziti/etc/ca/intermediate/certs/ca-chain.cert.pem"
alt_server_certs:
 - server_cert: "./ziti/etc/ca/intermediate/certs/alt01-ctrl-server.cert.pem"
   server_key: "./ziti/etc/ca/intermediate/certs/alt01-ctrl-server.key.pem"
 - server_cert: "pem:-----BEGIN CERTIFICATE-----\nIIGBjCCA+6gAwIBAgICEAAwDQYJKoZIhvcNAQELBQAwgZcxCzAJBgNVBAYTAlVT..."
   server_key: "pem:-----BEGIN CERTIFICATE-----\nMIIEuDCCAqCgAwIBAgICEAAwDQYJKoZIhvcNAQELBQAwgYsxCzAJBgNVBAYTAlVT..."
```


# Release 0.26.1

There was a missed dependency update for xweb in 0.26.0 that kept SNI from working in HTTP API components. This would
affect SNI support for all REST APIs.

## What's New
- Edge
  - Fixes missing identity update in xweb
- Fabric
  - Fixes missing identity update in xweb
  - Bug Fixes
- Ziti CLI
  - N/A
- SDK Golang
  - N/A

## Edge
### Bug Fixes
* [Fix panic on remote resolve connections](https://github.com/openziti/edge/pull/1088)

## Fabric
### Bug Fixes
* [Logging erroneously indicates conflicting conditions returned from route attempt](https://github.com/openziti/fabric/issues/446)

# Release 0.26.0

## Breaking Changes

* The fabric management terminators API has changed the name of some fields. See below for details.
* The management channel, which was formerly deprecated is now removed
* Support for the old metrics subsystem has been removed. 

## What's New
- Edge
  - N/A
- Fabric
  - Terminator fields name changes
  - Circuit failed events
  - Additional circuit inspect information gathered
  - Management channel has been removed
  - Old metrics subsystem removed
  - Circuit createdAt
  - Bug Fixes
- Ziti CLI
  - Terminator fields name changes
  - Bug Fixes
- SDK Golang
  - N/A
- Identity
  - All OpenZiti implementations now support multiple certificate chains in the `server_cert` field to enable SNI scenarios

## Fabric
### Terminator fields name changes

The following fields have been renamed:

* `identity` -> `instanceId`
* `identitySecret` -> `instanceSecret`

The use of `identity` was confusing as identity is also used in the edge. While terminator instanceId
could be an edge identity id or something related to an edge identity, it could also be something
entirely unrelated. To reduce semantic overload, we've renamed it to instanceId, which hopefully is 
more descriptive. In general all terminators with the same instance id should end up at the same 
hosting process. 

### Circuit failed events

The fabric can now emit circuit events when a circuit creation failed.

Here is an example event:
```
{
  "namespace": "fabric.circuits",
  "event_type": "failed",
  "circuit_id": "DtZLURFgP",
  "timestamp": "2022-06-22T14:24:18.389718316-04:00",
  "client_id": "cl4pxcvyl000m5qgd1xwcfg1u",
  "service_id": "dH0lwdc5P",
  "instance_id": "",
  "creation_timespan": 739021,
  "path": "[r/niY.XmLArx]->[l/1UZCUTGhHuJygXld8CxXPs]->[r/YPpTEd8JP]",
  "terminator_local_address": "",
  "link_count": 1,
  "path_cost": 327152,
  "failure_cause": "ROUTER_ERR_CONN_REFUSED"
}
```

Note the `event_type` is failed. For events of this type only, the `failure_cause` will be populated. The current set of failure causes is:

* `INVALID_SERVICE`
* `ID_GENERATION_ERR`
* `NO_TERMINATORS`
* `NO_ONLINE_TERMINATORS`
* `NO_PATH`
* `PATH_MISSING_LINK`
* `INVALID_STRATEGY`
* `STRATEGY_ERR`
* `ROUTER_ERR_GENERIC`
* `ROUTER_ERR_INVALID_TERMINATOR`
* `ROUTER_ERR_MISCONFIGURED_TERMINATOR`
* `ROUTER_ERR_DIAL_TIMED_OUT`
* `ROUTER_ERR_CONN_REFUSED`

In addition to the `failure_cause` field, there is also a new `instance_id` field. This will be populated for all circuit event types and
will have the instance id requested by the dial. This is generally only applicable when using addressable terminators. If no instance id
was specified, the field will be blank.

### Circuit Inspect Enhancements

Circuit inspect will now gather more information.

* xgress details now includes the xgress sequence
* The receive buffer now has the following new fields
    * acquiredSafely
    * maxSequence
    * nextPayload
    * payloadCount
    * sequence

### Management channel removed

The management channel has been removed. The ziti-fabric cli, which used to use the management channel,
has been absorbed into the ziti CLI, and now used the fabric REST API and/or websockets where appropriate.

The `mgmt:` stanza in configuration files, which used to be required, will now be ignored.

### Old Metrics Subsystem removed

Formerly metrics could be exported to file via the `metrics:` configuration stanza. This was superceded by
the events subsystem, which contains metrics as well as other events. 

This also means that we no longer support pushing metrics directly to InfluxDB. However, we now have a 
Prometheus endpoint available, which can also be used to feed information to InfluxDB.

### Circuit createdAt

Circuits now have a createdAt field, visible via the REST API. 

### Bug Fixes

* Fix for issue where smart routing could break a circuit if a router became unavailable while circuits were being updated

## Ziti CLI
### Terminator Field Name Changes
The `ziti fabric create terminator` operation now takes a `--instance-id` flag instead of an `--identity` flag.

The `ziti fabric list terminators` operation now shows `InstanceId` instead of `Identity`. 

### Bug Fixes

* Fixed a bug where the controller advertised name was not properly set when the value of EXTERNAL_DNS was set.

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
the same timeout througout the dial process.

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
In previous releases when a router recieved a link dial from another router, it would verify
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
* Bug fix: When establishing links, a link could be closed while being registered, leading the controlller and router to get out of sync
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

A new primary authentication mechanism is available in addition to `cert` and `passsword` (UPDB). The internal
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

# Release 0.24.13

* Enhancement: Added new `noTraversal` field to routers. Configures if a router should allow/disallow traversal. Required on create/update commands.
* Enhancement: `ziti edge update edge-router` now supports either `--no-traversal` flag which will allow/disallow a given router from being used to traverse. 
* Enhancement: `ziti fabric list routers` and `ziti edge list routers` will now display the noTraversal flag of associated routers. 
* Feature: 1st Party Certificate Extension

## 1st Party Certificate Extension

Ziti Edge Client and Management API both support certificate extension for Ziti provisioned certificates. Before a 
client certificate expires, the client can elect to generate a new client certificate that will extend its valid period
and allows the client to optionally utilize a new private key.

Process Outline:

1) The client enrolls, obtaining a client certificate that is signed by the Ziti Controller
2) The client authenticates
3) The client provides a CSR
4) The client receives a new public certificate
5) The client verifies with the controller the new public certificate has been obtained

### Detailed Outline

The client enrolls and authenticates with the controller as normal. If the client wishes to extend its client certificate,
it can request that at any time by doing:

```
POST /edge/{client|management}/current-identity/authenticators/{id}/extend

{
  "clientCertCsr": "-----BEGIN NEW CERTIFICATE REQUEST-----\n..."
}

```

If the authenticator specified by `{id}` is a certificate based authenticator and provisioned by Ziti, it will be allowed.
If not, 4xx HTTP status code errors will be returned outlining the issue. If ok, a 200 OK will be returned in the format of:

```
{
  "clientCert": "-----BEGIN CERTIFICATE-----\n....",
  "ca": ""-----BEGIN CERTIFICATE-----\n...."
}
```

At this point the controller will have stored the new certificate, but it is not usable for authentication until the client
proves that is has properly stored the client certificate. This verification is done by sending the client certificate
back to the controller:

```
POST /edge/{client|management}/current-identity/authenticators{id}/extend-verify
{
  "clientCert": "-----BEGIN CERTIFICATE-----\n...."
}
```

On success, 200 OK is returned and the new client certificate should be used for all future authentication requests. 

# Release 0.24.12

* Enhancement: Allow xgress proxy configuration in router config to accept service id or service name
* Build: Docker build process fixes 

# Release 0.24.11

* Bug fix: Fix ziti CLI env. Config was getting set to current directory, instead of defaulting to under $HOME
* Enhancement: Go tunneler support for service-side resolution of SRV, MX, TXT records for wildcard domains 

# Release 0.24.10

* Bug fix: Fix goroutine leak in channel
    * Regression introduced in v0.24.5
* Bug fix: Deleted routers should now be forcefully disconnected on delete
* Bug fix: Circuit timeouts, and not just failures, should now also incur failure costs on the related terminator when dialing
* Bug fix: Entity count events and the summary REST service now distinguish between fabric and edge service and routers. The edge counts names are suffixed with '.edge'
* Enhancement: Circuit events of all types now include the full set of attributes
* Enhancement: The `ziti edge list summary` now shows entity counts in alphabetical order of the entity type 
* Enhancement: `ziti edge update edge-router` now supports a `--cost` flag which will update a given routers associated cost.
* Enhancement: `ziti fabric list routers` and `ziti edge list routers` will now display the cost of associated routers.

# Release 0.24.9
* Enhancement: `ziti` now has subcommands under `create config` which will properly emit configuration files for 
  `controller`, `router edge` and `router fabric`. 

# Release 0.24.8

* Bug fix: Move control change presence handler notification out of bind handler
* Bug fix: Posture queries now have updatedAt values that increase on state change as well as posture check change
* Enhancement: xweb HTTP servers (edge, fabric REST APIs) now support compression requests from clients via `Accept-Encoding` headers (gzip, br, deflate)

# Release 0.24.7

* Bug fix: bbolt deadlock that could happen if posture cache evaluation coincided with a bbolt mmap operation
    * regression introduced in v0.22.1
* Bug fix: metrics event filtering 
    * regression introduced in 0.24.5 with the metrics name change

# Release 0.24.6

* Update bbolt library to v1.3.6 

# Release 0.24.5

* Enhancement: Durable Eventual Events
* Enhancement: API Session/Service Policy Enforcer Metrics
* Enhancement: Support Controller Address Changes
* Enhancement: Control Channel Metrics Split
* Enhancement: Metrics Output Size Reduction
* Enhancement: Channel Library Updates

## Durable Eventual Events

The controller now supports internal events to delay the processing cost of operations that do not need to resolve
immediately, but must resolve at some point. Events in the controller may pile up at increased load time and that load
level can be seen in a new gauge metric `eventual.events`.

- `eventual.events` - The count of outstanding eventual events

## API Session/Service Policy Enforcer Metrics

New metrics have been added to track internal processes of the controller that enforces API Sessions and Service
Policies.

- `api.session.enforcer.run` - a timer metric of run time of the API Session enforcer
- `api.session.enforcer.delete` - a meter metric of the number of API Sessions deleted
- `service.policy.enforcer.run` - a timer metric of run time of the Service Policy enforcer
- `service.policy.enforcer.event` - a timer metric of the run time for discrete enforcer events
- `service.policy.enforcer.event.deletes` - a meter of the number of signaling delete events processed
- `service.policy.enforcer.run.deletes` - a meter of the number of actual session deletes processed

## Support Controller Address Changes

The Ziti controller now supports additional address fields which can be used to signal endpoint software and routers to
update their configured controller address. The settings are useful in scenarios where moving between IP/hostnames is
desired. Use of these settings has security concerns that must be met in order to maintain connectivity and trust
between endpoint software and routers.

### Security Requirements

These are true for all REST API and control channel addresses.

1) The old IP/hostname and the new IP/hostname must be present on the certificate defined by the `cert` field before
   starting the transition
2) Adding the new IP/hostname to the SANs of an existing controller will require the generating and signing of a new
   certificate
3) The newly generated and signed certificate must still validate with the CAs provided to routers and endpoints
4) The old IP/hostname can only be removed after all in-use routers/endpoints have connected and upgraded addresses

### Process Outline

1) Generate new server certificates with additional SANs for the new IP/hostname - transitional server certificate
2) Update the controller configure to use the new transitional server certificate for the desired listeners (
   control/REST APIs)
3) Restart the controller
4) Upgrade all routers to v0.24.5 or later
5) Upgrade all SDK clients to versions that support controller address changes
6) Verify existing routers and REST API clients can still connect with the old IP/hostname
7) Define the new settings required for the REST APIs (`newAddress`) and/or control channel (`newListener`), see below
8) Restart the controller
9) Verify existing routers and REST API clients configuration files have updated
10) After all clients/routers have updated their addresses, transition the `newAddress` and `newListener` values to the
    default `address` and `listener` fields.
11) Remove the `newAddress` and `newListener` fields.
12) Restart the controller
13) Optionally generate a new server certificate without the old IP/hostname SANs and verify clients/routers can connect

Notes:

- This process may take days, weeks, or months depending on the size of the nework and how often the router/clients are
  run
- It is imperative that all clients/routers that will remain in use after the IP/hostname move connect at least once
  after `newAddress` and `newListener` values are configured and in use
- Clients/routers that do not receive the new address will need to be manually reconfigured by finding their
  configuration file and updating the controller address

### Control Channel Setting

The controller listener defined in the `ctrl` section now supports a `newListener` option which must be a supported
address format (generally in the form of `<protocol>:<host>:<port>`).

Once `newListener` is set, the controller will start to send out the new listener address to connecting routers after
the controller is restarted. All security concerns listed above must be met or routers will not be able to connect to
the controller.

```
ctrl:
  listener: tls:127.0.0.1:6262
  options:
    # (optional) settings
    # ...

    # A listener address which will be sent to connecting routers in order to change their configured controller
    # address. If defined, routers will update address configuration to immediately use the new address for future
    # connections. The value of newListener must be resolvable both via DNS and validate via certificates
    #newListener: tls:localhost:6262
```

### REST API Setting

REST APIs addresses are defined in the `web` section of the controller configuration. The `web` sections
contains `bindPoint`s that define which network interfaces the REST API server will listen on via the
`interface` field. The external address used to access that `bindPoint` is defined by the `address` field. An
additional `newAddress` field can optionally be set.

Once `newAddress` is set, the controller will start to send out the new address to all clients via the HTTP
header `ziti-ctrl-address`. The header will be present on all responses from the controller for the specific
`bindPoint`. All security concerns listed above must be met or client will not be able to connect to the controller.

```
web:
  # name - required
  # Provides a name for this listener, used for logging output. Not required to be unique, but is highly suggested.
  - name: all-apis-localhost
    # bindPoints - required
    # One or more bind points are required. A bind point specifies an interface (interface:port string) that defines
    # where on the host machine the webListener will listen and the address (host:port) that should be used to
    # publicly address the webListener(i.e. mydomain.com, localhost, 127.0.0.1). This public address may be used for
    # incoming address resolution as well as used in responses in the API.
    bindPoints:
      #interface - required
      # A host:port string on which network interface to listen on. 0.0.0.0 will listen on all interfaces
      - interface: 127.0.0.1:1280

        # address - required
        # The public address that external incoming requests will be able to resolve. Used in request processing and
        # response content that requires full host:port/path addresses.
        address: 127.0.0.1:1280

        # newAddress - optional
        # A host:port string which will be sent out as an HTTP header "ziti-new-address" if specified. If the header
        # is present, clients should update location configuration to immediately use the new address for future
        # connections. The value of newAddress must be resolvable both via DNS and validate via certificates
        newAddress: localhost:1280
```

## Control Channel Latency Metrics Changes

The control channel metrics have been broken into two separate metrics. Previously the metric measured how long it took for the message to be enqueued, sent and a reply received. Now the time to write to wire has been broken out.

* `ctrl.latency` - This now measures the time from wire send to response received
* `ctrl.queue_time` - This measure the time from when the send is requested to when it actually is written to the wire

## Metrics Output Size Reduction

If using the JSON metrics events output, the output has changed.

A metrics entry which previously would have looked like:

```
{
  "metric": "ctrl.tx.bytesrate",
  "metrics": {
    "ctrl.tx.bytesrate.count": 222,
    "ctrl.tx.bytesrate.m15_rate": 0.37625904063382576,
    "ctrl.tx.bytesrate.m1_rate": 0.12238911649077193,
    "ctrl.tx.bytesrate.m5_rate": 0.13784280219782497,
    "ctrl.tx.bytesrate.mean_rate": 0.1373326200238093
  },
  "namespace": "metrics",
  "source_entity_id": "z7ZmJux8a7",
  "source_event_id": "7b77ac53-c017-409e-afcc-fd0e1878a301",
  "source_id": "ctrl_client",
  "timestamp": "2022-01-26T21:46:45.866133131Z"
}
```

will now look like:

```
{
  "metric": "ctrl.tx.bytesrate",
  "metrics": {
    "count": 222,
    "m15_rate": 0.37625904063382576,
    "m1_rate": 0.12238911649077193,
    "m5_rate": 0.13784280219782497,
    "mean_rate": 0.1373326200238093
  },
  "namespace": "metrics",
  "source_entity_id": "z7ZmJux8a7",
  "source_event_id": "7b77ac53-c017-409e-afcc-fd0e1878a301",
  "source_id": "ctrl_client",
  "timestamp": "2022-01-26T21:46:45.866133131Z",
  "version" : 2
}
```

Note that the metric keys no longer have the metric name as a prefix. Also, the emitted metric has a new `version` field which is set to 2. 

Metrics with a single key, which previously looked like:

```
{
  "metric": "xgress.acks.queue_size",
  "metrics": {
    "xgress.acks.queue_size": 0
  },
  "namespace": "metrics",
  "source_event_id": "6eb30de2-55de-49d5-828f-4268a3707512",
  "source_id": "z7ZmJux8a7",
  "timestamp": "2022-01-26T22:06:33.242933687Z",
  "version": 2
}
```

now look like:

```
{
  "metric": "xgress.acks.queue_size",
  "metrics": {
    "value": 0
  },
  "namespace": "metrics",
  "source_event_id": "6eb30de2-55de-49d5-828f-4268a3707512",
  "source_id": "z7ZmJux8a7",
  "timestamp": "2022-01-26T22:06:33.242933687Z",
  "version": 2
}
```

## Channel Library Updates

The channel library, which is used by edge communications, control channel, links and management channel, has been refactored. It now does a better job handling canceled messaged through the send process. If a message send times out before it is sent, the message will now no longer be sent when it gets to the head of the queue. Channels can now be instrumented to allow better metrics gathering, as seen above the the split out control channel latency metrics. Channel internals have also been refactored so that initialization is better defined, leading to better concurrency characteristics. 

# Release 0.24.4

## What's New

* Enhancement: Cache sessions for the router/tunneler, to minimize the creation of unnecessary sessions
* Enhancement: Add send timeouts for route messages
* Enhancement: Add write timeout configuration for control channel
* Enhancement: API Session and Session deletes are now separate and eventually consistent
* Enhancement: API Session synchronization with routers no longer blocks database transactions
* Bug fix: fix message priority sorting

## Control Channel Timeouts

The controller config file now allows setting a write timeout for control channel connections. If a control channel
write times out, because the connection is in a bad state or because a router is in a bad state, the control channel
will be closed. This will allow the router to reconnect.

```
ctrl:
  listener:             tls:127.0.0.1:6262
    options:
      # Sets the control channel write timeout. A write timeout will close the control channel, so the router will reconnect
      writeTimeout: 15s
``` 

# Release 0.24.3

## What's New

* Enhancement: API Session delete events now include the related identity id
* Enhancement: controller and router start up messages now include the component id
* Enhancement: New metric `identity.refresh` which counts how often an identity should have to refresh the service list
  because of a service, config or policy change
* Enhancement: Edge REST services will now set the content-length on response, which will prevent response from being
  chunked
* Enhancement: Edge REST API calls will now show in metrics in the format of <path>.<method>
* Bug fix: fix controller panic during circuit creation if router is unexpectedly deleted during routing

# Release 0.24.2

## What's New

* Bug fix: link verification could panic if link was established before control was finished establishing
* Bug fix: When checking edge terminator validity in the router, check terminator id as well the address
* Bug fix: xweb uses idleTimeout correctly, was previously using writeTimeout instead
* Enhancement: Improve logging around links in routers. Ensure we close both channels when closing a split link
* Enhancement: Add support for inspect in `ziti fabric`. Works the same as `ziti-fabric inspect`

# Release 0.24.1

## What's New

* Bug Fix: Very first time using ziti cli to login with `ziti edge login` would panic
* Security: When using new fabric REST API in fabric only mode, certs weren't being properly checked. Regression exists
  only in 0.24.0

# Release 0.24.0

## Breaking Changes

* ziti-fabric-gw has been removed since the fabric now has its own REST API
* ziti-fabric-test is no longer being built by default and won't be included in future release bundles.
  Use `go build --tags all ./...` to build it
* ziti-fabric has been deprecated. Most of its features are now available in the `ziti` CLI under `ziti fabric`

## What's New

* Feature: Fabric REST API
* Performance: Additional route selection work
* Bug Fix: Fix controller deadlock which can happen if a control channel is closed while controller is responding
* Bug fix: Fix panic for UDP-only tproxy intercepts

## Fabric REST API

The fabric now has a REST API in addition to the channel2 management API. To enable it, add the fabric binding to the
apis section off the xweb config, as follows:

```
    apis:
      # binding - required
      # Specifies an API to bind to this webListener. Built-in APIs are
      #   - health-checks
      - binding: fabric
```

If running without the edge, the fabric API uses client certificates for authorization, much like the existing channel2
mgmt based API does. If running with the edge, the edge provides authentication/authorization for the fabric REST APIs.

### Supported Operations

These operations are supported in the REST API. The ziti CLI has been updated to use this in the new `ziti fabric`
sub-command.

* Services: create/read/update/delete
* Routers: create/read/update/delete
* Terminators: create/read/update/delete
* Links: read/update
* Circuits: read/delete

### Unsupported Operations

Some operations from ziti-fabric aren't get supported:

* Stream metrics/traces/circuits
    * This feature may be re-implemented in terms of websockets, or may be left as-is, or may be dropped
* Inspect (get stackdumps)
    * This will be ported to `ziti fabric`
* Decode trace files
    * This may be ported to `ziti-ops`

# Release 0.23.1

## What's New

* Performance: Improve route selection cpu and memory use.
* Bug fix: Fix controller panic in routes.MapApiSessionToRestModel caused by missing return

# Release 0.23.0

## What's New

* Bug fix: Fix panic in router when router is shutdown before control channel is established
* Enhancement: Add source/target router ids on link metrics.
* Security: Fabric management channel wasn't properly validating certs against the server cert chain
* Security: Router link listeners weren't properly validating certs against the server cert chain
* Security: Link listeners now validate incoming links to ensure that the link was requested by the controller and the
  correct router dialed
* Security: Don't allow link forwarding entries to be overriden, as link ids should be unique
* Security: Validate ctrl channel clients against controller cert chain in addition to checking cert fingerprint

## Breaking Changes

The link validation required a controller side and router side component. The controller will continue to work with
earlier routers, but the routers with version >= 0.23.0 will need a controller with version >= 0.23.0.

## Link Metrics Router Ids

The link router ids will now be included as tags on the metrics.

```
{
  "metric": "link.latency",
  "metrics": {
    "link.latency.count": 322,
    "link.latency.max": 844083,
    "link.latency.mean": 236462.8671875,
    "link.latency.min": 100560,
    "link.latency.p50": 212710.5,
    "link.latency.p75": 260137.75,
    "link.latency.p95": 491181.89999999997,
    "link.latency.p99": 820171.6299999995,
    "link.latency.p999": 844083,
    "link.latency.p9999": 844083,
    "link.latency.std_dev": 118676.24663550049,
    "link.latency.variance": 14084051515.49014
  },
  "namespace": "metrics",
  "source_entity_id": "lDWL",
  "source_event_id": "52f9de3e-4293-4d4f-9dc8-5c4f40b04d12",
  "source_id": "4ecTdw8lG6",
  "tags": {
    "sourceRouterId": "CorTdA8l7",
    "targetRouterId": "4ecTdw8lG6"
  },
  "timestamp": "2021-11-10T18:04:32.087107445Z"
}
```

Note that this information is injected into the metric in the controller. If the controller doesn't know about the link,
because of a controller restart, the information can't be added.

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
- `400 BAD REQUESET` w/ standard error messaging for field errors or CSR processing errors

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

* Bug fix: Fix panic in listener close if the socket hadn't been initalized yet
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

Commands under `ziti edge` now reserve the `-i` flag for specifying client identity. Any command line argumet which
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
* ziti egde update ca --identity-atributes (now -a)

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

Previously whenever a router connected we'd look for new links possiblities and create new links between routers where
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

The Ziti CLI now suports multiple identities. An identity can be specified using `--cli-identity` or `-i`.

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
Settting identity 'dev' as default in ~/.config/ziti/ziti-cli.json

$ ziti edge login localhost:1280
Enter username: admin
Enter password: 
Token: e325d91c-a452-4454-a733-cfad88bfa356
Saving identity 'dev' to ~/.config/ziti/ziti-cli.json

$ ziti edge list services
id: -JucPW0kGR    name: ssh    encryption required: true    terminator strategy: smartrouting    role attributes: ["ssh"]
results: 1-1 of 1

$ ziti edge use default
Settting identity 'default' as default in ~/.config/ziti/ziti-cli.json
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

# Release 0.21.0

## Semantic now Required for policies (BREAKING CHANGE)

Previouxly semantic was optional when creating or updating policies (POST or PUT), defaulting to `AllOf` when not
specified. It is now required.

## What's New

* Bug fix: Using PUT for policies without including the semantic would cause them to be evaluated using the AllOf
  semantic
* Bug fix: Additional concurrency fix in posture data
* Feature: Ziti CLI now supports a comprehensive set of `ca` and `cas` options
* Feature: `ziti ps` now supports `set-channel-log-level` and `clear-channel-log-level` operations
* Change: Previouxly semantic was optional when creating or updating policies (POST or PUT), defaulting to `AllOf` when
  not specified. It is now required.

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
    * Services, edge routers and identities can be search by role attribute.
      Ex: `ziti edge list services 'anyOf(roleAttributes) = "one"'`
    * Polices can be searched by roles. Ex: `ziti edge list service-policies 'anyOf(identityRoles) = "#all"'`

# Release 0.20.12

## What's New

* Bug fix: [edge#641](https://github.com/openziti/edge/issues/641)Management and Client API nested resources now
  support `limit` and `offset` outside of `filter` as query params
* Feature: MFA Timeout Options

## MFA Timeout Options

The MFA posture check now supports three options:

* `timeoutSeconds` - the number of seconds before an MFA TOTP will need to be provided before the posture check begins
  to fail (optional)
* `promptOnWake` - reduces the current timeout to 5m (if not less than already) when an endpoint reports a "wake"
  event (optional)
* `promptOnUnlock` - reduces the current timeout to 5m (if not less than already) when an endpoint reports an "unlock"
  event (optional)
* `ignoreLegacyEndpoints` - forces all other options to be ignored for legacy clients that do not support event state (
  optional)

Event states, `promptOnWake` and `promptOnUnlock` are only supported in Ziti C SDK v0.20.0 and later. Individual ZDE/ZME
clients may take time to update. If older endpoint are used with the new MFA options `ignoreLegacyEndpoints` allows
administrators to decide how those clients should be treated. If `ignoreLegacyEndpoints` is `true`, they will not be
subject to timeout or wake events.

# Release 0.20.11

* Bug fix: CLI Admin create/update/delete for UPDB authenticators now function properly
* Maintenance: better logging [sdk-golang#161](https://github.com/openziti/sdk-golang/pull/161)
  and [edge#700](https://github.com/openziti/edge/pull/700)
* Bug fix: [sdk-golang#162](https://github.com/openziti/sdk-golang/pull/162) fix race condition on close of ziti
  connections

# Release 0.20.10

## What's New

* Bug fix: patch for process multi would clear information
* Bug fix: [ziti#420](https://github.com/openziti/ziti/issues/420) fix ziti-tunnel failover with multiple interfaces
  when once becomes unavailable
* Bug fix: [edge#670](https://github.com/openziti/edge/issues/670) fix ziti-tunnel issue where address were left
  assigned to loopback after clean shutdown
* Bug fix: race condition in edge session sync could cause router panic. Regression since 0.20.9
* Bug fix: terminator updates and deletes from the combined router/tunneler weren't working
* Feature: Router health checks
* Feature: Controller health check

## Router Health Checks

Routers can now enable an HTTP health check endpoint. The health check is configured in the router config file with the
new `healthChecks` section.

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

The health check endpoint is configured via XWeb, same as in the controller. As section like the following can be added
to the router config to enable the endpoint.

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

Routers can now enable an HTTP health check endpoint. The health check is configured in the router config file with the
new `healthChecks` section.

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

The health check endpoint is configured via XWeb. In order to enable the health check endpoint, add it **first** to the
list of apis.

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
* Router identity now propagated through the link establishment plumbing. Will facilitate
  router-directed `transport.Configuration` profiles in a future release.
* Bug fix: tunneler identity appData wasn't propagated to tunneler/router
* Bug fix: API session updates were only being sent to one router (regression since 0.20.4)
* Bug fix: API session enforcer wasn't being started (regression since 0.20.0)
* Bug fix: Setting per identity service costs/precedences didn't work with PATCH

### Split Xlink Payload/Ack Channels

Split payload and ack channels are enabled by default, preserving the behavior of previous releases. To disable split
channels, merge the following stanza into your router configuration:

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
* ziti ps stack now takes a --stack-timeout and will quit after the specified timeout if the stack dump hasn't completed
  yet
* ziti now supports posture check types of process multi
* Fixes a bug in Ziti Management API where posture checks of type process multi were missing their base entity
  information (createdAt, updatedAt, etc.)

# Release 0.20.1

## What's New

* Fixes a bug in the GO sdk which could cause panic by return nil connection and nil error
* [ziti#170](https://github.com/openziti/ziti/issues/170) Fixes the service poll refresh default for ziti-tunnel host
  mode
* Fixes a deadlock in control channel reconnect logic triggerable when network path to controller is unreliable

# Release 0.20.0

## What's New

* Fix bug in router/tunneler where only first 10 services would get picked up for intercepting/hosting
* Fix bug in router/tunneler where we'd process services multiple times on service add/remove/update
* Historical Changelog Split
* Edge Management REST API Transit Router Deprecation
* Edge REST API Split & Configuration Changes

### Historical Changelog Split

Changelogs for previous minor versions are now split into their own files under `/changelogs`.

### Edge Management REST API Transit Router Deprecation

The endpoint `/transit-routers` is now `/routers`. Use of the former name is considered deprecated. This endpoint only
affects the new Edge Management API.

### Edge REST API Split

The Edge REST API has now been split into two APIs: The Edge Client API and the Edge Management API. There are now two
Open API 2.0 specifications present in the `edge` repository under `/specs/client.yml`
and `/specs/management.yml`. These two files are generated (see the scripts in `/scripts/`) from decomposed YAML source
files present in `/specs/source`.

The APIs are now hosted on separate URL paths:

- Client API: `/edge/client/v1`
- Management API: `/edge/management/v1`

Legacy path support is present for the Client API only. The Management API does not support legacy URL paths. The Client
API Legacy paths that are supported are as follows:

- No Prefix: `/*`
- Edge Prefix: `/edge/v1/*`

This support is only expected to last until all Ziti SDKs move to using the new prefixed paths and versions that do not
reach the end of their lifecycle. After that time, support will be removed. It is highly  
suggested that URL path prefixes be updated or dynamically looked up via the `/version` endpoint (see below)

#### Client and Management API Capabilities

The Client API represents only functionality required by and endpoint to connected to and use services. This API
services Ziti SDKs.

The Management API represents all administrative configuration capabilities. The Management API is meant to be used by
the Ziti Admin Console (ZAC) or other administrative integrations.

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

The underlying framework used to host the Edge REST API has been moved into a new library that can be found in
the `fabric` repository under the module name `xweb`. XWeb allows arbitrary APIs and website capabilities to be hosted
on one or more http servers bound to any number of network interfaces and ports.

The main result of this is that the Edge Client and Management APIs can be hosted on separate ports or even on separate
network interfaces if desired. This allows for configurations where the Edge Management API is not accessible outside of
localhost or is only presented to network interfaces that are inwardly facing.

The introduction of XWeb has necessitated changes to the controller configuration. For a full documented example see the
file `/etc/ctrl.with.edge.yml` in this repository.

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

The `web` section now allows Ziti APIs to be configured on various network interfaces and ports according to deployment
requirements. The `web` section is an array of configuration that defines `WebListener`s. Each `WebListener` has its own
HTTP configuration, `BindPoint`s, identity override, and `API`s which are referenced by `binding` name.

Each `WebListener` maps to at least one HTTP server that will be bound on at least one `BindPoint`
(network interface/port combination and external address) and will host one or more `API`s defined in the `api`
section. `API`s are configured by `binding` name. The following `binding` names are currently supported:

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

All optional values are defaulted. The smallest configuration possible that places the Edge Client and Managements APIs
on the same `BindPoint` would be:

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

The following examples places the Management API on localhost and the Client API on all available interface and
advertised as `client.api.ziti.dev:1280`:

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

All Edge APIs support the `/version` endpoint and report all the APIs supported by the controller. Each API now has
a `binding` (string name) which is a global handle for that API's capabilities. See the current list below

- Client API: `edge-client`, `edge`
- Management API: `edge-management`

Note: `edge` is an alias of `edge-client` for the `/version` endpoint only. It is considered deprecated.

These `bind names` can be used to parse the information returned by the `/version` endpoint to obtain the most correct
URL path for each API and version present. At a future date, other APIs with new `binding`s
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
