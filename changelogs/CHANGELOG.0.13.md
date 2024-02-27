# Release 0.13.9

## Theme

Ziti 0.13.9 includes the following:

* Adds paging information to cli commands

Example

 ```shell script
$ ec list api-sessions "true sort by token skip 2 limit 3" 
id: 37dd1463-e4e7-40de-9a63-f75486430361    token: 0b392a2f-47f8-4561-af63-93807ce70d93    identity: Default Admin
id: 6fb5b488-debf-4212-9670-f250e31b3d4f    token: 15ae6b00-f123-458c-a121-5cf91983a2c2    identity: Default Admin
id: 8aa4a074-b2c7-4d55-9f56-17199ab6ac11    token: 1b9418d8-b9a7-4e39-a876-7a9588f5e7ed    identity: Default Admin
results: 3-5 of 23
```

# Release 0.13.8

## Theme

Ziti 0.13.8 includes the following:

* Fixes Ziti Edge Router ignoring connect options for SDK listener

# Release 0.13.7

## Theme

Ziti 0.13.7 includes the following:

* Improvements to sdk availability when hosting services
* Various bug fixes to related to terminators and transit routers

## SDK Resilience

The golang sdk now has a new listen method on context, which takes listen options.

```
type Context interface {
    ...
    ListenWithOptions(serviceName string, options *edge.ListenOptions) (net.Listener, error)
    ...
}

type ListenOptions struct {
    Cost           uint16
    ConnectTimeout time.Duration
    MaxConnections int
}
```

The SDK now supports the following:

* Configuring connect timeout
* Allow establishing new session, if existing session goes away
* Allow establishing new API session, existing API session goes away
* If client doesn't have access to service, it should stop listening and return an error
* If client can't establish or re-establish API session, it should stop listening and return error

If paired with a ziti controller/routers which support terminator strategies for HA/HS, the
following features are also supported:

* Handle listen to multiple edge routers.
* Allow configuring max number of connections to edge routers

# Release 0.13.6

## Theme

* Fixes the `-n` flag being ignored for `ziti-enroll`

# Release 0.13.5

## Theme

* Adds ability to verify 3rd party CAs via the CLI in the Ziti Edge API

## Ziti CLI Verify CA Support

Previous to this version the CLI was only capable of creating, editing, and deleting CAs. For a CA
to be useful it must be verified. If not, it cannot be used for enrollment or authentication. The
verification process requires HTTP requests and the creation of a signed verification certificate.
The Ziti CLI can now perform all or part of this process.

### Example: No Existing Verification Cert

This example is useful for situations where access to the CA's private key is possible. This command
will fetch the CA's verification token from the Ziti Edge API, create a short lived (5 min)
verification certificate, and use it to verify the CA.

This example includes the `--password` flag which is optional. If the
`--password` flag is not included and the private key is encrypted the user will be prompted for the
password.

- `myCa` is the name or id of a CA that has already been created.
- `ca.cert.pem` the CA's public x509 PEM formatted certificate
- `ca.key.pem` the CA's private x509 PEM formatted key

```
$ ziti edge controller verify ca myCa --cacert ca.cert.pem --cakey ca.key.pem --password 1234
```

### Example: Existing Verification Certificate

This example is useful for situations where access to the signing CA's private key is not possible (
off-site, coldstore, etc.). This example assumes that the appropriate `openssl` commands have been
run to generate the verification script.

- `myCa` is the name or id of a CA that has already been created.
- `verificationCert.pem` is a PEM encoded x509 certificate that has the common name set to the
  verification token of `myCa`

```
$ ziti edge controller verify ca myCa --cert verificationCert.pem
```

### Command help:

```
$ ziti edge controller verify ca --help

Usage:
  ziti edge controller verify ca <name> ( --cert <pemCertFile> | --cacert
  <signingCaCert> --cakey <signingCaKey> [--password <caKeyPassword>]) [flags]

Flags:
  -a, --cacert string     The path to the CA cert that should be used togenerate and sign a verification cert
  -k, --cakey string      The path to the CA key that should be used to generate and sign a verification cert
  -c, --cert string       The path to a cert with the CN set as the verification token and signed by the target CA
  -h, --help              help for ca
  -j, --output-json       Output the full JSON response from the Ziti Edge Controller
  -p, --password string   The password for the CA key if necessary
```

