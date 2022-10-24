# Overview

This tutorial covers how to deploy Ziti from the ground up on a single local machine. After following this guide, it
should be possible to extrapolate the setup to multiple machines. This setup includes both the Fabric and optional
Edge components:

- 1x Ziti Controller + Edge
- 1x Ziti Edge Router
- 1x Ziti Tunneler
- 1x Ziti Service (a demo netcat service)

It will also include provisioning and running a Ziti Tunneler with a demo netcat service. The services will use Ziti
router egress.  The full connection will be:

    SDK -> Edge Router -> Router -> Service

## Build Requirements

If you have not already built the apps in this repo you may go back to [the previous article about getting set up for local development](./002-local-dev.md) for build instructions. The remainder of the tutorial will assume you have installed all the apps so they can be found in your shell's executable search `PATH`.

## Setting up your first environment

Now that you have ziti cloned and compiling, the next step is to get your very first
environment running.  These steps will bring you through setting up your first environment
with ziti. We will:

- [Requirements](#Requirements)
- [Establish Environment Variables](#Establish-Environment-Variables)
- [Create A Certificate Authority](#Create A Certificate Authority)
- [Configure & Run A Ziti Controller](#Configure-&-Run-A-Ziti-Controller)
- [Configure & Run A Ziti Router](#Configure-&-Run-A-Ziti-Router)
- [Configure & Run A Ziti Edge Router](#Configure-&-Run-A-Ziti-Edge-Router)
- [Configure A Service & Ziti Tunneler](#Configure-A-Service-&-Ziti-Tunneler)
- [Configuring a hosted service](#Configuring-A-Hosted-Service)

## Requirements

### Required Tooling

- A bash shell
- nc (netcat)

### A note on Windows

These commands all require a running bash shell. Windows users this means you'll need to use
WSL, [cygwin](https://www.cygwin.com/), a Linux virtual machine, or some other environment that supports a bash compliant
shell. The easiest thing might just be to use the shell that comes with [git bashfor windows](https://gitforwindows.org/).
WSL is maturing more and more: [Mintty and WSL](https://github.com/mintty/wsltty).

Also note that commands for `ziti`, `ziti-controller`, and `ziti-router` may need to have the `.exe`
suffix added into the command provided in this document.

### Initialize the Environment

The remainder of this local development tutorital will instruct you to run terminal commands with current working directory of the top-level of this checked-out repo. The generated configuration files will use filesystem paths that are relative to this directory.

```bash
# this ./db directory is ignored by Git and will house the tutorial files
mkdir -p ./db
```

### Initialize the Controller

Before you can run the controller will initialize its configuration and database. We'll use the demo PKI that's checked in to this repo in `./etc/`.

```bash
ZITI_HOME=. \                              
ZITI_CTRL_LISTENER_ADDRESS=127.0.0.1 \
ZITI_CTRL_EDGE_LISTENER_HOST_PORT=127.0.0.1:1280 \
ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT=127.0.0.1:1280 \
ZITI_CTRL_IDENTITY_CERT=./etc/ca/intermediate/certs/ctrl-client.cert.pem \
ZITI_CTRL_IDENTITY_SERVER_CERT=./etc/ca/intermediate/certs/ctrl-server.cert.pem \
ZITI_CTRL_IDENTITY_KEY=./etc/ca/intermediate/private/ctrl.key.pem \
ZITI_CTRL_IDENTITY_CA=./etc/ca/intermediate/certs/ca-chain.cert.pem \
ZITI_SIGNING_CERT=./etc/ca/intermediate/certs/intermediate.cert.pem \
ZITI_SIGNING_KEY=./etc/ca/intermediate/private/intermediate.key.decrypted.pem \
    ziti create config controller \
        --output ./db/ctrl-config.yml
```

```bash
ziti-controller edge init ./db/ctrl-config.yml -u ADMIN_NAME -p ADMIN_PW
```

### Run the Controller

Edge SDKs will connect to the running controller to authenticate and request a session.

```bash
ziti-controller run ./db/ctrl-config.yml
```

### Login to the Controller

This step will save a session token in the `ziti` CLI's configuration cache.

```bash
ziti edge login -u ADMIN_NAME -p ADMIN_PW
```

Subsequent `ziti` CLI commands will automatically re-use this session token. You'll need to perform this login step again when the token expires.

### Initialize an Edge Router

Request an enrollment token from the controller for router01.

```bash
ziti edge create edge-router router01 \
    --jwt-output-file /tmp/router01.jwt \
    --tunneler-enabled
```

Generate a configuration file for router01.

```bash
ZITI_HOME=./db \
ZITI_CTRL_ADVERTISED_ADDRESS=127.0.0.1 \
ZITI_EDGE_ROUTER_RAWNAME=localhost \
    ziti create config router edge \
    --routerName router01 \
    --output ./db/router01-config.yml
```

Enroll router01 by presenting the token to the controller to receive a certificate in the filesystem location specified in the configuration file.

```bash
ziti-router enroll --jwt /tmp/router01.jwt ./db/router01-config.yml
```

### Run the Edge Router

Edge SDKs will connect to the running edge router to connect to services.

```bash
ziti-router run ./db/router01-config.yml
```

## Configure A Service & Ziti Tunneler

1. Create a service that will facilitate connecting to a local netcat server listening on port 7256 and that egresses
the Ziti Fabric on our "r01" router

        ziti edge create service netcat7256 localhost 7256 r01 tcp://localhost:7256

1. Create a Ziti Edge Identity for the Ziti Tunneler process

        ziti edge create identity device identity01 -o $ZITI_HOME/identity01.jwt

1. Create an AppWan to associate the Ziti Tunneler identity (identity01) to the service (netcat7256)

        ziti edge create app-wan appwan01 -s netcat7256 -i identity01

1. Enroll the Ziti Tunneler's identity

        ziti-enroller --jwt $ZITI_HOME/identity01.jwt -o $ZITI_HOME/identity01.json

1. Start the Ziti Tunneler in proxy mode

        ziti-tunnel proxy netcat7256:8145 -i $ZITI_HOME/identity01.json

1. Start the netcat server

        nc -k -n 127.0.0.1 -l 7256

1. Start the netcat client that will connect to the Ziti Tunnel proxy

        nc -v 127.0.0.1 8145

## Configuring A Hosted Service

Services can also be hosted by another SDK. For this to work a second Ziti Tunneler can be setup to act as the host. The
exact details of this are beyond this document but the high level steps are outlined below:

1. Create a new Ziti Identity via `ziti edge create identity` to allow another Ziti Tunneler to run
1. Enroll the Ziti Identitiy via `ziti enroll`
1. Add the new Ziti Identity to the hosts of a new service or an existing service
1. Start the new Ziti Tunneler

Example Hosted Service

    ziti edge create service <service name> <dns host> <dns port> \
    --hosted
    --hosted-ids <someIdentityId>
    --tags tunneler.dial.addr=<address for tunneler to dial>

Example:

    ziti edge create service postgresql pg 5432 \-
    --hosted
    --hosted-ids 40c025cb-bc92-4a54-b55f-1429412f2644
    -tags tunneler.dial.addr=tcp:127.0.0.1:5432
