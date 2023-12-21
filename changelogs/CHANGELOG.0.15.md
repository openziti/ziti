# Release 0.15.3

* What's New:
    * Add example docker compose for ziti-tunnel

# Release 0.15.2

* What's New:
    * [#140](https://github.com/openziti/ziti/issues/140) - Allow logging JSON request for Ziti CLI
    * [#148](https://github.com/openziti/ziti/issues/148) - Show isOnline in ziti edge list
      edge-routers
    * [#144](https://github.com/openziti/ziti/issues/144) - Allow ziti-fabric list to use queries.
      Default to `true limit none`

* Bug Fixes:
    * [#142](https://github.com/openziti/ziti/issues/142) - fix CLI ca create not defaulting
      identity roles
    * [#146](https://github.com/openziti/ziti/issues/146) - Export edge router JWT fails sometimes
      when there are more than 10 edge routers
    * [#147](https://github.com/openziti/ziti/issues/147) - Fix paging output when using 'limit
      none'
    * [edge#243](https://github.com/openziti/edge/issue/243) - Session creation only returns 10 edge
      routers
    * [edge#245](https://github.com/openziti/edge/issue/245) - fingerprint calculation changed from
      0.14 to 0.15. Ensure 0.15 routers can work with 0.14 controllers
    * [edge#248](https://github.com/openziti/edge/issue/248) - Edge Router Hello can time out on
      slow networks with many links to establish
    * [foundation#103](https://github.com/openziti/foundation/issues/103) - Fix config file env
      injection for lists

# Release 0.15.1

* What's New:
  No new functionality introduced.

* Bug fixes
    * [#129](https://github.com/openziti/ziti/issues/129) - minor issue with `ziti-tunnel enroll`
      outputting the success message at ERROR level
    * [#131](https://github.com/openziti/ziti/issues/131) - issues w/ creating identities, CAs and
      validating CAs
    * [#133](https://github.com/openziti/ziti/issues/133) - fix service lookup by name when creating
      service edge router policies
    * [edge#191](https://github.com/openziti/edge/issues/191) - updating self password via CLI would
      error with 404 not found
    * [edge#231](https://github.com/openziti/edge/issues/231) - identities missing enrollment
      expiresAt property
    * [edge#237](https://github.com/openziti/edge/issues/237) - Policy Advisor CLI is failing
      because common routers IsOnline value is missing
    * [edge#233](https://github.com/openziti/edge/issues/233) - REST API Errors should be
      application/json if possible
    * [edge#240](https://github.com/openziti/edge/issues/240) - listing specs results in a 404

# Release 0.15.0

Ziti 0.15.0 includes the following:

* The ability to invoke a database snapshot/backup
    * [Create fabric mgmt API to request database snapshot/backup be created](https://github.com/openziti/fabric/issues/99)
    * [Add snapshot db REST API](https://github.com/openziti/edge/issues/206)
* Removal of deprecated code/migrations
    * [Remove postgres store code including migrations](https://github.com/openziti/edge/issues/195)
    * Remove deprecated AppWan and Clusters - These have been replaced by service policies and
      service edge router policies respectively
* Edge Routers are now a subtype of Fabric Routers
    *
  see [Unverified Edge Routers Cannot Be Used For Terminators](https://github.com/openziti/edge/issues/144)
* Fabric services and routers now have names
    * see [Add name to service and router](https://github.com/openziti/fabric/issues/101)
* cosmetic changes to the ziti-enroller binary
* cosmetic changes to the ziti-tunnel binary when running the enroll subcommand
* Memory leak remediation in the `PayloadBuffer` subsystem. Corrects unbounded memory growth
  in `ziti-router`.
* Edge REST API Enhancements
    * [OpenApi 2.0/Swagger](https://github.com/openziti/edge/issues/108)
    * [Changes to support Fabric REST API](https://github.com/openziti/edge/issues/101)

## Removal of deprecated code

The code to migrate a Ziti instance from pre-0.9 releases has been removed. If you want to migrate
from a pre-0.9 version you should first update to 0.14.12, then to new versions.

## Database Snapshots

Database snapshots can now be triggered in a variety of ways to cause the creation of a database
backup/snapshot. This can be done from the ziti-fabric CLI, the ziti CLI and the REST API

    $ ziti-fabric snapshot-db
    $ ziti edge snapshot-db

The REST API is available by POSTing to `/edge/v1/database/snapshot`. This ability is only available
to administrators.

The snapshot will be a copy of the database file created within a bolt transaction. The file name
will have the data and time appended to it. Snapshotting can be done at most once per minute.

## Edge Routers/Fabric Router subtyping

Previously edge routers and fabric routers were closely related, but weren't actually the same
entity. When an edge router was created, there was no corresponding fabric router until the edge
router had been successfully enrolled.

Now, edge routers are a type of fabric router. When an edge router is created, it will be visible as
a fabric router with no fingerprint. This means that the corresponding router application won't be
able to connect until enrollment is complete.

This simplifies some things, including allowing adding terminators to an edge router before
enrollment is complete.

## Fabric Router and Service Names

Previously fabric routers and services only had ids, which were assumed to be something user
friendly. Now they also have a name. If no name is provided, the id will be used for the name as
well. This was done primarily so that we have consistency between the fabric and the edge. Now when
viewing a service or router you can be sure to find the label in the same place.

### Edge REST API Enhancements

The v0.15.0 brings in new Edge REST API changes that are being made in preparation for future
enhancements. Please read these changes carefully and adopt new patterns to avoid future
incompatibility issues.

#### OpenApi 2.0/Swagger

The REST presentation of the Edge REST API is now fully generated from the Open API 2.0/Swagger
specification in `edge/spec`. The the generated code is in `edge/rest_model`, `edge/rest_server`,
and
`edge_rest_client`. The code is generated by installing `go-swagger`, currently at version 0.24.0.

The generated code introduces a few changes that may impact clients:

* `content-type` and `accept` headers are now meaningful and important
* enrollment endpoints can return JSON if JSON is explicitly set in
  `accept` headers
* API input validation errors
* various entity ref bugs
* standardization of id properties

#### Content Type / Accept Headers

For `content-type` and `accept` headers, if `accept` is not being set, clients usually send
an `accept` of `*/*` - accepting anything. If so, the Edge REST API will continue to
return `content-type`s that are the same as previous versions. However, non-JSON responses from
enrollment endpoints are now deprecated.

If a client is setting the `accept` header to anything other than
`application/json` for most endpoints, the API will return errors stating the that the content types
are not acceptable.

#### API Input Validation

API input validation is now handled by the Open API libraries and go-swagger generated code. The
error formats returned are largely the same. However validations errors now all return the same
outer error and set the cause error properly. Prior to this change errors were handled in an
inconsistent manner.

#### Entity Ref Bugs

Various entity references were fixed where URLs were pointing to the wrong or invalid API URLs.

#### Id Properties

Id properties are now fully typed: <type>Id, in API request/responses.

Entities affected:

* config
    * `type` to `configTypeid`
* identity service config
    * `service` to `serviceId`
    * `config` to `configId`

`IdentityTypeId` references were not updated as they are slated for removal and are now deprecated.
This includes `/identity-type` and associated `/identity` properties for create/update/patch
operations.

### Changes to support Fabric REST API

The following changes were done to support the future Fabric REST API

* Edge REST API base moved to `/edge/v1`
* `apiVersions` was introduced to `GET /versions`
* move away from UUID formats for ids to shortIds

#### Base Path

The Edge REST API now has a base path of `edge/v1`. The previous base path, `/`, is now deprecated
but remains active till a later release. This move is to create room for the Fabric REST API to take
over the root path and allow other components to register APIs.

#### API Versions

For now the `GET /versions` functionality is handled by the Edge REST API but will be subsumed by a
future Fabric REST API.

The `GET /versions` now reports version information in a map structure such that future REST APIs,
as they are introduced, can register supported versions. It is the goal of the Ziti REST APIs to
support multiple API versions for backwards comparability.

Example `GET /versions` response:

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
        "buildDate": "2020-06-11 16:03:13",
        "revision": "95e78d4bc64b",
        "runtimeVersion": "go1.14.3",
        "version": "v0.15.0"
    },
    "meta": {}
}
```

Example of a theoretical future version with the Fabric REST API:

```
{
    "data": {
        "apiVersions": {
            "edge": {
                "v1": {
                    "path": "/edge/v1"
                }
            }
            "fabric": {
                "v1": {
                    "path": "/fabric/v1"
                }
            }
        },
        "buildDate": "2020-06-20 12:43:03",
        "revision": "1a27ed4bc64b",
        "runtimeVersion": "go1.14.3",
        "version": "v0.15.10"
    },
    "meta": {}
}
```

#### ShortIds

The Edge REST API has used UUID and its associated UUID text format for all ids. In 0.15 and
forward, `shortIds` will be used and their associated format.

* make ids more human human friendly (logs, visual comparison)
* consolidate on ids that look similar between Fabric and Edge entities
* maintain a high degree of uniqueness comparable to UUIDs

All Ziti REST APIs will specify their ids as `strings`. If clients treat ids as opaque strings, then
no comparability issues are expected. It is highly highly suggested that all clients follow this
pattern.
