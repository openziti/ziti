# HA Setup for Development

**NOTE: HA is in beta. Bug reports are appreciated**

To set up a local three node HA cluster, do the following.

## Create The Necessary PKI

Run the `create-pki.sh` script found in the folder.

## Running the Controllers

1. The controller configuration files have relative paths, so make sure you're running things from
   this directory.
2. Start all three controllers
    1. `ziti controller run ctrl1.yml`
    1. `ziti controller run ctrl2.yml`
    1. `ziti controller run ctrl3.yml`
1. Initialize the first controller using the agent
    1. `ziti agent cluster init -i ctrl1 admin admin 'Default Admin'`
1. Add the other two nodes to the cluster
    1. `ziti agent cluster add -i ctrl1 tls:localhost:6363`
    1. `ziti agent cluster add -i ctrl1 tls:localhost:6464`

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

## Running HA Go SDKs 

As of Golang SDK v1.2.3, no special changes should be required to work with HA systems.