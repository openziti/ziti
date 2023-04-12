# Release 0.27.9

## What's New

* Refactored the websocket transport to fix a concurrency issue
* v0.27.6 changed delete behaviors to error if the entity was not found. This release reverts that behavior.

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.53 -> v2.0.58](https://github.com/openziti/channel/compare/v2.0.53...v2.0.58)
* github.com/openziti/edge: [v0.24.228 -> v0.24.239](https://github.com/openziti/edge/compare/v0.24.228...v0.24.239)
    * [Issue #1391](https://github.com/openziti/edge/issues/1391) - AuthPolicies for identities is missing a reference link

* github.com/openziti/edge-api: [v0.25.9 -> v0.25.11](https://github.com/openziti/edge-api/compare/v0.25.9...v0.25.11)
* github.com/openziti/fabric: [v0.22.77 -> v0.22.87](https://github.com/openziti/fabric/compare/v0.22.77...v0.22.87)
* github.com/openziti/foundation/v2: [v2.0.18 -> v2.0.21](https://github.com/openziti/foundation/compare/v2.0.18...v2.0.21)
* github.com/openziti/identity: [v1.0.42 -> v1.0.45](https://github.com/openziti/identity/compare/v1.0.42...v1.0.45)
* github.com/openziti/runzmd: [v1.0.18 -> v1.0.20](https://github.com/openziti/runzmd/compare/v1.0.18...v1.0.20)
* github.com/openziti/storage: [v0.1.46 -> v0.1.49](https://github.com/openziti/storage/compare/v0.1.46...v0.1.49)
* github.com/openziti/transport/v2: [v2.0.68 -> v2.0.72](https://github.com/openziti/transport/compare/v2.0.68...v2.0.72)
* github.com/openziti/metrics: [v1.2.16 -> v1.2.19](https://github.com/openziti/metrics/compare/v1.2.16...v1.2.19)
* github.com/openziti/ziti: [v0.27.8 -> v0.27.9](https://github.com/openziti/ziti/compare/v0.27.8...v0.27.9)

# Release 0.27.8

## What's New

* CLI additions for auth policies and external JWT signers
* Performance improvements for listing services

## Component Updates and Bug Fixes

* github.com/openziti/edge: [v0.24.224 -> v0.24.228](https://github.com/openziti/edge/compare/v0.24.224...v0.24.228)
    * [Issue #1388](https://github.com/openziti/edge/issues/1388) - Make better use of identity service indexes for service list
    * [Issue #1386](https://github.com/openziti/edge/issues/1386) - PUT on identities results in an error and internal PANIC

* github.com/openziti/fabric: [v0.22.76 -> v0.22.77](https://github.com/openziti/fabric/compare/v0.22.76...v0.22.77)
* github.com/openziti/storage: [v0.1.45 -> v0.1.46](https://github.com/openziti/storage/compare/v0.1.45...v0.1.46)
* github.com/openziti/ziti: [v0.27.7 -> v0.27.8](https://github.com/openziti/ziti/compare/v0.27.7...v0.27.8)
    * [Issue #1064](https://github.com/openziti/ziti/issues/1064) - Support auth-policy assignments on identities via the CLI
    * [Issue #1058](https://github.com/openziti/ziti/issues/1058) - Allow Auth Policy Create/Update/Delete via CLI
    * [Issue #1059](https://github.com/openziti/ziti/issues/1059) - Expose Delete for Ext JWT Signers in CLI

# Release 0.27.7

## What's New

* This release updates the build to use Go 1.20

# Release 0.27.6

## What's New

* Makes inspect CLI more discoverable by adding subcommands for inspectable values
* Adds new inspection allowing configs to be retrieved: `ziti fabric inspect config`
* Many improvements to edge-router/tunneler hosting performance with large numbers of hosted services
    * Routers should no longer overwhelm controller while setting up or reestablishing hosting
* Adds ability to disable router
* Adds CLI command to compact offline bbolt database: `ziti ops db compact <src> <dst>`
* Adds CLI command to re-enroll edge routers: `ziti edge re-enroll edge-router`
* Routers can now be disabled. Connections to the controller from disabled routers will be rejected.
    * Disable with: `ziti fabric update router <router-id> --disabled`
    * Enable with:  `ziti fabric update router <router-id> --disabled=false`

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.8 -> v1.0.10](https://github.com/openziti/agent/compare/v1.0.8...v1.0.10)
* github.com/openziti/channel/v2: [v2.0.27 -> v2.0.53](https://github.com/openziti/channel/compare/v2.0.27...v2.0.53)
    * [Issue #83](https://github.com/openziti/channel/issues/83) - Improve protocol mismatch error(s)
    * [Issue #93](https://github.com/openziti/channel/issues/93) - Fix atomic 64-bit alignment error on arm devices

* github.com/openziti/edge: [v0.24.125 -> v0.24.224](https://github.com/openziti/edge/compare/v0.24.125...v0.24.224)
    * [Issue #1373](https://github.com/openziti/edge/issues/1373) - Add support for disabled flag to edge and transit routers 
    * [Issue #1374](https://github.com/openziti/edge/issues/1374) - Multiple MFA enrollments cannot be cleaned up by administrators
    * [Issue #1336](https://github.com/openziti/edge/issues/1336) - xgress_edge_tunnel shouldn't stop/start host on control channel reconnect
    * [Issue #1369](https://github.com/openziti/edge/issues/1369) - Add missing entity type id for TransitRouter
    * [Issue #1366](https://github.com/openziti/edge/issues/1366) - Error message incorrectly state 'invalid api session' when it's an invalid session
    * [Issue #1364](https://github.com/openziti/edge/issues/1364) - Cache api-sessions for tunneler router so we don't need to unnecessarily create new sessions
    * [Issue #1362](https://github.com/openziti/edge/issues/1362) - Rate limit terminator creates for router/tunneler
    * [Issue #1359](https://github.com/openziti/edge/issues/1359) - Sessions creates should be idempotent
    * [Issue #1355](https://github.com/openziti/edge/issues/1355) - Handle duplicate create terminator requests if create terminator fails
    * [Issue #1350](https://github.com/openziti/edge/issues/1350) - Router event processing can deadlock
    * [Issue #1329](https://github.com/openziti/edge/issues/1329) - UDP connections can drop data if datagrams are > 10k in size
    * [Issue #1310](https://github.com/openziti/edge/issues/1310) - Creating a cert backed ext-jwt-signer causes nil dereference

* github.com/openziti/edge-api: [v0.25.6 -> v0.25.9](https://github.com/openziti/edge-api/compare/v0.25.6...v0.25.9)
* github.com/openziti/fabric: [v0.22.24 -> v0.22.76](https://github.com/openziti/fabric/compare/v0.22.24...v0.22.76)
    * [Issue #651](https://github.com/openziti/fabric/issues/651) - Add router enable/disable mechanism
    * [Issue #648](https://github.com/openziti/fabric/issues/648) - Add rate limiter pool to router for operations with potential to flood the controller 
    * [Issue #610](https://github.com/openziti/fabric/issues/610) - Fix router disconnect when endpoint removed from cluster
    * [Issue #622](https://github.com/openziti/fabric/issues/622) - fatal error: concurrent map iteration and map write in logContext.WireEntry
    * [Issue #507](https://github.com/openziti/fabric/issues/507) - Add configuration for control channel heartbeat
    * [Issue #584](https://github.com/openziti/fabric/issues/584) - Add cluster events
    * [Issue #599](https://github.com/openziti/fabric/issues/599) - Add release and transfer leadership commands
    * [Issue #606](https://github.com/openziti/fabric/issues/606) - Ensure consistent use of peer address
    * [Issue #598](https://github.com/openziti/fabric/issues/598) - Add support to fabric inspect to propagate inspect to other controllers
    * [Issue #597](https://github.com/openziti/fabric/issues/597) - Make raft settings configurable
    * [Issue #604](https://github.com/openziti/fabric/issues/604) - Don't create link dropped msg metric until channel bind time
    * [Issue #638](https://github.com/openziti/fabric/issues/638) - Fix atomic 64-bit alignment error on arm devices

* github.com/openziti/foundation/v2: [v2.0.10 -> v2.0.18](https://github.com/openziti/foundation/compare/v2.0.10...v2.0.18)
* github.com/openziti/identity: [v1.0.30 -> v1.0.42](https://github.com/openziti/identity/compare/v1.0.30...v1.0.42)
* github.com/openziti/runzmd: [v1.0.9 -> v1.0.18](https://github.com/openziti/runzmd/compare/v1.0.9...v1.0.18)
* github.com/openziti/sdk-golang: [v0.18.28 -> v0.18.76](https://github.com/openziti/sdk-golang/compare/v0.18.28...v0.18.76)
    * [Issue #356](https://github.com/openziti/sdk-golang/issues/356) - sdk connections should respect net.Conn deadline related API specifications 

* github.com/openziti/storage: [v0.1.34 -> v0.1.45](https://github.com/openziti/storage/compare/v0.1.34...v0.1.45)
* github.com/openziti/transport/v2: [v2.0.51 -> v2.0.68](https://github.com/openziti/transport/compare/v2.0.51...v2.0.68)
* github.com/openziti/jwks: [v1.0.2 -> v1.0.3](https://github.com/openziti/jwks/compare/v1.0.2...v1.0.3)
* github.com/openziti/metrics: [v1.2.3 -> v1.2.16](https://github.com/openziti/metrics/compare/v1.2.3...v1.2.16)
* github.com/openziti/ziti: [v0.27.5 -> v0.27.6](https://github.com/openziti/ziti/compare/v0.27.5...v0.27.6)
    * [Issue #1041](https://github.com/openziti/ziti/issues/1041) - Add ziti compact command to CLI
    * [Issue #1032](https://github.com/openziti/ziti/issues/1032) - ziti edge create service fails silently if config names don't exist
    * [Issue #1031](https://github.com/openziti/ziti/issues/1031) - Fixed quickstart bug with arm and arm64 ambiguity when running quickstart on arm architecture

# Release 0.27.5

## What's New

* Fixes an issue with `ziti` CLI when using a globally trusted CA
* Fixes bug where `ziti agent stack` was calling `ziti agent stats`
* ziti controller/router no longer compare the running version with 
  the latest from github by default. Set ZITI_CHECK_VERSION=true to
  enable this behavior

## Component Updates and Bug Fixes

* github.com/openziti/edge: [v0.24.121 -> v0.24.125](https://github.com/openziti/edge/compare/v0.24.121...v0.24.125)
* github.com/openziti/fabric: [v0.22.20 -> v0.22.24](https://github.com/openziti/fabric/compare/v0.22.20...v0.22.24)
    * [Issue #601](https://github.com/openziti/fabric/issues/601) - Only use endpoints file in router once endpoints have changed
    * [Issue #583](https://github.com/openziti/fabric/issues/583) - Compress raft snapshots

* github.com/openziti/sdk-golang: [v0.18.27 -> v0.18.28](https://github.com/openziti/sdk-golang/compare/v0.18.27...v0.18.28)
* github.com/openziti/storage: [v0.1.33 -> v0.1.34](https://github.com/openziti/storage/compare/v0.1.33...v0.1.34)
* github.com/openziti/ziti: [v0.27.4 -> v0.27.5](https://github.com/openziti/ziti/compare/v0.27.4...v0.27.5)

# Release 0.27.4

## What's New

This release contains a fix for a controller deadlock

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.26 -> v2.0.27](https://github.com/openziti/channel/compare/v2.0.26...v2.0.27)
* github.com/openziti/edge: [v0.24.115 -> v0.24.121](https://github.com/openziti/edge/compare/v0.24.115...v0.24.121)
    * [Issue #1303](https://github.com/openziti/edge/issues/1303) - Fix deadlock when flushing api session heartbeats 

* github.com/openziti/fabric: [v0.22.19 -> v0.22.20](https://github.com/openziti/fabric/compare/v0.22.19...v0.22.20)
* github.com/openziti/sdk-golang: [v0.18.26 -> v0.18.27](https://github.com/openziti/sdk-golang/compare/v0.18.26...v0.18.27)
* github.com/openziti/transport/v2: [v2.0.50 -> v2.0.51](https://github.com/openziti/transport/compare/v2.0.50...v2.0.51)
* github.com/openziti/ziti: [v0.27.3 -> v0.27.4](https://github.com/openziti/ziti/compare/v0.27.3...v0.27.4)

# Release 0.27.3

## What's New

* Docker images for `ziti` CLI

* New Raft interaction commands
    * `raft-leave` allows removal of controllers from the raft cluster
    * `raft-list` lists all connected controllers and their version/connected status
    * `fabric raft list-members` same info as the agent command, but over rest

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.7 -> v1.0.8](https://github.com/openziti/agent/compare/v1.0.7...v1.0.8)
* github.com/openziti/channel/v2: [v2.0.25 -> v2.0.26](https://github.com/openziti/channel/compare/v2.0.25...v2.0.26)
* github.com/openziti/edge: [v0.24.95 -> v0.24.115](https://github.com/openziti/edge/compare/v0.24.95...v0.24.115)
    * [Issue #1292](https://github.com/openziti/edge/issues/1292) - Support alternative tproxy configuration methods

* github.com/openziti/edge-api: v0.25.6 (new)
* github.com/openziti/fabric: [v0.22.7 -> v0.22.19](https://github.com/openziti/fabric/compare/v0.22.7...v0.22.19)
    * [Issue #592](https://github.com/openziti/fabric/issues/592) - Incoming "gateway" connections should be logged at a socket level
    * [Issue #588](https://github.com/openziti/fabric/issues/588) - Make service events more consistent
    * [Issue #589](https://github.com/openziti/fabric/issues/589) - Add duration to circuit updated and deleted events
    * [Issue #508](https://github.com/openziti/fabric/issues/508) - Refactor router debug ops for multiple controllers

* github.com/openziti/identity: [v1.0.29 -> v1.0.30](https://github.com/openziti/identity/compare/v1.0.29...v1.0.30)
* github.com/openziti/runzmd: [v1.0.7 -> v1.0.9](https://github.com/openziti/runzmd/compare/v1.0.7...v1.0.9)
* github.com/openziti/sdk-golang: [v0.18.21 -> v0.18.26](https://github.com/openziti/sdk-golang/compare/v0.18.21...v0.18.26)
* github.com/openziti/storage: [v0.1.31 -> v0.1.33](https://github.com/openziti/storage/compare/v0.1.31...v0.1.33)
* github.com/openziti/transport/v2: [v2.0.49 -> v2.0.50](https://github.com/openziti/transport/compare/v2.0.49...v2.0.50)
* github.com/openziti/ziti: [v0.27.2 -> v0.27.3](https://github.com/openziti/ziti/compare/v0.27.2...v0.27.3)
    * [Issue #974](https://github.com/openziti/ziti/issues/974) - tunnel "host" and "proxy" modes shouldn't run the nameserver
    * [Issue #972](https://github.com/openziti/ziti/issues/972) - tunnel segfault

# Release 0.27.2

## What's New

* Bug fixes

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.24 -> v2.0.25](https://github.com/openziti/channel/compare/v2.0.24...v2.0.25)
* github.com/openziti/edge: [v0.24.86 -> v0.24.95](https://github.com/openziti/edge/compare/v0.24.86...v0.24.95)
    * [Issue #1282](https://github.com/openziti/edge/issues/1282) - Ensure entity count events can be configured to only be emitted on the leader
    * [Issue #1279](https://github.com/openziti/edge/issues/1279) - Constrain config-type schema to accept only object types

* github.com/openziti/fabric: [v0.22.1 -> v0.22.7](https://github.com/openziti/fabric/compare/v0.22.1...v0.22.7)
    * [Issue #573](https://github.com/openziti/fabric/issues/573) - Ensure specific events aren't duplicated in raft cluster
    * [Issue #577](https://github.com/openziti/fabric/issues/577) - JSON Event formatter isn't putting events on their own line
    * [Issue #571](https://github.com/openziti/fabric/issues/571) - Move raft.advertiseAddress to ctrl for consistency
    * [Issue #569](https://github.com/openziti/fabric/issues/569) - Support automatic migration and agent based migration
    * [Issue #567](https://github.com/openziti/fabric/issues/567) - Remove link dropped_msg metrics for closed links
    * [Issue #566](https://github.com/openziti/fabric/issues/566) - Link listeners aren't properly configuring channel out queue size 

* github.com/openziti/foundation/v2: [v2.0.9 -> v2.0.10](https://github.com/openziti/foundation/compare/v2.0.9...v2.0.10)
* github.com/openziti/identity: [v1.0.28 -> v1.0.29](https://github.com/openziti/identity/compare/v1.0.28...v1.0.29)
* github.com/openziti/sdk-golang: [v0.18.19 -> v0.18.21](https://github.com/openziti/sdk-golang/compare/v0.18.19...v0.18.21)
* github.com/openziti/storage: [v0.1.30 -> v0.1.31](https://github.com/openziti/storage/compare/v0.1.30...v0.1.31)
* github.com/openziti/transport/v2: [v2.0.48 -> v2.0.49](https://github.com/openziti/transport/compare/v2.0.48...v2.0.49)
* github.com/openziti/metrics: [v1.2.2 -> v1.2.3](https://github.com/openziti/metrics/compare/v1.2.2...v1.2.3)
* github.com/openziti/ziti: [v0.27.1 -> v0.27.2](https://github.com/openziti/ziti/compare/v0.27.1...v0.27.2)
    * [Issue #916](https://github.com/openziti/ziti/issues/916) - Allow defining resource tags via json in the cli


# Release 0.27.1

## What's New

* Event streaming over websocket
    * `ziti fabric stream events`
    * Events use same JSON formatting as the file based streaming
    * Plain Text formatting removed
    * Individual streaming of metrics/circuits removed in favor of unified events streaming
* Improvements to router/tunneler terminator creation
    * Create terminator requests are now idempotent, so repeated requests will not result in multiple terminators
    * Create terminator requests are now asynchronous, so responses will no longer get timed out
    * There is new timer metric from routers, timing how long terminator creates take: `xgress_edge_tunnel.terminator.create_timer`

## Component Updates and Bug Fixes

* github.com/openziti/edge: [v0.24.75 -> v0.24.86](https://github.com/openziti/edge/compare/v0.24.75...v0.24.86)
    * [Issue #1272](https://github.com/openziti/edge/issues/1272) - Mark xgress_edge and xgress_edge_tunnel created terminators as system entity
    * [Issue #1270](https://github.com/openziti/edge/issues/1270) - Make xgress_edge_tunnel service hosting more scalabe
    * [Issue #1268](https://github.com/openziti/edge/issues/1268) - session deletion can get stalled by restarts

* github.com/openziti/fabric: [v0.21.36 -> v0.22.1](https://github.com/openziti/fabric/compare/v0.21.36...v0.22.1)
    * [Issue #563](https://github.com/openziti/fabric/issues/563) - Allow streaming events over webscocket, replacing stream circuits and stream metrics
    * [Issue #552](https://github.com/openziti/fabric/issues/552) - Add minimum cost delta for smart routing
    * [Issue #558](https://github.com/openziti/fabric/issues/558) - Allow terminators to be marked as system entities

* github.com/openziti/ziti: [v0.27.0 -> v0.27.1](https://github.com/openziti/ziti/compare/v0.27.0...v0.27.1)
    * [Issue #928](https://github.com/openziti/ziti/issues/928) - ziti fabric update terminator should not require setting router
    * [Issue #929](https://github.com/openziti/ziti/issues/929) - zit fabric list terminators isn't showing cost or dynamic cost 

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

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.5 -> v1.0.7](https://github.com/openziti/agent/compare/v1.0.5...v1.0.7)
* github.com/openziti/channel/v2: [v2.0.12 -> v2.0.24](https://github.com/openziti/channel/compare/v2.0.12...v2.0.24)
* github.com/openziti/edge: [v0.24.36 -> v0.24.75](https://github.com/openziti/edge/compare/v0.24.36...v0.24.75)
    * [Issue #1253](https://github.com/openziti/edge/issues/1253) - Panic in controller getting hello from edge router
    * [Issue #1233](https://github.com/openziti/edge/issues/1233) - edge-routers ref link in identities endpoint is incorrectly keyed
    * [Issue #1234](https://github.com/openziti/edge/issues/1234) - identities missing service-config link ref
    * [Issue #1232](https://github.com/openziti/edge/issues/1232) - edge management api identity-types endpoint produces incorrect links

* github.com/openziti/fabric: [v0.21.17 -> v0.21.36](https://github.com/openziti/fabric/compare/v0.21.17...v0.21.36)
    * [Issue #525](https://github.com/openziti/fabric/issues/525) - Update metrics message propagation from router to controller for HA

* github.com/openziti/foundation/v2: [v2.0.7 -> v2.0.9](https://github.com/openziti/foundation/compare/v2.0.7...v2.0.9)
* github.com/openziti/identity: [v1.0.20 -> v1.0.28](https://github.com/openziti/identity/compare/v1.0.20...v1.0.28)
* github.com/openziti/runzmd: [v1.0.3 -> v1.0.7](https://github.com/openziti/runzmd/compare/v1.0.3...v1.0.7)
* github.com/openziti/sdk-golang: [v0.16.146 -> v0.18.19](https://github.com/openziti/sdk-golang/compare/v0.16.146...v0.18.19)
* github.com/openziti/storage: [v0.1.26 -> v0.1.30](https://github.com/openziti/storage/compare/v0.1.26...v0.1.30)
* github.com/openziti/transport/v2: [v2.0.38 -> v2.0.48](https://github.com/openziti/transport/compare/v2.0.38...v2.0.48)
* github.com/openziti/metrics: [v1.1.5 -> v1.2.2](https://github.com/openziti/metrics/compare/v1.1.5...v1.2.2)
* github.com/openziti/ziti: [v0.26.11 -> v0.26.12](https://github.com/openziti/ziti/compare/v0.26.11...v0.26.12)
    * [Issue #892](https://github.com/openziti/ziti/issues/892) - Add timeout to ziti agent controller snapshot-db command
    * [Issue #917](https://github.com/openziti/ziti/issues/917) - ZITI_BIN_ROOT is incorrect in docker env
    * [Issue #912](https://github.com/openziti/ziti/issues/912) - Binaries not updated in docker-compose env with new image
    * [Issue #897](https://github.com/openziti/ziti/issues/897) - Add CLI options to manage  /edge/v1/transit-routers
    * [Issue #706](https://github.com/openziti/ziti/issues/706) - Add port check to quickstart

# Older Changelogs

Changelogs for previous releases can be found in [changelogs](./changelogs).