# Release 0.13.4

## Theme

* Updates `quickstart` scripts

# Release 0.13.3

## Theme

Ziti 0.13.3 includes the following:

* Adds connect parameters for incoming channel2 connections (control, management, and SDK
  connections)
    * The options have internal defaults are needed only when connections

## Connection Parameters

A new set of options have been introduced for channel2 backed listeners. Channel2 is a library used
to establish message based connections between a channel2 client and server. Most importantly this
is used for control and management connections in the `ziti-controller` and for the SDK connections
accepted in `ziti-router`. Setting these values to invalid values will result in errors during
startup of the `ziti-controller` and `ziti-router`

* `maxQueuedConnects` - set the maximum number of connect requests that are buffered and waiting to
  be acknowledged (1 to 5000, default 1000)
* `maxOutstandingConnects` - the maximum number of connects that have begun hello synchronization (1
  to 1000, default 16)
* `connectTimeoutMs` - the number of milliseconds to wait before a hello synchronization fails and
  closes the connection (30ms to 60000ms, default: 1000ms)

Example: `ziti-controller` configuration file:

```
# the endpoint that routers will connect to the controller over.
ctrl:
  listener:             tls:127.0.0.1:6262
  options:
    maxQueuedConnects:      50
    maxOutstandingConnects: 100
    connectTimeoutMs:       3000

# the endpoint that management tools connect to the controller over.
mgmt:
  listener:             tls:127.0.0.1:10000
  options:
    maxQueuedConnects:      50
    maxOutstandingConnects: 100
    connectTimeoutMs:       3000
```

Example: `ziti-router` configuration file:

```
listeners:
  - binding: edge
    address: tls:0.0.0.0:3022
    options:
      # (required) The public hostname and port combination that Ziti SDKs should connect on. Previously this was in the chanIngress section.
      advertise: 127.0.0.1:3022
      maxQueuedConnects:      50
      maxOutstandingConnects: 100
      connectTimeoutMs:       3000
```

# Release 0.13

## Theme

Ziti 0.13 includes the following:

* Changes to make working with policies easier, including
    * New APIs to list existing role attributes used by edge routers, identities and services
    * New APIs to list entities related by polices (such as listing edge routers available to a
      service via service edge router policies)
    * Enhancements to the LIST APIs for edge routers, identities and services which allow one to
      filter by roles
    * A policy advisor API, which helps analyze policies and current system state to figure out if
      an identity should be able to use a service and if not, why not
* CA Auto Enrollment now allows identities to inherit role attributes from the validating CA
    * New `identityRole` attributes added to CA entities
* New APIs to list and manage Transit Routers
* Transit Routers now support enrolment via `ziti-router enroll`
* Embedded Swagger/OpenAPI 2.0 endpoint
* A small set of APIs accepted id or name. These have been changed to accept only id
* Fabric enhancements
    * New Xlink framework encapsulating the router capabilities for creating overlay mesh links.
    * Adjustable Xgress MTU size.
* All Ziti Go projects are now being built with Go 1.14
    * See here for change to Go in 1.14 - https://golang.org/doc/go1.14

## Making Policies More User Friendly

### Listing Role Attributes in Use

There are three new endpoints for listing role attributes in use.

    * Endpoint: /edge-router-role-attributes
    * Endpoint: /identity-role-attributes
    * Endpoint: /service-role-attributes

All three support the same operations:

    * Supported operations
        * List: GET
            * Supports filtering
            * role attributes can be filtered/sorted using the symbol `id`
            * Ex:`?filter=id contains "north" limit 5`

The CLI supports these new operations as well.

    ziti edge controller list edge-router-role-attributes
    ziti edge controller list identity-role-attributes
    ziti edge controller list service-role-attributes

Example output:

    $ ec list service-role-attributes "true sort by id desc limit 5" -j
    {
        "meta": {
            "filterableFields": [
                "id"
            ],
            "pagination": {
                "limit": 5,
                "offset": 0,
                "totalCount": 10
            }
        },
        "data": [
            "two",
            "three",
            "support",
            "sales",
            "one"
        ]
    }

## Listing Entities Related by Policies

