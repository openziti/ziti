# OpenZiti HA

This document gives a brief overview of how OpenZiti HA works and how it differs from running
OpenZiti in non-HA mode.

## Distributed Model

The OpenZiti controller uses RAFT to distribute the data model. Specifically it uses the
[HashiCorp Raft Library](https://github.com/hashicorp/raft/).

### Updates

The basic flow for model updates is as follows:

1. A client requests a model update via the REST API.
2. The controller checks if it is the raft cluster leader. If it is not, it forwards the request to the leader.
3. Once the request is on the leader, it applies the model update to the raft log. This involves getting a quorum of the controllers to accept the update.
4. One the update has been accepted, it will be executed on each node of the cluster. This will generate create one or more changes to the bolt database.
5. The results of the operation (success or failure) are returned to the controller which received the original REST request.
6. The result is returned to the REST client.

### Reads

Reads are always done to the local bolt database for performance. The assumption is that if something
like a policy change is delayed, it may temporarily allow a circuit to be created, but as soon as
the policy update is applied, it will make changes to circuits as necessary.

### System of Record

In controller that's not configured for HA, the bolt database is the system of record. In an HA
setup, the raft journal is the system of record. The raft journal is stored in two places,
a snapshot directory and a bolt database of raft journal entries.

So a non-HA setup will have:

* ctrl.db

An HA setup will have:

* raft.db - the bolt database containing raft journal entries
* snapshots/ - a directory containing raft snapshots. Each snapshot is snapshot of the controller bolt db
* ctrl.db - the controller bolt db, with the current state of the model

When an HA controller starts up, it will first apply the newest snapshot, then any newer journal entries
that aren't yet contained in a snapshot. This means that an HA controller should start with a
blank DB that can be overwritten by snapshot and/or have journal entries applied to it. It is for this
reason that the controller

##       
