# Upgrading to HA

## Controller Changes

### Updating Configuration

Add a `raft` stanza to the configuration. See the
[relevant configuration reference](https://openziti.io/docs/reference/configuration/controller#raft)
for information on other fields.

```yaml
raft:
  dataDir: /path/to/data/dir
```

The `dataDir` will be used to store the following:

* `ctrl-ha.db` - the ziti model bbolt database
* `raft.db` - the raft bbolt database
* `snapshots/` - a directory to store raft snapshots

### Importing The Datastore

There are two ways to initialize an HA cluster from an existing controller database.

1. Leave the `db` configuration in the config file. If this config settings is found, then when the
   raft cluster is bootstrapped it will initialize itself from that database
2. Use the `ziti agent controller init-from-db <path/to/db>` command. The path will be interpreted
   by the controller.

### Recommendations

When migrating an existing controller to HA, first get that controller running in a HA mode. Then,
when it's up and running, add additional nodes to the cluster using `ziti agent cluster add
<peer address>`. The means in the `raft` configuration section that `minClusterSize` can be omitted
or set to 1 and that `bootstrapMembers` can also be omitted

## Router Changes

### Updating Configuration

#### Controller Endpoint

When a router connects to a controller, it will receive an updated list of all the controllers in
the cluster. Should the cluster change while the router is connected, it will also receive an
updated list.

This means that the endpoints list can be set manually, this shouldn't be necessary. Note that while
the controller `endpoint` value can still be set, there's a newer `endpoints` value which allows
setting a list instead.

**Old Configuration**

```yaml
ctrl:
  endpoint: ctrl1.mycompany.com:443
```

**New Configuration**

```yaml
ctrl:
  endpoints:
    - ctrl1.mycompany.com:443
    - ctrl2.mycompany.com:443
```
