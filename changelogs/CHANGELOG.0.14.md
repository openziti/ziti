# Release 0.14.13

Ziti 0.14.13 includes the following:

* Ensure version information gets updated on non linux-amd64 builds

# Release 0.14.12

Ziti 0.14.12 includes the following:

* Fix the logging prefix to be github.com/openziti

# Release 0.14.11

Ziti 0.14.11 includes the following:

* [Ziti-Tunnel - Bind terminators are only created during startup](https://github.com/openziti/sdk-golang/issues/56)
* [Close on one side of connection doesn't propagate to other side](https://github.com/openziti/edge/issues/189)
* [Simplify sequencer close logic](https://github.com/openziti/foundation/issues/81)
* Misc Fixes
    * PEM decoding returns error when not able to decode
    * Ziti enrolment capabilities now supports `plain/text`, `application/x-pem-file`,
      and `application/json` response `accept` and `content-types`
* CLI Change
    * ziti-tunnel has learned a new subcommand `enroll`. Usage is identical to the
      existing `ziti-enroller`

# Release 0.14.10

Ziti 0.14.10 includes the following:

* Doc updates

# Release 0.14.9

Ziti 0.14.9 includes the following:

* [Move ziti edge controller commands to ziti edge](https://github.com/openziti/ziti/issues/108)
    * Note: for now `ziti edge` and `ziti edge controller` will both have edge controller related
      commands. `ziti edge controller` is deprecated and will be removed in a future release. Please
      update your scripts.

# Release 0.14.8

Ziti 0.14.8 includes the following:

* Doc updates

# Release 0.14.7

Ziti 0.14.7 includes the following:

* [Add CLI support for updating terminators](https://github.com/openziti/ziti/issues/106)
* [Add CLI support for managing identity service config overrides](https://github.com/openziti/ziti/issues/105)

NOTE: 0.14.6 was released with the same code as 0.14.5 due to CI re-running

# Release 0.14.5

## Theme

Ziti 0.14.5 includes the following:

### Features

* Ziti Edge API
    * [CA Identity Name Format](https://github.com/openziti/edge/issues/147)
* [Remove sourceType from metrics](https://github.com/openziti/foundation/issues/68)
* Fix name of metric from `egress.tx.Msgrate` to `egress.tx.msgrate`

## Ziti Edge API

### CA Identity Name Format

A new field, `identityNameFormat`,has been added to all certificate authority elements (`GET /cas`)
that is available for all CRUD operations. This field is optional and defaults
to `[caName] - [commonName]`. All existing CAs will also default to `[caName] - [commonName]`.

The field, `identityNameFormat`, may contain any text and optionally include the following strings
that are replaced with described values:

* `[caId]` - the id of the CA used for auto enrollment
* `[caName]` - the name of the CA used for auto enrollment
* `[commonName]` - the common name supplied by the enrolling cert
* `[identityName]` - the name supplied during enrollment (if any, defaults to `[identityId]` if
  the `name` field is blank during enrollment)
* `[identityId]` - id of the resulting identity

The default, `[caName] - [commonName]`, would result in the following for a CA named "myCa" with an
enrolling certificate with the common name "laptop01":

```
myCa - laptop01
```

#### Identity Name Collisions

If an `identityNameFormat` results in a name collision during enrollment, an incrementing number
will be appended to the resulting identity name. If this is not desired, define
an `identityNameFormat` that does not collide by using the above replacement strings and ensuring
the resulting values (i.e. from`commonName`) are unique.

# Release 0.14.4

## Theme

Ziti 0.14.4 includes the following:

### Misc

* Migration to github.com/openziti

# Release 0.14.3

## Theme

Ziti 0.14.3 includes the following:

### Fixes

* [orphaned enrollments/authenticators post identity PUT](https://github.com/openziti/edge/issues/158)

## Orphaned Enrollments/Authenticators

When updating an identity via PUT it was possible to clear the authenticators and enrollments
associated with the identity making it impossible to authenticate as that identity or complete
enrollment. This release removes failed enrollments, associates orphaned authenticators with their
target identities, addresses the root cause, and adds regression tests.

# Release 0.14.2

## Theme

Ziti 0.14.2 includes the following:

* CLI enhancements
    * [can't create service policy with @ identity name](https://github.com/openziti/ziti/issues/93)
    * [Add CLI commands to allow updating policies and role attributes](https://github.com/openziti/ziti/issues/94)
    * [CLI: read config/config-type JSON from file](https://github.com/openziti/ziti/issues/90)
* [Not found errors for assigned/related ids do not say which resource was not found](https://github.com/openziti/edge/issues/148)
* Fixes to connection setup timing

## CLI Updates

### Names in Policy Roles

Polices can now be created from the CLI using @name. This was previously supported natively in the
REST APIs, however it was stripped out for consistency. The CLI now supports this by looking up
names and replacing them with IDS when they are entered. When policies are listed they will show
names instead of IDs now as well.

```shell script
$ ziti edge controller create service-policy test-names Dial -i '#all' -s '@ssh'
Found services with id db9488ba-d0af-455b-9503-c6df88f228ff for name ssh
ba233791-8fde-44ba-9509-948275e3e3bb

$ ziti edge controller list service-policies 'name="test-names"'
id: ba233791-8fde-44ba-9509-948275e3e3bb    name: test-names    type: Dial    service roles: [@ssh]    identity roles: [#all]
results: 1-1 of 1 
```

### New Update Commands

There are now update commands which allow updating role attributes on edge-routers, identities and
services and roles on all three policy types.

All the update commands also allow changing the entity and policy names.

```shell script
$ ziti edge controller update identity jsmith-laptop -a us-east,sales
$ ziti edge controller update service-policy sales-na -s o365,mattermost
```

### Breaking Change to CLI commands

The shorthands for some policy flags have changed

* The shorthand for create edge-router-policy `--edge-router-roles` is now `-e`. It was `-r`
* The shorthand for create service-edge-router-policy `--edge-router-roles` is now `-e`. It was `-r`
* The shorthand for create service-policy `--service-roles` is now `-s`. It was `-r`

# Release 0.14.1

## Theme

Ziti 0.14.1 includes the following:

### Features

* [Enable graceful shutdown of bound connections](https://github.com/openziti/edge/issues/149)

### Fixes

* [Enrollments w/ 0 length bodies cause enrollment errors](https://github.com/openziti/edge/issues/150)
* Fixed race condition in end-to-end encryption setup
* Xt fixes
    * Fixed strategies missing session ended events
    * Fixed costed terminator sorting
    * Fixed race condition where terminators could be selected right after delete because they would
      have default cost.
    * Expanded space between precedence levels to ensure terminator static cost doesn't allow total
      costs to jump precedence boundary
    * Fixed type error in failure cost tracker
* Logging cleanup - many log statements that were error or info have been dropped to debug
* ziti-probe can now handle partial configs

## Graceful SDK Hosted Application Shutdown

The Golang SDK now returns an edge.Listener instead of a net.Listener from Listen

```
type Listener interface {
    net.Listener
    UpdateCost(cost uint16) error
    UpdatePrecedence(precedence Precedence) error
    UpdateCostAndPrecedence(cost uint16, precedence Precedence) error
}
```

This allows clients to set their precedence to `failed` when shutting down. This will allow them to
gracefully finishing any outstanding requests while ensuring that no new requests will be routed to
this application. This should allow for applications to do round-robin upgrades without service
interruption to clients. It also allows clients to influence costs on the fly based on knowledge
available to the application.

Support is currently limited to the Golang SDK, support in other SDKs will be forthcoming as it is
prioritized.

# Release 0.14.0

## Theme

Ziti 0.14.0 includes the following:

### Features

* The first full implementation of high availability (HA) and horizontal scale (HS) services

### Fixes

* [When using index scanner, wrong count is returned when using skip](https://github.com/openziti/foundation/issues/62)
* fabric now includes migration to extract terminators from services
* more errors which were returning 500 now return appropriate 404 or 400 field errors
* terminators are now validated when routers connect, and invalid ones can be removed
* a potential race condition in UDP connection last time has been fixed and UDP connection logging
  has been tidied
* Terminator precedence may now be specified in the golang SDK in the listen options when binding a
  service

## HA/HS

Ziti 0.12 extracted terminators from services. Services could have multiple terminators but only the
first one would get used. Service have a `terminatorStrategy` field which was previously unused. Now
the terminatorStrategy will determine how Ziti picks from multiple terminators to enable either HA
or HS behavior.

### Xt

The fabric now includes a new framework called Xt (eXtensible Terminators) which allows defining
terminator strategies and defines how terminator strategies and external components integrate with
smart routing. The general flow of terminator selection goes as follows:

1. A client requests a new session for a service
1. Smart routing finds all the active terminators for the session (active meaning the terminator's
   router is connected)
1. Smart routing calculates a cost for each terminator then hands the service's terminator strategy
   a list of terminators and their costs ranked from lowest to highest
1. The strategy returns the terminator that should be used
1. A new session is created using that path.

Strategies will often work by adjusting terminator costs. The selection algorithm the simply returns
the lowest cost option presented by smart routing.

#### Costs

There are a number of elements which feed the smart routing cost algorithm.

##### Route Cost

The cost of the route from the initiating route to the terminator router will be included in the
terminator cost. This cost may be influenced by things such as link latencies and user determined
link costs.

##### Static Cost

Each terminator has a static cost which can be set or updated when the terminator is created. SDK
applications can set the terminator cost when they invoke the Listen operation.

#### Precedence

Each terminator has a precedence. There are three precedence levels: `required`, `default`
and `failed`.

Smart routing will always rank terminators with higher precedence levels higher than terminators
with lower precedence levers. So required terminators will always be first, default second and
failed third. Precedence levels can be used to implement HA. The primary will be marked as required
and the secondary as default. When the primary is determined to be down, either by some internal or
external set of heuristics, it will be marked as Failed and new sessions will go to the secondary.
When the primary recovers it can be bumped back up to Required.

##### Dynamic Cost

Each terminator also has a dynamic cost that will move a terminator up and down relative to its
precedence. This cost can be driven by stratagies or by external components. A strategy might use
number of active of open sessions or dial successes and failures to drive the cost.

##### Cost API

Costs can be set via the Costs API in Xt:

```
package xt

type Costs interface {
	ClearCost(terminatorId string)
	GetCost(terminatorId string) uint32
	GetStats(terminatorId string) Stats
	GetPrecedence(terminatorId string) Precedence
	SetPrecedence(terminatorId string, precedence Precedence)
	SetPrecedenceCost(terminatorId string, weight uint16)
	UpdatePrecedenceCost(terminatorId string, updateF func(uint16) uint16)
	GetPrecedenceCost(terminatorId string) uint16
}
```

Each terminator has an associated precedence and dynamic cost. This can be reduced to a single cost.
The cost algorithm ensures terminators at difference precedence levels do not overlap. So a
terminator which is marked failed, with dynamic cost 0, will always have a higher calculated cost
than a terminator with default precedence and maximum value for dynamic cost.

#### Strategies

Strategies must implement the following interface:

```
package xt

type Strategy interface {
	Select(terminators []CostedTerminator) (Terminator, error)
	HandleTerminatorChange(event StrategyChangeEvent) error
	NotifyEvent(event TerminatorEvent)
}
```

The `Select` method will be called by smart routing to pick terminators for a session. The session
can react to terminator changes, such when a terminator is added to or removed from a service. The
service is also notified via `NotifyEvent` whenever a session dial succeeds or fails and when a
session for the service is ended.

The fabric currently provides four strategy implementations.

##### `smartrouting`

This is the default strategy. It always uses the lowest cost terminator. It drives costs as follows:

* Cost is proportional to number of open sessions
* Dial failures drive the cost up
* Dial successes drive the cost down, but only as much as they were previously driven up by failures

##### `weighted`

This strategy drives costs in the same way as the `smartrouting` strategy. However instead of always
picking the lowest cost terminator it does a weighted random selection across all terminators of the
highest precedence. If a terminator has double the cost of another terminator it should get picked
approximately half as often.

##### `random`

This strategy does not change terminator weights. It does simple random selection across all
terminators of the highest precedence.

##### `ha`

This strategy assumes that one terminator will have `required` precedence and there will be a
secondary terminator with `default` precedence. If three consecutive dials to the highest ranked
terminator fail in a row it will be marked as failed. This will allow the secondary to take over. If
the primary recovers it can be marked as required again via the APIs.

### API Changes

The terminator endpoint now supports setting the static terminator cost and terminator precedence.

    * Endpoint: /terminators
        * Operations: PUT/POST/PATCH now take 
            * cost, type uint16, default 0
            * precedence, type string, default 'default', valid values: required, default, failed
        * Operation: GET now returns staticCost, dynamicCost

