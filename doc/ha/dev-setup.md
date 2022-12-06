# HA Setup for Development

**NOTE: HA is a work in progress and not yet usable for anything other than developing HA**

To set up a local three node HA cluster, do the following.

## Create a CA

Create a self-signed certificate authority (CA) for the trust-root of your cluster

```
ziti pki create ca --trust-domain ha.test --pki-root ./pki --ca-file ca --ca-name 'HA Example Trust Root'
```

## Create Controller Certs

We are going to create an intermediate CA for each controller. We'll use this intermediate CA
for the following purposes:

1. To create a cert which will represent the controller. It will be used
    1. On the client side when dialing other controllers in the cluster
    2. On the server side when receiving connections from other controllers
    3. On the server side when receiving connections from routers
    4. On the server when handling REST API requests
2. To create identity certs as part of the identity enrollment process
3. To create router certs as part of the router enrollment process

### Notes

#### Client vs Server Certs

You may use separate certs and keys for client and server connections, but it's not necessary.
When you use a server cert on the client side it exposes information about what IPs and DNS entries
the cert is valid for, but since we're only connecting to other controllers, this should not be
a concern. However, the option to use separate certs is available, should you wish to use it.

#### REST Endpoint Certs

You may also use a different set of certs for the REST endpoint.

#### Sharing Signing Certs

You could use the same signing cert for all controllers in a cluster. However, if a signing
cert is ever compromised, all certs signed by the signing cert would need to be revoked. By
using a separate cert for each controller we limit the fallout from an individual controller
or cert being compromised.

### Create the Controller 1 signing and server certs

```shell
# Create the controller 1 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl1 --intermediate-name 'Controller One Signing Cert'

# Create the controller 1 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl1 --dns localhost --ip 127.0.0.1 --server-name ctrl1 --spiffe-id 'controller/ctrl1'

# Create the controller 2 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl2 --intermediate-name 'Controller Two Signing Cert'

# Create the controller 2 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl2 --dns localhost --ip 127.0.0.1 --server-name ctrl2 --spiffe-id 'controller/ctrl2'

# Create the controller 3 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl3 --intermediate-name 'Controller Three Signing Cert'

# Create the controller 3 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl3 --dns localhost --ip 127.0.0.1 --server-name ctrl3 --spiffe-id 'controller/ctrl3'
```

## Running the Controllers

1. The controller configuration files have relative paths, so make sure you're running things from this directory.
2. Start all three controllers
    1. `ziti controller run ctrl1.yml`
    2. `ziti controller run ctrl2.yml`
    3. `ziti controller run ctrl3.yml`
    4. All three are configured with `minClusterSize` of 3, so they will wait to be joined to a raft cluster
    5. The ctrl1.yml config file has the other two controllers as bootstrap members, so when it starts the first controller will start trying form the raft cluster.
3. Initialize the edge using the agent
    1. `ziti agent controller init <pid of any controller> admin admin 'Default Admin'`
    2. You can of course use different values if you desire

You should now have a three node cluster running. You can log into each controller individually.

1. `ziti edge login localhost:1280`
2. `ziti edge -i ctrl2 login localhost:1380`
3. `ziti edge -i ctrl3 login localhost:1480`

You could then create some model data on any controller:

```
ziti learn demo setup echo client 
ziti learn demo setup echo single-sdk-hosted
```

Any view the results on any controller

```
ziti edge ls services
ziti edge -i ctrl2 ls services
ziti edge -i ctrl3 ls services
```
