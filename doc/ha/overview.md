# OpenZiti HA

This document gives a brief overview of how OpenZiti HA works and how it differs from running
OpenZiti in non-HA mode.

To set up a developer HA network see the [HA Developer Setup Guide](./dev-setup.md).

## Operational Considerations

### System of Record

In controller that's not configured for HA, the bolt database is the system of record. In an HA
setup, the raft journal is the system of record. The raft journal is stored in two places, a
snapshot directory and a bolt database of raft journal entries.

So a non-HA setup will have:

* ctrl.db

An HA setup will have:

* raft.db - the bolt database containing raft journal entries
* snapshots/ - a directory containing raft snapshots. Each snapshot is snapshot of the controller
  bolt db
* ctrl.db - the controller bolt db, with the current state of the model

The location of all three is controlled by the raft/dataDir config property.

```yaml
raft:
  dataDir: /var/ziti/data/
```

When an HA controller starts up, it will first apply the newest snapshot, then any newer journal
entries that aren't yet contained in a snapshot. This means that an HA controller should start with
a blank DB that can be overwritten by snapshot and/or have journal entries applied to it. So an HA
controller will delete or rename the existing controller database and start with a fresh bolt db.

### Bootstrapping

#### Cluster Initialization

Initial cluster setup can be configured either in the config file or via agent commands.

The controller will not fully start until the raft cluster has bootstrapped. The minimum number of
cluster members is set in the config file. Recommended cluster sizes are 3, 5 or 7. A cluster size
of 1 is mostly useful for testing and development.

**Config File Example**

```yaml
raft:
  dataDir: /var/ziti/data/
  minClusterSize: 3
  bootstrapMembers:
    - tls:192.168.1.100
    - tls:192.168.1.101
```

Note that `bootstrapMembers` can only be used when setting the cluster up for the first time and
should only be used on a single node. It cannot be used later to add additional nodes to an existing
cluster.

**Agent Commands**

There are now three new agent commands specific to the controller.

**Adding Members**

```shell
# Adding Members
ziti agent cluster add <other controller raft address>

# Listing Members
ziti agent cluster list

# Removing Members
ziti agent cluster remove <controller id>

# Transfer Leadership
ziti agent cluster transfer-leadership [new leader id]
```

#### Edge Admin Initialization

Because RAFT is now the system of record, the previous pattern for configuring the default admin
won't work. In a non-HA system you initialized the controller raft DB with an admin user directly.
If you do this with an HA system, the changes you made directly to the DB will be lost and replaced
by whatever is in raft. To initialize an HA controller cluster use the new agent command.

```shell
ziti agent controller init <admin username> <admin password> <admin name>
```

The controller will not fully start until the edge admin has been initialized.

### Snapshot Application and Restarts

If a controller receives a snapshot to apply after starting up, it will apply the snapshot and then
terminate. This assumes that there is a restart script which will bring the controller back up after
it terminates.

This should only happen if a controller is connected to the cluster and then gets disconnected for
long enough that a snapshot is created while it's disconnected. Because applying a snapshot requires
replacing the underlying controller bolt DB, the easiest way to do that is restart. That way we
don't have to worry about replacing the bolt DB underneath a running system.

### Metrics

In an HA system, routers will send metrics to all controllers to which they are connected. There is
a new `doNotPropagate` flag in the metrics message, which will be set to false until the router has
successfully delivered the metrics message to a controller. The flag will then be set to true. So
the first controller to get the metrics message is expected to deliver the metrics message to the
events system for external integrators. The other controllers will have `doNotPropage` set to true,
and will only use the metrics message internally, to update routing data.

### Certificates

There are many ways to set up certificates, so this will just cover a recommended configuration.

The primary thing to ensure is that controllers have a shared root of trust. A configuration that
works would be as follows:

1. Create a self-signed root CA
2. Create an intermediate signing cert for each controller
3. Create a server cert using the signing cert for each controller
4. Make sure that the CA bundle for each server includes both the root CA and the intermediate CA
   for that server

Note that controller server certs must contain a SPIFFE id of the form

```
spiffe://<trust domain>/controller/<controller id>
```

So if your trust domain is `example.com` and your controller id is `ctrl1`, then your SPIFFE id
would be:

```
spiffe://example.com/controller/ctrl1
```

**SPIFFE ID Notes:**

* This ID must be set as the only URI in the `X509v3 Subject Alternative Name` field in the
  certificate.
* These IDs are used to allow the controllers to identify each during the mTLS negotiation.
* The OpenZiti CLI supports creating SPIFFE IDs in your certs
    * Use the `--trust-domain` flag when creating CAs
    * Use the `--spiffe-id` flag when creating server or client certificates