This adds operations to the `/services`, `/identities` and `/edge-routers` endpoints.

    * Endpoint: /edge-routers
    * New operations
       * Query related identities: GET /edge-routers/<edge-router-id>/identities?filter=<optional-filter>
       * Query related services: GET /edge-routers/<edge-router-id>/services?filter=<optional-filter>

    * Endpoint: /identities
    * New operations
       * Query related edge routers: GET /identities/<identity-id>/edge-routers?filter=<optional-filter>
       * Query related services: GET /identities/<identity-id>/services?filter=<optional-filter>

    * Endpoint: /services
    * New operations
       * Query related identities: GET /services/<service-id>/identities?filter=<optional-filter>
       * Query related edge routers: GET /services/<service-id>/edge-routers?filter=<optional-filter>

## Filtering Entity Lists by Roles

When building UIs it may be useful to list entities which have role attributes by role filters, to
see what policy changes may look like.

     * Endpoint: /edge-routers
     * Query: GET /edge-routers now supports two new query parameters
         * roleFilter. May be specified more than one. Should be an id or role specifier (ex: @38683097-2412-4335-b056-ae8747314dd3 or #sales)
         * roleSemantic. Optional. Defaults to AllOf if not specified. Indicates which semantic to use when evaluating role matches
 
     * Endpoint: /identities
     * Query: GET /identities now supports two new query parameters
         * roleFilter. May be specified more than one. Should be an id or role specifier (ex: @38683097-2412-4335-b056-ae8747314dd3 or #sales)
         * roleSemantic. Optional. Defaults to AllOf if not specified. Indicates which semantic to use when evaluating role matches
 
     * Endpoint: /services
     * Query: GET /services now supports two new query parameters
         * roleFilter. May be specified more than one. Should be an id or role specifier (ex: @38683097-2412-4335-b056-ae8747314dd3 or #sales)
         * roleSemantic. Optional. Defaults to AllOf if not specified. Indicates which semantic to use when evaluating role matches

Note that a roleFilter should have one role specifier (like `@some-id` or `#sales`). If you wish to
specify multiple, provide multiple role filters.

    /edge-routers?roleFilter=#sales&roleFilter=#us

These are also supported from the CLI when listing edge routers, identities and services using the
--role-filters and --role-semantic flags.

Example:

    $ ec list services
    id: 2a724ae7-8b8f-4688-90df-34951bce6720    name: grpc-ping    terminator strategy:     role attributes: ["fortio","fortio-server"]
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: 9480e39d-0664-4482-b230-5da2c17b225b    name: iperf    terminator strategy:     role attributes: {}
    id: a949cf80-b11b-4cce-bbb7-d2e4767878a6    name: baz    terminator strategy:     role attributes: ["development","sales","support"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]
    
    $ ec list services --role-filters '#three'
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]

    $ ec list services --role-filters '#three','#two'
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]
    
    $ ec list services --role-filters '#three','#sales' --role-semantic AnyOf
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: a949cf80-b11b-4cce-bbb7-d2e4767878a6    name: baz    terminator strategy:     role attributes: ["development","sales","support"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]
    
    $ ec list services --role-filters '#three''#sales','@4e33859b-070d-42b1-8b40-4adf973f680c' --role-semantic AnyOf
    id: 38683097-2412-4335-b056-ae8747314dd3    name: quux    terminator strategy:     role attributes: ["blop","sales","three"]
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: a949cf80-b11b-4cce-bbb7-d2e4767878a6    name: baz    terminator strategy:     role attributes: ["development","sales","support"]
    id: ad95ec7d-6c05-42b6-b278-2a98a7e502df    name: bar    terminator strategy:     role attributes: ["four","three","two"]
    id: e9673c77-7463-4517-a642-641ef35855cf    name: foo    terminator strategy:     role attributes: ["one","three","two"]

## Policy Advisor

This adds a new operation to the /identities endpoint

    * Endpoint: /identities
    * New operations
       * Query related identities: GET /identities/<identity-id>/policy-advice/<service-id>

This will return the following information about the identity and service:

* If the identity can dial the service
* If the identity can bind the service
* How many edge routers the identity has access to
* How many edge routers the service can be accessed through
* Which edge routers the identity and service have in common (if this is none, then the service
  can't be accessed by the identity)
* Which of the common edge routers are on-line

Example result:

    {
        "meta": {},
        "data": {
            "identityId": "700347c8-ca3a-4438-9060-68f7255ee4f8",
            "identity": {
                "entity": "identities",
                "id": "700347c8-ca3a-4438-9060-68f7255ee4f8",
                "name": "ssh-host",
                "_links": {
                    "self": {
                        "href": "./identities/700347c8-ca3a-4438-9060-68f7255ee4f8"
                    }
                }
            },
            "serviceId": "8fa27a3e-ffb1-4bd1-befa-fcd38a6c26b3",
            "service": {
                "entity": "services",
                "id": "8fa27a3e-ffb1-4bd1-befa-fcd38a6c26b3",
                "name": "ssh",
                "_links": {
                    "self": {
                        "href": "./services/8fa27a3e-ffb1-4bd1-befa-fcd38a6c26b3"
                    }
                }
            },
            "isBindAllowed": true,
            "isDialAllowed": false,
            "identityRouterCount": 2,
            "serviceRouterCount": 2,
            "commonRouters": [
                {
                    "entity": "edge-routers",
                    "id": "43d220d8-860e-4d80-a25c-97322a7326b4",
                    "name": "us-west-1",
                    "_links": {
                        "self": {
                            "href": "./edge-routers/43d220d8-860e-4d80-a25c-97322a7326b4"
                        }
                    },
                    "isOnline": false
                },
                {
                    "entity": "edge-routers",
                    "id": "8c118857-c12e-430d-9109-c31f535933f6",
                    "name": "us-east-1",
                    "_links": {
                        "self": {
                            "href": "./edge-routers/8c118857-c12e-430d-9109-c31f535933f6"
                        }
                    },
                    "isOnline": true
                }
            ]
        }
    }

The CLI has also been updated with a new policy-advisor common.

Examples:

    # Inspect all identities for policy issues
    ziti edge controller policy-advisor identities

    # Inspect just the jsmith-laptop identity for policy issues with all services that the identity can access
    ziti edge controller policy-advisor identities jsmith-laptop

    # Inspect the jsmith-laptop identity for issues related to the ssh service
    ziti edge controller policy-advisor identities jsmith-laptop ssh

    # Inspect all services for policy issues
    ziti edge controller policy-advisor services

    # Inspect just the ssh service for policy issues for all identities the service can access
    ziti edge controller policy-advisor services ssh

    # Inspect the ssh service for issues related to the jsmith-laptop identity 
    ziti edge controller policy-advisor identities ssh jsmith-laptop

Some example output of the CLI:

    $ ec policy-advisor identities -q
    ERROR: mlapin-laptop (1) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    ERROR: mlapin-laptop (1) -> ssh (2) Common Routers: (0/0) Dial: Y Bind: N 
      - Identity and services have no edge routers in common. Adjust edge router policies and/or service edge router policies.
    
    ERROR: ndaniels-laptop (1) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    ERROR: ndaniels-laptop (1) -> ssh (2) Common Routers: (0/1) Dial: Y Bind: N 
      - Common edge routers are all off-line. Bring routers back on-line or adjust edge router policies and/or service edge router policies to increase common router pool.
    
    ERROR: Default Admin 
      - Identity does not have access to any services. Adjust service policies.
    
    ERROR: jsmith-laptop (2) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    OKAY : jsmith-laptop (2) -> ssh (2) Common Routers: (1/2) Dial: Y Bind: N 
    
    ERROR: ssh-host (2) -> ssh-backup (0) Common Routers: (0/0) Dial: N Bind: Y 
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    OKAY : ssh-host (2) -> ssh (2) Common Routers: (1/2) Dial: N Bind: Y 
    
    ERROR: aortega-laptop 
      - Identity does not have access to any services. Adjust service policies.
    
    ERROR: djones-laptop (0) -> ssh-backup (0) Common Routers: (0/0) Dial: Y Bind: N 
      - Identity has no edge routers assigned. Adjust edge router policies.
      - Service has no edge routers assigned. Adjust service edge router policies.
    
    ERROR: djones-laptop (0) -> ssh (2) Common Routers: (0/0) Dial: Y Bind: N 
      - Identity has no edge routers assigned. Adjust edge router policies.

    $ ec policy-advisor identities aortega-laptop ssh-backup -q
    Found identities with id 70567104-d4bd-45f1-8179-bd1e6ab8751f for name aortega-laptop
    Found services with id 46e94977-0efc-4e7d-b9ae-cc8df1c95fc1 for name ssh-backup
    ERROR: aortega-laptop (0) -> ssh-backup (0) Common Routers: (0/0) Dial: N Bind: N 
      - No access to service. Adjust service policies.
      - Identity has no edge routers assigned. Adjust edge router policies.
      - Service has no edge routers assigned. Adjust service edge router policies.

## CA Auto Enrollment Identity Attributes

Identities that are enrolled via a CA can now inherit a static list of identity role attributes. The
normal create, update, patch requests supported by the CA entities now allow the role attributes to
be specified. CA identity role attribute changes do no propagate to identities that have completed
enrollment.

This feature allows a simple degree of automation for identities that are auto-provisioning through
a third party CA.

* `identityRoles` added to `/ca` endpoints for normal CRUD operations
* `identityRoles` from a CA entity are point-in-time copies

## New APIs to list and manage Transit Routers

The endpoint`/transit-routers` has been added to create and manage Transit Routers. Transit Routers
do not handle incoming Ziti Edge SDK connections.

    * Endpoint: /transit-routers
    * Supported operations
        * Detail: GET /transit-routers/<transit-router-id>
        * List: GET /transit-routers/
        * Create: POST /transit-routers
        * Update All Fields: PUT /transit-routers/<transit-router-id>
        * Update Selective Fields: PATCH /transit-routers/<transit-router-id>
        * Delete: DELETE /transit-routers/<transit-router-id>
     * Properties
         * Transit Routers support the standard properties (id, createdAt, updatedAt, tags)
         * name - Type string - a friendly Edge name for the transit router
         * fingerprint - Type string - a hex string fingerprint of the transit router's public certificate (post enrollment)
         * isVerified - Type bool - true if the router has completed enrollment
         * isOnline - Type bool - true if the router is currently connected to the controller
         * enrollmentToken - Type string - the enrollment token that would be used during enrollment (nil post enrollment)
         * enrollmentJwt - Type string - an enrollment JWT suitable for use with "ziti-router enroll" (nil post enrollment)
         * enrollmentCreatedAt - Type date-time - the date and time the enrollment was created (nil post enrollment)
         * enrollmentExpiresAt - Type date-time - the date and time the enrollment expires at (matches JWT expiration time, nil post enrollment)

Example list output:

```json
{
  "meta": {
    "filterableFields": [
      "id",
      "createdAt",
      "updatedAt",
      "name"
    ],
    "pagination": {
      "limit": 10,
      "offset": 0,
      "totalCount": 2
    }
  },
  "data": [
    {
      "id": "002",
      "createdAt": "2020-03-30T00:55:38.1701084Z",
      "updatedAt": "2020-03-30T00:55:38.1701084Z",
      "_links": {
        "self": {
          "href": "./transit-routers/002"
        }
      },
      "tags": {},
      "name": "",
      "fingerprint": "07e011481921b4734df82c52ae2b3113617cdd18",
      "isVerified": true,
      "isOnline": false,
      "enrollmentToken": null,
      "enrollmentJwt": null,
      "enrollmentCreatedAt": null,
      "enrollmentExpiresAt": null
    },
    {
      "id": "99f4109b-cd6d-40e3-9a62-bee24d7eccd6",
      "createdAt": "2020-03-30T17:48:17.2949059Z",
      "updatedAt": "2020-03-30T17:48:17.2949059Z",
      "_links": {
        "self": {
          "href": "./transit-routers/99f4109b-cd6d-40e3-9a62-bee24d7eccd6"
        }
      },
      "tags": {},
      "name": "",
      "fingerprint": "25d1048f3c7bc4a5956ce7316e2ca70999c0e27d",
      "isVerified": true,
      "isOnline": false,
      "enrollmentToken": null,
      "enrollmentJwt": null,
      "enrollmentCreatedAt": null,
      "enrollmentExpiresAt": null
    }
  ]
}
```

## Transit Routers now support enrolment via `ziti-router enroll`

Transit Routers now enroll using the same command: `ziti-router enroll <config> -j <jwt>`. During
the enrollment process, the CSR properties used will be taken from `edge.csr`. If `edge.csr` does
not exist `csr` will be utilized. If both are missing an error will occur.

Example router configuration:

```yaml
v: 3

identity:
  cert: etc/ca/intermediate/certs/001-client.cert.pem
  server_cert: etc/ca/intermediate/certs/001-server.cert.pem
  key: etc/ca/intermediate/private/001.key.pem
  ca: etc/ca/intermediate/certs/ca-chain.cert.pem

# Configure the forwarder options
#
forwarder:
  # How frequently does the forwarder probe the link latency. This will ultimately determine the resolution of the
  # responsiveness available to smart routing. This resolution comes at the expense of bandwidth utilization for the
  # probes, control plane utilization, and CPU utilization processing the results.
  #
  latencyProbeInterval: 1000

# Optional CSR section for transit router enrollment via `ziti-router enroll <config> -j <jwt>`
csr:
  country: US
  province: NC
  locality: Charlotte
  organization: NetFoundry
  organizationalUnit: Ziti
  sans:
    dns:
      - "localhost"
      - "test-network"
      - "test-network.localhost"
      - "ziti-dev-ingress01"
    email:
      - "admin@example.com"
    ip:
      - "127.0.0.1"
    uri:
      - "ziti://ziti-dev-router01/made/up/example"


#trace:
#  path:                 001.trace

#profile:
#  memory:
#    path:               001.memprof
#  cpu:
#    path:               001.cpuprof

ctrl:
  endpoint: tls:127.0.0.1:6262

link:
  dialers:
    - binding: transport

listeners:
  # basic ssh proxy
  - binding: proxy
    address: tcp:0.0.0.0:1122
    service: ssh
    options:
      mtu: 768

  # for iperf_tcp (iperf3)
  - binding: proxy
    address: tcp:0.0.0.0:7001
    service: iperf

  # for iperf_udp (iperf3)
  - binding: proxy_udp
    address: udp:0.0.0.0:7001
    service: iperf_udp

  # example xgress_transport
  - binding: transport
    address: tls:0.0.0.0:7002
    options:
      retransmission: true
      randomDrops: true
      drop1InN: 5000

  # example xgress_udp
  - binding: transport_udp
    address: udp:0.0.0.0:7003
    options:
      retransmission: true
      randomDrops: true
      drop1InN: 5000

```

## Embedded Swagger/OpenAPI 2.0 endpoint

The endpoint`/specs` has been added to retrieve API specifications from the Ziti Controller. The
specifications are specific to the version of the controller deployed.

The main endpoint to retrieve the Swagger/Open API 2.0 specification is: `/specs/swagger/spec`

    * Endpoint: /specs
    * Supported operations
        * Detail: GET /specs/<spec-id>
        * Get Spec: GET /specs/<spec-id>/spec
        * List: GET /specs/
     * Properties
         * Transit Routers support the standard properties (id, createdAt, updatedAt, tags)
         * name - Type string - the and intent of the spec

## APIs now only accept ID, not ID or Name

1. Some APIs related to configurations accepted config names or ids. These now only accept name.
1. Policies would accept entity references with names as well as ids. So you could use `@ssh`, for
   example when referencing the ssh service. These now also only accept ID

In general allowing both values adds complexity to the server side code. Consuming code, such as
user interfaces or the ziti cli, can do the name to id translation just as easily.

## Fabric Enhancements

### Xlink Framework

The new Xlink framework **requires** that the router configuration file is updated to `v: 3`.

The `link` section of the router configuration is now structured like this:

```
link:
  listeners:
    - binding:          transport
      bind:             tls:127.0.0.1:6002
      advertise:        tls:127.0.0.1:6002
      options:
        outQueueSize:   16
  dialers:
    - binding:          transport
```

The `link/listeners/bind` address replaces the old `link/listener` address, and
the `link/listeners/advertise` address replaces the old `link/advertise` address.

**The router configuration MUST be updated to include `link/dialers` section with a `transport`
binding (as in the above example) to include support for outbound link dialing.**

Subsequent releases will include support for multiple Xlink listeners and dialers. 0.13 supports
only a single listener and dialer to be configured.

### Xgress MTU

The Xgress listeners and dialers now support an `mtu` option in their `options` stanza:

```
listeners:
  # basic ssh proxy
  - binding:            proxy
    address:            tcp:0.0.0.0:1122
    service:            ssh
    options:
      mtu:              768
      
dialers:
  - binding:            transport
    options:
      mtu:              512
```

This MTU controls the maximum size of the `Payload` packet sent across the data plane of the
overlay.

