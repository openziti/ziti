# HA Setup for Development

**NOTE: HA is a work in progress and not yet usable for anything other than developing HA**

To set up a local three node HA cluster, do the following.

1. Ensure ZITI_SOURCE is set to the directory under which you have ziti checked out
    1. Ex: if you have ziti checked out to ~/work/ziti, then set ZITI_SOURCE to ~/work
1. Create a directory where you want to store controller databases and raft data. 
1. Set ZITI_DATA to this directory
1. Run `ziti-controller run ctrl1.yml` with the ctrl1.yml in this directory.
    1. This first controller is going to start a 1 node cluster, because raft/minClusterSize is set to 1
1. Initialize the edge by doing `ziti agent controller init admin admin 'Default Admin'` 
    1. You can of course use different values if you desire
1. Start the second and third controllers
    1. These are both configured with `minClusterSize` of 3, so they will wait to be joined to a raft cluster
1. Find the pid of the first ziti-controller instance
1. Add the first controller
    1. `ziti agent controller raft-join <pid of first controller> $(cat $ZITI_DATA/ctrl1/id) tls:localhost:6363`
1. Join the second controller
    1. `ziti agent controller raft-join <pid of first controller> $(cat $ZITI_DATA/ctrl2/id) tls:localhost:6464`

You should now have a three node cluster running. You can log into each controller individually.

1. `ziti edge login localhost:1280`
1. `ziti edge -i ctrl2 login localhost:1380`
1. `ziti edge -i ctrl3 login localhost:1480`

You could then create some model data on any controller:

```
ziti demo setup echo client 
ziti demo setup echo single-sdk-hosted
```

Any view the results on any controller

```
ziti edge ls services
ziti edge -i ctrl2 ls services
ziti edge -i ctrl3 ls services
```
