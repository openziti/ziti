# Release 0.12

## Theme

Ziti 0.12 includes the following:

* Terminators have been extracted from services
    * Terminators define where a service terminates. Previously each service had exactly one
      terminator. Now services can have 0 to N terminators.
* List APIs now support inline paging
* Association APIs now support filtering, paging and querying
* The bolt datastore creates a backup of the datastore file before attempting a schema/data
  migration
* Fabric and edge code are now much more closely aligned at the persistence and model layers
* Some deprecated endpoints are now being removed

## Terminators

See https://github.com/openziti/fabric/wiki/Pluggable-Service-Terminators for a discussion of what
service terminators are, the motivation for extracting them from services and the design for how
they will work.

This release includes the following:

* Terminators extracted from service as separate entities
* When SDK applications bind and unbind, the controller now dynamically adds/removes terminators as
  appropriate

This release does not yet include a terminator strategy API. Strategies can be specified per
service, but if a service has multiple terminators the first one will be used. The terminator
strategy API along with some implementations will be added in a follow-up release. This release also
does not include strategy inputs on terminators as discussed in the above design document. If
strategy inputs end up being useful, they may be added in the future.

### Terminator related API changes

There is a new terminators endpoint:

    * Endpoint: /terminators
    * Supported operations
        * Detail: GET /terminators/<terminator-id>
        * List: GET /terminators/
        * Create: POST /terminators
        * Update All Fields: PUT /terminators/<terminator-id>
        * Update Selective Fields: PATCH /terminators/<terminator-id>
        * Delete: DELETE /terminators/<terminator-id>
     * Properties
         * Terminators support the standard properties (id, createdAt, updatedAt, tags)
         * service - type: uuid, must be a valid service id
         * router - type: uuid, must be a valid router id
         * binding - type: string. Optional, defaults to "transport". The xgress binding on the selected router to use
         * address - type: string. The address that will be dialed using the xgress component on the selected router

The service endpoint has changes as well:

    * Endpoint: /services
    * New operations
       * Query related endpoints: GET /services/<service-id>/terminators?filter=<optional-filter>
    * The following properties have been removed
       * egressRouter
       * endpointAddress
    * The following property has been added
       * terminatorStrategy - type: string, optional. The terminator strategy to use. Currently unused.

The fabric service definition has also changed (visible from ziti-fabric).

* The following properties have been removed
    * `binding`
    * `egressRouter`
    * `endpointAddress`
* The following property has been added
    * `terminatorStrategy`

The ziti and ziti-fabric CLIs have been updated with new terminator related functionality, so that
terminators can be viewed, created and deleted from both.

## Filtering/Sorting/Paging Changes

List operations on entities previously allowed the following parameters:

* `filter`
* `sort`
* `limit`
* `offset`

These are all still supported, but now sort, limit and offset can also be included in the filter. If
parameters are specified both in the filter and in an explicit query parameter, the filter takes
precedence.

When listing entities from the ziti CLI, filters can be included as an optional argument.

For example:

    $ ziti edge controller list services
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: 9480e39d-0664-4482-b230-5da2c17b225b    name: iperf    terminator strategy:     role attributes: {}
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    id: dc0446f0-7eaa-465f-80b5-c88f0a6b59cc    name: grpc-ping    terminator strategy:     role attributes: ["fortio","fortio-server"]
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}

    $ ziti edge controller list services 'name contains "s"'
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}

    $ ziti edge controller list services 'name contains "s" sort by name'
    id: 37f1e34c-af06-442f-8e62-032916912bc6    name: grpc-ping-standalone    terminator strategy:     role attributes: {}
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}
    id: cd1ae16e-5015-49ad-9864-3ca0f5814091    name: ssh    terminator strategy:     role attributes: {}

    $ ziti edge controller list services 'name contains "s" sort by name skip 1 limit 2'
    id: dcc9922a-c681-41bf-8079-be2163509702    name: mattermost    terminator strategy:     role attributes: {}
    id: 4e33859b-070d-42b1-8b40-4adf973f680c    name: simple    terminator strategy:     role attributes: {}

Association lists now also support filtering, sorting and paging. Association GET operations only
support the filter parameter.

    $ ziti edge controller list service terminators ssh
    Found services with id cd1ae16e-5015-49ad-9864-3ca0f5814091 for name ssh
    id: 41f4fd01-0bd7-4987-93b3-3b2217b00a22    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:22
    id: a5213300-9c5f-4b0e-a790-1ed460964d7c    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:2022

    $ ziti edge controller list service terminators ssh "true sort by address"
    Found services with id cd1ae16e-5015-49ad-9864-3ca0f5814091 for name ssh
    id: a5213300-9c5f-4b0e-a790-1ed460964d7c    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:2022
    id: 41f4fd01-0bd7-4987-93b3-3b2217b00a22    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:22

    $ ziti edge controller list service terminators ssh "true sort by address desc"
    Found services with id cd1ae16e-5015-49ad-9864-3ca0f5814091 for name ssh
    id: 41f4fd01-0bd7-4987-93b3-3b2217b00a22    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:22
    id: a5213300-9c5f-4b0e-a790-1ed460964d7c    serviceId: cd1ae16e-5015-49ad-9864-3ca0f5814091    routerId: 888cfde1-5786-4ba8-aa75-9f97804cb7bb    binding: transport    address: tcp:localhost:2022

## Bolt Datastore Migrations

The fabric now supports migrating schema/data from one version to another. The fabric and edge share
a common framework for migration. The migration framework now also automatically backs up the bolt
data file before migration data. The backup file will have the same name as the original bolt file
but with a timestamp appended to it.

Example:

    Original file: /tmp/ziti-bolt.db
    Backup file:   /tmp/ziti-bolt.db-20200316-134725

The fabric and edge schemas do not yet get migrated in the same transaction. This will be addressed
in a follow-up release.

## Fabric and Edge Alignment

The fabric and edge persistence and model layers are now using the same foundational plumbing. This
will allow for a common API layer in a follow-up release.

As part of this consolidation effort, fabric entities now share the same set of common properties as
edge entities, namely:

* `id`
* `createdAt`
* `updatedAt`
* `tags`

Previously the only common property was `id`.

## Deprecated Endpoints

The `/gateways` (replaced by `/edge-routers`) and `network-sessions` (replaced by `/sessions`)
endpoints, which were previously deprecated, have now been removed.

## Miscellaneous

There is a new `ziti edge controller version` command which shows information about the version of
the controller being connected to:

Example:

    $ ziti edge controller version
    Version     : v0.9.0
    GIT revision: ea556fc18740
    Build Date  : 2020-02-11 16:09:08
    Runtime     : go1.13
