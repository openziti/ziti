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
    * [Issue #317](https://github.com/openziti/sdk-golang/issues/317) - No documentation for call example
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
        version: 3
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

Links have a 64 message queue for incoming messages. Up to 64 messages are taken off the queue, sorted in priority order and then sent. Once the sorted list of messages has been sent, the next set of messages are dequeue, sorted and sent. If the queue fills while the current set of sorted messages is being sent, message will now be dropped instead of waiting for queue space to open up.

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
* `routerLinkNew` : Generated when a router sends a router link message to the controller and the link is new to the controller
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

Formerly metrics could be exported to file via the `metrics:` configuration stanza. This was superseded by
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
