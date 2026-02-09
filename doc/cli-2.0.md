# Ziti CLI 2.0 Layout

The V2 CLI layout reorganizes commands for better discoverability and consistency. V1 remains unchanged for backward compatibility. V2 is enabled via `ZITI_CLI_LAYOUT=2` environment variable or `"layout": 2` in the config file.

## Summary of Changes

### Top-Level CRUD Commands
Commands from `ziti edge` and `ziti fabric` are consolidated at the top level:
- `ziti create` - create entities (identities, services, routers, etc.)
- `ziti delete` - delete entities
- `ziti list` - list entities
- `ziti update` - update entities

Edge commands are the default for entities that exist in both edge and fabric. Fabric-only variants (`fabric-router`, `fabric-service`) are hidden.

### Session Management
- `ziti login` - log into a controller (was `ziti edge login`)
  - `ziti login forget` - forget stored credentials (was `ziti edge logout`)
  - `ziti login use` - switch active profile (was `ziti edge use`)

### Setup Commands
New `ziti setup` command for bootstrapping Ziti infrastructure:
- `ziti setup controller config` - create controller config file
- `ziti setup controller database` - initialize controller database (was `ziti controller edge init`)
- `ziti setup router config` - create router config file
- `ziti setup environment` - display/generate environment variables
- `ziti setup pki` - PKI creation (ca, intermediate, server, client, key, csr)

### Operations (`ziti ops`)
Consolidated operational utilities:
- `ziti ops agent` - IPC agent commands (was top-level `ziti agent`)
- `ziti ops cluster` - HA cluster management
- `ziti ops db` - database utilities (consolidated from multiple locations)
- `ziti ops inspect` - inspection commands (was `ziti fabric inspect`)
- `ziti ops stream` - event/trace streaming (was `ziti fabric stream`)
- `ziti ops trace` - identity tracing (was `ziti edge trace`)
- `ziti ops validate` - validation commands (consolidated from edge/fabric)
- `ziti ops tools` - utility tools
  - `completion` - shell completion scripts
  - `le` - Let's Encrypt commands
  - `log-format` - log formatting
  - `unwrap` - unwrap identity files

### Verification Commands
- `ziti verify` - consolidated verification tools
  - `ca` - verify a CA
  - `ext-jwt-signer` - test external JWT signer
  - `network` - verify overlay configuration
  - `policy` - verify policies (was `ziti edge policy-advisor`)
  - `traceroute` - run traceroute on a service
  - `traffic` - verify traffic flow

### Enrollment Commands
- `ziti enroll identity` - enroll an identity
- `ziti enroll router` - enroll a router (was `edge-router`)
- `ziti enroll reenroll-router` - re-enroll a router

### Get Commands
- `ziti get config` - display config definition
- `ziti get config-type` - display config type schema
- `ziti get controller-version` - show controller version (was `ziti edge version`)

### Deprecated V1 Aliases
All V1 command paths remain functional as hidden, deprecated aliases. Running any V1 path prints a deprecation notice directing users to the V2 equivalent. This preserves backward compatibility for existing scripts.

- `ziti edge *` - full V1 edge command tree (deprecated, use top-level CRUD/login/verify commands)
- `ziti fabric *` - full V1 fabric command tree (deprecated, use top-level CRUD and `ziti ops` commands)
- `ziti create client/server/intermediate/csr` - PKI commands (deprecated, use `ziti setup pki *`)
- `ziti create config controller/router/environment` - config file generators (deprecated, use `ziti setup *`)
- `ziti completion` - shell completion (deprecated, use `ziti ops tools completion`)
- `ziti enroll edge-router` - router enrollment (deprecated, use `ziti enroll router`)
- `ziti ops log-format` / `ziti ops unwrap` - utilities (deprecated, use `ziti ops tools *`)
- `ziti ops verify` - verification (deprecated, use `ziti verify`)

**Known exceptions:**
- `ziti create ca` is NOT aliased — it conflicts with the V2 edge CA create command at the same path.

### Hidden in V2
- `ziti pki` - hidden (moved to `ziti setup pki`)
- `ziti agent` - hidden (moved to `ziti ops agent`)

### CLI Configuration
- `ziti cli` - manage CLI configuration
  - `ziti cli set layout` - set CLI layout version (1 or 2)
  - `ziti cli get layout` - get current CLI layout version
  - `ziti cli set alias` - create command aliases
  - `ziti cli get alias` - list command aliases
  - `ziti cli remove alias` - remove command aliases

### Hidden Power-User Aliases
For convenience, these hidden aliases remain functional:
- `ziti agent` - alias for `ziti ops agent`
- `ziti log-format` - alias for `ziti ops tools log-format`

---

## Command Tree

The tree below includes all commands. Deprecated V1 aliases (`ziti edge *`, `ziti fabric *`, etc.) are omitted for brevity — they mirror the full V1 tree and are all marked `(hidden)`. Run `ziti command-tree` to see the complete tree including deprecated aliases.

```
ziti
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
ziti run quickstart ha  (hidden)
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
ziti get
ziti get config
ziti get config-type
ziti get controller-version
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
