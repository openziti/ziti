# Release 2.1.0

## What's New

* [Cluster Quorum Recover](#cluster_quorum_recovery) - A mechanism for recovering clusters that have irrevocably lost the ability to form a quorum

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


## Component Updates and Bug Fixes

* github.com/openziti/ziti/v2: [v2.0.0 -> v2.1.0](https://github.com/openziti/ziti/compare/v2.0.0...v2.1.0)
    * [Issue #3849](https://github.com/openziti/ziti/issues/3849) - Add a recover mechanism for when a controller cluster can't form a quorum


