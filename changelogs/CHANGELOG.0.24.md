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

- This process may take days, weeks, or months depending on the size of the network and how often the router/clients are
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
