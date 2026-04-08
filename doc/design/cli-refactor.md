# Ziti CLI Consolidation

## The Problem

Working with Ziti resources today means knowing which sub-tree a command lives under. Want to create an identity? That's `ziti edge create identity`. A terminator? `ziti fabric create terminator`. Listing services requires `ziti edge list services`, but listing circuits requires `ziti fabric list circuits`. Logging in is `ziti edge login`.

For someone already familiar with the split, this is fine. For everyone else, the first question is always: "is this an edge thing or a fabric thing?" That's not a question users should need to answer just to list what's in their network.

## What Changed

The most commonly used commands from `ziti edge` and `ziti fabric` are now consolidated at the top level:

- `ziti create` - create identities, services, routers, terminators, policies, etc.
- `ziti delete` - delete them
- `ziti list` (aliased as `ziti ls`) - list them
- `ziti update` - update them
- `ziti login` - log into a controller
  - `ziti login forget` - forget stored credentials (was `ziti edge logout`)
  - `ziti login use` - switch active identity/profile (was `ziti edge use`)

For entities that exist in both edge and fabric (routers, services), the edge commands are the default since that's what most users interact with. The fabric-only variants are still there as `fabric-router` and `fabric-service`, just hidden from help output.

`ziti edge` and `ziti fabric` still work exactly as before. Nothing is removed or deprecated. The new top-level commands are a shortcut, not a replacement.

## How Merging Works

Both `ziti edge` and `ziti fabric` contribute subcommands to each top-level CRUD command. When both have a command with the same name (like `create service`), the edge version wins and the fabric version is available under a prefixed name (`create fabric-service`).

The one exception: `ziti create terminator` is the *fabric* terminator command, since the edge terminator command is excluded from the consolidated tree (the fabric version is the one people actually use directly).

## CLI Configuration

A hidden `ziti cli` command manages CLI configuration:

- `ziti cli set layout <1|2>` - switch between layout 1 (default) and layout 2 (experimental)
- `ziti cli get layout` - show current layout
- `ziti cli set alias <name> <target>` - create command aliases
- `ziti cli get alias` - list aliases
- `ziti cli remove alias <name>` - remove an alias

Aliases let you define shortcuts. For example, `ziti cli set alias ag "ops agent"` makes `ziti ag status` expand to `ziti ops agent status`.

The layout can also be set via the `ZITI_CLI_LAYOUT` environment variable (`1` or `2`), which takes precedence over the config file.

## Layout 2 (Experimental)

Layout 2 is an experimental reorganization of the CLI. It is not finalized and will likely change. You can try it with `ZITI_CLI_LAYOUT=2` or `ziti cli set layout 2`.

Beyond the consolidated CRUD and login commands (which are the same in both layouts), layout 2 reorganizes operational and setup commands into a cleaner hierarchy.

### Setup

`ziti setup` groups bootstrapping commands:

- `ziti setup controller config` - create a controller config file
- `ziti setup controller database` - initialize the controller database (was `ziti controller edge init`)
- `ziti setup router config` - create a router config file
- `ziti setup environment` - generate environment variables
- `ziti setup pki` - create PKI artifacts: ca, intermediate, server, client, key, csr

### Operations

`ziti ops` consolidates operational utilities that were previously scattered:

- `ziti ops agent` - IPC agent commands (was top-level `ziti agent`)
- `ziti ops cluster` - HA cluster management
- `ziti ops db` - database utilities (consolidated from multiple locations)
- `ziti ops inspect` - inspection commands (was `ziti fabric inspect`)
- `ziti ops stream` - event/trace streaming (was `ziti fabric stream`)
- `ziti ops trace` - identity tracing (was `ziti edge trace`)
- `ziti ops validate` - validation commands (consolidated from edge/fabric)
- `ziti ops tools` - miscellaneous utilities
  - `completion` - shell completion scripts
  - `le` - Let's Encrypt commands
  - `log-format` - log formatting
  - `unwrap` - unwrap identity files

