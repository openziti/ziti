# HA Setup for Development

**NOTE: HA is in alpha. Expect bugs. Bug reports are appreciated**

To set up a local three node HA cluster, do the following.

## Create The Necessary PKI

Either run the `create-pki.sh` script found in the folder, or follow the steps in
the [HA PKI Guide](./dev-setup-ha-pki.md)

## Running the Controllers

1. The controller configuration files have relative paths, so make sure you're running things from
   this directory.
2. Start all three controllers
    1. `ziti controller run ctrl1.yml`
    2. `ziti controller run ctrl2.yml`
    3. `ziti controller run ctrl3.yml`
    4. All three are configured with `minClusterSize` of 3, so they will wait to be joined to a raft
       cluster
    5. The ctrl1.yml config file has the other two controllers as bootstrap members, so when it
       starts the first controller will start trying form the raft cluster.
3. Initialize the edge using the agent
    1. `ziti agent controller init -p <pid of any controller> admin admin 'Default Admin'`
    2. You can of course use different values if you desire

You should now have a three node cluster running. You can log into each controller individually.

1. `ziti edge login localhost:1280`
2. `ziti edge -i ctrl2 login localhost:1380`
3. `ziti edge -i ctrl3 login localhost:1480`

You could then create some model data on any controller:

```
# This will create the client side identity and policies
ziti demo setup echo client 

# This will create the server side identity and policies
ziti demo setup echo single-sdk-hosted
```

Any view the results on any controller

```
ziti edge login localhost:1280
ziti edge ls services

ziti edge login -i ctrl2 localhost:1380
ziti edge -i ctrl2 ls services

ziti edge login -i ctrl3 localhost:1480
ziti edge -i ctrl3 ls services
```
