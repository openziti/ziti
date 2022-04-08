# Purpose

This script sets up an echo service which is hosted by a router-embedded tunneler and
is accessed by another sdk application.

# Prerequisites

You need at least one controller and an edge router running. for this to work.
You can use the quick-start script found [here](https://github.com/openziti/ziti/tree/release-next/quickstart).

# Setup

First, ensure we're logged into the controller.

```action:ziti-login allowRetry=true
ziti edge login
```

```action:keep-session-alive interval=1m
If your session times out you can run ziti edge login again.
```

Remove any entities from previous runs.

```action:ziti
ziti edge delete service echo
ziti edge delete config echo-host
ziti edge delete identities zcat echo-host
ziti edge delete service-policies echo-dial echo-bind
ziti edge delete edge-router-policies echo
ziti edge delete service-edge-router-policies echo 
```

Create the echo-host config

```action:ziti-create-config name=echo-host type=host.v2
{
    "terminators" : [
        {
            "address" : "localhost",
            "port" : 1234,
            "protocol" : "tcp"   
        }
    ]
}
```

Create the echo service

```action:ziti
ziti edge create service echo -c echo-host
```

Select an edge router to host this service on

```action:select-edge-router
Pick the name of an edge router to use. It will be referenced in this tutorial as ${edgeRouterName}
```

Update the select edge router with the `echo-host` attribute so it gets tied into the policies

```action:ziti templatize=true
ziti edge update identity ${edgeRouterName} -a echo-host
```

Create and enroll the client app identity

```action:ziti
ziti edge create identity service zcat -a echo,echo-client -o zcat.jwt
ziti edge enroll --rm zcat.jwt
```

Configure policies

```action:ziti
ziti edge create service-policy echo-dial Dial --service-roles @echo --identity-roles #echo-client
ziti edge create service-policy echo-bind Bind --service-roles @echo --identity-roles #echo-host
ziti edge create edge-router-policy echo --identity-roles #echo --edge-router-roles #all
ziti edge create service-edge-router-policy echo --service-roles @echo --edge-router-roles #all
```

You should now be to run the echo server with

```
ziti demo echo-server -p 1234
```

and
the zcat client using

```
ziti demo zcat -i zcat.json ziti:echo
```