### Verification

`ziti verify` consolidates verification tools:

- `ca` - verify a CA
- `ext-jwt-signer` - test an external JWT signer
- `network` - verify overlay configuration
- `policy` - verify policies between identities and services (was `ziti edge policy-advisor`)
- `traceroute` - run traceroute on a service
- `traffic` - verify traffic flow

### Enrollment

`ziti enroll` uses cleaner naming:

- `ziti enroll identity` - enroll an identity
- `ziti enroll router` - enroll a router (was `edge-router`)
- `ziti enroll reenroll-router` - re-enroll a router

### Get

`ziti get` surfaces a few read-only lookups:

- `ziti get config` - display a config definition
- `ziti get config-type` - display a config type schema
- `ziti get controller-version` - show controller version (was `ziti edge version`)

### Backward Compatibility

Every V1 command path still works in layout 2 as a hidden command. No warnings are printed. These include:

- `ziti edge *` and `ziti fabric *` - the full V1 command trees
- `ziti create client/server/intermediate/csr` - PKI commands (also `ziti setup pki *`)
- `ziti create config controller/router/environment` - config generators (also `ziti setup *`)
- `ziti completion` - shell completion (also `ziti ops tools completion`)
- `ziti enroll edge-router` - router enrollment (also `ziti enroll router`)
- `ziti ops log-format` / `ziti ops unwrap` - utilities (also `ziti ops tools *`)
- `ziti ops verify` - verification (also top-level `ziti verify`)

Hidden power-user aliases at the root level:

- `ziti agent` - shortcut for `ziti ops agent`
- `ziti log-format` - shortcut for `ziti ops tools log-format`

One known exception: `ziti create ca` is the edge CA create command in both layouts. The PKI CA command lives at `ziti pki create ca` (or `ziti setup pki ca` in layout 2).

---

## Command Tree (Layout 2)

The tree below omits the hidden `ziti edge *` and `ziti fabric *` backward-compatible paths for brevity. They mirror the full V1 tree. Run `ziti command-tree` to see everything.

