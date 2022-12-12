# Purpose

This script sets up an echo service which is hosted by two SDK applications and
is accessed by another sdk application.

# Prerequisites

You need at least one controller and an edge router running. for this to work.
You can use the quick-start script found [here](https://github.com/openziti/ziti/tree/release-next/quickstart).

# Setup

## Ensure we're logged into the controller.

```action:ziti-login allowRetry=true
ziti edge login
```

```action:keep-session-alive interval=1m
If your session times out you can run ziti edge login again.
```

## Remove any entities from previous runs.

```action:ziti
ziti edge delete service echo
ziti edge delete config echo-host
ziti edge delete identities echo-host-1 echo-host-2
ziti edge delete service-policies echo-bind
ziti edge delete edge-router-policies echo
ziti edge delete service-edge-router-policies echo 
```

## Create the echo service

```action:ziti
ziti edge create service echo -a echo
```

## Create and enroll the hosting identities

```action:ziti
ziti edge create identity service echo-host-1 -a echo,echo-host -o echo-host-1.jwt
ziti edge enroll --rm echo-host-1.jwt

ziti edge create identity service echo-host-2 -a echo,echo-host -o echo-host-2.jwt
ziti edge enroll --rm echo-host-2.jwt
```

# Configure policies

```action:ziti
ziti edge create service-policy echo-bind Bind --service-roles @echo --identity-roles #echo-host
ziti edge create edge-router-policy echo --identity-roles #echo --edge-router-roles #all
ziti edge create service-edge-router-policy echo --service-roles @echo --edge-router-roles #all
```

You should now be to run two instances of the echo server with:

```
ziti demo echo-server -i echo-host-1.json
ziti demo echo-server -i echo-host-2.json
```

and
the zcat client using

```
ziti demo zcat -i zcat.json ziti:echo
```
