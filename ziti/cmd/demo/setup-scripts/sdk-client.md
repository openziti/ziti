# Purpose

This script sets up the SDK client side for an echo service

# Prerequisites

You need at least one controller and an edge router running. for this to work.
You can use the quick-start script found [here](https://github.com/openziti/ziti/tree/release-next/quickstart).

# Setup

## Ensure we're logged into the controller

```action:ziti-login allowRetry=true
ziti edge login
```

<!--action:keep-session-alive interval=1m quiet=false-->

## Remove any entities from previous runs

```action:ziti
ziti edge delete identities zcat 
ziti edge delete service-policies echo-dial
```

## Create and enroll the client app identity

```action:ziti
ziti edge create identity service zcat -a echo,echo-client -o zcat.jwt
ziti edge enroll --rm zcat.jwt
```

## Configure a dial policy

```action:ziti
ziti edge create service-policy echo-dial Dial --service-roles #echo --identity-roles #echo-client
```

## Summary

After you've configured the service side, you should now be to run the zcat client using

```
ziti demo zcat -i zcat.json ziti:echo
```
