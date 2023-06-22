# Release 0.28.4

## Component Updates and Bug Fixes

* Restores Ziti Edge Client API as the default handler for `/version` and as the root handler to support previously enrolled GO SDK clients

# Release 0.28.3

## What's New

Bug fix

## Component Updates and Bug Fixes

* github.com/openziti/ziti: [v0.28.2 -> v0.28.3](https://github.com/openziti/ziti/compare/v0.28.2...v0.28.3)

# Release 0.28.2

## What's New

* IMPORTANT: If you update your OpenZiti binaries to this version or later (which can be done easily with the `getZiti()` function, you will need to migrate any existing network that has been developed using OpenZiti v0.27.5 or earlier binaries as the new binaries will expect the new environment variable names. A function `performMigration()` has been provided in the `ziti-cli-script.sh` for this purpose. Simply source the latest `ziti-cli-script.sh`, and your current network's .env file, then run `performMigration()` to update environment variable name references. If the migration process cannot find your existing environment file in the default location, you will need to provide the path to the migration function, ex: `performMigration <path-to-environment-file>`
* If you were using the `ZITI_HOME` environment variable to configure where your ziti CLI profiles were stored, you should now use `ZITI_CONFIG_DIR` instead.


## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.80 -> v2.0.81](https://github.com/openziti/channel/compare/v2.0.80...v2.0.81)
* github.com/openziti/edge: [v0.24.326 -> v0.24.345](https://github.com/openziti/edge/compare/v0.24.326...v0.24.345)
    * [Issue #1528](https://github.com/openziti/edge/issues/1528) - edge unbind returns incorect message if token is not suplied or invalid
    * [Issue #1416](https://github.com/openziti/edge/issues/1416) - Allow MFA token name to be configured

* github.com/openziti/edge-api: [v0.25.25 -> v0.25.29](https://github.com/openziti/edge-api/compare/v0.25.25...v0.25.29)
* github.com/openziti/fabric: [v0.23.35 -> v0.23.39](https://github.com/openziti/fabric/compare/v0.23.35...v0.23.39)
    * [Issue #751](https://github.com/openziti/fabric/issues/751) - Use of Fprintf causing buffer pool corruption with amqp event output

* github.com/openziti/foundation/v2: [v2.0.25 -> v2.0.26](https://github.com/openziti/foundation/compare/v2.0.25...v2.0.26)
* github.com/openziti/identity: [v1.0.56 -> v1.0.57](https://github.com/openziti/identity/compare/v1.0.56...v1.0.57)
* github.com/openziti/runzmd: [v1.0.25 -> v1.0.26](https://github.com/openziti/runzmd/compare/v1.0.25...v1.0.26)
* github.com/openziti/sdk-golang: [v0.20.58 -> v0.20.67](https://github.com/openziti/sdk-golang/compare/v0.20.58...v0.20.67)
* github.com/openziti/storage: [v0.2.7 -> v0.2.8](https://github.com/openziti/storage/compare/v0.2.7...v0.2.8)
* github.com/openziti/transport/v2: [v2.0.90 -> v2.0.91](https://github.com/openziti/transport/compare/v2.0.90...v2.0.91)
* github.com/openziti/metrics: [v1.2.26 -> v1.2.27](https://github.com/openziti/metrics/compare/v1.2.26...v1.2.27)
* github.com/openziti/secretstream: [v0.1.8 -> v0.1.9](https://github.com/openziti/secretstream/compare/v0.1.8...v0.1.9)
* github.com/openziti/ziti: [v0.28.1 -> v0.28.2](https://github.com/openziti/ziti/compare/v0.28.1...v0.28.2)
    * [Issue #1144](https://github.com/openziti/ziti/issues/1144) - DB explore subcommand panic
    * [Issue #986](https://github.com/openziti/ziti/issues/986) - Updated default ports in `.env` file to match documentation.
    * [Issue #920](https://github.com/openziti/ziti/issues/920) - Fixed bug causing failure when re-running quickstart.
    * [Issue #779](https://github.com/openziti/ziti/issues/779) - Add ability to upgrade ziti binaries using a quickstart function.
    * [Issue #761](https://github.com/openziti/ziti/issues/761) - Remove Management Listener section from controller config.
    * [Issue #650](https://github.com/openziti/ziti/issues/650) - Removed/Updated references to `ZITI_EDGE_CONTROLLER_API`
    * Quickstart environment variable names have been cleaned up.
    * [Issue #1030](https://github.com/openziti/ziti/issues/1030) - Provide an upgrade path for quickstart cleanup

# Release 0.28.1

## What's New

* `ziti` CLI now trims jwt files specified for login preventing a confusing invalid header field value for "Authorization"
  error when trying to use `-e` flag

## Router Health Check Changes

The link health check on routers now supports an initial delay configuration.

```

healthChecks:
  linkCheck:
    minLinks: 1
    interval: 30s
    initialDelay: 5s
```

The health check will also now start with an initial state of unhealthy, unless `minLinks` is set to zero.

Finally, link checks now include the addresses associated with the links:

```json
    {
        "details": [
            {
                "linkId": "6a72EtnLib5nUvjhVLuHOb",
                "destRouterId": "5uUxuQ3u6Q",
                "latency": 2732886.5,
                "addresses": {
                    "ack": {
                        "localAddr": "tcp:127.0.0.1:4023",
                        "remoteAddr": "tcp:127.0.0.1:33520"
                    },
                    "payload": {
                        "localAddr": "tcp:127.0.0.1:4023",
                        "remoteAddr": "tcp:127.0.0.1:33504"
                    }
                }
            }
        ],
        "healthy": true,
        "id": "link.health",
        "lastCheckDuration": "53.213µs",
        "lastCheckTime": "2023-06-01T18:35:11Z"
    }
```

## Event Changes

### AMQP Event Writer Changes
A new field is available to the AMQP Event Writer. `bufferSize` denotes how many messages ziti will hold during AMQP connection outages. Any messages exceeding this limit will be logged and dropped.

Example configuration:
```
events:
  jsonLogger:
    subscriptions:
      - type: fabric.circuits
    handler:
      type: amqp
      format: json
      url: "amqp://localhost:5672" 
      queue: ziti
      durable: true      //default:true
      autoDelete: false  //default:false
      exclusive: false   //default:false
      noWait: false      //default:false
      bufferSize: 50     //default:50
```
 
## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.13 -> v1.0.14](https://github.com/openziti/agent/compare/v1.0.13...v1.0.14)
* github.com/openziti/channel/v2: [v2.0.78 -> v2.0.80](https://github.com/openziti/channel/compare/v2.0.78...v2.0.80)
* github.com/openziti/edge: [v0.24.309 -> v0.24.326](https://github.com/openziti/edge/compare/v0.24.309...v0.24.326)
    * [Issue #1512](https://github.com/openziti/edge/issues/1512) - Panic when removing edge terminator with expired session
    * [Issue #1509](https://github.com/openziti/edge/issues/1509) - SDK hosted terminators are being removed twice, causing spurious errors
    * [Issue #1507](https://github.com/openziti/edge/issues/1507) - edge-router with encryption disabled fails
    * [Issue #1517](https://github.com/openziti/edge/issues/1517) - allow wildcard domains in intercept.v1 addresses

* github.com/openziti/edge-api: [v0.25.24 -> v0.25.25](https://github.com/openziti/edge-api/compare/v0.25.24...v0.25.25)
* github.com/openziti/fabric: [v0.23.29 -> v0.23.35](https://github.com/openziti/fabric/compare/v0.23.29...v0.23.35)
    * [Issue #538](https://github.com/openziti/fabric/issues/538) - Allow quiescing/dequiescing routers
    * [Issue #738](https://github.com/openziti/fabric/issues/738) - Timeout from routing is getting reported as conn refused instead of timeout
    * [Issue #737](https://github.com/openziti/fabric/issues/737) - Router link check should support initial delay configuration 
    * [Issue #735](https://github.com/openziti/fabric/issues/735) - router link health check should only be passing initially if min links is zero
    * [Issue #733](https://github.com/openziti/fabric/issues/733) - Show link addresses in health check
    * [Issue #732](https://github.com/openziti/fabric/issues/732) - Added new `bufferSize` config option to amqp handler. Connection handling now happens in the background with exponential retries.

* github.com/openziti/foundation/v2: [v2.0.24 -> v2.0.25](https://github.com/openziti/foundation/compare/v2.0.24...v2.0.25)
* github.com/openziti/identity: [v1.0.54 -> v1.0.56](https://github.com/openziti/identity/compare/v1.0.54...v1.0.56)
* github.com/openziti/runzmd: [v1.0.24 -> v1.0.25](https://github.com/openziti/runzmd/compare/v1.0.24...v1.0.25)
* github.com/openziti/sdk-golang: [v0.20.51 -> v0.20.58](https://github.com/openziti/sdk-golang/compare/v0.20.51...v0.20.58)
    * [Issue #409](https://github.com/openziti/sdk-golang/issues/409) - sdk-golang v0.20.49 loops forever with older 'ws://' edge router

* github.com/openziti/storage: [v0.2.6 -> v0.2.7](https://github.com/openziti/storage/compare/v0.2.6...v0.2.7)
* github.com/openziti/transport/v2: [v2.0.88 -> v2.0.90](https://github.com/openziti/transport/compare/v2.0.88...v2.0.90)
* github.com/openziti/metrics: [v1.2.25 -> v1.2.26](https://github.com/openziti/metrics/compare/v1.2.25...v1.2.26)
* github.com/openziti/ziti: [v0.28.0 -> v0.28.1](https://github.com/openziti/ziti/compare/v0.28.0...v0.28.1)
  * [Issue #1132](https://github.com/openziti/ziti/issues/1132) - Updated `ws` protocol to `wss` as `ws` is no longer supported.
 
# Release 0.28.0

## What's New

* Event changes
  * Added AMQP event writter for events
  * Add entity change events for auditing or external integration
  * Add usage event filtering
  * Add annotations to circuit events
* CLI additions for `ziti` to login with certificates or external-jwt-signers
* NOTE: ziti edge login flag changes:
  * `-c` flag has been changed to map to `--client-cert`
  * `--cert` is now `--ca` and has no short flag representation
  * `-e/--ext-jwt` allows a user to supply a file containing a jwt used with ext-jwt-signers to login
  * `-c/--client-cert` allows a certificate to be supplied to login (used with `-k/--client-key`)
  * `-k/--client-key` allows a key to be supplied to login (used with `-c/--client-cert`)
* Config type changes
  * address fields in `intercept.v1`, `host.v1`, and `host.v2` config types now permit hostnames with underscores.
* Edge Router/Tunneler now supports setting default UDP idle timeout/check interval

## Event Changes

### AMQP Event Writer
Previously events could only be emitted to a file. They can now also be emitted to an AMQP endpoint. 

Example configuration:
```
events:
  jsonLogger:
    subscriptions:
      - type: fabric.circuits
    handler:
      type: amqp
      format: json
      url: "amqp://localhost:5672" 
      queue: ziti
      durable: true      //default:true
      autoDelete: false  //default:false
      exclusive: false   //default:false
      noWait: false      //default:false
```

### Entity Change Events
OpenZiti can now be configured to emit entity change events. These events describe the changes when entities stored in the 
bbolt database are created, updated or deleted.

Note that events are emitted during the transaction. They are emitted at the end, so it's unlikely, but possible that an event will be emitted for a change which is rolled back. For this reason a following event will emitted when the change is committed. If a system crashes after commit, but before the committed event can be emitted, it will be emitted on the next startup.

Example configuration:

```
events:
  jsonLogger:
    subscriptions:
      - type: entityChange
        include:
          - services
          - identities
    handler:
      type: file
      format: json
      path: /tmp/ziti-events.log
```

See the related issue for discussion: https://github.com/openziti/fabric/issues/562

Example output:

```
{
  "namespace": "entityChange",
  "eventId": "326faf6c-8123-42ae-9ed8-6fd9560eb567",
  "eventType": "created",
  "timestamp": "2023-05-11T21:41:47.128588927-04:00",
  "metadata": {
    "author": {
      "type": "identity",
      "id": "ji2Rt8KJ4",
      "name": "Default Admin"
    },
    "source": {
      "type": "rest",
      "auth": "edge",
      "localAddr": "localhost:1280",
      "remoteAddr": "127.0.0.1:37578",
      "method": "POST"
    },
    "version": "v0.0.0"
  },
  "entityType": "services",
  "isParentEvent": false,
  "initialState": null,
  "finalState": {
    "id": "6S0bCGWb6yrAutXwSQaLiv",
    "createdAt": "2023-05-12T01:41:47.128138887Z",
    "updatedAt": "2023-05-12T01:41:47.128138887Z",
    "tags": {},
    "isSystem": false,
    "name": "test",
    "terminatorStrategy": "smartrouting",
    "roleAttributes": [
      "goodbye",
      "hello"
    ],
    "configs": null,
    "encryptionRequired": true
  }
}

{
  "namespace": "entityChange",
  "eventId": "326faf6c-8123-42ae-9ed8-6fd9560eb567",
  "eventType": "committed",
  "timestamp": "2023-05-11T21:41:47.129235443-04:00"
}
```

### Usage Event Filtering
Usage events, version 3, can now be filtered based on type. 

The valid types include:

* ingress.rx
* ingress.tx
* egress.rx
* egress.tx
* fabric.rx
* fabric.tx

Example configuration:

```
events:
  jsonLogger:
    subscriptions:
      - type: fabric.usage
        version: 3
        include:
          - ingress.rx
          - egress.rx
```

### Circuit Event Annotations
Circuit events initiated from the edge are now annotated with clientId, hostId and serviceId, to match usage events. The client and host ids are identity ids. 

Example output:

```
 {
  "namespace": "fabric.circuits",
  "version": 2,
  "event_type": "created",
  "circuit_id": "0CEjWYiw6",
  "timestamp": "2023-05-05T11:44:03.242399585-04:00",
  "client_id": "clhaq7u7600o4ucgdpxy9i4t1",
  "service_id": "QARLLTKjqfLZytmSsIqba",
  "terminator_id": "7ddcd421-2b00-4b49-9ac0-8c78fe388c30",
  "instance_id": "",
  "creation_timespan": 1014280,
  "path": {
    "nodes": [
      "U7OwPtfjg",
      "a4rC9DrZ3"
    ],
    "links": [
      "7Ru3hoxsssZzUNOyvd8Jcb"
    ],
    "ingress_id": "K9lD",
    "egress_id": "rQLK",
    "initiator_local_addr": "100.64.0.1:1234",
    "initiator_remote_addr": "100.64.0.1:37640",
    "terminator_local_addr": "127.0.0.1:45566",
    "terminator_remote_addr": "127.0.0.1:1234"
  },
  "link_count": 1,
  "path_cost": 392151,
  "tags": {
    "clientId": "U7OwPtfjg",
    "hostId": "a4rC9DrZ3",
    "serviceId": "QARLLTKjqfLZytmSsIqba"
  }
}
```

## ER/T UDP Settings

The edge router tunneler now allows configuring a timeout and check interval for tproxy UDP intercepts. By default intercepted UDP 
connections will be closed after five minutes of no traffic, checking every thirty seconds. The configuration is done in the router 
config file, in the options for the tunnel module. Note that these configuration options only apply to tproxy intercepts, not to
proxy or host side UDP connections.

Example configuration:

```yaml
listeners:
  - binding: tunnel
    options:
      mode: tproxy
      udpIdleTimeout: 10s
      udpCheckInterval: 5s
```

## Component Updates and Bug Fixes
* github.com/openziti/agent: [v1.0.10 -> v1.0.13](https://github.com/openziti/agent/compare/v1.0.10...v1.0.13)
* github.com/openziti/channel/v2: [v2.0.58 -> v2.0.78](https://github.com/openziti/channel/compare/v2.0.58...v2.0.78)
    * [Issue #98](https://github.com/openziti/channel/issues/98) - Set default connect timeout to 5 seconds

* github.com/openziti/edge: [v0.24.239 -> v0.24.309](https://github.com/openziti/edge/compare/v0.24.239...v0.24.309)
    * [Issue #1503](https://github.com/openziti/edge/issues/1503) - Support configurable UDP idle timeout and check interval for tproxy in edge router tunneler
    * [Issue #1471](https://github.com/openziti/edge/issues/1471) - UDP intercept connections report incorrect local/remote addresses, making confusing events
    * [Issue #629](https://github.com/openziti/edge/issues/629) - emit entity change events
    * [Issue #1295](https://github.com/openziti/edge/issues/1295) - Ensure DB migrations work properly in a clustered setup (edge)
    * [Issue #1418](https://github.com/openziti/edge/issues/1418) - Checks for session edge router availablility are inefficient

* github.com/openziti/edge-api: [v0.25.11 -> v0.25.24](https://github.com/openziti/edge-api/compare/v0.25.11...v0.25.24)
* github.com/openziti/fabric: [v0.22.87 -> v0.23.29](https://github.com/openziti/fabric/compare/v0.22.87...v0.23.29)
    * [Issue #724](https://github.com/openziti/fabric/issues/724) - Controller should be notified of forwarding faults on links 
    * [Issue #725](https://github.com/openziti/fabric/issues/725) - If reroute fails, circuit should be torn down
    * [Issue #706](https://github.com/openziti/fabric/issues/706) - Fix panic in link close 
    * [Issue #700](https://github.com/openziti/fabric/issues/700) - Additional Health Checks exposed on Edge Router
    * [Issue #595](https://github.com/openziti/fabric/issues/595) - Add include filtering for V3 usage metrics 
    * [Issue #684](https://github.com/openziti/fabric/issues/684) - Add tag annotations to circuit events, similar to usage events
    * [Issue #562](https://github.com/openziti/fabric/issues/562) - Add entity change events
    * [Issue #677](https://github.com/openziti/fabric/issues/677) - Rework raft startup
    * [Issue #582](https://github.com/openziti/fabric/issues/582) - Ensure DB migrations work properly in a clustered setup (fabric)
    * [Issue #668](https://github.com/openziti/fabric/issues/668) - Add network.Run watchdog, to warn if processing is delayed

* github.com/openziti/foundation/v2: [v2.0.21 -> v2.0.24](https://github.com/openziti/foundation/compare/v2.0.21...v2.0.24)
* github.com/openziti/identity: [v1.0.45 -> v1.0.54](https://github.com/openziti/identity/compare/v1.0.45...v1.0.54)
* github.com/openziti/runzmd: [v1.0.20 -> v1.0.24](https://github.com/openziti/runzmd/compare/v1.0.20...v1.0.24)
* github.com/openziti/sdk-golang: [v0.18.76 -> v0.20.51](https://github.com/openziti/sdk-golang/compare/v0.18.76...v0.20.51)
    * [Issue #407](https://github.com/openziti/sdk-golang/issues/407) - Allowing filtering which edge router urls the sdk uses 
    * [Issue #394](https://github.com/openziti/sdk-golang/issues/394) - SDK does not recover from API session expiration (during app/computer suspend)

* github.com/openziti/storage: [v0.1.49 -> v0.2.6](https://github.com/openziti/storage/compare/v0.1.49...v0.2.6)
* github.com/openziti/transport/v2: [v2.0.72 -> v2.0.88](https://github.com/openziti/transport/compare/v2.0.72...v2.0.88)
* github.com/openziti/metrics: [v1.2.19 -> v1.2.25](https://github.com/openziti/metrics/compare/v1.2.19...v1.2.25)
* github.com/openziti/secretstream: v0.1.8 (new)
* github.com/openziti/ziti: [v0.27.9 -> v0.28.0](https://github.com/openziti/ziti/compare/v0.27.9...v0.28.0)
    * [Issue #1112](https://github.com/openziti/ziti/issues/1112) - `ziti pki create` creates CA's and intermediates w/ the same DN
    * [Issue #1087](https://github.com/openziti/ziti/issues/1087) - re-enable CI in forks
    * [Issue #1013](https://github.com/openziti/ziti/issues/1013) - docker env password is renewed at each `docker-compose up`
    * [Issue #1077](https://github.com/openziti/ziti/issues/1077) - Show auth-policy name on identity list instead of id
    * [Issue #1119](https://github.com/openziti/ziti/issues/1119) - intercept.v1 config should permit underscores in the address
    * [Issue #1123](https://github.com/openziti/ziti/issues/1123) - cannot update config types with ziti cli