```
ziti agent  (hidden)
ziti agent clear-channel-log-level
ziti agent cluster
ziti agent cluster add
ziti agent cluster init
ziti agent cluster list
ziti agent cluster remove
ziti agent cluster restore-from-db
ziti agent cluster transfer-leadership
ziti agent controller
ziti agent controller snapshot-db
ziti agent dump-heap
ziti agent gc
ziti agent goversion
ziti agent list
ziti agent memstats
ziti agent pprof-cpu
ziti agent pprof-heap
ziti agent ps
ziti agent router
ziti agent router decommission
ziti agent router dequiesce  (hidden)
ziti agent router disconnect
ziti agent router dump-api-sessions
ziti agent router dump-links
ziti agent router dump-routes
ziti agent router forget-link
ziti agent router quiesce  (hidden)
ziti agent router reconnect
ziti agent router route
ziti agent router unroute
ziti agent set-channel-log-level
ziti agent set-log-level
ziti agent setgc
ziti agent stack
ziti agent stats
ziti agent trace
ziti agent tunnel
ziti agent tunnel dump-sdk
ziti art  (hidden)
ziti cli  (hidden)
ziti cli get
ziti cli get alias
ziti cli get layout
ziti cli remove
ziti cli remove alias
ziti cli set
ziti cli set alias
ziti cli set layout
ziti command-tree  (hidden)
ziti completion  (hidden)
ziti controller  (hidden)
ziti controller delete-sessions
ziti controller delete-sessions-from-db
ziti controller edge
ziti controller edge init
ziti controller run
ziti controller version  (hidden)
ziti create
ziti create auth-policy
ziti create authenticator
ziti create authenticator updb
ziti create ca
ziti create client  (hidden)
ziti create config
ziti create config controller  (hidden)
ziti create config environment  (hidden)
ziti create config router  (hidden)
ziti create config router edge
ziti create config router fabric
ziti create config-type
ziti create csr  (hidden)
ziti create edge-router
ziti create edge-router-policy
ziti create enrollment
ziti create enrollment ott
ziti create enrollment ottca
ziti create enrollment updb
ziti create ext-jwt-signer
ziti create fabric-router  (hidden)
ziti create fabric-service  (hidden)
ziti create identity
ziti create intermediate  (hidden)
ziti create posture-check
ziti create posture-check domain
ziti create posture-check mac
ziti create posture-check mfa
ziti create posture-check os
ziti create posture-check process
ziti create posture-check process-multi
ziti create server  (hidden)
ziti create service
ziti create service-edge-router-policy
ziti create service-policy
ziti create terminator
ziti create transit-router
ziti delete
ziti delete api-session
ziti delete api-session where
ziti delete auth-policy
ziti delete auth-policy where
ziti delete authenticator
ziti delete authenticator where
ziti delete ca
ziti delete ca where
ziti delete circuit
ziti delete circuit where
ziti delete config
ziti delete config where
ziti delete config-type
ziti delete config-type where
ziti delete edge-router
ziti delete edge-router where
ziti delete edge-router-policy
ziti delete edge-router-policy where
ziti delete enrollment
ziti delete enrollment where
ziti delete external-jwt-signer
ziti delete external-jwt-signer where
ziti delete fabric-router  (hidden)
ziti delete fabric-router where
ziti delete fabric-service  (hidden)
ziti delete fabric-service where
ziti delete identity
ziti delete identity where
ziti delete link
ziti delete link where
ziti delete posture-check
ziti delete posture-check where
ziti delete service
ziti delete service where
ziti delete service-edge-router-policy
ziti delete service-edge-router-policy where
ziti delete service-policy
ziti delete service-policy where
ziti delete session
ziti delete session where
ziti delete terminator
ziti delete terminator where
ziti delete transit-router
ziti delete transit-router where
ziti demo
ziti demo agent
ziti demo agent clear-channel-log-level
ziti demo agent cluster
ziti demo agent cluster add
ziti demo agent cluster init
ziti demo agent cluster list
ziti demo agent cluster remove
ziti demo agent cluster restore-from-db
ziti demo agent cluster transfer-leadership
ziti demo agent controller
ziti demo agent controller snapshot-db
ziti demo agent dump-heap
ziti demo agent echo-server
ziti demo agent echo-server update-terminator
ziti demo agent gc
ziti demo agent goversion
ziti demo agent list
ziti demo agent memstats
ziti demo agent pprof-cpu
ziti demo agent pprof-heap
ziti demo agent ps
ziti demo agent router
ziti demo agent router decommission
ziti demo agent router dequiesce  (hidden)
ziti demo agent router disconnect
ziti demo agent router dump-api-sessions
ziti demo agent router dump-links
ziti demo agent router dump-routes
ziti demo agent router forget-link
ziti demo agent router quiesce  (hidden)
ziti demo agent router reconnect
ziti demo agent router route
ziti demo agent router unroute
ziti demo agent set-channel-log-level
ziti demo agent set-log-level
ziti demo agent setgc
ziti demo agent stack
ziti demo agent stats
ziti demo agent trace
ziti demo agent tunnel
ziti demo agent tunnel dump-sdk
ziti demo echo-server
ziti demo first-service
ziti demo plain-echo-client
ziti demo plain-echo-server
ziti demo setup
ziti demo setup echo
ziti demo setup echo client
ziti demo setup echo multi-router-tunneler-hosted
ziti demo setup echo multi-sdk-hosted
ziti demo setup echo multi-tunneler-hosted
ziti demo setup echo router-tunneler-both-sides
ziti demo setup echo single-router-tunneler-hosted
ziti demo setup echo single-sdk-hosted
ziti demo setup echo update-config-addressable
ziti demo setup echo update-config-ha
ziti demo zcat
ziti demo zcatCloseTest  (hidden)
ziti demo ziti-echo-client
ziti demo ziti-echo-server
ziti dump-cli  (hidden)
ziti enroll
ziti enroll edge-router  (hidden)
ziti enroll identity
ziti enroll reenroll-router
ziti enroll router
ziti gendoc  (hidden)
ziti get
ziti get config
ziti get config-type
ziti get controller-version
ziti help
ziti list
ziti list api-sessions
ziti list auth-policies
ziti list authenticators
ziti list cas
ziti list circuits
ziti list config
ziti list config services
ziti list config-type
ziti list config-type configs
ziti list config-types
ziti list configs
ziti list controllers
ziti list edge-router
ziti list edge-router edge-router-policies
ziti list edge-router identities
ziti list edge-router service-edge-router-policies
ziti list edge-router services
ziti list edge-router-policies
ziti list edge-router-policy
ziti list edge-router-policy edge-routers
ziti list edge-router-policy identities
ziti list edge-router-role-attributes
ziti list edge-routers
ziti list enrollments
ziti list ext-jwt-signers
ziti list fabric-routers  (hidden)
ziti list fabric-services  (hidden)
ziti list identities
ziti list identity
ziti list identity edge-router-policies
ziti list identity edge-routers
ziti list identity service-configs
ziti list identity service-policies
ziti list identity services
ziti list identity-role-attributes
ziti list links
ziti list network-jwts
ziti list posture-check-role-attributes
ziti list posture-check-types
ziti list posture-checks
ziti list service
ziti list service configs
ziti list service edge-routers
ziti list service identities
ziti list service service-edge-router-policies
ziti list service service-policies
ziti list service terminators
ziti list service-edge-router-policies
ziti list service-edge-router-policy
ziti list service-edge-router-policy edge-routers
ziti list service-edge-router-policy services
ziti list service-policies
ziti list service-policy
ziti list service-policy identities
ziti list service-policy posture-checks
ziti list service-policy services
ziti list service-role-attributes
ziti list services
ziti list sessions
ziti list summary
ziti list terminators
ziti list transit-routers
ziti log-format  (hidden)
ziti login
ziti login forget
ziti login use
ziti ops
ziti ops agent
ziti ops agent clear-channel-log-level
ziti ops agent cluster
ziti ops agent cluster add
ziti ops agent cluster init
ziti ops agent cluster list
ziti ops agent cluster remove
ziti ops agent cluster restore-from-db
ziti ops agent cluster transfer-leadership
ziti ops agent controller
ziti ops agent controller snapshot-db
ziti ops agent dump-heap
ziti ops agent gc
ziti ops agent goversion
ziti ops agent list
ziti ops agent memstats
ziti ops agent pprof-cpu
ziti ops agent pprof-heap
ziti ops agent ps
ziti ops agent router
ziti ops agent router decommission
ziti ops agent router dequiesce  (hidden)
ziti ops agent router disconnect
ziti ops agent router dump-api-sessions
ziti ops agent router dump-links
ziti ops agent router dump-routes
ziti ops agent router forget-link
ziti ops agent router quiesce  (hidden)
ziti ops agent router reconnect
ziti ops agent router route
ziti ops agent router unroute
ziti ops agent set-channel-log-level
ziti ops agent set-log-level
ziti ops agent setgc
ziti ops agent stack
ziti ops agent stats
ziti ops agent trace
ziti ops agent tunnel
ziti ops agent tunnel dump-sdk
ziti ops cluster
ziti ops cluster add
ziti ops cluster list
ziti ops cluster remove
ziti ops cluster transfer-leadership
ziti ops db
ziti ops db add-debug-admin
ziti ops db anonymize
ziti ops db check-integrity-status
ziti ops db compact
ziti ops db delete-sessions-from-db
ziti ops db du
ziti ops db explore
ziti ops db get-db-version
ziti ops db set-db-version  (hidden)
ziti ops db snapshot
ziti ops db start-check-integrity
ziti ops export  (hidden)
ziti ops import  (hidden)
ziti ops inspect
ziti ops inspect circuit
ziti ops inspect cluster-config
ziti ops inspect config
ziti ops inspect connected-peers
ziti ops inspect connected-routers
ziti ops inspect ctrl-dialer
ziti ops inspect data-model-index
ziti ops inspect ert-terminators
ziti ops inspect identity-connection-statuses
ziti ops inspect links
ziti ops inspect metrics
ziti ops inspect router-circuits
ziti ops inspect router-controllers
ziti ops inspect router-data-model
ziti ops inspect router-data-model-index
ziti ops inspect router-edge-circuits
ziti ops inspect router-messaging
ziti ops inspect router-sdk-circuits
ziti ops inspect sdk
ziti ops inspect sdk-terminators
ziti ops inspect stackdump
ziti ops inspect terminator-costs
ziti ops log-format  (hidden)
ziti ops stream
ziti ops stream events
ziti ops stream traces
ziti ops stream traces toggle
ziti ops stream traces toggle pipe
ziti ops tools
ziti ops tools completion
ziti ops tools le
ziti ops tools le create
ziti ops tools le list
ziti ops tools le renew
ziti ops tools le revoke
ziti ops tools log-format
ziti ops tools unwrap
ziti ops trace
ziti ops trace identity
ziti ops unwrap  (hidden)
ziti ops validate
ziti ops validate circuits
ziti ops validate identity-connection-statuses
ziti ops validate router-data-model
ziti ops validate router-ert-terminators
ziti ops validate router-links
ziti ops validate router-sdk-terminators
ziti ops validate service-hosting
ziti ops validate terminators
ziti ops verify  (hidden)
ziti ops verify ext-jwt-signer
ziti ops verify ext-jwt-signer oidc
ziti ops verify network
ziti ops verify traffic
ziti pki  (hidden)
ziti pki create
ziti pki create ca
ziti pki create client
ziti pki create csr
ziti pki create intermediate
ziti pki create key
ziti pki create server
ziti pki le
ziti pki le create
ziti pki le list
ziti pki le renew
ziti pki le revoke
ziti router  (hidden)
ziti router enroll
ziti router run
ziti router version  (hidden)
ziti run
ziti run controller
ziti run quickstart
ziti run quickstart join  (hidden)
ziti run router
ziti run tunnel  (hidden)
ziti run tunnel host
ziti run tunnel proxy
ziti run tunnel tproxy
ziti run tunnel version  (hidden)
ziti setup
ziti setup controller
ziti setup controller config
ziti setup controller database
ziti setup environment
ziti setup pki
ziti setup pki ca
ziti setup pki client
ziti setup pki csr
ziti setup pki intermediate
ziti setup pki key
ziti setup pki server
ziti setup router
ziti setup router config
ziti setup router config edge
ziti setup router config fabric
ziti tunnel  (hidden)
ziti tunnel host
ziti tunnel proxy
ziti tunnel run
ziti tunnel tproxy
ziti tunnel version  (hidden)
ziti update
ziti update auth-policy
ziti update authenticator
ziti update authenticator cert
ziti update authenticator updb
ziti update ca
ziti update config
ziti update config-type
ziti update edge-router
ziti update edge-router-policy
ziti update ext-jwt-signer
ziti update fabric-router  (hidden)
ziti update fabric-service  (hidden)
ziti update identity
ziti update identity-configs
ziti update link
ziti update posture-check
ziti update posture-check domain
ziti update posture-check mac
ziti update posture-check mfa
ziti update posture-check os
ziti update posture-check process
ziti update service
ziti update service-edge-router-policy
ziti update service-policy
ziti update terminator
ziti verify
ziti verify ca
ziti verify ext-jwt-signer
ziti verify ext-jwt-signer oidc
ziti verify network
ziti verify policy
ziti verify policy identities
ziti verify policy services
ziti verify traceroute
ziti verify traffic
ziti version  (hidden)
```
