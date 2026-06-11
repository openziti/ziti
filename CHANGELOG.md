# Release 2.1.0

## What's New

* [Cluster Quorum Recover](#cluster_quorum_recovery) - A mechanism for recovering clusters that have irrevocably lost the ability to form a quorum
* [Fully Connected Controller Mesh](#fully-connected-controller-mesh) - Controllers now proactively keep the cluster mesh fully connected
* [Config Type Target Field](#config-type-target-field) - Config types now have a target field indicating whether they apply to services, routers or other entities
* [Wildcard OIDC Issuers](#wildcard-oidc-issuers) - Controllers with a wildcard server-certificate SAN can serve OIDC for explicitly allow-listed hostnames
* [Router Configs](#router-configs) - Allow routers to have a list of associated configs

## Cluster Quorum Recovery

A new offline CLI command, `ziti ops cluster recover <controller-config>`, lets
operators rebuild a stuck HA controller cluster after losing quorum. Use it when
enough controllers are permanently gone that `ziti ops cluster add` and
`ziti ops cluster remove` fail with "no leader" — for example, a 2-node cluster
where one node is unrecoverable, or a 3-node cluster that lost two nodes at once.

The command must be run while the surviving controller process is stopped. It
reads the same controller config the controller would, opens the raft data
directory, calls `raft.RecoverCluster` to force the configuration down to a
single local node, and aligns the FSM-tracked member list and snapshot data so
no stale peers leak through on restart. After it succeeds, restart the
controller and add new peers normally with `ziti ops cluster add`.

### End-to-End Encryption (e2ee) Improvements

* Add support for negotiating e2ee scheme during Dial/Accept handshake
* Allow hosting-side crypto material to be generated on per connection basis (instead of per terminator)


## Fully Connected Controller Mesh

In an HA cluster, controllers form a mesh of channel connections that raft uses to
communicate. Previously we followed the raft library's lead: connections were made as
needed to allow elections, and after that only the leader maintained connections to its
followers. That is enough for raft itself, but it means most nodes have no direct link to
most other nodes.

Now controllers keep the mesh fully connected. This gives better visibility into system
state from any node, not just the leader, and it provides a baseline for building
additional non-raft coordination features on top of the mesh.

Each controller runs a `PeerDialer` that proactively dials every known cluster member and
works to keep the mesh fully connected. Failed dials are retried with exponential backoff,
and when two controllers dial each other at the same time a deterministic tie-break (based
on their SPIFFE IDs) keeps a single connection rather than a redundant pair. The dialer's
state can be inspected with `ziti fabric inspect ctrl-peer-dialer`.

The dialer is tunable via a new optional `cluster.dialer` config section. All values have
defaults, so no configuration change is required:

```yaml
cluster:
  dialer:
    minRetryInterval: 1s     # minimum time between dial retries
    maxRetryInterval: 1m     # maximum time between dial retries
    retryBackoffFactor: 2.0  # multiplier applied to the retry interval after each failure
    fastFailureWindow: 30s   # if a connection is lost within this window, apply backoff instead of resetting the retry delay
    dialTimeout: 10s         # maximum time a single dial attempt may run
    scanInterval: 30s        # period of the full scan that reconciles dial state against membership
    queueCheckInterval: 5s   # how often expired entries are popped from the retry queue
```

A related `cluster.nonMemberGrace` setting (default `1m`) controls how long a leader will
let a TLS-valid but non-member controller stay connected to the mesh before dropping it.
This gives a controller that is being added to the cluster time to be accepted as a member
before its connection is reaped.

## Config Type Target Field

Config types now have an optional `target` field that indicates what kind of entity the config type
is intended for. Valid values are `"service"`, `"router"`, and `"other"`. The field is set on creation
and is immutable afterward.

This is the first step toward controller-managed router configuration. The `target` field lets us
distinguish between config types meant for services, config types meant for routers, and config types
meant for other purposes, which keeps UIs, APIs, and validation clean. See
`doc/design/ctrl-managed-router-config.md` for the full design.

A database migration sets `target = "service"` on all existing config types. Services and identity
service config overrides now require that referenced configs have a config type with
`target = "service"`.

The CLI has been updated to support the new field:

* `ziti edge create config-type` now accepts a `--target` flag
* `ziti edge list config-types` now shows a `Target` column

## Wildcard OIDC Issuers

Controllers can serve OIDC for hostnames covered by a wildcard server-certificate SAN. Prior to 2.1,
wildcard SANs were excluded from the set of valid OIDC issuers, so `/oidc/*` requests to a
wildcard-covered hostname returned `404`.

Wildcard SANs cannot be used as a literal OIDC issuer (`https://*.example.com/oidc` is not a usable URL).
Instead, the `edge-oidc` API binding now accepts an `allowedHostnames` option listing the exact hostnames
(covered by the wildcard) that may be served as issuers. If omitted, the wildcard contributes no issuers
and the controller logs a warning at startup; a malformed entry (a non-string value, or one containing a
wildcard character) is a startup error:

```yaml
web:
  - name: client-management
    apis:
      - binding: edge-oidc
        options:
          allowedHostnames:
            - ctrl.example.com
```

Each entry must be an exact hostname (no patterns) that an active server-certificate SAN actually covers;
entries are matched against wildcard SANs using standard X.509 hostname rules. The resulting OIDC issuers
are therefore concrete, fixed hostnames, so the set of valid `iss` values stays closed. Concrete
(non-wildcard) SANs continue to be served as issuers automatically and do not need to be listed.

## Router Configs

Routers (edge, transit, and fabric) now have a `configs` field that holds a list of config IDs the
router should use. This is the second step toward controller-managed router configuration: routers
can now be associated with configs in the same way services already can.

Validation rules:

* Every config referenced by a router must use a config type with `target = "router"`. Configs
  with `target = "service"` (or anything else) are rejected.
* A router may reference at most one config per config type. Attempting to attach two configs of
  the same type is rejected with a duplicate-config error naming both configs.
* Deleting a config automatically removes it from the `configs` list of any router that referenced
  it, so dangling references are not possible.

The `configs` field is exposed on router create, update, patch, and detail responses across the
edge, transit, and fabric router REST APIs.

The CLI has been updated to support the new field:

* `ziti edge create edge-router` accepts `--config <id>` (repeatable)
* `ziti edge create transit-router` accepts `--config <id>` (repeatable)
* `ziti edge update edge-router` accepts `--config <id>` to replace the router's config list
* `ziti fabric create router` accepts `--config <id>` (repeatable)
* `ziti fabric update router` accepts `--config <id>` to replace the router's config list

## Component Updates and Bug Fixes

* github.com/openziti/foundation/v2: [v2.0.91 -> v2.0.92](https://github.com/openziti/foundation/compare/v2.0.91...v2.0.92)
* github.com/openziti/identity: [v1.0.129 -> v1.0.130](https://github.com/openziti/identity/compare/v1.0.129...v1.0.130)
* github.com/openziti/sdk-golang: [v1.7.0 -> v1.8.0](https://github.com/openziti/sdk-golang/compare/v1.7.0...v1.8.0)
    * [Issue #927](https://github.com/openziti/sdk-golang/issues/927) - Apply exponential backoff to auth retry attempts
    * [Issue #926](https://github.com/openziti/sdk-golang/issues/926) - Refresh OIDC token using a window to avoid race conditions and herding
    * [Issue #925](https://github.com/openziti/sdk-golang/issues/925) - Switch controllers on a broader set of errors
    * [Issue #924](https://github.com/openziti/sdk-golang/issues/924) - Make controller http timeout configurable, with a default of 30s
    * [Issue #932](https://github.com/openziti/sdk-golang/issues/932) - API Session Certificate chain is not preserved

* github.com/openziti/ziti/v2: [v2.0.0 -> v2.1.0](https://github.com/openziti/ziti/compare/v2.0.0...v2.1.0)
    * [Issue #1593](https://github.com/openziti/ziti/issues/1593) - Expanded attribute query support in management API; add policy attribute support and usage count
    * [Issue #3867](https://github.com/openziti/ziti/issues/3867) - Tunneler skips iptables rules for services sharing an intercept hostname
    * [Issue #3949](https://github.com/openziti/ziti/issues/3949) - DeleteById swallows errors when firing change events
    * [Issue #3945](https://github.com/openziti/ziti/issues/3945) - Increase certificate serial number namespace to 159 bits
    * [Issue #3942](https://github.com/openziti/ziti/issues/3942) - Prep for channel v5: bind handler invocation, send priorities
    * [Issue #3938](https://github.com/openziti/ziti/issues/3938) - Carry the link id in a link header instead of only in the channel identity token
    * [Issue #3914](https://github.com/openziti/ziti/issues/3914) - ziti login fails with oidc + wildcard certs
    * [Issue #3891](https://github.com/openziti/ziti/issues/3891) - oidc auth fails with wildcard server-cert SANs
    * [Issue #3744](https://github.com/openziti/ziti/issues/3744) - Add a target field to config type
    * [Issue #3684](https://github.com/openziti/ziti/issues/3684) - Keep controller mesh fully connected, as much as possible
    * [Issue #3849](https://github.com/openziti/ziti/issues/3849) - Add a recover mechanism for when a controller cluster can't form a quorum


