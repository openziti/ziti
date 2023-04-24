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

## Event Changes

### AMPQ Event Writer
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

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.58 -> v2.0.64](https://github.com/openziti/channel/compare/v2.0.58...v2.0.64)
    * [Issue #98](https://github.com/openziti/channel/issues/98) - Set default connect timeout to 5 seconds

* github.com/openziti/edge: [v0.24.239 -> v0.24.266](https://github.com/openziti/edge/compare/v0.24.239...v0.24.266)
    * [Issue #1471](https://github.com/openziti/edge/issues/1471) - UDP intercept connections report incorrect local/remote addresses, making confusing events
    * [Issue #629](https://github.com/openziti/edge/issues/629) - emit entity change events
    * [Issue #1295](https://github.com/openziti/edge/issues/1295) - Ensure DB migrations work properly in a clustered setup (edge)
    * [Issue #1418](https://github.com/openziti/edge/issues/1418) - Checks for session edge router availablility are inefficient

* github.com/openziti/edge-api: [v0.25.11 -> v0.25.18](https://github.com/openziti/edge-api/compare/v0.25.11...v0.25.18)
* github.com/openziti/fabric: [v0.22.87 -> v0.23.11](https://github.com/openziti/fabric/compare/v0.22.87...v0.23.11)
    * [Issue #706](https://github.com/openziti/fabric/issues/706) - Fix panic in link close 
    * [Issue #700](https://github.com/openziti/fabric/issues/700) - Additional Health Checks exposed on Edge Router
    * [Issue #595](https://github.com/openziti/fabric/issues/595) - Add include filtering for V3 usage metrics 
    * [Issue #684](https://github.com/openziti/fabric/issues/684) - Add tag annotations to circuit events, similar to usage events
    * [Issue #562](https://github.com/openziti/fabric/issues/562) - Add entity change events
    * [Issue #677](https://github.com/openziti/fabric/issues/677) - Rework raft startup
    * [Issue #582](https://github.com/openziti/fabric/issues/582) - Ensure DB migrations work properly in a clustered setup (fabric)
    * [Issue #668](https://github.com/openziti/fabric/issues/668) - Add network.Run watchdog, to warn if processing is delayed

* github.com/openziti/foundation/v2: [v2.0.21 -> v2.0.22](https://github.com/openziti/foundation/compare/v2.0.21...v2.0.22)
* github.com/openziti/identity: [v1.0.45 -> v1.0.48](https://github.com/openziti/identity/compare/v1.0.45...v1.0.48)
* github.com/openziti/runzmd: [v1.0.20 -> v1.0.21](https://github.com/openziti/runzmd/compare/v1.0.20...v1.0.21)
* github.com/openziti/sdk-golang: [v0.18.76 -> v0.20.20](https://github.com/openziti/sdk-golang/compare/v0.18.76...v0.20.20)
* github.com/openziti/storage: [v0.1.49 -> v0.2.2](https://github.com/openziti/storage/compare/v0.1.49...v0.2.2)
* github.com/openziti/transport/v2: [v2.0.72 -> v2.0.77](https://github.com/openziti/transport/compare/v2.0.72...v2.0.77)
* github.com/openziti/metrics: [v1.2.19 -> v1.2.21](https://github.com/openziti/metrics/compare/v1.2.19...v1.2.21)
* github.com/openziti/secretstream: v0.1.7 (new)
* github.com/openziti/ziti: [v0.27.9 -> v0.28.0](https://github.com/openziti/ziti/compare/v0.27.9...v0.28.0)
    * [Issue #1087](https://github.com/openziti/ziti/issues/1087) - re-enable CI in forks
    * [Issue #1013](https://github.com/openziti/ziti/issues/1013) - docker env password is renewed at each `docker-compose up`
    * [Issue #1077](https://github.com/openziti/ziti/issues/1077) - Show auth-policy name on identity list instead of id

