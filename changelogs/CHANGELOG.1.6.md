# Release 1.6.8

## What's New

* Bug fixes and library updates
* Session Events for JWT Sessions
* OIDC Fix when using a separate certificate for the API

## Session Events for JWT sessions

When using JWT sessions, instead of legacy sessions, session events will now be created.
There is a new `provider` field in session events, whose value will either be `legacy` or `jwt`.

## OIDC Fix 

There was an issue where OIDC authentication would fail if the API was configured with a different 
certificate than the controller's root identity certificate. 

The v1.2.3 release of the Go SDK made OIDC the default, if the controller supported it. Since the
quickstart uses separate certs certs, this was quickly noticed. If using the v1.2.3 release of
the Go SDK, and affected by this issue, updating to OpenZiti controller v1.6.8 should resolve the 
problem.

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.30 -> v1.0.31](https://github.com/openziti/agent/compare/v1.0.30...v1.0.31)
* github.com/openziti/channel/v4: [v4.2.21 -> v4.2.28](https://github.com/openziti/channel/compare/v4.2.21...v4.2.28)
* github.com/openziti/foundation/v2: [v2.0.70 -> v2.0.72](https://github.com/openziti/foundation/compare/v2.0.70...v2.0.72)
* github.com/openziti/identity: [v1.0.109 -> v1.0.111](https://github.com/openziti/identity/compare/v1.0.109...v1.0.111)
* github.com/openziti/runzmd: [v1.0.77 -> v1.0.80](https://github.com/openziti/runzmd/compare/v1.0.77...v1.0.80)
* github.com/openziti/sdk-golang: [v1.2.2 -> v1.2.3](https://github.com/openziti/sdk-golang/compare/v1.2.2...v1.2.3)
    * [Issue #779](https://github.com/openziti/sdk-golang/issues/779) - Remove need to EnableHA flag in Go SDK

* github.com/openziti/secretstream: [v0.1.38 -> v0.1.39](https://github.com/openziti/secretstream/compare/v0.1.38...v0.1.39)
* github.com/openziti/storage: [v0.4.22 -> v0.4.26](https://github.com/openziti/storage/compare/v0.4.22...v0.4.26)
* github.com/openziti/transport/v2: [v2.0.183 -> v2.0.188](https://github.com/openziti/transport/compare/v2.0.183...v2.0.188)
* github.com/openziti/ziti: [v1.6.7 -> v1.6.8](https://github.com/openziti/ziti/compare/v1.6.7...v1.6.8)
    * [Issue #3207](https://github.com/openziti/ziti/issues/3207) - Allow router embedders to customize config before start
    * [Issue #3241](https://github.com/openziti/ziti/issues/3241) - Disconnecting Routers May Have Nil Fingerprint, causes panic
    * [Issue #3248](https://github.com/openziti/ziti/issues/3248) - let cluster agent also support unix domain sockets
    * [Issue #3219](https://github.com/openziti/ziti/issues/3219) - AuthenticatorManager ReadByFingerprint/Username should use indexes
    * [Issue #3225](https://github.com/openziti/ziti/issues/3225) - JWT edge sessions should generate events
    * [Issue #3245](https://github.com/openziti/ziti/issues/3245) - Revocation time check is checking wrong entity
    * [Issue #3231](https://github.com/openziti/ziti/issues/3231) - OIDC authentication fails if the client api has a separate cert chain
    * [Issue #3239](https://github.com/openziti/ziti/issues/3239) - Router JWTs use Identity expiration configuration value
    * [Issue #3226](https://github.com/openziti/ziti/issues/3226) - Only report router network interfaces if controller supports receiving those events
    * [Issue #3164](https://github.com/openziti/ziti/issues/3164) - Router data model doesn't work correctly if the edge listener isn't enabled

# Release 1.6.7

## What's New

* Bug fixes and library updates

## Component Updates and Bug Fixes

* github.com/openziti/channel/v4: [v4.2.18 -> v4.2.21](https://github.com/openziti/channel/compare/v4.2.18...v4.2.21)
    * [Issue #203](https://github.com/openziti/channel/issues/203) - Track last dial time in UnderlayConstraints

* github.com/openziti/edge-api: [v0.26.46 -> v0.26.47](https://github.com/openziti/edge-api/compare/v0.26.46...v0.26.47)
* github.com/openziti/sdk-golang: [v1.2.1 -> v1.2.2](https://github.com/openziti/sdk-golang/compare/v1.2.1...v1.2.2)
    * [Issue #786](https://github.com/openziti/sdk-golang/issues/786) - Slow down dials to an ER if they happen too quickly
    * [Issue #784](https://github.com/openziti/sdk-golang/issues/784) - Drop retransmit error to debug

* github.com/openziti/secretstream: [v0.1.37 -> v0.1.38](https://github.com/openziti/secretstream/compare/v0.1.37...v0.1.38)
* github.com/openziti/transport/v2: [v2.0.182 -> v2.0.183](https://github.com/openziti/transport/compare/v2.0.182...v2.0.183)
* github.com/openziti/ziti: [v1.6.6 -> v1.6.7](https://github.com/openziti/ziti/compare/v1.6.6...v1.6.7)
    * [Issue #3199](https://github.com/openziti/ziti/issues/3199) - Other routers don't react to link listener address changes
    * [Issue #3178](https://github.com/openziti/ziti/issues/3178) - Controller List Edge APIs Missing
    * [Issue #3193](https://github.com/openziti/ziti/issues/3193) - Add flag to TOTP auth query with enrollment state
    * [Issue #3162](https://github.com/openziti/ziti/issues/3162) - Update go tunnel implementation to support multiple interfaces

# Release 1.6.6

## What's New

* SDK Flow Control Updates
* Multi-underlay links
* Nested Identity App Data

## SDK Flow Control Updates

The circuit testing for sdk flow control is complete. Many fixes were made. The SDK flow control
code is still considered experimental, in that the features or API may change. However, it should
now be feature complete and relatively stable.. Once it has been in production use for a reasonable
 period and no further changes are anticipated, it will be marked stable.

## Multi-underlay Link

In previous releases, routers would attempt to set up two connections per link, one for payloads and one for acks.
If either one failed, the whole link would be torn down. With this release, links can be made up of a
user-configurable number of connections. 

### Link Connection Types

Link connections are of two types:

* default - These may carry payloads and acks. As long as there is at least one default connection, the link will stay up.
* ack - These may carry only acks. They act as a prioritization mechanism for acks. There may be zero ack connections.

The desired number of default and ack channels can be configured in the router configuration.

```yaml
link:
  dialers:
    - binding: transport

      # Target number of default connections. Allowed range 1-100. Defaults to 3.
      maxDefaultConnections: 3
  
      # Target number of ack connections. Allowed range 1-100. Defaults to 1.
      maxAckConnections: 1

      # Time to delay making additional connections after the initial connection. Defaults to 3s
      # Reduces connection churn when routers are dialing each other at the same time.
      startupDelay: 3s
```

It's recommended to configure at least two connections per link.

**Why Multiple Connections?**

1. They allow  for link continuity even if one of the connections goes down. 
2. They can keep traffic moving if one of the connections stalls for some reason.
3. Using multiple links also multiples the number of OS buffers in use, although the amount of per-connection buffers can also be bumped up at the OS level. 

**Why a ACK Priority Connection?**

If a payload gets dropped, it will need to be retransmitted. If an ack gets dropped, a payload
that's already been received will be retransmitted. Acks are also generally much smaller than 
payloads. The faster we can deliver them, the faster the flow control logic can react.

**How Many Connections?**

At least two. However, having more connections doesn't increase the physical bandwidth available between routers. Some 
additional connections provide additional resilience and perhaps more performance due to increased OS resources. However,
the benefits diminish quickly. More than the default of three is unlikely to provide much benefit. 

**How is traffic load-balanced?**

There is a queue for payloads and other for acks. Default connections pull from both queues, ack connections only pull from
the ack queue. Because connections pull from the queues, if one connection is slower it will naturally pull fewer messages
than other connections.

### Backwards Compatibility

When creating links to a router older than 1.6.6, routers will fallback to the old logic and dial one payload and one
ack channel. 

### Link Events

Links will now report their connections to the controller. They are now reported when listing links using `ziti fabric list links`.

Here is an example from a test setup.

```
$ ziti fabric list links 'skip 3 limit 2'
╭────────────────────────┬───────────────────────┬────────────────────────┬─────────────┬─────────────┬─────────────┬───────────┬────────┬───────────┬──────────────────────────────────────────────────────────────╮
│ ID                     │ DIALER                │ ACCEPTOR               │ STATIC COST │ SRC LATENCY │ DST LATENCY │ STATE     │ STATUS │ FULL COST │ CONNECTIONS                                                  │
├────────────────────────┼───────────────────────┼────────────────────────┼─────────────┼─────────────┼─────────────┼───────────┼────────┼───────────┼──────────────────────────────────────────────────────────────┤
│ 101OzJLiMrrFSpwT0LnYOY │ router-eu-central-3.7 │ router-eu-central-2.11 │           1 │       2.7ms │       2.7ms │ Connected │     up │         5 │ link.default: tcp:10.0.0.230:40028 -> tcp:54.93.210.111:6011 │
│                        │                       │                        │             │             │             │           │        │           │ link.default: tcp:10.0.0.230:40032 -> tcp:54.93.210.111:6011 │
│                        │                       │                        │             │             │             │           │        │           │ link.ack: tcp:10.0.0.230:46092 -> tcp:54.93.210.111:6011     │
│                        │                       │                        │             │             │             │           │        │           │ link.default: tcp:10.0.0.230:46096 -> tcp:54.93.210.111:6011 │
│ 101YAe327nSngeRIXeKR0T │ router-eu-central-3.5 │ router-us-east-4.17    │           1 │      91.5ms │      91.4ms │ Connected │     up │       183 │ ack: tcp:10.0.0.230:57574 -> tcp:13.220.214.103:6017         │
│                        │                       │                        │             │             │             │           │        │           │ payload: tcp:10.0.0.230:57568 -> tcp:13.220.214.103:6017     │
╰────────────────────────┴───────────────────────┴────────────────────────┴─────────────┴─────────────┴─────────────┴───────────┴────────┴───────────┴──────────────────────────────────────────────────────────────╯
results: 4-5 of 79803
```

A link is considered created once it has an initial default connection. The link will then attempt to reach the desired count of default 
and ack connections. Whenever a new underlay connection is established or closes, the controller will be notified and an event will
be generated.

Link event example:

```
{
  "namespace": "link",
  "event_src_id": "ctrl_client",
  "timestamp": "2025-07-11T10:35:01.614896435-04:00",
  "event_type": "connectionsChanged",
  "link_id": "7mCYLrQAiO93du7SLGDeXf",
  "connections": [
    {
      "id": "link.default",
      "local_addr": "tcp:127.0.0.1:33682",
      "remote_addr": "tcp:127.0.0.1:4024"
    },
    {
      "id": "link.default",
      "local_addr": "tcp:127.0.0.1:33686",
      "remote_addr": "tcp:127.0.0.1:4024"
    },
    {
      "id": "link.ack",
      "local_addr": "tcp:127.0.0.1:33696",
      "remote_addr": "tcp:127.0.0.1:4024"
    },
    {
      "id": "link.default",
      "local_addr": "tcp:127.0.0.1:33702",
      "remote_addr": "tcp:127.0.0.1:4024"
    }
  ]
}
```

**NOTES**

1. Link events show the full set of connections for the current state instead of the change.
2. New routers dialing older routers will still report link connections. See the second link in the list above.
3. Old routers will not report connections.

## Nested Identity App Data

Identity app data may now be a full JSON document, rather than just a single layer map. There 
are also some additional CLI methods to work with the data:

```
$ ziti edge create identity test --app-data foo=bar
$ ziti edge create identity test --app-data-json '{ "foo" : "bar", "test" : { "nested" : true, "number" : 234 } }'
$ ziti edge create identity test --app-data-json-file test-app-data.json 

$ ziti edge update identity test --app-data foo=bar
$ ziti edge update identity test --app-data-json '{ "foo" : "bar", "test" : { "nested" : true, "number" : 234 } }'
$ ziti edge update identity test --app-data-json-file test-app-data.json 
```

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.29 -> v1.0.30](https://github.com/openziti/agent/compare/v1.0.29...v1.0.30)
    * [Issue #27](https://github.com/openziti/agent/issues/27) - Add support for generating heap dumps

* github.com/openziti/channel/v4: [v4.2.13 -> v4.2.18](https://github.com/openziti/channel/compare/v4.2.13...v4.2.18)
    * [Issue #201](https://github.com/openziti/channel/issues/201) - SendAndWait methods should return an error if the channel closes instead of blocking
    * [Issue #199](https://github.com/openziti/channel/issues/199) - Reject multi-underlay connections that are the first connection for a channel, but aren't marked as such.
    * [Issue #197](https://github.com/openziti/channel/issues/197) - Break out of dial loop if channel is closed

* github.com/openziti/foundation/v2: [v2.0.69 -> v2.0.70](https://github.com/openziti/foundation/compare/v2.0.69...v2.0.70)
* github.com/openziti/identity: [v1.0.108 -> v1.0.109](https://github.com/openziti/identity/compare/v1.0.108...v1.0.109)
* github.com/openziti/runzmd: [v1.0.76 -> v1.0.77](https://github.com/openziti/runzmd/compare/v1.0.76...v1.0.77)
* github.com/openziti/sdk-golang: [v1.1.2 -> v1.2.1](https://github.com/openziti/sdk-golang/compare/v1.1.2...v1.2.1)
    * [Issue #777](https://github.com/openziti/sdk-golang/issues/777) - OIDC auth token refresh doesn't fall back to re-auth if token has expired
    * [Issue #772](https://github.com/openziti/sdk-golang/issues/772) - xgress close tweaks
    * [Issue #769](https://github.com/openziti/sdk-golang/issues/769) - Require sdk flow control when using more than one default connection
    * [Issue #765](https://github.com/openziti/sdk-golang/issues/765) - Allow independent close of xgress send and receive
    * [Issue #763](https://github.com/openziti/sdk-golang/issues/763) - Use a go-routine pool for payload ingest
    * [Issue #761](https://github.com/openziti/sdk-golang/issues/761) - Use cmap.ConcurrentMap for message multiplexer
    * [Issue #754](https://github.com/openziti/sdk-golang/issues/754) - panic: unaligned 64-bit atomic operation when running on 32-bit raspberry pi
    * [Issue #757](https://github.com/openziti/sdk-golang/issues/757) - Not authenticated check fails on session create when using OIDC

* github.com/openziti/secretstream: [v0.1.36 -> v0.1.37](https://github.com/openziti/secretstream/compare/v0.1.36...v0.1.37)
* github.com/openziti/storage: [v0.4.20 -> v0.4.22](https://github.com/openziti/storage/compare/v0.4.20...v0.4.22)
* github.com/openziti/transport/v2: [v2.0.180 -> v2.0.182](https://github.com/openziti/transport/compare/v2.0.180...v2.0.182)
* github.com/openziti/ziti: [v1.6.5 -> v1.6.6](https://github.com/openziti/ziti/compare/v1.6.5...v1.6.6)
    * [Issue #3183](https://github.com/openziti/ziti/issues/3183) - Add support for generating heap dumps using the agent
    * [Issue #3161](https://github.com/openziti/ziti/issues/3161) - Allow setting structured data in identity appData from CLI
    * [Issue #3169](https://github.com/openziti/ziti/issues/3169) - Allow identity app data to be a full JSON document, rather than just a flat map
    * [Issue #3134](https://github.com/openziti/ziti/issues/3134) - Support multi-underlay links
    * [Issue #3165](https://github.com/openziti/ziti/issues/3165) - Docker controller doesn't renew identity


# Release 1.6.5

## What's New

Bugfixes and dependency updates.

## Component Updates and Bug Fixes

* github.com/openziti/channel/v4: [v4.2.8 -> v4.2.13](https://github.com/openziti/channel/compare/v4.2.8...v4.2.13)
    * [Issue #194](https://github.com/openziti/channel/issues/194) - Add GetUnderlays and GetUnderlayCountsByType to Channel

* github.com/openziti/foundation/v2: [v2.0.66 -> v2.0.69](https://github.com/openziti/foundation/compare/v2.0.66...v2.0.69)
    * [Issue #443](https://github.com/openziti/foundation/issues/443) - Allow injecting custom method into go-routine pools, to allow identifying them in stack dumps

* github.com/openziti/identity: [v1.0.105 -> v1.0.108](https://github.com/openziti/identity/compare/v1.0.105...v1.0.108)
* github.com/openziti/metrics: [v1.4.1 -> v1.4.2](https://github.com/openziti/metrics/compare/v1.4.1...v1.4.2)
* github.com/openziti/runzmd: [v1.0.73 -> v1.0.76](https://github.com/openziti/runzmd/compare/v1.0.73...v1.0.76)
* github.com/openziti/storage: [v0.4.17 -> v0.4.20](https://github.com/openziti/storage/compare/v0.4.17...v0.4.20)
* github.com/openziti/transport/v2: [v2.0.177 -> v2.0.180](https://github.com/openziti/transport/compare/v2.0.177...v2.0.180)
* github.com/openziti/xweb/v2: [v2.3.3 -> v2.3.4](https://github.com/openziti/xweb/compare/v2.3.3...v2.3.4)
* github.com/openziti/ziti: [v1.6.3 -> v1.6.5](https://github.com/openziti/ziti/compare/v1.6.3...v1.6.5)
    * [Issue #3149](https://github.com/openziti/ziti/pull/3149) - add dial/bind type column to sp list

# Release 1.6.4

This version was intentionally skipped and not released because the 1.6.3 FIPS binary had an erroneous internal version of 1.6.4.

# Release 1.6.3

## What's New

* Router Network Interface Discovery

## Router Interface Discovery

Routers can now discover their network interfaces and publish this information to the
controller. 

This feature will be used in the future to allow controller side configuration of
router link listeners and edge listeners. 

### Update Router Configuration

There is new router configuration to manage this:

```
interfaceDiscovery:
  # This feature is enabled by default, but can be disabled by setting this to true
  disabled: false

  # How often to poll for interface changes. Defaults to 1 minute
  checkInterval: 1m

  # How often to report the current set of interfaces, when nothing has changed.
  # This is a failsafe reporting mechanism, in the very unlikely event that an 
  # earlier change report was lost or disregarded due to distributed controller
  # eventual consistency
  minReportInterval: 24h
```

### Update REST APIs

Network interfaces, where reported, can now be viewed on the following endpoints.

* routers
* edge-routers
* identities (if the router is an ER/T, with an associated identities)

At some point in the future, we expect to allow SDKs to also optionally report their
network interfaces as well. Those will be available via the `identities` REST API.

Example:

```
$ ziti fabric list routers 'name="edge-router-1"' -j | jq
{
  "data": [
    {
      "_links": {
        "self": {
          "href": "./routers/oLvcT6VepI"
        },
        "terminators": {
          "href": "./routers/oLvcT6VepI/terminators"
        }
      },
      "createdAt": "2025-03-24T04:35:59.077Z",
      "id": "oLvcT6VepI",
      "tags": {},
      "updatedAt": "2025-06-11T14:29:22.083Z",
      "connected": false,
      "cost": 0,
      "disabled": false,
      "fingerprint": "b7b03c55be77df0ec57e49d8fe2610e0b99fa61c",
      "interfaces": [
        {
          "addresses": [
            "192.168.3.29/24",
            "aaaa::aaaa:aaaa:aaaa:aaaa/64"
          ],
          "hardwareAddress": "aa:aa:aa:aa:aa:aa",
          "index": 4,
          "isBroadcast": true,
          "isLoopback": false,
          "isMulticast": true,
          "isRunning": true,
          "isUp": true,
          "mtu": 1500,
          "name": "wifi0"
        },
        {
          "addresses": null,
          "hardwareAddress": "aa:aa:aa:aa:aa:aa",
          "index": 2,
          "isBroadcast": true,
          "isLoopback": false,
          "isMulticast": true,
          "isRunning": false,
          "isUp": true,
          "mtu": 1500,
          "name": "eth1"
        },
        {
          "addresses": [
            "192.168.1.2/24",
            "aaaa::aaaa:aaa:aaaa:aaaa/64"
          ],
          "hardwareAddress": "aa:aa:aa:aa:aa:aa",
          "index": 16,
          "isBroadcast": true,
          "isLoopback": false,
          "isMulticast": true,
          "isRunning": true,
          "isUp": true,
          "mtu": 1500,
          "name": "eth0"
        },
        {
          "addresses": [
            "127.0.0.1/8",
            "::1/128"
          ],
          "hardwareAddress": "",
          "index": 1,
          "isBroadcast": false,
          "isLoopback": true,
          "isMulticast": false,
          "isRunning": true,
          "isUp": true,
          "mtu": 65536,
          "name": "lo"
        }
      ],
      "listenerAddresses": null,
      "name": "edge-router-1",
      "noTraversal": false
    }
  ],
  "meta": {
    "filterableFields": [
      "id",
      "isSystem",
      "name",
      "fingerprint",
      "cost",
      "createdAt",
      "updatedAt",
      "tags",
      "noTraversal",
      "disabled",
      "connected"
    ],
    "pagination": {
      "limit": 10,
      "offset": 0,
      "totalCount": 1
    }
  }
}
```

Note that addresses have been sanitized.


## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.27 -> v1.0.29](https://github.com/openziti/agent/compare/v1.0.27...v1.0.29)
* github.com/openziti/channel/v4: [v4.2.0 -> v4.2.8](https://github.com/openziti/channel/compare/v4.2.0...v4.2.8)
* github.com/openziti/edge-api: [v0.26.45 -> v0.26.46](https://github.com/openziti/edge-api/compare/v0.26.45...v0.26.46)
    * [Issue #155](https://github.com/openziti/edge-api/issues/155) - Add network interface list to routers and identities

* github.com/openziti/foundation/v2: [v2.0.63 -> v2.0.66](https://github.com/openziti/foundation/compare/v2.0.63...v2.0.66)
* github.com/openziti/identity: [v1.0.101 -> v1.0.105](https://github.com/openziti/identity/compare/v1.0.101...v1.0.105)
* github.com/openziti/runzmd: [v1.0.72 -> v1.0.73](https://github.com/openziti/runzmd/compare/v1.0.72...v1.0.73)
* github.com/openziti/sdk-golang: [v1.1.1 -> v1.1.2](https://github.com/openziti/sdk-golang/compare/v1.1.1...v1.1.2)
    * [Issue #742](https://github.com/openziti/sdk-golang/issues/742) - Additional CtrlId and GetDestinationType for inspect support
    * [Issue #739](https://github.com/openziti/sdk-golang/issues/739) - go-jose v2.6.3 CVE-2025-27144 resolution
    * [Issue #735](https://github.com/openziti/sdk-golang/issues/735) - Ensure Authenticate can't be called in parallel

* github.com/openziti/secretstream: [v0.1.34 -> v0.1.36](https://github.com/openziti/secretstream/compare/v0.1.34...v0.1.36)
* github.com/openziti/storage: [v0.4.11 -> v0.4.17](https://github.com/openziti/storage/compare/v0.4.11...v0.4.17)
    * [Issue #106](https://github.com/openziti/storage/issues/106) - panic in TypedBucket.GetList

* github.com/openziti/transport/v2: [v2.0.171 -> v2.0.177](https://github.com/openziti/transport/compare/v2.0.171...v2.0.177)
* github.com/openziti/xweb/v2: [v2.3.2 -> v2.3.3](https://github.com/openziti/xweb/compare/v2.3.2...v2.3.3)
* github.com/openziti/ziti: [v1.6.2 -> v1.6.3](https://github.com/openziti/ziti/compare/v1.6.2...v1.6.3)
    * [Issue #3124](https://github.com/openziti/ziti/issues/3124) - ids used by circuits and ingress/egress can conflict in an HA setup
    * [Issue #3117](https://github.com/openziti/ziti/issues/3117) - authenticators LastAuthResolvedToRoot not set, createdAt/lastUpdateAt zero zulu
    * [Issue #3111](https://github.com/openziti/ziti/issues/3111) - Add API for xgress router factories allowing router env injection
    * [Issue #3119](https://github.com/openziti/ziti/issues/3119) - Using the same heartbeatmsg instance across channels causes data race
    * [Issue #3115](https://github.com/openziti/ziti/issues/3115) - Fix racy link state access in router link registry
    * [Issue #3113](https://github.com/openziti/ziti/issues/3113) - Close links when link groups no longer indicate that a link should be allowed
    * [Issue #3082](https://github.com/openziti/ziti/issues/3082) - Add network interfaces to controller data model
    * [Issue #3083](https://github.com/openziti/ziti/issues/3083) - Add optional network interface discovery to routers
    * [Issue #2862](https://github.com/openziti/ziti/issues/2862) - Large scale data-flow test
    * [Issue #3102](https://github.com/openziti/ziti/issues/3102) - Implement remote control for ziti-traffic-test/loop4
    * [Issue #3098](https://github.com/openziti/ziti/issues/3098) - Implement circuit validation API and CLI

# Release 1.6.2

## What's New

* System Certificate Authentication Improper Chain Detection
* Authentication Events
* Multi-underlay channel group secret
* Flag to disable posture check functionality

## System Certificate Authentication Improper Chain Detection

Previous versions of SDKs and controllers issued and stored client certificate chains with differing levels of fidelity.
Depending on when an identity was enrolled, it may or may not have a proper client certificate chain. In single
controller environments, the controller will automatically include its known intermediates to help create valid
x509 certificate chains back to the network's Root CA. 

In HA environments, each controller does not know of all the intermediates in the system by default. In order to support
dynamically scaling controllers, clients must store and provide a proper client certificate chain (a leaf certificate 
followed by all necessary intermediate CA certs). Identities enrolled with SDKs that did not store the chain
or controllers that did not provide the chain will encounter authentication issues in multi-controller environments.

To help determine which identities have issues, the system has been augmented to allow detection of problematic 
certificate authenticators. Firstly, authenticators have flags on them that are set during authentication to detect
the current behavior of a specific client that owns and is invoking the certificate authenticator.

### Detecting Improper Client Cert Chains As A Client SDK

The current API Sessions endpoint (`https://<host>/edge/client/v1/current-api-session`) now returns a value named
`improperClientCertChain` as a boolean value. If it is not present or `false` no action should be taken. If `true`
it means that the current API Session was authenticated with an internal PKI issued certificate where the client
did not provide a full chain to the root CA during authentication; indicating a problem with the certificate storage
mechanism in the application or due to the controller version used during enrollment/extension not providing a chain. 
The SDK should proactively opt to begin certificate extension on its own to obtain a proper chain. Authentication
succeeded in this case because the controller relied upon a deprecated certificate pool that happen to include the necessary
intermediate CAs.

### Detecting Clients Without Proper Chains

After a client has authenticated with the system via a network-issued certificate at least one time, a number of fields
are set depending on what the client provided. These values can be reviewed via the Edge Management API for 
Authenticators (`edge/management/v1/authenticators`). Where an authenticator has `isIssuedByNetwork` set to `true` and
`lastAuthResolvedToRoot` set to `false`, indicates that the related identity/client is not providing a chain when it
should.

Additionally, if authenticator events are enabled and being processed, events will have a field 
`improper_client_cert_chain` set to `true` (see Authentication Events below)

## Fixing Clients Chains

Once an authenticator has been identified as problematic, an administrator should verify the client is using the newest
possible versions of its SDK and either re-enroll it or request the identity to extend the next time it 
authenticates. Both scenarios result in a new certificate and chain being provided to the client.

- **Re-Enrollment** removes the current authenticator, making authentication impossible until enrollment is completed.
  Once completed, a new authenticator is created, and the new certificate generated from the process can be used to 
  authenticate. Humans or other external automation processes have to deliver and consume the new enrollment JWT.
- **Extension** leaves the authenticator in place and authentication can still occur. When the extension process is
  completed, the authenticator is updated and will use the new certificate generated from the process. SDKs will 
  handle the process by raising SDK specific events to drive the process.

While related and similar, using one over the other depends on the situation. Re-Enrollment is best for when
an client or its host are unrecoverable; either through damage, decommissioning, or suspected to be compromised.
Extension is useful when the client and host are in a known good status and one simply wants to issue new
certificates to said client. Extension is also "hands off" from the client as long as the client is using 
SDKs that support enrollment request/key rolling.

## Useful Authenticator Values

Authenticators are an entity within OpenZiti and can be queried via the CLI (`ziti edge list authenticators`) or
Edge Management API (`GET https://<host>/edge/management/v1/authenticators`). Below are properties that are 
useful for determining clients with improper chains.

- `isIssuedByNetwork` (boolean) indicates the authenticator is a certificate that was issued by the network
- `isExtendRequested` (boolean) indicated the authenticator will report to the client to extend its certificate on next
  authentication.
- `isKeyRollRequested` (boolean) indicates if key rolling is requested with an outstanding `isExtendRequested`
- `extendRequestedAt` (string, date time) indicates when an outstanding extension was requested, otherwise `null`
  or not provided
- `lastAuthResolvedToRoot` (boolean) indicates if the last time the client authenticated, it provided a certificate
  chain that resolved to the root CA. Only valid if `isIssuedByNetwork` is `true`.

### Re-Enrollment

Re-enrollment can be accomplished via the CLI or the Edge Management API. Re-enrollment removes the current authenticator,
stopping the client from authenticating. All of the underlying configuration for the related identity is preserved. 
The newly created enrollment will have a new enrollment JWT that will be consumed during the enrollment process and 
will result in a new client certificate and chain.

**CLI:**

`ziti edge update authenticator cert <id> --re-enroll [--duration <duration>]`

**Edge Management API:**

```http request
POST /edge/management/v1/authenticators/:id/re-enroll

{
    expiresAt: "2025-05-21T13:47:26Z"
}
```

Both of the above provide an enrollment id and or enrollment JWT token as output. These can be consumed in various
OpenZiti applications or CLI commands.

**CLI Enrollment**

```
 ziti edge enroll -h
enroll an identity

Usage:
  ziti edge enroll path/to/jwt [flags]

Flags:
      --ca string         Additional trusted certificates
  -c, --cert string       The certificate to present when establishing a connection.
  -h, --help              help for enroll
  -n, --idname string     Names the identity. Ignored if not 3rd party auto enrollment
  -j, --jwt string        Enrollment token (JWT file). Required
  -k, --key string        The key to use with the certificate. Optionally specify the engine to use. supported engines: [parsec]
  -a, --keyAlg RSA|EC     Crypto algorithm to use when generating private key (default RSA)
  -o, --out string        Output configuration file.
  -p, --password string   Password for updb enrollment, prompted if not provided and necessary
      --rm                Remove the JWT on success
  -u, --username string   Username for updb enrollment, prompted if not provided and necessary
  -v, --verbose           Enable verbose logging
```

**OpenZiti Tunneler Enrollment**

`ziti-edge-tunnel add --jwt "$(< ./in-file.jwt)" --identity myIdentityName`

### Request Extension

Extension leaves the authenticator in place, but notifies SDKs that they should generate a CSR to request a new
certificate. Once the process is complete, the client will have a new certificate and chain that must be used
for later authentication requests. This can be initiated through the CLI or Edge Management API.

**CLI:**

`ziti edge update authenticator cert <id> --request-extend [--request-key-roll]`

**Edge Management API:**

```http request
POST /edge/management/v1/authenticators/:id/request-extend

{
    rollKeys: true,
}
```

These commands will cause the authenticator to be updated to have `isExtendRequested`, extendRequestedAt`, and
optionally `isKeyRollRequested` updated on the authenticator. These values will be used to signal the client to
extend their certificate on the next successful authentication attempt.

## Authentication Events

A new event names space `authentication` has been added for events with two types `success` and `fail`. These can be 
enabled via the controller configuration files.

Event Documentation:

```
// An AuthenticationEvent is emitted when an authentication attempt is made
//
// Types of authentication events
//   - fail - authentication failed
//   - success - authentication succeeded
//
// Types of authentication methods
//   - updb - username password from the internal database
//   - cert - a certificate, either first party or 3rd party
//   - ext-jwt - an external JWT from an IDP
```

Example Controller Configuration:

```yaml
events:
  jsonLogger:
    subscriptions:
      - type: authentication
#          - success
#          - fail
    handler:
      type: file
      format: json
      path: ${TMPDIR}/ziti-events.log
```

Example Output:
```json
{
  "namespace": "authentication",
  "event_src_id": "ctrl_client",
  "timestamp": "2025-05-21T10:24:32.6532054-04:00",
  "event_type": "fail",
  "type": "password",
  "authenticator_id": "B0JrMDPGxd",
  "external_jwt_signer_id": "",
  "identity_id": "BGJabPP0K",
  "auth_policy_id": "default",
  "remote_address": "127.0.0.1:51587",
  "success": false,
  "reason": "could not authenticate, password does not match",
  "improper_client_cert_chain": false
}
```

## Multi-underlay channel group secret

For additional security the experimental multi-underlay channel code now requires that 
clients provide a shared secret. This ensures that channels are get the expected 
underlays without requiring much larger group ids. On the client side this will require
the go sdk version to be v1.1.1 or greater. 

## Disabling posture check functionality

As posture check functionality can have performance impacts, it can now be disabled, for users
who don't need it.

This is controlled in the controller config file.

```
edge:
  # Set to true to disable posture check functionality
  disablePostureChecks: false
```

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.26 -> v1.0.27](https://github.com/openziti/agent/compare/v1.0.26...v1.0.27)
* github.com/openziti/channel/v4: [v4.0.6 -> v4.2.0](https://github.com/openziti/channel/compare/v4.0.6...v4.2.0)
    * [Issue #190](https://github.com/openziti/channel/issues/190) - Allow setting a context.Context for channel send timeouts/cancellation
    * [Issue #187](https://github.com/openziti/channel/issues/187) - Allow fallback to regular channel when 'is grouped' isn't set when using multi-listener
    * [Issue #185](https://github.com/openziti/channel/issues/185) - Add group secret for multi-underlay channels

* github.com/openziti/edge-api: [v0.26.43 -> v0.26.45](https://github.com/openziti/edge-api/compare/v0.26.43...v0.26.45)
* github.com/openziti/foundation/v2: [v2.0.59 -> v2.0.63](https://github.com/openziti/foundation/compare/v2.0.59...v2.0.63)
* github.com/openziti/runzmd: [v1.0.67 -> v1.0.72](https://github.com/openziti/runzmd/compare/v1.0.67...v1.0.72)
* github.com/openziti/sdk-golang: [v1.1.0 -> v1.1.1](https://github.com/openziti/sdk-golang/compare/v1.1.0...v1.1.1)
    * [Issue #735](https://github.com/openziti/sdk-golang/issues/735) - Ensure Authenticate can't be called in parallel

* github.com/openziti/secretstream: [v0.1.32 -> v0.1.34](https://github.com/openziti/secretstream/compare/v0.1.32...v0.1.34)
* github.com/openziti/storage: [v0.4.7 -> v0.4.11](https://github.com/openziti/storage/compare/v0.4.7...v0.4.11)
* github.com/openziti/transport/v2: [v2.0.168 -> v2.0.171](https://github.com/openziti/transport/compare/v2.0.168...v2.0.171)
* github.com/openziti/xweb/v2: [v2.3.1 -> v2.3.2](https://github.com/openziti/xweb/compare/v2.3.1...v2.3.2)
* github.com/openziti/ziti: [v1.6.1 -> v1.6.2](https://github.com/openziti/ziti/compare/v1.6.1...v1.6.2)
    * [Issue #3072](https://github.com/openziti/ziti/issues/3072) - router active channel map is using client supplied id, which can cause map collisions
    * [Issue #3068](https://github.com/openziti/ziti/issues/3068) - Auto CA enrollments do not dispatch events in HA
    * [Issue #3063](https://github.com/openziti/ziti/issues/3063) - Allow disabling posture check functionality
    * [Issue #3052](https://github.com/openziti/ziti/issues/3052) - Terminator Create events for addressable terminators
    * [Issue #3059](https://github.com/openziti/ziti/issues/3059) - Refresh JWTs can desync internal validation token
    * [Issue #3008](https://github.com/openziti/ziti/issues/3008) - UPDB Enroll w/ 1.5.1 `ziti` panics against 1.5.0 and lower controllers
    * [Issue #3048](https://github.com/openziti/ziti/issues/3048) - Add improper client cert chain flag
    * [Issue #2614](https://github.com/openziti/ziti/issues/2614) - Add Configuration for OIDC RefreshToken/Access Token


# Release 1.6.1

## What's New

* Bug fixes and library updates
* Ability to request that SDKs extend and optionally roll their key
* Address translations can now be specified in host.v1 service configuration

## Ability to request that SDKs extend and optionally roll their key

It is now possible for administrators to flag specific certificate authenticators as needed to `extend` their current
certificate early and/or optionally roll the keypair that underpins the certificate. This capability only works for
certificates issued by the OpenZiti network. If '3rd party CAs' are in use, those certificate authenticators will not
work with this system.

SDKs must support this capability for it to have any effect, and the application utilizing the SDK must respond to the 
certificate extension events to store certificate credentials.

This capability is located in the Management API at `/edge/management/v1/authenticators/{id>/request-extend`.
Its payload is currently and optional boolean value for `rollKeys` that can be set to true/false and defaults to
false if not provided. 

This can also be issued via the CLI:

```
> ziti edge update authenticator cert -h
Request a specific certificate authenticator to --requestExtend or --requestKeyRoll, --requestKeyRoll implies --requestExtend

Usage:
  ziti edge update authenticator cert <authenticatorId> [--requestExtend] [--requestKeyRoll] [flags]

Flags:
  -h, --help             help for cert
  -e, --requestExtend    Specify the certificate authenticator should be flagged for extension
  -r, --requestKeyRoll   Specify the certificate authenticator should be flagged for key rolling, implies --requestExtend
```

Requesting an extension flags new fields on a certificate authenticator in the values `isExtendRequest` and
`isKeyRollRequested`. These values are set to false after the client performs a certificate extension. The CLI
has been updated to report these values on certificate authenticators via `ziti edge list authenticators`.

These values are also present on the `/edge/client/v1/current-api-session` endpoint when a client has use certificate
authentication to initiate an API Session using a certificate authenticator.

Additionally, a log of key rolling activity per authenticator will be available in a future release.

## host.v1 Address Translation

The host.v1 service configuration type now includes a `forwardAddressTranslations` field that specifies
how a hosting tunneler should translate destination IPs from the client when connecting to the underlay
application.

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.42 -> v0.26.43](https://github.com/openziti/edge-api/compare/v0.26.42...v0.26.43)
* github.com/openziti/ziti: [v1.6.0 -> v1.6.1](https://github.com/openziti/ziti/compare/v1.6.0...v1.6.1)
  * [Issue #2996](https://github.com/openziti/ziti/issues/2996) - Add ability to signal SDKs to extend cert authenticator
  * [Issue #2963](https://github.com/openziti/ziti/issues/2963) - support intercept.v1 --> host.v1 address translation


# Release 1.6.0

## What's New

* Bug fixes and library updates
* Change to cluster add peer
* Experimental multi-underlay SDK support
* Experimental SDK flow-control support

## Cluster Add Peer Change

The `ziti agent cluster add` command no longer supports the `--id` argument 
for providing the peer id. The add operation will now always connect to the
peer, verify the certs and get the peer advertise address and id from the 
peer directly. This will ensure that the peer is reachable and valid before
it is added to the cluster. 

## Multi-underlay SDK support

For SDKs which support it, the edge router now supports a separate control channel
connection along side the data connection. If the SDK doesn't request separate
channels, the edge router will continue to work with a single connection. This 
feature is still experimental and may have bugs, may be changed or be removed.

## SDK Flow-control

For SDKs which support it, the edge router now supports delegating flow control
to the SDK. This feature is still experimental and may have bugs, may be changed 
or be removed.

## Component Updates and Bug Fixes

* github.com/openziti/channel/v4: [v3.0.39 -> v4.0.6](https://github.com/openziti/channel/compare/v3.0.39...v4.0.6)
    * [Issue #182](https://github.com/openziti/channel/issues/182) - MultiListener can deadlock
    * [Issue #180](https://github.com/openziti/channel/issues/180) - Add GetUserData to Channel interface
    * [Issue #176](https://github.com/openziti/channel/issues/176) - Multi-channel need a mechanism to notify the txer that the underlay has closed
    * [Issue #172](https://github.com/openziti/channel/issues/172) - Support multi-underlay channels

* github.com/openziti/identity: [v1.0.100 -> v1.0.101](https://github.com/openziti/identity/compare/v1.0.100...v1.0.101)
    * [Issue #64](https://github.com/openziti/identity/issues/64) - Support a way to check if a cert/serverCert can be saved

* github.com/openziti/metrics: [v1.3.0 -> v1.4.1](https://github.com/openziti/metrics/compare/v1.3.0...v1.4.1)
    * [Issue #53](https://github.com/openziti/metrics/issues/53) - Add reporter useful for emitting metrics to stdout

* github.com/openziti/sdk-golang: [v0.25.1 -> v1.1.0](https://github.com/openziti/sdk-golang/compare/v0.25.1...v1.1.0)
    * [Issue #702](https://github.com/openziti/sdk-golang/issues/702) - [Go SDK] Support xgress flow control from the SDK
    * [Issue #722](https://github.com/openziti/sdk-golang/issues/722) - Move xgress impl to SDK
    * [Issue #717](https://github.com/openziti/sdk-golang/issues/717) - ER connection race condition can leak connections
    * [Issue #689](https://github.com/openziti/sdk-golang/issues/689) - Concurrent map iteration and modification in getEdgeRouterConn causes panic
    * [Issue #701](https://github.com/openziti/sdk-golang/issues/701) - Support multi-underlay channels for edge router connections

* github.com/openziti/transport/v2: [v2.0.167 -> v2.0.168](https://github.com/openziti/transport/compare/v2.0.167...v2.0.168)
* github.com/openziti/xweb/v2: [v2.3.0 -> v2.3.1](https://github.com/openziti/xweb/compare/v2.3.0...v2.3.1)
* github.com/openziti/ziti: [v1.5.4 -> v1.6.0](https://github.com/openziti/ziti/compare/v1.5.4...v1.6.0)
    * [Issue #3005](https://github.com/openziti/ziti/issues/3005) - Always check that a controller is reachable and valid before adding it to an HA controller cluster
    * [Issue #2986](https://github.com/openziti/ziti/issues/2986) - [Router] Support xgress flow control from the SDK
    * [Issue #2999](https://github.com/openziti/ziti/issues/2999) - OIDC JWT backed sessions cannot verify extended certs
    * [Issue #2997](https://github.com/openziti/ziti/issues/2997) - Add Authenticator Id to OIDC JWTs/return for current-api-session
    * [Issue #2904](https://github.com/openziti/ziti/issues/2904) - Support client certificate authorities in TLS handshake
    * [Issue #2973](https://github.com/openziti/ziti/issues/2973) - CLI: add a subcommand to retrieve network JWT
    * [Issue #2984](https://github.com/openziti/ziti/issues/2984) - Extend enrollments does not return a full chain
    * [Issue #2930](https://github.com/openziti/ziti/issues/2930) - Support multi-underlay channels for the edge SDK
    * [Issue #2978](https://github.com/openziti/ziti/issues/2978) - Create loop4 sim for testing circuit contention and scale
    * [Issue #2981](https://github.com/openziti/ziti/issues/2981) - Remove PayloadBufferForwarder API from xgress retransmitter
    * [Issue #2906](https://github.com/openziti/ziti/issues/2906) - Controller not removed from DB controller store when removed from controller
    * [Issue #2922](https://github.com/openziti/ziti/issues/2922) - Validate node address before adding to cluster
    * [Issue #2932](https://github.com/openziti/ziti/issues/2932) - Fix router data model 'create public key' related errors
    * [Issue #2919](https://github.com/openziti/ziti/issues/2919) - Make xgress pluggable, so it can be used from the SDK
    * [Issue #2955](https://github.com/openziti/ziti/issues/2955) - Extract xgress inspection types
    * [Issue #2954](https://github.com/openziti/ziti/issues/2954) - Encapsulate xgress metrics
    * [Issue #2952](https://github.com/openziti/ziti/issues/2952) - Remove global payload ingester
    * [Issue #2951](https://github.com/openziti/ziti/issues/2951) - Remove global xgress retransmitter
    * [Issue #2950](https://github.com/openziti/ziti/issues/2950) - Move router specific xgress code to a new xgress_router package
    * [Issue #2920](https://github.com/openziti/ziti/issues/2920) - Make xgress acker configurable