See [Developer Setup](./dev-setup.md) for the commands to make this happen and a bit more discussion
of certs.

### Open Ports

Controllers now establish connections with each other, for two purposes.

1. Forwarding model updates to the leader, so they can be applied to the raft cluster
2. raft communication

Both kinds of traffic flow over the same connection.

These connections do not require any extra open ports as we are using the control channel listener
to listen to both router and controller connections. As part of the connection process the
connection type is provided and the appropriate authentication and connection setup happens based on
the connection type. If no connection type is provided, it's assumed to be a router.

## Distributed Model

When looking at how to make the OpenZiti controller model distributed, we first looked at what
characteristics we needed for the model data.

Model data is the information the controller needs to figure out what it can do. This includes
things like:

* Services
* Routers
* Terminator
* Identities
* Policies
* Configs

### Model Data Characteristics

* All data required on every controller
* Read characteristics
    * Reads happen all the time, from every client and as well as admins
    * Speed is very important. They affect how every client perceives the system.
    * Availability is very important. Without reading definitions, can’t create new connections
    * Can be against stale data, if we get consistency within a reasonable timeframe (seconds to
      minutes)
* Write characteristics
    * Writes only happen from administrators
    * Speed needs to be reasonable, but doesn't need to be blazing fast
    * Write availability can be interrupted, since it primarily affects management operations
    * Must be consistent. Write validation can’t happen with stale data. Don’t want to have to deal
      with reconciling concurrent, contradictory write operations.
* Generally involves controller to controller coordination

Of the distribution mechanisms we looked at, RAFT had the best fit.

### RAFT Characteristics

* Writes
    * Consistency over availability
    * Good but not stellar performance
* Reads
    * Every node has full state
    * Local state is always internally consistent, but maybe slightly behind the leader
    * No coordination required for reads
    * Fast reads
    * Reads work even when other nodes are unavailable
    * If latest data is desired, reads can be forwarded to the current leader

So the OpenZiti controller uses RAFT to distribute the data model. Specifically it uses the
[HashiCorp Raft Library](https://github.com/hashicorp/raft/).

### Updates

The basic flow for model updates is as follows:

1. A client requests a model update via the REST API.
2. The controller checks if it is the raft cluster leader. If it is not, it forwards the request to
   the leader.
3. Once the request is on the leader, it applies the model update to the raft log. This involves
   getting a quorum of the controllers to accept the update.
4. One the update has been accepted, it will be executed on each node of the cluster. This will
   generate create one or more changes to the bolt database.
5. The results of the operation (success or failure) are returned to the controller which received
   the original REST request.
6. The controller waits until the operation has been applied locally.
7. The result is returned to the REST client.

### Reads

Reads are always done to the local bolt database for performance. The assumption is that if
something like a policy change is delayed, it may temporarily allow a circuit to be created, but as
soon as the policy update is applied, it will make changes to circuits as necessary.

## Runtime Data

In addition to model data, the controller also manages some amount of runtime data. This data is for
running OpenZiti's core functions, i.e. managing the flow of data across the mesh, along with
related authentication data. So this includes things like:

* Links
* Circuits
* API Sessions
* Sessions
* Posture Data

### Runtime Data Characteristics

Runtime data has different characteristics than the model data does.

* Not necessarily shared across controllers
* Reads **and** writes must be very fast
* Generally involves sdk to controller or controller to router coordination

Because writes must also be fast, RAFT is not a good candidate for storing this data. Good
performance is critical for these components, so they are each evaluated individually.

### Links

Each controller currently needs to know about links so that it can make routing decisions. However,
links exist on routers. So, routers are the source of record for links. When a router connects to a
controller, the router will tell the controller about any links that it already has. The controller
will ask to fill in any missing links and the controller will ensure that it doesn't create
duplicate links if multiple controllers request the same link be created. If there are duplicates,
the router will inform the controller of the existing link.

The allows the routers to properly handle link dials from multiple routers and keep controllers up
to date with the current known links.

### Circuits

Circuits were and continue to be stored in memory for both standalone and HA mode
controllers.Circuits are not distributed. Rather, each controller remains responsible for any
circuits that it created.

When a router needs to initiate circuit creation it will pick the one with the lowest response time
and send a circuit creation request to that router. The controller will establish a route. Route
tables as well as the xgress endpoints now track which controller is responsible for the associated
circuit. This way when failures or other notifications need to be sent, the router knows which
controller to talk to.

This gets routing working with multiple controllers without a major refactor. Future work will
likely delegate more routing control to the routers, so routing should get more robust and
distributed over time.

### Api Sessions, Sessions, Posture Data

API Sessions and Sessions are moving to bearer tokens. Posture Data is TBD.
